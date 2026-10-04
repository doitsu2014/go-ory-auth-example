import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/app/app.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/router.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/config/env.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/interceptors.dart';
import 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/sign_in_screen.dart';
import 'package:mocktail/mocktail.dart';

import '../helpers/helpers.dart';

void main() {
  group('authRedirect', () {
    final authed = Authenticated(session(verified: true));

    test('unknown -> splash', () {
      expect(authRedirect(const AuthUnknown(), Routes.profile), Routes.splash);
      expect(authRedirect(const AuthUnknown(), Routes.splash), isNull);
    });

    test('unauthenticated: public allowed, private -> sign-in', () {
      const s = Unauthenticated();
      expect(authRedirect(s, Routes.signIn), isNull);
      expect(authRedirect(s, Routes.signUp), isNull);
      expect(authRedirect(s, Routes.forgotPassword), isNull);
      expect(authRedirect(s, Routes.profile), Routes.signIn);
      expect(authRedirect(s, Routes.settings), Routes.signIn);
      expect(authRedirect(s, Routes.verifyEmail), Routes.signIn);
      expect(authRedirect(s, Routes.splash), Routes.signIn);
    });

    test('authenticated: public/splash -> profile, private allowed', () {
      expect(authRedirect(authed, Routes.signIn), Routes.profile);
      expect(authRedirect(authed, Routes.splash), Routes.profile);
      expect(authRedirect(authed, Routes.settings), isNull);
    });

    test('pending verification -> verify-email', () {
      final s = Authenticated(session(), pendingVerification: true);
      expect(authRedirect(s, Routes.signUp), Routes.verifyEmail);
      expect(authRedirect(s, Routes.profile), Routes.verifyEmail);
      expect(authRedirect(s, Routes.verifyEmail), isNull);
    });
  });

  testWidgets('startup without token lands on sign-in', (tester) async {
    final repo = MockAuthRepository();
    when(repo.restoreSession).thenAnswer((_) async => null);
    await tester.pumpWidget(
      ProviderScope(
        retry: (_, _) => null,
        overrides: [authRepositoryProvider.overrideWithValue(repo)],
        child: const App(),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.byType(SignInScreen), findsOneWidget);
  });

  test('controller: stale token at startup -> Unauthenticated', () async {
    final repo = MockAuthRepository();
    final restored = Completer<KratosSession?>();
    when(repo.restoreSession).thenAnswer((_) => restored.future);
    final c = ProviderContainer(
      retry: (_, _) => null,
      overrides: [authRepositoryProvider.overrideWithValue(repo)],
    );
    addTearDown(c.dispose);
    expect(c.read(authControllerProvider), isA<AuthUnknown>());
    await Future<void>.delayed(Duration.zero);
    restored.complete(null);
    await Future<void>.delayed(Duration.zero);
    expect(c.read(authControllerProvider), isA<Unauthenticated>());
  });

  test('dio 401 -> single-flight sign-out for concurrent requests', () async {
    var signOuts = 0;
    final gate = Completer<void>();
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = _Always401()
      ..interceptors.add(
        BearerAuthInterceptor(
          readToken: () async => 'ory_st_x',
          onUnauthorized: () async {
            signOuts++;
            await gate.future;
          },
        ),
      );
    final calls = [
      for (var i = 0; i < 3; i++)
        dio
            .get<void>('/v1/me')
            .then<Object?>((_) => null, onError: (Object e) => e),
    ];
    await Future<void>.delayed(const Duration(milliseconds: 20));
    gate.complete();
    final results = await Future.wait(calls);
    expect(signOuts, 1);
    expect(results, everyElement(isA<DioException>()));
  });

  test(
    'late 401 for an older token does not sign out a newer session',
    () async {
      var current = 'ory_st_old';
      var signOuts = 0;
      final adapter = _Always401(onFetch: () => current = 'ory_st_new');
      final dio = Dio(BaseOptions(baseUrl: 'http://test'))
        ..httpClientAdapter = adapter
        ..interceptors.add(
          BearerAuthInterceptor(
            readToken: () async => current,
            onUnauthorized: () async => signOuts++,
          ),
        );
      await expectLater(dio.get<void>('/v1/me'), throwsA(isA<DioException>()));
      expect(adapter.lastAuth, 'Bearer ory_st_old');
      expect(signOuts, 0);
    },
  );

  test('release build rejects non-https URLs; debug allows them', () {
    const local = Env(
      kratosPublicUrl: 'http://10.0.2.2:4433',
      apiUrl: 'http://10.0.2.2:8080',
    );
    expect(() => local.ensureSecure(release: false), returnsNormally);
    expect(() => local.ensureSecure(release: true), throwsStateError);
    const prod = Env(
      kratosPublicUrl: 'https://auth.example.com',
      apiUrl: 'https://api.example.com',
    );
    expect(() => prod.ensureSecure(release: true), returnsNormally);
  });

  test('bearer header is attached from the token store', () async {
    final adapter = _Always401();
    final dio = Dio(BaseOptions(baseUrl: 'http://test'))
      ..httpClientAdapter = adapter
      ..interceptors.add(
        BearerAuthInterceptor(
          readToken: () async => 'ory_st_abc',
          onUnauthorized: () async {},
        ),
      );
    await dio
        .get<void>('/v1/me')
        .catchError((_) => Response<void>(requestOptions: RequestOptions()));
    expect(adapter.lastAuth, 'Bearer ory_st_abc');
  });
}

class _Always401 implements HttpClientAdapter {
  _Always401({this.onFetch});

  final void Function()? onFetch;
  String? lastAuth;

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    lastAuth = options.headers['Authorization'] as String?;
    onFetch?.call();
    return ResponseBody.fromString(
      '{"code":"unauthenticated","status":401,'
      '"title":"x","type":"about:blank"}',
      401,
      headers: {
        Headers.contentTypeHeader: ['application/problem+json'],
      },
    );
  }

  @override
  void close({bool force = false}) {}
}
