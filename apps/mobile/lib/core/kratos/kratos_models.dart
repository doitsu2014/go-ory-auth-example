// Hand-written immutable view models of the Kratos public API payloads.
//
// They are parsed from the wire JSON (snake_case) so the same parser handles
// successful responses (via ory_client models re-serialised to JSON) and error
// bodies (raw JSON from a 400 / 422 response).

typedef Json = Map<String, dynamic>;

Json? _asMap(Object? v) => v is Map ? v.cast<String, dynamic>() : null;
List<Object?> _asList(Object? v) => v is List ? v.cast<Object?>() : const [];

/// Kratos `ui.text` (message or label). [id] is the stable message id used
/// for localisation; [text] is the English fallback.
class UiText {
  const UiText({
    required this.id,
    required this.text,
    required this.type,
    this.context = const {},
  });

  factory UiText.fromJson(Json json) => UiText(
    id: (json['id'] as num?)?.toInt() ?? 0,
    text: json['text'] as String? ?? '',
    type: json['type'] as String? ?? 'info',
    context: _asMap(json['context']) ?? const {},
  );

  final int id;
  final String text;

  /// `info` | `error` | `success`.
  final String type;
  final Json context;

  bool get isError => type == 'error';

  @override
  String toString() => 'UiText($id, $type)';
}

/// A Kratos `ui.nodes[]` entry. Only input nodes matter for the hand-built
/// screens; other node types are kept for completeness.
class UiNode {
  const UiNode({
    required this.type,
    required this.group,
    required this.name,
    this.inputType,
    this.value,
    this.required = false,
    this.messages = const [],
    this.label,
  });

  factory UiNode.fromJson(Json json) {
    final attrs = _asMap(json['attributes']) ?? const {};
    final meta = _asMap(json['meta']) ?? const {};
    final label = _asMap(meta['label']);
    return UiNode(
      type: json['type'] as String? ?? 'input',
      group: json['group'] as String? ?? 'default',
      name: attrs['name'] as String? ?? attrs['id'] as String? ?? '',
      inputType: attrs['type'] as String?,
      value: attrs['value'],
      required: attrs['required'] as bool? ?? false,
      messages: _asList(json['messages'])
          .map((m) => UiText.fromJson(_asMap(m) ?? const {}))
          .toList(),
      label: label == null ? null : UiText.fromJson(label),
    );
  }

  final String type;
  final String group;
  final String name;
  final String? inputType;
  final Object? value;
  final bool required;
  final List<UiText> messages;
  final UiText? label;
}

/// `continue_with[]` actions returned by Kratos.
sealed class ContinueWith {
  const ContinueWith();

  static ContinueWith fromJson(Json json) {
    final flow = _asMap(json['flow']);
    switch (json['action']) {
      case 'set_ory_session_token':
        return ContinueWithSetSessionToken(
          json['ory_session_token'] as String? ?? '',
        );
      case 'show_verification_ui':
        return ContinueWithVerificationUi(
          flowId: flow?['id'] as String? ?? '',
          address: flow?['verifiable_address'] as String?,
        );
      case 'show_settings_ui':
        return ContinueWithSettingsUi(flowId: flow?['id'] as String? ?? '');
      default:
        return ContinueWithUnknown(json['action'] as String? ?? '');
    }
  }
}

final class ContinueWithSetSessionToken extends ContinueWith {
  const ContinueWithSetSessionToken(this.sessionToken);
  final String sessionToken;

  @override
  String toString() => 'ContinueWithSetSessionToken(<redacted>)';
}

final class ContinueWithVerificationUi extends ContinueWith {
  const ContinueWithVerificationUi({required this.flowId, this.address});
  final String flowId;
  final String? address;
}

final class ContinueWithSettingsUi extends ContinueWith {
  const ContinueWithSettingsUi({required this.flowId});
  final String flowId;
}

final class ContinueWithUnknown extends ContinueWith {
  const ContinueWithUnknown(this.action);
  final String action;
}

List<ContinueWith> parseContinueWith(Object? raw) =>
    _asList(raw)
        .map((e) => ContinueWith.fromJson(_asMap(e) ?? const {}))
        .toList();

/// Any self-service flow (registration, login, verification, recovery,
/// settings).
class KratosFlow {
  const KratosFlow({
    required this.id,
    this.type = 'api',
    this.state,
    this.nodes = const [],
    this.messages = const [],
    this.continueWith = const [],
  });

  factory KratosFlow.fromJson(Json json) {
    final ui = _asMap(json['ui']) ?? const {};
    return KratosFlow(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? 'api',
      state: json['state']?.toString(),
      nodes: _asList(ui['nodes'])
          .map((n) => UiNode.fromJson(_asMap(n) ?? const {}))
          .toList(),
      messages: _asList(ui['messages'])
          .map((m) => UiText.fromJson(_asMap(m) ?? const {}))
          .toList(),
      continueWith: parseContinueWith(json['continue_with']),
    );
  }

  final String id;
  final String type;

  /// e.g. `choose_method`, `sent_email`, `passed_challenge`, `success`.
  final String? state;
  final List<UiNode> nodes;

  /// Global (`ui.messages`) messages.
  final List<UiText> messages;
  final List<ContinueWith> continueWith;

  UiNode? node(String name) {
    for (final n in nodes) {
      if (n.name == name) return n;
    }
    return null;
  }

  /// Messages attached to the node(s) named [name].
  List<UiText> messagesFor(String name) => [
    for (final n in nodes)
      if (n.name == name) ...n.messages,
  ];

  /// Node messages for nodes the screen does not render, so they are never
  /// silently dropped.
  List<UiText> unboundMessages(Set<String> boundNames) => [
    for (final n in nodes)
      if (!boundNames.contains(n.name)) ...n.messages,
  ];

  bool get hasErrors =>
      messages.any((m) => m.isError) ||
      nodes.any((n) => n.messages.any((m) => m.isError));
}

class VerifiableAddress {
  const VerifiableAddress({
    required this.value,
    required this.verified,
    required this.via,
  });

  factory VerifiableAddress.fromJson(Json json) => VerifiableAddress(
    value: json['value'] as String? ?? '',
    verified: json['verified'] as bool? ?? false,
    via: json['via'] as String? ?? 'email',
  );

  final String value;
  final bool verified;
  final String via;
}

class KratosIdentity {
  const KratosIdentity({
    required this.id,
    required this.schemaId,
    this.traits = const {},
    this.verifiableAddresses = const [],
  });

  factory KratosIdentity.fromJson(Json json) => KratosIdentity(
    id: json['id'] as String? ?? '',
    schemaId: json['schema_id'] as String? ?? '',
    traits: _asMap(json['traits']) ?? const {},
    verifiableAddresses: _asList(json['verifiable_addresses'])
        .map((a) => VerifiableAddress.fromJson(_asMap(a) ?? const {}))
        .toList(),
  );

  final String id;
  final String schemaId;
  final Json traits;
  final List<VerifiableAddress> verifiableAddresses;

  /// The Kratos login identifier: `traits.login_id` (opaque handle, ADR-0014),
  /// or the legacy `traits.email` of a not-yet-migrated identity. Used as the
  /// identifier for Kratos calls only; **never displayed** (the contact
  /// shown to the user comes from `GET /v1/me`).
  String get loginId =>
      traits['login_id'] as String? ?? traits['email'] as String? ?? '';

  /// The login identifier (email or phone behind the handle) is verified.
  bool get loginVerified =>
      loginId.isNotEmpty &&
      verifiableAddresses.any((a) => a.value == loginId && a.verified);

  /// Kept for existing callers: "email verified" now means the login
  /// identifier is verified.
  bool get emailVerified => loginVerified;
}

class KratosSession {
  const KratosSession({
    required this.id,
    required this.active,
    required this.identity,
    this.expiresAt,
  });

  factory KratosSession.fromJson(Json json) => KratosSession(
    id: json['id'] as String? ?? '',
    active: json['active'] as bool? ?? false,
    expiresAt: DateTime.tryParse(json['expires_at'] as String? ?? ''),
    identity: KratosIdentity.fromJson(_asMap(json['identity']) ?? const {}),
  );

  final String id;
  final bool active;
  final DateTime? expiresAt;
  final KratosIdentity identity;
}

/// Successful native registration / login: the session token is returned
/// once and must go straight to secure storage.
class NativeAuthResult {
  const NativeAuthResult({
    required this.sessionToken,
    required this.session,
    this.continueWith = const [],
  });

  factory NativeAuthResult.fromJson(Json json) {
    final cw = parseContinueWith(json['continue_with']);
    var token = json['session_token'] as String?;
    if (token == null || token.isEmpty) {
      for (final c in cw) {
        if (c is ContinueWithSetSessionToken) token = c.sessionToken;
      }
    }
    final session = _asMap(json['session']);
    return NativeAuthResult(
      sessionToken: token ?? '',
      session: session == null ? null : KratosSession.fromJson(session),
      continueWith: cw,
    );
  }

  final String sessionToken;
  final KratosSession? session;
  final List<ContinueWith> continueWith;

  String? get verificationFlowId {
    for (final c in continueWith) {
      if (c is ContinueWithVerificationUi && c.flowId.isNotEmpty) {
        return c.flowId;
      }
    }
    return null;
  }

  @override
  String toString() => 'NativeAuthResult(sessionToken: <redacted>)';
}
