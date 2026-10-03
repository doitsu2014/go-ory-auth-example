/// Build-time configuration, injected with
/// `--dart-define-from-file=env/<flavour>.json`.
///
/// No secrets live here: only public base URLs.
class Env {
  const Env({required this.kratosPublicUrl, required this.apiUrl});

  /// Reads the compile-time defines. Defaults target the Android emulator
  /// talking to the local compose stack on the host.
  factory Env.fromEnvironment() => const Env(
    kratosPublicUrl: String.fromEnvironment(
      'KRATOS_PUBLIC_URL',
      defaultValue: 'http://10.0.2.2:4433',
    ),
    apiUrl: String.fromEnvironment(
      'API_URL',
      defaultValue: 'http://10.0.2.2:8080',
    ),
  );

  /// Kratos public API base URL (no trailing slash).
  final String kratosPublicUrl;

  /// identity-service base URL (no trailing slash).
  final String apiUrl;

  /// Fails fast when a release build is configured with a non-HTTPS URL
  /// (security §6.6: cleartext only for the local flavour).
  void ensureSecure({required bool release}) {
    if (!release) return;
    for (final (name, url) in [
      ('KRATOS_PUBLIC_URL', kratosPublicUrl),
      ('API_URL', apiUrl),
    ]) {
      if (Uri.tryParse(url)?.scheme != 'https') {
        throw StateError('$name must be https in release builds');
      }
    }
  }
}
