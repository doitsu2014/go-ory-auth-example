# go_ory_auth_mobile (U6)

Customer mobile app. Talks to **Kratos public** with **native API flows only**
(registration, login, verification, recovery, settings, logout; no browser
flows, no WebView) and to **identity-service** `/v1/me` with
`Authorization: Bearer <session_token>`.

Design: `docs/principles/04-mobile-flutter.md`, `docs/architecture/03-auth-flows.md`.

## Run (local compose stack)

```sh
cp env/local.json.example env/local.json          # Android emulator (10.0.2.2)
cp env/local.ios.json.example env/local.ios.json  # iOS simulator (localhost)
flutter pub get
flutter run --dart-define-from-file=env/local.json
```

## Verify

```sh
flutter analyze
flutter test                       # unit + widget (integration tests are skipped)
flutter test -P integration        # real stack: Kratos :4433, Mailpit :8025, identity-service :8080 (optional)
# equivalent: flutter test --tags integration --run-skipped
```

The integration test (`test/integration/kratos_native_flow_test.dart`) is a
plain Dart `test`, so it does real HTTP. It registers a random user, reads the
verification and recovery codes from the Mailpit API, verifies the address,
changes the password through a settings flow, runs recovery, logs in, calls
`GET /v1/me` (only when identity-service answers), and logs out.

## Layout

```
lib/
  main.dart
  app/        app.dart (MaterialApp + l10n), router.dart (auth-aware redirect), providers.dart (DI)
  core/
    config/   env.dart (KRATOS_PUBLIC_URL, API_URL via --dart-define-from-file)
    kratos/   KratosClient (+ ory_client implementation), view models, error mapper, message-id → ARB
    logging/  AppLogger with redaction filter (no print)
    network/  dio interceptors (Bearer, X-Request-Id, redacting log, single-flight 401), problem+json → AppFailure
    storage/  SecureTokenStore (Keychain first_unlock_this_device / Keystore)
  features/
    auth/     data (AuthRepository), domain (AuthState), presentation (sign in/up, verify email, forgot password)
    profile/  /v1/me repository + models, profile and edit screens
    settings/ change password (settings flow), sign out
  l10n/       app_vi.arb (default), app_en.arb, gen/ (committed output of flutter gen-l10n)
```

## Cleartext HTTP (local only)

- **Android**: the main manifest sets `usesCleartextTraffic="false"`. Only the
  **debug** source set adds `res/xml/network_security_config.xml`, which allows
  cleartext to `10.0.2.2`, `localhost`, `127.0.0.1`. Release builds are HTTPS only.
- **iOS**: `Info.plist` has no ATS exception. For a local simulator run against
  `http://localhost`, add this to a **Debug-only** Info.plist / build setting,
  and never ship it in Release:

  ```xml
  <key>NSAppTransportSecurity</key>
  <dict><key>NSAllowsLocalNetworking</key><true/></dict>
  ```

Release builds: `flutter build apk --release --obfuscate --split-debug-info=build/symbols`.

## Server-side requirements (Kratos v26.2.0)

- `feature_flags.use_continue_with_transitions: true` is required. With it, a
  native recovery code submit returns `continue_with` [`set_ory_session_token`,
  `show_settings_ui`], and the app sets the new password in that privileged
  settings flow. Without it Kratos answers `422 browser_location_change_required`
  with no token; the app keeps that path only as a fallback
  (`recoverySessionUnavailable`), and the integration test fails on it.
- If registration doesn't return `show_verification_ui`, the verify screen
  starts its own native verification flow, which emails a fresh code.
- The verification endpoint redirects (303) to the browser UI unless
  `Accept: application/json` is sent. The Kratos dio client always sends it and
  never follows redirects.
- When a privileged recovery session is abandoned, the app revokes it.
- Changing the password after `privileged_session_max_age` (15 min) gets
  `403 session_refresh_required`. The settings screen then asks for the current
  password, runs a refresh login (`/self-service/login/api?refresh=true` with
  `X-Session-Token`), and retries the change.
- A Kratos `401` on any flow screen while signed in wipes the token, and the
  router sends the user to sign-in.
