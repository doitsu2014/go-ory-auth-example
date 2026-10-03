import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// Native registration flow: `{method: password, traits{email}, password}`.
/// The name is not asked here: it is personal info, entered on the
/// personal-info screen after email verification. On success the session
/// token is stored and the router follows `continue_with` to the
/// verify-email screen.
class SignUpScreen extends ConsumerStatefulWidget {
  const SignUpScreen({super.key});

  @override
  ConsumerState<SignUpScreen> createState() => _SignUpScreenState();
}

class _SignUpScreenState extends ConsumerState<SignUpScreen>
    with FlowFormMixin {
  static const _bound = {'traits.email', 'password', 'csrf_token', 'method'};
  final _email = TextEditingController();
  final _password = TextEditingController();

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
    for (final c in [_email, _password]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _submit() => runSubmit((flow) async {
    final outcome = await ref
        .read(authRepositoryProvider)
        .register(
          flowId: flow!.id,
          email: _email.text.trim(),
          password: _password.text,
        );
    ref.read(authControllerProvider.notifier).registered(outcome);
  });

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return FormPage(
      title: l10n.signUp,
      children: [
        FailureBanner(failure: failure, onRetry: loadFlow),
        FlowMessages(messages: globalMessages(_bound)),
        TextField(
          key: const Key('signUp.email'),
          controller: _email,
          keyboardType: TextInputType.emailAddress,
          autofillHints: const [AutofillHints.email],
          decoration: InputDecoration(
            labelText: l10n.email,
            errorText: fieldError(context, 'traits.email'),
          ),
        ),
        TextField(
          key: const Key('signUp.password'),
          controller: _password,
          obscureText: true,
          autofillHints: const [AutofillHints.newPassword],
          decoration: InputDecoration(
            labelText: l10n.password,
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
