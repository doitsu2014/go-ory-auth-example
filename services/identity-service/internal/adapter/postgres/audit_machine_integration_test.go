//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
)

// TestM2MFR06_AuditActionAllowlistInSQL: the machine filter is applied in
// the query (exact paging) and LIKE metacharacters in prefixes are literal.
func TestM2MFR06_AuditActionAllowlistInSQL(t *testing.T) {
	s := newStore(t)
	r := s.Repos().Audit
	ctx := context.Background()
	target := "m2m-" + uuid.NewString()
	tt := "customer"
	for _, a := range []audit.Action{
		audit.ActionCustomerDisabled, audit.ActionCustomerPIIRevealed, audit.ActionAdminRoleChanged,
		audit.ActionCustomerPIILookup, audit.ActionServiceClientCreated, "serviceXclient.fake", audit.ActionCustomerPIIUpdated,
	} {
		if err := r.Append(ctx, audit.Event{ActorID: uuid.New(), Action: a, TargetType: tt, TargetID: target, RequestID: "rq",
			Details: map[string]any{"k": "v"}}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.List(ctx, audit.MachineFilter(audit.Filter{TargetType: &tt, TargetID: &target, Limit: 50}))
	if err != nil {
		t.Fatal(err)
	}
	var actions []audit.Action
	for _, e := range got {
		actions = append(actions, e.Action)
	}
	want := []audit.Action{audit.ActionServiceClientCreated, audit.ActionAdminRoleChanged, audit.ActionCustomerDisabled}
	if len(actions) != len(want) {
		t.Fatalf("actions %v want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("actions %v want %v", actions, want)
		}
	}
	// Exact page size despite hidden rows.
	page, err := r.List(ctx, audit.MachineFilter(audit.Filter{TargetType: &tt, TargetID: &target, Limit: 2}))
	if err != nil || len(page) != 2 {
		t.Fatalf("page: %d %v", len(page), err)
	}
	// An unrestricted filter still returns everything.
	all, err := r.List(ctx, audit.Filter{TargetType: &tt, TargetID: &target, Limit: 50})
	if err != nil || len(all) != 7 {
		t.Fatalf("unrestricted: %d %v", len(all), err)
	}
	// Set-but-empty allowlist matches nothing (fail closed).
	none, err := r.List(ctx, audit.Filter{TargetType: &tt, TargetID: &target, Limit: 50, Actions: []audit.Action{"nothing.matches"}})
	if err != nil || len(none) != 0 {
		t.Fatalf("no match: %d %v", len(none), err)
	}
}
