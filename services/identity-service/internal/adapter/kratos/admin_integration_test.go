//go:build integration

package kratos

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

func TestFR11_KratosAdminAdapter(t *testing.T) {
	env := itest.Load()
	a := NewAdmin(env.KratosAdmin, nil)
	ctx := context.Background()
	email := itest.UniqueEmail("kadmin")

	ident, err := a.CreateIdentity(ctx, app.NewIdentity{SchemaID: "admin", Email: email, Name: identity.Name{First: "K"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.DeleteIdentity(context.Background(), ident.ID) })
	if _, err := a.CreateIdentity(ctx, app.NewIdentity{SchemaID: "admin", Email: email}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	got, err := a.GetIdentity(ctx, ident.ID)
	if err != nil || got.SchemaID != "admin" || got.Email != email || got.Name.First != "K" || got.HasTOTP || got.State != identity.StateActive {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := a.GetIdentity(ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	list, _, err := a.ListIdentities(ctx, app.IdentityQuery{Email: email})
	if err != nil || len(list) != 1 || list[0].ID != ident.ID {
		t.Fatalf("list by email: %+v %v", list, err)
	}
	page, next, err := a.ListIdentities(ctx, app.IdentityQuery{PageSize: 1, IncludeTOTP: true})
	if err != nil || len(page) != 1 {
		t.Fatalf("page: %v", err)
	}
	if next != "" {
		if _, _, err := a.ListIdentities(ctx, app.IdentityQuery{PageSize: 1, PageToken: next}); err != nil {
			t.Fatalf("next page: %v", err)
		}
	}
	if err := a.SetState(ctx, ident.ID, identity.StateInactive); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.GetIdentity(ctx, ident.ID); got.State != identity.StateInactive {
		t.Fatal("state not changed")
	}
	if err := a.RevokeIdentitySessions(ctx, ident.ID); err != nil {
		t.Fatalf("revoke with no sessions: %v", err)
	}
	if err := a.RevokeSession(ctx, uuid.New()); err != nil {
		t.Fatalf("revoke unknown session: %v", err)
	}
	rc, err := a.CreateRecoveryCode(ctx, ident.ID, 24*time.Hour)
	if err != nil || rc.Link == "" || rc.Code == "" || rc.ExpiresAt.Before(time.Now().Add(23*time.Hour)) {
		t.Fatalf("recovery: link?%v code?%v exp=%v err=%v", rc.Link != "", rc.Code != "", rc.ExpiresAt, err)
	}
	if err := a.Ready(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFR08_VerifierAgainstKratos(t *testing.T) {
	env := itest.Load()
	email := itest.UniqueEmail("verifier")
	reg := env.RegisterCustomer(t, email)
	t.Cleanup(func() { env.DeleteIdentity(t, uuid.MustParse(reg.Session.Identity.ID)) })
	v := NewSessionVerifier(env.KratosPublic, nil, nil)
	ctx := context.Background()
	p, err := v.Verify(ctx, app.Credential{Kind: app.CredentialToken, Value: reg.SessionToken})
	if err != nil || p.Kind != identity.KindCustomer || p.Email != email || p.EmailVerified || p.AAL != identity.AAL1 || p.SessionID == uuid.Nil {
		t.Fatalf("verify: %+v %v", p, err)
	}
	if _, err := v.Verify(ctx, app.Credential{Kind: app.CredentialToken, Value: "ory_st_invalid"}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatalf("bad token: %v", err)
	}
	// A session token presented as a cookie is not a valid cookie session.
	if _, err := v.Verify(ctx, app.Credential{Kind: app.CredentialCookie, Value: reg.SessionToken}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatalf("token as cookie: %v", err)
	}
	if err := v.Ready(ctx); err != nil {
		t.Fatal(err)
	}
}
