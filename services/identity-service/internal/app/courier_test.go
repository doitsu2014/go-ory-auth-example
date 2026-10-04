package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

type courierEnv struct {
	*loginEnv
	sms  *testutil.SMS
	disp *app.CourierDispatcher
}

func newCourierEnv(t *testing.T) *courierEnv {
	t.Helper()
	l := newLoginEnv(t)
	l.svc.RegisterLimiter = nil // fixtures register many customers
	c := &courierEnv{loginEnv: l, sms: &testutil.SMS{}}
	c.disp = &app.CourierDispatcher{
		Logins: l.svc, Identities: l.ids, Profiles: l.store.Repos().Profiles, Mailer: l.mail, SMS: c.sms,
		Dedupe: l.store.Repos().Dispatches, DedupeKey: []byte("0123456789abcdef0123456789abcdef"),
		RecipientRules: app.CourierRecipientRule, SMSDailyBudget: 3, SMSCountryDailyBudget: 1000,
		Phase: app.PhaseTransition, Clock: l.clock, Log: l.svc.Log,
	}
	return c
}

func (c *courierEnv) customer(t *testing.T, typ, value string) identity.Identity {
	t.Helper()
	return c.ids.AddCustomer(c.resolve(t, typ, value, "registration"), false)
}

func msg(to string, id uuid.UUID, code string) app.CourierMessage {
	return app.CourierMessage{Recipient: to, TemplateType: "verification_code_valid", IdentityID: id, Code: code, ExpiresInMinutes: 60}
}

func TestPLIFR05_DeliversToRealAddress(t *testing.T) {
	c := newCourierEnv(t)
	ctx := context.Background()
	em := c.customer(t, "email", tEmail)
	res, err := c.disp.Dispatch(ctx, msg(strings.ToUpper(em.LoginID), em.ID, "123456"))
	if err != nil || res.Outcome != app.CourierSent || res.Channel != app.ChannelEmail {
		t.Fatalf("email: %+v %v", res, err)
	}
	if len(c.mail.LoginSent) != 1 || c.mail.LoginSent[0].To != tEmail || !strings.Contains(c.mail.LoginSent[0].Message.Text, "123456") {
		t.Fatalf("mail %+v", c.mail.LoginSent)
	}
	if strings.Contains(c.mail.LoginSent[0].Message.Text, "login.invalid") {
		t.Fatal("pseudonym leaked into the message")
	}
	ph := c.customer(t, "phone", tLoginPhone)
	rec := app.CourierMessage{Recipient: ph.LoginID, TemplateType: "recovery_code_valid", IdentityID: ph.ID, Code: "654321"}
	if res, err := c.disp.Dispatch(ctx, rec); err != nil || res.Channel != app.ChannelSMS {
		t.Fatalf("sms: %+v %v", res, err)
	}
	if len(c.sms.Sent) != 1 || c.sms.Sent[0].To != "+84912345678" || !strings.Contains(c.sms.Sent[0].Message.SMS, "654321") {
		t.Fatalf("sms %+v", c.sms.Sent)
	}
	// The registration webhook was missed: delivery bound the rows.
	if len(c.store.Logins) != 2 {
		t.Fatal("rows")
	}
	for _, r := range c.store.Logins {
		if r.IdentityID == nil {
			t.Fatal("dispatch must bind the row to the confirmed identity")
		}
	}
}

func TestPLINFR10_DedupeAndRetry(t *testing.T) {
	c := newCourierEnv(t)
	ctx := context.Background()
	em := c.customer(t, "email", tEmail)
	m := msg(em.LoginID, em.ID, "111111")
	c.mail.Err = errors.New("smtp down")
	if _, err := c.disp.Dispatch(ctx, m); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("transient: %v", err)
	}
	c.mail.Err = nil
	for i, want := range []app.CourierOutcome{app.CourierSent, app.CourierDuplicate} {
		res, err := c.disp.Dispatch(ctx, m)
		if err != nil || res.Outcome != want {
			t.Fatalf("attempt %d: %+v %v", i, res, err)
		}
	}
	if len(c.mail.LoginSent) != 1 {
		t.Fatalf("sent %d times", len(c.mail.LoginSent))
	}
	if res, _ := c.disp.Dispatch(ctx, msg(em.LoginID, em.ID, "222222")); res.Outcome != app.CourierSent {
		t.Fatal("a new code is a new message")
	}
}

func TestPLIA4_DropDecisionTable(t *testing.T) {
	c := newCourierEnv(t)
	ctx := context.Background()
	em := c.customer(t, "email", tEmail)
	victim := c.customer(t, "email", "victim@example.com")
	admin := c.ids.Add(identity.Identity{SchemaID: "admin", Email: "ops@example.com", LoginID: "ops@example.com"})
	legacy := c.ids.AddCustomer("legacy@example.com", true)
	unknown := c.resolve(t, "email", "nobody@example.com", "sign_in")
	ghost := c.ids.AddCustomer(unknown, false)
	for _, tc := range []struct {
		name string
		m    app.CourierMessage
		want string
	}{
		{"unsupported template", app.CourierMessage{Recipient: em.LoginID, TemplateType: "login_code_valid", IdentityID: em.ID, Code: "123456"}, app.DropUnsupportedTemplate},
		{"bad code", msg(em.LoginID, em.ID, "12345x"), app.DropBadPayload},
		{"no identity id", msg(em.LoginID, uuid.Nil, "123456"), app.DropBadPayload},
		{"identity gone", msg(em.LoginID, uuid.New(), "123456"), app.DropUnbindable},
		{"pseudonym of another identity", msg(victim.LoginID, em.ID, "123456"), app.DropUnbindable},
		{"no vault row", msg(unknown, ghost.ID, "123456"), app.DropUnresolved},
		{"open relay attempt", msg("someone@evil.example", admin.ID, "123456"), app.DropPlaintextRefused},
		{"customer plaintext not its own", msg("x@example.com", legacy.ID, "123456"), app.DropPlaintextRefused},
	} {
		res, err := c.disp.Dispatch(ctx, tc.m)
		if err != nil || res.Outcome != app.CourierDropped || res.Reason != tc.want {
			t.Errorf("%s: %+v %v", tc.name, res, err)
		}
	}
	if len(c.mail.LoginSent) != 0 {
		t.Fatalf("dropped messages were sent: %d", len(c.mail.LoginSent))
	}
	// Admin and (during the transition) legacy customers get their own mail.
	for _, m := range []app.CourierMessage{msg("OPS@example.com", admin.ID, "333333"), msg("legacy@example.com", legacy.ID, "444444")} {
		if res, err := c.disp.Dispatch(ctx, m); err != nil || res.Outcome != app.CourierSent {
			t.Fatalf("pass-through: %+v %v", res, err)
		}
	}
	c.disp.Phase = app.PhaseComplete
	if res, _ := c.disp.Dispatch(ctx, msg("legacy@example.com", legacy.ID, "555555")); res.Reason != app.DropPlaintextRefused {
		t.Fatal("legacy pass-through must stop after the migration")
	}
}

func TestPLIA3_Quotas(t *testing.T) {
	c := newCourierEnv(t)
	ctx := context.Background()
	em := c.customer(t, "email", tEmail)
	codes := []string{"100001", "100002", "100003", "100004", "100005", "100006"}
	var last app.CourierResult
	for _, code := range codes {
		last, _ = c.disp.Dispatch(ctx, msg(em.LoginID, em.ID, code))
	}
	if last.Reason != app.DropOverQuota || len(c.mail.LoginSent) != 5 {
		t.Fatalf("recipient quota: %+v sent=%d", last, len(c.mail.LoginSent))
	}
	// Global SMS budget (3/day in this env).
	for i, v := range []string{"0912000001", "0912000002", "0912000003", "0912000004"} {
		ph := c.customer(t, "phone", v)
		res, _ := c.disp.Dispatch(ctx, msg(ph.LoginID, ph.ID, "200000"))
		if want := i < 3; (res.Outcome == app.CourierSent) != want {
			t.Fatalf("sms %d: %+v", i, res)
		}
	}
	c.disp.SMS = nil
	ph := c.customer(t, "phone", "0912000009")
	if res, _ := c.disp.Dispatch(ctx, msg(ph.LoginID, ph.ID, "300000")); res.Reason != app.DropChannelDisabled {
		t.Fatalf("no SMS channel: %+v", res)
	}
}

func TestPLIFR06_LocaleFromProfile(t *testing.T) {
	c := newCourierEnv(t)
	ctx := context.Background()
	em := c.customer(t, "email", tEmail)
	r := c.store.Repos().Profiles
	_ = r.Ensure(ctx, em.ID, identity.KindCustomer)
	p, _ := r.Get(ctx, em.ID)
	p.Locale = "en-US"
	_, _ = r.Update(ctx, p)
	if _, err := c.disp.Dispatch(ctx, msg(em.LoginID, em.ID, "123456")); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.mail.LoginSent[0].Message.Subject, "Your verification code") {
		t.Fatalf("subject %q", c.mail.LoginSent[0].Message.Subject)
	}
}

func TestPLIA11_TransientOutageNeverDrainsQuota(t *testing.T) {
	c := newCourierEnv(t)
	ctx := context.Background()
	em := c.customer(t, "email", tEmail)
	m := msg(em.LoginID, em.ID, "123123")
	c.mail.Err = errors.New("smtp down")
	for i := range 8 { // more retries than the 5/h quota
		if _, err := c.disp.Dispatch(ctx, m); !errors.Is(err, app.ErrDependencyUnavailable) {
			t.Fatalf("retry %d: %v", i, err)
		}
	}
	c.mail.Err = nil
	if res, err := c.disp.Dispatch(ctx, m); err != nil || res.Outcome != app.CourierSent {
		t.Fatalf("after the outage the code must still be delivered: %+v %v", res, err)
	}
}

func TestPLIA11_PermanentRejectionIsDropped(t *testing.T) {
	c := newCourierEnv(t)
	ctx := context.Background()
	em := c.customer(t, "email", tEmail)
	c.mail.Err = fmt.Errorf("%w: rcpt 550", app.ErrPermanentDelivery)
	res, err := c.disp.Dispatch(ctx, msg(em.LoginID, em.ID, "321321"))
	if err != nil || res.Outcome != app.CourierDropped || res.Reason != app.DropProviderRejected {
		t.Fatalf("permanent: %+v %v", res, err)
	}
	c.mail.Err = nil
	if res, _ := c.disp.Dispatch(ctx, msg(em.LoginID, em.ID, "321321")); res.Outcome != app.CourierDuplicate {
		t.Fatal("a dropped message is final (no Kratos retry storm)")
	}
}

func TestPLIA3_CountryBudgetIndependent(t *testing.T) {
	c := newCourierEnv(t)
	c.disp.SMSDailyBudget, c.disp.SMSCountryDailyBudget = 100, 1
	ctx := context.Background()
	a := c.customer(t, "phone", "0912000101")
	b := c.customer(t, "phone", "0912000102")
	if res, _ := c.disp.Dispatch(ctx, msg(a.LoginID, a.ID, "400001")); res.Outcome != app.CourierSent {
		t.Fatal("first SMS")
	}
	if res, _ := c.disp.Dispatch(ctx, msg(b.LoginID, b.ID, "400002")); res.Reason != app.DropOverQuota {
		t.Fatalf("per-country budget: %+v", res)
	}
}
