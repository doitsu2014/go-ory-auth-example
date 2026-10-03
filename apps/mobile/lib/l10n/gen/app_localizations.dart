import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_vi.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'gen/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
    : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations)!;
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
        delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
      ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('vi'),
  ];

  /// No description provided for @appTitle.
  ///
  /// In en, this message translates to:
  /// **'Go Ory Auth'**
  String get appTitle;

  /// No description provided for @welcome.
  ///
  /// In en, this message translates to:
  /// **'Welcome'**
  String get welcome;

  /// No description provided for @signIn.
  ///
  /// In en, this message translates to:
  /// **'Sign in'**
  String get signIn;

  /// No description provided for @signUp.
  ///
  /// In en, this message translates to:
  /// **'Sign up'**
  String get signUp;

  /// No description provided for @signOut.
  ///
  /// In en, this message translates to:
  /// **'Sign out'**
  String get signOut;

  /// No description provided for @email.
  ///
  /// In en, this message translates to:
  /// **'Email'**
  String get email;

  /// No description provided for @password.
  ///
  /// In en, this message translates to:
  /// **'Password'**
  String get password;

  /// No description provided for @newPassword.
  ///
  /// In en, this message translates to:
  /// **'New password'**
  String get newPassword;

  /// No description provided for @firstName.
  ///
  /// In en, this message translates to:
  /// **'First name'**
  String get firstName;

  /// No description provided for @lastName.
  ///
  /// In en, this message translates to:
  /// **'Last name'**
  String get lastName;

  /// No description provided for @fullName.
  ///
  /// In en, this message translates to:
  /// **'Full name'**
  String get fullName;

  /// Display order of a person's name parts.
  ///
  /// In en, this message translates to:
  /// **'{first} {last}'**
  String personName(String first, String last);

  /// No description provided for @forgotPassword.
  ///
  /// In en, this message translates to:
  /// **'Forgot password?'**
  String get forgotPassword;

  /// No description provided for @noAccount.
  ///
  /// In en, this message translates to:
  /// **'No account yet? Sign up'**
  String get noAccount;

  /// No description provided for @haveAccount.
  ///
  /// In en, this message translates to:
  /// **'Already have an account? Sign in'**
  String get haveAccount;

  /// No description provided for @verifyEmailTitle.
  ///
  /// In en, this message translates to:
  /// **'Verify your email'**
  String get verifyEmailTitle;

  /// No description provided for @verifyEmailBody.
  ///
  /// In en, this message translates to:
  /// **'Enter the code we emailed to {email}.'**
  String verifyEmailBody(String email);

  /// No description provided for @verificationCode.
  ///
  /// In en, this message translates to:
  /// **'Verification code'**
  String get verificationCode;

  /// No description provided for @verify.
  ///
  /// In en, this message translates to:
  /// **'Verify'**
  String get verify;

  /// No description provided for @resendCode.
  ///
  /// In en, this message translates to:
  /// **'Resend code'**
  String get resendCode;

  /// No description provided for @skipForNow.
  ///
  /// In en, this message translates to:
  /// **'Later'**
  String get skipForNow;

  /// No description provided for @recoveryTitle.
  ///
  /// In en, this message translates to:
  /// **'Reset password'**
  String get recoveryTitle;

  /// No description provided for @recoveryEmailBody.
  ///
  /// In en, this message translates to:
  /// **'Enter your account email. If it exists we will send a recovery code.'**
  String get recoveryEmailBody;

  /// No description provided for @recoveryCodeBody.
  ///
  /// In en, this message translates to:
  /// **'Enter the recovery code we emailed you.'**
  String get recoveryCodeBody;

  /// No description provided for @recoveryCode.
  ///
  /// In en, this message translates to:
  /// **'Recovery code'**
  String get recoveryCode;

  /// No description provided for @sendCode.
  ///
  /// In en, this message translates to:
  /// **'Send code'**
  String get sendCode;

  /// No description provided for @continueAction.
  ///
  /// In en, this message translates to:
  /// **'Continue'**
  String get continueAction;

  /// No description provided for @setNewPasswordBody.
  ///
  /// In en, this message translates to:
  /// **'Choose a new password.'**
  String get setNewPasswordBody;

  /// No description provided for @savePassword.
  ///
  /// In en, this message translates to:
  /// **'Save password'**
  String get savePassword;

  /// No description provided for @profileTitle.
  ///
  /// In en, this message translates to:
  /// **'Profile'**
  String get profileTitle;

  /// No description provided for @settingsTitle.
  ///
  /// In en, this message translates to:
  /// **'Security settings'**
  String get settingsTitle;

  /// No description provided for @displayName.
  ///
  /// In en, this message translates to:
  /// **'Display name'**
  String get displayName;

  /// No description provided for @locale.
  ///
  /// In en, this message translates to:
  /// **'Language'**
  String get locale;

  /// No description provided for @save.
  ///
  /// In en, this message translates to:
  /// **'Save'**
  String get save;

  /// No description provided for @edit.
  ///
  /// In en, this message translates to:
  /// **'Edit'**
  String get edit;

  /// No description provided for @verified.
  ///
  /// In en, this message translates to:
  /// **'Verified'**
  String get verified;

  /// No description provided for @notVerified.
  ///
  /// In en, this message translates to:
  /// **'Not verified'**
  String get notVerified;

  /// No description provided for @verifyNow.
  ///
  /// In en, this message translates to:
  /// **'Verify now'**
  String get verifyNow;

  /// No description provided for @changePassword.
  ///
  /// In en, this message translates to:
  /// **'Change password'**
  String get changePassword;

  /// No description provided for @passwordChanged.
  ///
  /// In en, this message translates to:
  /// **'Password changed.'**
  String get passwordChanged;

  /// No description provided for @profileSaved.
  ///
  /// In en, this message translates to:
  /// **'Profile saved.'**
  String get profileSaved;

  /// No description provided for @networkError.
  ///
  /// In en, this message translates to:
  /// **'Can\'t reach the server. Check your connection and try again.'**
  String get networkError;

  /// No description provided for @retry.
  ///
  /// In en, this message translates to:
  /// **'Retry'**
  String get retry;

  /// No description provided for @flowExpired.
  ///
  /// In en, this message translates to:
  /// **'This form expired and has been restarted. Please try again.'**
  String get flowExpired;

  /// No description provided for @genericError.
  ///
  /// In en, this message translates to:
  /// **'Something went wrong. Please try again.'**
  String get genericError;

  /// No description provided for @sessionRefreshRequired.
  ///
  /// In en, this message translates to:
  /// **'For your security, sign in again before changing your password.'**
  String get sessionRefreshRequired;

  /// No description provided for @emailNotVerified.
  ///
  /// In en, this message translates to:
  /// **'Verify your email address to continue.'**
  String get emailNotVerified;

  /// No description provided for @recoverySessionUnavailable.
  ///
  /// In en, this message translates to:
  /// **'The recovery code was accepted, but the server did not start a session. Please sign in with your password or try again later.'**
  String get recoverySessionUnavailable;

  /// No description provided for @validationFailed.
  ///
  /// In en, this message translates to:
  /// **'Please check the highlighted fields.'**
  String get validationFailed;

  /// No description provided for @fieldTooLong.
  ///
  /// In en, this message translates to:
  /// **'Too long.'**
  String get fieldTooLong;

  /// No description provided for @fieldRequired.
  ///
  /// In en, this message translates to:
  /// **'This field is required.'**
  String get fieldRequired;

  /// No description provided for @unauthenticated.
  ///
  /// In en, this message translates to:
  /// **'Your session has ended. Please sign in again.'**
  String get unauthenticated;

  /// No description provided for @kratos1050001.
  ///
  /// In en, this message translates to:
  /// **'Your changes have been saved.'**
  String get kratos1050001;

  /// No description provided for @kratos1060001.
  ///
  /// In en, this message translates to:
  /// **'You successfully recovered your account. Please change your password.'**
  String get kratos1060001;

  /// No description provided for @kratos1060003.
  ///
  /// In en, this message translates to:
  /// **'If an account exists for this address, we sent a recovery code to it.'**
  String get kratos1060003;

  /// No description provided for @kratos1080002.
  ///
  /// In en, this message translates to:
  /// **'Your email address is verified.'**
  String get kratos1080002;

  /// No description provided for @kratos1080003.
  ///
  /// In en, this message translates to:
  /// **'We sent a verification code to your email address.'**
  String get kratos1080003;

  /// No description provided for @kratos4000002.
  ///
  /// In en, this message translates to:
  /// **'This field is required.'**
  String get kratos4000002;

  /// No description provided for @kratos4000006.
  ///
  /// In en, this message translates to:
  /// **'The email or password is incorrect.'**
  String get kratos4000006;

  /// No description provided for @kratos4000007.
  ///
  /// In en, this message translates to:
  /// **'An account with this email already exists.'**
  String get kratos4000007;

  /// No description provided for @kratos4000010.
  ///
  /// In en, this message translates to:
  /// **'This account is not active yet. Did you verify your email address?'**
  String get kratos4000010;

  /// No description provided for @kratos4000031.
  ///
  /// In en, this message translates to:
  /// **'The password is too similar to your email.'**
  String get kratos4000031;

  /// No description provided for @kratos4000032.
  ///
  /// In en, this message translates to:
  /// **'The password must be at least {minLength} characters long.'**
  String kratos4000032(String minLength);

  /// No description provided for @kratos4000034.
  ///
  /// In en, this message translates to:
  /// **'This password appeared in a data breach. Please choose another one.'**
  String get kratos4000034;

  /// No description provided for @kratos4060006.
  ///
  /// In en, this message translates to:
  /// **'The recovery code is invalid or has already been used.'**
  String get kratos4060006;

  /// No description provided for @kratos4070006.
  ///
  /// In en, this message translates to:
  /// **'The verification code is invalid or has already been used.'**
  String get kratos4070006;

  /// No description provided for @kratosFlowExpired.
  ///
  /// In en, this message translates to:
  /// **'The request expired. Please try again.'**
  String get kratosFlowExpired;

  /// No description provided for @reauthBody.
  ///
  /// In en, this message translates to:
  /// **'For your security, enter your current password to continue.'**
  String get reauthBody;

  /// No description provided for @currentPassword.
  ///
  /// In en, this message translates to:
  /// **'Current password'**
  String get currentPassword;

  /// No description provided for @confirm.
  ///
  /// In en, this message translates to:
  /// **'Confirm'**
  String get confirm;

  /// No description provided for @personalInfoTitle.
  ///
  /// In en, this message translates to:
  /// **'Personal information'**
  String get personalInfoTitle;

  /// No description provided for @phoneNumber.
  ///
  /// In en, this message translates to:
  /// **'Phone number'**
  String get phoneNumber;

  /// No description provided for @phoneNumberHint.
  ///
  /// In en, this message translates to:
  /// **'+84901234567'**
  String get phoneNumberHint;

  /// No description provided for @dateOfBirth.
  ///
  /// In en, this message translates to:
  /// **'Date of birth'**
  String get dateOfBirth;

  /// No description provided for @addressSection.
  ///
  /// In en, this message translates to:
  /// **'Address'**
  String get addressSection;

  /// No description provided for @addressLine1.
  ///
  /// In en, this message translates to:
  /// **'Address line 1'**
  String get addressLine1;

  /// No description provided for @addressLine2.
  ///
  /// In en, this message translates to:
  /// **'Address line 2 (optional)'**
  String get addressLine2;

  /// No description provided for @city.
  ///
  /// In en, this message translates to:
  /// **'City / province'**
  String get city;

  /// No description provided for @region.
  ///
  /// In en, this message translates to:
  /// **'Region / state (optional)'**
  String get region;

  /// No description provided for @postalCode.
  ///
  /// In en, this message translates to:
  /// **'Postal code (optional)'**
  String get postalCode;

  /// No description provided for @country.
  ///
  /// In en, this message translates to:
  /// **'Country code (e.g. VN)'**
  String get country;

  /// No description provided for @nationalIdSection.
  ///
  /// In en, this message translates to:
  /// **'ID document'**
  String get nationalIdSection;

  /// No description provided for @nationalIdType.
  ///
  /// In en, this message translates to:
  /// **'Document type'**
  String get nationalIdType;

  /// No description provided for @nationalIdTypeNone.
  ///
  /// In en, this message translates to:
  /// **'None'**
  String get nationalIdTypeNone;

  /// No description provided for @nationalIdTypeCccd.
  ///
  /// In en, this message translates to:
  /// **'Citizen ID (CCCD)'**
  String get nationalIdTypeCccd;

  /// No description provided for @nationalIdTypePassport.
  ///
  /// In en, this message translates to:
  /// **'Passport'**
  String get nationalIdTypePassport;

  /// No description provided for @nationalIdTypeOther.
  ///
  /// In en, this message translates to:
  /// **'Other'**
  String get nationalIdTypeOther;

  /// No description provided for @nationalIdNumber.
  ///
  /// In en, this message translates to:
  /// **'Document number'**
  String get nationalIdNumber;

  /// No description provided for @notProvided.
  ///
  /// In en, this message translates to:
  /// **'Not provided'**
  String get notProvided;

  /// No description provided for @clear.
  ///
  /// In en, this message translates to:
  /// **'Clear'**
  String get clear;

  /// No description provided for @cancel.
  ///
  /// In en, this message translates to:
  /// **'Cancel'**
  String get cancel;

  /// No description provided for @erase.
  ///
  /// In en, this message translates to:
  /// **'Erase'**
  String get erase;

  /// No description provided for @personalInfoSaved.
  ///
  /// In en, this message translates to:
  /// **'Personal information saved.'**
  String get personalInfoSaved;

  /// No description provided for @personalInfoErase.
  ///
  /// In en, this message translates to:
  /// **'Erase personal information'**
  String get personalInfoErase;

  /// No description provided for @personalInfoEraseConfirmTitle.
  ///
  /// In en, this message translates to:
  /// **'Erase personal information?'**
  String get personalInfoEraseConfirmTitle;

  /// No description provided for @personalInfoEraseConfirmBody.
  ///
  /// In en, this message translates to:
  /// **'Your phone number, date of birth, address and ID document will be permanently deleted. This cannot be undone.'**
  String get personalInfoEraseConfirmBody;

  /// No description provided for @personalInfoErased.
  ///
  /// In en, this message translates to:
  /// **'Personal information erased.'**
  String get personalInfoErased;

  /// No description provided for @dependencyUnavailable.
  ///
  /// In en, this message translates to:
  /// **'This service is temporarily unavailable. Please try again shortly.'**
  String get dependencyUnavailable;

  /// No description provided for @fieldInvalidFormat.
  ///
  /// In en, this message translates to:
  /// **'Invalid format.'**
  String get fieldInvalidFormat;

  /// No description provided for @fieldOutOfRange.
  ///
  /// In en, this message translates to:
  /// **'Value is out of the allowed range.'**
  String get fieldOutOfRange;

  /// No description provided for @fieldInvalidCharacters.
  ///
  /// In en, this message translates to:
  /// **'Contains characters that are not allowed.'**
  String get fieldInvalidCharacters;

  /// No description provided for @dateOfBirthOutOfRange.
  ///
  /// In en, this message translates to:
  /// **'You must be between 13 and 120 years old.'**
  String get dateOfBirthOutOfRange;
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'vi'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return AppLocalizationsEn();
    case 'vi':
      return AppLocalizationsVi();
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
