import 'package:dio/dio.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_error_mapper.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_messages.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/network/problem_mapper.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

DioException _bad(int status, Object? body) {
  final req = RequestOptions(path: '/x');
  return DioException.badResponse(
    statusCode: status,
    requestOptions: req,
    response: Response<Object?>(
      requestOptions: req,
      statusCode: status,
      data: body,
    ),
  );
}

void main() {
  group('Kratos error -> AppFailure', () {
    test('400 with flow -> FlowValidationFailure carrying node messages', () {
      final f = mapKratosError(
        _bad(400, {
          'id': 'f1',
          'ui': {
            'nodes': [
              {
                'type': 'input',
                'group': 'password',
                'attributes': {'name': 'password', 'type': 'password'},
                'messages': [
                  {'id': 4000034, 'text': 'breached', 'type': 'error'},
                ],
                'meta': <String, dynamic>{},
              },
            ],
            'messages': [
              {'id': 4000006, 'text': 'invalid creds', 'type': 'error'},
            ],
          },
        }),
      );
      expect(f, isA<FlowValidationFailure>());
      final flow = (f as FlowValidationFailure).flow;
      expect(flow.id, 'f1');
      expect(flow.messagesFor('password').single.id, 4000034);
      expect(flow.messages.single.id, 4000006);
      expect(flow.hasErrors, isTrue);
    });

    test('410 -> FlowExpiredFailure', () {
      expect(
        mapKratosError(
          _bad(410, {
            'error': {'id': 'self_service_flow_expired', 'code': 410},
          }),
        ),
        isA<FlowExpiredFailure>(),
      );
    });

    test('401 -> UnauthenticatedFailure', () {
      expect(mapKratosError(_bad(401, {})), isA<UnauthenticatedFailure>());
    });

    test('403 session_refresh_required -> ApiFailure(code)', () {
      final f = mapKratosError(
        _bad(403, {
          'error': {'id': 'session_refresh_required', 'code': 403},
        }),
      );
      expect(
        f,
        isA<ApiFailure>().having(
          (e) => e.code,
          'code',
          'session_refresh_required',
        ),
      );
    });

    test('422 browser_location_change_required keeps redirect in detail', () {
      final f = mapKratosError(
        _bad(422, {
          'error': {'id': 'browser_location_change_required'},
          'redirect_browser_to': 'http://x/settings?flow=1',
        }),
      );
      expect(f.code, 'browser_location_change_required');
      expect(f.detail, contains('settings'));
    });

    test('connection error -> NetworkFailure', () {
      final e = DioException.connectionError(
        requestOptions: RequestOptions(path: '/'),
        reason: 'refused',
      );
      expect(mapKratosError(e), isA<NetworkFailure>());
    });
  });

  group('problem+json -> AppFailure', () {
    test('403 email_not_verified', () {
      final f = mapApiError(
        _bad(403, {
          'type': 'https://docs/problems/email-not-verified',
          'title': 'Email not verified',
          'status': 403,
          'code': 'email_not_verified',
        }),
      );
      expect(
        f,
        isA<ApiFailure>().having((e) => e.code, 'code', 'email_not_verified'),
      );
    });

    test('422 validation_failed with field errors', () {
      final f = mapApiError(
        _bad(422, {
          'title': 'Validation failed',
          'status': 422,
          'code': 'validation_failed',
          'errors': [
            {'field': 'display_name', 'code': 'too_long'},
          ],
        }),
      ) as ApiFailure;
      expect(f.code, 'validation_failed');
      expect(f.fieldErrors.single.field, 'display_name');
      expect(f.fieldErrors.single.code, 'too_long');
    });

    test('401 -> UnauthenticatedFailure', () {
      expect(
        mapApiError(_bad(401, {'code': 'unauthenticated'})),
        isA<UnauthenticatedFailure>(),
      );
    });

    test(
      'unknown code is kept verbatim; missing body falls back by status',
      () {
        expect(mapApiError(_bad(409, {'code': 'brand_new'})).code, 'brand_new');
        expect(mapApiError(_bad(503, null)).code, 'dependency_unavailable');
      },
    );

    test('timeout -> NetworkFailure', () {
      final e = DioException(
        requestOptions: RequestOptions(path: '/'),
        type: DioExceptionType.receiveTimeout,
      );
      expect(mapApiError(e), isA<NetworkFailure>());
    });
  });

  group('Kratos message id -> localised text', () {
    final en = lookupAppLocalizations(const Locale('en'));
    final vi = lookupAppLocalizations(const Locale('vi'));

    test('known id uses ARB with context placeholders', () {
      final m = err(
        4000032,
        'The password must be at least 12 characters long, but got 5.',
        {'min_length': 12},
      );
      expect(
        kratosMessage(en, m),
        'The password must be at least 12 characters long.',
      );
      expect(kratosMessage(vi, m), 'Mật khẩu phải có ít nhất 12 ký tự.');
    });

    test('invalid credentials (4000006) is localised', () {
      expect(
        kratosMessage(vi, err(4000006, 'x')),
        'Email hoặc mật khẩu không đúng.',
      );
    });

    test('unknown id falls back to Kratos text', () {
      expect(
        kratosMessage(vi, err(4999999, 'Some new Kratos message')),
        'Some new Kratos message',
      );
    });

    test('failureMessage maps API codes', () {
      expect(
        failureMessage(en, const ApiFailure('session_refresh_required')),
        en.sessionRefreshRequired,
      );
      expect(failureMessage(en, const NetworkFailure()), en.networkError);
      expect(failureMessage(en, const ApiFailure('whatever')), en.genericError);
      expect(
        failureMessage(en, const ApiFailure('dependency_unavailable')),
        en.dependencyUnavailable,
      );
      expect(const ApiFailure('dependency_unavailable').isRetryable, isTrue);
      expect(const NetworkFailure().isRetryable, isTrue);
      expect(const ApiFailure('validation_failed').isRetryable, isFalse);
    });
  });

  group('log redaction', () {
    test('removes session tokens, bearer credentials, passwords, codes', () {
      final out = AppLogger.redact(
        'token ory_st_AbC123xyz; Authorization: Bearer ory_st_AbC123xyz '
        '{"password":"hunter2hunter2","code":"123456","session_token":"s"}',
      );
      expect(out, isNot(contains('AbC123xyz')));
      expect(out, isNot(contains('hunter2')));
      expect(out, isNot(contains('123456')));
      expect(out, contains('<redacted>'));
    });

    test('logger never emits the raw token', () {
      final lines = <String>[];
      AppLogger(sink: (_, m) => lines.add(m))
          .info('stored ory_st_secretvalue1');
      expect(lines.single, isNot(contains('secretvalue1')));
    });

    test('NativeAuthResult.toString hides the token', () {
      const r = NativeAuthResult(sessionToken: 'ory_st_secret', session: null);
      expect(r.toString(), isNot(contains('secret')));
    });
  });
}

UiText err(int id, String text, [Map<String, dynamic> ctx = const {}]) =>
    UiText(id: id, text: text, type: 'error', context: ctx);
