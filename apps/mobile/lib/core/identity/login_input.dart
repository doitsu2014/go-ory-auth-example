import 'package:flutter/foundation.dart';

/// What the customer signs in with (ADR-0013, PLI-FR-08). The address itself
/// never reaches Kratos: identity-service signs the customer in with the
/// account's opaque handle (ADR-0014).
enum LoginType {
  email,
  phone;

  /// Wire name (`LoginType` in the identity-service contract).
  String get wire => name;

  static LoginType? fromWire(String? value) => switch (value) {
    'email' => LoginType.email,
    'phone' => LoginType.phone,
    _ => null,
  };
}

/// Field error codes produced by [LoginInput.validate]; the same codes the
/// server returns in `errors[].code`.
abstract final class LoginFieldCodes {
  static const invalidFormat = 'invalid_format';
  static const tooLong = 'too_long';
  static const invalidCharacters = 'invalid_characters';
  static const unsupportedCountry = 'unsupported_country';
}

/// The email address or phone number as typed. Its value is never logged.
@immutable
final class LoginInput {
  const LoginInput({required this.type, required this.value});

  final LoginType type;

  /// As typed; the server normalises it.
  final String value;

  static const maxLength = 320;
  static const _maxEmailLength = 254;
  static final _email = RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]+$');
  static final _separators = RegExp(r'[\s\-.()]');
  static final _digits = RegExp(r'^[0-9]+$');
  static final _controls = RegExp(r'[\u0000-\u001f\u007f-\u009f]');

  /// Client-side format check for UX only (the server decides). Returns a
  /// field error code, or null when the input looks acceptable.
  ///
  /// Email: `local@domain.tld` shape. Phone: digits with optional spaces,
  /// dashes, dots or parentheses, either international (`+84…`, `0084…`) or
  /// Vietnamese national format (leading `0`).
  String? validate() {
    if (value.length > maxLength) return LoginFieldCodes.tooLong;
    if (_controls.hasMatch(value)) return LoginFieldCodes.invalidCharacters;
    final v = value.trim();
    switch (type) {
      case LoginType.email:
        if (v.length > _maxEmailLength) return LoginFieldCodes.tooLong;
        return _email.hasMatch(v) ? null : LoginFieldCodes.invalidFormat;
      case LoginType.phone:
        final e164 = _phoneE164(v);
        if (e164 == null) return LoginFieldCodes.invalidFormat;
        final digits = e164.length - 1;
        return digits >= 8 && digits <= 15
            ? null
            : LoginFieldCodes.invalidFormat;
    }
  }

  static String? _phoneE164(String raw) {
    final s = raw.replaceAll(_separators, '');
    final String international;
    if (s.startsWith('+')) {
      international = s.substring(1);
    } else if (s.startsWith('00')) {
      international = s.substring(2);
    } else if (s.startsWith('0')) {
      international = '84${s.substring(1)}';
    } else {
      return null;
    }
    if (!_digits.hasMatch(international) || international.startsWith('0')) {
      return null;
    }
    return '+$international';
  }

  @override
  bool operator ==(Object other) =>
      other is LoginInput && other.type == type && other.value == value;

  @override
  int get hashCode => Object.hash(type, value);

  @override
  String toString() => 'LoginInput(${type.name}, <redacted>)';
}
