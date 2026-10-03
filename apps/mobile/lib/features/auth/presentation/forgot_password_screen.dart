import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

enum RecoveryStep { email, code, newPassword }

/// Native recovery flow (code): email → code → privileged settings flow
/// (`continue_with`) → new password → signed in.
class ForgotPasswordScreen extends ConsumerStatefulWidget {
  const ForgotPasswordScreen({super.key});

  @override
  ConsumerState<ForgotPasswordScreen> createState() =>
      _ForgotPasswordScreenState();
}

class _ForgotPasswordScreenState extends ConsumerState<ForgotPasswordScreen>
    with FlowFormMixin {
  static const _bound = {'email', 'code', 'password', 'csrf_token', 'method'};
  final _email = TextEditingController();
  final _code = TextEditingController();
  final _password = TextEditingController();
  RecoveryStep _step = RecoveryStep.email;
  RecoveryGrant? _grant;
  bool _completed = false;
  late final AuthRepository _repo;

  @override
  Future<KratosFlow> Function() get createFlow => () {
    // A new recovery flow always restarts at the email step.
    if (mounted && _step != RecoveryStep.email) {
      setState(() => _step = RecoveryStep.email);
    }
    return ref.read(authRepositoryProvider).startRecovery();
  };

  @override
  void initState() {
    super.initState();
    _repo = ref.read(authRepositoryProvider);
    unawaited(loadFlow());
  }

  @override
  void dispose() {
    final grant = _grant;
    if (grant != null && !_completed) unawaited(_repo.abandonRecovery(grant));
    for (final c in [_email, _code, _password]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _sendCode() => runSubmit((flow) async {
    final result = await ref
        .read(authRepositoryProvider)
        .requestRecoveryCode(flowId: flow!.id, email: _email.text.trim());
    setState(() {
      this.flow = result;
      _step = RecoveryStep.code;
    });
  });

  Future<void> _submitCode() => runSubmit((flow) async {
    final grant = await ref
        .read(authRepositoryProvider)
        .submitRecoveryCode(flowId: flow!.id, code: _code.text.trim());
    setState(() {
      _grant = grant;
      this.flow = KratosFlow(id: grant.settingsFlowId);
      _step = RecoveryStep.newPassword;
    });
  });

  Future<void> _savePassword() => runSubmit((_) async {
    final session = await ref
        .read(authRepositoryProvider)
        .completeRecovery(grant: _grant!, password: _password.text);
    _completed = true;
    ref.read(authControllerProvider.notifier).signedIn(session);
  });

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final step = switch (_step) {
      RecoveryStep.email => [
        Text(l10n.recoveryEmailBody),
        TextField(
          key: const Key('recovery.email'),
          controller: _email,
          keyboardType: TextInputType.emailAddress,
          autofillHints: const [AutofillHints.email],
          decoration: InputDecoration(
            labelText: l10n.email,
            errorText: fieldError(context, 'email'),
          ),
        ),
        SubmitButton(
          key: const Key('recovery.send'),
          label: l10n.sendCode,
          submitting: submitting,
          onPressed: _sendCode,
        ),
      ],
      RecoveryStep.code => [
        Text(l10n.recoveryCodeBody),
        TextField(
          key: const Key('recovery.code'),
          controller: _code,
          keyboardType: TextInputType.number,
          autofillHints: const [AutofillHints.oneTimeCode],
          decoration: InputDecoration(
            labelText: l10n.recoveryCode,
            errorText: fieldError(context, 'code'),
          ),
        ),
        SubmitButton(
          key: const Key('recovery.submitCode'),
          label: l10n.continueAction,
          submitting: submitting,
          onPressed: _submitCode,
        ),
      ],
      RecoveryStep.newPassword => [
        Text(l10n.setNewPasswordBody),
        TextField(
          key: const Key('recovery.password'),
          controller: _password,
          obscureText: true,
          autofillHints: const [AutofillHints.newPassword],
          decoration: InputDecoration(
            labelText: l10n.newPassword,
            errorText: fieldError(context, 'password'),
          ),
        ),
        SubmitButton(
          key: const Key('recovery.savePassword'),
          label: l10n.savePassword,
          submitting: submitting,
          onPressed: _savePassword,
        ),
      ],
    };
    return FormPage(
      title: l10n.recoveryTitle,
      children: [
        FailureBanner(failure: failure, onRetry: loadFlow),
        FlowMessages(messages: globalMessages(_bound)),
        ...step,
      ],
    );
  }
}
