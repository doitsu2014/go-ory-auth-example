import 'dart:developer' as developer;

/// Minimal logger with a mandatory redaction filter.
///
/// Session tokens, bearer credentials, passwords and verification/recovery
/// codes must never reach a log sink. Every message passes through [redact]
/// before it is emitted. There is no `print` anywhere in the app.
class AppLogger {
  AppLogger({LogSink? sink}) : _sink = sink ?? _developerSink;

  final LogSink _sink;

  static void _developerSink(String level, String message) =>
      developer.log(message, name: 'app.$level');

  void debug(String message) => _sink('debug', redact(message));
  void info(String message) => _sink('info', redact(message));
  void warn(String message) => _sink('warn', redact(message));
  void error(String message) => _sink('error', redact(message));

  static final List<(RegExp, String)> _rules = [
    // Kratos session tokens (ory_st_...), exchange codes, etc.
    (RegExp('ory_[a-z]{2,4}_[A-Za-z0-9]+'), 'ory_<redacted>'),
    // Authorization: Bearer <...>
    (
      RegExp(r'(Bearer\s+)[A-Za-z0-9._~+/=-]+', caseSensitive: false),
      r'$1<redacted>',
    ),
    // JSON-ish / header-ish sensitive keys.
    (
      RegExp(
        '("?(?:session_token|ory_session_token|x-session-token|password|'
        r'code|token|authorization|cookie|set-cookie)"?\s*[:=]\s*)'
        r'("[^"]*"|\[[^\]]*\]|[^,}\s]+)',
        caseSensitive: false,
      ),
      r'$1<redacted>',
    ),
  ];

  /// Removes credentials from [input].
  static String redact(String input) {
    var out = input;
    for (final (pattern, replacement) in _rules) {
      out = out.replaceAllMapped(pattern, (m) {
        return replacement.replaceAllMapped(
          RegExp(r'\$(\d)'),
          (g) => m.group(int.parse(g.group(1)!)) ?? '',
        );
      });
    }
    return out;
  }
}

typedef LogSink = void Function(String level, String message);
