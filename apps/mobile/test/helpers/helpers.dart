import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/app/app.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/identity/customer_auth_client.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_client.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/profile/data/profile_repository.dart';
import 'package:go_ory_auth_mobile/features/settings/data/settings_repository.dart';
import 'package:go_router/go_router.dart';
import 'package:mocktail/mocktail.dart';

class MockKratosClient extends Mock implements KratosClient {}

class MockAuthRepository extends Mock implements AuthRepository {}

class MockCustomerAuthApi extends Mock implements CustomerAuthApi {}

/// A Kratos login handle in the schema's shape
/// (`^[a-z2-7]{52}@login\.invalid$`).
const pseudonym =
    'l4cwc5fmnvxqxufy7wuuh2mfathke4fvwo3curj5ydaoo3iijsgq@login.invalid';

/// mocktail fallbacks for `any()` on login arguments.
void registerLoginFallbacks() {
  registerFallbackValue(const LoginInput(type: LoginType.email, value: ''));
}

class MockSettingsRepository extends Mock implements SettingsRepository {}

class MockProfileRepository extends Mock implements ProfileRepository {}

/// Auth controller pinned to a given initial state (no startup restore).
class FixedAuth extends AuthController {
  FixedAuth(this._initial);
  final AuthState _initial;

  @override
  AuthState build() => _initial;
}

/// Signed-in overrides: fixed [Authenticated] state; `forget`/`logout` stubbed.
List<Override> signedInOverrides(
  MockAuthRepository repo, {
  bool verified = true,
}) {
  when(repo.forget).thenAnswer((_) async {});
  when(repo.logout).thenAnswer((_) async {});
  return [
    authRepositoryProvider.overrideWithValue(repo),
    authControllerProvider.overrideWith(
      () => FixedAuth(Authenticated(session(verified: verified))),
    ),
  ];
}

AuthState authStateOf(WidgetTester tester, Finder finder) =>
    ProviderScope.containerOf(tester.element(finder))
        .read(authControllerProvider);

/// A customer session: `traits.login_id` is the handle (ADR-0013/0014).
KratosSession session({String loginId = pseudonym, bool verified = false}) =>
    KratosSession(
      id: 'sess-1',
      active: true,
      identity: KratosIdentity(
        id: 'id-1',
        schemaId: 'customer',
        traits: {'login_id': loginId},
        verifiableAddresses: [
          VerifiableAddress(value: loginId, verified: verified, via: 'email'),
        ],
      ),
    );

KratosFlow flowWith({
  String id = 'flow-1',
  String? state,
  Map<String, List<UiText>> nodeMessages = const {},
  List<UiText> messages = const [],
}) => KratosFlow(
  id: id,
  state: state,
  messages: messages,
  nodes: [
    for (final e in nodeMessages.entries)
      UiNode(type: 'input', group: 'default', name: e.key, messages: e.value),
  ],
);

UiText err(int id, String text, [Map<String, dynamic> ctx = const {}]) =>
    UiText(id: id, text: text, type: 'error', context: ctx);

/// Pumps [screen] at `/` inside a minimal router (other routes are stubs) in
/// English.
Future<GoRouter> pumpScreen(
  WidgetTester tester,
  Widget screen, {
  List<Override> overrides = const [],
}) async {
  final router = GoRouter(
    routes: [
      GoRoute(path: '/', builder: (_, _) => screen),
      for (final r in [
        Routes.profile,
        Routes.signIn,
        Routes.signUp,
        Routes.forgotPassword,
        Routes.verifyEmail,
        Routes.settings,
        Routes.personalInfo,
      ])
        GoRoute(
          path: r,
          builder: (_, _) => Scaffold(body: Text('route:$r')),
        ),
    ],
  );
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: overrides,
      child: MaterialApp.router(
        locale: const Locale('en'),
        localizationsDelegates: localizationsDelegates,
        supportedLocales: supportedLocales,
        routerConfig: router,
      ),
    ),
  );
  await tester.pump();
  return router;
}

/// Overrides that make the auth controller start "unauthenticated".
List<Override> authOverrides(MockAuthRepository repo) {
  when(repo.restoreSession).thenAnswer((_) async => null);
  return [authRepositoryProvider.overrideWithValue(repo)];
}
