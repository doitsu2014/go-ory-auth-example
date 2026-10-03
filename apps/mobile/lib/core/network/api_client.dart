import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';
import 'package:go_ory_auth_mobile/core/network/interceptors.dart';

/// dio instance for identity-service `/v1/*` (Bearer session token only).
Dio buildApiDio({
  required String baseUrl,
  required Future<String?> Function() readToken,
  required Future<void> Function() onUnauthorized,
  required AppLogger logger,
}) {
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
    BearerAuthInterceptor(readToken: readToken, onUnauthorized: onUnauthorized),
    RedactingLogInterceptor(logger),
  ]);
  return dio;
}
