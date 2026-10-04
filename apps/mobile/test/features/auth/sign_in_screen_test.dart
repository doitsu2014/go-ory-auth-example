import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
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
    when(repo.startLogin).thenAnswer((_) async => flowWith(id: 'login-1'));
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
        flowId: any(named: 'flowId'),
        login: any(named: 'login'),
        password: any(named: 'password'),
      ),
    ).thenAnswer((_) => answer());
  }

  testWidgets('idle: creates a native login flow and shows the form', (
    tester,
  ) async {
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    verify(repo.startLogin).called(1);
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

  testWidgets('field error: node message rendered under the field', (
    tester,
  ) async {
    stubLogin(
      () => Future.error(
        FlowValidationFailure(
          flowWith(
            id: 'login-1',
            nodeMessages: {
              'identifier': [err(4000002, 'Property identifier is missing.')],
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
    // Same flow re-rendered: no new flow created.
    verify(repo.startLogin).called(1);
  });

  testWidgets('global error: ui.messages mapped by id', (tester) async {
    stubLogin(
      () => Future.error(
        FlowValidationFailure(
          flowWith(
            id: 'login-1',
            messages: [err(4000006, 'The provided credentials are invalid')],
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
          flowId: 'login-1',
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
        flowId: any(named: 'flowId'),
        login: any(named: 'login'),
        password: any(named: 'password'),
      ),
    );

    // Editing the field clears the stale local error.
    await tester.enterText(find.byKey(const Key('signIn.login')), 'not-an-e');
    await tester.pump();
    expect(find.text('Enter a valid email address.'), findsNothing);
  });

  testWidgets('resolver 503: service unavailable banner with retry', (
    tester,
  ) async {
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

  testWidgets('resolver 429 without Retry-After: generic rate-limit text', (
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

  testWidgets('flow load network error clears after a successful retry', (
    tester,
  ) async {
    var calls = 0;
    when(repo.startLogin).thenAnswer((_) async {
      if (calls++ == 0) throw const NetworkFailure();
      return flowWith(id: 'login-2');
    });
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('flow.failure')), findsOneWidget);
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('flow.failure')), findsNothing);
  });

  testWidgets('410 expired: restarts the flow', (tester) async {
    stubLogin(() => Future.error(const FlowExpiredFailure()));
    await pumpScreen(
      tester,
      const SignInScreen(),
      overrides: authOverrides(repo),
    );
    await fill(tester);
    await tester.tap(find.byKey(const Key('signIn.submit')));
    await tester.pumpAndSettle();
    verify(repo.startLogin).called(2);
    expect(find.textContaining('expired'), findsOneWidget);
  });
}
