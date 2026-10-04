import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
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

  (HttpCustomerAuthApi, _Stub) build(
    int status,
    Object? body, {
    Map<String, List<String>> headers = const {},
  }) {
    final stub = _Stub(status, body, headers: headers);
    final dio = buildPublicApiDio(
      baseUrl: 'http://api.invalid',
      logger: AppLogger(),
    )..interceptors.add(stub);
    return (HttpCustomerAuthApi(dio), stub);
  }

  Map<String, Object> sessionBody({String? verificationFlowId}) => {
    'session_token': 'ory_st_new',
    'session': {
      'id': 'sess-1',
      'active': true,
      'identity': {
        'id': 'id-1',
        'schema_id': 'customer',
        'traits': {'login_id': pseudonym},
      },
    },
    'verification_flow_id': ?verificationFlowId,
  };

  test('login: POST /v1/auth/login {login as typed, password}; no '
      'Authorization header; one call', () async {
    final (client, stub) = build(200, sessionBody());
    final out = await client.login(phone, 'pw');
    final req = stub.request!;
    expect(req.method, 'POST');
    expect(req.path, '/v1/auth/login');
    expect(req.data, {
      'login': {'type': 'phone', 'value': '0901 234 567'},
      'password': 'pw',
    });
    expect(req.headers.containsKey('Authorization'), isFalse);
    expect(out.sessionToken, 'ory_st_new');
    expect(out.session?.identity.loginId, pseudonym);
  });

  test('registration carries the verification flow id', () async {
    final (client, stub) = build(200, sessionBody(verificationFlowId: 'vf-1'));
    final out = await client.register(email, 'pw');
    expect(stub.request!.path, '/v1/auth/registration');
    expect(out.verificationFlowId, 'vf-1');
  });

  test('recovery: POST /v1/auth/recovery {login} -> sealed id', () async {
    final (client, stub) = build(200, {'recovery_id': 'vault:v1:abc'});
    expect(await client.startRecovery(email), 'vault:v1:abc');
    expect(stub.request!.data, {
      'login': {'type': 'email', 'value': 'An@Example.com'},
    });
  });

  test('400 auth_flow_rejected -> FlowValidationFailure with Kratos ids on '
      'login / password / form', () async {
    final (client, _) = build(400, {
      'code': 'auth_flow_rejected',
      'status': 400,
      'errors': [
        {'field': 'form', 'code': '4000006'},
        {'field': 'password', 'code': '4000032'},
        {'field': 'login', 'code': '4000007'},
      ],
    });
    try {
      await client.login(email, 'pw');
      fail('expected FlowValidationFailure');
    } on FlowValidationFailure catch (f) {
      expect(f.flow.messages.single.id, 4000006);
      expect(f.flow.messagesFor(AuthFlowFields.password).single.id, 4000032);
      expect(f.flow.messagesFor(AuthFlowFields.login).single.id, 4000007);
      expect(f.flow.hasErrors, isTrue);
    }
  });

  test('recovery code: POST /v1/auth/recovery/code -> grant', () async {
    final (client, stub) = build(200, {
      'session_token': 'ory_st_priv',
      'settings_flow_id': 's1',
    });
    final g = await client.submitRecoveryCode('vault:v1:abc', '123456');
    expect(stub.request!.path, '/v1/auth/recovery/code');
    expect(stub.request!.data, {
      'recovery_id': 'vault:v1:abc',
      'code': '123456',
    });
    expect(g.sessionToken, 'ory_st_priv');
    expect(g.settingsFlowId, 's1');
  });

  test('410 auth_flow_expired -> FlowExpiredFailure', () async {
    final (client, _) = build(410, {
      'code': 'auth_flow_expired',
      'status': 410,
    });
    await expectLater(
      client.submitRecoveryCode('vault:v1:abc', '123456'),
      throwsA(isA<FlowExpiredFailure>()),
    );
  });

  test('422 -> validation_failed with field errors', () async {
    final (client, _) = build(422, {
      'code': 'validation_failed',
      'status': 422,
      'errors': [
        {'field': 'login.value', 'code': 'unsupported_country'},
      ],
    });
    await expectLater(
      client.register(phone, 'pw'),
      throwsA(
        isA<ApiFailure>()
            .having((f) => f.code, 'code', 'validation_failed')
            .having((f) => f.fieldErrors.single.field, 'field', 'login.value'),
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
      client.login(email, 'pw'),
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

  test('503 -> retryable dependency_unavailable', () async {
    final (client, _) = build(503, {'code': 'dependency_unavailable'});
    await expectLater(
      client.startRecovery(email),
      throwsA(isA<ApiFailure>().having((f) => f.isRetryable, 'retry', isTrue)),
    );
  });

  test('a 200 without a session token is rejected', () async {
    final (client, _) = build(200, {'session': <String, Object>{}});
    await expectLater(
      client.login(email, 'pw'),
      throwsA(isA<UnknownFailure>()),
    );
  });
}
