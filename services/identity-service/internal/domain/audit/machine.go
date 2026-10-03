package audit

import (
	"strings"
)

// Machine audit feed allowlists (security review §6 B1). Machines see only
// state changes of customers, admin management and service client
// management; personal-data actions (customer.pii.*) never appear. Detail
// keys outside machineDetailKeys (reason, ticket_ref, bidx, matched_ids,
// field names, …) are dropped.
var (
	machineActions = []Action{
		ActionCustomerDisabled, ActionCustomerEnabled, ActionCustomerSessionsRevoked,
	}
	machineActionPrefixes = []string{"admin.", "service_client."}
)

// MachineFilter restricts f to the machine action allowlist.
func MachineFilter(f Filter) Filter {
	f.Actions = append([]Action(nil), machineActions...)
	f.ActionPrefixes = append([]string(nil), machineActionPrefixes...)
	return f
}

// MachineVisible reports whether a machine may see an event with action a.
func MachineVisible(a Action) bool {
	if strings.HasPrefix(string(a), "customer.pii.") {
		return false
	}
	return Filter{Actions: machineActions, ActionPrefixes: machineActionPrefixes}.Matches(a)
}

// MachineDetails is the allowlisted, type-checked subset of event details.
type MachineDetails struct {
	Role         *string
	PreviousRole *string
	Scopes       []string
	Name         *string
}

// Empty reports whether no allowlisted key survived.
func (d MachineDetails) Empty() bool {
	return d.Role == nil && d.PreviousRole == nil && d.Scopes == nil && d.Name == nil
}

// MachineDetailsOf keeps the allowlisted keys of details with the expected
// types (role, previous_role, name: string; scopes: []string). Everything
// else is dropped.
func MachineDetailsOf(details map[string]any) MachineDetails {
	var d MachineDetails
	str := func(k string) *string {
		if s, ok := details[k].(string); ok {
			return &s
		}
		return nil
	}
	d.Role, d.PreviousRole, d.Name = str("role"), str("previous_role"), str("name")
	switch v := details["scopes"].(type) {
	case []string:
		d.Scopes = append([]string{}, v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			s, ok := x.(string)
			if !ok {
				out = nil
				break
			}
			out = append(out, s)
		}
		d.Scopes = out
	}
	return d
}
