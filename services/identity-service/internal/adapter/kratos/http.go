// Package kratos implements the SessionVerifier and IdentityAdmin ports with
// thin hand-written adapters over the Kratos v26 public and admin HTTP APIs.
package kratos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// DefaultTimeout bounds every outbound call to Ory (06-security §6.4).
const DefaultTimeout = 2 * time.Second

const maxResponseBody = 4 << 20

type client struct {
	base string
	http *http.Client
}

func newClient(base string, hc *http.Client) client {
	if hc == nil {
		hc = &http.Client{Timeout: DefaultTimeout}
	}
	return client{base: strings.TrimRight(base, "/"), http: hc}
}

// kratosError is the Kratos error envelope.
type kratosError struct {
	Error struct {
		ID      string `json:"id"`
		Code    int    `json:"code"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	} `json:"error"`
}

type response struct {
	status int
	header http.Header
	body   []byte
}

// do sends a request. Idempotent (GET) requests are retried once on transport
// errors or 5xx. Transport failures map to ErrDependencyUnavailable.
func (c client) do(ctx context.Context, method, path string, header http.Header, body any) (response, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return response{}, fmt.Errorf("encode body: %w", err)
		}
		payload = b
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
			return response{}, fmt.Errorf("build request: %w", err)
		}
		for k, vs := range header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: kratos %s %s: %v", app.ErrDependencyUnavailable, method, redactPath(path), transportCause(err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("%w: read kratos response: %v", app.ErrDependencyUnavailable, err)
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("%w: kratos %s %s: status %d", app.ErrDependencyUnavailable, method, redactPath(path), resp.StatusCode)
			continue
		}
		return response{status: resp.StatusCode, header: resp.Header, body: b}, nil
	}
	return response{}, lastErr
}

// transportCause strips the *url.Error wrapper, whose text carries the full
// request URL including the query (e.g. credentials_identifier=<address>,
// SEC-C01). Only the underlying cause (dial error, timeout) is kept.
func transportCause(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return errors.New("timeout")
		}
		return ue.Err
	}
	return err
}

func redactPath(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i]
	}
	return p
}

func (r response) errorID() string {
	var e kratosError
	if json.Unmarshal(r.body, &e) == nil {
		return e.Error.ID
	}
	return ""
}

func (r response) decode(v any) error {
	if err := json.Unmarshal(r.body, v); err != nil {
		return fmt.Errorf("decode kratos response: %w", err)
	}
	return nil
}

// unexpected wraps a non-success status; never includes the response body,
// which could contain identity data.
func (r response) unexpected(op string) error {
	if id := r.errorID(); id != "" {
		return fmt.Errorf("kratos %s: status %d (%s)", op, r.status, id)
	}
	return fmt.Errorf("kratos %s: status %d", op, r.status)
}
