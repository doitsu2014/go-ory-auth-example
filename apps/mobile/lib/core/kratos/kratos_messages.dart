import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

/// Localises a Kratos `ui.text` by its stable message `id`; unknown ids fall
/// back to Kratos' English `text` (architecture §3.9).
String kratosMessage(AppLocalizations l10n, UiText m) {
  switch (m.id) {
    case 1050001:
      return l10n.kratos1050001;
    case 1060001:
      return l10n.kratos1060001;
    case 1060003:
      return l10n.kratos1060003;
    case 1080002:
      return l10n.kratos1080002;
    case 1080003:
      return l10n.kratos1080003;
    case 4000002:
      return l10n.kratos4000002;
    case 4000006:
      return l10n.kratos4000006;
    case 4000007:
      return l10n.kratos4000007;
    case 4000010:
      return l10n.kratos4000010;
    case 4000031:
      return l10n.kratos4000031;
    case 4000032:
      return l10n.kratos4000032('${m.context['min_length'] ?? 12}');
    case 4000034:
      return l10n.kratos4000034;
    case 4060006:
      return l10n.kratos4060006;
    case 4070006:
      return l10n.kratos4070006;
    // identity-service pre-registration webhook (ADR-0013): an app that
    // still sends `traits.email`, and an unresolved / unconfirmed pseudonym.
    case 4049001:
      return l10n.kratos4049001;
    case 4049002:
      return l10n.kratos4049002;
    // Flow expired variants (login, registration, settings, recovery,
    // verification).
    case 4010001:
    case 4040001:
    case 4050001:
    case 4060005:
    case 4070005:
      return l10n.kratosFlowExpired;
  }
  return m.text.isNotEmpty ? m.text : l10n.genericError;
}

/// Localised text for a non-flow [AppFailure].
String failureMessage(AppLocalizations l10n, AppFailure f) {
  return switch (f) {
    NetworkFailure() => l10n.networkError,
    FlowExpiredFailure() => l10n.flowExpired,
    UnauthenticatedFailure() => l10n.unauthenticated,
    FlowValidationFailure() => l10n.validationFailed,
    UnknownFailure() => l10n.genericError,
    ApiFailure(:final code, :final retryAfter) => switch (code) {
      'rate_limited' => rateLimitedMessage(l10n, retryAfter),
      'session_refresh_required' => l10n.sessionRefreshRequired,
      'email_not_verified' => l10n.emailNotVerified,
      'recovery_session_unavailable' => l10n.recoverySessionUnavailable,
      'validation_failed' => l10n.validationFailed,
      'dependency_unavailable' => l10n.dependencyUnavailable,
      _ => l10n.genericError,
    },
  };
}

/// Rate-limit text for a `Retry-After` delay: seconds below a minute,
/// whole minutes from 60 s, whole hours from 3600 s (rounded up, so the user
/// is never told to retry too early).
String rateLimitedMessage(AppLocalizations l10n, Duration? retryAfter) {
  final seconds = retryAfter?.inSeconds ?? 0;
  if (seconds <= 0) return l10n.rateLimited;
  if (seconds < 60) return l10n.rateLimitedRetryAfter(seconds);
  if (seconds < 3600) {
    return l10n.rateLimitedRetryAfterMinutes((seconds / 60).ceil());
  }
  return l10n.rateLimitedRetryAfterHours((seconds / 3600).ceil());
}
