// identity-service `/v1/me` models (api/openapi/identity-service.v1.yaml).
// Hand-written instead of openapi-generator `dart-dio` (needs a JVM).

import 'package:go_ory_auth_mobile/core/identity/login_input.dart';

/// `LoginIdentifier`: the contact the customer signs in with (email
/// lower-cased or phone in E.164). This, not the Kratos traits, is what the
/// app displays (PLI-FR-08).
class LoginContact {
  const LoginContact({required this.type, required this.value});

  factory LoginContact.fromJson(Map<String, dynamic> json) => LoginContact(
    type: LoginType.fromWire(json['type'] as String?) ?? LoginType.email,
    value: json['value'] as String? ?? '',
  );

  final LoginType type;
  final String value;
}

/// `Me` schema. Required: id, login, email_verified, locale, created_at.
/// `email` is deprecated (present only for email logins). The deprecated
/// `name` is never returned (NAME-FR-07) and is ignored if present: the name
/// is personal info (`PersonalInfo.name`).
class Me {
  const Me({
    required this.id,
    required this.login,
    required this.emailVerified,
    required this.locale,
    required this.createdAt,
    this.email,
    this.displayName,
    this.avatarUrl,
  });

  factory Me.fromJson(Map<String, dynamic> json) => Me(
    id: json['id'] as String,
    login: _login(json),
    email: json['email'] as String?,
    emailVerified: json['email_verified'] as bool,
    locale: json['locale'] as String? ?? '',
    createdAt: DateTime.parse(json['created_at'] as String),
    displayName: json['display_name'] as String?,
    avatarUrl: json['avatar_url'] as String?,
  );

  final String id;
  final LoginContact login;

  /// Deprecated by the contract; use [login].
  final String? email;

  /// The login identifier (email or phone) is verified.
  final bool emailVerified;
  final String? displayName;
  final String? avatarUrl;
  final String locale;
  final DateTime createdAt;
}

/// `login`, or a legacy server's `email`.
LoginContact _login(Map<String, dynamic> json) {
  final login = json['login'];
  if (login is Map) return LoginContact.fromJson(login.cast<String, dynamic>());
  return LoginContact(
    type: LoginType.email,
    value: json['email'] as String? ?? '',
  );
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
