import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/platform/secure_screen.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info_validation.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// `GET /v1/me/personal-info`. Memory only (never persisted): auto-disposed
/// when the screen goes away, and dropped as soon as the signed-in identity
/// changes or signs out.
final FutureProvider<PersonalInfo> personalInfoProvider =
    FutureProvider.autoDispose<PersonalInfo>((ref) {
      final identityId = ref.watch(
        authControllerProvider.select(
          (s) => s is Authenticated ? s.session.identity.id : null,
        ),
      );
      if (identityId == null) throw const UnauthenticatedFailure();
      return ref.watch(profileRepositoryProvider).getPersonalInfo();
    }, retry: (_, _) => null);

/// View / edit / erase the caller's personal information (PII-FR-10).
///
/// - Validation mirrors the server (PII-FR-03); server `422` field errors
///   (dotted paths, codes only) render on the same fields.
/// - `403 email_not_verified` on save → message + "verify now".
/// - `503 dependency_unavailable` → banner with retry.
/// - `401` → the session is wiped and the router returns to sign-in.
/// - Values are never logged and never written to disk; screenshots are
///   blocked (Android `FLAG_SECURE`) and the app-switcher snapshot is covered.
class PersonalInfoScreen extends ConsumerStatefulWidget {
  const PersonalInfoScreen({super.key});

  @override
  ConsumerState<PersonalInfoScreen> createState() => _PersonalInfoScreenState();
}

class _PersonalInfoScreenState extends ConsumerState<PersonalInfoScreen> {
  final _phone = TextEditingController();
  final _dobText = TextEditingController();
  final _line1 = TextEditingController();
  final _line2 = TextEditingController();
  final _city = TextEditingController();
  final _region = TextEditingController();
  final _postalCode = TextEditingController();
  final _country = TextEditingController();
  final _idNumber = TextEditingController();

  late final List<TextEditingController> _controllers = [
    _phone,
    _dobText,
    _line1,
    _line2,
    _city,
    _region,
    _postalCode,
    _country,
    _idNumber,
  ];

  bool _editing = false;
  DateTime? _dob;
  String? _idType;
  Map<String, String> _errors = const {};
  AppFailure? _failure;
  bool _busy = false;

  @override
  void dispose() {
    for (final c in _controllers) {
      c
        ..clear()
        ..dispose();
    }
    super.dispose();
  }

  void _openEditor(PersonalInfo info) {
    final a = info.address;
    final id = info.nationalId;
    _phone.text = info.phoneNumber ?? '';
    _line1.text = a?.line1 ?? '';
    _line2.text = a?.line2 ?? '';
    _city.text = a?.city ?? '';
    _region.text = a?.region ?? '';
    _postalCode.text = a?.postalCode ?? '';
    _country.text = a?.country ?? '';
    _idNumber.text = id?.number ?? '';
    setState(() {
      _setDob(info.dateOfBirth);
      _idType = id?.type;
      _errors = const {};
      _failure = null;
      _editing = true;
    });
  }

  /// Leaves edit mode and drops the form copy of the values.
  void _closeEditor() {
    for (final c in _controllers) {
      c.clear();
    }
    setState(() {
      _dob = null;
      _idType = null;
      _errors = const {};
      _failure = null;
      _editing = false;
    });
  }

  void _setDob(DateTime? d) {
    _dob = d;
    _dobText.text = d == null
        ? ''
        : MaterialLocalizations.of(context).formatCompactDate(d);
  }

  PersonalInfoDraft _draft() => PersonalInfoDraft(
    phoneNumber: _phone.text,
    dateOfBirth: _dob,
    line1: _line1.text,
    line2: _line2.text,
    city: _city.text,
    region: _region.text,
    postalCode: _postalCode.text,
    country: _country.text,
    nationalIdType: _idType,
    nationalIdNumber: _idNumber.text,
  );

  Future<void> _pickDob() async {
    final today = utcToday();
    final first = earliestDateOfBirth(today);
    final last = latestDateOfBirth(today);
    final current = _dob;
    final picked = await showDatePicker(
      context: context,
      firstDate: first,
      lastDate: last,
      initialDate: current != null && !current.isBefore(first)
          ? (current.isAfter(last) ? last : current)
          : last,
      initialEntryMode: DatePickerEntryMode.calendarOnly,
    );
    if (picked != null && mounted) {
      setState(() => _setDob(DateUtils.dateOnly(picked)));
    }
  }

  /// [auth] is captured before the request: `ref` is unusable once the
  /// screen has been disposed while the request was in flight.
  Future<void> _sessionLost(
    UnauthenticatedFailure f,
    AuthController auth,
  ) async {
    if (mounted) {
      setState(() => _failure = f);
      if (ref.read(authControllerProvider) is! Authenticated) return;
    }
    // Single-flight; harmless if the 401 interceptor already signed out.
    await auth.handleUnauthorized();
  }

  Future<void> _save() async {
    if (_busy) return;
    final draft = _draft();
    final errors = draft.validate(utcToday());
    setState(() {
      _errors = errors;
      _failure = null;
    });
    if (errors.isNotEmpty) return;
    setState(() => _busy = true);
    final auth = ref.read(authControllerProvider.notifier);
    try {
      await ref
          .read(profileRepositoryProvider)
          .putPersonalInfo(draft.toPersonalInfo());
      // Left mid-request: the auto-disposed provider refetches on next visit.
      if (!mounted) return;
      ref.invalidate(personalInfoProvider);
      _closeEditor();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(AppLocalizations.of(context).personalInfoSaved)),
      );
    } on UnauthenticatedFailure catch (f) {
      await _sessionLost(f, auth);
    } on ApiFailure catch (f) {
      if (mounted) {
        setState(() {
          _failure = f;
          _errors = {for (final e in f.fieldErrors) e.field: e.code};
        });
      }
    } on AppFailure catch (f) {
      if (mounted) setState(() => _failure = f);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _erase() async {
    if (_busy) return;
    final l10n = AppLocalizations.of(context);
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        key: const Key('personalInfo.eraseDialog'),
        title: Text(l10n.personalInfoEraseConfirmTitle),
        content: Text(l10n.personalInfoEraseConfirmBody),
        actions: [
          TextButton(
            key: const Key('personalInfo.eraseCancel'),
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text(l10n.cancel),
          ),
          FilledButton(
            key: const Key('personalInfo.eraseConfirm'),
            style: FilledButton.styleFrom(
              backgroundColor: Theme.of(ctx).colorScheme.error,
              foregroundColor: Theme.of(ctx).colorScheme.onError,
            ),
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text(l10n.erase),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    setState(() {
      _busy = true;
      _failure = null;
    });
    final auth = ref.read(authControllerProvider.notifier);
    try {
      await ref.read(profileRepositoryProvider).erasePersonalInfo();
      if (!mounted) return;
      ref.invalidate(personalInfoProvider);
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(l10n.personalInfoErased)));
    } on UnauthenticatedFailure catch (f) {
      await _sessionLost(f, auth);
    } on AppFailure catch (f) {
      if (mounted) setState(() => _failure = f);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// Localised message for [field]. Object-level server errors (`address`,
  /// `national_id`) show on the first field of the group.
  String? _errorText(AppLocalizations l10n, String field) {
    final code =
        _errors[field] ??
        switch (field) {
          PiiField.line1 => _errors[PiiField.address],
          PiiField.nationalIdNumber => _errors[PiiField.nationalId],
          _ => null,
        };
    if (code == null) return null;
    return switch (code) {
      PiiErrorCode.required => l10n.fieldRequired,
      PiiErrorCode.tooLong => l10n.fieldTooLong,
      PiiErrorCode.invalidFormat => l10n.fieldInvalidFormat,
      PiiErrorCode.invalidCharacters => l10n.fieldInvalidCharacters,
      PiiErrorCode.outOfRange =>
        field == PiiField.dateOfBirth
            ? l10n.dateOfBirthOutOfRange
            : l10n.fieldOutOfRange,
      _ => l10n.validationFailed,
    };
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final info = ref.watch(personalInfoProvider);
    return PiiShield(
      secureScreen: ref.watch(secureScreenProvider),
      child: Scaffold(
        appBar: AppBar(
          title: Text(l10n.personalInfoTitle),
          leading: _editing
              ? IconButton(
                  key: const Key('personalInfo.cancel'),
                  tooltip: l10n.cancel,
                  icon: const Icon(Icons.close),
                  onPressed: _busy ? null : _closeEditor,
                )
              : null,
        ),
        body: SafeArea(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 480),
              child: switch (info) {
                _ when _editing => _form(l10n),
                AsyncData(:final value) => _view(l10n, value),
                AsyncError(:final error) => _loadError(l10n, error),
                _ => const Center(child: CircularProgressIndicator()),
              },
            ),
          ),
        ),
      ),
    );
  }

  Widget _loadError(AppLocalizations l10n, Object error) {
    final failure = error is AppFailure
        ? error
        : UnknownFailure(detail: error.runtimeType.toString());
    void retry() => ref.invalidate(personalInfoProvider);
    return ListView(
      padding: const EdgeInsets.all(24),
      children: [
        FailureBanner(
          key: const Key('personalInfo.loadError'),
          failure: failure,
          onRetry: retry,
        ),
        if (!failure.isRetryable && failure is! UnauthenticatedFailure) ...[
          const SizedBox(height: 12),
          FilledButton(onPressed: retry, child: Text(l10n.retry)),
        ],
      ],
    );
  }

  Widget _view(AppLocalizations l10n, PersonalInfo info) {
    final scheme = Theme.of(context).colorScheme;
    final dob = info.dateOfBirth;
    final a = info.address;
    final id = info.nationalId;
    String or(String? v) => v == null || v.isEmpty ? l10n.notProvided : v;
    return RefreshIndicator(
      onRefresh: () => ref.refresh(personalInfoProvider.future),
      child: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          FailureBanner(failure: _failure),
          ListTile(
            title: Text(l10n.phoneNumber),
            subtitle: Text(
              or(info.phoneNumber),
              key: const Key('personalInfo.view.phone'),
            ),
          ),
          ListTile(
            title: Text(l10n.dateOfBirth),
            subtitle: Text(
              dob == null
                  ? l10n.notProvided
                  : MaterialLocalizations.of(context).formatCompactDate(dob),
              key: const Key('personalInfo.view.dob'),
            ),
          ),
          ListTile(
            title: Text(l10n.addressSection),
            subtitle: Text(
              a == null
                  ? l10n.notProvided
                  : [
                      a.line1,
                      a.line2,
                      a.city,
                      a.region,
                      a.postalCode,
                      a.country,
                    ].whereType<String>().where((s) => s.isNotEmpty).join(', '),
              key: const Key('personalInfo.view.address'),
            ),
          ),
          ListTile(
            title: Text(l10n.nationalIdSection),
            subtitle: Text(
              id == null
                  ? l10n.notProvided
                  : '${_idTypeLabel(l10n, id.type)}: ${id.number}',
              key: const Key('personalInfo.view.nationalId'),
            ),
          ),
          const SizedBox(height: 12),
          FilledButton.icon(
            key: const Key('personalInfo.edit'),
            icon: const Icon(Icons.edit),
            label: Text(l10n.edit),
            onPressed: _busy ? null : () => _openEditor(info),
          ),
          const SizedBox(height: 12),
          OutlinedButton.icon(
            key: const Key('personalInfo.erase'),
            style: OutlinedButton.styleFrom(foregroundColor: scheme.error),
            icon: const Icon(Icons.delete_forever),
            label: Text(l10n.personalInfoErase),
            onPressed: _busy ? null : _erase,
          ),
        ],
      ),
    );
  }

  String _idTypeLabel(AppLocalizations l10n, String type) => switch (type) {
    NationalId.cccd => l10n.nationalIdTypeCccd,
    NationalId.passport => l10n.nationalIdTypePassport,
    NationalId.other => l10n.nationalIdTypeOther,
    _ => type,
  };

  Widget _text(
    AppLocalizations l10n, {
    required String field,
    required TextEditingController controller,
    required String label,
    int? maxLength,
    String? hint,
    TextInputType? keyboardType,
    TextCapitalization capitalization = TextCapitalization.none,
  }) => TextField(
    key: Key('personalInfo.$field'),
    controller: controller,
    maxLength: maxLength,
    keyboardType: keyboardType,
    textCapitalization: capitalization,
    // PII: no autocorrect/suggestion learning on the keyboard.
    autocorrect: false,
    enableSuggestions: false,
    decoration: InputDecoration(
      labelText: label,
      hintText: hint,
      errorText: _errorText(l10n, field),
      counterText: '',
    ),
  );

  Widget _form(AppLocalizations l10n) {
    final f = _failure;
    final children = <Widget>[
      FailureBanner(failure: f, onRetry: _save),
      if (f is ApiFailure && f.code == 'email_not_verified')
        Align(
          alignment: Alignment.centerRight,
          child: TextButton(
            key: const Key('personalInfo.verifyNow'),
            onPressed: () {
              ref.read(authControllerProvider.notifier).requireVerification();
              context.go(Routes.verifyEmail);
            },
            child: Text(l10n.verifyNow),
          ),
        ),
      _text(
        l10n,
        field: PiiField.phoneNumber,
        controller: _phone,
        label: l10n.phoneNumber,
        hint: l10n.phoneNumberHint,
        keyboardType: TextInputType.phone,
        maxLength: 32,
      ),
      TextField(
        key: const Key('personalInfo.date_of_birth'),
        controller: _dobText,
        readOnly: true,
        onTap: _pickDob,
        decoration: InputDecoration(
          labelText: l10n.dateOfBirth,
          errorText: _errorText(l10n, PiiField.dateOfBirth),
          suffixIcon: _dob == null
              ? const Icon(Icons.calendar_today)
              : IconButton(
                  key: const Key('personalInfo.dobClear'),
                  tooltip: l10n.clear,
                  icon: const Icon(Icons.clear),
                  onPressed: () => setState(() => _setDob(null)),
                ),
        ),
      ),
      Text(l10n.addressSection, style: Theme.of(context).textTheme.titleSmall),
      _text(
        l10n,
        field: PiiField.line1,
        controller: _line1,
        label: l10n.addressLine1,
        maxLength: 200,
      ),
      _text(
        l10n,
        field: PiiField.line2,
        controller: _line2,
        label: l10n.addressLine2,
        maxLength: 200,
      ),
      _text(
        l10n,
        field: PiiField.city,
        controller: _city,
        label: l10n.city,
        maxLength: 100,
      ),
      _text(
        l10n,
        field: PiiField.region,
        controller: _region,
        label: l10n.region,
        maxLength: 100,
      ),
      _text(
        l10n,
        field: PiiField.postalCode,
        controller: _postalCode,
        label: l10n.postalCode,
        maxLength: 20,
      ),
      _text(
        l10n,
        field: PiiField.country,
        controller: _country,
        label: l10n.country,
        maxLength: 2,
        capitalization: TextCapitalization.characters,
      ),
      Text(
        l10n.nationalIdSection,
        style: Theme.of(context).textTheme.titleSmall,
      ),
      DropdownButtonFormField<String?>(
        key: const Key('personalInfo.national_id.type'),
        initialValue: _idType,
        decoration: InputDecoration(
          labelText: l10n.nationalIdType,
          errorText: _errorText(l10n, PiiField.nationalIdType),
        ),
        items: [
          DropdownMenuItem(child: Text(l10n.nationalIdTypeNone)),
          for (final t in NationalId.types)
            DropdownMenuItem(value: t, child: Text(_idTypeLabel(l10n, t))),
          // Keep an unknown server type selectable rather than dropping it.
          if (_idType != null && !NationalId.types.contains(_idType))
            DropdownMenuItem(value: _idType, child: Text(_idType!)),
        ],
        onChanged: (v) => setState(() => _idType = v),
      ),
      _text(
        l10n,
        field: PiiField.nationalIdNumber,
        controller: _idNumber,
        label: l10n.nationalIdNumber,
        maxLength: 20,
        capitalization: TextCapitalization.characters,
      ),
      SubmitButton(
        key: const Key('personalInfo.save'),
        label: l10n.save,
        submitting: _busy,
        onPressed: _save,
      ),
    ];
    return ListView(
      padding: const EdgeInsets.all(24),
      children: [
        for (final c in children)
          Padding(padding: const EdgeInsets.only(bottom: 16), child: c),
      ],
    );
  }
}
