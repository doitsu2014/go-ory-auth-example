import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';

/// Maps a failed Kratos public API call to a typed [AppFailure]
/// (architecture §3.9).
AppFailure mapKratosError(Object error) {
  if (error is AppFailure) return error;
  if (error is! DioException) {
    return UnknownFailure(detail: error.runtimeType.toString());
  }
  switch (error.type) {
    case DioExceptionType.connectionTimeout:
    case DioExceptionType.sendTimeout:
    case DioExceptionType.receiveTimeout:
    case DioExceptionType.transformTimeout:
    case DioExceptionType.connectionError:
      return NetworkFailure(detail: error.type.name);
    case DioExceptionType.badCertificate:
      return const NetworkFailure(detail: 'bad_certificate');
    case DioExceptionType.cancel:
      return const UnknownFailure(detail: 'cancelled');
    case DioExceptionType.badResponse:
    case DioExceptionType.unknown:
      break;
  }
  final response = error.response;
  if (response == null) {
    return NetworkFailure(detail: error.type.name);
  }
  return mapKratosResponse(response.statusCode ?? 0, response.data);
}

/// Maps a non-2xx Kratos response body.
AppFailure mapKratosResponse(int status, Object? data) {
  final body = data is Map
      ? data.cast<String, dynamic>()
      : const <String, dynamic>{};
  final err = body['error'] is Map
      ? (body['error'] as Map).cast<String, dynamic>()
      : const <String, dynamic>{};
  final errorId = err['id'] as String?;
  final reason = err['reason'] as String? ?? err['message'] as String?;

  if (status == 400 && body['ui'] is Map) {
    return FlowValidationFailure(KratosFlow.fromJson(body));
  }
  if (status == 410 ||
      status == 404 ||
      errorId == 'self_service_flow_expired') {
    return FlowExpiredFailure(detail: errorId ?? reason);
  }
  if (status == 401) {
    return UnauthenticatedFailure(detail: errorId);
  }
  if (status == 422 && errorId == 'browser_location_change_required') {
    return ApiFailure(
      errorId!,
      status: status,
      detail: body['redirect_browser_to'] as String?,
    );
  }
  return ApiFailure(
    errorId ?? (status >= 500 ? 'server_error' : 'bad_request'),
    status: status,
    detail: reason,
  );
}
