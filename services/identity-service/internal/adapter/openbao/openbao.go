// Package openbao implements app.KeyManager over the OpenBao (Vault
// compatible) Transit secrets engine with a thin hand-written HTTP client
// (verified against OpenBao 2.4.1):
//
//   - POST /v1/transit/encrypt/<kek>            wrap a DEK (associated_data = subject)
//   - POST /v1/transit/decrypt/<kek>            unwrap a DEK (same associated_data)
//   - POST /v1/transit/hmac/<bidx>/sha2-256     blind index, key_version pinned to 1
//   - POST /v1/transit/hmac/<login-hmac>/sha2-256  login pseudonym, key_version pinned to 1
//   - POST /v1/transit/encrypt/<login-kek>      seal a login identifier (associated_data)
//   - POST /v1/transit/decrypt/<login-kek>      open login identifiers (batch_input)
//   - POST /v1/auth/token/renew-self            periodic token renewal
//
// The token comes from a file that the init job may replace; it is re-read
// whenever the file changes. Request and response bodies are never logged
// and never appear in errors. Transport failures, 5xx (sealed), 429 and a
// rejected token map to app.ErrDependencyUnavailable; a decrypt
// authentication failure maps to app.ErrDataIntegrity.
package openbao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// Defaults (technical spec §3).
const (
	DefaultTimeout     = 2 * time.Second
	DefaultKEKName     = "identity-pii-kek"
	DefaultBidxKeyName = "identity-pii-bidx"
	// BidxKeyVersion is pinned: the index key is never rotated in v1
	// (rotation would need a re-index job).
	BidxKeyVersion = 1
	// DefaultLoginHMACKeyName and DefaultLoginKEKName are the login
	// pseudonym and login encryption keys (PLI DD-01, DD-03).
	DefaultLoginHMACKeyName = "identity-login-pseudonym"
	DefaultLoginKEKName     = "identity-login-kek"
	// LoginHMACKeyVersion is pinned: every Kratos login_id depends on it.
	// Rotation is the re-key procedure (A7), never an in-place rotate.
	LoginHMACKeyVersion = 1
	// maxBatch bounds one transit/decrypt batch.
	maxBatch         = 100
	maxResponseBody  = 1 << 20
	maxTokenFileSize = 4 << 10
)

var (
	keyNameRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	versionedRe  = regexp.MustCompile(`^vault:v([0-9]+):([A-Za-z0-9+/=]+)$`)
	errBadStatus = errors.New("openbao: unexpected status")
)

// Config configures the client.
type Config struct {
	Addr        string
	TokenFile   string
	KEKName     string
	BidxKeyName string
	// LoginHMACKeyName and LoginKEKName default to the constants above.
	LoginHMACKeyName string
	LoginKEKName     string
	Timeout          time.Duration
	// CAFile optionally holds a PEM CA bundle replacing the system roots.
	CAFile string
	HTTP   *http.Client
	Log    *slog.Logger
}

// Client implements app.KeyManager.
type Client struct {
	addr      string
	tokenFile string
	kek       string
	bidx      string
	loginHMAC string
	loginKEK  string
	timeout   time.Duration
	http      *http.Client
	log       *slog.Logger

	mu      sync.Mutex
	token   string
	tokMod  time.Time
	tokSize int64
}

var (
	_ app.KeyManager = (*Client)(nil)
	_ app.LoginKeys  = (*Client)(nil)
)

// New validates the configuration. The token file is read lazily.
func New(cfg Config) (*Client, error) {
	if cfg.KEKName == "" {
		cfg.KEKName = DefaultKEKName
	}
	if cfg.BidxKeyName == "" {
		cfg.BidxKeyName = DefaultBidxKeyName
	}
	if cfg.LoginHMACKeyName == "" {
		cfg.LoginHMACKeyName = DefaultLoginHMACKeyName
	}
	if cfg.LoginKEKName == "" {
		cfg.LoginKEKName = DefaultLoginKEKName
	}
	names := map[string]bool{}
	for _, n := range []string{cfg.KEKName, cfg.BidxKeyName, cfg.LoginHMACKeyName, cfg.LoginKEKName} {
		if !keyNameRe.MatchString(n) {
			return nil, errors.New("openbao: invalid key name")
		}
		names[n] = true
	}
	if len(names) != 4 {
		return nil, errors.New("openbao: key names must be distinct")
	}
	if cfg.Addr == "" || cfg.TokenFile == "" {
		return nil, errors.New("openbao: address and token file are required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.HTTP == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.CAFile != "" {
			pem, err := os.ReadFile(cfg.CAFile)
			if err != nil {
				return nil, errors.New("openbao: CA file unreadable")
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("openbao: CA file holds no PEM certificates")
			}
			tr.TLSClientConfig.RootCAs = pool
		}
		cfg.HTTP = &http.Client{Timeout: cfg.Timeout, Transport: tr}
	} else {
		hc := *cfg.HTTP
		cfg.HTTP = &hc
	}
	// Never follow redirects: the token header must only go to Addr.
	cfg.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{
		addr: strings.TrimRight(cfg.Addr, "/"), tokenFile: cfg.TokenFile, kek: cfg.KEKName, bidx: cfg.BidxKeyName,
		loginHMAC: cfg.LoginHMACKeyName, loginKEK: cfg.LoginKEKName,
		timeout: cfg.Timeout, http: cfg.HTTP, log: cfg.Log,
	}, nil
}

func unavailable(op string, format string, args ...any) error {
	return fmt.Errorf("%w: openbao %s: %s", app.ErrDependencyUnavailable, op, fmt.Sprintf(format, args...))
}

// currentToken returns the token, re-reading the file when it changed.
func (c *Client) currentToken(op string) (string, error) {
	fi, err := os.Stat(c.tokenFile)
	if err != nil {
		return "", unavailable(op, "token file unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && fi.ModTime().Equal(c.tokMod) && fi.Size() == c.tokSize {
		return c.token, nil
	}
	f, err := os.Open(c.tokenFile)
	if err != nil {
		return "", unavailable(op, "token file unreadable")
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxTokenFileSize))
	tok := strings.TrimSpace(string(b))
	if err != nil || tok == "" {
		return "", unavailable(op, "token file empty")
	}
	c.token, c.tokMod, c.tokSize = tok, fi.ModTime(), fi.Size()
	return tok, nil
}

// forgetToken forces a re-read on the next call (e.g. after a 403).
func (c *Client) forgetToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
}

type statusError struct {
	op     string
	status int
	// authFailed: a 400 whose body reports an authentication failure
	// (decrypt with wrong key/associated data). The body itself is never kept.
	authFailed bool
}

func (e *statusError) Error() string { return fmt.Sprintf("openbao %s: status %d", e.op, e.status) }
func (e *statusError) Unwrap() error { return errBadStatus }

// do POSTs body to /v1/<path> and returns the status and body of a response
// that is not a transport-level, permission, throttling or server failure
// (those are app.ErrDependencyUnavailable). The caller clears the body.
func (c *Client) do(ctx context.Context, op, path string, body any) (int, []byte, error) {
	tok, err := c.currentToken(op)
	if err != nil {
		return 0, nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("openbao %s: encode request", op)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.addr+"/v1/"+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("openbao %s: build request", op)
	}
	req.Header.Set("X-Vault-Token", tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	clear(payload)
	if err != nil {
		return 0, nil, unavailable(op, "transport: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return 0, nil, unavailable(op, "read response")
	}
	switch {
	case resp.StatusCode == http.StatusForbidden:
		clear(b)
		c.forgetToken()
		return 0, nil, unavailable(op, "permission denied (status 403)")
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		clear(b)
		return 0, nil, unavailable(op, "status %d", resp.StatusCode)
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		clear(b)
		return 0, nil, unavailable(op, "unexpected redirect (status %d)", resp.StatusCode)
	}
	return resp.StatusCode, b, nil
}

// call POSTs body to /v1/<path> and decodes the JSON response into out.
func (c *Client) call(ctx context.Context, op, path string, body, out any) error {
	status, b, err := c.do(ctx, op, path, body)
	if err != nil {
		return err
	}
	defer clear(b)
	switch {
	case status == http.StatusOK || status == http.StatusNoContent:
	case status == http.StatusBadRequest:
		if isAuthFailure(b) {
			return &statusError{op: op, status: status, authFailed: true}
		}
		return unavailable(op, "status 400")
	default:
		return &statusError{op: op, status: status}
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("openbao %s: decode response", op)
	}
	return nil
}

func isAuthFailure(b []byte) bool {
	return bytes.Contains(b, []byte("message authentication failed")) || bytes.Contains(b, []byte("invalid ciphertext"))
}

func parseVersioned(s string) (int, string, bool) {
	m := versionedRe.FindStringSubmatch(s)
	if m == nil {
		return 0, "", false
	}
	v, err := strconv.Atoi(m[1])
	if err != nil || v < 1 {
		return 0, "", false
	}
	return v, m[2], true
}

// WrapDEK implements app.KeyManager (transit/encrypt with associated_data).
func (c *Client) WrapDEK(ctx context.Context, kc app.KeyContext, dek []byte) (app.Wrapped, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	pt := base64.StdEncoding.EncodeToString(dek)
	err := c.call(ctx, "encrypt", "transit/encrypt/"+c.kek, map[string]any{
		"plaintext": pt, "associated_data": base64.StdEncoding.EncodeToString(kc.AssociatedData()),
	}, &out)
	if err != nil {
		return app.Wrapped{}, err
	}
	v, _, ok := parseVersioned(out.Data.Ciphertext)
	if !ok || len(out.Data.Ciphertext) > 512 {
		return app.Wrapped{}, errors.New("openbao encrypt: malformed ciphertext")
	}
	return app.Wrapped{Ciphertext: out.Data.Ciphertext, KEKName: c.kek, KEKVersion: v}, nil
}

// UnwrapDEK implements app.KeyManager (transit/decrypt with associated_data).
// Wrong associated data ("message authentication failed", 400) is
// app.ErrDataIntegrity.
func (c *Client) UnwrapDEK(ctx context.Context, kc app.KeyContext, w app.Wrapped) ([]byte, error) {
	kek := w.KEKName
	if kek == "" {
		kek = c.kek
	}
	if !keyNameRe.MatchString(kek) {
		return nil, app.ErrDataIntegrity
	}
	var out struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	defer func() { out.Data.Plaintext = "" }()
	err := c.call(ctx, "decrypt", "transit/decrypt/"+kek, map[string]any{
		"ciphertext": w.Ciphertext, "associated_data": base64.StdEncoding.EncodeToString(kc.AssociatedData()),
	}, &out)
	if err != nil {
		var se *statusError
		if errors.As(err, &se) && se.authFailed {
			return nil, fmt.Errorf("openbao decrypt: %w", app.ErrDataIntegrity)
		}
		return nil, err
	}
	dek, err := base64.StdEncoding.DecodeString(out.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("openbao decrypt: %w", app.ErrDataIntegrity)
	}
	return dek, nil
}

// BlindIndex implements app.KeyManager (transit/hmac sha2-256, key_version 1).
func (c *Client) BlindIndex(ctx context.Context, input []byte) (app.BlindIndex, error) {
	var out struct {
		Data struct {
			HMAC string `json:"hmac"`
		} `json:"data"`
	}
	err := c.call(ctx, "hmac", "transit/hmac/"+c.bidx+"/sha2-256", map[string]any{
		"input": base64.StdEncoding.EncodeToString(input), "key_version": BidxKeyVersion,
	}, &out)
	if err != nil {
		return app.BlindIndex{}, err
	}
	v, b64, ok := parseVersioned(out.Data.HMAC)
	if !ok {
		return app.BlindIndex{}, errors.New("openbao hmac: malformed response")
	}
	sum, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(sum) != 32 {
		return app.BlindIndex{}, errors.New("openbao hmac: malformed response")
	}
	return app.BlindIndex{Sum: sum, KeyVersion: v}, nil
}

// RenewSelf renews the periodic token and returns its new TTL.
func (c *Client) RenewSelf(ctx context.Context) (time.Duration, error) {
	var out struct {
		Auth struct {
			LeaseDuration int `json:"lease_duration"`
		} `json:"auth"`
	}
	if err := c.call(ctx, "renew-self", "auth/token/renew-self", map[string]any{}, &out); err != nil {
		return 0, err
	}
	return time.Duration(out.Auth.LeaseDuration) * time.Second, nil
}

// RenewLoop renews the token every TTL/2 until ctx ends (DD-11). Failures
// are logged as openbao_token_renew_failed and retried every 30 s; requests
// keep failing closed (503) while the token is invalid.
func (c *Client) RenewLoop(ctx context.Context) {
	for {
		ttl, err := c.RenewSelf(ctx)
		wait := ttl / 2
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.WarnContext(ctx, "openbao_token_renew_failed", "error", err.Error())
			wait = 30 * time.Second
		}
		wait = min(max(wait, 5*time.Second), 12*time.Hour)
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// Pseudonym implements app.LoginKeys (transit/hmac sha2-256 on the login
// HMAC key, key_version pinned to 1).
func (c *Client) Pseudonym(ctx context.Context, input []byte) (login.Pseudonym, error) {
	var out struct {
		Data struct {
			HMAC string `json:"hmac"`
		} `json:"data"`
	}
	err := c.call(ctx, "login hmac", "transit/hmac/"+c.loginHMAC+"/sha2-256", map[string]any{
		"input": base64.StdEncoding.EncodeToString(input), "key_version": LoginHMACKeyVersion,
	}, &out)
	if err != nil {
		return login.Pseudonym{}, err
	}
	v, b64, ok := parseVersioned(out.Data.HMAC)
	sum, derr := base64.StdEncoding.DecodeString(b64)
	if !ok || v != LoginHMACKeyVersion || derr != nil || len(sum) != login.PseudonymLen {
		return login.Pseudonym{}, errors.New("openbao login hmac: malformed response")
	}
	var p login.Pseudonym
	copy(p[:], sum)
	return p, nil
}

// SealLogin implements app.LoginKeys (transit/encrypt with associated_data).
func (c *Client) SealLogin(ctx context.Context, ad, plaintext []byte) (string, int, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	err := c.call(ctx, "login encrypt", "transit/encrypt/"+c.loginKEK, map[string]any{
		"plaintext":       base64.StdEncoding.EncodeToString(plaintext),
		"associated_data": base64.StdEncoding.EncodeToString(ad),
	}, &out)
	if err != nil {
		return "", 0, err
	}
	v, _, ok := parseVersioned(out.Data.Ciphertext)
	if !ok || len(out.Data.Ciphertext) > 2048 {
		return "", 0, errors.New("openbao login encrypt: malformed ciphertext")
	}
	return out.Data.Ciphertext, v, nil
}

// OpenLogins implements app.LoginKeys (transit/decrypt with batch_input, in
// chunks of maxBatch). OpenBao answers a batch with a per-item failure with
// status 400 and per-item results (spike S6); those items become
// app.ErrDataIntegrity, the others still decrypt.
func (c *Client) OpenLogins(ctx context.Context, items []app.SealedLogin) ([][]byte, []error, error) {
	pts := make([][]byte, len(items))
	errs := make([]error, len(items))
	for start := 0; start < len(items); start += maxBatch {
		end := min(start+maxBatch, len(items))
		if err := c.openChunk(ctx, items[start:end], pts[start:end], errs[start:end]); err != nil {
			for _, p := range pts {
				clear(p)
			}
			return nil, nil, err
		}
	}
	return pts, errs, nil
}

func (c *Client) openChunk(ctx context.Context, items []app.SealedLogin, pts [][]byte, errs []error) error {
	const op = "login decrypt"
	batch := make([]map[string]string, len(items))
	for i, it := range items {
		batch[i] = map[string]string{
			"ciphertext": it.Ciphertext, "associated_data": base64.StdEncoding.EncodeToString(it.AD),
		}
	}
	status, b, err := c.do(ctx, op, "transit/decrypt/"+c.loginKEK, map[string]any{"batch_input": batch})
	if err != nil {
		return err
	}
	defer clear(b)
	if status != http.StatusOK && status != http.StatusBadRequest {
		return &statusError{op: op, status: status}
	}
	var out struct {
		Data struct {
			BatchResults []struct {
				Plaintext string `json:"plaintext"`
				Error     string `json:"error"`
			} `json:"batch_results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil || len(out.Data.BatchResults) != len(items) {
		if status == http.StatusBadRequest {
			return unavailable(op, "status 400")
		}
		return fmt.Errorf("openbao %s: decode response", op)
	}
	for i, r := range out.Data.BatchResults {
		if r.Error != "" {
			errs[i] = fmt.Errorf("openbao %s: %w", op, app.ErrDataIntegrity)
			continue
		}
		pt, err := base64.StdEncoding.DecodeString(r.Plaintext)
		if err != nil {
			errs[i] = fmt.Errorf("openbao %s: %w", op, app.ErrDataIntegrity)
			continue
		}
		pts[i] = pt
		out.Data.BatchResults[i].Plaintext = ""
	}
	return nil
}
