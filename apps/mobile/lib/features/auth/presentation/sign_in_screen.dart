import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/login_identifier_field.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// Sign-in with email / phone number and password: one call to
/// identity-service, which runs the Kratos native login flow with the
/// account's opaque handle (ADR-0014, PLX-FR-01). Rejections come back as
/// Kratos message ids on the `login`, `password` and `form` nodes.
class SignInScreen extends ConsumerStatefulWidget {
  const SignInScreen({super.key});

  @override
  ConsumerState<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends ConsumerState<SignInScreen>
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
    _login.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final login = LoginInput(type: _loginType, value: _login.text);
    final code = login.validate();
    setState(() => _loginCode = code);
    if (code != null) return;
    await runSubmit((_) async {
      final session = await ref
          .read(authRepositoryProvider)
          .login(login: login, password: _password.text);
      ref.read(authControllerProvider.notifier).signedIn(session);
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final auth = ref.watch(authControllerProvider);
    final startup = auth is Unauthenticated ? auth.startupFailure : null;
    return FormPage(
      title: l10n.signIn,
      children: [
        if (startup != null)
          FailureBanner(
            failure: startup,
            onRetry: () => ref.read(authControllerProvider.notifier).restore(),
          ),
        FailureBanner(failure: failure, onRetry: _submit),
        FlowMessages(messages: globalMessages(_bound)),
        LoginIdentifierField(
          keyPrefix: 'signIn',
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
          key: const Key('signIn.password'),
          controller: _password,
          obscureText: true,
          autofillHints: const [AutofillHints.password],
          onSubmitted: (_) => _submit(),
          decoration: InputDecoration(
            labelText: l10n.password,
            errorText: fieldError(context, AuthFlowFields.password),
          ),
        ),
        SubmitButton(
          key: const Key('signIn.submit'),
          label: l10n.signIn,
          submitting: submitting,
          onPressed: _submit,
        ),
        TextButton(
          onPressed: () => context.push(Routes.forgotPassword),
          child: Text(l10n.forgotPassword),
        ),
        TextButton(
          onPressed: () => context.go(Routes.signUp),
          child: Text(l10n.noAccount),
        ),
      ],
    );
  }
}
