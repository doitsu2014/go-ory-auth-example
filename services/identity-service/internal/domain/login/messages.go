package login

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// TemplateType is a Kratos courier template type we deliver (A4 allow-list).
type TemplateType string

// Delivered template types. Kratos only sends the "invalid" variants when
// notify_unknown_recipients is on (it is off), so they are not delivered.
const (
	TemplateVerificationCode TemplateType = "verification_code_valid"
	TemplateRecoveryCode     TemplateType = "recovery_code_valid"
)

// ParseTemplateType validates a template type.
func ParseTemplateType(s string) (TemplateType, bool) {
	switch TemplateType(s) {
	case TemplateVerificationCode, TemplateRecoveryCode:
		return TemplateType(s), true
	}
	return "", false
}

// ErrUnsupportedTemplate is returned by Render for an unknown template type.
var ErrUnsupportedTemplate = errors.New("login message: unsupported template")

var codeRe = regexp.MustCompile(`^[0-9]{6}$`)

// ValidCode reports whether code is a 6-digit Kratos one-time code.
func ValidCode(code string) bool { return codeRe.MatchString(code) }

// Message is a rendered message: Subject and Text for email, SMS for phone
// (ASCII only, ≤ 160 characters so it fits one GSM-7 segment).
type Message struct {
	Subject string
	Text    string
	SMS     string
}

// Locales.
const (
	LocaleVI = "vi"
	LocaleEN = "en"
)

// NormalizeLocale maps a profile locale ("vi-VN", "en", …) to a supported
// message locale; Vietnamese is the default.
func NormalizeLocale(l string) string {
	if strings.HasPrefix(strings.ToLower(l), "en") {
		return LocaleEN
	}
	return LocaleVI
}

type texts struct{ subject, body, sms string }

var templates = map[TemplateType]map[string]texts{
	TemplateVerificationCode: {
		LocaleVI: {
			subject: "Mã xác minh của bạn: %[1]s",
			body:    "Mã xác minh tài khoản go-ory-auth-example của bạn là:\n\n%[1]s\n\nMã có hiệu lực trong %[2]d phút. Nếu bạn không yêu cầu, hãy bỏ qua tin này.\n",
			sms:     "go-ory-auth-example: ma xac minh cua ban la %[1]s (hieu luc %[2]d phut). Khong chia se ma nay.",
		},
		LocaleEN: {
			subject: "Your verification code: %[1]s",
			body:    "Your go-ory-auth-example verification code is:\n\n%[1]s\n\nIt expires in %[2]d minutes. If you did not request it, ignore this message.\n",
			sms:     "go-ory-auth-example: your verification code is %[1]s (valid %[2]d min). Do not share it.",
		},
	},
	TemplateRecoveryCode: {
		LocaleVI: {
			subject: "Mã khôi phục tài khoản: %[1]s",
			body:    "Mã khôi phục tài khoản go-ory-auth-example của bạn là:\n\n%[1]s\n\nMã có hiệu lực trong %[2]d phút. Nếu bạn không yêu cầu đặt lại mật khẩu, hãy bỏ qua tin này; mật khẩu của bạn không thay đổi.\n",
			sms:     "go-ory-auth-example: ma khoi phuc tai khoan la %[1]s (hieu luc %[2]d phut). Khong chia se ma nay.",
		},
		LocaleEN: {
			subject: "Your account recovery code: %[1]s",
			body:    "Your go-ory-auth-example account recovery code is:\n\n%[1]s\n\nIt expires in %[2]d minutes. If you did not ask to reset your password, ignore this message; your password is unchanged.\n",
			sms:     "go-ory-auth-example: your account recovery code is %[1]s (valid %[2]d min). Do not share it.",
		},
	},
}

// DefaultExpiryMinutes is used when Kratos does not send expires_in_minutes.
const DefaultExpiryMinutes = 15

// Render renders a message. The code must be a valid 6-digit code; it is the
// only dynamic content, so no address or pseudonym can appear in a message.
func Render(t TemplateType, locale, code string, expiresInMinutes int) (Message, error) {
	byLocale, ok := templates[t]
	if !ok {
		return Message{}, ErrUnsupportedTemplate
	}
	if !ValidCode(code) {
		return Message{}, errors.New("login message: invalid code")
	}
	if expiresInMinutes <= 0 || expiresInMinutes > 1440 {
		expiresInMinutes = DefaultExpiryMinutes
	}
	tx := byLocale[NormalizeLocale(locale)]
	return Message{
		Subject: fmt.Sprintf(tx.subject, code),
		Text:    fmt.Sprintf(tx.body, code, expiresInMinutes),
		SMS:     fmt.Sprintf(tx.sms, code, expiresInMinutes),
	}, nil
}
