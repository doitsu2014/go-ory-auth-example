package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

// Plane is the API plane a request belongs to.
type Plane int

// Planes.
const (
	PlaneNone Plane = iota
	PlaneCustomer
	PlaneAdmin
	// PlaneMachine is /m2m/v1: Hydra client_credentials JWTs only.
	PlaneMachine
	// PlanePublic is /v1/auth/*: no credential, rate limited per client IP
	// (customer login, registration and recovery, ADR-0014).
	PlanePublic
)

// publicPrefix is the unauthenticated part of the customer API.
const publicPrefix = "/v1/auth/"

func planeOf(r *http.Request) Plane {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, publicPrefix):
		return PlanePublic
	case p == "/v1" || strings.HasPrefix(p, "/v1/"):
		return PlaneCustomer
	case p == "/admin/v1" || strings.HasPrefix(p, "/admin/v1/"):
		return PlaneAdmin
	case p == "/m2m/v1" || strings.HasPrefix(p, "/m2m/v1/"):
		return PlaneMachine
	}
	return PlaneNone
}

// reqState is mutable per-request state shared with the access log.
type reqState struct {
	mu          sync.Mutex
	requestID   string
	identityID  string
	problemCode string
	actor       *app.Actor
	// Machine plane: the verified service client and the scope the matched
	// route requires (for WWW-Authenticate on 403).
	machine       *machine.Principal
	requiredScope machine.Scope
}

type stateKey struct{}

func stateFrom(ctx context.Context) *reqState {
	s, _ := ctx.Value(stateKey{}).(*reqState)
	return s
}

// RequestIDFrom returns the request id.
func RequestIDFrom(ctx context.Context) string {
	if s := stateFrom(ctx); s != nil {
		return s.requestID
	}
	return ""
}

func setProblemCode(ctx context.Context, code string) {
	if s := stateFrom(ctx); s != nil {
		s.mu.Lock()
		s.problemCode = code
		s.mu.Unlock()
	}
}

func setActor(ctx context.Context, a app.Actor) {
	if s := stateFrom(ctx); s != nil {
		s.mu.Lock()
		s.actor = &a
		s.identityID = a.Principal.IdentityID.String()
		s.mu.Unlock()
	}
}

func setMachine(ctx context.Context, p machine.Principal) {
	if s := stateFrom(ctx); s != nil {
		s.mu.Lock()
		s.machine = &p
		s.mu.Unlock()
	}
}

func setRequiredScope(ctx context.Context, sc machine.Scope) {
	if s := stateFrom(ctx); s != nil {
		s.mu.Lock()
		s.requiredScope = sc
		s.mu.Unlock()
	}
}

func requiredScopeFrom(ctx context.Context) machine.Scope {
	if s := stateFrom(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.requiredScope
	}
	return ""
}

// MachineFrom returns the service client set by the machine guard.
func MachineFrom(ctx context.Context) (machine.Principal, bool) {
	s := stateFrom(ctx)
	if s == nil {
		return machine.Principal{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.machine == nil {
		return machine.Principal{}, false
	}
	return *s.machine, true
}

// ActorFrom returns the authenticated actor set by the auth middleware.
func ActorFrom(ctx context.Context) (app.Actor, bool) {
	s := stateFrom(ctx)
	if s == nil {
		return app.Actor{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.actor == nil {
		return app.Actor{}, false
	}
	return *s.actor, true
}

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

func newRequestID(inbound string) string {
	if requestIDRe.MatchString(inbound) {
		return inbound
	}
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.NewString()
}

// clientIP returns the caller address. With trustedHops > 0 it takes the
// right-most X-Forwarded-For entry not added by a trusted proxy
// (entries[len-trustedHops]); otherwise, or if the header is too short or
// malformed, the TCP peer address.
func clientIP(r *http.Request, trustedHops int) *netip.Addr {
	if trustedHops > 0 {
		var entries []string
		for _, line := range r.Header.Values("X-Forwarded-For") {
			for _, e := range strings.Split(line, ",") {
				entries = append(entries, strings.TrimSpace(e))
			}
		}
		if len(entries) >= trustedHops {
			if a, err := netip.ParseAddr(entries[len(entries)-trustedHops]); err == nil {
				a = a.Unmap().WithZone("")
				return &a
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	a = a.Unmap().WithZone("")
	return &a
}

// routingPath is the path chi routes on (RawPath when set).
func routingPath(r *http.Request) string {
	if r.URL.RawPath != "" {
		return r.URL.RawPath
	}
	return r.URL.Path
}

type publicClientKey struct{}

// publicClient is the end client of a public-plane request.
type publicClient struct {
	ip        netip.Addr
	userAgent string
}

func withPublicClient(ctx context.Context, ip netip.Addr, userAgent string) context.Context {
	return context.WithValue(ctx, publicClientKey{}, publicClient{ip: ip, userAgent: userAgent})
}

// PublicClientIPFrom returns the client IP of a public-plane request.
func PublicClientIPFrom(ctx context.Context) (netip.Addr, bool) {
	c, ok := ctx.Value(publicClientKey{}).(publicClient)
	return c.ip, ok
}

// PublicFlowClientFrom returns the end client of a public-plane request, as
// forwarded to Kratos.
func PublicFlowClientFrom(ctx context.Context) (app.FlowClient, bool) {
	c, ok := ctx.Value(publicClientKey{}).(publicClient)
	return app.FlowClient{IP: c.ip, UserAgent: c.userAgent}, ok
}
