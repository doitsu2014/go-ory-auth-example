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
    ApiFailure(:final code) => switch (code) {
      'session_refresh_required' => l10n.sessionRefreshRequired,
      'email_not_verified' => l10n.emailNotVerified,
      'recovery_session_unavailable' => l10n.recoverySessionUnavailable,
      'validation_failed' => l10n.validationFailed,
      'dependency_unavailable' => l10n.dependencyUnavailable,
      _ => l10n.genericError,
    },
  };
}
