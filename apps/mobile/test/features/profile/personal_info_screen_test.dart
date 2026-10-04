import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/platform/secure_screen.dart';
import 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info_validation.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/personal_info_screen.dart';
import 'package:go_router/go_router.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

PersonalInfo filled() => PersonalInfo(
  name: const PersonName(first: 'Ánh', last: 'Nguyễn'),
  phoneNumber: '+84901234567',
  dateOfBirth: DateTime(1990, 5, 17),
  address: const Address(line1: '1 Le Loi', city: 'HCMC', country: 'VN'),
  nationalId: const NationalId(type: 'cccd', number: '079123456123'),
);

Finder field(String path) => find.byKey(Key('personalInfo.$path'));

void main() {
  late MockAuthRepository auth;
  late MockProfileRepository profile;

  setUpAll(() => registerFallbackValue(const PersonalInfo()));

  setUp(() {
    auth = MockAuthRepository();
    profile = MockProfileRepository();
  });

  Future<GoRouter> pump(WidgetTester tester) async {
    // Tall viewport so the whole (lazy) form is built.
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final router = await pumpScreen(
      tester,
      const PersonalInfoScreen(),
      overrides: [
        ...signedInOverrides(auth),
        profileRepositoryProvider.overrideWithValue(profile),
      ],
    );
    await tester.pumpAndSettle();
    return router;
  }

  Future<void> openEditor(WidgetTester tester) async {
    await tester.tap(find.byKey(const Key('personalInfo.edit')));
    await tester.pumpAndSettle();
  }

  Future<void> save(WidgetTester tester) async {
    await tester.tap(find.byKey(const Key('personalInfo.save')));
    await tester.pumpAndSettle();
  }

  PersonalInfo sent() =>
      verify(() => profile.putPersonalInfo(captureAny())).captured.single
          as PersonalInfo;

  group('view', () {
    testWidgets('shows stored values', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      await pump(tester);
      expect(find.text('+84901234567'), findsOneWidget);
      expect(
        tester
            .widget<Text>(find.byKey(const Key('personalInfo.view.dob')))
            .data,
        allOf(contains('1990'), contains('17')),
      );
      expect(find.text('1 Le Loi, HCMC, VN'), findsOneWidget);
      expect(find.text('Citizen ID (CCCD): 079123456123'), findsOneWidget);
    });

    testWidgets('never set -> "Not provided"', (tester) async {
      when(profile.getPersonalInfo)
          .thenAnswer((_) async => const PersonalInfo());
      await pump(tester);
      expect(find.text('Not provided'), findsNWidgets(5));
    });

    testWidgets('503 dependency_unavailable -> banner with retry', (
      tester,
    ) async {
      var calls = 0;
      when(profile.getPersonalInfo).thenAnswer((_) async {
        if (calls++ == 0) {
          throw const ApiFailure('dependency_unavailable', status: 503);
        }
        return filled();
      });
      await pump(tester);
      expect(find.byKey(const Key('personalInfo.loadError')), findsOneWidget);
      expect(
        find.text(
          'This service is temporarily unavailable. Please try again shortly.',
        ),
        findsOneWidget,
      );
      await tester.tap(find.text('Retry'));
      await tester.pumpAndSettle();
      expect(find.text('+84901234567'), findsOneWidget);
      verify(profile.getPersonalInfo).called(2);
    });
  });

  group('edit', () {
    testWidgets('name fields come first; name round-trips (NAME-FR-09)', (
      tester,
    ) async {
      var stored = const PersonalInfo();
      when(profile.getPersonalInfo).thenAnswer((_) async => stored);
      when(() => profile.putPersonalInfo(any())).thenAnswer((inv) async {
        // Simulate the server round trip through the wire JSON.
        final body = inv.positionalArguments.single as PersonalInfo;
        return stored = PersonalInfo.fromJson(body.toJson());
      });
      await pump(tester);
      expect(
        tester
            .widget<Text>(find.byKey(const Key('personalInfo.view.name')))
            .data,
        'Not provided',
      );
      await openEditor(tester);
      final firstY = tester.getTopLeft(field(PiiField.firstName)).dy;
      final lastY = tester.getTopLeft(field(PiiField.lastName)).dy;
      final phoneY = tester.getTopLeft(field(PiiField.phoneNumber)).dy;
      expect(firstY, lessThan(lastY));
      expect(lastY, lessThan(phoneY));

      await tester.enterText(field(PiiField.firstName), ' Thị Ngọc Ánh ');
      await tester.enterText(field(PiiField.lastName), 'Nguyễn ');
      await save(tester);
      expect(sent().toJson()['name'], {
        'first': 'Thị Ngọc Ánh',
        'last': 'Nguyễn',
      });
      expect(
        tester
            .widget<Text>(find.byKey(const Key('personalInfo.view.name')))
            .data,
        'Thị Ngọc Ánh Nguyễn',
      );

      // Re-opening starts from the stored name; clearing both sends null.
      await openEditor(tester);
      expect(
        tester
            .widget<TextField>(
              find.descendant(
                of: field(PiiField.firstName),
                matching: find.byType(TextField),
                matchRoot: true,
              ),
            )
            .controller!
            .text,
        'Thị Ngọc Ánh',
      );
      await tester.enterText(field(PiiField.firstName), '');
      await tester.enterText(field(PiiField.lastName), ' ');
      await save(tester);
      final second =
          verify(() => profile.putPersonalInfo(captureAny())).captured.single
              as PersonalInfo;
      expect(second.toJson()['name'], isNull);
    });

    testWidgets('name: C1 control / Cf format characters blocked client-side', (
      tester,
    ) async {
      when(profile.getPersonalInfo)
          .thenAnswer((_) async => const PersonalInfo());
      await pump(tester);
      await openEditor(tester);
      await tester.enterText(field(PiiField.firstName), 'An\u0085h');
      await tester.enterText(field(PiiField.lastName), 'Ng\u202Euyen');
      await save(tester);
      expect(
        find.text('Contains characters that are not allowed.'),
        findsNWidgets(2),
      );
      verifyNever(() => profile.putPersonalInfo(any()));
    });

    testWidgets('client validation blocks the request', (tester) async {
      when(profile.getPersonalInfo)
          .thenAnswer((_) async => const PersonalInfo());
      await pump(tester);
      await openEditor(tester);
      await tester.enterText(field(PiiField.phoneNumber), '0901234567');
      await tester.enterText(field(PiiField.city), 'HCMC');
      await tester.enterText(field(PiiField.nationalIdNumber), 'ab-1');
      await save(tester);
      expect(find.text('Invalid format.'), findsNWidgets(2)); // phone, id
      // line1 + country (address started), national id type.
      expect(find.text('This field is required.'), findsNWidgets(3));
      verifyNever(() => profile.putPersonalInfo(any()));
    });

    testWidgets('save normalises phone, country, id number and closes', (
      tester,
    ) async {
      when(profile.getPersonalInfo)
          .thenAnswer((_) async => const PersonalInfo());
      when(() => profile.putPersonalInfo(any()))
          .thenAnswer((_) async => filled());
      await pump(tester);
      await openEditor(tester);
      await tester.enterText(field(PiiField.phoneNumber), '+84 90-123.4567');
      await tester.enterText(field(PiiField.line1), '1 Le Loi');
      await tester.enterText(field(PiiField.city), 'HCMC');
      await tester.enterText(field(PiiField.country), 'vn');
      await tester.tap(field(PiiField.nationalIdType));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Passport').last);
      await tester.pumpAndSettle();
      await tester.enterText(field(PiiField.nationalIdNumber), 'b1234567');
      await save(tester);

      expect(sent().toJson(), {
        'name': null,
        'phone_number': '+84901234567',
        'date_of_birth': null,
        'address': {'line1': '1 Le Loi', 'city': 'HCMC', 'country': 'VN'},
        'national_id': {'type': 'passport', 'number': 'B1234567'},
      });
      expect(find.text('Personal information saved.'), findsOneWidget);
      expect(find.byKey(const Key('personalInfo.save')), findsNothing);
    });

    testWidgets('date of birth picker is limited to age 13-120', (
      tester,
    ) async {
      when(profile.getPersonalInfo)
          .thenAnswer((_) async => const PersonalInfo());
      when(() => profile.putPersonalInfo(any()))
          .thenAnswer((_) async => const PersonalInfo());
      await pump(tester);
      await openEditor(tester);
      await tester.tap(field(PiiField.dateOfBirth));
      await tester.pumpAndSettle();
      final picker = tester.widget<DatePickerDialog>(
        find.byType(DatePickerDialog),
      );
      final today = DateUtils.dateOnly(DateTime.now());
      expect(picker.lastDate, latestDateOfBirth(today));
      expect(picker.firstDate, earliestDateOfBirth(today));
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();
      await save(tester);
      expect(
        sent().toJson()['date_of_birth'],
        formatDate(latestDateOfBirth(today)),
      );
    });

    testWidgets('edit starts from the stored values; clearing DOB sends null', (
      tester,
    ) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      when(() => profile.putPersonalInfo(any()))
          .thenAnswer((_) async => filled());
      await pump(tester);
      await openEditor(tester);
      await tester.tap(find.byKey(const Key('personalInfo.dobClear')));
      await tester.pump();
      await save(tester);
      expect(sent().toJson(), {
        'name': {'first': 'Ánh', 'last': 'Nguyễn'},
        'phone_number': '+84901234567',
        'date_of_birth': null,
        'address': {'line1': '1 Le Loi', 'city': 'HCMC', 'country': 'VN'},
        'national_id': {'type': 'cccd', 'number': '079123456123'},
      });
    });

    testWidgets('422 field errors map to the dotted fields', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      when(() => profile.putPersonalInfo(any())).thenThrow(
        const ApiFailure(
          'validation_failed',
          status: 422,
          fieldErrors: [
            FieldError(field: 'name.last', code: 'invalid_characters'),
            FieldError(field: 'address.country', code: 'invalid_format'),
            FieldError(field: 'national_id.number', code: 'too_long'),
            FieldError(field: 'date_of_birth', code: 'out_of_range'),
          ],
        ),
      );
      await pump(tester);
      await openEditor(tester);
      await save(tester);
      expect(find.text('Please check the highlighted fields.'), findsOneWidget);
      InputDecoration deco(String path) => tester
          .widget<TextField>(
            find.descendant(
              of: field(path),
              matching: find.byType(TextField),
              matchRoot: true,
            ),
          )
          .decoration!;
      expect(deco(PiiField.country).errorText, 'Invalid format.');
      expect(deco(PiiField.nationalIdNumber).errorText, 'Too long.');
      expect(
        deco(PiiField.dateOfBirth).errorText,
        'You must be between 13 and 120 years old.',
      );
      expect(
        deco(PiiField.lastName).errorText,
        'Contains characters that are not allowed.',
      );
      expect(deco(PiiField.firstName).errorText, isNull);
      expect(deco(PiiField.phoneNumber).errorText, isNull);
    });

    testWidgets('403 email_not_verified -> message + verify now', (
      tester,
    ) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      when(() => profile.putPersonalInfo(any()))
          .thenThrow(const ApiFailure('email_not_verified', status: 403));
      await pump(tester);
      await openEditor(tester);
      await save(tester);
      expect(
        find.text('Verify your email or phone number to continue.'),
        findsOneWidget,
      );
      await tester.tap(find.byKey(const Key('personalInfo.verifyNow')));
      await tester.pumpAndSettle();
      expect(find.text('route:/verify-email'), findsOneWidget);
    });

    testWidgets('503 on save -> banner; retry resubmits', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      var calls = 0;
      when(() => profile.putPersonalInfo(any())).thenAnswer((_) async {
        if (calls++ == 0) {
          throw const ApiFailure('dependency_unavailable', status: 503);
        }
        return filled();
      });
      await pump(tester);
      await openEditor(tester);
      await save(tester);
      expect(find.byKey(const Key('flow.failure')), findsOneWidget);
      await tester.tap(find.text('Retry'));
      await tester.pumpAndSettle();
      verify(() => profile.putPersonalInfo(any())).called(2);
      expect(find.text('Personal information saved.'), findsOneWidget);
    });

    testWidgets('401 -> signed out', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      when(() => profile.putPersonalInfo(any()))
          .thenThrow(const UnauthenticatedFailure());
      await pump(tester);
      await openEditor(tester);
      await save(tester);
      verify(auth.forget).called(1);
      expect(
        authStateOf(tester, find.byType(MaterialApp)),
        isA<Unauthenticated>(),
      );
    });
  });

  group('erase', () {
    testWidgets('cancel keeps data; confirm erases and reloads', (
      tester,
    ) async {
      var erased = false;
      when(profile.getPersonalInfo)
          .thenAnswer((_) async => erased ? const PersonalInfo() : filled());
      when(profile.erasePersonalInfo).thenAnswer((_) async => erased = true);
      await pump(tester);

      await tester.tap(find.byKey(const Key('personalInfo.erase')));
      await tester.pumpAndSettle();
      expect(find.text('Erase personal information?'), findsOneWidget);
      await tester.tap(find.byKey(const Key('personalInfo.eraseCancel')));
      await tester.pumpAndSettle();
      verifyNever(profile.erasePersonalInfo);
      expect(find.text('+84901234567'), findsOneWidget);

      await tester.tap(find.byKey(const Key('personalInfo.erase')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('personalInfo.eraseConfirm')));
      await tester.pumpAndSettle();
      verify(profile.erasePersonalInfo).called(1);
      expect(find.text('Personal information erased.'), findsOneWidget);
      expect(find.text('Not provided'), findsNWidgets(5));
    });

    testWidgets('erase failure -> banner, data kept', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      when(profile.erasePersonalInfo).thenThrow(const NetworkFailure());
      await pump(tester);
      await tester.tap(find.byKey(const Key('personalInfo.erase')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('personalInfo.eraseConfirm')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('flow.failure')), findsOneWidget);
      expect(find.text('+84901234567'), findsOneWidget);
    });
  });

  group('leaving while a request is in flight', () {
    testWidgets('PUT completes after the screen is gone', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      final put = Completer<PersonalInfo>();
      when(() => profile.putPersonalInfo(any())).thenAnswer((_) => put.future);
      final router = await pump(tester);
      await openEditor(tester);
      await tester.tap(find.byKey(const Key('personalInfo.save')));
      await tester.pump();
      router.go(Routes.profile);
      await tester.pumpAndSettle();
      expect(find.byType(PersonalInfoScreen), findsNothing);
      put.complete(filled());
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      expect(find.text('route:/profile'), findsOneWidget);
    });

    testWidgets('DELETE fails after the screen is gone', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      final del = Completer<void>();
      when(profile.erasePersonalInfo).thenAnswer((_) => del.future);
      final router = await pump(tester);
      await tester.tap(find.byKey(const Key('personalInfo.erase')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('personalInfo.eraseConfirm')));
      await tester.pump();
      router.go(Routes.profile);
      await tester.pumpAndSettle();
      del.completeError(const NetworkFailure());
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
    });

    testWidgets('401 after the screen is gone still signs out', (tester) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      final del = Completer<void>();
      when(profile.erasePersonalInfo).thenAnswer((_) => del.future);
      final router = await pump(tester);
      await tester.tap(find.byKey(const Key('personalInfo.erase')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('personalInfo.eraseConfirm')));
      await tester.pump();
      router.go(Routes.profile);
      await tester.pumpAndSettle();
      del.completeError(const UnauthenticatedFailure());
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      verify(auth.forget).called(1);
      expect(
        authStateOf(tester, find.byType(MaterialApp)),
        isA<Unauthenticated>(),
      );
    });
  });

  group('screen protection', () {
    testWidgets('FLAG_SECURE on enter, cleared on leave (Android channel)', (
      tester,
    ) async {
      final calls = <String>[];
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        const MethodChannel(SecureScreen.channelName),
        (call) async {
          calls.add(call.method);
          return null;
        },
      );
      addTearDown(
        () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
          const MethodChannel(SecureScreen.channelName),
          null,
        ),
      );
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      final router = await pump(tester);
      expect(calls, ['enable']);
      router.go(Routes.profile);
      await tester.pumpAndSettle();
      expect(calls, ['enable', 'disable']);
    });

    testWidgets('opaque cover while the app is inactive / paused', (
      tester,
    ) async {
      when(profile.getPersonalInfo).thenAnswer((_) async => filled());
      await pump(tester);
      final cover = find.byKey(const Key('sensitive.cover'));
      expect(cover, findsNothing);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
      await tester.pump();
      expect(cover, findsOneWidget);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await tester.pump();
      expect(cover, findsOneWidget);
      tester.binding
        ..handleAppLifecycleStateChanged(AppLifecycleState.hidden)
        ..handleAppLifecycleStateChanged(AppLifecycleState.inactive)
        ..handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pump();
      expect(cover, findsNothing);
      expect(find.text('+84901234567'), findsOneWidget);
    });
  });
}
