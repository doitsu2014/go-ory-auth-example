// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Vietnamese (`vi`).
class AppLocalizationsVi extends AppLocalizations {
  AppLocalizationsVi([String locale = 'vi']) : super(locale);

  @override
  String get appTitle => 'Go Ory Auth';

  @override
  String get welcome => 'Chào mừng';

  @override
  String get signIn => 'Đăng nhập';

  @override
  String get signUp => 'Đăng ký';

  @override
  String get signOut => 'Đăng xuất';

  @override
  String get email => 'Email';

  @override
  String get password => 'Mật khẩu';

  @override
  String get newPassword => 'Mật khẩu mới';

  @override
  String get firstName => 'Tên';

  @override
  String get lastName => 'Họ';

  @override
  String get fullName => 'Họ và tên';

  @override
  String personName(String first, String last) {
    return '$last $first';
  }

  @override
  String get forgotPassword => 'Quên mật khẩu?';

  @override
  String get noAccount => 'Chưa có tài khoản? Đăng ký';

  @override
  String get haveAccount => 'Đã có tài khoản? Đăng nhập';

  @override
  String get verifyEmailTitle => 'Xác minh email';

  @override
  String verifyEmailBody(String email) {
    return 'Nhập mã chúng tôi đã gửi tới $email.';
  }

  @override
  String get verificationCode => 'Mã xác minh';

  @override
  String get verify => 'Xác minh';

  @override
  String get resendCode => 'Gửi lại mã';

  @override
  String get skipForNow => 'Để sau';

  @override
  String get recoveryTitle => 'Đặt lại mật khẩu';

  @override
  String get recoveryEmailBody =>
      'Nhập email tài khoản. Nếu tài khoản tồn tại, chúng tôi sẽ gửi mã khôi phục.';

  @override
  String get recoveryCodeBody =>
      'Nhập mã khôi phục chúng tôi đã gửi qua email.';

  @override
  String get recoveryCode => 'Mã khôi phục';

  @override
  String get sendCode => 'Gửi mã';

  @override
  String get continueAction => 'Tiếp tục';

  @override
  String get setNewPasswordBody => 'Chọn mật khẩu mới.';

  @override
  String get savePassword => 'Lưu mật khẩu';

  @override
  String get profileTitle => 'Hồ sơ';

  @override
  String get settingsTitle => 'Cài đặt bảo mật';

  @override
  String get displayName => 'Tên hiển thị';

  @override
  String get locale => 'Ngôn ngữ';

  @override
  String get save => 'Lưu';

  @override
  String get edit => 'Sửa';

  @override
  String get verified => 'Đã xác minh';

  @override
  String get notVerified => 'Chưa xác minh';

  @override
  String get verifyNow => 'Xác minh ngay';

  @override
  String get changePassword => 'Đổi mật khẩu';

  @override
  String get passwordChanged => 'Đã đổi mật khẩu.';

  @override
  String get profileSaved => 'Đã lưu hồ sơ.';

  @override
  String get networkError =>
      'Không kết nối được máy chủ. Kiểm tra mạng và thử lại.';

  @override
  String get retry => 'Thử lại';

  @override
  String get flowExpired =>
      'Biểu mẫu đã hết hạn và được tạo lại. Vui lòng thử lại.';

  @override
  String get genericError => 'Đã có lỗi xảy ra. Vui lòng thử lại.';

  @override
  String get sessionRefreshRequired =>
      'Để bảo mật, vui lòng đăng nhập lại trước khi đổi mật khẩu.';

  @override
  String get emailNotVerified => 'Vui lòng xác minh email để tiếp tục.';

  @override
  String get recoverySessionUnavailable =>
      'Mã khôi phục hợp lệ nhưng máy chủ không tạo phiên đăng nhập. Vui lòng đăng nhập bằng mật khẩu hoặc thử lại sau.';

  @override
  String get validationFailed => 'Vui lòng kiểm tra các trường được đánh dấu.';

  @override
  String get fieldTooLong => 'Quá dài.';

  @override
  String get fieldRequired => 'Trường này là bắt buộc.';

  @override
  String get unauthenticated =>
      'Phiên đăng nhập đã kết thúc. Vui lòng đăng nhập lại.';

  @override
  String get kratos1050001 => 'Đã lưu thay đổi.';

  @override
  String get kratos1060001 =>
      'Bạn đã khôi phục tài khoản. Vui lòng đổi mật khẩu.';

  @override
  String get kratos1060003 =>
      'Nếu địa chỉ này có tài khoản, chúng tôi đã gửi mã khôi phục tới đó.';

  @override
  String get kratos1080002 => 'Email của bạn đã được xác minh.';

  @override
  String get kratos1080003 => 'Chúng tôi đã gửi mã xác minh tới email của bạn.';

  @override
  String get kratos4000002 => 'Trường này là bắt buộc.';

  @override
  String get kratos4000006 => 'Email hoặc mật khẩu không đúng.';

  @override
  String get kratos4000007 => 'Email này đã được dùng cho một tài khoản khác.';

  @override
  String get kratos4000010 =>
      'Tài khoản chưa được kích hoạt. Bạn đã xác minh email chưa?';

  @override
  String get kratos4000031 => 'Mật khẩu quá giống với email.';

  @override
  String kratos4000032(String minLength) {
    return 'Mật khẩu phải có ít nhất $minLength ký tự.';
  }

  @override
  String get kratos4000034 =>
      'Mật khẩu này đã bị lộ trong một vụ rò rỉ dữ liệu. Vui lòng chọn mật khẩu khác.';

  @override
  String get kratos4060006 => 'Mã khôi phục không hợp lệ hoặc đã được sử dụng.';

  @override
  String get kratos4070006 => 'Mã xác minh không hợp lệ hoặc đã được sử dụng.';

  @override
  String get kratosFlowExpired => 'Yêu cầu đã hết hạn. Vui lòng thử lại.';

  @override
  String get reauthBody => 'Để bảo mật, nhập mật khẩu hiện tại để tiếp tục.';

  @override
  String get currentPassword => 'Mật khẩu hiện tại';

  @override
  String get confirm => 'Xác nhận';

  @override
  String get personalInfoTitle => 'Thông tin cá nhân';

  @override
  String get phoneNumber => 'Số điện thoại';

  @override
  String get phoneNumberHint => '+84901234567';

  @override
  String get dateOfBirth => 'Ngày sinh';

  @override
  String get addressSection => 'Địa chỉ';

  @override
  String get addressLine1 => 'Địa chỉ (dòng 1)';

  @override
  String get addressLine2 => 'Địa chỉ (dòng 2, không bắt buộc)';

  @override
  String get city => 'Thành phố / tỉnh';

  @override
  String get region => 'Vùng / bang (không bắt buộc)';

  @override
  String get postalCode => 'Mã bưu chính (không bắt buộc)';

  @override
  String get country => 'Mã quốc gia (ví dụ VN)';

  @override
  String get nationalIdSection => 'Giấy tờ tùy thân';

  @override
  String get nationalIdType => 'Loại giấy tờ';

  @override
  String get nationalIdTypeNone => 'Không có';

  @override
  String get nationalIdTypeCccd => 'Căn cước công dân (CCCD)';

  @override
  String get nationalIdTypePassport => 'Hộ chiếu';

  @override
  String get nationalIdTypeOther => 'Khác';

  @override
  String get nationalIdNumber => 'Số giấy tờ';

  @override
  String get notProvided => 'Chưa cung cấp';

  @override
  String get clear => 'Bỏ chọn';

  @override
  String get cancel => 'Hủy';

  @override
  String get erase => 'Xóa';

  @override
  String get personalInfoSaved => 'Đã lưu thông tin cá nhân.';

  @override
  String get personalInfoErase => 'Xóa thông tin cá nhân';

  @override
  String get personalInfoEraseConfirmTitle => 'Xóa thông tin cá nhân?';

  @override
  String get personalInfoEraseConfirmBody =>
      'Số điện thoại, ngày sinh, địa chỉ và giấy tờ tùy thân của bạn sẽ bị xóa vĩnh viễn. Không thể hoàn tác.';

  @override
  String get personalInfoErased => 'Đã xóa thông tin cá nhân.';

  @override
  String get dependencyUnavailable =>
      'Dịch vụ tạm thời không khả dụng. Vui lòng thử lại sau ít phút.';

  @override
  String get fieldInvalidFormat => 'Định dạng không hợp lệ.';

  @override
  String get fieldOutOfRange => 'Giá trị nằm ngoài phạm vi cho phép.';

  @override
  String get fieldInvalidCharacters => 'Chứa ký tự không được phép.';

  @override
  String get dateOfBirthOutOfRange => 'Bạn phải từ 13 đến 120 tuổi.';
}
