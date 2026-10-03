// identity-service `/v1/me` models (api/openapi/identity-service.v1.yaml).
// Hand-written instead of openapi-generator `dart-dio` (needs a JVM).

/// `Me` schema. Required: id, email, email_verified, locale, created_at.
/// The deprecated `name` is never returned (NAME-FR-07) and is ignored if
/// present: the name is personal info (`PersonalInfo.name`).
class Me {
  const Me({
    required this.id,
    required this.email,
    required this.emailVerified,
    required this.locale,
    required this.createdAt,
    this.displayName,
    this.avatarUrl,
  });

  factory Me.fromJson(Map<String, dynamic> json) => Me(
    id: json['id'] as String,
    email: json['email'] as String,
    emailVerified: json['email_verified'] as bool,
    locale: json['locale'] as String? ?? '',
    createdAt: DateTime.parse(json['created_at'] as String),
    displayName: json['display_name'] as String?,
    avatarUrl: json['avatar_url'] as String?,
  );

  final String id;
  final String email;
  final bool emailVerified;
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
