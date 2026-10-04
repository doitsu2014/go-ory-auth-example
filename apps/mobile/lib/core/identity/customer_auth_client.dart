import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/network/interceptors.dart';
import 'package:go_ory_auth_mobile/core/network/problem_mapper.dart';

/// Customer sign-in, registration and recovery start through identity-service
/// (ADR-0014): the app sends the email / phone number as typed and
/// identity-service drives the Kratos native flow with the account's opaque
/// handle, which the app never learns before it is signed in. One call per
/// sign-in.
///
/// Throws [AppFailure] only:
/// - [FlowValidationFailure] for `400 auth_flow_rejected` (Kratos message ids
///   on the synthetic nodes `login`, `password` and `form`),
/// - `ApiFailure('validation_failed')` with `fieldErrors` on `login.type` /
///   `login.value` / `password` (422),
/// - `ApiFailure('rate_limited')` with `retryAfter` (429),
/// - `ApiFailure('dependency_unavailable')` (503, retryable),
/// - [NetworkFailure].
abstract interface class CustomerAuthApi {
  Future<NativeAuthResult> login(LoginInput login, String password);

  /// The result carries the verification flow id when Kratos started one.
  Future<NativeAuthResult> register(LoginInput login, String password);

  /// Starts a Kratos recovery flow (method code) and returns an opaque
  /// `recovery_id` (the flow sealed by identity-service: a Kratos recovery
  /// flow would show the handle to anyone holding its id). The answer has the
  /// same shape whether or not the account exists.
  Future<String> startRecovery(LoginInput login);

  /// Submits the code on [recoveryId]. Returns the privileged session and
  /// the settings flow; the new password then goes to Kratos directly.
  /// A wrong code is a [FlowValidationFailure]; an expired flow a
  /// [FlowExpiredFailure] (`410 auth_flow_expired`).
  Future<({String sessionToken, String settingsFlowId})> submitRecoveryCode(
    String recoveryId,
    String code,
  );
}

/// Names of the synthetic flow nodes built from `auth_flow_rejected`.
abstract final class AuthFlowFields {
  static const login = 'login';
  static const password = 'password';
  static const form = 'form';
}

/// identity-service `POST /v1/auth/{login,registration,recovery}`. Public
/// endpoints: the dio instance must not carry the Bearer interceptor. The
/// body (address, password) is never logged: the log interceptor writes
/// method, path and status only.
class HttpCustomerAuthApi implements CustomerAuthApi {
  HttpCustomerAuthApi(this._dio);

  final Dio _dio;

  static const loginPath = '/v1/auth/login';
  static const registrationPath = '/v1/auth/registration';
  static const recoveryPath = '/v1/auth/recovery';
  static const recoveryCodePath = '/v1/auth/recovery/code';

  static Map<String, Object> _login(LoginInput l) => {
    'type': l.type.wire,
    'value': l.value,
  };

  @override
  Future<NativeAuthResult> login(LoginInput login, String password) =>
      _auth(loginPath, {'login': _login(login), 'password': password});

  @override
  Future<NativeAuthResult> register(LoginInput login, String password) =>
      _auth(registrationPath, {'login': _login(login), 'password': password});

  @override
  Future<String> startRecovery(LoginInput login) async {
    final data = await _post(recoveryPath, {'login': _login(login)});
    final id = data['recovery_id'];
    if (id is! String || id.isEmpty) {
      throw const UnknownFailure(detail: 'bad_recovery_response');
    }
    return id;
  }

  @override
  Future<({String sessionToken, String settingsFlowId})> submitRecoveryCode(
    String recoveryId,
    String code,
  ) async {
    final data = await _post(recoveryCodePath, {
      'recovery_id': recoveryId,
      'code': code,
    });
    final token = data['session_token'];
    final settings = data['settings_flow_id'];
    if (token is! String || token.isEmpty || settings is! String) {
      throw const UnknownFailure(detail: 'bad_recovery_code_response');
    }
    return (sessionToken: token, settingsFlowId: settings);
  }

  Future<NativeAuthResult> _auth(String path, Map<String, Object> body) async {
    final data = await _post(path, body);
    final token = data['session_token'];
    final session = data['session'];
    if (token is! String || token.isEmpty || session is! Map) {
      throw const UnknownFailure(detail: 'bad_session_response');
    }
    final verificationFlowId = data['verification_flow_id'];
    return NativeAuthResult(
      sessionToken: token,
      session: KratosSession.fromJson(session.cast<String, dynamic>()),
      continueWith: [
        if (verificationFlowId is String && verificationFlowId.isNotEmpty)
          ContinueWithVerificationUi(flowId: verificationFlowId),
      ],
    );
  }

  Future<Json> _post(String path, Map<String, Object> body) async {
    try {
      final res = await _dio.post<Map<String, dynamic>>(path, data: body);
      return res.data ?? const {};
    } on DioException catch (e) {
      final r = e.response;
      final data = r?.data;
      if (r?.statusCode == 410 &&
          data is Map &&
          data['code'] == 'auth_flow_expired') {
        throw const FlowExpiredFailure(detail: 'auth_flow_expired');
      }
      if (r?.statusCode == 400 &&
          data is Map &&
          data['code'] == 'auth_flow_rejected') {
        throw FlowValidationFailure(
          rejectedFlow(mapProblem(400, data) as ApiFailure),
        );
      }
      throw mapApiError(e);
    } on AppFailure {
      rethrow;
    } on Object catch (e) {
      throw UnknownFailure(detail: e.runtimeType.toString());
    }
  }
}

/// The synthetic flow for an `auth_flow_rejected` problem: one node per
/// field (`login`, `password`) with the Kratos message ids as error messages;
/// `form` messages are global. The text stays empty: the UI localises by id
/// (`kratosMessage`).
KratosFlow rejectedFlow(ApiFailure problem) {
  final byField = <String, List<UiText>>{};
  final global = <UiText>[];
  for (final e in problem.fieldErrors) {
    final m = UiText(id: int.tryParse(e.code) ?? 0, text: '', type: 'error');
    if (e.field == AuthFlowFields.login || e.field == AuthFlowFields.password) {
      (byField[e.field] ??= []).add(m);
    } else {
      global.add(m);
    }
  }
  return KratosFlow(
    id: '',
    messages: global,
    nodes: [
      for (final e in byField.entries)
        UiNode(type: 'input', group: 'default', name: e.key, messages: e.value),
    ],
  );
}

/// dio for the public identity-service endpoints: no `Authorization` header.
Dio buildPublicApiDio({required String baseUrl, required AppLogger logger}) {
  final dio = Dio(
    BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: const Duration(seconds: 10),
      // Kratos hashes the password and checks it against HaveIBeenPwned.
      receiveTimeout: const Duration(seconds: 30),
      headers: {'Accept': 'application/json, application/problem+json'},
    ),
  );
  dio.interceptors.addAll([
    RequestIdInterceptor(),
    RedactingLogInterceptor(logger),
  ]);
  return dio;
}
