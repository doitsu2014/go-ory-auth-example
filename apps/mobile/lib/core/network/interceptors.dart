import 'dart:async';
import 'dart:math';

import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';

/// Adds an `X-Request-Id` to every request so client and server logs can be
/// correlated.
class RequestIdInterceptor extends Interceptor {
  RequestIdInterceptor({Random? random}) : _random = random ?? Random.secure();

  static const header = 'X-Request-Id';
  final Random _random;

  String _next() => List.generate(
    16,
    (_) => _random.nextInt(256).toRadixString(16).padLeft(2, '0'),
  ).join();

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    options.headers.putIfAbsent(header, _next);
    handler.next(options);
  }
}

/// Logs method, path, status and duration only. Headers and bodies are never
/// logged; whatever is logged still goes through [AppLogger.redact].
class RedactingLogInterceptor extends Interceptor {
  RedactingLogInterceptor(this._logger);

  final AppLogger _logger;
  static const _startKey = 'log.start';

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    options.extra[_startKey] = DateTime.now();
    handler.next(options);
  }

  @override
  void onResponse(
    Response<dynamic> response,
    ResponseInterceptorHandler handler,
  ) {
    _logger.debug(_line(response.requestOptions, response.statusCode));
    handler.next(response);
  }

  @override
  void onError(DioException err, ErrorInterceptorHandler handler) {
    _logger.warn(
      '${_line(err.requestOptions, err.response?.statusCode)} ${err.type.name}',
    );
    handler.next(err);
  }

  String _line(RequestOptions o, int? status) {
    final start = o.extra[_startKey];
    final ms = start is DateTime
        ? DateTime.now().difference(start).inMilliseconds
        : -1;
    // Path only: query strings may carry flow ids / tokens.
    return '${o.method} ${o.uri.path} -> ${status ?? '-'} (${ms}ms) '
        'rid=${o.headers[RequestIdInterceptor.header] ?? '-'}';
  }
}

/// Adds `Authorization: Bearer <session_token>` for identity-service and
/// triggers a **single-flight** sign-out on `401`.
class BearerAuthInterceptor extends Interceptor {
  BearerAuthInterceptor({
    required this.readToken,
    required this.onUnauthorized,
  });

  final Future<String?> Function() readToken;
  final Future<void> Function() onUnauthorized;
  Future<void>? _inFlight;

  /// `options.extra` key holding the token a request was sent with (never
  /// logged; extras are not part of the log line).
  static const tokenKey = 'auth.token';

  @override
  Future<void> onRequest(
    RequestOptions options,
    RequestInterceptorHandler handler,
  ) async {
    final token = await readToken();
    if (token != null && token.isNotEmpty) {
      options.headers['Authorization'] = 'Bearer $token';
      options.extra[tokenKey] = token;
    }
    handler.next(options);
  }

  @override
  Future<void> onError(
    DioException err,
    ErrorInterceptorHandler handler,
  ) async {
    final sentWith = err.requestOptions.extra[tokenKey];
    // Only a 401 for the *current* token signs out: a late 401 for a request
    // sent with an older token must not wipe a newer session.
    if (err.response?.statusCode == 401 &&
        sentWith != null &&
        sentWith == await readToken()) {
      // Many concurrent requests may fail with 401 at once; sign out once.
      final pending = _inFlight ??= onUnauthorized().whenComplete(
        () => _inFlight = null,
      );
      await pending;
    }
    handler.next(err);
  }
}
