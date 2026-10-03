import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/me.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/profile_screen.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// `PATCH /v1/me` (display_name, locale). `403 email_not_verified` routes to
/// verification; `422 validation_failed` shows field errors.
class EditProfileScreen extends ConsumerStatefulWidget {
  const EditProfileScreen({required this.me, super.key});

  final Me? me;

  @override
  ConsumerState<EditProfileScreen> createState() => _EditProfileScreenState();
}

class _EditProfileScreenState extends ConsumerState<EditProfileScreen> {
  static const locales = ['vi-VN', 'en-US'];
  late final _displayName = TextEditingController(
    text: widget.me?.displayName ?? '',
  );
  late String _locale = locales.contains(widget.me?.locale)
      ? widget.me!.locale
      : locales.first;
  bool _saving = false;
  AppFailure? _failure;

  @override
  void dispose() {
    _displayName.dispose();
    super.dispose();
  }

  String? _fieldError(AppLocalizations l10n, String field) {
    final f = _failure;
    if (f is! ApiFailure) return null;
    final errs = f.fieldErrors.where((e) => e.field == field);
    if (errs.isEmpty) return null;
    return errs
        .map(
          (e) => switch (e.code) {
            'too_long' => l10n.fieldTooLong,
            'required' => l10n.fieldRequired,
            _ => l10n.validationFailed,
          },
        )
        .join('\n');
  }

  Future<void> _save() async {
    setState(() {
      _saving = true;
      _failure = null;
    });
    final name = _displayName.text.trim();
    try {
      await ref
          .read(profileRepositoryProvider)
          .updateMe(
            UpdateMeRequest(
              displayName: name.isEmpty ? null : name,
              clearDisplayName: name.isEmpty,
              locale: _locale,
            ),
          );
      ref.invalidate(meProvider);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(AppLocalizations.of(context).profileSaved)),
      );
      context.pop();
    } on ApiFailure catch (f) {
      if (f.code == 'email_not_verified') {
        ref.read(authControllerProvider.notifier).requireVerification();
        if (mounted) context.go(Routes.verifyEmail);
        return;
      }
      setState(() => _failure = f);
    } on AppFailure catch (f) {
      setState(() => _failure = f);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return FormPage(
      title: l10n.edit,
      children: [
        FailureBanner(failure: _failure),
        TextField(
          key: const Key('editProfile.displayName'),
          controller: _displayName,
          maxLength: 100,
          decoration: InputDecoration(
            labelText: l10n.displayName,
            errorText: _fieldError(l10n, 'display_name'),
          ),
        ),
        DropdownButtonFormField<String>(
          key: const Key('editProfile.locale'),
          initialValue: _locale,
          decoration: InputDecoration(
            labelText: l10n.locale,
            errorText: _fieldError(l10n, 'locale'),
          ),
          items: [
            for (final l in locales) DropdownMenuItem(value: l, child: Text(l)),
          ],
          onChanged: (v) => setState(() => _locale = v ?? _locale),
        ),
        SubmitButton(
          key: const Key('editProfile.save'),
          label: l10n.save,
          submitting: _saving,
          onPressed: _save,
        ),
      ],
    );
  }
}
