// identity-service `/v1/me/personal-info` models
// (api/openapi/identity-service.v1.yaml: PersonalInfo, Address, NationalId).
// Hand-written like `me.dart`. Values are PII: these types never override
// `toString` with field values and are never persisted or logged.

/// `PersonName` schema: `{first?, last?}`, each trimmed, ≤ 100 runes, no
/// control characters. An object with both parts empty means "no name".
class PersonName {
  const PersonName({this.first, this.last});

  factory PersonName.fromJson(Map<String, dynamic> json) => PersonName(
    first: json['first'] as String?,
    last: json['last'] as String?,
  );

  /// `null` when both parts are missing or blank (treated as absent).
  static PersonName? parse(Object? json) {
    if (json is! Map) return null;
    final n = PersonName.fromJson(json.cast<String, dynamic>());
    return n.isEmpty ? null : n;
  }

  final String? first;
  final String? last;

  bool get isEmpty =>
      (first?.trim().isEmpty ?? true) && (last?.trim().isEmpty ?? true);

  /// "first last" (blank parts skipped).
  String get full => [
    first,
    last,
  ].whereType<String>().where((s) => s.trim().isNotEmpty).join(' ');

  /// Blank parts are omitted (`additionalProperties: false`).
  Map<String, dynamic> toJson() => {
    if (first != null && first!.isNotEmpty) 'first': first,
    if (last != null && last!.isNotEmpty) 'last': last,
  };

  @override
  String toString() => 'PersonName(<redacted>)';
}

/// `Address` schema. Required: line1, city, country (ISO 3166-1 alpha-2).
class Address {
  const Address({
    required this.line1,
    required this.city,
    required this.country,
    this.line2,
    this.region,
    this.postalCode,
  });

  factory Address.fromJson(Map<String, dynamic> json) => Address(
    line1: json['line1'] as String? ?? '',
    line2: json['line2'] as String?,
    city: json['city'] as String? ?? '',
    region: json['region'] as String?,
    postalCode: json['postal_code'] as String?,
    country: json['country'] as String? ?? '',
  );

  final String line1;
  final String? line2;
  final String city;
  final String? region;
  final String? postalCode;
  final String country;

  /// Optional fields are omitted when null (`additionalProperties: false`).
  Map<String, dynamic> toJson() => {
    'line1': line1,
    if (line2 != null) 'line2': line2,
    'city': city,
    if (region != null) 'region': region,
    if (postalCode != null) 'postal_code': postalCode,
    'country': country,
  };

  @override
  String toString() => 'Address(<redacted>)';
}

/// `NationalId` schema. `type` ∈ [types]; `number` upper-cased
/// `^[A-Z0-9]{6,20}$`. Unknown server types are kept verbatim.
class NationalId {
  const NationalId({required this.type, required this.number});

  factory NationalId.fromJson(Map<String, dynamic> json) => NationalId(
    type: json['type'] as String? ?? '',
    number: json['number'] as String? ?? '',
  );

  static const cccd = 'cccd';
  static const passport = 'passport';
  static const other = 'other';
  static const List<String> types = [cccd, passport, other];

  final String type;
  final String number;

  Map<String, dynamic> toJson() => {'type': type, 'number': number};

  @override
  String toString() => 'NationalId(<redacted>)';
}

/// `PersonalInfo` schema. Every field is nullable ("never set" → `null`).
/// `PUT` is a full replacement, so [toJson] always sends every writable key
/// (explicit `null` clears); `updated_at` is read-only and never sent.
class PersonalInfo {
  const PersonalInfo({
    this.name,
    this.phoneNumber,
    this.dateOfBirth,
    this.address,
    this.nationalId,
    this.updatedAt,
  });

  factory PersonalInfo.fromJson(Map<String, dynamic> json) => PersonalInfo(
    name: PersonName.parse(json['name']),
    phoneNumber: json['phone_number'] as String?,
    dateOfBirth: json['date_of_birth'] is String
        ? parseDate(json['date_of_birth'] as String)
        : null,
    address: json['address'] is Map
        ? Address.fromJson((json['address'] as Map).cast<String, dynamic>())
        : null,
    nationalId: json['national_id'] is Map
        ? NationalId.fromJson(
            (json['national_id'] as Map).cast<String, dynamic>(),
          )
        : null,
    updatedAt: json['updated_at'] is String
        ? DateTime.parse(json['updated_at'] as String)
        : null,
  );

  /// `null` when never set or both parts are empty.
  final PersonName? name;
  final String? phoneNumber;

  /// Calendar date only (year/month/day are meaningful; time is ignored).
  final DateTime? dateOfBirth;
  final Address? address;
  final NationalId? nationalId;
  final DateTime? updatedAt;

  bool get isEmpty =>
      name == null &&
      phoneNumber == null &&
      dateOfBirth == null &&
      address == null &&
      nationalId == null;

  Map<String, dynamic> toJson() => {
    'name': name == null || name!.isEmpty ? null : name!.toJson(),
    'phone_number': phoneNumber,
    'date_of_birth': dateOfBirth == null ? null : formatDate(dateOfBirth!),
    'address': address?.toJson(),
    'national_id': nationalId?.toJson(),
  };

  @override
  String toString() => 'PersonalInfo(<redacted>)';
}

/// `YYYY-MM-DD` → local calendar date (no time zone shift).
DateTime parseDate(String value) {
  final d = DateTime.parse(value);
  return DateTime(d.year, d.month, d.day);
}

/// Calendar date → `YYYY-MM-DD`.
String formatDate(DateTime d) =>
    '${d.year.toString().padLeft(4, '0')}-'
    '${d.month.toString().padLeft(2, '0')}-'
    '${d.day.toString().padLeft(2, '0')}';
