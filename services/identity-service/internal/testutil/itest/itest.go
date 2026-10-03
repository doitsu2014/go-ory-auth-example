//go:build integration

// Package itest holds helpers for integration tests against the local stack
// (`make infra-up`). Defaults match deploy/compose/.env.example; override with
// the same env vars the service uses.
package itest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP uses HMAC-SHA1.
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Env is the integration environment.
type Env struct {
	DatabaseURL, MigrateURL   string
	KratosPublic, KratosAdmin string
	KetoRead, KetoWrite       string
	MailpitAPI, SMTPURL       string
	// OpenBaoAddr and OpenBaoTokenFile reach the local OpenBao with the
	// identity-service app token (`make bao-token` at the repo root).
	OpenBaoAddr, OpenBaoTokenFile string
	// HydraAdmin (127.0.0.1 only) and HydraPublic (token endpoint, JWKS).
	// HydraIssuer is the exact iss of Hydra tokens.
	HydraAdmin, HydraPublic, HydraIssuer string
	// ClientTagKey is the in-process service's M2M_CLIENT_TAG_KEY (tests run
	// their own service instance, so any ≥ 32-byte key works).
	ClientTagKey []byte
}

// RepoRoot walks up from the working directory to the repository root
// (the directory holding deploy/compose/docker-compose.yml).
func RepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "deploy", "compose", "docker-compose.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// RequireBaoToken fails with a hint when the app token file is missing.
func (e Env) RequireBaoToken(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(e.OpenBaoTokenFile); err != nil {
		t.Fatalf("OpenBao app token not found at %s: run `make bao-token` at the repo root", e.OpenBaoTokenFile)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Load returns the environment.
func Load() Env {
	return Env{
		DatabaseURL:  getenv("DATABASE_URL", "postgres://identity_app:local-identity-app-pw@127.0.0.1:5432/identity?sslmode=disable"),
		MigrateURL:   getenv("MIGRATE_DATABASE_URL", "postgres://identity_migrator:local-identity-migrator-pw@127.0.0.1:5432/identity?sslmode=disable"),
		KratosPublic: getenv("KRATOS_PUBLIC_URL", "http://localhost:4433"),
		KratosAdmin:  getenv("KRATOS_ADMIN_URL", "http://127.0.0.1:4434"),
		KetoRead:     getenv("KETO_READ_URL", "http://127.0.0.1:4466"),
		KetoWrite:    getenv("KETO_WRITE_URL", "http://127.0.0.1:4467"),
		MailpitAPI:   getenv("MAILPIT_API_URL", "http://127.0.0.1:8025"),
		SMTPURL:      getenv("SMTP_URL", "smtp://127.0.0.1:1025"),
		OpenBaoAddr:  getenv("PII_OPENBAO_ADDR", "http://127.0.0.1:8200"),
		OpenBaoTokenFile: getenv("PII_OPENBAO_TOKEN_FILE",
			filepath.Join(RepoRoot(), "deploy", "compose", ".local", "identity-service.token")),
		HydraAdmin:   getenv("HYDRA_ADMIN_URL", "http://127.0.0.1:4445"),
		HydraPublic:  getenv("HYDRA_PUBLIC_URL", "http://localhost:4444"),
		HydraIssuer:  getenv("M2M_ISSUER", "http://localhost:4444"),
		ClientTagKey: []byte("itest-only-client-tag-key-000000"),
	}
}

// Password is a strong password for test identities.
const Password = "correct-horse-battery-staple-91!"

// UniqueEmail returns a fresh address.
func UniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%s@example.local", prefix, strings.ReplaceAll(uuid.NewString()[:13], "-", ""))
}

// JSON does an HTTP request with a JSON body and decodes a JSON response.
func JSON(t *testing.T, c *http.Client, method, u string, hdr http.Header, body, out any) (int, http.Header) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, u, rdr)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json")
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s %s: decode %d %s: %v", method, u, resp.StatusCode, b, err)
		}
	}
	return resp.StatusCode, resp.Header
}

// Flow is the subset of a Kratos flow used by tests.
type Flow struct {
	ID string `json:"id"`
	UI struct {
		Nodes []struct {
			Attributes struct {
				Name  string `json:"name"`
				Value any    `json:"value"`
				ID    string `json:"id"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"attributes"`
		} `json:"nodes"`
		Messages []struct {
			ID   int    `json:"id"`
			Text string `json:"text"`
		} `json:"messages"`
	} `json:"ui"`
	ContinueWith []struct {
		Action string `json:"action"`
		Flow   struct {
			ID string `json:"id"`
		} `json:"flow"`
	} `json:"continue_with"`
	SessionToken string `json:"session_token"`
	Session      struct {
		ID       string `json:"id"`
		AAL      string `json:"authenticator_assurance_level"`
		Identity struct {
			ID string `json:"id"`
		} `json:"identity"`
	} `json:"session"`
}

// CSRF returns the csrf_token node value.
func (f Flow) CSRF() string {
	for _, n := range f.UI.Nodes {
		if n.Attributes.Name == "csrf_token" {
			s, _ := n.Attributes.Value.(string)
			return s
		}
	}
	return ""
}

// TOTPSecret returns the secret shown in a settings flow.
func (f Flow) TOTPSecret() string {
	for _, n := range f.UI.Nodes {
		if n.Attributes.ID == "totp_secret_key" {
			return n.Attributes.Text.Text
		}
	}
	return ""
}

// RegisterCustomer registers via the native API flow and returns the flow
// response (session token, identity id, verification flow id).
func (e Env) RegisterCustomer(t *testing.T, email string) Flow {
	t.Helper()
	var flow Flow
	if st, _ := JSON(t, nil, "GET", e.KratosPublic+"/self-service/registration/api", nil, nil, &flow); st != 200 {
		t.Fatalf("registration flow: %d", st)
	}
	var out Flow
	st, _ := JSON(t, nil, "POST", e.KratosPublic+"/self-service/registration?flow="+flow.ID, nil, map[string]any{
		"method": "password", "password": Password, "traits": map[string]any{"email": email},
	}, &out)
	if st != 200 || out.SessionToken == "" {
		t.Fatalf("register: %d %+v", st, out.UI.Messages)
	}
	return out
}

// RegisterCustomerTraits submits a native registration with arbitrary traits
// and returns the HTTP status (for schema checks).
func (e Env) RegisterCustomerTraits(t *testing.T, traits map[string]any) int {
	t.Helper()
	var flow Flow
	if st, _ := JSON(t, nil, "GET", e.KratosPublic+"/self-service/registration/api", nil, nil, &flow); st != 200 {
		t.Fatalf("registration flow: %d", st)
	}
	var out Flow
	st, _ := JSON(t, nil, "POST", e.KratosPublic+"/self-service/registration?flow="+flow.ID, nil, map[string]any{
		"method": "password", "password": Password, "traits": traits,
	}, &out)
	if out.Session.Identity.ID != "" {
		if id, err := uuid.Parse(out.Session.Identity.ID); err == nil {
			e.DeleteIdentity(t, id)
		}
	}
	return st
}

// LoginAPI logs in via the native API flow; returns status and flow response.
func (e Env) LoginAPI(t *testing.T, email string) (int, Flow) {
	t.Helper()
	var flow Flow
	JSON(t, nil, "GET", e.KratosPublic+"/self-service/login/api", nil, nil, &flow)
	var out Flow
	st, _ := JSON(t, nil, "POST", e.KratosPublic+"/self-service/login?flow="+flow.ID, nil,
		map[string]any{"method": "password", "identifier": email, "password": Password}, &out)
	return st, out
}

// Browser is a cookie-carrying browser-flow client.
type Browser struct {
	Env  Env
	HTTP *http.Client
}

// NewBrowser creates a browser client with a cookie jar.
func (e Env) NewBrowser() *Browser {
	jar, _ := cookiejar.New(nil)
	return &Browser{Env: e, HTTP: &http.Client{Jar: jar, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// SessionCookie returns the ory_kratos_session cookie value.
func (b *Browser) SessionCookie() string {
	u, _ := url.Parse(b.Env.KratosPublic)
	for _, c := range b.HTTP.Jar.Cookies(u) {
		if c.Name == "ory_kratos_session" {
			return c.Value
		}
	}
	return ""
}

// Login runs a browser login flow with password (aal1) or TOTP (aal2).
func (b *Browser) Login(t *testing.T, email, totpSecret string) (int, Flow) {
	t.Helper()
	path := "/self-service/login/browser"
	if totpSecret != "" {
		path += "?aal=aal2"
	}
	var flow Flow
	if st, _ := JSON(t, b.HTTP, "GET", b.Env.KratosPublic+path, nil, nil, &flow); st != 200 {
		t.Fatalf("browser login flow: %d", st)
	}
	body := map[string]any{"csrf_token": flow.CSRF()}
	if totpSecret != "" {
		body["method"], body["totp_code"] = "totp", TOTP(totpSecret, time.Now())
	} else {
		body["method"], body["identifier"], body["password"] = "password", email, Password
	}
	var out Flow
	st, _ := JSON(t, b.HTTP, "POST", b.Env.KratosPublic+"/self-service/login?flow="+flow.ID, nil, body, &out)
	return st, out
}

// EnrolTOTP enrols TOTP through the browser settings flow and returns the secret.
func (b *Browser) EnrolTOTP(t *testing.T) string {
	t.Helper()
	var flow Flow
	if st, _ := JSON(t, b.HTTP, "GET", b.Env.KratosPublic+"/self-service/settings/browser", nil, nil, &flow); st != 200 {
		t.Fatalf("settings flow: %d", st)
	}
	secret := flow.TOTPSecret()
	if secret == "" {
		t.Fatal("no totp secret in settings flow")
	}
	var out Flow
	st, _ := JSON(t, b.HTTP, "POST", b.Env.KratosPublic+"/self-service/settings?flow="+flow.ID, nil,
		map[string]any{"method": "totp", "totp_code": TOTP(secret, time.Now()), "csrf_token": flow.CSRF()}, &out)
	if st != 200 {
		t.Fatalf("enrol totp: %d %+v", st, out.UI.Messages)
	}
	return secret
}

// CreateAdminWithPassword creates an admin identity with a password via the
// Kratos admin API (test-only shortcut for the invitation flow).
func (e Env) CreateAdminWithPassword(t *testing.T, email string) uuid.UUID {
	t.Helper()
	var out struct {
		ID uuid.UUID `json:"id"`
	}
	st, _ := JSON(t, nil, "POST", e.KratosAdmin+"/admin/identities", nil, map[string]any{
		"schema_id": "admin", "state": "active", "traits": map[string]any{"email": email},
		"credentials": map[string]any{"password": map[string]any{"config": map[string]any{"password": Password}}},
	}, &out)
	if st != 201 {
		t.Fatalf("create admin: %d", st)
	}
	return out.ID
}

// DeleteIdentity removes an identity (cleanup).
func (e Env) DeleteIdentity(t *testing.T, id uuid.UUID) {
	JSON(t, nil, "DELETE", e.KratosAdmin+"/admin/identities/"+id.String(), nil, nil, nil)
}

// TOTP computes an RFC 6238 code (SHA1, 6 digits, 30 s).
func TOTP(secret string, at time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil {
		return ""
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}

var sixDigits = regexp.MustCompile(`\b(\d{6})\b`)

// MailBody waits for the newest message to addr and returns its text body.
func (e Env) MailBody(t *testing.T, addr string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var res struct {
			Messages []struct {
				ID string `json:"ID"`
			} `json:"messages"`
		}
		JSON(t, nil, "GET", e.MailpitAPI+"/api/v1/search?query="+url.QueryEscape("to:"+addr), nil, nil, &res)
		if len(res.Messages) > 0 {
			var msg struct {
				Text string `json:"Text"`
			}
			JSON(t, nil, "GET", e.MailpitAPI+"/api/v1/message/"+res.Messages[0].ID, nil, nil, &msg)
			return msg.Text
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("no mail to %s", addr)
	return ""
}

func (e Env) mailIDs(t *testing.T, addr string) []string {
	var res struct {
		Messages []struct {
			ID string `json:"ID"`
		} `json:"messages"`
	}
	JSON(t, nil, "GET", e.MailpitAPI+"/api/v1/search?query="+url.QueryEscape("to:"+addr), nil, nil, &res)
	ids := make([]string, len(res.Messages))
	for i, m := range res.Messages {
		ids[i] = m.ID
	}
	return ids
}

// VerifyEmail runs a native verification flow: request a code, read it from
// Mailpit, submit it. (The registration response carries no
// show_verification_ui continue_with in this Kratos config.)
func (e Env) VerifyEmail(t *testing.T, email string) {
	t.Helper()
	before := len(e.mailIDs(t, email))
	var flow Flow
	JSON(t, nil, "GET", e.KratosPublic+"/self-service/verification/api", nil, nil, &flow)
	if st, _ := JSON(t, nil, "POST", e.KratosPublic+"/self-service/verification?flow="+flow.ID, nil,
		map[string]any{"method": "code", "email": email}, nil); st != 200 {
		t.Fatalf("request verification code: %d", st)
	}
	var newest string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if ids := e.mailIDs(t, email); len(ids) > before {
			newest = ids[0]
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if newest == "" {
		t.Fatal("verification email not received")
	}
	var msg struct {
		Text string `json:"Text"`
	}
	JSON(t, nil, "GET", e.MailpitAPI+"/api/v1/message/"+newest, nil, nil, &msg)
	m := sixDigits.FindStringSubmatch(msg.Text)
	if m == nil {
		t.Fatal("no code in verification email")
	}
	var out Flow
	if st, _ := JSON(t, nil, "POST", e.KratosPublic+"/self-service/verification?flow="+flow.ID, nil,
		map[string]any{"method": "code", "code": m[1]}, &out); st != 200 {
		t.Fatalf("verify: %d %+v", st, out.UI.Messages)
	}
}

// JWKSURL is Hydra's public key set.
func (e Env) JWKSURL() string {
	return strings.TrimRight(e.HydraPublic, "/") + "/.well-known/jwks.json"
}

// ClientCredentialsToken asks Hydra's token endpoint for an access token
// (client_secret_basic). audience "" omits the parameter. It returns the
// HTTP status and the access token (empty unless 200).
func (e Env) ClientCredentialsToken(t *testing.T, clientID, secret, scope, audience string) (int, string) {
	t.Helper()
	form := url.Values{"grant_type": {"client_credentials"}}
	if scope != "" {
		form.Set("scope", scope)
	}
	if audience != "" {
		form.Set("audience", audience)
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", strings.TrimRight(e.HydraPublic, "/")+"/oauth2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("token endpoint: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, ""
	}
	if !strings.EqualFold(out.TokenType, "bearer") || out.AccessToken == "" {
		t.Fatalf("token response: type %q", out.TokenType)
	}
	return resp.StatusCode, out.AccessToken
}

// DeleteHydraClient removes a Hydra client (cleanup; 404 is fine).
func (e Env) DeleteHydraClient(t *testing.T, clientID string) {
	JSON(t, nil, "DELETE", strings.TrimRight(e.HydraAdmin, "/")+"/admin/clients/"+url.PathEscape(clientID), nil, nil, nil)
}
