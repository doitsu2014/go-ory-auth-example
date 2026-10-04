import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/me.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/edit_profile_screen.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/profile_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

Me me({
  bool verified = true,
  String? displayName = 'An',
  LoginContact login = const LoginContact(
    type: LoginType.email,
    value: 'an@example.com',
  ),
}) => Me(
  id: '5d9c2c61-6a1e-4b8f-9b8a-2f9d6f0c1e11',
  login: login,
  email: login.type == LoginType.email ? login.value : null,
  emailVerified: verified,
  locale: 'vi-VN',
  createdAt: DateTime.utc(2026, 10, 3),
  displayName: displayName,
);

void main() {
  late MockAuthRepository auth;
  late MockProfileRepository profile;

  setUpAll(() => registerFallbackValue(const UpdateMeRequest()));

  setUp(() {
    auth = MockAuthRepository();
    profile = MockProfileRepository();
  });

  Future<void> pump(WidgetTester tester, Widget screen) => pumpScreen(
    tester,
    screen,
    overrides: [
      ...signedInOverrides(auth, verified: false),
      profileRepositoryProvider.overrideWithValue(profile),
    ],
  );

  group('ProfileScreen', () {
    testWidgets('data: email, verified badge, name, display name, locale', (
      tester,
    ) async {
      when(profile.getMe).thenAnswer((_) async => me());
      await pump(tester, const ProfileScreen());
      await tester.pumpAndSettle();
      expect(find.text('an@example.com'), findsOneWidget);
      expect(
        tester.widget<Text>(find.byKey(const Key('profile.loginType'))).data,
        'Email',
      );
      expect(find.text('Verified'), findsOneWidget);
      expect(find.text('An'), findsOneWidget); // display name (nickname)
      expect(find.text('vi-VN'), findsOneWidget);
      expect(find.text('Verify now'), findsNothing);
    });

    testWidgets('phone login: shows me.login (type + value), never Kratos '
        'traits', (tester) async {
      when(profile.getMe).thenAnswer(
        (_) async => me(
          login: const LoginContact(
            type: LoginType.phone,
            value: '+84901234567',
          ),
        ),
      );
      await pump(tester, const ProfileScreen());
      await tester.pumpAndSettle();
      expect(
        tester.widget<Text>(find.byKey(const Key('profile.login'))).data,
        '+84901234567',
      );
      expect(
        tester.widget<Text>(find.byKey(const Key('profile.loginType'))).data,
        'Phone number',
      );
      expect(find.textContaining('login.invalid'), findsNothing);
    });

    testWidgets('no real-name row; personal info is not fetched', (
      tester,
    ) async {
      // The real name is PII: shown only on the protected personal-info
      // screen. A deprecated `Me.name` (NAME-FR-07) is ignored.
      final parsed = Me.fromJson({
        'id': '5d9c2c61-6a1e-4b8f-9b8a-2f9d6f0c1e11',
        'login': {'type': 'email', 'value': 'an@example.com'},
        'email': 'an@example.com',
        'email_verified': true,
        'locale': 'vi-VN',
        'created_at': '2026-10-03T00:00:00Z',
        'name': {'first': 'Kratos', 'last': 'Trait'},
        'display_name': 'Khách hàng 01',
      });
      when(profile.getMe).thenAnswer((_) async => parsed);
      await pump(tester, const ProfileScreen());
      await tester.pumpAndSettle();
      expect(find.text('Khách hàng 01'), findsOneWidget);
      expect(find.textContaining('Kratos'), findsNothing);
      expect(find.text('Full name'), findsNothing);
      expect(find.byKey(const Key('profile.name')), findsNothing);
      verifyNever(profile.getPersonalInfo);
    });

    testWidgets('unverified: badge + verify now routes to verification', (
      tester,
    ) async {
      when(profile.getMe).thenAnswer((_) async => me(verified: false));
      await pump(tester, const ProfileScreen());
      await tester.pumpAndSettle();
      expect(find.text('Not verified'), findsOneWidget);
      await tester.tap(find.text('Verify now'));
      await tester.pumpAndSettle();
      expect(find.text('route:/verify-email'), findsOneWidget);
    });

    testWidgets('personal information entry opens /profile/personal-info', (
      tester,
    ) async {
      when(profile.getMe).thenAnswer((_) async => me());
      await pump(tester, const ProfileScreen());
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('profile.personalInfo')));
      await tester.pumpAndSettle();
      expect(find.text('route:/profile/personal-info'), findsOneWidget);
    });

    testWidgets('error: localised failure + retry', (tester) async {
      when(profile.getMe).thenThrow(const NetworkFailure());
      await pump(tester, const ProfileScreen());
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('profile.error')), findsOneWidget);
      expect(find.text('Retry'), findsOneWidget);
    });
  });

  group('EditProfileScreen', () {
    testWidgets('403 email_not_verified -> verification required', (
      tester,
    ) async {
      when(() => profile.updateMe(any()))
          .thenThrow(const ApiFailure('email_not_verified', status: 403));
      await pump(tester, EditProfileScreen(me: me(verified: false)));
      await tester.tap(find.byKey(const Key('editProfile.save')));
      await tester.pumpAndSettle();
      expect(find.text('route:/verify-email'), findsOneWidget);
      final state = authStateOf(tester, find.text('route:/verify-email'));
      expect(
        state,
        isA<Authenticated>().having(
          (s) => s.pendingVerification,
          'pending',
          isTrue,
        ),
      );
    });

    testWidgets('422 validation_failed -> field error', (tester) async {
      when(() => profile.updateMe(any())).thenThrow(
        const ApiFailure(
          'validation_failed',
          status: 422,
          fieldErrors: [FieldError(field: 'display_name', code: 'too_long')],
        ),
      );
      await pump(tester, EditProfileScreen(me: me()));
      await tester.enterText(
        find.byKey(const Key('editProfile.displayName')),
        'x' * 50,
      );
      await tester.tap(find.byKey(const Key('editProfile.save')));
      await tester.pumpAndSettle();
      expect(find.text('Too long.'), findsOneWidget);
      final sent =
          verify(() => profile.updateMe(captureAny())).captured.single
              as UpdateMeRequest;
      expect(sent.toJson(), {'display_name': 'x' * 50, 'locale': 'vi-VN'});
    });
  });
}
