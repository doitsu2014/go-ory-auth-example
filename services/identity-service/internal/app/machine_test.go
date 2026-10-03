package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

// --- M2M-FR-08..11: service clients ---

type scEnv struct {
	*env
	clients  *testutil.ServiceClients
	verifier *testutil.MachineVerifier
	super    app.Actor
	support  app.Actor
}

func newSCEnv() *scEnv {
	e := newEnv()
	s := &scEnv{env: e, clients: testutil.NewServiceClients(e.clock), verifier: testutil.NewMachineVerifier(),
		super: adminActor(uuid.New()), support: adminActor(uuid.New())}
	e.authz.GrantRole(s.super.Principal.IdentityID, identity.RoleSuperAdmin)
	e.authz.GrantRole(s.support.Principal.IdentityID, identity.RoleAdmin)
	return s
}

func (s *scEnv) svc() *app.ServiceClientService {
	return &app.ServiceClientService{Authz: s.authz, Clients: s.clients, Verifier: s.verifier, Tx: s.store,
		Idempotency: s.store.Repos().Idempotency, Clock: s.clock}
}

var scopesBoth = []string{"customers:read", "audit:read"}

func TestM2MFR08_CreateIsAuditedIdempotentAndReturnsSecretOnce(t *testing.T) {
	s := newSCEnv()
	ctx := context.Background()
	key := uuid.NewString()
	out, err := s.svc().Create(ctx, s.super, key, "billing-sync", "ops@example.com", scopesBoth)
	if err != nil {
		t.Fatal(err)
	}
	if out.Secret == "" || out.Client.ClientID == "" || out.Client.Name != "billing-sync" {
		t.Fatalf("create: %+v", out)
	}
	if acts := s.store.AuditActions(); len(acts) != 1 || acts[0] != audit.ActionServiceClientCreated {
		t.Fatalf("audit: %v", acts)
	}
	ev := s.store.Events[0]
	if ev.TargetType != audit.TargetServiceClient || ev.TargetID != out.Client.ClientID || ev.ActorID != s.super.Principal.IdentityID {
		t.Fatalf("audit event: %+v", ev)
	}
	for _, v := range ev.Details {
		if strings.Contains(strings.ToLower(toString(v)), "secret") {
			t.Fatalf("audit details leak the secret: %+v", ev.Details)
		}
	}
	// The stored idempotent response never holds the secret.
	for _, rec := range s.store.Idem {
		if strings.Contains(string(rec.ResponseBody), out.Secret) || strings.Contains(string(rec.ResponseBody), "secret") {
			t.Fatalf("idempotency record holds the secret: %s", rec.ResponseBody)
		}
	}
	again, err := s.svc().Create(ctx, s.super, key, "billing-sync", "ops@example.com", scopesBoth)
	if err != nil || again.Secret != "" || again.Client.ClientID != out.Client.ClientID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if len(s.clients.M) != 1 || len(s.store.Events) != 1 {
		t.Fatal("replay must not create a second client or audit row")
	}
	if _, err := s.svc().Create(ctx, s.super, key, "other-name", "ops@example.com", scopesBoth); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("key reuse with another body: %v", err)
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestM2MFR08_CreateValidationAndPermission(t *testing.T) {
	s := newSCEnv()
	ctx := context.Background()
	if _, err := s.svc().Create(ctx, s.support, uuid.NewString(), "billing", "a@b.co", scopesBoth); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("admin without manage_service_clients: %v", err)
	}
	if _, err := s.svc().Create(ctx, app.Actor{Principal: customerPrincipal(true)}, uuid.NewString(), "billing", "a@b.co", scopesBoth); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("customer: %v", err)
	}
	var ve *app.ValidationError
	if _, err := s.svc().Create(ctx, s.super, uuid.NewString(), "Bad Name", "a@b.co", []string{"admin"}); !errors.As(err, &ve) || len(ve.Fields) != 2 {
		t.Fatalf("validation: %v", err)
	}
	if _, err := s.svc().Create(ctx, s.super, "", "billing", "a@b.co", scopesBoth); !errors.As(err, &ve) {
		t.Fatalf("missing key: %v", err)
	}
	if len(s.clients.M) != 0 {
		t.Fatal("nothing may be created on invalid input")
	}
}

func TestM2MFR08_CreateCompensatesWhenRecordingFails(t *testing.T) {
	s := newSCEnv()
	s.store.AuditErr = errors.New("db down")
	key := uuid.NewString()
	if _, err := s.svc().Create(context.Background(), s.super, key, "billing", "a@b.co", scopesBoth); err == nil {
		t.Fatal("want error")
	}
	if len(s.clients.M) != 0 || len(s.clients.Deleted) != 1 {
		t.Fatalf("Hydra client must be deleted again: %+v deleted=%v", s.clients.M, s.clients.Deleted)
	}
	if len(s.store.Idem) != 0 {
		t.Fatal("idempotency key must be released for a retry")
	}
	s.store.AuditErr = nil
	if out, err := s.svc().Create(context.Background(), s.super, key, "billing", "a@b.co", scopesBoth); err != nil || out.Secret == "" {
		t.Fatalf("retry after release: %v", err)
	}
}

func TestM2MFR08_HydraFailureReleasesKey(t *testing.T) {
	s := newSCEnv()
	s.clients.Err = app.ErrDependencyUnavailable
	if _, err := s.svc().Create(context.Background(), s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("want 503: %v", err)
	}
	if len(s.store.Idem) != 0 || len(s.store.Events) != 0 {
		t.Fatal("no key and no audit row may remain")
	}
}

func TestM2MFR13_CreateAsSystemIsAuditedWithSystemActor(t *testing.T) {
	s := newSCEnv()
	out, err := s.svc().CreateAsSystem(context.Background(), "nightly-export", "ops@example.com", []string{"audit:read"})
	if err != nil || out.Secret == "" {
		t.Fatalf("cli create: %+v %v", out, err)
	}
	ev := s.store.Events[0]
	if ev.ActorID != audit.SystemActor || ev.Action != audit.ActionServiceClientCreated || ev.RequestID == "" {
		t.Fatalf("system audit: %+v", ev)
	}
	if out.Client.CreatedBy == nil || *out.Client.CreatedBy != audit.SystemActor {
		t.Fatalf("created_by: %+v", out.Client.CreatedBy)
	}
}

func TestM2MFR10_RotateSetsSecretTokensValidAfterAndInvalidates(t *testing.T) {
	s := newSCEnv()
	ctx := context.Background()
	out, _ := s.svc().Create(ctx, s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth)
	rot, err := s.svc().RotateSecret(ctx, s.super, out.Client.ClientID)
	if err != nil {
		t.Fatal(err)
	}
	if rot.Secret == "" || rot.Secret == out.Secret || s.clients.Secret(out.Client.ClientID) != rot.Secret {
		t.Fatalf("rotate: %q", rot.Secret)
	}
	if len(rot.Secret) != 43 { // base64url of 32 bytes
		t.Fatalf("secret length %d", len(rot.Secret))
	}
	if got := s.clients.M[out.Client.ClientID].TokensValidAfter; !got.Equal(t0.Add(time.Second)) {
		t.Fatalf("tokens_valid_after = %v, want ceil(now)+1 = %v", got, t0.Add(time.Second))
	}
	if len(s.verifier.Invalidated) != 1 || s.verifier.Invalidated[0] != out.Client.ClientID {
		t.Fatalf("status cache not purged: %v", s.verifier.Invalidated)
	}
	if acts := s.store.AuditActions(); !slices.Equal(acts[1:], []audit.Action{audit.ActionServiceClientSecretRotationStarted, audit.ActionServiceClientSecretRotated}) {
		t.Fatalf("audit: %v", acts)
	}
	if _, err := s.svc().RotateSecret(ctx, s.super, "missing"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown client: %v", err)
	}
	if _, err := s.svc().RotateSecret(ctx, s.super, "../admin/clients"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("path-like id: %v", err)
	}
}

func TestM2MFR10_RotateHydraFailureIsRecorded(t *testing.T) {
	s := newSCEnv()
	ctx := context.Background()
	out, _ := s.svc().Create(ctx, s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth)
	s.clients.SetSecretErr = app.ErrDependencyUnavailable
	if _, err := s.svc().RotateSecret(ctx, s.super, out.Client.ClientID); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("want 503: %v", err)
	}
	want := []audit.Action{audit.ActionServiceClientCreated, audit.ActionServiceClientSecretRotationStarted, audit.ActionServiceClientSecretRotationFailed}
	if acts := s.store.AuditActions(); !slices.Equal(acts, want) {
		t.Fatalf("audit: %v", acts)
	}
	if s.clients.Secret(out.Client.ClientID) != out.Secret || len(s.verifier.Invalidated) != 1 {
		t.Fatal("secret unchanged; cache purged anyway (outcome may be unknown)")
	}
}

// Item 4: a failure to record the final row after Hydra already changed the
// secret must not lock the client out: the new secret is still returned.
func TestM2MFR10_RotateFinalAuditFailureStillReturnsSecret(t *testing.T) {
	s := newSCEnv()
	ctx := context.Background()
	var logs bytes.Buffer
	svc := s.svc()
	svc.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	out, _ := svc.Create(ctx, s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth)
	s.clients.OnSetSecret = func() { s.store.TxErrOnce = errors.New("commit failed") }
	rot, err := svc.RotateSecret(ctx, s.super, out.Client.ClientID)
	if err != nil || rot.Secret == "" || s.clients.Secret(out.Client.ClientID) != rot.Secret {
		t.Fatalf("new secret must be returned: %+v %v", rot, err)
	}
	if acts := s.store.AuditActions(); !slices.Equal(acts, []audit.Action{audit.ActionServiceClientCreated, audit.ActionServiceClientSecretRotationStarted}) {
		t.Fatalf("started row must be committed before the PATCH: %v", acts)
	}
	if !strings.Contains(logs.String(), "service_client_audit_incomplete") || strings.Contains(logs.String(), rot.Secret) {
		t.Fatalf("gap must be logged without the secret: %s", logs.String())
	}
}

func TestM2MFR10_RotateStartedRowFailureChangesNothing(t *testing.T) {
	s := newSCEnv()
	ctx := context.Background()
	out, _ := s.svc().Create(ctx, s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth)
	s.store.AuditErr = errors.New("db down")
	if _, err := s.svc().RotateSecret(ctx, s.super, out.Client.ClientID); err == nil {
		t.Fatal("want error")
	}
	if s.clients.Secret(out.Client.ClientID) != out.Secret {
		t.Fatal("Hydra must not be called without the started row")
	}
}

func TestTokensValidAfterIsCeilPlusOne(t *testing.T) {
	if got := app.TokensValidAfter(t0); !got.Equal(t0.Add(time.Second)) {
		t.Fatalf("whole second: %v", got)
	}
	if got := app.TokensValidAfter(t0.Add(time.Millisecond)); !got.Equal(t0.Add(2 * time.Second)) {
		t.Fatalf("fraction: %v", got)
	}
}

// Item 3: an ambiguous Hydra failure (the client may exist) is compensated
// by deleting the id identity-service chose.
func TestM2MFR08_AmbiguousCreateFailureIsCompensated(t *testing.T) {
	s := newSCEnv()
	var logs bytes.Buffer
	svc := s.svc()
	svc.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	s.clients.CreateStoresThenFails = fmt.Errorf("%w: hydra POST /admin/clients: transport error", app.ErrDependencyUnavailable)
	if _, err := svc.Create(context.Background(), s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("want 503: %v", err)
	}
	if len(s.clients.M) != 0 || len(s.clients.Deleted) != 1 || len(s.store.Idem) != 0 || len(s.store.Events) != 0 {
		t.Fatalf("orphan must be deleted, key released, nothing audited: %+v %v", s.clients.M, s.clients.Deleted)
	}
	// Compensation itself fails: possible_orphaned_service_client (name/actor/id only).
	s.clients.DeleteErr = app.ErrDependencyUnavailable
	if _, err := svc.Create(context.Background(), s.super, uuid.NewString(), "billing-two", "a@b.co", scopesBoth); err == nil {
		t.Fatal("want error")
	}
	l := logs.String()
	if !strings.Contains(l, "possible_orphaned_service_client") || !strings.Contains(l, "billing-two") ||
		!strings.Contains(l, s.super.Principal.IdentityID.String()) || strings.Contains(l, "hydra-secret") {
		t.Fatalf("orphan log: %s", l)
	}
	// CLI path compensates the same way.
	s.clients.DeleteErr = nil
	if _, err := svc.CreateAsSystem(context.Background(), "nightly", "a@b.co", scopesBoth); err == nil {
		t.Fatal("want error")
	}
	if _, ok := s.clients.M[s.clients.Deleted[len(s.clients.Deleted)-1]]; ok {
		t.Fatal("CLI orphan not deleted")
	}
}

func TestM2MFR08_NonAmbiguousCreateFailureDeletesNothing(t *testing.T) {
	s := newSCEnv()
	s.clients.Err = app.ErrConflict
	if _, err := s.svc().Create(context.Background(), s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("want conflict: %v", err)
	}
	if len(s.clients.Deleted) != 0 {
		t.Fatal("a definite rejection must not trigger a delete")
	}
}

func TestM2MFR08_CreateSurvivesRequestCancellation(t *testing.T) {
	s := newSCEnv()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := s.svc().Create(ctx, s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth)
	if err != nil || out.Secret == "" {
		t.Fatalf("create with a cancelled request: %v", err)
	}
	if s.clients.CreateCtxErr != nil {
		t.Fatal("the Hydra call must run detached from request cancellation")
	}
	if acts := s.store.AuditActions(); len(acts) != 1 {
		t.Fatalf("audited: %v", acts)
	}
}

func TestM2MFR11_DeleteIsAuditedAndPurgesCache(t *testing.T) {
	s := newSCEnv()
	ctx := context.Background()
	out, _ := s.svc().Create(ctx, s.super, uuid.NewString(), "billing", "a@b.co", scopesBoth)
	if err := s.svc().Delete(ctx, s.super, out.Client.ClientID); err != nil {
		t.Fatal(err)
	}
	if len(s.clients.M) != 0 || len(s.verifier.Invalidated) != 1 {
		t.Fatal("delete + purge")
	}
	if acts := s.store.AuditActions(); !slices.Equal(acts[1:], []audit.Action{audit.ActionServiceClientDeletionStarted, audit.ActionServiceClientDeleted}) {
		t.Fatalf("audit: %v", acts)
	}
	if err := s.svc().Delete(ctx, s.super, out.Client.ClientID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := s.svc().List(ctx, s.support); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("list permission: %v", err)
	}
	// Hydra delete fails: deletion_failed, client kept.
	out2, _ := s.svc().Create(ctx, s.super, uuid.NewString(), "billing-two", "a@b.co", scopesBoth)
	s.clients.DeleteErr = app.ErrDependencyUnavailable
	if err := s.svc().Delete(ctx, s.super, out2.Client.ClientID); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("delete failure: %v", err)
	}
	if acts := s.store.AuditActions(); acts[len(acts)-1] != audit.ActionServiceClientDeletionFailed {
		t.Fatalf("audit: %v", acts)
	}
}

// --- M2M-FR-05, 06: machine plane use cases ---

func TestM2MFR05_CustomerStatusHasNoPIIAndHidesAdmins(t *testing.T) {
	e := newEnv()
	svc := &app.MachineService{Identities: e.ids, Audit: e.store.Repos().Audit}
	cust := e.ids.Add(identity.Identity{SchemaID: "customer", Email: "c@example.com", EmailVerified: true, Name: identity.Name{First: "C"}, CreatedAt: t0})
	adm := e.ids.Add(identity.Identity{SchemaID: "admin", Email: "a@example.com"})
	p := machine.Principal{ClientID: "c1", Scopes: []machine.Scope{machine.ScopeCustomersRead}}
	v, err := svc.Customer(context.Background(), p, cust.ID)
	if err != nil || v.ID != cust.ID || !v.EmailVerified || v.State != identity.StateActive || !v.CreatedAt.Equal(t0) {
		t.Fatalf("customer: %+v %v", v, err)
	}
	if _, err := svc.Customer(context.Background(), p, adm.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("admin id: %v", err)
	}
	if _, err := svc.Customer(context.Background(), p, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	wrong := machine.Principal{ClientID: "c1", Scopes: []machine.Scope{machine.ScopeAuditRead}}
	if _, err := svc.Customer(context.Background(), wrong, cust.ID); !errors.Is(err, app.ErrInsufficientScope) {
		t.Fatalf("scope: %v", err)
	}
}

func TestM2MFR06_AuditFeedAllowlist(t *testing.T) {
	e := newEnv()
	r := e.store.Repos()
	ctx := context.Background()
	add := func(a audit.Action, details map[string]any) {
		if err := r.Audit.Append(ctx, audit.Event{ActorID: uuid.New(), Action: a, TargetType: "customer", TargetID: "x", RequestID: "rq", Details: details}); err != nil {
			t.Fatal(err)
		}
	}
	add(audit.ActionCustomerDisabled, map[string]any{"reason": "fraud"})
	add(audit.ActionCustomerPIIRevealed, map[string]any{"fields": []any{"phone_number"}, "ticket_ref": "T-1"})
	add(audit.ActionAdminRoleChanged, map[string]any{"role": "admin", "previous_role": "support"})
	add(audit.ActionCustomerPIILookup, map[string]any{"bidx": "x", "matched_ids": []any{"y"}})
	add(audit.ActionServiceClientCreated, map[string]any{"name": "billing", "scopes": []any{"audit:read"}})
	for range 3 {
		add(audit.ActionCustomerPIIUpdated, map[string]any{"fields": []any{"address"}})
	}
	svc := &app.MachineService{Identities: e.ids, Audit: r.Audit}
	p := machine.Principal{ClientID: "c1", Scopes: []machine.Scope{machine.ScopeAuditRead}}
	page, err := svc.AuditEvents(ctx, p, audit.Filter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Exact paging despite hidden rows: 2 items + a cursor, then 1 item.
	if len(page.Items) != 2 || page.Next == nil {
		t.Fatalf("page 1: %+v", page)
	}
	page2, err := svc.AuditEvents(ctx, p, audit.Filter{Limit: 2, After: page.Next})
	if err != nil || len(page2.Items) != 1 || page2.Next != nil {
		t.Fatalf("page 2: %+v %v", page2, err)
	}
	all := append(page.Items, page2.Items...)
	for _, ev := range all {
		if strings.HasPrefix(string(ev.Action), "customer.pii.") {
			t.Fatalf("pii action leaked: %s", ev.Action)
		}
	}
	if all[0].Action != audit.ActionServiceClientCreated || *all[0].Details.Name != "billing" || len(all[0].Details.Scopes) != 1 {
		t.Fatalf("service client event: %+v", all[0])
	}
	if all[1].Action != audit.ActionAdminRoleChanged || *all[1].Details.Role != "admin" || *all[1].Details.PreviousRole != "support" {
		t.Fatalf("role event: %+v", all[1])
	}
	if all[2].Action != audit.ActionCustomerDisabled || !all[2].Details.Empty() {
		t.Fatalf("reason must be dropped: %+v", all[2])
	}
	if _, err := svc.AuditEvents(ctx, machine.Principal{ClientID: "c1", Scopes: []machine.Scope{machine.ScopeCustomersRead}}, audit.Filter{}); !errors.Is(err, app.ErrInsufficientScope) {
		t.Fatalf("scope: %v", err)
	}
	if _, err := svc.AuditEvents(ctx, machine.Principal{}, audit.Filter{}); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("no principal: %v", err)
	}
}
