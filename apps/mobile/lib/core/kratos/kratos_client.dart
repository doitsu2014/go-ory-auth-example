import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';

/// Kratos public API, **native (API) flows only** (ADR-0003). No browser
/// flows, no WebView.
///
/// Every method throws an `AppFailure` on error (see `kratos_error_mapper`):
/// `FlowValidationFailure` (400 + flow), `FlowExpiredFailure` (410),
/// `UnauthenticatedFailure` (401), `NetworkFailure`, `ApiFailure`.
///
/// Session tokens are passed per call and never stored here.
abstract interface class KratosClient {
  Future<KratosFlow> createRegistrationFlow();

  /// Traits are `{login_id: <pseudonym>}` only (ADR-0013): never the real
  /// email or phone number.
  Future<NativeAuthResult> submitRegistration({
    required String flowId,
    required String loginId,
    required String password,
  });

  Future<KratosFlow> createLoginFlow({
    bool refresh = false,
    String? sessionToken,
  });

  Future<NativeAuthResult> submitLogin({
    required String flowId,
    required String identifier,
    required String password,
    String? sessionToken,
  });

  Future<KratosFlow> createVerificationFlow();

  /// Sends either `email` (request / resend a code) or `code`. `email` is the
  /// pseudonymous login identifier: Kratos never sees the real address.
  Future<KratosFlow> submitVerification({
    required String flowId,
    String? email,
    String? code,
  });

  Future<KratosFlow> createRecoveryFlow();

  /// Sends either `email` (the pseudonymous login identifier, to request a
  /// code) or `code`. On a valid code Kratos
  /// returns the flow with `continue_with` [set_ory_session_token,
  /// show_settings_ui].
  Future<KratosFlow> submitRecovery({
    required String flowId,
    String? email,
    String? code,
  });

  Future<KratosFlow> createSettingsFlow({required String sessionToken});

  Future<KratosFlow> submitSettingsPassword({
    required String flowId,
    required String sessionToken,
    required String password,
  });

  Future<KratosSession> toSession({required String sessionToken});

  Future<void> logout({required String sessionToken});
}
