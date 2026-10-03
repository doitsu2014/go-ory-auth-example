import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/auth_controller.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/forgot_password_screen.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/sign_in_screen.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/sign_up_screen.dart';
import 'package:go_ory_auth_mobile/features/auth/presentation/verify_email_screen.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/me.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/edit_profile_screen.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/personal_info_screen.dart';
import 'package:go_ory_auth_mobile/features/profile/presentation/profile_screen.dart';
import 'package:go_ory_auth_mobile/features/settings/presentation/settings_screen.dart';
import 'package:go_router/go_router.dart';

/// Auth-aware redirect (pure, unit-tested).
String? authRedirect(AuthState auth, String location) {
  final isPublic = Routes.public.contains(location);
  switch (auth) {
    case AuthUnknown():
      return location == Routes.splash ? null : Routes.splash;
    case Unauthenticated():
      return isPublic ? null : Routes.signIn;
    case Authenticated(:final pendingVerification):
      if (pendingVerification && location != Routes.verifyEmail) {
        return Routes.verifyEmail;
      }
      if (isPublic || location == Routes.splash) return Routes.profile;
      return null;
  }
}

final routerProvider = Provider<GoRouter>((ref) {
  final refresh = ValueNotifier<AuthState>(ref.read(authControllerProvider));
  ref
    ..listen<AuthState>(
      authControllerProvider,
      (_, next) => refresh.value = next,
    )
    ..onDispose(refresh.dispose);

  final router = GoRouter(
    initialLocation: Routes.splash,
    refreshListenable: refresh,
    redirect: (context, state) =>
        authRedirect(refresh.value, state.matchedLocation),
    routes: [
      GoRoute(path: Routes.splash, builder: (_, _) => const SplashScreen()),
      GoRoute(path: Routes.signIn, builder: (_, _) => const SignInScreen()),
      GoRoute(path: Routes.signUp, builder: (_, _) => const SignUpScreen()),
      GoRoute(
        path: Routes.forgotPassword,
        builder: (_, _) => const ForgotPasswordScreen(),
      ),
      GoRoute(
        path: Routes.verifyEmail,
        builder: (_, _) => const VerifyEmailScreen(),
      ),
      GoRoute(
        path: Routes.profile,
        builder: (_, _) => const ProfileScreen(),
        routes: [
          GoRoute(
            path: 'edit',
            builder: (_, state) => EditProfileScreen(me: state.extra as Me?),
          ),
          GoRoute(
            path: 'personal-info',
            builder: (_, _) => const PersonalInfoScreen(),
          ),
        ],
      ),
      GoRoute(path: Routes.settings, builder: (_, _) => const SettingsScreen()),
    ],
  );
  ref.onDispose(router.dispose);
  return router;
});

class SplashScreen extends StatelessWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context) => const Scaffold(
    body: Center(child: CircularProgressIndicator(key: Key('splash'))),
  );
}
