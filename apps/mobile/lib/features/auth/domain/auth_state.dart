import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';

/// Authentication state held by `AuthController`. The session token itself
/// is not part of the state (it lives only in `TokenStore`).
sealed class AuthState {
  const AuthState();
}

/// Startup: token not checked yet.
final class AuthUnknown extends AuthState {
  const AuthUnknown();
}

final class Unauthenticated extends AuthState {
  const Unauthenticated({this.startupFailure});

  /// Set when the stored session could not be checked (e.g. offline); the
  /// token is kept so the user can retry.
  final AppFailure? startupFailure;
}

final class Authenticated extends AuthState {
  const Authenticated(
    this.session, {
    this.pendingVerification = false,
    this.verificationFlowId,
  });

  final KratosSession session;

  /// True right after sign-up (or after `403 email_not_verified`): route to
  /// the verify-email screen.
  final bool pendingVerification;

  /// Flow id from `continue_with: show_verification_ui`, when Kratos sent it.
  final String? verificationFlowId;

  /// Pseudonymous Kratos identifier; for Kratos calls only, never shown.
  String get loginId => session.identity.loginId;
  bool get loginVerified => session.identity.loginVerified;

  Authenticated copyWith({
    KratosSession? session,
    bool? pendingVerification,
    String? verificationFlowId,
  }) => Authenticated(
    session ?? this.session,
    pendingVerification: pendingVerification ?? this.pendingVerification,
    verificationFlowId: verificationFlowId ?? this.verificationFlowId,
  );
}
