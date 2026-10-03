import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

/// Android `FLAG_SECURE` toggle (blocks screenshots, screen recording and the
/// recent-apps thumbnail) via a MethodChannel handled in `MainActivity`.
/// Reference-counted so nested sensitive screens keep the flag until the last
/// one leaves. No-op on other platforms.
class SecureScreen {
  SecureScreen({MethodChannel? channel})
    : _channel = channel ?? const MethodChannel(channelName);

  static const channelName = 'go_ory_auth_mobile/secure_screen';
  final MethodChannel _channel;
  int _holders = 0;

  bool get _supported =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.android;

  Future<void> acquire() async {
    if (_holders++ == 0) await _invoke('enable');
  }

  Future<void> release() async {
    if (_holders == 0) return;
    if (--_holders == 0) await _invoke('disable');
  }

  Future<void> _invoke(String method) async {
    if (!_supported) return;
    try {
      await _channel.invokeMethod<void>(method);
    } on PlatformException {
      // Best effort: the screen still works without the flag.
    } on MissingPluginException {
      // e.g. a host without the channel (tests, add-to-app).
    }
  }
}

/// Wraps PII content: holds [SecureScreen] while mounted and covers the
/// content with an opaque layer whenever the app is not in the foreground
/// (iOS app-switcher snapshot, Android fallback).
class PiiShield extends StatefulWidget {
  const PiiShield({required this.secureScreen, required this.child, super.key});

  final SecureScreen secureScreen;
  final Widget child;

  @override
  State<PiiShield> createState() => _PiiShieldState();
}

class _PiiShieldState extends State<PiiShield> with WidgetsBindingObserver {
  late final SecureScreen _secure = widget.secureScreen;
  bool _obscured = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _obscured = _shouldObscure(WidgetsBinding.instance.lifecycleState);
    unawaited(_secure.acquire());
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    unawaited(_secure.release());
    super.dispose();
  }

  static bool _shouldObscure(AppLifecycleState? s) => switch (s) {
    AppLifecycleState.inactive ||
    AppLifecycleState.paused ||
    AppLifecycleState.hidden => true,
    _ => false,
  };

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final obscure = _shouldObscure(state);
    if (obscure != _obscured) setState(() => _obscured = obscure);
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.passthrough,
      children: [
        // Keep the subtree (and its form state) alive under the cover.
        widget.child,
        if (_obscured)
          const Positioned.fill(
            child: ColoredBox(
              key: Key('sensitive.cover'),
              color: Color(0xFF000000),
            ),
          ),
      ],
    );
  }
}
