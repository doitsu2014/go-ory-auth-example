package platform

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// Redacted replaces sensitive values in logs.
const Redacted = "[REDACTED]"

// sensitiveKeys are attribute keys whose values are never logged (T12).
var sensitiveKeys = map[string]struct{}{
	"authorization": {}, "cookie": {}, "set-cookie": {}, "x-session-token": {},
	"session_token": {}, "token": {}, "password": {}, "code": {}, "recovery_code": {},
	"recovery_link": {}, "link": {}, "secret": {}, "api_key": {}, "email": {},
	"ory_kratos_session": {}, "dsn": {}, "database_url": {},
	// Customer PII and key material (intent 261003-encrypt-user-pii).
	"phone": {}, "phone_number": {}, "date_of_birth": {}, "dob": {}, "address": {}, "national_id": {},
	"plaintext": {}, "dek": {}, "x-vault-token": {}, "pii_local_kek": {}, "pii_local_bidx_key": {},
}

// IsSensitiveKey reports whether a log attribute key must be redacted.
func IsSensitiveKey(k string) bool {
	_, ok := sensitiveKeys[strings.ToLower(k)]
	return ok
}

// RedactingHandler wraps a slog.Handler and redacts sensitive attributes,
// including inside groups.
type RedactingHandler struct{ next slog.Handler }

// NewRedactingHandler wraps next.
func NewRedactingHandler(next slog.Handler) *RedactingHandler { return &RedactingHandler{next: next} }

// Enabled implements slog.Handler.
func (h *RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

// Handle implements slog.Handler.
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(redact(a))
		return true
	})
	return h.next.Handle(ctx, nr)
}

// WithAttrs implements slog.Handler.
func (h *RedactingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(as))
	for i, a := range as {
		out[i] = redact(a)
	}
	return &RedactingHandler{next: h.next.WithAttrs(out)}
}

// WithGroup implements slog.Handler.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name)}
}

func redact(a slog.Attr) slog.Attr {
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		attrs := v.Group()
		out := make([]any, len(attrs))
		for i, ga := range attrs {
			out[i] = redact(ga)
		}
		return slog.Group(a.Key, out...)
	}
	return slog.Attr{Key: a.Key, Value: v}
}

// NewLogger returns a JSON slog logger with redaction.
func NewLogger(w io.Writer, level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(NewRedactingHandler(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})))
}
