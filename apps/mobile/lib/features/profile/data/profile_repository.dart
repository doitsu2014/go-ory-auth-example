import 'package:dio/dio.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/network/problem_mapper.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/me.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/personal_info.dart';

/// identity-service `GET/PATCH /v1/me` and `GET/PUT/DELETE
/// /v1/me/personal-info`. The Bearer token and 401 handling
/// live in the dio interceptors. Throws [AppFailure] only.
class ProfileRepository {
  ProfileRepository(this._dio);

  final Dio _dio;

  Future<Me> getMe() async {
    try {
      final res = await _dio.get<Map<String, dynamic>>('/v1/me');
      return Me.fromJson(res.data!);
    } on DioException catch (e) {
      throw mapApiError(e);
    } on Object catch (e) {
      throw UnknownFailure(detail: e.runtimeType.toString());
    }
  }

  /// Errors: `422 validation_failed` (field errors), `403 email_not_verified`.
  Future<Me> updateMe(UpdateMeRequest request) async {
    try {
      final res = await _dio.patch<Map<String, dynamic>>(
        '/v1/me',
        data: request.toJson(),
      );
      return Me.fromJson(res.data!);
    } on DioException catch (e) {
      throw mapApiError(e);
    } on Object catch (e) {
      throw UnknownFailure(detail: e.runtimeType.toString());
    }
  }

  /// `GET /v1/me/personal-info`. Errors: `503 dependency_unavailable`.
  Future<PersonalInfo> getPersonalInfo() async {
    try {
      final res = await _dio.get<Map<String, dynamic>>(_personalInfo);
      return PersonalInfo.fromJson(res.data!);
    } on DioException catch (e) {
      throw mapApiError(e);
    } on Object catch (e) {
      throw UnknownFailure(detail: e.runtimeType.toString());
    }
  }

  /// `PUT /v1/me/personal-info` (full replacement). Errors:
  /// `403 email_not_verified`, `422 validation_failed` (field + code only,
  /// never values), `503 dependency_unavailable`.
  Future<PersonalInfo> putPersonalInfo(PersonalInfo info) async {
    try {
      final res = await _dio.put<Map<String, dynamic>>(
        _personalInfo,
        data: info.toJson(),
      );
      return PersonalInfo.fromJson(res.data!);
    } on DioException catch (e) {
      throw mapApiError(e);
    } on Object catch (e) {
      throw UnknownFailure(detail: e.runtimeType.toString());
    }
  }

  /// `DELETE /v1/me/personal-info` (crypto-shred, idempotent, `204`).
  Future<void> erasePersonalInfo() async {
    try {
      await _dio.delete<void>(_personalInfo);
    } on DioException catch (e) {
      throw mapApiError(e);
    } on Object catch (e) {
      throw UnknownFailure(detail: e.runtimeType.toString());
    }
  }

  static const _personalInfo = '/v1/me/personal-info';
}
