import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_client.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_error_mapper.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
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

/// Registration, login, verification, recovery and logout. Owns
/// writing/wiping the session token. Throws [AppFailure] only.
///
/// What the customer types (email / phone number) goes only to
/// identity-service ([CustomerAuthApi], ADR-0014), which signs in, registers
/// and starts recovery with the account's opaque Kratos handle. Steps that
/// need no address (codes, settings, logout) talk to Kratos native flows
/// directly; verification uses the signed-in owner's own handle.
class AuthRepository {
  AuthRepository({
    required KratosClient kratos,
    required TokenStore tokens,
    required CustomerAuthApi customerAuth,
  }) : _kratos = kratos,
       _tokens = tokens,
       _customerAuth = customerAuth;

  final KratosClient _kratos;
  final TokenStore _tokens;
  final CustomerAuthApi _customerAuth;

  Future<T> _guard<T>(Future<T> Function() body) async {
    try {
      return await body();
    } on Object catch (e) {
      throw mapKratosError(e);
    }
  }

  /// The signed-in owner's handle; Kratos is never addressed with "".
  String _handle(String loginId) {
    if (loginId.isEmpty) throw const UnauthenticatedFailure();
    return loginId;
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

  // --- registration and login (identity-service) --------------------------

  Future<RegistrationOutcome> register({
    required LoginInput login,
    required String password,
  }) => _guard(() async {
    final result = await _customerAuth.register(login, password);
    final session = await _persist(result);
    return RegistrationOutcome(
      session: session,
      verificationFlowId: result.verificationFlowId,
    );
  });

  /// Signs in with what the customer typed: one call to identity-service.
  Future<KratosSession> login({
    required LoginInput login,
    required String password,
  }) => _guard(() async {
    final result = await _customerAuth.login(login, password);
    return await _persist(result);
  });

  // --- verification -------------------------------------------------------

  /// Creates a verification flow and asks Kratos to send a code (email or
  /// SMS, chosen server side from the handle). [loginId] is the signed-in
  /// owner's handle (`traits.login_id`).
  Future<KratosFlow> startVerification({required String loginId}) =>
      _guard(() async {
        final email = _handle(loginId);
        final flow = await _kratos.createVerificationFlow();
        return await _kratos.submitVerification(flowId: flow.id, email: email);
      });

  /// Resends a code on an existing flow.
  Future<KratosFlow> resendVerificationCode({
    required String flowId,
    required String loginId,
  }) => _guard(() async {
    final email = _handle(loginId);
    return _codeResult(
      await _kratos.submitVerification(flowId: flowId, email: email),
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

  /// Starts recovery through identity-service, which answers the same
  /// whether or not the account exists. The returned flow's id is the sealed
  /// `recovery_id` the code is then submitted with.
  Future<KratosFlow> requestRecoveryCode({required LoginInput login}) =>
      _guard(() async {
        final flowId = await _customerAuth.startRecovery(login);
        // Kratos's own "code sent if the account exists" notice (1060003).
        return KratosFlow(
          id: flowId,
          state: 'sent_email',
          messages: const [UiText(id: 1060003, text: '', type: 'info')],
        );
      });

  /// Exchanges the recovery code for a privileged session + settings flow,
  /// through identity-service: [flowId] is the sealed `recovery_id` from
  /// [requestRecoveryCode], never a Kratos flow id.
  Future<RecoveryGrant> submitRecoveryCode({
    required String flowId,
    required String code,
  }) => _guard(() async {
    final g = await _customerAuth.submitRecoveryCode(flowId, code);
    final settingsFlowId = g.settingsFlowId.isNotEmpty
        ? g.settingsFlowId
        : (await _kratos.createSettingsFlow(sessionToken: g.sessionToken)).id;
    return RecoveryGrant(
      settingsFlowId: settingsFlowId,
      sessionToken: g.sessionToken,
    );
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

  /// `performNativeLogout` then wipe the session token. Storage is wiped
  /// even if Kratos is unreachable or the session is already gone.
  Future<void> logout() async {
    final token = await _tokens.read();
    try {
      if (token != null && token.isNotEmpty) {
        await _kratos.logout(sessionToken: token);
      }
    } on Object {
      // Best effort: the local credential is removed regardless.
    } finally {
      await _tokens.clear();
    }
  }

  /// Local wipe only (used on `401` from identity-service).
  Future<void> forget() => _tokens.clear();
}
