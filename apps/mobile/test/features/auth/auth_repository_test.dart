import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/storage/secure_token_store.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockKratosClient kratos;
  late InMemoryTokenStore tokens;
  late MockCustomerAuthApi customerAuth;
  late AuthRepository repo;

  const emailLogin = LoginInput(type: LoginType.email, value: 'An@Example.com');
  const phoneLogin = LoginInput(type: LoginType.phone, value: '0901 234 567');

  setUpAll(registerLoginFallbacks);

  setUp(() {
    kratos = MockKratosClient();
    tokens = InMemoryTokenStore();
    customerAuth = MockCustomerAuthApi();
    repo = AuthRepository(
      kratos: kratos,
      tokens: tokens,
      customerAuth: customerAuth,
    );
  });

  group('restoreSession', () {
    test('no token -> null, Kratos not called', () async {
      expect(await repo.restoreSession(), isNull);
      verifyNever(
        () => kratos.toSession(sessionToken: any(named: 'sessionToken')),
      );
    });

    test('valid token -> session', () async {
      await tokens.write('ory_st_valid');
      when(() => kratos.toSession(sessionToken: 'ory_st_valid'))
          .thenAnswer((_) async => session());
      expect((await repo.restoreSession())?.id, 'sess-1');
      expect(await tokens.read(), 'ory_st_valid');
    });

    test('401 -> token wiped, null', () async {
      await tokens.write('ory_st_stale');
      when(() => kratos.toSession(sessionToken: any(named: 'sessionToken')))
          .thenThrow(const UnauthenticatedFailure());
      expect(await repo.restoreSession(), isNull);
      expect(await tokens.read(), isNull);
    });

    test('network error -> NetworkFailure, token kept', () async {
      await tokens.write('ory_st_valid');
      when(() => kratos.toSession(sessionToken: any(named: 'sessionToken')))
          .thenThrow(const NetworkFailure());
      await expectLater(repo.restoreSession(), throwsA(isA<NetworkFailure>()));
      expect(await tokens.read(), 'ory_st_valid');
    });
  });

  group('register', () {
    for (final login in [emailLogin, phoneLogin]) {
      test('${login.type.name}: one call to identity-service with what was '
          'typed; stores token, keeps the verification flow', () async {
        when(() => customerAuth.register(login, 'pw')).thenAnswer(
          (_) async => NativeAuthResult(
            sessionToken: 'ory_st_new',
            session: session(),
            continueWith: const [ContinueWithVerificationUi(flowId: 'vf-1')],
          ),
        );
        final out = await repo.register(login: login, password: 'pw');
        expect(out.verificationFlowId, 'vf-1');
        expect(out.session.identity.loginId, pseudonym);
        expect(await tokens.read(), 'ory_st_new');
        verifyZeroInteractions(kratos);
      });
    }

    test('rejection (password policy) -> FlowValidationFailure, nothing '
        'stored', () async {
      final bad = flowWith(
        nodeMessages: {
          'password': [err(4000032, '')],
        },
      );
      when(() => customerAuth.register(any(), any()))
          .thenThrow(FlowValidationFailure(bad));
      await expectLater(
        repo.register(login: emailLogin, password: 'x'),
        throwsA(isA<FlowValidationFailure>()),
      );
      expect(await tokens.read(), isNull);
    });

    test('a session without the session object is read from Kratos', () async {
      when(() => customerAuth.register(any(), any())).thenAnswer(
        (_) async =>
            const NativeAuthResult(sessionToken: 'ory_st_new', session: null),
      );
      when(() => kratos.toSession(sessionToken: 'ory_st_new'))
          .thenAnswer((_) async => session());
      final out = await repo.register(login: emailLogin, password: 'pw');
      expect(out.session.id, 'sess-1');
    });
  });

  group('login', () {
    for (final login in [emailLogin, phoneLogin]) {
      test('${login.type.name}: one call to identity-service, token '
          'stored', () async {
        when(() => customerAuth.login(login, 'pw')).thenAnswer(
          (_) async => NativeAuthResult(
            sessionToken: 'ory_st_login',
            session: session(),
          ),
        );
        final s = await repo.login(login: login, password: 'pw');
        expect(s.id, 'sess-1');
        expect(await tokens.read(), 'ory_st_login');
        verifyZeroInteractions(kratos);
      });
    }

    test('wrong password: FlowValidationFailure, no token', () async {
      when(() => customerAuth.login(any(), any())).thenThrow(
        FlowValidationFailure(flowWith(messages: [err(4000006, '')])),
      );
      await expectLater(
        repo.login(login: emailLogin, password: 'pw'),
        throwsA(isA<FlowValidationFailure>()),
      );
      expect(await tokens.read(), isNull);
    });

    test('429 / 503 surface as ApiFailure', () async {
      for (final failure in [
        const ApiFailure(
          'rate_limited',
          status: 429,
          retryAfter: Duration(seconds: 30),
        ),
        const ApiFailure('dependency_unavailable', status: 503),
      ]) {
        when(() => customerAuth.login(any(), any())).thenThrow(failure);
        await expectLater(
          repo.login(login: phoneLogin, password: 'pw'),
          throwsA(same(failure)),
        );
      }
    });

    test('an empty session token is refused', () async {
      when(() => customerAuth.login(any(), any())).thenAnswer(
        (_) async => const NativeAuthResult(sessionToken: '', session: null),
      );
      await expectLater(
        repo.login(login: emailLogin, password: 'pw'),
        throwsA(isA<ApiFailure>()),
      );
      expect(await tokens.read(), isNull);
    });
  });

  group('verification', () {
    test('empty session handle -> UnauthenticatedFailure, Kratos not '
        'called', () async {
      await expectLater(
        repo.startVerification(loginId: ''),
        throwsA(isA<UnauthenticatedFailure>()),
      );
      await expectLater(
        repo.resendVerificationCode(flowId: 'vf', loginId: ''),
        throwsA(isA<UnauthenticatedFailure>()),
      );
      verifyNever(() => kratos.createVerificationFlow());
      verifyNever(
        () => kratos.submitVerification(
          flowId: any(named: 'flowId'),
          email: any(named: 'email'),
        ),
      );
    });

    test('start / resend with the session handle as Kratos email', () async {
      when(kratos.createVerificationFlow)
          .thenAnswer((_) async => flowWith(id: 'v1'));
      when(() => kratos.submitVerification(flowId: 'v1', email: pseudonym))
          .thenAnswer((_) async => flowWith(id: 'v1', state: 'sent_email'));
      await repo.startVerification(loginId: pseudonym);
      await repo.resendVerificationCode(flowId: 'v1', loginId: pseudonym);
      verify(() => kratos.submitVerification(flowId: 'v1', email: pseudonym))
          .called(2);
      verifyZeroInteractions(customerAuth);
    });

    test('wrong code (200 + error message) -> FlowValidationFailure', () async {
      when(() => kratos.submitVerification(flowId: 'v1', code: '000000'))
          .thenAnswer(
            (_) async => flowWith(
              id: 'v1',
              state: 'sent_email',
              messages: [err(4070006, 'invalid')],
            ),
          );
      await expectLater(
        repo.verify(flowId: 'v1', code: '000000'),
        throwsA(isA<FlowValidationFailure>()),
      );
    });

    test('passed_challenge -> flow', () async {
      when(
        () => kratos.submitVerification(flowId: 'v1', code: '123456'),
      ).thenAnswer((_) async => flowWith(id: 'v1', state: 'passed_challenge'));
      expect(
        (await repo.verify(flowId: 'v1', code: '123456')).state,
        'passed_challenge',
      );
    });
  });

  group('recovery', () {
    for (final login in [emailLogin, phoneLogin]) {
      test('${login.type.name}: request code starts the flow through '
          'identity-service; the code step continues on its flow id', () async {
        when(() => customerAuth.startRecovery(login))
            .thenAnswer((_) async => 'r1');
        final flow = await repo.requestRecoveryCode(login: login);
        expect(flow.id, 'r1');
        expect(flow.state, 'sent_email');
        expect(flow.messages.single.id, 1060003);
        verifyZeroInteractions(kratos);
      });
    }

    test('code goes through identity-service on the sealed id; new '
        'password signs in', () async {
      when(() => customerAuth.submitRecoveryCode('sealed-r1', '111111'))
          .thenAnswer(
            (_) async => (sessionToken: 'ory_st_priv', settingsFlowId: 's1'),
          );
      when(
        () => kratos.submitSettingsPassword(
          flowId: 's1',
          sessionToken: 'ory_st_priv',
          password: 'new-pass',
        ),
      ).thenAnswer((_) async => flowWith(id: 's1', state: 'success'));
      when(() => kratos.toSession(sessionToken: 'ory_st_priv'))
          .thenAnswer((_) async => session());

      final grant = await repo.submitRecoveryCode(
        flowId: 'sealed-r1',
        code: '111111',
      );
      expect(grant.settingsFlowId, 's1');
      expect(
        await tokens.read(),
        isNull,
        reason: 'not signed in until password set',
      );
      await repo.completeRecovery(grant: grant, password: 'new-pass');
      expect(await tokens.read(), 'ory_st_priv');
    });

    test('no settings flow in the grant: one is created', () async {
      when(() => customerAuth.submitRecoveryCode(any(), any())).thenAnswer(
        (_) async => (sessionToken: 'ory_st_priv', settingsFlowId: ''),
      );
      when(() => kratos.createSettingsFlow(sessionToken: 'ory_st_priv'))
          .thenAnswer((_) async => flowWith(id: 's2'));
      final grant = await repo.submitRecoveryCode(flowId: 'r', code: '111111');
      expect(grant.settingsFlowId, 's2');
    });

    test('wrong code / expired flow surface unchanged', () async {
      for (final failure in <AppFailure>[
        FlowValidationFailure(flowWith(messages: [err(4060006, '')])),
        const FlowExpiredFailure(),
      ]) {
        when(() => customerAuth.submitRecoveryCode(any(), any()))
            .thenThrow(failure);
        await expectLater(
          repo.submitRecoveryCode(flowId: 'r', code: '000000'),
          throwsA(same(failure)),
        );
      }
    });
  });

  group('logout', () {
    test('performNativeLogout then wipe', () async {
      await tokens.write('ory_st_x');
      when(() => kratos.logout(sessionToken: 'ory_st_x'))
          .thenAnswer((_) async {});
      await repo.logout();
      verify(() => kratos.logout(sessionToken: 'ory_st_x')).called(1);
      expect(await tokens.read(), isNull);
    });

    test('wipes even when Kratos is unreachable', () async {
      await tokens.write('ory_st_x');
      when(() => kratos.logout(sessionToken: any(named: 'sessionToken')))
          .thenThrow(const NetworkFailure());
      await repo.logout();
      expect(await tokens.read(), isNull);
    });
  });
}
