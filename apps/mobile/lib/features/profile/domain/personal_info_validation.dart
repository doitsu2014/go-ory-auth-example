import 'package:go_ory_auth_mobile/features/profile/domain/personal_info.dart';

// Client-side validation mirroring identity-service (PII-FR-03). Error codes
// and dotted field paths are the same as the server's `errors[]`, so client
// and server errors render through one mapping.

abstract final class PiiField {
  static const name = 'name';
  static const firstName = 'name.first';
  static const lastName = 'name.last';
  static const phoneNumber = 'phone_number';
  static const dateOfBirth = 'date_of_birth';
  static const address = 'address';
  static const line1 = 'address.line1';
  static const line2 = 'address.line2';
  static const city = 'address.city';
  static const region = 'address.region';
  static const postalCode = 'address.postal_code';
  static const country = 'address.country';
  static const nationalId = 'national_id';
  static const nationalIdType = 'national_id.type';
  static const nationalIdNumber = 'national_id.number';
}

abstract final class PiiErrorCode {
  static const invalidFormat = 'invalid_format';
  static const tooLong = 'too_long';
  static const outOfRange = 'out_of_range';
  static const required = 'required';
  static const invalidCharacters = 'invalid_characters';
}

const maxNameLength = 100;
const minAge = 13;
const maxAge = 120;

final _e164 = RegExp(r'^\+[1-9]\d{7,14}$');
final _phoneSeparators = RegExp(r'[\s.\-]');
final _country = RegExp(r'^[A-Z]{2}$');
final _nationalIdNumber = RegExp(r'^[A-Z0-9]{6,20}$');
// Go `unicode.IsControl`: C0, DEL and C1.
final _controlChars = RegExp(r'[\x00-\x1F\x7F-\x9F]');
// Unicode format characters (category Cf: ZWSP, bidi overrides/isolates,
// BOM, ...). Rejected in names, like the server.
final _formatChars = RegExp(r'\p{Cf}', unicode: true);

/// Name part: trimmed; a part of only whitespace / format characters is
/// empty.
String normaliseNamePart(String raw) {
  final v = raw.trim();
  return v.replaceAll(_formatChars, '').trim().isEmpty ? '' : v;
}

/// Strips spaces, dashes and dots (the server does the same).
String normalisePhone(String raw) => raw.replaceAll(_phoneSeparators, '');

String normaliseCountry(String raw) => raw.trim().toUpperCase();

String normaliseNationalIdNumber(String raw) =>
    raw.replaceAll(RegExp(r'\s'), '').toUpperCase();

/// Today's calendar date in UTC (the server computes age in UTC).
DateTime utcToday([DateTime? now]) {
  final u = (now ?? DateTime.now()).toUtc();
  return DateTime(u.year, u.month, u.day);
}

/// Whole years between [dob] and [today].
int ageOn(DateTime dob, DateTime today) {
  var age = today.year - dob.year;
  if (today.month < dob.month ||
      (today.month == dob.month && today.day < dob.day)) {
    age--;
  }
  return age;
}

/// Earliest / latest selectable date of birth for an age of 13–120 [today].
DateTime earliestDateOfBirth(DateTime today) =>
    DateTime(today.year - maxAge - 1, today.month, today.day + 1);
DateTime latestDateOfBirth(DateTime today) =>
    DateTime(today.year - minAge, today.month, today.day);

/// Raw form input. Empty strings mean "not provided".
class PersonalInfoDraft {
  const PersonalInfoDraft({
    this.firstName = '',
    this.lastName = '',
    this.phoneNumber = '',
    this.dateOfBirth,
    this.line1 = '',
    this.line2 = '',
    this.city = '',
    this.region = '',
    this.postalCode = '',
    this.country = '',
    this.nationalIdType,
    this.nationalIdNumber = '',
  });

  final String firstName;
  final String lastName;
  final String phoneNumber;
  final DateTime? dateOfBirth;
  final String line1;
  final String line2;
  final String city;
  final String region;
  final String postalCode;
  final String country;
  final String? nationalIdType;
  final String nationalIdNumber;

  bool get _hasAddress => [
    line1,
    line2,
    city,
    region,
    postalCode,
    country,
  ].any((s) => s.trim().isNotEmpty);

  bool get _hasNationalId =>
      nationalIdType != null || nationalIdNumber.trim().isNotEmpty;

  /// Field path → error code. Empty when valid.
  Map<String, String> validate(DateTime today) {
    final errors = <String, String>{};

    // Same text rules as the server: trimmed, length in runes (not UTF-16
    // units, so Vietnamese diacritics and astral characters count once),
    // no C0/C1 control characters.
    void text(String field, String value, int max, {bool required = false}) {
      final v = value.trim();
      if (v.isEmpty) {
        if (required) errors[field] = PiiErrorCode.required;
      } else if (v.runes.length > max) {
        errors[field] = PiiErrorCode.tooLong;
      } else if (_controlChars.hasMatch(v)) {
        errors[field] = PiiErrorCode.invalidCharacters;
      }
    }

    void name(String field, String value) {
      final v = normaliseNamePart(value);
      text(field, v, maxNameLength);
      if (v.isNotEmpty &&
          !errors.containsKey(field) &&
          _formatChars.hasMatch(v)) {
        errors[field] = PiiErrorCode.invalidCharacters;
      }
    }

    name(PiiField.firstName, firstName);
    name(PiiField.lastName, lastName);

    final phone = normalisePhone(phoneNumber.trim());
    if (phone.isNotEmpty && !_e164.hasMatch(phone)) {
      errors[PiiField.phoneNumber] = PiiErrorCode.invalidFormat;
    }

    final dob = dateOfBirth;
    if (dob != null) {
      final age = ageOn(dob, today);
      if (age < minAge || age > maxAge) {
        errors[PiiField.dateOfBirth] = PiiErrorCode.outOfRange;
      }
    }

    if (_hasAddress) {
      text(PiiField.line1, line1, 200, required: true);
      text(PiiField.line2, line2, 200);
      text(PiiField.city, city, 100, required: true);
      text(PiiField.region, region, 100);
      text(PiiField.postalCode, postalCode, 20);
      final c = normaliseCountry(country);
      if (c.isEmpty) {
        errors[PiiField.country] = PiiErrorCode.required;
      } else if (!_country.hasMatch(c)) {
        errors[PiiField.country] = PiiErrorCode.invalidFormat;
      }
    }

    if (_hasNationalId) {
      if (nationalIdType == null) {
        errors[PiiField.nationalIdType] = PiiErrorCode.required;
      } else if (!NationalId.types.contains(nationalIdType)) {
        errors[PiiField.nationalIdType] = PiiErrorCode.invalidFormat;
      }
      final n = normaliseNationalIdNumber(nationalIdNumber);
      if (n.isEmpty) {
        errors[PiiField.nationalIdNumber] = PiiErrorCode.required;
      } else if (n.runes.length > 20) {
        errors[PiiField.nationalIdNumber] = PiiErrorCode.tooLong;
      } else if (!_nationalIdNumber.hasMatch(n)) {
        errors[PiiField.nationalIdNumber] = PiiErrorCode.invalidFormat;
      }
    }
    return errors;
  }

  /// Normalised request body. Call only when [validate] is empty.
  PersonalInfo toPersonalInfo() {
    String? opt(String v) => v.trim().isEmpty ? null : v.trim();
    final phone = normalisePhone(phoneNumber.trim());
    final first = opt(normaliseNamePart(firstName));
    final last = opt(normaliseNamePart(lastName));
    return PersonalInfo(
      name: first == null && last == null
          ? null
          : PersonName(first: first, last: last),
      phoneNumber: phone.isEmpty ? null : phone,
      dateOfBirth: dateOfBirth,
      address: _hasAddress
          ? Address(
              line1: line1.trim(),
              line2: opt(line2),
              city: city.trim(),
              region: opt(region),
              postalCode: opt(postalCode),
              country: normaliseCountry(country),
            )
          : null,
      nationalId: _hasNationalId
          ? NationalId(
              type: nationalIdType!,
              number: normaliseNationalIdNumber(nationalIdNumber),
            )
          : null,
    );
  }
}
