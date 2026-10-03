import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';

/// Typed failure surfaced by every repository. The UI never sees a
/// `DioException`.
sealed class AppFailure implements Exception {
  const AppFailure(this.code, {this.status, this.detail});

  /// Stable machine code (identity-service problem `code`, Kratos error `id`,
  /// or a client-side code such as `network`).
  final String code;

  /// HTTP status when the failure came from a response.
  final int? status;

  /// Developer-facing detail; never shown verbatim when a localised message
  /// exists, never contains credentials.
  final String? detail;

  /// Transient: the UI offers a retry (no connectivity, or identity-service
  /// `503 dependency_unavailable`).
  bool get isRetryable =>
      this is NetworkFailure || code == 'dependency_unavailable';

  @override
  String toString() => 'AppFailure(code: $code, status: $status)';
}

/// No connectivity / timeout / DNS. UI offers a retry.
final class NetworkFailure extends AppFailure {
  const NetworkFailure({super.detail}) : super('network');
}

/// No valid session (Kratos 401 or identity-service `unauthenticated`).
final class UnauthenticatedFailure extends AppFailure {
  const UnauthenticatedFailure({super.detail})
    : super('unauthenticated', status: 401);
}

/// Kratos answered `400` (or `200` with error messages for code methods)
/// with the flow: re-render the **same** flow with its messages.
final class FlowValidationFailure extends AppFailure {
  const FlowValidationFailure(this.flow)
    : super('flow_validation', status: 400);

  final KratosFlow flow;
}

/// Kratos answered `410` (flow expired) or the flow id is unknown: start a new
/// flow.
final class FlowExpiredFailure extends AppFailure {
  const FlowExpiredFailure({super.detail})
    : super('self_service_flow_expired', status: 410);
}

/// Error from an API: identity-service problem+json (`code`) or a Kratos
/// generic error (`error.id`), e.g. `email_not_verified`,
/// `session_refresh_required`, `session_aal2_required`.
final class ApiFailure extends AppFailure {
  const ApiFailure(
    super.code, {
    super.status,
    super.detail,
    this.fieldErrors = const [],
  });

  final List<FieldError> fieldErrors;
}

/// Anything unexpected (bad payload, programming error).
final class UnknownFailure extends AppFailure {
  const UnknownFailure({super.detail}) : super('unknown');
}

/// identity-service `errors[]` entry.
class FieldError {
  const FieldError({required this.field, required this.code});

  final String field;
  final String code;
}
