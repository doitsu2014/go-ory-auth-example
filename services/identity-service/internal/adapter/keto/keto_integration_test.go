//go:build integration

package keto

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

// TestFR06_KetoRolesAndPermissions verifies the OPL model and the subject
// format User:<uuid> (subject set, empty relation) against Keto v26.
func TestFR06_KetoRolesAndPermissions(t *testing.T) {
	env := itest.Load()
	c := New(env.KetoRead, env.KetoWrite, nil)
	ctx := context.Background()
	id := uuid.New()
	t.Cleanup(func() { _ = c.RemoveAll(context.Background(), id) })

	perms := func() map[identity.Permission]bool {
		out := map[identity.Permission]bool{}
		for _, p := range identity.AllPermissions {
			ok, err := c.Check(ctx, id, p)
			if err != nil {
				t.Fatal(err)
			}
			out[p] = ok
		}
		return out
	}
	if p := perms(); p[identity.PermViewCustomers] || p[identity.PermManageAdmins] {
		t.Fatalf("no role must grant nothing: %v", p)
	}
	tests := []struct {
		role identity.Role
		want map[identity.Permission]bool
	}{
		{identity.RoleSupport, map[identity.Permission]bool{identity.PermViewCustomers: true}},
		{identity.RoleAdmin, map[identity.Permission]bool{identity.PermViewCustomers: true, identity.PermManageCustomers: true, identity.PermViewAudit: true,
			identity.PermRevealCustomerPII: true}},
		{identity.RoleSuperAdmin, map[identity.Permission]bool{identity.PermViewCustomers: true, identity.PermManageCustomers: true, identity.PermViewAudit: true,
			identity.PermManageAdmins: true, identity.PermRevealCustomerPII: true, identity.PermManageServiceClients: true}},
	}
	for _, tt := range tests {
		if err := c.SetRole(ctx, id, tt.role); err != nil {
			t.Fatal(err)
		}
		roles, err := c.RolesOf(ctx, id)
		if err != nil || len(roles) != 1 || roles[0] != tt.role {
			t.Fatalf("roles after SetRole(%s): %v %v", tt.role, roles, err)
		}
		got := perms()
		for _, p := range identity.AllPermissions {
			if got[p] != tt.want[p] {
				t.Fatalf("%s: %s=%v want %v", tt.role, p, got[p], tt.want[p])
			}
		}
	}
	n, err := c.CountRole(ctx, identity.RoleSuperAdmin)
	if err != nil || n < 1 {
		t.Fatalf("count: %d %v", n, err)
	}
	all, err := c.AllAssignments(ctx)
	if err != nil || len(all[id]) != 1 {
		t.Fatalf("assignments: %v %v", all[id], err)
	}
	if err := c.RemoveAll(ctx, id); err != nil {
		t.Fatal(err)
	}
	if roles, _ := c.RolesOf(ctx, id); len(roles) != 0 {
		t.Fatalf("not removed: %v", roles)
	}
	if err := c.Ready(ctx); err != nil {
		t.Fatal(err)
	}
}
