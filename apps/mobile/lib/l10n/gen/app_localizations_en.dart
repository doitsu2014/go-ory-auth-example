// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'Go Ory Auth';

  @override
  String get welcome => 'Welcome';

  @override
  String get signIn => 'Sign in';

  @override
  String get signUp => 'Sign up';

  @override
  String get signOut => 'Sign out';

  @override
  String get email => 'Email';

  @override
  String get password => 'Password';

  @override
  String get newPassword => 'New password';

  @override
  String get firstName => 'First name';

  @override
  String get lastName => 'Last name';

  @override
  String get fullName => 'Full name';

  @override
  String personName(String first, String last) {
    return '$first $last';
  }

  @override
  String get forgotPassword => 'Forgot password?';

  @override
  String get noAccount => 'No account yet? Sign up';

  @override
  String get haveAccount => 'Already have an account? Sign in';

  @override
  String get verifyEmailTitle => 'Verify your email';

  @override
  String verifyEmailBody(String email) {
    return 'Enter the code we emailed to $email.';
  }

  @override
  String get verificationCode => 'Verification code';

  @override
  String get verify => 'Verify';

  @override
  String get resendCode => 'Resend code';

  @override
  String get skipForNow => 'Later';

  @override
  String get recoveryTitle => 'Reset password';

  @override
  String get recoveryEmailBody =>
      'Enter your account email. If it exists we will send a recovery code.';

  @override
  String get recoveryCodeBody => 'Enter the recovery code we emailed you.';

  @override
  String get recoveryCode => 'Recovery code';

  @override
  String get sendCode => 'Send code';

  @override
  String get continueAction => 'Continue';

  @override
  String get setNewPasswordBody => 'Choose a new password.';

  @override
  String get savePassword => 'Save password';

  @override
  String get profileTitle => 'Profile';

  @override
  String get settingsTitle => 'Security settings';

  @override
  String get displayName => 'Display name';

  @override
  String get locale => 'Language';

  @override
  String get save => 'Save';

  @override
  String get edit => 'Edit';

  @override
  String get verified => 'Verified';

  @override
  String get notVerified => 'Not verified';

  @override
  String get verifyNow => 'Verify now';

  @override
  String get changePassword => 'Change password';

  @override
  String get passwordChanged => 'Password changed.';

  @override
  String get profileSaved => 'Profile saved.';

  @override
  String get networkError =>
      'Can\'t reach the server. Check your connection and try again.';

  @override
  String get retry => 'Retry';

  @override
  String get flowExpired =>
      'This form expired and has been restarted. Please try again.';

  @override
  String get genericError => 'Something went wrong. Please try again.';

  @override
  String get sessionRefreshRequired =>
      'For your security, sign in again before changing your password.';

  @override
  String get emailNotVerified => 'Verify your email address to continue.';

  @override
  String get recoverySessionUnavailable =>
      'The recovery code was accepted, but the server did not start a session. Please sign in with your password or try again later.';

  @override
  String get validationFailed => 'Please check the highlighted fields.';

  @override
  String get fieldTooLong => 'Too long.';

  @override
  String get fieldRequired => 'This field is required.';

  @override
  String get unauthenticated => 'Your session has ended. Please sign in again.';

  @override
  String get kratos1050001 => 'Your changes have been saved.';

  @override
  String get kratos1060001 =>
      'You successfully recovered your account. Please change your password.';

  @override
  String get kratos1060003 =>
      'If an account exists for this address, we sent a recovery code to it.';

  @override
  String get kratos1080002 => 'Your email address is verified.';

  @override
  String get kratos1080003 =>
      'We sent a verification code to your email address.';

  @override
  String get kratos4000002 => 'This field is required.';

  @override
  String get kratos4000006 => 'The email or password is incorrect.';

  @override
  String get kratos4000007 => 'An account with this email already exists.';

  @override
  String get kratos4000010 =>
      'This account is not active yet. Did you verify your email address?';

  @override
  String get kratos4000031 => 'The password is too similar to your email.';

  @override
  String kratos4000032(String minLength) {
    return 'The password must be at least $minLength characters long.';
  }

  @override
  String get kratos4000034 =>
      'This password appeared in a data breach. Please choose another one.';

  @override
  String get kratos4060006 =>
      'The recovery code is invalid or has already been used.';

  @override
  String get kratos4070006 =>
      'The verification code is invalid or has already been used.';

  @override
  String get kratosFlowExpired => 'The request expired. Please try again.';

  @override
  String get reauthBody =>
      'For your security, enter your current password to continue.';

  @override
  String get currentPassword => 'Current password';

  @override
  String get confirm => 'Confirm';

  @override
  String get personalInfoTitle => 'Personal information';

  @override
  String get phoneNumber => 'Phone number';

  @override
  String get phoneNumberHint => '+84901234567';

  @override
  String get dateOfBirth => 'Date of birth';

  @override
  String get addressSection => 'Address';

  @override
  String get addressLine1 => 'Address line 1';

  @override
  String get addressLine2 => 'Address line 2 (optional)';

  @override
  String get city => 'City / province';

  @override
  String get region => 'Region / state (optional)';

  @override
  String get postalCode => 'Postal code (optional)';

  @override
  String get country => 'Country code (e.g. VN)';

  @override
  String get nationalIdSection => 'ID document';

  @override
  String get nationalIdType => 'Document type';

  @override
  String get nationalIdTypeNone => 'None';

  @override
  String get nationalIdTypeCccd => 'Citizen ID (CCCD)';

  @override
  String get nationalIdTypePassport => 'Passport';

  @override
  String get nationalIdTypeOther => 'Other';

  @override
  String get nationalIdNumber => 'Document number';

  @override
  String get notProvided => 'Not provided';

  @override
  String get clear => 'Clear';

  @override
  String get cancel => 'Cancel';

  @override
  String get erase => 'Erase';

  @override
  String get personalInfoSaved => 'Personal information saved.';

  @override
  String get personalInfoErase => 'Erase personal information';

  @override
  String get personalInfoEraseConfirmTitle => 'Erase personal information?';

  @override
  String get personalInfoEraseConfirmBody =>
      'Your phone number, date of birth, address and ID document will be permanently deleted. This cannot be undone.';

  @override
  String get personalInfoErased => 'Personal information erased.';

  @override
  String get dependencyUnavailable =>
      'This service is temporarily unavailable. Please try again shortly.';

  @override
  String get fieldInvalidFormat => 'Invalid format.';

  @override
  String get fieldOutOfRange => 'Value is out of the allowed range.';

  @override
  String get fieldInvalidCharacters =>
      'Contains characters that are not allowed.';

  @override
  String get dateOfBirthOutOfRange =>
      'You must be between 13 and 120 years old.';
}
