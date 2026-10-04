import 'package:flutter/material.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

/// Email | Phone choice plus the matching text field (PLI-FR-08). Keys:
/// `<keyPrefix>.loginType`, `<keyPrefix>.loginType.email|phone` and
/// `<keyPrefix>.login` (the text field).
class LoginIdentifierField extends StatelessWidget {
  const LoginIdentifierField({
    required this.keyPrefix,
    required this.controller,
    required this.type,
    required this.onTypeChanged,
    this.onChanged,
    this.errorText,
    this.textInputAction,
    super.key,
  });

  final String keyPrefix;
  final TextEditingController controller;
  final LoginType type;
  final ValueChanged<LoginType> onTypeChanged;

  /// Called when the typed value changes (e.g. to clear a stale error).
  final ValueChanged<String>? onChanged;
  final String? errorText;
  final TextInputAction? textInputAction;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final isEmail = type == LoginType.email;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Semantics(
          label: l10n.loginTypeLabel,
          child: SegmentedButton<LoginType>(
            key: Key('$keyPrefix.loginType'),
            showSelectedIcon: false,
            segments: [
              ButtonSegment(
                value: LoginType.email,
                icon: const Icon(Icons.email_outlined),
                label: Text(
                  l10n.loginTypeEmail,
                  key: Key('$keyPrefix.loginType.email'),
                ),
              ),
              ButtonSegment(
                value: LoginType.phone,
                icon: const Icon(Icons.phone_iphone),
                label: Text(
                  l10n.loginTypePhone,
                  key: Key('$keyPrefix.loginType.phone'),
                ),
              ),
            ],
            selected: {type},
            onSelectionChanged: (s) {
              if (s.first == type) return;
              // The other type's text makes no sense in this field.
              controller.clear();
              onTypeChanged(s.first);
            },
          ),
        ),
        const SizedBox(height: 12),
        TextField(
          key: Key('$keyPrefix.login'),
          controller: controller,
          keyboardType: isEmail
              ? TextInputType.emailAddress
              : TextInputType.phone,
          autofillHints: isEmail
              ? const [AutofillHints.email]
              : const [AutofillHints.telephoneNumber],
          autocorrect: false,
          textInputAction: textInputAction,
          onChanged: onChanged,
          decoration: InputDecoration(
            labelText: isEmail ? l10n.email : l10n.phoneNumber,
            hintText: isEmail ? null : l10n.loginPhoneHint,
            errorText: errorText,
          ),
        ),
      ],
    );
  }
}

/// Message for a login identifier field error code (client-side check or
/// identity-service `errors[].code`).
String loginFieldMessage(AppLocalizations l10n, LoginType type, String code) =>
    switch (code) {
      LoginFieldCodes.invalidFormat =>
        type == LoginType.email
            ? l10n.loginInvalidEmail
            : l10n.loginInvalidPhone,
      LoginFieldCodes.tooLong => l10n.fieldTooLong,
      LoginFieldCodes.invalidCharacters => l10n.fieldInvalidCharacters,
      LoginFieldCodes.unsupportedCountry => l10n.loginUnsupportedCountry,
      _ => l10n.validationFailed,
    };

/// Field error code on the login from an identity-service
/// `422 validation_failed` (fields `login.value`, `login.type`), or null.
String? loginFieldCode(AppFailure? failure) {
  if (failure is! ApiFailure || failure.code != 'validation_failed') {
    return null;
  }
  for (final e in failure.fieldErrors) {
    if (e.field == 'login.value' || e.field == 'login.type') {
      return e.code;
    }
  }
  return null;
}

/// Error text for the login field: the local format check first, then
/// identity-service's field error, then Kratos' messages ([kratosError]).
String? loginErrorText(
  AppLocalizations l10n,
  LoginType type, {
  String? localCode,
  AppFailure? failure,
  String? kratosError,
}) {
  final code = localCode ?? loginFieldCode(failure);
  if (code != null) return loginFieldMessage(l10n, type, code);
  return kratosError;
}
