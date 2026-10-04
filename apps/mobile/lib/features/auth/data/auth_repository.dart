import 'package:go_ory_auth_mobile/core/identity/login_identifier_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_client.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_error_mapper.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/storage/login_identifier_cache.dart';
import 'package:go_ory_auth_mobile/core/storage/secure_token_store.dart';

/// Result of a successful native registration.
class RegistrationOutcome {
  const RegistrationOutcome({required this.session, this.verificationFlowId});

  final KratosSession session;

  /// From `continue_with: show_verification_ui`; null when Kratos did not
  /// send it (then the verify screen starts its own verification flow).
  final String? verificationFlowId;
}

/// Privileged session granted by a successful recovery code.
class RecoveryGrant {
  const RecoveryGrant({
    required this.settingsFlowId,
    required this.sessionToken,
  });

  final String settingsFlowId;
  final String sessionToken;

  @override
  String toString() => 'RecoveryGrant($settingsFlowId, <redacted>)';
}

/// Registration, login, verification, recovery and logout over Kratos native
/// flows. Owns writing/wiping the session token. Throws [AppFailure] only.
///
/// Every flow that takes an identifier resolves the email / phone number to
/// its pseudonym first (`POST /v1/auth/identifiers`, DD-18) and sends only
/// the pseudonym to Kratos (ADR-0013).
class AuthRepository {
  AuthRepository({
    required KratosClient kratos,
    required TokenStore tokens,
    required LoginIdentifierResolver resolver,
    required LoginIdentifierCache loginCache,
  }) : _kratos = kratos,
       _tokens = tokens,
       _resolver = resolver,
       _loginCache = loginCache;

  final KratosClient _kratos;
  final TokenStore _tokens;
  final LoginIdentifierResolver _resolver;
  final LoginIdentifierCache _loginCache;

  /// The pseudonym resolved for the last registration attempt, kept so a
  /// resubmit of the same flow with the same input (e.g. after Kratos
  /// rejected the password) does not resolve again: registration resolves
  /// are rate-limited per client IP (5/min, 30/day).
  ({String flowId, LoginInput login, String loginId})? _registration;

  /// The Kratos identifier for [target]: the session's pseudonym as is, or
  /// the resolver's answer for what the customer typed.
  Future<String> _identifierFor(LoginTarget target, LoginPurpose purpose) =>
      switch (target) {
        // No pseudonym in the session: never address Kratos with "".
        PseudonymousLogin(:final identifier) when identifier.isEmpty =>
          Future.error(const UnauthenticatedFailure()),
        PseudonymousLogin(:final identifier) => Future.value(identifier),
        final LoginInput input => _resolver.resolve(input, purpose),
      };

  Future<T> _guard<T>(Future<T> Function() body) async {
    try {
      return await body();
    } on Object catch (e) {
      throw mapKratosError(e);
    }
  }

  // --- session ------------------------------------------------------------

  Future<String?> currentToken() => _tokens.read();

  /// Startup check: `200` → session, `401` → token wiped, `null`.
  Future<KratosSession?> restoreSession() => _guard(() async {
    final token = await _tokens.read();
    if (token == null || token.isEmpty) return null;
    try {
      return await _kratos.toSession(sessionToken: token);
    } on UnauthenticatedFailure {
      await _tokens.clear();
      return null;
    }
  });

  /// Re-reads the session (e.g. after verification flips `verified`).
  Future<KratosSession> refreshSession() => _guard(() async {
    final token = await _tokens.read();
    if (token == null) throw const UnauthenticatedFailure();
    return await _kratos.toSession(sessionToken: token);
  });

  Future<KratosSession> _persist(NativeAuthResult result) async {
    if (result.sessionToken.isEmpty) {
      throw const ApiFailure('session_token_missing');
    }
    await _tokens.write(result.sessionToken);
    return result.session ??
        await _kratos.toSession(sessionToken: result.sessionToken);
  }

  // --- registration ------------------------------------------------------

  Future<KratosFlow> startRegistration() =>
      _guard(_kratos.createRegistrationFlow);

  Future<RegistrationOutcome> register({
    required String flowId,
    required LoginInput login,
    required String password,
  }) => _guard(() async {
    final memo = _registration;
    final String loginId;
    if (memo != null && memo.flowId == flowId && memo.login == login) {
      loginId = memo.loginId;
    } else {
      loginId = await _identifierFor(login, LoginPurpose.registration);
      _registration = (flowId: flowId, login: login, loginId: loginId);
    }
    final result = await _kratos.submitRegistration(
      flowId: flowId,
      loginId: loginId,
      password: password,
    );
    _registration = null;
    final session = await _persist(result);
    return RegistrationOutcome(
      session: session,
      verificationFlowId: result.verificationFlowId,
    );
  });

  // --- login --------------------------------------------------------------

  Future<KratosFlow> startLogin() => _guard(_kratos.createLoginFlow);

  /// Signs in with [login]. Re-uses the pseudonym cached by the last
  /// successful sign-in with the same input; resolves it otherwise. A
  /// rejected cached pseudonym is dropped so the next attempt resolves again
  /// (no automatic retry: a wrong password must not count twice).
  Future<KratosSession> login({
    required String flowId,
    required LoginInput login,
    required String password,
  }) => _guard(() async {
    final cached = await _readCache(login);
    final identifier =
        cached ?? await _identifierFor(login, LoginPurpose.signIn);
    final NativeAuthResult result;
    try {
      result = await _kratos.submitLogin(
        flowId: flowId,
        identifier: identifier,
        password: password,
      );
    } on FlowValidationFailure {
      if (cached != null) await _removeCache(login);
      rethrow;
    }
    final session = await _persist(result);
    await _writeCache(login, identifier);
    return session;
  });

  // The cache is an optimisation: storage errors never fail a sign-in.
  Future<String?> _readCache(LoginInput login) async {
    try {
      final v = await _loginCache.read(login);
      return v == null || v.isEmpty ? null : v;
    } on Object {
      return null;
    }
  }

  Future<void> _writeCache(LoginInput login, String identifier) async {
    try {
      await _loginCache.write(login, identifier);
    } on Object {
      // Resolved again next time.
    }
  }

  Future<void> _clearCache() async {
    try {
      await _loginCache.clear();
    } on Object {
      // Ignored: see _writeCache.
    }
  }

  Future<void> _removeCache(LoginInput login) async {
    try {
      await _loginCache.remove(login);
    } on Object {
      // Ignored: see _writeCache.
    }
  }

  // --- verification -------------------------------------------------------

  /// Creates a verification flow and asks Kratos to send a code (email or
  /// SMS, chosen server side from the pseudonym).
  Future<KratosFlow> startVerification(LoginTarget target) => _guard(() async {
    final loginId = await _identifierFor(target, LoginPurpose.verification);
    final flow = await _kratos.createVerificationFlow();
    return await _kratos.submitVerification(flowId: flow.id, email: loginId);
  });

  /// Resends a code on an existing flow.
  Future<KratosFlow> resendVerificationCode({
    required String flowId,
    required LoginTarget target,
  }) => _guard(() async {
    final loginId = await _identifierFor(target, LoginPurpose.verification);
    return _codeResult(
      await _kratos.submitVerification(flowId: flowId, email: loginId),
    );
  });

  /// Submits the emailed code. A wrong code comes back as `200` with an error
  /// message, which is surfaced as [FlowValidationFailure].
  Future<KratosFlow> verify({required String flowId, required String code}) =>
      _guard(() async {
        final flow = await _kratos.submitVerification(
          flowId: flowId,
          code: code,
        );
        if (flow.state == 'passed_challenge') return flow;
        return _codeResult(flow);
      });

  KratosFlow _codeResult(KratosFlow flow) {
    if (flow.hasErrors) throw FlowValidationFailure(flow);
    return flow;
  }

  // --- recovery -----------------------------------------------------------

  Future<KratosFlow> startRecovery() => _guard(_kratos.createRecoveryFlow);

  Future<KratosFlow> requestRecoveryCode({
    required String flowId,
    required LoginInput login,
  }) => _guard(() async {
    final loginId = await _identifierFor(login, LoginPurpose.recovery);
    return _codeResult(
      await _kratos.submitRecovery(flowId: flowId, email: loginId),
    );
  });

  /// Exchanges the recovery code for a privileged session + settings flow
  /// (`continue_with`).
  Future<RecoveryGrant> submitRecoveryCode({
    required String flowId,
    required String code,
  }) => _guard(() async {
    final KratosFlow flow;
    try {
      flow = await _kratos.submitRecovery(flowId: flowId, code: code);
    } on ApiFailure catch (e) {
      if (e.code == 'browser_location_change_required') {
        // Kratos answered with the browser redirect instead of
        // continue_with (needs `feature_flags.use_continue_with_transitions`).
        throw const ApiFailure('recovery_session_unavailable', status: 422);
      }
      rethrow;
    }
    String? token;
    String? settingsFlowId;
    for (final c in flow.continueWith) {
      if (c is ContinueWithSetSessionToken) token = c.sessionToken;
      if (c is ContinueWithSettingsUi) settingsFlowId = c.flowId;
    }
    if (token == null || token.isEmpty) {
      _codeResult(flow);
      throw const ApiFailure('recovery_session_unavailable');
    }
    settingsFlowId ??= (await _kratos.createSettingsFlow(sessionToken: token))
        .id;
    return RecoveryGrant(settingsFlowId: settingsFlowId, sessionToken: token);
  });

  /// Sets the new password in the privileged settings flow, then signs in
  /// with the recovery session.
  Future<KratosSession> completeRecovery({
    required RecoveryGrant grant,
    required String password,
  }) => _guard(() async {
    try {
      await _kratos.submitSettingsPassword(
        flowId: grant.settingsFlowId,
        sessionToken: grant.sessionToken,
        password: password,
      );
    } on FlowExpiredFailure {
      final fresh = await _kratos.createSettingsFlow(
        sessionToken: grant.sessionToken,
      );
      await _kratos.submitSettingsPassword(
        flowId: fresh.id,
        sessionToken: grant.sessionToken,
        password: password,
      );
    }
    await _tokens.write(grant.sessionToken);
    return await _kratos.toSession(sessionToken: grant.sessionToken);
  });

  /// The user left the recovery flow before setting a password: revoke the
  /// privileged session Kratos issued for it (best effort).
  Future<void> abandonRecovery(RecoveryGrant grant) async {
    try {
      await _kratos.logout(sessionToken: grant.sessionToken);
    } on Object {
      // Expires on its own (privileged window); nothing else to do.
    }
  }

  // --- logout -------------------------------------------------------------

  /// `performNativeLogout` then wipe storage (session token and every cached
  /// pseudonym). Storage is wiped even if Kratos is unreachable or the
  /// session is already gone.
  Future<void> logout() async {
    final token = await _tokens.read();
    _registration = null;
    try {
      if (token != null && token.isNotEmpty) {
        await _kratos.logout(sessionToken: token);
      }
    } on Object {
      // Best effort: the local credential is removed regardless.
    } finally {
      await _tokens.clear();
      await _clearCache();
    }
  }

  /// Local wipe only (used on `401` from identity-service).
  Future<void> forget() => _tokens.clear();
}
