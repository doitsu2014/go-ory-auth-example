# Mobile Guidelines (Flutter)

## 1. Stack

| Concern | Choice |
| --- | --- |
| Flutter / Dart | latest stable, pinned in `.fvmrc` (FVM) |
| State management | Riverpod (`flutter_riverpod` + code generation) |
| Navigation | `go_router` with an auth-aware `redirect` |
| HTTP | `dio` with interceptors (auth header, request id, logging w/ redaction) |
| Ory | `ory_client` (Dart, pub.dev) for Kratos public API flows |
| API client | `openapi-generator` (`dart-dio`) from `api/openapi/identity-service.v1.yaml` |
| Secure storage | `flutter_secure_storage` |
| Models | `freezed` + `json_serializable` |
| i18n | `flutter_localizations` + ARB files (vi, en) |
| Tests | `flutter_test`, `mocktail`, `integration_test` (or Patrol) |
| Lint | `flutter_lints` / `very_good_analysis` |
| Flavours | `local`, `staging`, `prod` via `--dart-define-from-file=env/<flavour>.json` |

## 2. Layout (feature-first, layered inside each feature)

```
apps/mobile/
├── lib/
│   ├── main_<flavour>.dart
│   ├── app/                 # App widget, router, theme, DI (providers)
│   ├── core/
│   │   ├── config/          # Env (KRATOS_PUBLIC_URL, API_URL)
│   │   ├── network/         # dio client, interceptors, problem+json → AppFailure
│   │   ├── storage/         # SecureTokenStore
│   │   └── kratos/          # KratosClient wrapper, UiNode → form model mapper, flow error handling
│   ├── features/
│   │   ├── auth/
│   │   │   ├── data/        # AuthRepository (registration/login/verification/recovery/logout)
│   │   │   ├── domain/      # AuthState (unauthenticated | authenticated(session) | needsVerification)
│   │   │   └── presentation/# SignUpScreen, SignInScreen, VerifyEmailScreen, ForgotPasswordScreen
│   │   ├── profile/         # ProfileRepository (identity-service /v1/me), screens
│   │   └── settings/        # change password (settings flow), sign out
│   └── l10n/
├── integration_test/
└── env/local.json.example
```

Dependency rule: `presentation → domain ← data`; widgets never call `dio` or
`ory_client` directly.

## 3. Auth rules

1. **Native API flows only**: `createNativeRegistrationFlow`, `createNativeLoginFlow`,
   `createNativeVerificationFlow`, `createNativeRecoveryFlow`, `createNativeSettingsFlow`,
   `performNativeLogout`. No WebView, no browser flows.
2. **Token storage**: the `session_token` lives only in `SecureTokenStore`
   (Keychain / Keystore). In memory it is held by the `AuthController`; never
   logged, never in analytics, never in crash reports.
3. **Startup**: read token → `toSession(xSessionToken)`; `200` → authenticated,
   `401` → delete token → sign-in.
4. **API calls**: dio interceptor adds `Authorization: Bearer <token>`; on `401`
   clears session and routes to sign-in (single-flight to avoid races).
5. **Registration → verification**: after `200`, follow `continue_with`
   (`show_verification_ui`) to the code screen; allow "resend code".
6. **Form rendering**: hand-built screens for the known fields (email, password,
   name, code) bound to Kratos node names; **errors always come from Kratos
   `ui.messages` / node messages** mapped by message `id` → ARB string. This
   keeps native UX while honouring Kratos validation (spike S1 compares with
   fully generic `ui.nodes` rendering).
7. Handle `400` (re-render same flow), `410` (restart flow), `403 aal2` (future MFA), network errors (retry UI).
8. **Logout**: `performNativeLogout(session_token)` then wipe storage and caches.

## 4. Coding rules

- Immutable state (`freezed`); `AsyncValue` for loading/error/data.
- Every repository method returns `Result`/throws a typed `AppFailure`; UI never sees `DioException`.
- No `print`; use a logger with a redaction filter.
- Release builds: `--obfuscate --split-debug-info`, cleartext HTTP disabled.
- Accessibility: semantic labels, dynamic text scaling, contrast.
- Generated code committed; CI verifies `build_runner` output is current.

## 5. Testing

- Unit: `AuthRepository` with mocked `ory_client`, mapping of Kratos errors → messages.
- Widget: each auth screen's states (idle, submitting, field error, global error).
- Integration (emulator + compose stack): register → read code from Mailpit API → verify → sign out → sign in → edit profile.
