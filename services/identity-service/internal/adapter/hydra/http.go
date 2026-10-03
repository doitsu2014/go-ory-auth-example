// Package hydra implements the machine-to-machine ports over Ory Hydra
// v26 (intent 261003-add-ory-hydra) with thin hand-written HTTP adapters:
//
//   - Admin: app.ServiceClientAdmin over the private admin API
//     (/admin/clients), restricted to clients whose metadata.managed_by is
//     "identity-service" (§6 B2), plus the client-status lookup.
//   - Verifier: app.MachineTokenVerifier, offline RS256 JWT verification
//     against the cached public JWKS plus a cached client-status check
//     (§6 A7–A9, A16, B2, B3).
//
// Clients never follow redirects, bound every call with a timeout and read
// at most a fixed number of bytes. Request and response bodies (which hold
// client secrets and registration tokens) are never logged and never appear
// in errors.
package hydra

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// DefaultTimeout bounds every call to the Hydra admin API.
const DefaultTimeout = 2 * time.Second

const maxResponseBody = 1 << 20

// newHTTPClient returns hc (copied) or a fresh client, never following
// redirects (a redirect could forward the request to another origin).
func newHTTPClient(hc *http.Client, timeout time.Duration) *http.Client {
	var c http.Client
	if hc != nil {
		c = *hc
	} else {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		c = http.Client{Transport: tr}
	}
	if c.Timeout <= 0 || c.Timeout > timeout {
		c.Timeout = timeout
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

type client struct {
	base string
	http *http.Client
}

type response struct {
	status int
	header http.Header
	body   []byte
}

// do sends a request. GETs are retried once on transport errors or 5xx.
// Transport failures and 5xx map to ErrDependencyUnavailable. Errors carry
// the method, the path without query and the status, nothing else.
func (c client) do(ctx context.Context, method, path, contentType string, body any) (response, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return response{}, fmt.Errorf("hydra %s %s: encode body", method, redactPath(path))
		}
		payload = b
		defer clear(payload)
	}
	attempts := 1
	if method == http.MethodGet {
		attempts = 2
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
		if err != nil {
			return response{}, fmt.Errorf("hydra %s %s: build request", method, redactPath(path))
		}
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			// The transport error may echo the URL; it carries no secrets
			// (paths hold client ids only) but is reduced to its kind.
			lastErr = fmt.Errorf("%w: hydra %s %s: transport error", app.ErrDependencyUnavailable, method, redactPath(path))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("%w: hydra %s %s: read response", app.ErrDependencyUnavailable, method, redactPath(path))
			continue
		}
		if len(b) > maxResponseBody {
			return response{}, fmt.Errorf("%w: hydra %s %s: response too large", app.ErrDependencyUnavailable, method, redactPath(path))
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("%w: hydra %s %s: status %d", app.ErrDependencyUnavailable, method, redactPath(path), resp.StatusCode)
			continue
		}
		return response{status: resp.StatusCode, header: resp.Header, body: b}, nil
	}
	return response{}, lastErr
}

func redactPath(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i]
	}
	return p
}

// unexpected wraps a non-success status without the body. An unexpected
// answer from Hydra is a dependency failure (503), never a client error.
func (r response) unexpected(op string) error {
	return fmt.Errorf("%w: hydra %s: status %d", app.ErrDependencyUnavailable, op, r.status)
}
