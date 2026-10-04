import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_messages.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

/// Security settings: change password (native settings flow with
/// `X-Session-Token`) and sign out (`performNativeLogout` + wipe).
///
/// When the privileged window (`privileged_session_max_age`, 15 min) has
/// passed, Kratos answers `403 session_refresh_required`: the screen asks for
/// the current password, runs a refresh login, then retries the change.
class SettingsScreen extends ConsumerStatefulWidget {
  const SettingsScreen({super.key});

  @override
  ConsumerState<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends ConsumerState<SettingsScreen>
    with FlowFormMixin {
  final _password = TextEditingController();
  final _current = TextEditingController();

  /// Set while a refresh login is needed; holds the new password to retry.
  String? _pendingPassword;

  /// Refresh-login flow returned with errors (e.g. wrong current password).
  KratosFlow? _reauthFlow;

  @override
  Future<KratosFlow> Function() get createFlow =>
      ref.read(settingsRepositoryProvider).startSettings;

  @override
  void dispose() {
    _password.dispose();
    _current.dispose();
    super.dispose();
  }

  Future<void> _submitPassword(KratosFlow flow, String password) async {
    try {
      final result = await ref
          .read(settingsRepositoryProvider)
          .changePassword(flowId: flow.id, password: password);
      _password.clear();
      // A settings flow is single-use after success.
      setState(() {
        this.flow = null;
        _pendingPassword = null;
      });
      if (!mounted) return;
      final l10n = AppLocalizations.of(context);
      final saved = result.messages.isNotEmpty ? result.messages.first : null;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          key: const Key('settings.saved'),
          content: Text(
            saved != null ? kratosMessage(l10n, saved) : l10n.passwordChanged,
          ),
        ),
      );
    } on ApiFailure catch (f) {
      if (f.code != 'session_refresh_required') rethrow;
      setState(() {
        _pendingPassword = password;
        _reauthFlow = null;
      });
    }
  }

  Future<void> _changePassword() =>
      runSubmit((flow) => _submitPassword(flow!, _password.text));

  Future<void> _reauthenticate() => runSubmit((_) async {
    final auth = ref.read(authControllerProvider);
    // PLI-FR-09: the session's login_id is already the pseudonym; the
    // customer does not re-type the email / phone and nothing is resolved.
    final loginId = auth is Authenticated ? auth.loginId : '';
    final settings = ref.read(settingsRepositoryProvider);
    try {
      await settings.reauthenticate(
        identifier: loginId,
        password: _current.text,
      );
    } on FlowValidationFailure catch (e) {
      // Keep the settings flow; show the login flow's messages here.
      setState(() => _reauthFlow = e.flow);
      return;
    }
    _current.clear();
    final pending = _pendingPassword ?? _password.text;
    setState(() {
      _pendingPassword = null;
      _reauthFlow = null;
    });
    // The old settings flow was created before the refresh; start a new one.
    final fresh = await settings.startSettings();
    flow = fresh;
    await _submitPassword(fresh, pending);
  });

  String? _reauthError(AppLocalizations l10n) {
    final f = _reauthFlow;
    if (f == null) return null;
    final msgs = [...f.messages, ...f.nodes.expand((n) => n.messages)];
    if (msgs.isEmpty) return null;
    return msgs.map((m) => kratosMessage(l10n, m)).join('\n');
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final needsReauth = _pendingPassword != null;
    return FormPage(
      title: l10n.settingsTitle,
      children: [
        Text(
          l10n.changePassword,
          style: Theme.of(context).textTheme.titleMedium,
        ),
        FailureBanner(failure: failure),
        FlowMessages(messages: flow?.messages ?? const []),
        if (needsReauth) ...[
          Text(l10n.reauthBody, key: const Key('settings.reauthBody')),
          TextField(
            key: const Key('settings.currentPassword'),
            controller: _current,
            obscureText: true,
            autofillHints: const [AutofillHints.password],
            decoration: InputDecoration(
              labelText: l10n.currentPassword,
              errorText: _reauthError(l10n),
            ),
          ),
          SubmitButton(
            key: const Key('settings.reauth'),
            label: l10n.confirm,
            submitting: submitting,
            onPressed: _reauthenticate,
          ),
        ] else ...[
          TextField(
            key: const Key('settings.password'),
            controller: _password,
            obscureText: true,
            autofillHints: const [AutofillHints.newPassword],
            decoration: InputDecoration(
              labelText: l10n.newPassword,
              errorText: fieldError(context, 'password'),
            ),
          ),
          SubmitButton(
            key: const Key('settings.changePassword'),
            label: l10n.changePassword,
            submitting: submitting,
            onPressed: _changePassword,
          ),
        ],
        const Divider(height: 32),
        OutlinedButton.icon(
          key: const Key('settings.signOut'),
          icon: const Icon(Icons.logout),
          label: Text(l10n.signOut),
          onPressed: () => ref.read(authControllerProvider.notifier).signOut(),
        ),
      ],
    );
  }
}
