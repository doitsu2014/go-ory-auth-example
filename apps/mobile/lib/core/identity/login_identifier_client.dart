import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/network/interceptors.dart';
import 'package:go_ory_auth_mobile/core/network/problem_mapper.dart';

/// Resolves what the customer typed to the pseudonymous login identifier
/// that Kratos knows (ADR-0013, DD-18). Throws [AppFailure] only:
/// - `ApiFailure('validation_failed')` with `fieldErrors` on `value` /
///   `type` / `purpose` (422),
/// - `ApiFailure('rate_limited')` with `retryAfter` (429),
/// - `ApiFailure('dependency_unavailable')` (503, retryable),
/// - [NetworkFailure].
abstract interface class LoginIdentifierResolver {
  Future<String> resolve(LoginInput input, LoginPurpose purpose);
}

/// identity-service `POST /v1/auth/identifiers`. Public endpoint: the dio
/// instance must not carry the Bearer interceptor. The value is never logged
/// (the log interceptor writes method, path and status only).
class HttpLoginIdentifierResolver implements LoginIdentifierResolver {
  HttpLoginIdentifierResolver(this._dio);

  final Dio _dio;

  static const path = '/v1/auth/identifiers';

  @override
  Future<String> resolve(LoginInput input, LoginPurpose purpose) async {
    try {
      final res = await _dio.post<Map<String, dynamic>>(
        path,
        data: {
          'type': input.type.wire,
          'value': input.value,
          'purpose': purpose.wire,
        },
      );
      final id = res.data?['identifier'];
      if (id is! String || !loginPseudonymPattern.hasMatch(id)) {
        throw const UnknownFailure(detail: 'bad_identifier_response');
      }
      return id;
    } on DioException catch (e) {
      throw mapApiError(e);
    } on AppFailure {
      rethrow;
    } on Object catch (e) {
      throw UnknownFailure(detail: e.runtimeType.toString());
    }
  }
}

/// dio for the public identity-service endpoints: no `Authorization` header.
Dio buildPublicApiDio({required String baseUrl, required AppLogger logger}) {
  final dio = Dio(
    BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 15),
      headers: {'Accept': 'application/json, application/problem+json'},
    ),
  );
  dio.interceptors.addAll([
    RequestIdInterceptor(),
    RedactingLogInterceptor(logger),
  ]);
  return dio;
}
