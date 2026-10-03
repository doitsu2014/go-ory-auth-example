import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/misc.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/forgot_password_screen.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/verify_email_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  group('VerifyEmailScreen', () {
    late MockAuthRepository repo;

    setUp(() {
      repo = MockAuthRepository();
    });

    List<Override> overrides({String? flowId}) => [
      authRepositoryProvider.overrideWithValue(repo),
      authControllerProvider.overrideWith(
        () => FixedAuth(
          Authenticated(
            session(),
            pendingVerification: true,
            verificationFlowId: flowId,
          ),
        ),
      ),
    ];

    testWidgets('idle with continue_with flow: no extra flow, email shown', (
      tester,
    ) async {
      await pumpScreen(
        tester,
        const VerifyEmailScreen(),
        overrides: overrides(flowId: 'vf-1'),
      );
      verifyNever(() => repo.startVerification(any()));
      expect(find.textContaining('an@example.com'), findsOneWidget);
    });

    testWidgets(
      'idle without continue_with: starts a verification flow (sends code)',
      (tester) async {
        when(() => repo.startVerification('an@example.com')).thenAnswer(
          (_) async => flowWith(
            id: 'vf-2',
            state: 'sent_email',
            messages: const [UiText(id: 1080003, text: 'sent', type: 'info')],
          ),
        );
        await pumpScreen(
          tester,
          const VerifyEmailScreen(),
          overrides: overrides(),
        );
        await tester.pumpAndSettle();
        verify(() => repo.startVerification('an@example.com')).called(1);
        expect(
          find.text('We sent a verification code to your email address.'),
          findsOneWidget,
        );
      },
    );

    testWidgets('submitting then wrong-code error, then resend', (
      tester,
    ) async {
      final pending = Completer<KratosFlow>();
      when(() => repo.verify(flowId: 'vf-1', code: '000000'))
          .thenAnswer((_) => pending.future);
      when(
        () => repo.resendVerificationCode(
          flowId: 'vf-1',
          email: 'an@example.com',
        ),
      ).thenAnswer(
        (_) async => flowWith(
          id: 'vf-1',
          messages: const [UiText(id: 1080003, text: 'sent', type: 'info')],
        ),
      );
      await pumpScreen(
        tester,
        const VerifyEmailScreen(),
        overrides: overrides(flowId: 'vf-1'),
      );
      await tester.enterText(find.byKey(const Key('verify.code')), '000000');
      await tester.tap(find.byKey(const Key('verify.submit')));
      await tester.pump();
      expect(find.byKey(const Key('flow.submitting')), findsOneWidget);

      pending.completeError(
        FlowValidationFailure(
          flowWith(id: 'vf-1', messages: [err(4070006, 'invalid')]),
        ),
      );
      await tester.pumpAndSettle();
      expect(
        find.text('The verification code is invalid or has already been used.'),
        findsOneWidget,
      );

      await tester.tap(find.byKey(const Key('verify.resend')));
      await tester.pumpAndSettle();
      verify(
        () => repo.resendVerificationCode(
          flowId: 'vf-1',
          email: 'an@example.com',
        ),
      ).called(1);
      expect(
        find.text('We sent a verification code to your email address.'),
        findsOneWidget,
      );
    });

    testWidgets('success navigates to profile', (tester) async {
      when(() => repo.verify(flowId: 'vf-1', code: '123456')).thenAnswer(
        (_) async => flowWith(id: 'vf-1', state: 'passed_challenge'),
      );
      when(repo.refreshSession)
          .thenAnswer((_) async => session(verified: true));
      await pumpScreen(
        tester,
        const VerifyEmailScreen(),
        overrides: overrides(flowId: 'vf-1'),
      );
      await tester.enterText(find.byKey(const Key('verify.code')), '123456');
      await tester.tap(find.byKey(const Key('verify.submit')));
      await tester.pumpAndSettle();
      expect(find.text('route:/profile'), findsOneWidget);
    });
  });

  group('ForgotPasswordScreen', () {
    late MockAuthRepository repo;

    setUp(() {
      repo = MockAuthRepository();
      when(repo.startRecovery).thenAnswer((_) async => flowWith(id: 'rf-1'));
      when(() => repo.abandonRecovery(any())).thenAnswer((_) async {});
    });

    testWidgets('idle: email step', (tester) async {
      await pumpScreen(
        tester,
        const ForgotPasswordScreen(),
        overrides: authOverrides(repo),
      );
      verify(repo.startRecovery).called(1);
      expect(find.byKey(const Key('recovery.email')), findsOneWidget);
    });

    testWidgets(
      'email -> code (submitting) -> wrong code error -> new password',
      (tester) async {
        when(
          () =>
              repo.requestRecoveryCode(flowId: 'rf-1', email: 'an@example.com'),
        ).thenAnswer(
          (_) async => flowWith(
            id: 'rf-1',
            state: 'sent_email',
            messages: const [UiText(id: 1060003, text: 'sent', type: 'info')],
          ),
        );
        final firstTry = Completer<RecoveryGrant>();
        var attempt = 0;
        when(
          () => repo.submitRecoveryCode(
            flowId: 'rf-1',
            code: any(named: 'code'),
          ),
        ).thenAnswer((_) {
          attempt++;
          return attempt == 1
              ? firstTry.future
              : Future.value(
                  const RecoveryGrant(
                    settingsFlowId: 'sf-1',
                    sessionToken: 'ory_st_priv',
                  ),
                );
        });
        await pumpScreen(
          tester,
          const ForgotPasswordScreen(),
          overrides: authOverrides(repo),
        );

        await tester.enterText(
          find.byKey(const Key('recovery.email')),
          'an@example.com',
        );
        await tester.tap(find.byKey(const Key('recovery.send')));
        await tester.pumpAndSettle();
        expect(
          find.text(
            'If an account exists for this address, '
            'we sent a recovery code to it.',
          ),
          findsOneWidget,
        );
        expect(find.byKey(const Key('recovery.code')), findsOneWidget);

        await tester.enterText(
          find.byKey(const Key('recovery.code')),
          '000000',
        );
        await tester.tap(find.byKey(const Key('recovery.submitCode')));
        await tester.pump();
        expect(find.byKey(const Key('flow.submitting')), findsOneWidget);
        firstTry.completeError(
          FlowValidationFailure(
            flowWith(id: 'rf-1', messages: [err(4060006, 'invalid')]),
          ),
        );
        await tester.pumpAndSettle();
        expect(
          find.text('The recovery code is invalid or has already been used.'),
          findsOneWidget,
        );

        await tester.enterText(
          find.byKey(const Key('recovery.code')),
          '123456',
        );
        await tester.tap(find.byKey(const Key('recovery.submitCode')));
        await tester.pumpAndSettle();
        expect(find.byKey(const Key('recovery.password')), findsOneWidget);
      },
    );

    testWidgets('continue_with path: code -> new password -> signed in', (
      tester,
    ) async {
      const grant = RecoveryGrant(
        settingsFlowId: 'sf-1',
        sessionToken: 'ory_st_priv',
      );
      when(
        () => repo.requestRecoveryCode(
          flowId: 'rf-1',
          email: any(named: 'email'),
        ),
      ).thenAnswer((_) async => flowWith(id: 'rf-1', state: 'sent_email'));
      when(() => repo.submitRecoveryCode(flowId: 'rf-1', code: '123456'))
          .thenAnswer((_) async => grant);
      when(
        () => repo.completeRecovery(grant: grant, password: 'n3w-Str0ng-pass!'),
      ).thenAnswer((_) async => session(verified: true));
      when(() => repo.abandonRecovery(any())).thenAnswer((_) async {});
      await pumpScreen(
        tester,
        const ForgotPasswordScreen(),
        overrides: authOverrides(repo),
      );
      await tester.enterText(
        find.byKey(const Key('recovery.email')),
        'an@example.com',
      );
      await tester.tap(find.byKey(const Key('recovery.send')));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('recovery.code')), '123456');
      await tester.tap(find.byKey(const Key('recovery.submitCode')));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.byKey(const Key('recovery.password')),
        'n3w-Str0ng-pass!',
      );
      await tester.tap(find.byKey(const Key('recovery.savePassword')));
      await tester.pumpAndSettle();
      expect(
        authStateOf(tester, find.byType(ForgotPasswordScreen)),
        isA<Authenticated>(),
      );
      // Completed: the privileged session is kept, not revoked.
      await tester.pumpWidget(const SizedBox());
      verifyNever(() => repo.abandonRecovery(any()));
    });

    testWidgets('abandoning after the code revokes the privileged session', (
      tester,
    ) async {
      const grant = RecoveryGrant(
        settingsFlowId: 'sf-1',
        sessionToken: 'ory_st_priv',
      );
      when(
        () => repo.requestRecoveryCode(
          flowId: 'rf-1',
          email: any(named: 'email'),
        ),
      ).thenAnswer((_) async => flowWith(id: 'rf-1', state: 'sent_email'));
      when(
        () => repo.submitRecoveryCode(
          flowId: 'rf-1',
          code: any(named: 'code'),
        ),
      ).thenAnswer((_) async => grant);
      when(() => repo.abandonRecovery(grant)).thenAnswer((_) async {});
      await pumpScreen(
        tester,
        const ForgotPasswordScreen(),
        overrides: authOverrides(repo),
      );
      await tester.enterText(
        find.byKey(const Key('recovery.email')),
        'an@example.com',
      );
      await tester.tap(find.byKey(const Key('recovery.send')));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('recovery.code')), '123456');
      await tester.tap(find.byKey(const Key('recovery.submitCode')));
      await tester.pumpAndSettle();
      await tester.pumpWidget(const SizedBox());
      verify(() => repo.abandonRecovery(grant)).called(1);
    });

    testWidgets('new password field error (breached) from settings flow', (
      tester,
    ) async {
      when(
        () => repo.requestRecoveryCode(
          flowId: 'rf-1',
          email: any(named: 'email'),
        ),
      ).thenAnswer((_) async => flowWith(id: 'rf-1', state: 'sent_email'));
      when(
        () => repo.submitRecoveryCode(
          flowId: 'rf-1',
          code: any(named: 'code'),
        ),
      ).thenAnswer(
        (_) async => const RecoveryGrant(
          settingsFlowId: 'sf-1',
          sessionToken: 'ory_st_priv',
        ),
      );
      when(
        () => repo.completeRecovery(
          grant: any(named: 'grant'),
          password: any(named: 'password'),
        ),
      ).thenThrow(
        FlowValidationFailure(
          flowWith(
            id: 'sf-1',
            nodeMessages: {
              'password': [err(4000034, 'breached')],
            },
          ),
        ),
      );
      await pumpScreen(
        tester,
        const ForgotPasswordScreen(),
        overrides: authOverrides(repo),
      );
      await tester.enterText(
        find.byKey(const Key('recovery.email')),
        'an@example.com',
      );
      await tester.tap(find.byKey(const Key('recovery.send')));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('recovery.code')), '123456');
      await tester.tap(find.byKey(const Key('recovery.submitCode')));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.byKey(const Key('recovery.password')),
        'password1234',
      );
      await tester.tap(find.byKey(const Key('recovery.savePassword')));
      await tester.pumpAndSettle();
      expect(
        find.text(
          'This password appeared in a data breach. Please choose another one.',
        ),
        findsOneWidget,
      );
    });

    testWidgets('global error: recovery session unavailable', (tester) async {
      when(
        () => repo.requestRecoveryCode(
          flowId: 'rf-1',
          email: any(named: 'email'),
        ),
      ).thenAnswer((_) async => flowWith(id: 'rf-1', state: 'sent_email'));
      when(
        () => repo.submitRecoveryCode(
          flowId: 'rf-1',
          code: any(named: 'code'),
        ),
      ).thenThrow(const ApiFailure('recovery_session_unavailable'));
      await pumpScreen(
        tester,
        const ForgotPasswordScreen(),
        overrides: authOverrides(repo),
      );
      await tester.enterText(
        find.byKey(const Key('recovery.email')),
        'an@example.com',
      );
      await tester.tap(find.byKey(const Key('recovery.send')));
      await tester.pumpAndSettle();
      await tester.enterText(find.byKey(const Key('recovery.code')), '123456');
      await tester.tap(find.byKey(const Key('recovery.submitCode')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('flow.failure')), findsOneWidget);
    });
  });

  setUpAll(() {
    registerFallbackValue(
      const RecoveryGrant(settingsFlowId: 'x', sessionToken: 'y'),
    );
  });
}
