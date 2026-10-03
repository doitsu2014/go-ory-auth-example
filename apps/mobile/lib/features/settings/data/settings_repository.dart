import 'package:go_ory_auth_mobile/core/kratos/kratos_client.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_error_mapper.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/storage/secure_token_store.dart';

/// Kratos native settings flow (`X-Session-Token`): change password.
class SettingsRepository {
  SettingsRepository({required KratosClient kratos, required TokenStore tokens})
    : _kratos = kratos,
      _tokens = tokens;

  final KratosClient _kratos;
  final TokenStore _tokens;

  Future<String> _token() async {
    final t = await _tokens.read();
    if (t == null || t.isEmpty) throw const UnauthenticatedFailure();
    return t;
  }

  Future<KratosFlow> startSettings() async {
    try {
      return await _kratos.createSettingsFlow(sessionToken: await _token());
    } on Object catch (e) {
      throw mapKratosError(e);
    }
  }

  /// Returns the flow in state `success`. Errors: [FlowValidationFailure]
  /// (weak / breached password), [FlowExpiredFailure], `ApiFailure`
  /// `session_refresh_required` (privileged session older than 15 min).
  Future<KratosFlow> changePassword({
    required String flowId,
    required String password,
  }) async {
    try {
      return await _kratos.submitSettingsPassword(
        flowId: flowId,
        sessionToken: await _token(),
        password: password,
      );
    } on Object catch (e) {
      throw mapKratosError(e);
    }
  }

  /// Refresh login (`createNativeLoginFlow(refresh: true)` with the current
  /// session token) after `session_refresh_required`: re-proves the password
  /// so the session is privileged again. Stores a rotated token if Kratos
  /// returns one. Errors: [FlowValidationFailure] (wrong password).
  Future<void> reauthenticate({
    required String identifier,
    required String password,
  }) async {
    try {
      final token = await _token();
      final flow = await _kratos.createLoginFlow(
        refresh: true,
        sessionToken: token,
      );
      final result = await _kratos.submitLogin(
        flowId: flow.id,
        identifier: identifier,
        password: password,
        sessionToken: token,
      );
      if (result.sessionToken.isNotEmpty && result.sessionToken != token) {
        await _tokens.write(result.sessionToken);
      }
    } on Object catch (e) {
      throw mapKratosError(e);
    }
  }
}
