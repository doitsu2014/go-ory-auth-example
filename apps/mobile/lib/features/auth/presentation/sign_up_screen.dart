import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/login_identifier_field.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// Native registration flow: the email / phone number is resolved to its
/// pseudonym first, then `{method: password, traits{login_id}, password}`
/// (ADR-0013, PLI-FR-08). The name is not asked here: it is personal info,
/// entered on the personal-info screen after verification. On success the
/// session token is stored and the router follows `continue_with` to the
/// verify screen.
class SignUpScreen extends ConsumerStatefulWidget {
  const SignUpScreen({super.key});

  @override
  ConsumerState<SignUpScreen> createState() => _SignUpScreenState();
}

class _SignUpScreenState extends ConsumerState<SignUpScreen>
    with FlowFormMixin {
  static const _bound = {
    'traits.login_id',
    'traits.email',
    'password',
    'csrf_token',
    'method',
  };
  final _login = TextEditingController();
  final _password = TextEditingController();
  LoginType _loginType = LoginType.email;

  /// Client-side format check result (UX only).
  String? _loginCode;

  @override
  Future<KratosFlow> Function() get createFlow =>
      ref.read(authRepositoryProvider).startRegistration;

  @override
  void initState() {
    super.initState();
    unawaited(loadFlow());
  }

  @override
  void dispose() {
    for (final c in [_login, _password]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _submit() async {
    final login = LoginInput(type: _loginType, value: _login.text);
    final code = login.validate();
    setState(() => _loginCode = code);
    if (code != null) return;
    await runSubmit((flow) async {
      final outcome = await ref
          .read(authRepositoryProvider)
          .register(flowId: flow!.id, login: login, password: _password.text);
      ref.read(authControllerProvider.notifier).registered(outcome);
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return FormPage(
      title: l10n.signUp,
      children: [
        FailureBanner(failure: failure, onRetry: loadFlow),
        FlowMessages(messages: globalMessages(_bound)),
        LoginIdentifierField(
          keyPrefix: 'signUp',
          controller: _login,
          type: _loginType,
          textInputAction: TextInputAction.next,
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
            kratosError:
                fieldError(context, 'traits.login_id') ??
                fieldError(context, 'traits.email'),
          ),
        ),
        TextField(
          key: const Key('signUp.password'),
          controller: _password,
          obscureText: true,
          autofillHints: const [AutofillHints.newPassword],
          decoration: InputDecoration(
            labelText: l10n.password,
            // R-06: the password must not contain the email / phone (Kratos
            // only sees the pseudonym, so it cannot check this itself).
            helperText: l10n.passwordHint,
            helperMaxLines: 2,
            errorText: fieldError(context, 'password'),
          ),
        ),
        SubmitButton(
          key: const Key('signUp.submit'),
          label: l10n.signUp,
          submitting: submitting,
          onPressed: _submit,
        ),
        TextButton(
          onPressed: () => context.go(Routes.signIn),
          child: Text(l10n.haveAccount),
        ),
      ],
    );
  }
}
