package audit

import (
	"slices"
	"testing"
)

func TestMachineVisible(t *testing.T) {
	visible := []Action{
		ActionCustomerDisabled, ActionCustomerEnabled, ActionCustomerSessionsRevoked,
		ActionAdminInvited, ActionAdminInvitationFailed, ActionAdminRoleChanged, ActionAdminBootstrapped, ActionAdminDeactivated,
		ActionServiceClientCreated, ActionServiceClientSecretRotated, ActionServiceClientDeleted,
	}
	for _, a := range visible {
		if !MachineVisible(a) {
			t.Errorf("%s must be visible", a)
		}
	}
	hidden := []Action{
		ActionCustomerPIIUpdated, ActionCustomerPIIErased, ActionCustomerPIIRevealed, ActionCustomerPIILookup,
		"customer.pii.future", "customer.something_new", "admins.x", "service_clientx", "", "admin",
	}
	for _, a := range hidden {
		if MachineVisible(a) {
			t.Errorf("%q must be hidden", a)
		}
	}
	// The SQL-side filter agrees with MachineVisible on every known action.
	f := MachineFilter(Filter{Limit: 5})
	if f.Limit != 5 || len(f.Actions) == 0 || len(f.ActionPrefixes) == 0 {
		t.Fatalf("MachineFilter: %+v", f)
	}
	for _, a := range append(visible, hidden...) {
		if f.Matches(a) != MachineVisible(a) {
			t.Errorf("filter and MachineVisible disagree on %q", a)
		}
	}
	if !(Filter{}).Matches(ActionCustomerPIIRevealed) {
		t.Fatal("an unrestricted filter matches everything")
	}
}

func TestMachineDetailsOf(t *testing.T) {
	d := MachineDetailsOf(map[string]any{
		"role": "admin", "previous_role": "support", "name": "billing-sync",
		"scopes": []any{"customers:read", "audit:read"},
		// Disallowed keys from the admin and PII audit events.
		"reason": "fraud ticket 12", "ticket_ref": "T-1", "bidx": "abc", "matched_ids": []any{"x"},
		"fields": []any{"phone_number"}, "email": "a@b.co",
	})
	if d.Role == nil || *d.Role != "admin" || d.PreviousRole == nil || *d.PreviousRole != "support" ||
		d.Name == nil || *d.Name != "billing-sync" || !slices.Equal(d.Scopes, []string{"customers:read", "audit:read"}) {
		t.Fatalf("allowlisted keys: %+v", d)
	}
	wrong := MachineDetailsOf(map[string]any{"role": 7, "scopes": []any{"a", 1}, "name": map[string]any{"x": 1}})
	if !wrong.Empty() {
		t.Fatalf("wrongly typed values must be dropped: %+v", wrong)
	}
	if !MachineDetailsOf(map[string]any{"reason": "x"}).Empty() || !MachineDetailsOf(nil).Empty() {
		t.Fatal("no allowlisted key → empty")
	}
	if got := MachineDetailsOf(map[string]any{"scopes": []string{"audit:read"}}); !slices.Equal(got.Scopes, []string{"audit:read"}) {
		t.Fatalf("[]string scopes: %+v", got)
	}
}
