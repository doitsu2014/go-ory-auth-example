package kratos

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// SessionCookieName is the Kratos session cookie.
const SessionCookieName = "ory_kratos_session"

// SessionVerifier resolves credentials through Kratos public
// GET /sessions/whoami, with a positive-only session cache.
type SessionVerifier struct {
	c     client
	cache *SessionCache
	now   func() time.Time
}

var _ app.SessionVerifier = (*SessionVerifier)(nil)

// NewSessionVerifier creates a verifier against the Kratos public URL.
func NewSessionVerifier(publicURL string, hc *http.Client, cache *SessionCache) *SessionVerifier {
	if cache == nil {
		cache = NewSessionCache(0, nil)
	}
	return &SessionVerifier{c: newClient(publicURL, hc), cache: cache, now: time.Now}
}

// Verify implements app.SessionVerifier.
//
//	whoami 200                         → Principal (cached)
//	whoami 401 / other 4xx             → ErrUnauthenticated
//	whoami 403 session_aal2_required   → ErrAAL2Required (never 401)
//	5xx / timeout                      → ErrDependencyUnavailable
func (v *SessionVerifier) Verify(ctx context.Context, cred app.Credential) (identity.Principal, error) {
	if cred.Value == "" {
		return identity.Principal{}, app.ErrUnauthenticated
	}
	if p, ok := v.cache.Get(cred); ok {
		return p, nil
	}
	h := http.Header{}
	switch cred.Kind {
	case app.CredentialToken:
		h.Set("X-Session-Token", cred.Value)
	case app.CredentialCookie:
		// Forward only the Kratos session cookie, never other cookies.
		h.Set("Cookie", SessionCookieName+"="+cred.Value)
	default:
		return identity.Principal{}, app.ErrUnauthenticated
	}
	startGen := v.cache.Generation()
	resp, err := v.c.do(ctx, http.MethodGet, "/sessions/whoami", h, nil)
	if err != nil {
		return identity.Principal{}, err
	}
	switch {
	case resp.status == http.StatusOK:
	case resp.status == http.StatusForbidden && resp.errorID() == "session_aal2_required":
		return identity.Principal{}, app.ErrAAL2Required
	case resp.status >= 400 && resp.status < 500:
		return identity.Principal{}, app.ErrUnauthenticated
	default:
		return identity.Principal{}, fmt.Errorf("%w: whoami status %d", app.ErrDependencyUnavailable, resp.status)
	}
	var s kSession
	if err := resp.decode(&s); err != nil {
		return identity.Principal{}, fmt.Errorf("%w: %v", app.ErrDependencyUnavailable, err)
	}
	if !s.Active || !v.now().Before(s.ExpiresAt) || s.Identity.State != string(identity.StateActive) {
		return identity.Principal{}, app.ErrUnauthenticated
	}
	p, ok := s.toPrincipal()
	if !ok {
		return identity.Principal{}, app.ErrForbidden
	}
	v.cache.PutIfFresh(cred, p, startGen)
	return p, nil
}

// Invalidate implements app.SessionVerifier.
func (v *SessionVerifier) Invalidate(identityID uuid.UUID) { v.cache.Invalidate(identityID) }

// Ready checks Kratos public readiness.
func (v *SessionVerifier) Ready(ctx context.Context) error {
	resp, err := v.c.do(ctx, http.MethodGet, "/health/ready", nil, nil)
	if err != nil {
		return err
	}
	if resp.status != http.StatusOK {
		return fmt.Errorf("kratos public not ready: %d", resp.status)
	}
	return nil
}
