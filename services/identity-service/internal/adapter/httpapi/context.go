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
)

// Plane is the API plane a request belongs to.
type Plane int

// Planes.
const (
	PlaneNone Plane = iota
	PlaneCustomer
	PlaneAdmin
)

func planeOf(r *http.Request) Plane {
	p := r.URL.Path
	switch {
	case p == "/v1" || strings.HasPrefix(p, "/v1/"):
		return PlaneCustomer
	case p == "/admin/v1" || strings.HasPrefix(p, "/admin/v1/"):
		return PlaneAdmin
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
