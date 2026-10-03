import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_messages.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

/// Shared lifecycle for hand-built screens bound to a Kratos native flow:
/// - `400` → re-render the **same** flow (messages come from Kratos),
/// - `410` → start a new flow and tell the user,
/// - `401` → the session is gone (revoked / disabled): wipe it and let the
///   router send the user to sign-in,
/// - other failures → banner (network errors offer retry).
mixin FlowFormMixin<W extends ConsumerStatefulWidget> on ConsumerState<W> {
  KratosFlow? flow;
  AppFailure? failure;
  bool submitting = false;

  /// Creates a fresh flow for this screen. Null when the screen is driven by
  /// an existing flow.
  Future<KratosFlow> Function()? get createFlow;

  Future<void> loadFlow() async {
    final create = createFlow;
    if (create == null) return;
    try {
      final f = await create();
      if (mounted) {
        setState(() {
          flow = f;
          // Keep the "form expired" notice visible after the restart.
          if (failure is! FlowExpiredFailure) failure = null;
        });
      }
    } on UnauthenticatedFailure catch (e) {
      await _sessionLost(e);
    } on AppFailure catch (e) {
      if (mounted) setState(() => failure = e);
    }
  }

  /// Kratos `401`: only meaningful while signed in (e.g. settings flow).
  Future<void> _sessionLost(UnauthenticatedFailure e) async {
    if (mounted) setState(() => failure = e);
    if (ref.read(authControllerProvider) is Authenticated) {
      await ref.read(authControllerProvider.notifier).handleUnauthorized();
    }
  }

  Future<void> runSubmit(Future<void> Function(KratosFlow? flow) action) async {
    if (submitting) return;
    setState(() {
      submitting = true;
      failure = null;
    });
    try {
      var current = flow;
      final create = createFlow;
      if (current == null && create != null) {
        current = await create();
        flow = current;
      }
      await action(current);
    } on FlowValidationFailure catch (e) {
      if (mounted) setState(() => flow = e.flow);
    } on FlowExpiredFailure catch (e) {
      if (mounted) {
        setState(() {
          failure = e;
          flow = null;
        });
      }
      await loadFlow();
    } on UnauthenticatedFailure catch (e) {
      await _sessionLost(e);
    } on AppFailure catch (e) {
      if (mounted) setState(() => failure = e);
    } finally {
      if (mounted) setState(() => submitting = false);
    }
  }

  /// Localised node messages for the input named [name] (errors first).
  String? fieldError(BuildContext context, String name) {
    final msgs = flow?.messagesFor(name) ?? const [];
    if (msgs.isEmpty) return null;
    final l10n = AppLocalizations.of(context);
    return msgs.map((m) => kratosMessage(l10n, m)).join('\n');
  }

  /// Global `ui.messages` plus node messages for nodes the screen does not
  /// render (never drop a Kratos message).
  List<UiText> globalMessages(Set<String> boundNames) {
    final f = flow;
    if (f == null) return const [];
    return [...f.messages, ...f.unboundMessages(boundNames)];
  }
}

/// Renders Kratos messages, coloured by type.
class FlowMessages extends StatelessWidget {
  const FlowMessages({required this.messages, super.key});

  final List<UiText> messages;

  @override
  Widget build(BuildContext context) {
    if (messages.isEmpty) return const SizedBox.shrink();
    final l10n = AppLocalizations.of(context);
    final scheme = Theme.of(context).colorScheme;
    return Column(
      key: const Key('flow.messages'),
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final m in messages)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Semantics(
              liveRegion: true,
              child: Text(
                kratosMessage(l10n, m),
                style: TextStyle(
                  color: m.isError
                      ? scheme.error
                      : (m.type == 'success'
                            ? Colors.green.shade700
                            : scheme.onSurface),
                ),
              ),
            ),
          ),
      ],
    );
  }
}

/// Non-flow failure (network, expired, API error) with optional retry.
class FailureBanner extends StatelessWidget {
  const FailureBanner({required this.failure, this.onRetry, super.key});

  final AppFailure? failure;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final f = failure;
    if (f == null) return const SizedBox.shrink();
    final l10n = AppLocalizations.of(context);
    final scheme = Theme.of(context).colorScheme;
    return Card(
      key: const Key('flow.failure'),
      color: scheme.errorContainer,
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          children: [
            Expanded(
              child: Semantics(
                liveRegion: true,
                child: Text(
                  failureMessage(l10n, f),
                  style: TextStyle(color: scheme.onErrorContainer),
                ),
              ),
            ),
            if (onRetry != null && f.isRetryable)
              TextButton(onPressed: onRetry, child: Text(l10n.retry)),
          ],
        ),
      ),
    );
  }
}

class SubmitButton extends StatelessWidget {
  const SubmitButton({
    required this.label,
    required this.submitting,
    required this.onPressed,
    super.key,
  });

  final String label;
  final bool submitting;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return FilledButton(
      onPressed: submitting ? null : onPressed,
      child: submitting
          ? const SizedBox.square(
              dimension: 20,
              child: CircularProgressIndicator(
                key: Key('flow.submitting'),
                strokeWidth: 2,
              ),
            )
          : Text(label),
    );
  }
}

/// Standard scrollable form scaffold.
class FormPage extends StatelessWidget {
  const FormPage({required this.title, required this.children, super.key});

  final String title;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(title)),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 480),
            child: ListView(
              padding: const EdgeInsets.all(24),
              children: [
                for (final c in children)
                  Padding(padding: const EdgeInsets.only(bottom: 16), child: c),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
