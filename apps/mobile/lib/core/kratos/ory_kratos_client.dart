import 'package:built_value/serializer.dart';
import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_client.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_error_mapper.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:one_of/one_of.dart';
import 'package:ory_client/ory_client.dart';

/// [KratosClient] backed by `ory_client` (`FrontendApi`) native flow calls.
///
/// ory_client's built_value models are re-serialised to wire JSON and parsed
/// into the app's own view models, so error bodies (raw JSON) and successful
/// responses share one parser and the rest of the app never depends on
/// built_value types.
class OryKratosClient implements KratosClient {
  OryKratosClient({
    required String baseUrl,
    List<Interceptor> interceptors = const [],
  }) : this.fromOry(
         OryClient(
           dio: Dio(
             BaseOptions(
               baseUrl: baseUrl,
               connectTimeout: const Duration(seconds: 10),
               receiveTimeout: const Duration(seconds: 15),
               // Kratos picks JSON vs redirect handling per request; some
               // endpoints (verification) redirect to the browser UI
               // without it.
               headers: {'Accept': 'application/json'},
               followRedirects: false,
             ),
           ),
           interceptors: interceptors,
         ),
       );

  OryKratosClient.fromOry(OryClient ory)
    : _api = ory.getFrontendApi(),
      _serializers = ory.serializers;

  final FrontendApi _api;
  final Serializers _serializers;

  /// Executes [send]; converts the typed response to JSON and parses it.
  /// If ory_client fails to *deserialize* a 2xx body (schema drift between
  /// the SDK and the pinned Kratos), falls back to the raw JSON.
  Future<T> _call<R extends Object, T>(
    Future<Response<R>> Function() send,
    FullType type,
    T Function(Json json) parse,
  ) async {
    try {
      final res = await send();
      final data = res.data;
      if (data == null) {
        return parse(const {});
      }
      final json = _serializers.serialize(data, specifiedType: type);
      return parse((json! as Map).cast<String, dynamic>());
    } on DioException catch (e) {
      final r = e.response;
      final status = r?.statusCode ?? 0;
      if (e.type == DioExceptionType.unknown &&
          r != null &&
          status >= 200 &&
          status < 300 &&
          r.data is Map) {
        return parse((r.data as Map).cast<String, dynamic>());
      }
      throw mapKratosError(e);
    }
  }

  KratosFlow _flow(Json j) => KratosFlow.fromJson(j);

  @override
  Future<KratosFlow> createLoginFlow({
    bool refresh = false,
    String? sessionToken,
  }) => _call(
    () => _api.createNativeLoginFlow(
      refresh: refresh ? true : null,
      xSessionToken: sessionToken,
    ),
    const FullType(LoginFlow),
    _flow,
  );

  @override
  Future<NativeAuthResult> submitLogin({
    required String flowId,
    required String identifier,
    required String password,
    String? sessionToken,
  }) {
    final body = UpdateLoginFlowBody(
      (b) => b
        ..oneOf = OneOfDynamic(
          typeIndex: 0,
          types: const [UpdateLoginFlowWithPasswordMethod],
          value: UpdateLoginFlowWithPasswordMethod(
            (p) => p
              ..method = 'password'
              ..identifier = identifier
              ..password = password,
          ),
        ),
    );
    return _call(
      () => _api.updateLoginFlow(
        flow: flowId,
        updateLoginFlowBody: body,
        xSessionToken: sessionToken,
      ),
      const FullType(SuccessfulNativeLogin),
      NativeAuthResult.fromJson,
    );
  }

  @override
  Future<KratosFlow> createVerificationFlow() => _call(
    _api.createNativeVerificationFlow,
    const FullType(VerificationFlow),
    _flow,
  );

  @override
  Future<KratosFlow> submitVerification({
    required String flowId,
    String? email,
    String? code,
  }) {
    final body = UpdateVerificationFlowBody(
      (b) => b
        ..oneOf = OneOfDynamic(
          typeIndex: 0,
          types: const [UpdateVerificationFlowWithCodeMethod],
          value: UpdateVerificationFlowWithCodeMethod(
            (p) => p
              ..method = UpdateVerificationFlowWithCodeMethodMethodEnum.code
              ..email = email
              ..code = code,
          ),
        ),
    );
    return _call(
      () => _api.updateVerificationFlow(
        flow: flowId,
        updateVerificationFlowBody: body,
      ),
      const FullType(VerificationFlow),
      _flow,
    );
  }

  @override
  Future<KratosFlow> createSettingsFlow({required String sessionToken}) =>
      _call(
        () => _api.createNativeSettingsFlow(xSessionToken: sessionToken),
        const FullType(SettingsFlow),
        _flow,
      );

  @override
  Future<KratosFlow> submitSettingsPassword({
    required String flowId,
    required String sessionToken,
    required String password,
  }) {
    final body = UpdateSettingsFlowBody(
      (b) => b
        ..oneOf = OneOfDynamic(
          typeIndex: 0,
          types: const [UpdateSettingsFlowWithPasswordMethod],
          value: UpdateSettingsFlowWithPasswordMethod(
            (p) => p
              ..method = 'password'
              ..password = password,
          ),
        ),
    );
    return _call(
      () => _api.updateSettingsFlow(
        flow: flowId,
        updateSettingsFlowBody: body,
        xSessionToken: sessionToken,
      ),
      const FullType(SettingsFlow),
      _flow,
    );
  }

  @override
  Future<KratosSession> toSession({required String sessionToken}) => _call(
    () => _api.toSession(xSessionToken: sessionToken),
    const FullType(Session),
    KratosSession.fromJson,
  );

  @override
  Future<void> logout({required String sessionToken}) async {
    try {
      await _api.performNativeLogout(
        performNativeLogoutBody: PerformNativeLogoutBody(
          (b) => b..sessionToken = sessionToken,
        ),
      );
    } on DioException catch (e) {
      throw mapKratosError(e);
    }
  }
}
