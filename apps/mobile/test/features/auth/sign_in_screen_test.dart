import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/sign_in_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockAuthRepository repo;

  setUp(() {
    repo = MockAuthRepository();
    when(repo.startLogin).thenAnswer((_) async => flowWith(id: 'login-1'));
  });

  Future<void> fill(WidgetTester tester) async {
    await tester.enterText(
      find.byKey(const Key('signIn.identifier')),
      'an@example.com',
    );
    await tester.enterText(
      find.byKey(const Key('signIn.password')),
      'correct horse battery',
    );
  }

  void stubLogin(Future<KratosSession> Function() answer) {
    when(
      () => repo.login(
        flowId: any(named: 'flowId'),
        identifier: any(named: 'identifier'),
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
    expect(find.byKey(const Key('signIn.identifier')), findsOneWidget);
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
    expect(find.text('The email or password is incorrect.'), findsOneWidget);
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
