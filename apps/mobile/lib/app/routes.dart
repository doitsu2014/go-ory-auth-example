abstract final class Routes {
  static const splash = '/splash';
  static const signIn = '/sign-in';
  static const signUp = '/sign-up';
  static const forgotPassword = '/forgot-password';
  static const verifyEmail = '/verify-email';
  static const profile = '/profile';
  static const editProfile = '/profile/edit';
  static const personalInfo = '/profile/personal-info';
  static const settings = '/settings';

  /// Reachable without a session.
  static const Set<String> public = {signIn, signUp, forgotPassword};
}
