// identity-service `/v1/me` models (api/openapi/identity-service.v1.yaml).
// Hand-written instead of openapi-generator `dart-dio` (needs a JVM).

class PersonName {
  const PersonName({this.first, this.last});

  factory PersonName.fromJson(Map<String, dynamic> json) => PersonName(
    first: json['first'] as String?,
    last: json['last'] as String?,
  );

  final String? first;
  final String? last;

  String get full =>
      [first, last].whereType<String>().where((s) => s.isNotEmpty).join(' ');

  Map<String, dynamic> toJson() => {
    if (first != null) 'first': first,
    if (last != null) 'last': last,
  };
}

/// `Me` schema. Required: id, email, email_verified, locale, created_at.
class Me {
  const Me({
    required this.id,
    required this.email,
    required this.emailVerified,
    required this.locale,
    required this.createdAt,
    this.name,
    this.displayName,
    this.avatarUrl,
  });

  factory Me.fromJson(Map<String, dynamic> json) => Me(
    id: json['id'] as String,
    email: json['email'] as String,
    emailVerified: json['email_verified'] as bool,
    locale: json['locale'] as String? ?? '',
    createdAt: DateTime.parse(json['created_at'] as String),
    name: json['name'] is Map
        ? PersonName.fromJson((json['name'] as Map).cast<String, dynamic>())
        : null,
    displayName: json['display_name'] as String?,
    avatarUrl: json['avatar_url'] as String?,
  );

  final String id;
  final String email;
  final bool emailVerified;
  final PersonName? name;
  final String? displayName;
  final String? avatarUrl;
  final String locale;
  final DateTime createdAt;
}

/// `UpdateMeRequest` (additionalProperties: false). Only set fields are sent.
class UpdateMeRequest {
  const UpdateMeRequest({
    this.displayName,
    this.locale,
    this.clearDisplayName = false,
  });

  final String? displayName;
  final String? locale;

  /// Sends `display_name: null` explicitly.
  final bool clearDisplayName;

  Map<String, dynamic> toJson() => {
    if (displayName != null || clearDisplayName)
      'display_name': clearDisplayName ? null : displayName,
    if (locale != null) 'locale': locale,
  };
}
