import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/storage/secure_token_store.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockKratosClient kratos;
  late InMemoryTokenStore tokens;
  late AuthRepository repo;

  setUp(() {
    kratos = MockKratosClient();
    tokens = InMemoryTokenStore();
    repo = AuthRepository(kratos: kratos, tokens: tokens);
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
    test(
      'stores token and follows continue_with show_verification_ui',
      () async {
        when(
          () => kratos.submitRegistration(
            flowId: 'f1',
            email: 'an@example.com',
            password: 'pw',
            firstName: 'An',
            lastName: 'Nguyen',
          ),
        ).thenAnswer(
          (_) async => NativeAuthResult(
            sessionToken: 'ory_st_new',
            session: session(),
            continueWith: const [ContinueWithVerificationUi(flowId: 'vf-1')],
          ),
        );
        final out = await repo.register(
          flowId: 'f1',
          email: 'an@example.com',
          password: 'pw',
          firstName: 'An',
          lastName: 'Nguyen',
        );
        expect(out.verificationFlowId, 'vf-1');
        expect(await tokens.read(), 'ory_st_new');
      },
    );

    test('400 -> FlowValidationFailure, nothing stored', () async {
      final bad = flowWith(
        nodeMessages: {
          'password': [
            err(4000032, 'too short', {'min_length': 12}),
          ],
        },
      );
      when(
        () => kratos.submitRegistration(
          flowId: any(named: 'flowId'),
          email: any(named: 'email'),
          password: any(named: 'password'),
          firstName: any(named: 'firstName'),
          lastName: any(named: 'lastName'),
        ),
      ).thenThrow(FlowValidationFailure(bad));
      await expectLater(
        repo.register(flowId: 'f1', email: 'a@b.c', password: 'x'),
        throwsA(isA<FlowValidationFailure>()),
      );
      expect(await tokens.read(), isNull);
    });
  });

  test('login stores token', () async {
    when(
      () => kratos.submitLogin(
        flowId: 'l1',
        identifier: 'an@example.com',
        password: 'pw',
      ),
    ).thenAnswer(
      (_) async =>
          NativeAuthResult(sessionToken: 'ory_st_login', session: session()),
    );
    await repo.login(
      flowId: 'l1',
      identifier: 'an@example.com',
      password: 'pw',
    );
    expect(await tokens.read(), 'ory_st_login');
  });

  group('verification', () {
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
    test('code -> grant from continue_with; new password signs in', () async {
      when(() => kratos.submitRecovery(flowId: 'r1', code: '111111'))
          .thenAnswer(
            (_) async => const KratosFlow(
              id: 'r1',
              state: 'passed_challenge',
              continueWith: [
                ContinueWithSettingsUi(flowId: 's1'),
                ContinueWithSetSessionToken('ory_st_priv'),
              ],
            ),
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

      final grant = await repo.submitRecoveryCode(flowId: 'r1', code: '111111');
      expect(grant.settingsFlowId, 's1');
      expect(
        await tokens.read(),
        isNull,
        reason: 'not signed in until password set',
      );
      await repo.completeRecovery(grant: grant, password: 'new-pass');
      expect(await tokens.read(), 'ory_st_priv');
    });

    test(
      '422 browser_location_change_required -> recovery_session_unavailable',
      () async {
        when(() => kratos.submitRecovery(flowId: 'r1', code: '111111'))
            .thenThrow(
              const ApiFailure('browser_location_change_required', status: 422),
            );
        await expectLater(
          repo.submitRecoveryCode(flowId: 'r1', code: '111111'),
          throwsA(
            isA<ApiFailure>().having(
              (f) => f.code,
              'code',
              'recovery_session_unavailable',
            ),
          ),
        );
      },
    );

    test('wrong code -> FlowValidationFailure', () async {
      when(() => kratos.submitRecovery(flowId: 'r1', code: '000000'))
          .thenAnswer(
            (_) async => flowWith(
              id: 'r1',
              state: 'sent_email',
              messages: [err(4060006, 'invalid')],
            ),
          );
      await expectLater(
        repo.submitRecoveryCode(flowId: 'r1', code: '000000'),
        throwsA(isA<FlowValidationFailure>()),
      );
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
