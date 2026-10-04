package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

// Policy is the authorization declaration of one route (P4: deny by default).
type Policy struct {
	Plane Plane
	// Permission is the Keto permit on Console:main the caller must hold.
	Permission identity.Permission
	// Self marks routes that act only on the caller (no Keto permission).
	Self bool
	// AllowAAL1 lets an admin with TOTP reach the route at AAL1
	// (only /admin/v1/me, so the web can route to step-up).
	AllowAAL1 bool
	// Scope is the OAuth2 scope a machine-plane route requires. Required on
	// PlaneMachine, forbidden elsewhere.
	Scope machine.Scope
}

// RoutePolicies declares the policy of every public route, keyed by
// "METHOD pattern". The server refuses to start if a registered route has no
// entry, an admin route declares neither a permission nor Self, a machine
// route declares no scope, or an entry has no route (see ValidatePolicies).
var RoutePolicies = map[string]Policy{
	// Public (no credential, per-IP limits in the use case). PLI-FR-01.
	"POST /v1/auth/identifiers": {Plane: PlanePublic},

	// Customer plane (bearer only). FR-09, FR-10.
	"GET /v1/me":   {Plane: PlaneCustomer, Self: true},
	"PATCH /v1/me": {Plane: PlaneCustomer, Self: true},
	// Personal information (PII-FR-01, 02, 04).
	"GET /v1/me/personal-info":    {Plane: PlaneCustomer, Self: true},
	"PUT /v1/me/personal-info":    {Plane: PlaneCustomer, Self: true},
	"DELETE /v1/me/personal-info": {Plane: PlaneCustomer, Self: true},

	// Admin plane (cookie only, AAL2 unless stated). FR-05..FR-07, FR-11, FR-12.
	"GET /admin/v1/me":                         {Plane: PlaneAdmin, Self: true, AllowAAL1: true},
	"GET /admin/v1/customers":                  {Plane: PlaneAdmin, Permission: identity.PermViewCustomers},
	"GET /admin/v1/customers/{id}":             {Plane: PlaneAdmin, Permission: identity.PermViewCustomers},
	"POST /admin/v1/customers/{id}/disable":    {Plane: PlaneAdmin, Permission: identity.PermManageCustomers},
	"POST /admin/v1/customers/{id}/enable":     {Plane: PlaneAdmin, Permission: identity.PermManageCustomers},
	"DELETE /admin/v1/customers/{id}/sessions": {Plane: PlaneAdmin, Permission: identity.PermManageCustomers},
	"GET /admin/v1/admins":                     {Plane: PlaneAdmin, Permission: identity.PermManageAdmins},
	"POST /admin/v1/admins":                    {Plane: PlaneAdmin, Permission: identity.PermManageAdmins},
	"PUT /admin/v1/admins/{id}/role":           {Plane: PlaneAdmin, Permission: identity.PermManageAdmins},
	"GET /admin/v1/audit-events":               {Plane: PlaneAdmin, Permission: identity.PermViewAudit},

	// Personal information (PII-FR-05..07).
	"POST /admin/v1/customers/lookup":                    {Plane: PlaneAdmin, Permission: identity.PermViewCustomers},
	"GET /admin/v1/customers/{id}/personal-info":         {Plane: PlaneAdmin, Permission: identity.PermViewCustomers},
	"POST /admin/v1/customers/{id}/personal-info/reveal": {Plane: PlaneAdmin, Permission: identity.PermRevealCustomerPII},

	// Service clients (M2M-FR-08..11).
	"GET /admin/v1/service-clients":                            {Plane: PlaneAdmin, Permission: identity.PermManageServiceClients},
	"POST /admin/v1/service-clients":                           {Plane: PlaneAdmin, Permission: identity.PermManageServiceClients},
	"GET /admin/v1/service-clients/{client_id}":                {Plane: PlaneAdmin, Permission: identity.PermManageServiceClients},
	"DELETE /admin/v1/service-clients/{client_id}":             {Plane: PlaneAdmin, Permission: identity.PermManageServiceClients},
	"POST /admin/v1/service-clients/{client_id}/rotate-secret": {Plane: PlaneAdmin, Permission: identity.PermManageServiceClients},

	// Machine plane (Hydra JWT only, scope per route). M2M-FR-04..06.
	"GET /m2m/v1/customers/{id}": {Plane: PlaneMachine, Scope: machine.ScopeCustomersRead},
	"GET /m2m/v1/audit-events":   {Plane: PlaneMachine, Scope: machine.ScopeAuditRead},
}

func routeKey(method, pattern string) string { return method + " " + pattern }

// ValidatePolicies walks the router and fails closed on any route without a
// valid policy, and on any policy without a route.
func ValidatePolicies(r chi.Routes, policies map[string]Policy) error {
	seen := map[string]bool{}
	var errs []error
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodOptions || method == http.MethodHead {
			return nil
		}
		k := routeKey(method, route)
		seen[k] = true
		p, ok := policies[k]
		if !ok {
			errs = append(errs, fmt.Errorf("route %q has no declared policy", k))
			return nil
		}
		wantPlane := PlaneNone
		switch {
		case strings.HasPrefix(route, publicPrefix):
			wantPlane = PlanePublic
		case strings.HasPrefix(route, "/admin/v1/"):
			wantPlane = PlaneAdmin
		case strings.HasPrefix(route, "/v1/"):
			wantPlane = PlaneCustomer
		case strings.HasPrefix(route, "/m2m/v1/"):
			wantPlane = PlaneMachine
		}
		if p.Plane != wantPlane || wantPlane == PlaneNone {
			errs = append(errs, fmt.Errorf("route %q: policy plane does not match its prefix", k))
		}
		if p.Plane == PlanePublic && (p.Permission != "" || p.Self || p.AllowAAL1 || p.Scope != "") {
			errs = append(errs, fmt.Errorf("public route %q may not declare authorization", k))
		}
		if p.Plane == PlaneAdmin && p.Permission == "" && !p.Self {
			errs = append(errs, fmt.Errorf("admin route %q declares no permission", k))
		}
		if p.Permission != "" && p.Self {
			errs = append(errs, fmt.Errorf("route %q declares both Self and a permission", k))
		}
		// §6 A13: machine routes are authorised by scope only; other planes
		// never by scope.
		if p.Plane == PlaneMachine {
			if _, ok := machine.ParseScope(string(p.Scope)); !ok {
				errs = append(errs, fmt.Errorf("machine route %q declares no valid scope", k))
			}
			if p.Permission != "" || p.Self || p.AllowAAL1 {
				errs = append(errs, fmt.Errorf("machine route %q may declare only a scope", k))
			}
		} else if p.Scope != "" {
			errs = append(errs, fmt.Errorf("route %q is not a machine route but declares a scope", k))
		}
		return nil
	})
	if err != nil {
		return err
	}
	var stale []string
	for k := range policies {
		if !seen[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(stale)
	for _, k := range stale {
		errs = append(errs, fmt.Errorf("policy %q has no route", k))
	}
	return errors.Join(errs...)
}
