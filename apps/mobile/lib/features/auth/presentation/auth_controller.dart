import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/auth/data/auth_repository.dart';
import 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';

export 'package:go_ory_auth_mobile/features/auth/domain/auth_state.dart';

/// Owns [AuthState]. Screens call the repository for flow steps and report
/// the outcome here; the router reacts to state changes.
class AuthController extends Notifier<AuthState> {
  Future<void>? _signingOut;

  AuthRepository get _repo => ref.read(authRepositoryProvider);

  /// Bumped on every state change; lets a slow startup check detect that
  /// the user already moved on (e.g. signed in) and drop its result.
  int _generation = 0;

  @override
  set state(AuthState value) {
    _generation++;
    super.state = value;
  }

  @override
  AuthState build() {
    final generation = _generation;
    unawaited(Future.microtask(() => _restore(generation)));
    return const AuthUnknown();
  }

  /// Startup: token → `toSession` → authenticated, or wipe → sign-in.
  /// The result is dropped if the state changed meanwhile (e.g. the user
  /// signed in while the check was in flight).
  Future<void> restore() => _restore(_generation);

  Future<void> _restore(int generation) async {
    AuthState next;
    try {
      final session = await _repo.restoreSession();
      next = session == null ? const Unauthenticated() : Authenticated(session);
    } on AppFailure catch (f) {
      next = Unauthenticated(startupFailure: f);
    }
    if (_generation == generation) state = next;
  }

  void signedIn(KratosSession session) => state = Authenticated(session);

  void registered(RegistrationOutcome outcome) => state = Authenticated(
    outcome.session,
    pendingVerification: !outcome.session.identity.emailVerified,
    verificationFlowId: outcome.verificationFlowId,
  );

  /// e.g. after `403 email_not_verified` from identity-service.
  void requireVerification() {
    final s = state;
    if (s is Authenticated) state = s.copyWith(pendingVerification: true);
  }

  /// After a passed verification challenge.
  Future<void> verificationCompleted() async {
    final s = state;
    if (s is! Authenticated) return;
    try {
      final session = await _repo.refreshSession();
      state = Authenticated(session);
    } on AppFailure {
      state = Authenticated(s.session);
    }
  }

  void skipVerification() {
    final s = state;
    if (s is Authenticated) state = Authenticated(s.session);
  }

  /// `performNativeLogout` + wipe. Single-flight.
  Future<void> signOut() => _signingOut ??= () async {
    try {
      await _repo.logout();
    } finally {
      state = const Unauthenticated();
      _signingOut = null;
    }
  }();

  /// `401` from identity-service: the session is gone server-side; wipe
  /// locally (single-flight) and return to sign-in.
  Future<void> handleUnauthorized() => _signingOut ??= () async {
    try {
      await _repo.forget();
    } finally {
      state = const Unauthenticated();
      _signingOut = null;
    }
  }();
}
