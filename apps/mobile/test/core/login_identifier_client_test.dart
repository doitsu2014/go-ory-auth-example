import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/identity/login_identifier_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';

import '../helpers/helpers.dart';

/// Captures the request and answers it with [status] / [body] / [headers]
/// without touching the network.
class _Stub extends Interceptor {
  _Stub(this.status, this.body, {this.headers = const {}});

  final int status;
  final Object? body;
  final Map<String, List<String>> headers;
  RequestOptions? request;

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    request = options;
    final response = Response<Object?>(
      requestOptions: options,
      statusCode: status,
      data: body,
      headers: Headers.fromMap(headers),
    );
    if (status >= 200 && status < 300) {
      handler.resolve(response);
    } else {
      handler.reject(
        DioException.badResponse(
          statusCode: status,
          requestOptions: options,
          response: response,
        ),
      );
    }
  }
}

void main() {
  const phone = LoginInput(type: LoginType.phone, value: '0901 234 567');
  const email = LoginInput(type: LoginType.email, value: 'An@Example.com');

  (HttpLoginIdentifierResolver, _Stub) build(
    int status,
    Object? body, {
    Map<String, List<String>> headers = const {},
  }) {
    final stub = _Stub(status, body, headers: headers);
    final dio = buildPublicApiDio(
      baseUrl: 'http://api.invalid',
      logger: AppLogger(),
    )..interceptors.add(stub);
    return (HttpLoginIdentifierResolver(dio), stub);
  }

  test('POST /v1/auth/identifiers {type, value as typed, purpose}; no '
      'Authorization header', () async {
    final (client, stub) = build(200, {'identifier': pseudonym});
    expect(await client.resolve(phone, LoginPurpose.signIn), pseudonym);
    final req = stub.request!;
    expect(req.method, 'POST');
    expect(req.path, '/v1/auth/identifiers');
    expect(req.data, {
      'type': 'phone',
      'value': '0901 234 567',
      'purpose': 'sign_in',
    });
    expect(req.headers.containsKey('Authorization'), isFalse);
  });

  test('purpose wire names', () async {
    for (final (purpose, wire) in [
      (LoginPurpose.registration, 'registration'),
      (LoginPurpose.signIn, 'sign_in'),
      (LoginPurpose.recovery, 'recovery'),
      (LoginPurpose.verification, 'verification'),
    ]) {
      final (client, stub) = build(200, {'identifier': pseudonym});
      await client.resolve(email, purpose);
      expect((stub.request!.data as Map)['purpose'], wire);
      expect((stub.request!.data as Map)['type'], 'email');
    }
  });

  test('422 -> validation_failed with field errors', () async {
    final (client, _) = build(422, {
      'code': 'validation_failed',
      'status': 422,
      'errors': [
        {'field': 'value', 'code': 'unsupported_country'},
      ],
    });
    await expectLater(
      client.resolve(phone, LoginPurpose.registration),
      throwsA(
        isA<ApiFailure>()
            .having((f) => f.code, 'code', 'validation_failed')
            .having((f) => f.fieldErrors.single.field, 'field', 'value')
            .having(
              (f) => f.fieldErrors.single.code,
              'field code',
              'unsupported_country',
            ),
      ),
    );
  });

  test('429 -> rate_limited with Retry-After', () async {
    final (client, _) = build(
      429,
      {'code': 'rate_limited', 'status': 429},
      headers: {
        'retry-after': ['17'],
      },
    );
    await expectLater(
      client.resolve(email, LoginPurpose.signIn),
      throwsA(
        isA<ApiFailure>()
            .having((f) => f.code, 'code', 'rate_limited')
            .having(
              (f) => f.retryAfter,
              'retryAfter',
              const Duration(seconds: 17),
            ),
      ),
    );
  });

  test('503 -> dependency_unavailable (retryable)', () async {
    final (client, _) = build(503, {'code': 'dependency_unavailable'});
    await expectLater(
      client.resolve(email, LoginPurpose.recovery),
      throwsA(
        isA<ApiFailure>()
            .having((f) => f.code, 'code', 'dependency_unavailable')
            .having((f) => f.isRetryable, 'retryable', isTrue),
      ),
    );
  });

  test('a response that is not a pseudonym is rejected', () async {
    final (client, _) = build(200, {'identifier': 'an@example.com'});
    await expectLater(
      client.resolve(email, LoginPurpose.signIn),
      throwsA(isA<UnknownFailure>()),
    );
  });

  test('connection error -> NetworkFailure', () async {
    final dio = buildPublicApiDio(
      baseUrl: 'http://api.invalid',
      logger: AppLogger(),
    );
    dio.interceptors.add(
      InterceptorsWrapper(
        onRequest: (o, h) => h.reject(
          DioException(
            requestOptions: o,
            type: DioExceptionType.connectionError,
          ),
        ),
      ),
    );
    await expectLater(
      HttpLoginIdentifierResolver(dio).resolve(email, LoginPurpose.signIn),
      throwsA(isA<NetworkFailure>()),
    );
  });

  group('LoginInput', () {
    String? check(LoginType t, String v) =>
        LoginInput(type: t, value: v).validate();

    test('email shape', () {
      expect(check(LoginType.email, ' An@Example.com '), isNull);
      for (final bad in ['', 'an', 'an@', 'an@example', 'a n@example.com']) {
        expect(check(LoginType.email, bad), 'invalid_format', reason: bad);
      }
      expect(check(LoginType.email, '${'a' * 250}@example.com'), 'too_long');
    });

    test('phone: VN national (0…) and international (+84… / 0084…)', () {
      for (final ok in [
        '0901234567',
        '0901 234 567',
        '090-123-4567',
        '+84901234567',
        '+84 90 123 4567',
        '0084901234567',
        '+12025550123',
      ]) {
        expect(check(LoginType.phone, ok), isNull, reason: ok);
      }
      for (final bad in ['', '901234567', '+84abc', '0', '+0901234567']) {
        expect(check(LoginType.phone, bad), 'invalid_format', reason: bad);
      }
      expect(check(LoginType.phone, '090\u0000123'), 'invalid_characters');
    });

    test('normalised (cache key only)', () {
      expect(
        const LoginInput(
          type: LoginType.phone,
          value: '0901 234 567',
        ).normalised,
        '+84901234567',
      );
      expect(
        const LoginInput(
          type: LoginType.email,
          value: ' An@Example.COM ',
        ).normalised,
        'an@example.com',
      );
    });

    test('toString never contains the value', () {
      expect(phone.toString(), isNot(contains('0901')));
      expect(
        const PseudonymousLogin(pseudonym).toString(),
        isNot(contains('login.invalid')),
      );
    });
  });
}
