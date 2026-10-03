import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';

/// Maps a failed identity-service call (RFC 9457 `application/problem+json`
/// with a stable `code`) to a typed [AppFailure]. Unknown codes are kept
/// verbatim, as the contract requires clients to tolerate them.
AppFailure mapApiError(Object error) {
  if (error is AppFailure) return error;
  if (error is! DioException) {
    return UnknownFailure(detail: error.runtimeType.toString());
  }
  final response = error.response;
  if (response == null) {
    switch (error.type) {
      case DioExceptionType.cancel:
        return const UnknownFailure(detail: 'cancelled');
      case DioExceptionType.connectionTimeout:
      case DioExceptionType.sendTimeout:
      case DioExceptionType.receiveTimeout:
      case DioExceptionType.transformTimeout:
      case DioExceptionType.connectionError:
      case DioExceptionType.badCertificate:
      case DioExceptionType.badResponse:
      case DioExceptionType.unknown:
        return NetworkFailure(detail: error.type.name);
    }
  }
  return mapProblem(response.statusCode ?? 0, response.data);
}

AppFailure mapProblem(int status, Object? data) {
  final body = data is Map
      ? data.cast<String, dynamic>()
      : const <String, dynamic>{};
  final code = body['code'] as String? ?? _fallbackCode(status);
  if (status == 401 || code == 'unauthenticated') {
    return UnauthenticatedFailure(detail: body['detail'] as String?);
  }
  final errors = body['errors'] is List
      ? (body['errors'] as List)
            .whereType<Map<dynamic, dynamic>>()
            .map(
              (e) => FieldError(
                field: e['field']?.toString() ?? '',
                code: e['code']?.toString() ?? '',
              ),
            )
            .toList()
      : const <FieldError>[];
  return ApiFailure(
    code,
    status: status,
    detail: body['detail'] as String? ?? body['title'] as String?,
    fieldErrors: errors,
  );
}

String _fallbackCode(int status) => switch (status) {
  401 => 'unauthenticated',
  403 => 'forbidden',
  404 => 'not_found',
  409 => 'conflict',
  422 => 'validation_failed',
  429 => 'rate_limited',
  503 => 'dependency_unavailable',
  _ => status >= 500 ? 'internal' : 'unknown',
};
