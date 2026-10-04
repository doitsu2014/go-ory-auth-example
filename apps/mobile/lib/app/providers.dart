import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/core/config/env.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_client.dart';
import 'package:go_ory_auth_mobile/core/kratos/ory_kratos_client.dart';
import 'package:go_ory_auth_mobile/core/logging/app_logger.dart';
import 'package:go_ory_auth_mobile/core/network/api_client.dart';
import 'package:go_ory_auth_mobile/core/network/interceptors.dart';
import 'package:go_ory_auth_mobile/core/platform/secure_screen.dart';
import 'package:go_ory_auth_mobile/core/storage/secure_token_store.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/profile/data/profile_repository.dart';
import 'package:go_ory_auth_mobile/features/settings/data/settings_repository.dart';

// Dependency graph. Tests override the leaf providers (kratos client, token
// store, repositories).

final envProvider = Provider<Env>(
  (ref) => Env.fromEnvironment()..ensureSecure(release: kReleaseMode),
);

final loggerProvider = Provider<AppLogger>((ref) => AppLogger());

final tokenStoreProvider = Provider<TokenStore>((ref) => SecureTokenStore());

final kratosClientProvider = Provider<KratosClient>((ref) {
  final env = ref.watch(envProvider);
  return OryKratosClient(
    baseUrl: env.kratosPublicUrl,
    interceptors: [
      RequestIdInterceptor(),
      RedactingLogInterceptor(ref.watch(loggerProvider)),
    ],
  );
});

/// `POST /v1/auth/{login,registration,recovery}` (public: no Bearer token).
final customerAuthProvider = Provider<CustomerAuthApi>(
  (ref) => HttpCustomerAuthApi(
    buildPublicApiDio(
      baseUrl: ref.watch(envProvider).apiUrl,
      logger: ref.watch(loggerProvider),
    ),
  ),
);

final authRepositoryProvider = Provider<AuthRepository>(
  (ref) => AuthRepository(
    kratos: ref.watch(kratosClientProvider),
    tokens: ref.watch(tokenStoreProvider),
    customerAuth: ref.watch(customerAuthProvider),
  ),
);

final settingsRepositoryProvider = Provider<SettingsRepository>(
  (ref) => SettingsRepository(
    kratos: ref.watch(kratosClientProvider),
    tokens: ref.watch(tokenStoreProvider),
  ),
);

final authControllerProvider = NotifierProvider<AuthController, AuthState>(
  AuthController.new,
);

final apiDioProvider = Provider<Dio>((ref) {
  final tokens = ref.watch(tokenStoreProvider);
  return buildApiDio(
    baseUrl: ref.watch(envProvider).apiUrl,
    readToken: tokens.read,
    onUnauthorized: () =>
        ref.read(authControllerProvider.notifier).handleUnauthorized(),
    logger: ref.watch(loggerProvider),
  );
});

final profileRepositoryProvider = Provider<ProfileRepository>(
  (ref) => ProfileRepository(ref.watch(apiDioProvider)),
);

final secureScreenProvider = Provider<SecureScreen>((ref) => SecureScreen());
