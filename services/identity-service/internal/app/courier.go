package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
)

// CourierMessage is a Kratos courier delivery request (api-contract §10).
// Only ids and the one-time code are taken from Kratos; Kratos-rendered
// subjects, bodies and URLs are never used (A4).
type CourierMessage struct {
	Recipient        string
	TemplateType     string
	IdentityID       uuid.UUID
	Code             string
	ExpiresInMinutes int
}

// CourierOutcome is what happened to a message.
type CourierOutcome string

// Outcomes. Dropped messages are acknowledged so Kratos does not retry
// (permanent failures, A11).
const (
	CourierSent      CourierOutcome = "sent"
	CourierDuplicate CourierOutcome = "duplicate"
	CourierDropped   CourierOutcome = "dropped"
)

// Drop reasons (metric label courier_dropped_total{reason}).
const (
	DropBadPayload          = "bad_payload"
	DropUnsupportedTemplate = "unsupported_template"
	DropUnresolved          = "unresolved"
	DropUnbindable          = "unbindable"
	DropPlaintextRefused    = "plaintext_refused"
	DropOverQuota           = "over_quota"
	DropChannelDisabled     = "channel_disabled"
	DropProviderRejected    = "provider_rejected"
)

// Channels.
const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
)

// CourierResult describes a handled message (never the address).
type CourierResult struct {
	Outcome CourierOutcome
	Reason  string
	Channel string
}

// Default SMS budgets per 24 h (A3); configurable.
const (
	DefaultSMSDailyBudget        = 1000
	DefaultSMSCountryDailyBudget = 1000
)

// dedupeStale is when a pending reservation is considered abandoned.
const dedupeStale = 2 * time.Minute

// CourierDispatcher delivers Kratos verification and recovery codes to the
// real address behind a pseudonym (PLI-FR-05/06): email via the Mailer,
// phone via the SMS sender. It is idempotent per (template, recipient, code)
// so Kratos retries never send twice (PLI-NFR-10, A9).
type CourierDispatcher struct {
	Logins     *LoginIdentifierService
	Identities IdentityAdmin
	Profiles   ProfileRepo
	Mailer     Mailer
	// SMS is nil when no SMS channel is configured.
	SMS    SMSSender
	Dedupe CourierDispatchRepo
	// DedupeKey keys the de-duplication hash (COURIER_DEDUPE_SECRET), so a
	// database reader cannot brute-force live codes from it.
	DedupeKey []byte
	// RecipientRules caps deliveries per recipient (A3). Quotas and SMS
	// budgets are counted in the database, so they survive restarts and are
	// shared by replicas (SEC-C02). Concurrent sends can overshoot a budget
	// by at most the number of in-flight messages.
	RecipientRules        []RateRule
	SMSDailyBudget        int
	SMSCountryDailyBudget int
	Phase                 MigrationPhase
	Clock                 Clock
	Log                   *slog.Logger
}

// target is a resolved delivery target.
type target struct {
	channel string
	address string
	country string
}

// Dispatch handles one courier message. A non-nil error is transient
// (key manager, database, Kratos, mail or SMS provider): the caller answers
// 5xx and Kratos retries.
func (d *CourierDispatcher) Dispatch(ctx context.Context, m CourierMessage) (CourierResult, error) {
	tt, ok := login.ParseTemplateType(m.TemplateType)
	if !ok {
		return d.drop(ctx, m, nil, DropUnsupportedTemplate), nil
	}
	if !login.ValidCode(m.Code) || m.IdentityID == uuid.Nil || m.Recipient == "" || len(m.Recipient) > login.MaxRawLength {
		return d.drop(ctx, m, nil, DropBadPayload), nil
	}
	m.Recipient = strings.ToLower(m.Recipient)
	key := d.dedupeKey(tt, m.Recipient, m.Code)
	reserved, err := d.Dedupe.Reserve(ctx, key, d.now().Add(-dedupeStale))
	if err != nil {
		return CourierResult{}, fmt.Errorf("reserve courier dispatch: %w", err)
	}
	if !reserved {
		return CourierResult{Outcome: CourierDuplicate}, nil
	}
	res, delivery, err := d.deliver(ctx, tt, m)
	if err != nil {
		if rerr := d.Dedupe.Release(context.WithoutCancel(ctx), key); rerr != nil {
			d.log().ErrorContext(ctx, "courier_release_failed", "error", rerr.Error())
		}
		return CourierResult{}, err
	}
	// Sent and permanently dropped messages are both final; only deliveries
	// count against the quotas.
	if err := d.Dedupe.MarkSent(context.WithoutCancel(ctx), key, delivery); err != nil {
		d.log().ErrorContext(ctx, "courier_mark_sent_failed", "error", err.Error())
	}
	return res, nil
}

func (d *CourierDispatcher) deliver(ctx context.Context, tt login.TemplateType, m CourierMessage) (CourierResult, *Delivery, error) {
	ident, err := d.Identities.GetIdentity(ctx, m.IdentityID)
	if errors.Is(err, ErrNotFound) {
		return d.drop(ctx, m, nil, DropUnbindable), nil, nil
	}
	if err != nil {
		return CourierResult{}, nil, fmt.Errorf("get identity: %w", err)
	}
	var tg target
	var reason string
	if p, ok := login.ParsePseudonym(m.Recipient); ok {
		tg, reason, err = d.pseudonymTarget(ctx, ident, p, m.Recipient)
	} else {
		tg, reason = d.plaintextTarget(ident, m.Recipient)
	}
	if err != nil {
		return CourierResult{}, nil, err
	}
	if reason != "" {
		return d.drop(ctx, m, &ident, reason), nil, nil
	}
	if tg.channel == ChannelSMS && d.SMS == nil {
		return d.drop(ctx, m, &ident, DropChannelDisabled), nil, nil
	}
	delivery := &Delivery{Channel: tg.channel, Country: tg.country, RecipientKey: d.recipientKey(m.Recipient)}
	over, err := d.overQuota(ctx, delivery)
	if err != nil {
		return CourierResult{}, nil, err
	}
	if over {
		return d.drop(ctx, m, &ident, DropOverQuota), nil, nil
	}
	msg, err := login.Render(tt, d.locale(ctx, ident), m.Code, m.ExpiresInMinutes)
	if err != nil {
		return d.drop(ctx, m, &ident, DropBadPayload), nil, nil
	}
	switch tg.channel {
	case ChannelSMS:
		err = d.SMS.SendSMS(ctx, tg.address, msg.SMS)
	default:
		err = d.Mailer.SendLoginMessage(ctx, tg.address, msg)
	}
	if errors.Is(err, ErrPermanentDelivery) {
		// Retrying cannot succeed: acknowledge so Kratos stops (A11).
		return d.drop(ctx, m, &ident, DropProviderRejected), nil, nil
	}
	if err != nil {
		return CourierResult{}, nil, fmt.Errorf("%w: send %s: %v", ErrDependencyUnavailable, tg.channel, err)
	}
	d.log().InfoContext(ctx, "courier_sent", "channel", tg.channel, "template", string(tt), "identity_id", ident.ID.String())
	return CourierResult{Outcome: CourierSent, Channel: tg.channel}, delivery, nil
}

// overQuota checks the per-recipient quota and, for SMS, the global and
// per-country daily budgets against delivered messages in the database.
func (d *CourierDispatcher) overQuota(ctx context.Context, dl *Delivery) (bool, error) {
	now := d.now()
	for _, r := range d.RecipientRules {
		n, err := d.Dedupe.CountDeliveriesTo(ctx, dl.RecipientKey, now.Add(-r.Window))
		if err != nil {
			return false, fmt.Errorf("count deliveries: %w", err)
		}
		if n >= int64(r.Limit) {
			return true, nil
		}
	}
	if dl.Channel != ChannelSMS {
		return false, nil
	}
	day := now.Add(-24 * time.Hour)
	for _, b := range []struct {
		country string
		limit   int
	}{{"", d.SMSDailyBudget}, {dl.Country, d.SMSCountryDailyBudget}} {
		if b.limit <= 0 {
			continue
		}
		n, err := d.Dedupe.CountSMS(ctx, b.country, day)
		if err != nil {
			return false, fmt.Errorf("count sms: %w", err)
		}
		if n >= int64(b.limit) {
			d.log().ErrorContext(ctx, "sms_budget_exhausted", "country", b.country)
			return true, nil
		}
	}
	return false, nil
}

// recipientKey is a keyed hash of the recipient (no plaintext in the table).
func (d *CourierDispatcher) recipientKey(recipient string) [32]byte {
	h := hmac.New(sha256.New, d.DedupeKey)
	h.Write([]byte("recipient\x00"))
	h.Write([]byte(recipient))
	var k [32]byte
	copy(k[:], h.Sum(nil))
	return k
}

// pseudonymTarget resolves a customer pseudonym. The identity named in the
// payload must carry this pseudonym in Kratos (authoritative, DD-11); the
// vault entry is bound to it if it was not yet (A4).
func (d *CourierDispatcher) pseudonymTarget(ctx context.Context, ident identity.Identity, p login.Pseudonym, recipient string) (target, string, error) {
	if !ident.IsCustomer() || !strings.EqualFold(ident.LoginID, recipient) {
		return target{}, DropUnbindable, nil
	}
	rec, err := d.Logins.Logins.Get(ctx, p)
	if errors.Is(err, ErrNotFound) {
		return target{}, DropUnresolved, nil
	}
	if err != nil {
		return target{}, "", fmt.Errorf("get login: %w", err)
	}
	if rec.IdentityID == nil || *rec.IdentityID != ident.ID {
		rec, err = d.Logins.bind(ctx, ident.ID, p)
		if errors.Is(err, ErrDataIntegrity) {
			return target{}, DropUnbindable, nil
		}
		if err != nil {
			return target{}, "", err
		}
	}
	id, err := d.Logins.open(ctx, rec)
	if errors.Is(err, ErrDataIntegrity) {
		return target{}, DropUnresolved, nil
	}
	if err != nil {
		return target{}, "", err
	}
	if id.Kind() == login.KindPhone {
		return target{channel: ChannelSMS, address: id.Value(), country: pii.CallingCode(id.Value())}, "", nil
	}
	return target{channel: ChannelEmail, address: id.Value()}, "", nil
}

// plaintextTarget allows a plaintext recipient only for an admin's own
// address, or a legacy customer's own email during the migration (A4).
func (d *CourierDispatcher) plaintextTarget(ident identity.Identity, recipient string) (target, string) {
	if !strings.EqualFold(ident.Email, recipient) {
		return target{}, DropPlaintextRefused
	}
	switch {
	case ident.SchemaID == string(identity.KindAdmin):
	case ident.IsCustomer() && d.Phase == PhaseTransition && !login.IsPseudonym(ident.LoginID):
	default:
		return target{}, DropPlaintextRefused
	}
	return target{channel: ChannelEmail, address: recipient}, ""
}

func (d *CourierDispatcher) locale(ctx context.Context, ident identity.Identity) string {
	if d.Profiles == nil {
		return login.LocaleVI
	}
	pr, err := d.Profiles.Get(ctx, ident.ID)
	if err != nil {
		return login.LocaleVI
	}
	return login.NormalizeLocale(pr.Locale)
}

func (d *CourierDispatcher) drop(ctx context.Context, m CourierMessage, ident *identity.Identity, reason string) CourierResult {
	attrs := []any{"reason", reason, "template", m.TemplateType}
	if ident != nil {
		attrs = append(attrs, "identity_id", ident.ID.String())
	}
	d.log().WarnContext(ctx, "courier_dropped", attrs...)
	return CourierResult{Outcome: CourierDropped, Reason: reason}
}

func (d *CourierDispatcher) dedupeKey(tt login.TemplateType, recipient, code string) [32]byte {
	h := hmac.New(sha256.New, d.DedupeKey)
	h.Write([]byte(tt))
	h.Write([]byte{0})
	h.Write([]byte(recipient))
	h.Write([]byte{0})
	h.Write([]byte(code))
	var k [32]byte
	copy(k[:], h.Sum(nil))
	return k
}

func (d *CourierDispatcher) now() time.Time {
	if d.Clock != nil {
		return d.Clock.Now()
	}
	return time.Now().UTC()
}

func (d *CourierDispatcher) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
