import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/login_identifier_field.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

enum RecoveryStep { email, code, newPassword }

/// Recovery by code: email / phone → identity-service starts the Kratos
/// native recovery flow with the account's handle (or a decoy, same answer,
/// ADR-0014) → code → privileged settings flow (`continue_with`) → new
/// password → signed in. The code and the new password go to Kratos
/// directly.
class ForgotPasswordScreen extends ConsumerStatefulWidget {
  const ForgotPasswordScreen({super.key});

  @override
  ConsumerState<ForgotPasswordScreen> createState() =>
      _ForgotPasswordScreenState();
}

class _ForgotPasswordScreenState extends ConsumerState<ForgotPasswordScreen>
    with FlowFormMixin {
  static const Set<String> _bound = {
    AuthFlowFields.login,
    'code',
    'password',
    'csrf_token',
    'method',
  };
  final _login = TextEditingController();
  final _code = TextEditingController();
  LoginType _loginType = LoginType.email;

  /// Client-side format check result (UX only).
  String? _loginCode;
  final _password = TextEditingController();
  RecoveryStep _step = RecoveryStep.email;
  RecoveryGrant? _grant;
  bool _completed = false;
  late final AuthRepository _repo;

  /// The flow is started by identity-service at the email step; an expired
  /// flow (code step) restarts there (see [build]).
  @override
  Future<KratosFlow> Function()? get createFlow => null;

  @override
  void initState() {
    super.initState();
    _repo = ref.read(authRepositoryProvider);
  }

  @override
  void dispose() {
    final grant = _grant;
    if (grant != null && !_completed) unawaited(_repo.abandonRecovery(grant));
    for (final c in [_login, _code, _password]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _sendCode() async {
    final login = LoginInput(type: _loginType, value: _login.text);
    final code = login.validate();
    setState(() => _loginCode = code);
    if (code != null) return;
    await runSubmit((_) async {
      final result = await ref
          .read(authRepositoryProvider)
          .requestRecoveryCode(login: login);
      setState(() {
        flow = result;
        _step = RecoveryStep.code;
      });
    });
  }

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
    // No flow before the email step, or after the recovery flow expired:
    // start again with the email / phone number.
    final current = flow == null && _step == RecoveryStep.code
        ? RecoveryStep.email
        : _step;
    final step = switch (current) {
      RecoveryStep.email => [
        Text(l10n.recoveryEmailBody),
        LoginIdentifierField(
          keyPrefix: 'recovery',
          controller: _login,
          type: _loginType,
          onTypeChanged: (t) => setState(() {
            _loginType = t;
            _loginCode = null;
          }),
          onChanged: (_) {
            // The format error described the previous value.
            if (_loginCode != null) setState(() => _loginCode = null);
          },
          errorText: loginErrorText(
            l10n,
            _loginType,
            localCode: _loginCode,
            failure: failure,
            kratosError: fieldError(context, AuthFlowFields.login),
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
        FailureBanner(failure: failure),
        FlowMessages(messages: globalMessages(_bound)),
        ...step,
      ],
    );
  }
}
