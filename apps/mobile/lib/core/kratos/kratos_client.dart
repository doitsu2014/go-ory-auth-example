import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';

/// Kratos public API, **native (API) flows only** (ADR-0003). No browser
/// flows, no WebView.
///
/// Flows that take what the customer typed (sign-in, registration, starting
/// recovery) go through identity-service instead (`CustomerAuthApi`,
/// ADR-0014). What stays here addresses Kratos with a flow id, a code, or
/// the signed-in owner's own handle (`traits.login_id`).
///
/// Every method throws an `AppFailure` on error (see `kratos_error_mapper`):
/// `FlowValidationFailure` (400 + flow), `FlowExpiredFailure` (410),
/// `UnauthenticatedFailure` (401), `NetworkFailure`, `ApiFailure`.
///
/// Session tokens are passed per call and never stored here.
abstract interface class KratosClient {
  /// Refresh login (settings) only: [submitLogin] then takes the session's
  /// own handle as `identifier`.
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
  /// signed-in owner's handle: Kratos never sees the real address.
  Future<KratosFlow> submitVerification({
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
