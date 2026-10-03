import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/sign_up_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockAuthRepository repo;

  setUp(() {
    repo = MockAuthRepository();
    when(repo.startRegistration).thenAnswer((_) async => flowWith(id: 'reg-1'));
  });

  void stubRegister(Future<RegistrationOutcome> Function() answer) {
    when(
      () => repo.register(
        flowId: any(named: 'flowId'),
        email: any(named: 'email'),
        password: any(named: 'password'),
      ),
    ).thenAnswer((_) => answer());
  }

  Future<void> fillAndSubmit(WidgetTester tester) async {
    await tester.enterText(
      find.byKey(const Key('signUp.email')),
      'an@example.com',
    );
    await tester.enterText(find.byKey(const Key('signUp.password')), 'short');
    await tester.tap(find.byKey(const Key('signUp.submit')));
  }

  testWidgets('idle', (tester) async {
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    verify(repo.startRegistration).called(1);
    for (final k in ['signUp.email', 'signUp.password']) {
      expect(find.byKey(Key(k)), findsOneWidget);
    }
    // NAME-FR-09: sign-up asks only email + password.
    for (final k in ['signUp.first', 'signUp.last']) {
      expect(find.byKey(Key(k)), findsNothing);
    }
    expect(find.byType(TextField), findsNWidgets(2));
    expect(find.byKey(const Key('flow.messages')), findsNothing);
  });

  testWidgets('submitting', (tester) async {
    final pending = Completer<RegistrationOutcome>();
    stubRegister(() => pending.future);
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    await fillAndSubmit(tester);
    await tester.pump();
    expect(find.byKey(const Key('flow.submitting')), findsOneWidget);
    pending.complete(RegistrationOutcome(session: session()));
    await tester.pumpAndSettle();
  });

  testWidgets('submits only email + password (no name)', (tester) async {
    stubRegister(() async => RegistrationOutcome(session: session()));
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    await fillAndSubmit(tester);
    await tester.pumpAndSettle();
    verify(
      () => repo.register(
        flowId: 'reg-1',
        email: 'an@example.com',
        password: 'short',
      ),
    ).called(1);
  });

  testWidgets('field errors: password policy + email taken, localised', (
    tester,
  ) async {
    stubRegister(
      () => Future.error(
        FlowValidationFailure(
          flowWith(
            id: 'reg-1',
            nodeMessages: {
              'password': [
                err(4000032, 'must be at least 12 characters', {
                  'min_length': 12,
                }),
              ],
              'traits.email': [
                err(
                  4000007,
                  'An account with the same identifier exists already.',
                ),
              ],
            },
          ),
        ),
      ),
    );
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    await fillAndSubmit(tester);
    await tester.pumpAndSettle();
    expect(
      find.text('The password must be at least 12 characters long.'),
      findsOneWidget,
    );
    expect(
      find.text('An account with this email already exists.'),
      findsOneWidget,
    );
  });

  testWidgets('global error: unknown message id falls back to Kratos text', (
    tester,
  ) async {
    stubRegister(
      () => Future.error(
        FlowValidationFailure(
          flowWith(
            id: 'reg-1',
            messages: [err(4999999, 'Brand new Kratos error')],
          ),
        ),
      ),
    );
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    await fillAndSubmit(tester);
    await tester.pumpAndSettle();
    expect(find.text('Brand new Kratos error'), findsOneWidget);
  });

  testWidgets('network error banner', (tester) async {
    stubRegister(() => Future.error(const NetworkFailure()));
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    await fillAndSubmit(tester);
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('flow.failure')), findsOneWidget);
  });
}
