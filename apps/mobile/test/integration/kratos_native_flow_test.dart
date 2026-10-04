// Real-stack test against the local compose stack (Kratos public :4433,
// Mailpit :8025, identity-service :8080). Every flow that takes an
// identifier resolves it first (`POST /v1/auth/identifiers`, ADR-0013), so
// identity-service is required. Codes for phone logins arrive through the
// local SMS sink (Mailpit, `<E.164 digits>@sms.local`). Plain `test` (no
// widget binding) so real HTTP works. Excluded by default; run with:
//
//   flutter test --tags integration
//
// Override endpoints with env vars KRATOS_PUBLIC_URL, MAILPIT_URL, API_URL.
@Tags(['integration'])
library;

import 'dart:io';
import 'dart:math';

import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/identity/login_identifier_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/ory_kratos_client.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';
import 'package:go_ory_auth_mobile/core/network/api_client.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/storage/login_identifier_cache.dart';
import 'package:go_ory_auth_mobile/core/storage/secure_token_store.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/profile/data/profile_repository.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info.dart';
import 'package:go_ory_auth_mobile/features/settings/data/settings_repository.dart';
import 'package:test/test.dart';

final Map<String, String> _env = Platform.environment;
final String kratosUrl = _env['KRATOS_PUBLIC_URL'] ?? 'http://localhost:4433';
final String mailpitUrl = _env['MAILPIT_URL'] ?? 'http://localhost:8025';
final String apiUrl = _env['API_URL'] ?? 'http://localhost:8080';

final Random _rng = Random.secure();

// Test PII values: asserted absent from every captured log line.
const _piiPhone = '+84901234567';
const _piiLine1 = '12 Integration Street';
const _piiIdNumber = '079123456123';

/// Progress lines (never contain credentials).
void _report(String line) => stdout.writeln(line);
String _randomAlnum(int n) {
  const chars = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  return List.generate(n, (_) => chars[_rng.nextInt(chars.length)]).join();
}

/// Strong random password (never in a breach list, never logged).
String strongPassword() => '${_randomAlnum(20)}!#${_randomAlnum(4)}';

/// identity-service message templates (vi/en, domain/login/messages.go).
bool _isRecovery(String snippet) {
  final s = snippet.toLowerCase();
  return s.contains('khôi phục') ||
      s.contains('khoi phuc') ||
      s.contains('recovery');
}

bool _isVerification(String snippet) {
  final s = snippet.toLowerCase();
  return s.contains('xác minh') ||
      s.contains('xac minh') ||
      s.contains('verification');
}

/// Polls Mailpit for the newest message to [email] (an email address or the
/// SMS sink address) created after [after] whose snippet satisfies [match],
/// and returns the 6-digit code.
Future<String> codeFromMailpit(
  String email, {
  required DateTime after,
  bool Function(String snippet)? match,
}) async {
  final mp = Dio(BaseOptions(baseUrl: mailpitUrl));
  for (var i = 0; i < 40; i++) {
    final res = await mp.get<Map<String, dynamic>>('/api/v1/messages');
    final messages = (res.data!['messages'] as List)
        .cast<Map<String, dynamic>>();
    for (final m in messages) {
      final to = (m['To'] as List).map((t) => (t as Map)['Address']).toList();
      final created = DateTime.parse(m['Created'] as String);
      final snippet = m['Snippet'] as String? ?? '';
      if (to.contains(email) &&
          created.isAfter(after) &&
          (match == null || match(snippet))) {
        final code = RegExp(r'\b(\d{6})\b').firstMatch(snippet)?.group(1);
        if (code != null) return code;
      }
    }
    await Future<void>.delayed(const Duration(milliseconds: 500));
  }
  throw StateError('no code email for $email in Mailpit');
}

Future<bool> identityServiceUp() async {
  try {
    await Dio(
      BaseOptions(
        connectTimeout: const Duration(seconds: 2),
        validateStatus: (_) => true,
      ),
    ).get<dynamic>('$apiUrl/v1/me');
    return true;
  } on DioException {
    return false;
  }
}

final RegExp _pseudonym = RegExp(r'^[a-z2-7]{52}@login\.invalid$');

void main() {
  final logLines = <String>[];
  final logger = AppLogger(sink: (_, m) => logLines.add(m));
  final kratos = OryKratosClient(baseUrl: kratosUrl);
  final resolver = HttpLoginIdentifierResolver(
    buildPublicApiDio(baseUrl: apiUrl, logger: logger),
  );
  late InMemoryTokenStore tokens;
  late AuthRepository auth;

  AuthRepository newAuth(TokenStore store) => AuthRepository(
    kratos: kratos,
    tokens: store,
    resolver: resolver,
    loginCache: InMemoryLoginIdentifierCache(),
  );

  final email =
      'mobile-it-${DateTime.now().millisecondsSinceEpoch}-'
      '${_randomAlnum(6).toLowerCase()}@example.com';
  final emailLogin = LoginInput(type: LoginType.email, value: email);
  // Login contacts of this run: asserted absent from every log line too.
  final loginValues = <String>[email];
  var password = strongPassword();

  setUpAll(() {
    tokens = InMemoryTokenStore();
    auth = newAuth(tokens);
  });

  test('registration -> token -> verify (Mailpit code) -> toSession', () async {
    final t0 = DateTime.now().toUtc().subtract(const Duration(seconds: 2));
    final regFlow = await auth.startRegistration();
    expect(regFlow.type, 'api');
    expect(regFlow.node('traits.login_id'), isNotNull);

    final outcome = await auth.register(
      flowId: regFlow.id,
      login: emailLogin,
      password: password,
    );
    final token = await tokens.read();
    expect(token, startsWith('ory_st_'));
    // Kratos holds the pseudonym only (ADR-0013).
    expect(outcome.session.identity.loginId, matches(_pseudonym));
    expect(outcome.session.identity.traits.containsKey('email'), isFalse);
    expect(outcome.session.identity.schemaId, 'customer');
    expect(outcome.session.identity.loginVerified, isFalse);

    // Follow continue_with show_verification_ui when Kratos sends it;
    // otherwise start a native verification flow (sends a fresh code).
    var verificationFlowId = outcome.verificationFlowId;
    var after = t0;
    if (verificationFlowId == null) {
      after = DateTime.now().toUtc().subtract(const Duration(seconds: 1));
      final vf = await auth.startVerification(
        PseudonymousLogin(outcome.session.identity.loginId),
      );
      expect(vf.state, 'sent_email');
      verificationFlowId = vf.id;
    }
    final flowId = verificationFlowId;
    // Delivered to the real address by identity-service's courier hook.
    final code = await codeFromMailpit(
      email,
      after: after,
      match: _isVerification,
    );

    // Wrong code first: surfaced as FlowValidationFailure with message id.
    await expectLater(
      auth.verify(flowId: flowId, code: code == '000000' ? '111111' : '000000'),
      throwsA(
        isA<FlowValidationFailure>().having(
          (f) => f.flow.messages.map((m) => m.id),
          'ids',
          contains(4070006),
        ),
      ),
    );

    final verified = await auth.verify(flowId: flowId, code: code);
    expect(verified.state, 'passed_challenge');

    final session = await auth.refreshSession();
    expect(session.active, isTrue);
    expect(session.identity.loginVerified, isTrue);
    _report(
      '[it] registration+verification OK (continue_with show_verification_ui '
      '${outcome.verificationFlowId == null ? 'absent, fallback' : 'present'})',
    );
  });

  test(
    'settings flow: weak password rejected by Kratos, strong one accepted',
    () async {
      final settings = SettingsRepository(kratos: kratos, tokens: tokens);
      // PLI-FR-09: re-authentication uses the session's pseudonym as is.
      final current = await auth.refreshSession();
      await settings.reauthenticate(
        identifier: current.identity.loginId,
        password: password,
      );
      final flow = await settings.startSettings();
      await expectLater(
        settings.changePassword(flowId: flow.id, password: 'password1234'),
        throwsA(
          isA<FlowValidationFailure>().having(
            (f) => f.flow.messagesFor('password').map((m) => m.id),
            'ids',
            isNotEmpty,
          ),
        ),
      );
      final again = await settings.startSettings();
      password = strongPassword();
      final ok = await settings.changePassword(
        flowId: again.id,
        password: password,
      );
      expect(ok.state, 'success');
    },
  );

  test('recovery (code) -> privileged session -> new password', () async {
    final t0 = DateTime.now().toUtc().subtract(const Duration(seconds: 1));
    final flow = await auth.startRecovery();
    final sent = await auth.requestRecoveryCode(
      flowId: flow.id,
      login: LoginInput(type: LoginType.email, value: email.toUpperCase()),
    );
    expect(sent.state, 'sent_email');
    // Codes travel through the Kratos http courier, so an earlier
    // verification mail can land after t0: accept only a recovery message.
    final code = await codeFromMailpit(email, after: t0, match: _isRecovery);
    try {
      final grant = await auth.submitRecoveryCode(flowId: flow.id, code: code);
      password = strongPassword();
      final s = await auth.completeRecovery(grant: grant, password: password);
      expect(s.identity.loginId, matches(_pseudonym));
      _report('[it] recovery OK');
    } on FlowValidationFailure catch (f) {
      _report(
        '[it] recovery validation: state=${f.flow.state} '
        'msgs=${f.flow.messages.map((m) => m.id)} '
        'nodes=${f.flow.nodes.expand((n) => n.messages).map((m) => m.id)}',
      );
      rethrow;
    } on ApiFailure catch (f) {
      if (f.code == 'recovery_session_unavailable') {
        fail(
          'Kratos returned 422 browser_location_change_required for native '
          'recovery: enable feature_flags.use_continue_with_transitions',
        );
      }
      rethrow;
    }
  });

  test(
    'native login + GET /v1/me (only when identity-service answers)',
    () async {
      if (!await identityServiceUp()) {
        markTestSkipped('identity-service not reachable at $apiUrl');
        return;
      }
      final loginTokens = InMemoryTokenStore();
      final loginRepo = newAuth(loginTokens);
      final flow = await loginRepo.startLogin();
      final session = await loginRepo.login(
        flowId: flow.id,
        login: emailLogin,
        password: password,
      );
      expect(session.identity.loginId, matches(_pseudonym));

      var unauthorized = 0;
      final profile = ProfileRepository(
        buildApiDio(
          baseUrl: apiUrl,
          readToken: loginTokens.read,
          onUnauthorized: () async => unauthorized++,
          logger: logger,
        ),
      );
      final me = await profile.getMe();
      expect(me.login.type, LoginType.email);
      expect(me.login.value, email);
      expect(me.emailVerified, isTrue);
      expect(unauthorized, 0);
      _report('[it] login + GET /v1/me OK');
      await loginRepo.logout();
    },
  );

  test(
    'PUT/GET/DELETE /v1/me/personal-info (only when the endpoint exists)',
    () async {
      if (!await identityServiceUp()) {
        markTestSkipped('identity-service not reachable at $apiUrl');
        return;
      }
      final loginTokens = InMemoryTokenStore();
      final loginRepo = newAuth(loginTokens);
      final flow = await loginRepo.startLogin();
      await loginRepo.login(
        flowId: flow.id,
        login: emailLogin,
        password: password,
      );
      final profile = ProfileRepository(
        buildApiDio(
          baseUrl: apiUrl,
          readToken: loginTokens.read,
          onUnauthorized: () async {},
          logger: logger,
        ),
      );
      try {
        try {
          await profile.getPersonalInfo();
        } on ApiFailure catch (f) {
          if (f.status == 404) {
            markTestSkipped(
              'GET /v1/me/personal-info returned 404: endpoint not deployed '
              'in this identity-service build yet',
            );
            return;
          }
          rethrow;
        }
        final stored = await profile.putPersonalInfo(
          PersonalInfo(
            phoneNumber: _piiPhone,
            dateOfBirth: DateTime(1990, 5, 17),
            address: const Address(
              line1: _piiLine1,
              city: 'Ho Chi Minh City',
              country: 'VN',
            ),
            nationalId: const NationalId(type: 'cccd', number: _piiIdNumber),
          ),
        );
        expect(stored.phoneNumber, _piiPhone);
        final read = await profile.getPersonalInfo();
        expect(read.phoneNumber, _piiPhone);
        expect(read.dateOfBirth, DateTime(1990, 5, 17));
        expect(read.address?.line1, _piiLine1);
        expect(read.address?.country, 'VN');
        expect(read.nationalId?.number, _piiIdNumber);

        await expectLater(
          profile.putPersonalInfo(const PersonalInfo(phoneNumber: '12')),
          throwsA(
            isA<ApiFailure>()
                .having((f) => f.status, 'status', 422)
                .having(
                  (f) => f.fieldErrors.map((e) => e.field),
                  'fields',
                  contains('phone_number'),
                ),
          ),
        );

        await profile.erasePersonalInfo();
        final erased = await profile.getPersonalInfo();
        expect(erased.isEmpty, isTrue);
        _report('[it] personal-info put/get/erase OK');
      } finally {
        await loginRepo.logout();
      }
    },
  );

  test('logout revokes the session token', () async {
    final token = await tokens.read();
    expect(token, isNotNull);
    await auth.logout();
    expect(await tokens.read(), isNull);
    await expectLater(
      kratos.toSession(sessionToken: token!),
      throwsA(isA<UnauthenticatedFailure>()),
    );
  });

  test('phone login: register -> SMS code -> verify -> sign out -> sign in '
      '(national format) -> recovery code', () async {
    final digits = '849${10000000 + _rng.nextInt(89999999)}';
    final phone = '+$digits';
    final national = '0${digits.substring(2)}';
    final sink = '$digits@sms.local';
    loginValues.addAll([phone, national]);
    final phoneTokens = InMemoryTokenStore();
    final phoneAuth = newAuth(phoneTokens);
    final phonePassword = strongPassword();

    final t0 = DateTime.now().toUtc().subtract(const Duration(seconds: 2));
    final reg = await phoneAuth.startRegistration();
    final outcome = await phoneAuth.register(
      flowId: reg.id,
      login: LoginInput(type: LoginType.phone, value: phone),
      password: phonePassword,
    );
    expect(outcome.session.identity.loginId, matches(_pseudonym));
    var flowId = outcome.verificationFlowId;
    if (flowId == null) {
      final vf = await phoneAuth.startVerification(
        PseudonymousLogin(outcome.session.identity.loginId),
      );
      flowId = vf.id;
    }
    final code = await codeFromMailpit(sink, after: t0, match: _isVerification);
    final verified = await phoneAuth.verify(flowId: flowId, code: code);
    expect(verified.state, 'passed_challenge');
    await phoneAuth.logout();

    final login = await phoneAuth.startLogin();
    final session = await phoneAuth.login(
      flowId: login.id,
      login: LoginInput(type: LoginType.phone, value: national),
      password: phonePassword,
    );
    expect(session.identity.loginId, outcome.session.identity.loginId);
    expect(session.identity.loginVerified, isTrue);

    if (await identityServiceUp()) {
      final profile = ProfileRepository(
        buildApiDio(
          baseUrl: apiUrl,
          readToken: phoneTokens.read,
          onUnauthorized: () async {},
          logger: logger,
        ),
      );
      final me = await profile.getMe();
      expect(me.login.type, LoginType.phone);
      expect(me.login.value, phone);
      expect(me.email, isNull);
    }
    await phoneAuth.logout();

    final t1 = DateTime.now().toUtc().subtract(const Duration(seconds: 1));
    final rec = await phoneAuth.startRecovery();
    final sent = await phoneAuth.requestRecoveryCode(
      flowId: rec.id,
      login: LoginInput(type: LoginType.phone, value: national),
    );
    expect(sent.state, 'sent_email');
    expect(
      await codeFromMailpit(sink, after: t1, match: _isRecovery),
      hasLength(6),
    );
    _report('[it] phone register/verify/sign-in/recovery OK');
  });

  tearDownAll(() {
    for (final l in logLines) {
      expect(l, isNot(contains('ory_st_')), reason: 'token leaked into logs');
      for (final v in [_piiPhone, _piiLine1, _piiIdNumber, ...loginValues]) {
        expect(l, isNot(contains(v)), reason: 'PII leaked into logs');
      }
    }
  });
}
