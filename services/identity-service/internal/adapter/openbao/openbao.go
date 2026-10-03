// Package openbao implements app.KeyManager over the OpenBao (Vault
// compatible) Transit secrets engine with a thin hand-written HTTP client
// (verified against OpenBao 2.4.1):
//
//   - POST /v1/transit/encrypt/<kek>            wrap a DEK (associated_data = subject)
//   - POST /v1/transit/decrypt/<kek>            unwrap a DEK (same associated_data)
//   - POST /v1/transit/hmac/<bidx>/sha2-256     blind index, key_version pinned to 1
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
)

// Defaults (technical spec §3).
const (
	DefaultTimeout     = 2 * time.Second
	DefaultKEKName     = "identity-pii-kek"
	DefaultBidxKeyName = "identity-pii-bidx"
	// BidxKeyVersion is pinned: the index key is never rotated in v1
	// (rotation would need a re-index job).
	BidxKeyVersion   = 1
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
	Timeout     time.Duration
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
	timeout   time.Duration
	http      *http.Client
	log       *slog.Logger

	mu      sync.Mutex
	token   string
	tokMod  time.Time
	tokSize int64
}

var _ app.KeyManager = (*Client)(nil)

// New validates the configuration. The token file is read lazily.
func New(cfg Config) (*Client, error) {
	if cfg.KEKName == "" {
		cfg.KEKName = DefaultKEKName
	}
	if cfg.BidxKeyName == "" {
		cfg.BidxKeyName = DefaultBidxKeyName
	}
	if !keyNameRe.MatchString(cfg.KEKName) || !keyNameRe.MatchString(cfg.BidxKeyName) {
		return nil, errors.New("openbao: invalid key name")
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

// call POSTs body to /v1/<path> and decodes the JSON response into out.
func (c *Client) call(ctx context.Context, op, path string, body, out any) error {
	tok, err := c.currentToken(op)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("openbao %s: encode request", op)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.addr+"/v1/"+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("openbao %s: build request", op)
	}
	req.Header.Set("X-Vault-Token", tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	clear(payload)
	if err != nil {
		return unavailable(op, "transport: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return unavailable(op, "read response")
	}
	defer clear(b)
	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent:
	case resp.StatusCode == http.StatusForbidden:
		c.forgetToken()
		return unavailable(op, "permission denied (status 403)")
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return unavailable(op, "status %d", resp.StatusCode)
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return unavailable(op, "unexpected redirect (status %d)", resp.StatusCode)
	case resp.StatusCode == http.StatusBadRequest:
		if bytes.Contains(b, []byte("message authentication failed")) || bytes.Contains(b, []byte("invalid ciphertext")) {
			return &statusError{op: op, status: resp.StatusCode, authFailed: true}
		}
		return unavailable(op, "status 400")
	default:
		return &statusError{op: op, status: resp.StatusCode}
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("openbao %s: decode response", op)
	}
	return nil
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
