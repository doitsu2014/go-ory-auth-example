import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/widgets/flow_form.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/me.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/profile_screen.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// Login identifier verification by code (email or SMS). Uses the flow from
/// `continue_with: show_verification_ui` when Kratos sent one; otherwise
/// starts a native verification flow, which sends a fresh code.
///
/// Kratos is addressed with the session's pseudonym (`loginId`); the contact
/// shown to the user comes from `GET /v1/me` (`login.value`). The pseudonym
/// is never displayed (PLI-FR-08).
class VerifyEmailScreen extends ConsumerStatefulWidget {
  const VerifyEmailScreen({super.key});

  @override
  ConsumerState<VerifyEmailScreen> createState() => _VerifyEmailScreenState();
}

class _VerifyEmailScreenState extends ConsumerState<VerifyEmailScreen>
    with FlowFormMixin {
  static const _bound = {'code', 'email', 'csrf_token', 'method'};
  final _code = TextEditingController();
  String? _providedFlowId;

  /// The session's pseudonym. Without a session (or a session without a
  /// login id) Kratos must not be called with an empty identifier: this is
  /// treated as a lost session ([UnauthenticatedFailure] → wipe → sign-in).
  LoginTarget get _target {
    final s = ref.read(authControllerProvider);
    final loginId = s is Authenticated ? s.loginId : '';
    if (loginId.isEmpty) throw const UnauthenticatedFailure();
    return PseudonymousLogin(loginId);
  }

  @override
  Future<KratosFlow> Function() get createFlow => () {
    final provided = _providedFlowId;
    if (provided != null) {
      _providedFlowId = null;
      return Future.value(KratosFlow(id: provided, state: 'sent_email'));
    }
    return ref.read(authRepositoryProvider).startVerification(_target);
  };

  @override
  void initState() {
    super.initState();
    final s = ref.read(authControllerProvider);
    _providedFlowId = s is Authenticated ? s.verificationFlowId : null;
    unawaited(loadFlow());
  }

  @override
  void dispose() {
    _code.dispose();
    super.dispose();
  }

  Future<void> _verify() => runSubmit((flow) async {
    final result = await ref
        .read(authRepositoryProvider)
        .verify(flowId: flow!.id, code: _code.text.trim());
    setState(() => this.flow = result);
    await ref.read(authControllerProvider.notifier).verificationCompleted();
    if (mounted) context.go(Routes.profile);
  });

  Future<void> _resend() => runSubmit((flow) async {
    final result = await ref
        .read(authRepositoryProvider)
        .resendVerificationCode(flowId: flow!.id, target: _target);
    setState(() => this.flow = result);
  });

  static String _body(AppLocalizations l10n, LoginContact? contact) {
    if (contact == null || contact.value.isEmpty) return l10n.verifyBodyGeneric;
    return switch (contact.type) {
      LoginType.email => l10n.verifyEmailBody(contact.value),
      LoginType.phone => l10n.verifyPhoneBody(contact.value),
    };
  }

  void _later() {
    ref.read(authControllerProvider.notifier).skipVerification();
    context.go(Routes.profile);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final contact = ref.watch(meProvider).value?.login;
    return FormPage(
      title: l10n.verifyEmailTitle,
      children: [
        Text(_body(l10n, contact), key: const Key('verify.body')),
        FailureBanner(failure: failure, onRetry: loadFlow),
        FlowMessages(messages: globalMessages(_bound)),
        TextField(
          key: const Key('verify.code'),
          controller: _code,
          keyboardType: TextInputType.number,
          autofillHints: const [AutofillHints.oneTimeCode],
          onSubmitted: (_) => _verify(),
          decoration: InputDecoration(
            labelText: l10n.verificationCode,
            errorText: fieldError(context, 'code'),
          ),
        ),
        SubmitButton(
          key: const Key('verify.submit'),
          label: l10n.verify,
          submitting: submitting,
          onPressed: _verify,
        ),
        OutlinedButton(
          key: const Key('verify.resend'),
          onPressed: submitting ? null : _resend,
          child: Text(l10n.resendCode),
        ),
        TextButton(onPressed: _later, child: Text(l10n.skipForNow)),
      ],
    );
  }
}
