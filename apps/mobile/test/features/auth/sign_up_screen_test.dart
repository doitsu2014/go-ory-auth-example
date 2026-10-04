import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/sign_up_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockAuthRepository repo;

  setUpAll(registerLoginFallbacks);

  setUp(() {
    repo = MockAuthRepository();
  });

  void stubRegister(Future<RegistrationOutcome> Function() answer) {
    when(
      () => repo.register(
        login: any(named: 'login'),
        password: any(named: 'password'),
      ),
    ).thenAnswer((_) => answer());
  }

  Future<void> fillAndSubmit(
    WidgetTester tester, {
    String login = 'an@example.com',
  }) async {
    await tester.enterText(find.byKey(const Key('signUp.login')), login);
    await tester.enterText(find.byKey(const Key('signUp.password')), 'short');
    await tester.tap(find.byKey(const Key('signUp.submit')));
  }

  testWidgets('idle', (tester) async {
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    for (final k in ['signUp.loginType', 'signUp.login', 'signUp.password']) {
      expect(find.byKey(Key(k)), findsOneWidget);
    }
    // NAME-FR-09: sign-up asks only the login (email | phone) + password.
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

  testWidgets('password hint: no email / phone in the password (R-06)', (
    tester,
  ) async {
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    expect(
      find.text(
        "Don't use your email address or phone number in your password.",
      ),
      findsOneWidget,
    );
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
        login: const LoginInput(type: LoginType.email, value: 'an@example.com'),
        password: 'short',
      ),
    ).called(1);
  });

  testWidgets('phone: keyboard + autofill per type, submits a phone login', (
    tester,
  ) async {
    stubRegister(() async => RegistrationOutcome(session: session()));
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    TextField field() =>
        tester.widget<TextField>(find.byKey(const Key('signUp.login')));
    expect(field().keyboardType, TextInputType.emailAddress);
    expect(field().autofillHints, [AutofillHints.email]);

    await tester.tap(find.byKey(const Key('signUp.loginType.phone')));
    await tester.pump();
    expect(field().keyboardType, TextInputType.phone);
    expect(field().autofillHints, [AutofillHints.telephoneNumber]);
    expect(find.text('Phone number'), findsOneWidget);

    await fillAndSubmit(tester, login: '0901 234 567');
    await tester.pumpAndSettle();
    verify(
      () => repo.register(
        login: const LoginInput(type: LoginType.phone, value: '0901 234 567'),
        password: 'short',
      ),
    ).called(1);
  });

  testWidgets('client-side format check blocks the submit (UX only)', (
    tester,
  ) async {
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    await tester.tap(find.byKey(const Key('signUp.loginType.phone')));
    await tester.pump();
    await fillAndSubmit(tester, login: '12ab');
    await tester.pumpAndSettle();
    expect(
      find.text('Enter a valid phone number, e.g. 0901234567 or +84901234567.'),
      findsOneWidget,
    );
    verifyNever(
      () => repo.register(
        login: any(named: 'login'),
        password: any(named: 'password'),
      ),
    );

    // Editing the field clears the stale local error.
    await tester.enterText(find.byKey(const Key('signUp.login')), '0901');
    await tester.pump();
    expect(
      find.text('Enter a valid phone number, e.g. 0901234567 or +84901234567.'),
      findsNothing,
    );
  });

  testWidgets('422 field error shown on the login field', (tester) async {
    stubRegister(
      () => Future.error(
        const ApiFailure(
          'validation_failed',
          status: 422,
          fieldErrors: [
            FieldError(field: 'login.value', code: 'unsupported_country'),
          ],
        ),
      ),
    );
    await pumpScreen(
      tester,
      const SignUpScreen(),
      overrides: authOverrides(repo),
    );
    await tester.tap(find.byKey(const Key('signUp.loginType.phone')));
    await tester.pump();
    await fillAndSubmit(tester, login: '+12025550123');
    await tester.pumpAndSettle();
    expect(
      find.text('Phone numbers from this country are not supported yet.'),
      findsOneWidget,
    );
  });

  testWidgets('429: rate-limit message with Retry-After', (tester) async {
    stubRegister(
      () => Future.error(
        const ApiFailure(
          'rate_limited',
          status: 429,
          retryAfter: Duration(seconds: 60),
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
      find.text('Too many attempts. Please try again in 1 minute.'),
      findsOneWidget,
    );
  });

  testWidgets('webhook message ids on the login node are localised', (
    tester,
  ) async {
    stubRegister(
      () => Future.error(
        FlowValidationFailure(
          flowWith(
            nodeMessages: {
              AuthFlowFields.login: [err(4049001, '')],
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
      find.text(
        'This app version is out of date. Please update the app and try again.',
      ),
      findsOneWidget,
    );
  });

  testWidgets('field errors: password policy + email taken, localised', (
    tester,
  ) async {
    stubRegister(
      () => Future.error(
        FlowValidationFailure(
          // identity-service forwards message ids only (no text, no
          // context): the default minimum length is shown.
          flowWith(
            nodeMessages: {
              AuthFlowFields.password: [err(4000032, '')],
              AuthFlowFields.login: [err(4000007, '')],
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
      find.text('An account with this email or phone number already exists.'),
      findsOneWidget,
    );
  });

  testWidgets('global error: unknown message id falls back to the generic '
      'text', (tester) async {
    stubRegister(
      () => Future.error(
        FlowValidationFailure(flowWith(messages: [err(4999999, '')])),
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
      find.text('Something went wrong. Please try again.'),
      findsOneWidget,
    );
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
