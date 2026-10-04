import 'dart:convert';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/core/storage/login_identifier_cache.dart';
import 'package:go_ory_auth_mobile/core/storage/secure_token_store.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:mocktail/mocktail.dart';

import '../../helpers/helpers.dart';

void main() {
  late MockKratosClient kratos;
  late InMemoryTokenStore tokens;
  late MockLoginIdentifierResolver resolver;
  late InMemoryLoginIdentifierCache cache;
  late AuthRepository repo;

  const emailLogin = LoginInput(type: LoginType.email, value: 'An@Example.com');
  const phoneLogin = LoginInput(type: LoginType.phone, value: '0901 234 567');

  const stalePseudonym =
      'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@login.invalid';

  setUpAll(registerLoginFallbacks);

  setUp(() {
    kratos = MockKratosClient();
    tokens = InMemoryTokenStore();
    resolver = MockLoginIdentifierResolver();
    cache = InMemoryLoginIdentifierCache();
    repo = AuthRepository(
      kratos: kratos,
      tokens: tokens,
      resolver: resolver,
      loginCache: cache,
    );
    when(() => resolver.resolve(any(), any()))
        .thenAnswer((_) async => pseudonym);
  });

  group('restoreSession', () {
    test('no token -> null, Kratos not called', () async {
      expect(await repo.restoreSession(), isNull);
      verifyNever(
        () => kratos.toSession(sessionToken: any(named: 'sessionToken')),
      );
    });

    test('valid token -> session', () async {
      await tokens.write('ory_st_valid');
      when(() => kratos.toSession(sessionToken: 'ory_st_valid'))
          .thenAnswer((_) async => session());
      expect((await repo.restoreSession())?.id, 'sess-1');
      expect(await tokens.read(), 'ory_st_valid');
    });

    test('401 -> token wiped, null', () async {
      await tokens.write('ory_st_stale');
      when(() => kratos.toSession(sessionToken: any(named: 'sessionToken')))
          .thenThrow(const UnauthenticatedFailure());
      expect(await repo.restoreSession(), isNull);
      expect(await tokens.read(), isNull);
    });

    test('network error -> NetworkFailure, token kept', () async {
      await tokens.write('ory_st_valid');
      when(() => kratos.toSession(sessionToken: any(named: 'sessionToken')))
          .thenThrow(const NetworkFailure());
      await expectLater(repo.restoreSession(), throwsA(isA<NetworkFailure>()));
      expect(await tokens.read(), 'ory_st_valid');
    });
  });

  group('register', () {
    for (final login in [emailLogin, phoneLogin]) {
      test('${login.type.name}: resolves (registration) then sends only the '
          'pseudonym; stores token, follows continue_with', () async {
        when(
          () => kratos.submitRegistration(
            flowId: 'f1',
            loginId: pseudonym,
            password: 'pw',
          ),
        ).thenAnswer(
          (_) async => NativeAuthResult(
            sessionToken: 'ory_st_new',
            session: session(),
            continueWith: const [ContinueWithVerificationUi(flowId: 'vf-1')],
          ),
        );
        final out = await repo.register(
          flowId: 'f1',
          login: login,
          password: 'pw',
        );
        verifyInOrder([
          () => resolver.resolve(login, LoginPurpose.registration),
          () => kratos.submitRegistration(
            flowId: 'f1',
            loginId: pseudonym,
            password: 'pw',
          ),
        ]);
        expect(out.verificationFlowId, 'vf-1');
        expect(await tokens.read(), 'ory_st_new');
      });
    }

    test(
      'resolver 422 -> ApiFailure with field errors, Kratos not called',
      () async {
        when(() => resolver.resolve(any(), any())).thenThrow(
          const ApiFailure(
            'validation_failed',
            status: 422,
            fieldErrors: [FieldError(field: 'value', code: 'invalid_format')],
          ),
        );
        await expectLater(
          repo.register(flowId: 'f1', login: emailLogin, password: 'pw'),
          throwsA(
            isA<ApiFailure>().having(
              (f) => f.fieldErrors.single.code,
              'code',
              'invalid_format',
            ),
          ),
        );
        verifyNever(
          () => kratos.submitRegistration(
            flowId: any(named: 'flowId'),
            loginId: any(named: 'loginId'),
            password: any(named: 'password'),
          ),
        );
        expect(await tokens.read(), isNull);
      },
    );

    test('400 -> FlowValidationFailure, nothing stored', () async {
      final bad = flowWith(
        nodeMessages: {
          'password': [
            err(4000032, 'too short', {'min_length': 12}),
          ],
        },
      );
      when(
        () => kratos.submitRegistration(
          flowId: any(named: 'flowId'),
          loginId: any(named: 'loginId'),
          password: any(named: 'password'),
        ),
      ).thenThrow(FlowValidationFailure(bad));
      await expectLater(
        repo.register(flowId: 'f1', login: emailLogin, password: 'x'),
        throwsA(isA<FlowValidationFailure>()),
      );
      expect(await tokens.read(), isNull);
    });

    group('resolve once per flow and input', () {
      FlowValidationFailure passwordRejected() => FlowValidationFailure(
        flowWith(
          id: 'f1',
          nodeMessages: {
            'password': [
              err(4000032, 'too short', {'min_length': 12}),
            ],
          },
        ),
      );

      void stubRegistration(NativeAuthResult Function(String password) answer) {
        when(
          () => kratos.submitRegistration(
            flowId: any(named: 'flowId'),
            loginId: any(named: 'loginId'),
            password: any(named: 'password'),
          ),
        ).thenAnswer(
          (inv) async => answer(inv.namedArguments[#password] as String),
        );
      }

      test('submit -> Kratos 400 on password -> submit again: exactly one '
          'resolve', () async {
        stubRegistration(
          (pw) => pw == 'short'
              ? throw passwordRejected()
              : NativeAuthResult(sessionToken: 'ory_st', session: session()),
        );
        await expectLater(
          repo.register(flowId: 'f1', login: emailLogin, password: 'short'),
          throwsA(isA<FlowValidationFailure>()),
        );
        await repo.register(
          flowId: 'f1',
          login: emailLogin,
          password: 'a long enough password',
        );
        verify(() => resolver.resolve(emailLogin, LoginPurpose.registration))
            .called(1);
        verify(
          () => kratos.submitRegistration(
            flowId: 'f1',
            loginId: pseudonym,
            password: any(named: 'password'),
          ),
        ).called(2);
      });

      test('changed input or a new flow resolves again', () async {
        stubRegistration((_) => throw passwordRejected());
        Future<void> attempt(String flowId, LoginInput login) => expectLater(
          repo.register(flowId: flowId, login: login, password: 'short'),
          throwsA(isA<FlowValidationFailure>()),
        );
        await attempt('f1', emailLogin);
        await attempt('f1', emailLogin);
        // Same value, other type.
        await attempt(
          'f1',
          LoginInput(type: LoginType.phone, value: emailLogin.value),
        );
        // Other value.
        await attempt('f1', phoneLogin);
        await attempt('f1', phoneLogin);
        // New flow.
        await attempt('f2', phoneLogin);
        verify(() => resolver.resolve(any(), LoginPurpose.registration))
            .called(4);
      });
    });
  });

  group('login', () {
    void stubLogin(String identifier, {String token = 'ory_st_login'}) {
      when(
        () => kratos.submitLogin(
          flowId: 'l1',
          identifier: identifier,
          password: 'pw',
        ),
      ).thenAnswer(
        (_) async => NativeAuthResult(sessionToken: token, session: session()),
      );
    }

    for (final login in [emailLogin, phoneLogin]) {
      test('${login.type.name}: resolves (sign_in), signs in with the '
          'pseudonym, stores token and caches the pseudonym', () async {
        stubLogin(pseudonym);
        await repo.login(flowId: 'l1', login: login, password: 'pw');
        verifyInOrder([
          () => resolver.resolve(login, LoginPurpose.signIn),
          () => kratos.submitLogin(
            flowId: 'l1',
            identifier: pseudonym,
            password: 'pw',
          ),
        ]);
        expect(await tokens.read(), 'ory_st_login');
        expect(await cache.read(login), pseudonym);
      });
    }

    test('cache key is HMAC-SHA256(install secret, type:normalised value); '
        'the address is never stored', () async {
      stubLogin(pseudonym);
      await repo.login(flowId: 'l1', login: phoneLogin, password: 'pw');
      final key = cache.entries.keys.singleWhere(
        (k) => k.startsWith(loginIdentifierCacheKeyPrefix),
      );
      expect(key, matches(RegExp(r'^login_id\.[0-9a-f]{64}$')));
      expect(key, isNot(contains('0901')));
      expect(cache.entries.values.join(), isNot(contains('0901')));
      final secret = base64Decode(
        cache.entries[loginIdentifierCacheSecretKey]!,
      );
      expect(secret, hasLength(32));
      expect(key, loginIdentifierCacheKey(phoneLogin, secret));
      expect(
        key,
        isNot('login_id.${sha256.convert(utf8.encode('phone:+84901234567'))}'),
        reason: 'not an unsalted hash of the address',
      );
      expect(
        key,
        await cache.keyFor(
          const LoginInput(type: LoginType.phone, value: '+84901234567'),
        ),
        reason: 'national and international forms share the entry',
      );
      expect(
        await cache.keyFor(emailLogin),
        await cache.keyFor(
          const LoginInput(type: LoginType.email, value: ' an@example.com '),
        ),
      );
      expect(
        await cache.keyFor(
          const LoginInput(type: LoginType.email, value: '0901234567'),
        ),
        isNot(await cache.keyFor(phoneLogin)),
      );
    });

    test('the secret is generated once per install and re-used', () async {
      final first = await cache.keyFor(emailLogin);
      final stored = cache.entries[loginIdentifierCacheSecretKey];
      expect(await cache.keyFor(emailLogin), first);
      expect(cache.entries[loginIdentifierCacheSecretKey], stored);

      // Another install (another secret) keys the same input differently.
      expect(
        await InMemoryLoginIdentifierCache().keyFor(emailLogin),
        isNot(first),
      );

      // A new cache instance over the same storage keeps the secret.
      final reopened = InMemoryLoginIdentifierCache()
        ..entries.addAll(cache.entries);
      expect(await reopened.keyFor(emailLogin), first);
    });

    test('a cached value that is not a pseudonym is evicted and resolved '
        'again', () async {
      cache.entries[await cache.keyFor(emailLogin)] = 'an@example.com';
      stubLogin(pseudonym);
      await repo.login(flowId: 'l1', login: emailLogin, password: 'pw');
      verify(() => resolver.resolve(emailLogin, LoginPurpose.signIn)).called(1);
      expect(await cache.read(emailLogin), pseudonym);
      expect(cache.entries.values, isNot(contains('an@example.com')));
    });

    test('next sign-in with the same input re-uses the cached pseudonym '
        '(no resolve)', () async {
      await cache.write(emailLogin, pseudonym);
      stubLogin(pseudonym);
      await repo.login(
        flowId: 'l1',
        login: const LoginInput(type: LoginType.email, value: 'an@example.com'),
        password: 'pw',
      );
      verifyNever(() => resolver.resolve(any(), any()));
      expect(await tokens.read(), 'ory_st_login');
    });

    test(
      'rejected cached pseudonym is dropped; next attempt resolves',
      () async {
        await cache.write(emailLogin, stalePseudonym);
        when(
          () => kratos.submitLogin(
            flowId: 'l1',
            identifier: stalePseudonym,
            password: 'pw',
          ),
        ).thenThrow(
          FlowValidationFailure(
            flowWith(id: 'l1', messages: [err(4000006, 'invalid')]),
          ),
        );
        await expectLater(
          repo.login(flowId: 'l1', login: emailLogin, password: 'pw'),
          throwsA(isA<FlowValidationFailure>()),
        );
        verifyNever(() => resolver.resolve(any(), any()));
        expect(await cache.read(emailLogin), isNull);

        stubLogin(pseudonym);
        await repo.login(flowId: 'l1', login: emailLogin, password: 'pw');
        verify(() => resolver.resolve(emailLogin, LoginPurpose.signIn))
            .called(1);
        expect(await cache.read(emailLogin), pseudonym);
      },
    );

    test('wrong password: nothing cached, no token', () async {
      when(
        () => kratos.submitLogin(
          flowId: any(named: 'flowId'),
          identifier: any(named: 'identifier'),
          password: any(named: 'password'),
        ),
      ).thenThrow(
        FlowValidationFailure(
          flowWith(id: 'l1', messages: [err(4000006, 'invalid')]),
        ),
      );
      await expectLater(
        repo.login(flowId: 'l1', login: emailLogin, password: 'pw'),
        throwsA(isA<FlowValidationFailure>()),
      );
      expect(
        cache.entries.keys.where(
          (k) => k.startsWith(loginIdentifierCacheKeyPrefix),
        ),
        isEmpty,
      );
      expect(await tokens.read(), isNull);
    });

    test(
      'resolver 429 / 503 surface as ApiFailure; Kratos not called',
      () async {
        for (final failure in [
          const ApiFailure(
            'rate_limited',
            status: 429,
            retryAfter: Duration(seconds: 30),
          ),
          const ApiFailure('dependency_unavailable', status: 503),
        ]) {
          when(() => resolver.resolve(any(), any())).thenThrow(failure);
          await expectLater(
            repo.login(flowId: 'l1', login: phoneLogin, password: 'pw'),
            throwsA(same(failure)),
          );
        }
        verifyNever(
          () => kratos.submitLogin(
            flowId: any(named: 'flowId'),
            identifier: any(named: 'identifier'),
            password: any(named: 'password'),
          ),
        );
      },
    );
  });

  group('verification', () {
    test('start with typed input: resolves (verification), then sends the '
        'pseudonym as Kratos email', () async {
      when(kratos.createVerificationFlow)
          .thenAnswer((_) async => flowWith(id: 'v1'));
      when(() => kratos.submitVerification(flowId: 'v1', email: pseudonym))
          .thenAnswer((_) async => flowWith(id: 'v1', state: 'sent_email'));
      await repo.startVerification(phoneLogin);
      verify(() => resolver.resolve(phoneLogin, LoginPurpose.verification))
          .called(1);
      verify(() => kratos.submitVerification(flowId: 'v1', email: pseudonym))
          .called(1);
    });

    test('empty session pseudonym -> UnauthenticatedFailure, Kratos not '
        'called', () async {
      await expectLater(
        repo.startVerification(const PseudonymousLogin('')),
        throwsA(isA<UnauthenticatedFailure>()),
      );
      await expectLater(
        repo.resendVerificationCode(
          flowId: 'vf',
          target: const PseudonymousLogin(''),
        ),
        throwsA(isA<UnauthenticatedFailure>()),
      );
      verifyNever(() => kratos.createVerificationFlow());
      verifyNever(
        () => kratos.submitVerification(
          flowId: any(named: 'flowId'),
          email: any(named: 'email'),
        ),
      );
    });

    test('start / resend with the session pseudonym: no resolve', () async {
      when(kratos.createVerificationFlow)
          .thenAnswer((_) async => flowWith(id: 'v1'));
      when(() => kratos.submitVerification(flowId: 'v1', email: pseudonym))
          .thenAnswer((_) async => flowWith(id: 'v1', state: 'sent_email'));
      await repo.startVerification(const PseudonymousLogin(pseudonym));
      await repo.resendVerificationCode(
        flowId: 'v1',
        target: const PseudonymousLogin(pseudonym),
      );
      verifyNever(() => resolver.resolve(any(), any()));
      verify(() => kratos.submitVerification(flowId: 'v1', email: pseudonym))
          .called(2);
    });

    test('resend with typed input resolves (verification)', () async {
      when(() => kratos.submitVerification(flowId: 'v1', email: pseudonym))
          .thenAnswer((_) async => flowWith(id: 'v1', state: 'sent_email'));
      await repo.resendVerificationCode(flowId: 'v1', target: emailLogin);
      verify(() => resolver.resolve(emailLogin, LoginPurpose.verification))
          .called(1);
    });

    test('wrong code (200 + error message) -> FlowValidationFailure', () async {
      when(() => kratos.submitVerification(flowId: 'v1', code: '000000'))
          .thenAnswer(
            (_) async => flowWith(
              id: 'v1',
              state: 'sent_email',
              messages: [err(4070006, 'invalid')],
            ),
          );
      await expectLater(
        repo.verify(flowId: 'v1', code: '000000'),
        throwsA(isA<FlowValidationFailure>()),
      );
    });

    test('passed_challenge -> flow', () async {
      when(
        () => kratos.submitVerification(flowId: 'v1', code: '123456'),
      ).thenAnswer((_) async => flowWith(id: 'v1', state: 'passed_challenge'));
      expect(
        (await repo.verify(flowId: 'v1', code: '123456')).state,
        'passed_challenge',
      );
    });
  });

  group('recovery', () {
    for (final login in [emailLogin, phoneLogin]) {
      test('${login.type.name}: request code resolves (recovery) and sends '
          'the pseudonym as Kratos email', () async {
        when(() => kratos.submitRecovery(flowId: 'r1', email: pseudonym))
            .thenAnswer((_) async => flowWith(id: 'r1', state: 'sent_email'));
        await repo.requestRecoveryCode(flowId: 'r1', login: login);
        verifyInOrder([
          () => resolver.resolve(login, LoginPurpose.recovery),
          () => kratos.submitRecovery(flowId: 'r1', email: pseudonym),
        ]);
      });
    }

    test('code -> grant from continue_with; new password signs in', () async {
      when(() => kratos.submitRecovery(flowId: 'r1', code: '111111'))
          .thenAnswer(
            (_) async => const KratosFlow(
              id: 'r1',
              state: 'passed_challenge',
              continueWith: [
                ContinueWithSettingsUi(flowId: 's1'),
                ContinueWithSetSessionToken('ory_st_priv'),
              ],
            ),
          );
      when(
        () => kratos.submitSettingsPassword(
          flowId: 's1',
          sessionToken: 'ory_st_priv',
          password: 'new-pass',
        ),
      ).thenAnswer((_) async => flowWith(id: 's1', state: 'success'));
      when(() => kratos.toSession(sessionToken: 'ory_st_priv'))
          .thenAnswer((_) async => session());

      final grant = await repo.submitRecoveryCode(flowId: 'r1', code: '111111');
      expect(grant.settingsFlowId, 's1');
      expect(
        await tokens.read(),
        isNull,
        reason: 'not signed in until password set',
      );
      await repo.completeRecovery(grant: grant, password: 'new-pass');
      expect(await tokens.read(), 'ory_st_priv');
    });

    test(
      '422 browser_location_change_required -> recovery_session_unavailable',
      () async {
        when(() => kratos.submitRecovery(flowId: 'r1', code: '111111'))
            .thenThrow(
              const ApiFailure('browser_location_change_required', status: 422),
            );
        await expectLater(
          repo.submitRecoveryCode(flowId: 'r1', code: '111111'),
          throwsA(
            isA<ApiFailure>().having(
              (f) => f.code,
              'code',
              'recovery_session_unavailable',
            ),
          ),
        );
      },
    );

    test('wrong code -> FlowValidationFailure', () async {
      when(() => kratos.submitRecovery(flowId: 'r1', code: '000000'))
          .thenAnswer(
            (_) async => flowWith(
              id: 'r1',
              state: 'sent_email',
              messages: [err(4060006, 'invalid')],
            ),
          );
      await expectLater(
        repo.submitRecoveryCode(flowId: 'r1', code: '000000'),
        throwsA(isA<FlowValidationFailure>()),
      );
    });
  });

  group('logout', () {
    test('performNativeLogout then wipe', () async {
      await tokens.write('ory_st_x');
      when(() => kratos.logout(sessionToken: 'ory_st_x'))
          .thenAnswer((_) async {});
      await repo.logout();
      verify(() => kratos.logout(sessionToken: 'ory_st_x')).called(1);
      expect(await tokens.read(), isNull);
    });

    test('clears every cached pseudonym, keeps the install secret', () async {
      await cache.write(emailLogin, pseudonym);
      await cache.write(phoneLogin, stalePseudonym);
      await tokens.write('ory_st_x');
      when(() => kratos.logout(sessionToken: any(named: 'sessionToken')))
          .thenThrow(const NetworkFailure());
      await repo.logout();
      expect(await cache.read(emailLogin), isNull);
      expect(await cache.read(phoneLogin), isNull);
      expect(
        cache.entries.keys.where(
          (k) => k.startsWith(loginIdentifierCacheKeyPrefix),
        ),
        isEmpty,
      );
      expect(cache.entries, contains(loginIdentifierCacheSecretKey));
    });

    test('wipes even when Kratos is unreachable', () async {
      await tokens.write('ory_st_x');
      when(() => kratos.logout(sessionToken: any(named: 'sessionToken')))
          .thenThrow(const NetworkFailure());
      await repo.logout();
      expect(await tokens.read(), isNull);
    });
  });
}
