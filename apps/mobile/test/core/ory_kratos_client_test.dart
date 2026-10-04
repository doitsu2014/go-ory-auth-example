import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/kratos/ory_kratos_client.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';

/// Captures the outgoing request and fails it before it hits the network.
class _Capture extends Interceptor {
  RequestOptions? request;

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    request = options;
    handler.reject(
      DioException(
        requestOptions: options,
        type: DioExceptionType.connectionError,
      ),
    );
  }
}

void main() {
  const pseudonym =
      'l4cwc5fmnvxqxufy7wuuh2mfathke4fvwo3curj5ydaoo3iijsgq@login.invalid';

  test('registration body carries traits.login_id only (NAME-FR-09, '
      'PLI-FR-08)', () async {
    final capture = _Capture();
    final client = OryKratosClient(
      baseUrl: 'http://kratos.invalid',
      interceptors: [capture],
    );
    await expectLater(
      client.submitRegistration(
        flowId: 'f1',
        loginId: pseudonym,
        password: 'pw',
      ),
      throwsA(isA<AppFailure>()),
    );
    final body = capture.request!.data;
    final json = (body is Map ? body : <String, dynamic>{})
        .cast<String, dynamic>();
    expect(json['method'], 'password');
    expect(json['traits'], {'login_id': pseudonym});
    expect((json['traits'] as Map).containsKey('name'), isFalse);
  });
}
