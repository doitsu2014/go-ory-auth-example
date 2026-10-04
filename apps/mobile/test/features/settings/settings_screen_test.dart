import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';
import 'package:go_ory_auth_mobile/features/settings/presentation/settings_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockAuthRepository auth;
  late MockSettingsRepository settings;
  var flowSeq = 0;

  setUp(() {
    auth = MockAuthRepository();
    settings = MockSettingsRepository();
    flowSeq = 0;
    when(settings.startSettings)
        .thenAnswer((_) async => flowWith(id: 'sf-${++flowSeq}'));
  });

  Future<void> pump(WidgetTester tester) => pumpScreen(
    tester,
    const SettingsScreen(),
    overrides: [
      ...signedInOverrides(auth),
      settingsRepositoryProvider.overrideWithValue(settings),
    ],
  );

  Future<void> submitNew(WidgetTester tester, String pw) async {
    await tester.enterText(find.byKey(const Key('settings.password')), pw);
    await tester.tap(find.byKey(const Key('settings.changePassword')));
    await tester.pumpAndSettle();
  }

  KratosFlow saved(String id) => flowWith(
    id: id,
    state: 'success',
    messages: const [UiText(id: 1050001, text: 'saved', type: 'success')],
  );

  testWidgets('success: Kratos message shown via its id', (tester) async {
    when(
      () => settings.changePassword(flowId: 'sf-1', password: 'Str0ng-new-pw!'),
    ).thenAnswer((_) async => saved('sf-1'));
    await pump(tester);
    await submitNew(tester, 'Str0ng-new-pw!');
    expect(find.text('Your changes have been saved.'), findsOneWidget);
  });

  testWidgets('400: password policy message under the field', (tester) async {
    when(
      () => settings.changePassword(
        flowId: any(named: 'flowId'),
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
    await pump(tester);
    await submitNew(tester, 'password1234');
    expect(
      find.text(
        'This password appeared in a data breach. Please choose another one.',
      ),
      findsOneWidget,
    );
  });

  testWidgets('session_refresh_required: re-auth then retry the change', (
    tester,
  ) async {
    when(
      () => settings.changePassword(flowId: 'sf-1', password: 'Str0ng-new-pw!'),
    ).thenThrow(const ApiFailure('session_refresh_required', status: 403));
    when(
      () => settings.reauthenticate(identifier: pseudonym, password: 'wrong'),
    ).thenThrow(
      FlowValidationFailure(
        flowWith(id: 'lf-1', messages: [err(4000006, 'invalid')]),
      ),
    );
    when(
      () =>
          settings.reauthenticate(identifier: pseudonym, password: 'old-pass'),
    ).thenAnswer((_) async {});
    when(
      () => settings.changePassword(flowId: 'sf-2', password: 'Str0ng-new-pw!'),
    ).thenAnswer((_) async => saved('sf-2'));

    await pump(tester);
    await submitNew(tester, 'Str0ng-new-pw!');
    expect(find.byKey(const Key('settings.reauthBody')), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('settings.currentPassword')),
      'wrong',
    );
    await tester.tap(find.byKey(const Key('settings.reauth')));
    await tester.pumpAndSettle();
    expect(
      find.text('The email/phone number or password is incorrect.'),
      findsOneWidget,
    );
    // PLI-FR-09: the identifier is the session's login_id (pseudonym); the
    // customer is never asked for the email / phone again.
    expect(find.byType(TextField), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('settings.currentPassword')),
      'old-pass',
    );
    await tester.tap(find.byKey(const Key('settings.reauth')));
    await tester.pumpAndSettle();
    verify(
      () =>
          settings.reauthenticate(identifier: pseudonym, password: 'old-pass'),
    ).called(1);
    verify(
      () => settings.changePassword(flowId: 'sf-2', password: 'Str0ng-new-pw!'),
    ).called(1);
    expect(find.text('Your changes have been saved.'), findsOneWidget);
    expect(find.byKey(const Key('settings.reauthBody')), findsNothing);
  });

  testWidgets('401: session wiped and state -> Unauthenticated', (
    tester,
  ) async {
    when(
      () => settings.changePassword(
        flowId: any(named: 'flowId'),
        password: any(named: 'password'),
      ),
    ).thenThrow(const UnauthenticatedFailure());
    await pump(tester);
    await submitNew(tester, 'Str0ng-new-pw!');
    verify(auth.forget).called(1);
    expect(
      authStateOf(tester, find.byType(SettingsScreen)),
      isA<Unauthenticated>(),
    );
  });

  testWidgets('401 while creating the flow also signs out', (tester) async {
    when(settings.startSettings).thenThrow(const UnauthenticatedFailure());
    await pump(tester);
    await submitNew(tester, 'x');
    verify(auth.forget).called(1);
  });
}
