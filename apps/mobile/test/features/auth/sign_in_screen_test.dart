import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/sign_in_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockAuthRepository repo;

  setUpAll(registerLoginFallbacks);

  setUp(() {
    repo = MockAuthRepository();
  });

  Future<void> fill(
    WidgetTester tester, {
    String login = 'an@example.com',
  }) async {
    await tester.enterText(find.byKey(const Key('signIn.login')), login);
    await tester.enterText(
      find.byKey(const Key('signIn.password')),
      'correct horse battery',
    );
  }

  void stubLogin(Future<KratosSession> Function() answer) {
    when(
      () => repo.login(
        login: any(named: 'login'),
        password: any(named: 'password'),
      ),
    ).thenAnswer((_) => answer());
  }

  testWidgets('idle: shows the form; no Kratos flow on the device', (
    tester,
  ) async {
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    verifyNever(
      () => repo.login(
        login: any(named: 'login'),
        password: any(named: 'password'),
      ),
    );
    expect(find.byKey(const Key('signIn.loginType')), findsOneWidget);
    expect(find.byKey(const Key('signIn.login')), findsOneWidget);
    expect(find.byKey(const Key('signIn.password')), findsOneWidget);
    expect(find.byKey(const Key('flow.submitting')), findsNothing);
    expect(find.byKey(const Key('flow.failure')), findsNothing);
  });

  testWidgets('submitting: spinner and disabled button', (tester) async {
    final pending = Completer<KratosSession>();
    stubLogin(() => pending.future);
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pump();
    expect(find.byKey(const Key('flow.submitting')), findsOneWidget);
    final button = tester.widget<FilledButton>(
      find.descendant(
        of: find.byKey(const Key('signIn.submit')),
        matching: find.byType(FilledButton),
      ),
    );
    expect(button.onPressed, isNull);
    pending.complete(session());
    await tester.pumpAndSettle();
  });

  testWidgets('field error: login node message rendered under the field', (
    tester,
  ) async {
    stubLogin(
      () => Future.error(
        FlowValidationFailure(
          flowWith(
            id: '',
            nodeMessages: {
              AuthFlowFields.login: [err(4000002, '')],
            },
          ),
        ),
      ),
    );
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    expect(find.text('This field is required.'), findsOneWidget);
  });

  testWidgets('global error: ui.messages mapped by id', (tester) async {
    stubLogin(
      () => Future.error(
        FlowValidationFailure(flowWith(id: '', messages: [err(4000006, '')])),
      ),
    );
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    expect(
      find.text('The email/phone number or password is incorrect.'),
      findsOneWidget,
    );
  });

  for (final (type, typed) in [
    (LoginType.email, 'An@Example.com'),
    (LoginType.phone, '+84 901 234 567'),
  ]) {
    testWidgets('submits a ${type.name} login as typed', (tester) async {
      stubLogin(() async => session());
      await pumpScreen(
        tester,
        const SignInScreen(),
        overrides: authOverrides(repo),
      );
      if (type == LoginType.phone) {
        await tester.tap(find.byKey(const Key('signIn.loginType.phone')));
        await tester.pump();
        final field = tester.widget<TextField>(
          find.byKey(const Key('signIn.login')),
        );
        expect(field.keyboardType, TextInputType.phone);
        expect(field.autofillHints, [AutofillHints.telephoneNumber]);
      }
      await fill(tester, login: typed);
      await tester.tap(find.byKey(const Key('signIn.submit')));
      await tester.pumpAndSettle();
      verify(
        () => repo.login(
          login: LoginInput(type: type, value: typed),
          password: 'correct horse battery',
        ),
      ).called(1);
    });
  }

  testWidgets('switching type clears the field', (tester) async {
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.loginType.phone')));
    await tester.pump();
    expect(
      tester
          .widget<TextField>(find.byKey(const Key('signIn.login')))
          .controller!
          .text,
      isEmpty,
    );
  });

  testWidgets('invalid email: local error, repository not called', (
    tester,
  ) async {
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester, login: 'not-an-email');
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    expect(find.text('Enter a valid email address.'), findsOneWidget);
    verifyNever(
      () => repo.login(
        login: any(named: 'login'),
        password: any(named: 'password'),
      ),
    );

    // Editing the field clears the stale local error.
    await tester.enterText(find.byKey(const Key('signIn.login')), 'not-an-e');
    await tester.pump();
    expect(find.text('Enter a valid email address.'), findsNothing);
  });

  testWidgets('503: service unavailable banner with retry', (tester) async {
    stubLogin(
      () =>
          Future.error(const ApiFailure('dependency_unavailable', status: 503)),
    );
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    expect(
      find.text(
        'This service is temporarily unavailable. Please try again shortly.',
      ),
      findsOneWidget,
    );
    expect(find.text('Retry'), findsOneWidget);
  });

  testWidgets('429 without Retry-After: generic rate-limit text', (
    tester,
  ) async {
    stubLogin(
      () => Future.error(const ApiFailure('rate_limited', status: 429)),
    );
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    expect(
      find.text('Too many attempts. Please wait a moment and try again.'),
      findsOneWidget,
    );
  });

  testWidgets('network error: banner with retry', (tester) async {
    stubLogin(() => Future.error(const NetworkFailure()));
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('flow.failure')), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
  });

  testWidgets('retry after a network error submits again', (tester) async {
    var calls = 0;
    stubLogin(() async {
      if (calls++ == 0) throw const NetworkFailure();
      return session();
    });
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('flow.failure')), findsOneWidget);
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('flow.failure')), findsNothing);
    expect(calls, 2);
  });
}
