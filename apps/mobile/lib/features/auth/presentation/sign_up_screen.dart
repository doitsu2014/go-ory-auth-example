import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/login_identifier_field.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// Registration with email / phone number and password: one call to
/// identity-service, which stores the address encrypted under a new opaque
/// handle and runs the Kratos native registration flow with it (ADR-0014,
/// PLX-FR-02). The name is not asked here: it is personal info, entered on
/// the personal-info screen after verification. On success the session
/// token is stored and the router goes to the verify screen.
class SignUpScreen extends ConsumerStatefulWidget {
  const SignUpScreen({super.key});

  @override
  ConsumerState<SignUpScreen> createState() => _SignUpScreenState();
}

class _SignUpScreenState extends ConsumerState<SignUpScreen>
    with FlowFormMixin {
  static const Set<String> _bound = {
    AuthFlowFields.login,
    AuthFlowFields.password,
  };
  final _login = TextEditingController();
  final _password = TextEditingController();
  LoginType _loginType = LoginType.email;

  /// Client-side format check result (UX only).
  String? _loginCode;

  /// No Kratos flow on the device: identity-service creates it.
  @override
  Future<KratosFlow> Function()? get createFlow => null;

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
    await runSubmit((_) async {
      final outcome = await ref
          .read(authRepositoryProvider)
          .register(login: login, password: _password.text);
      ref.read(authControllerProvider.notifier).registered(outcome);
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return FormPage(
      title: l10n.signUp,
      children: [
        FailureBanner(failure: failure, onRetry: _submit),
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
            kratosError: fieldError(context, AuthFlowFields.login),
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
            // only sees the handle, so it cannot check this itself).
            helperText: l10n.passwordHint,
            helperMaxLines: 2,
            errorText: fieldError(context, AuthFlowFields.password),
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
