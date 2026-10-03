import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info_validation.dart';

void main() {
  group('PersonalInfo JSON', () {
    test('fromJson: full payload', () {
      final p = PersonalInfo.fromJson({
        'phone_number': '+84901234567',
        'date_of_birth': '1990-05-17',
        'address': {
          'line1': '1 Le Loi',
          'line2': null,
          'city': 'HCMC',
          'region': 'D1',
          'postal_code': '700000',
          'country': 'VN',
        },
        'national_id': {'type': 'cccd', 'number': '079123456123'},
        'updated_at': '2026-10-03T08:00:00Z',
      });
      expect(p.phoneNumber, '+84901234567');
      expect(p.dateOfBirth, DateTime(1990, 5, 17));
      expect(p.address!.line1, '1 Le Loi');
      expect(p.address!.line2, isNull);
      expect(p.address!.region, 'D1');
      expect(p.address!.postalCode, '700000');
      expect(p.address!.country, 'VN');
      expect(p.nationalId!.type, NationalId.cccd);
      expect(p.nationalId!.number, '079123456123');
      expect(p.updatedAt, DateTime.utc(2026, 10, 3, 8));
      expect(p.isEmpty, isFalse);
    });

    test('fromJson: never set -> all null', () {
      final p = PersonalInfo.fromJson({
        'phone_number': null,
        'date_of_birth': null,
        'address': null,
        'national_id': null,
        'updated_at': null,
      });
      expect(p.isEmpty, isTrue);
      expect(p.updatedAt, isNull);
      expect(PersonalInfo.fromJson(const {}).isEmpty, isTrue);
    });

    test('toJson: full replacement sends explicit nulls, no updated_at', () {
      expect(PersonalInfo(updatedAt: DateTime.utc(2026)).toJson(), {
        'phone_number': null,
        'date_of_birth': null,
        'address': null,
        'national_id': null,
      });
    });

    test('toJson: values; optional address fields omitted when null', () {
      final json = PersonalInfo(
        phoneNumber: '+84901234567',
        dateOfBirth: DateTime(2001, 2, 3),
        address: const Address(line1: 'L1', city: 'C', country: 'VN'),
        nationalId: const NationalId(type: 'passport', number: 'B1234567'),
      ).toJson();
      expect(json, {
        'phone_number': '+84901234567',
        'date_of_birth': '2001-02-03',
        'address': {'line1': 'L1', 'city': 'C', 'country': 'VN'},
        'national_id': {'type': 'passport', 'number': 'B1234567'},
      });
    });

    test('round trip', () {
      const src = {
        'phone_number': '+14155550100',
        'date_of_birth': '1985-12-31',
        'address': {
          'line1': 'a',
          'line2': 'b',
          'city': 'c',
          'region': 'd',
          'postal_code': 'e',
          'country': 'US',
        },
        'national_id': {'type': 'other', 'number': 'X123456'},
      };
      expect(PersonalInfo.fromJson(src).toJson(), src);
    });

    test('toString never prints values', () {
      const p = PersonalInfo(
        phoneNumber: '+84901234567',
        address: Address(line1: 'secret st', city: 'C', country: 'VN'),
        nationalId: NationalId(type: 'cccd', number: '079123456123'),
      );
      for (final s in [p, p.address, p.nationalId].map((o) => '$o')) {
        expect(s, isNot(contains('84901234567')));
        expect(s, isNot(contains('secret')));
        expect(s, isNot(contains('079123456123')));
      }
    });
  });

  group('validation (mirrors PII-FR-03)', () {
    final today = DateTime(2026, 10, 3);

    test('phone: separators stripped, E.164 enforced', () {
      expect(normalisePhone('+84 90-123.4567'), '+84901234567');
      expect(
        const PersonalInfoDraft(phoneNumber: '+84 90-123.4567').validate(today),
        isEmpty,
      );
      for (final bad in ['0901234567', '+0901234567', '+8490', '+84abc12345']) {
        expect(PersonalInfoDraft(phoneNumber: bad).validate(today), {
          PiiField.phoneNumber: PiiErrorCode.invalidFormat,
        }, reason: bad);
      }
      expect(
        const PersonalInfoDraft(phoneNumber: '+84 901 234 567')
            .toPersonalInfo()
            .phoneNumber,
        '+84901234567',
      );
    });

    test('date of birth: age 13-120 inclusive', () {
      Map<String, String> v(DateTime d) =>
          PersonalInfoDraft(dateOfBirth: d).validate(today);
      expect(v(DateTime(2013, 10, 3)), isEmpty); // 13 today
      expect(v(DateTime(2013, 10, 4)), {
        PiiField.dateOfBirth: PiiErrorCode.outOfRange,
      });
      expect(v(DateTime(1906, 10, 4)), isEmpty); // 119, turns 120 tomorrow
      expect(v(DateTime(1905, 10, 4)), isEmpty); // 120
      expect(v(DateTime(1905, 10, 3)), {
        PiiField.dateOfBirth: PiiErrorCode.outOfRange,
      }); // 121
      expect(ageOn(latestDateOfBirth(today), today), 13);
      expect(ageOn(earliestDateOfBirth(today), today), 120);
    });

    test('address: required fields when any is set; country alpha-2', () {
      expect(const PersonalInfoDraft(city: 'HCMC').validate(today), {
        PiiField.line1: PiiErrorCode.required,
        PiiField.country: PiiErrorCode.required,
      });
      expect(
        const PersonalInfoDraft(
          line1: 'L',
          city: 'C',
          country: 'V1',
        ).validate(today),
        {PiiField.country: PiiErrorCode.invalidFormat},
      );
      expect(
        PersonalInfoDraft(
          line1: 'x' * 201,
          city: 'C',
          postalCode: 'p' * 21,
          region: 'a\u0007b',
          country: 'vn',
        ).validate(today),
        {
          PiiField.line1: PiiErrorCode.tooLong,
          PiiField.postalCode: PiiErrorCode.tooLong,
          PiiField.region: PiiErrorCode.invalidCharacters,
        },
      );
      final a = const PersonalInfoDraft(
        line1: ' L ',
        city: 'C',
        country: ' vn ',
      ).toPersonalInfo().address!;
      expect(a.toJson(), {'line1': 'L', 'city': 'C', 'country': 'VN'});
    });

    test(r'national id: type + number, upper-cased, ^[A-Z0-9]{6,20}$', () {
      expect(
        const PersonalInfoDraft(nationalIdNumber: 'b1234567').validate(today),
        {PiiField.nationalIdType: PiiErrorCode.required},
      );
      expect(const PersonalInfoDraft(nationalIdType: 'cccd').validate(today), {
        PiiField.nationalIdNumber: PiiErrorCode.required,
      });
      for (final bad in ['ab12', 'AB-12345', 'ÀB12345']) {
        expect(
          PersonalInfoDraft(
            nationalIdType: 'other',
            nationalIdNumber: bad,
          ).validate(today),
          {PiiField.nationalIdNumber: PiiErrorCode.invalidFormat},
          reason: bad,
        );
      }
      const ok = PersonalInfoDraft(
        nationalIdType: 'passport',
        nationalIdNumber: 'b1234567',
      );
      expect(ok.validate(today), isEmpty);
      expect(ok.toPersonalInfo().nationalId!.toJson(), {
        'type': 'passport',
        'number': 'B1234567',
      });
    });

    test('lengths count runes (as the server does)', () {
      // 200 astral code points = 400 UTF-16 code units: still valid.
      final emoji200 = '\u{1F600}' * 200;
      expect(
        PersonalInfoDraft(
          line1: emoji200,
          city: 'C',
          country: 'VN',
        ).validate(today),
        isEmpty,
      );
      expect(
        PersonalInfoDraft(
          line1: '\u{1F600}' * 201,
          city: 'C',
          country: 'VN',
        ).validate(today),
        {PiiField.line1: PiiErrorCode.tooLong},
      );
    });

    test('control characters: C0, DEL and C1 rejected', () {
      for (final c in [
        '\u0000',
        '\u001F',
        '\u007F',
        '\u0080',
        '\u0085',
        '\u009F',
      ]) {
        expect(
          PersonalInfoDraft(
            line1: 'a${c}b',
            city: 'C',
            country: 'VN',
          ).validate(today),
          {PiiField.line1: PiiErrorCode.invalidCharacters},
          reason: c.codeUnitAt(0).toRadixString(16),
        );
      }
      expect(
        const PersonalInfoDraft(
          line1: 'Đường Lê Lợi \u00A0',
          city: 'C',
          country: 'VN',
        ).validate(today),
        isEmpty,
      );
    });

    test('age uses the UTC calendar date', () {
      // 2026-10-03 23:30 at UTC-5 is already 2026-10-04 in UTC.
      final now = DateTime.parse('2026-10-03T23:30:00-05:00');
      expect(utcToday(now), DateTime(2026, 10, 4));
      // Turns 13 on 2026-10-04: valid by the server's (UTC) clock.
      expect(
        PersonalInfoDraft(dateOfBirth: DateTime(2013, 10, 4))
            .validate(utcToday(now)),
        isEmpty,
      );
    });

    test('empty draft is valid and clears everything', () {
      const d = PersonalInfoDraft();
      expect(d.validate(today), isEmpty);
      expect(d.toPersonalInfo().isEmpty, isTrue);
    });
  });
}
