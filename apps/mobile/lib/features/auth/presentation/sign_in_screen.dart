import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// Native login flow: `createNativeLoginFlow` → `{method: password,
/// identifier, password}`.
class SignInScreen extends ConsumerStatefulWidget {
  const SignInScreen({super.key});

  @override
  ConsumerState<SignInScreen> createState() => _SignInScreenState();
}

class _SignInScreenState extends ConsumerState<SignInScreen>
    with FlowFormMixin {
  static const _bound = {'identifier', 'password', 'csrf_token', 'method'};
  final _identifier = TextEditingController();
  final _password = TextEditingController();

  @override
  Future<KratosFlow> Function() get createFlow =>
      ref.read(authRepositoryProvider).startLogin;

  @override
  void initState() {
    super.initState();
    unawaited(loadFlow());
  }

  @override
  void dispose() {
    _identifier.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit() => runSubmit((flow) async {
    final session = await ref
        .read(authRepositoryProvider)
        .login(
          flowId: flow!.id,
          identifier: _identifier.text.trim(),
          password: _password.text,
        );
    ref.read(authControllerProvider.notifier).signedIn(session);
  });

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
        FailureBanner(failure: failure, onRetry: loadFlow),
        FlowMessages(messages: globalMessages(_bound)),
        TextField(
          key: const Key('signIn.identifier'),
          controller: _identifier,
          keyboardType: TextInputType.emailAddress,
          autofillHints: const [AutofillHints.email],
          textInputAction: TextInputAction.next,
          decoration: InputDecoration(
            labelText: l10n.email,
            errorText: fieldError(context, 'identifier'),
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
            errorText: fieldError(context, 'password'),
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
