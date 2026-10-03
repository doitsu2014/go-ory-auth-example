package hydra

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

// Client registration constants (§6 A10).
const (
	grantClientCredentials = "client_credentials"
	authMethodBasic        = "client_secret_basic"
	strategyJWT            = "jwt"
	listPageSize           = 500
	maxListPages           = 20
)

// Admin implements app.ServiceClientAdmin over the Hydra admin API. Only
// clients that pass managed are ever returned.
type Admin struct {
	c        client
	audience string
	tagKey   []byte
}

var _ app.ServiceClientAdmin = (*Admin)(nil)

// MinTagKeyBytes is the minimum length of the client integrity key.
const MinTagKeyBytes = 32

// NewAdmin creates the adapter for the Hydra admin URL. audience is the
// resource server audience every managed client must have; tagKey (≥ 32
// bytes, M2M_CLIENT_TAG_KEY) authenticates the client registration (S4).
func NewAdmin(adminURL, audience string, tagKey []byte, hc *http.Client) (*Admin, error) {
	if len(tagKey) < MinTagKeyBytes {
		return nil, errors.New("hydra admin: client tag key must be at least 32 bytes")
	}
	return &Admin{
		c: client{base: strings.TrimRight(adminURL, "/"), http: newHTTPClient(hc, DefaultTimeout)}, audience: audience,
		tagKey: append([]byte(nil), tagKey...),
	}, nil
}

// integrityTag binds a registration to identity-service (security S4): the
// Hydra admin API is unauthenticated, so anyone reaching it could create a
// client with managed_by=identity-service; without the key they cannot
// produce this tag. Input: client_id | sorted scopes | audience | created_by.
func integrityTag(key []byte, clientID string, scopes []string, audience, createdBy string) string {
	sorted := append([]string(nil), scopes...)
	slices.Sort(sorted)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(clientID + "|" + strings.Join(sorted, " ") + "|" + audience + "|" + createdBy))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// hClient is the subset of a Hydra OAuth2 client used here.
type hClient struct {
	ClientID                string          `json:"client_id"`
	ClientName              string          `json:"client_name"`
	ClientSecret            string          `json:"client_secret,omitempty"`
	GrantTypes              []string        `json:"grant_types"`
	ResponseTypes           []string        `json:"response_types"`
	Scope                   string          `json:"scope"`
	Audience                []string        `json:"audience"`
	TokenEndpointAuthMethod string          `json:"token_endpoint_auth_method"`
	AccessTokenStrategy     string          `json:"access_token_strategy"`
	CreatedAt               time.Time       `json:"created_at"`
	Metadata                json.RawMessage `json:"metadata"`
}

type hMetadata struct {
	ManagedBy string `json:"managed_by"`
	Owner     string `json:"owner"`
	CreatedBy string `json:"created_by"`
	// TokensValidAfter (unix seconds) is set on rotation (§6 B3).
	TokensValidAfter int64 `json:"tokens_valid_after,omitempty"`
	// Integrity is the HMAC tag of the registration (see integrityTag).
	Integrity string `json:"integrity,omitempty"`
}

// metadata decodes the metadata; ok=false when it is absent or malformed
// (such a client is never treated as managed).
func (c hClient) metadata() (hMetadata, bool) {
	var m hMetadata
	if len(c.Metadata) == 0 || json.Unmarshal(c.Metadata, &m) != nil {
		return hMetadata{}, false
	}
	return m, true
}

// managed reports whether c is a client identity-service registered and
// that can only obtain tokens for this service: metadata.managed_by,
// grant_types == [client_credentials], audience == [audience] (§6 B2) and a
// valid integrity tag (S4; absent or wrong → not managed).
func (a *Admin) managed(c hClient) (hMetadata, bool) {
	m, ok := c.metadata()
	if !ok || m.ManagedBy != machine.ManagedBy {
		return hMetadata{}, false
	}
	if !slices.Equal(c.GrantTypes, []string{grantClientCredentials}) || !slices.Equal(c.Audience, []string{a.audience}) {
		return hMetadata{}, false
	}
	got, err := base64.RawURLEncoding.DecodeString(m.Integrity)
	if err != nil || len(got) != sha256.Size {
		return hMetadata{}, false
	}
	want, _ := base64.RawURLEncoding.DecodeString(integrityTag(a.tagKey, c.ClientID, strings.Fields(c.Scope), a.audience, m.CreatedBy))
	if !hmac.Equal(got, want) {
		return hMetadata{}, false
	}
	return m, true
}

func (c hClient) toDomain(m hMetadata) machine.ServiceClient {
	out := machine.ServiceClient{
		ClientID: c.ClientID, Name: c.ClientName, Owner: m.Owner,
		Scopes: machine.KnownScopes(strings.Fields(c.Scope)), CreatedAt: c.CreatedAt.UTC(),
	}
	if id, err := uuid.Parse(m.CreatedBy); err == nil {
		out.CreatedBy = &id
	}
	return out
}

func clientPath(id string) string { return "/admin/clients/" + url.PathEscape(id) }

// Create implements app.ServiceClientAdmin. The body is explicit (§6 A10):
// client_secret_basic, client_credentials only, response type token, the
// service audience, JWT access tokens and no redirect URIs, JWKS or custom
// lifespans. The client_id is chosen by the caller (so the integrity tag can
// be set atomically and an ambiguous failure can be compensated); Hydra
// generates the secret.
func (a *Admin) Create(ctx context.Context, in app.NewServiceClient) (machine.ServiceClient, string, error) {
	r := in.Registration
	if in.ClientID == "" {
		return machine.ServiceClient{}, "", errors.New("hydra create client: client_id is required")
	}
	scopes := strings.Fields(machine.ScopeString(r.Scopes))
	createdBy := in.CreatedBy.String()
	body := map[string]any{
		"client_id":                  in.ClientID,
		"client_name":                r.Name,
		"grant_types":                []string{grantClientCredentials},
		"response_types":             []string{"token"},
		"scope":                      machine.ScopeString(r.Scopes),
		"audience":                   []string{a.audience},
		"token_endpoint_auth_method": authMethodBasic,
		"access_token_strategy":      strategyJWT,
		"metadata": hMetadata{
			ManagedBy: machine.ManagedBy, Owner: r.Owner, CreatedBy: createdBy,
			Integrity: integrityTag(a.tagKey, in.ClientID, scopes, a.audience, createdBy),
		},
	}
	resp, err := a.c.do(ctx, http.MethodPost, "/admin/clients", "application/json", body)
	if err != nil {
		return machine.ServiceClient{}, "", err
	}
	defer clear(resp.body)
	switch resp.status {
	case http.StatusCreated:
	case http.StatusConflict:
		return machine.ServiceClient{}, "", fmt.Errorf("%w: client id already exists", app.ErrConflict)
	default:
		return machine.ServiceClient{}, "", resp.unexpected("create client")
	}
	var c hClient
	if err := json.Unmarshal(resp.body, &c); err != nil || c.ClientID == "" {
		return machine.ServiceClient{}, "", errors.New("hydra create client: malformed response")
	}
	m, ok := a.managed(c)
	if !ok || c.ClientSecret == "" || c.ClientID != in.ClientID {
		_ = a.Delete(context.WithoutCancel(ctx), c.ClientID)
		return machine.ServiceClient{}, "", errors.New("hydra create client: response does not match the registration")
	}
	return c.toDomain(m), c.ClientSecret, nil
}

// getRaw fetches one client; found=false on 404 (and 400, a malformed id).
// Every other non-200 (401/403/429 from a proxy, redirects, …) is
// ErrDependencyUnavailable: it says nothing about the client and must never
// become a cached "not found".
func (a *Admin) getRaw(ctx context.Context, id string) (hClient, bool, error) {
	resp, err := a.c.do(ctx, http.MethodGet, clientPath(id), "", nil)
	if err != nil {
		return hClient{}, false, err
	}
	switch resp.status {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusBadRequest:
		return hClient{}, false, nil
	default:
		return hClient{}, false, fmt.Errorf("%w: hydra get client: status %d", app.ErrDependencyUnavailable, resp.status)
	}
	var c hClient
	if err := json.Unmarshal(resp.body, &c); err != nil {
		return hClient{}, false, fmt.Errorf("%w: hydra get client: malformed response", app.ErrDependencyUnavailable)
	}
	c.ClientSecret = ""
	return c, true, nil
}

// Get implements app.ServiceClientAdmin.
func (a *Admin) Get(ctx context.Context, id string) (machine.ServiceClient, error) {
	c, found, err := a.getRaw(ctx, id)
	if err != nil {
		return machine.ServiceClient{}, err
	}
	m, ok := a.managed(c)
	if !found || !ok || c.ClientID != id {
		return machine.ServiceClient{}, app.ErrNotFound
	}
	return c.toDomain(m), nil
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// List implements app.ServiceClientAdmin. Hydra cannot filter by metadata,
// so this pages through all clients (at most maxListPages × listPageSize)
// and keeps the managed ones.
func (a *Admin) List(ctx context.Context) ([]machine.ServiceClient, error) {
	out := []machine.ServiceClient{}
	token := ""
	for range maxListPages {
		v := url.Values{}
		v.Set("page_size", strconv.Itoa(listPageSize))
		if token != "" {
			v.Set("page_token", token)
		}
		resp, err := a.c.do(ctx, http.MethodGet, "/admin/clients?"+v.Encode(), "", nil)
		if err != nil {
			return nil, err
		}
		if resp.status != http.StatusOK {
			return nil, resp.unexpected("list clients")
		}
		var page []hClient
		if err := json.Unmarshal(resp.body, &page); err != nil {
			return nil, fmt.Errorf("%w: hydra list clients: malformed response", app.ErrDependencyUnavailable)
		}
		for _, c := range page {
			c.ClientSecret = ""
			if m, ok := a.managed(c); ok {
				out = append(out, c.toDomain(m))
			}
		}
		token = nextPageToken(resp.header)
		if token == "" || len(page) == 0 {
			break
		}
	}
	slices.SortFunc(out, func(x, y machine.ServiceClient) int {
		if c := y.CreatedAt.Compare(x.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(x.ClientID, y.ClientID)
	})
	return out, nil
}

func nextPageToken(h http.Header) string {
	for _, l := range h.Values("Link") {
		m := nextLinkRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		u, err := url.Parse(m[1])
		if err != nil {
			return ""
		}
		return u.Query().Get("page_token")
	}
	return ""
}

// SetSecret implements app.ServiceClientAdmin with a JSON patch of
// /client_secret and /metadata/tokens_valid_after (§6 A10, B3).
func (a *Admin) SetSecret(ctx context.Context, id, secret string, tokensValidAfter time.Time) error {
	patch := []map[string]any{
		{"op": "replace", "path": "/client_secret", "value": secret},
		{"op": "add", "path": "/metadata/tokens_valid_after", "value": tokensValidAfter.Unix()},
	}
	resp, err := a.c.do(ctx, http.MethodPatch, clientPath(id), "application/json", patch)
	if err != nil {
		return err
	}
	clear(resp.body) // the response may echo the client
	switch resp.status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return app.ErrNotFound
	}
	return resp.unexpected("patch client")
}

// Delete implements app.ServiceClientAdmin.
func (a *Admin) Delete(ctx context.Context, id string) error {
	resp, err := a.c.do(ctx, http.MethodDelete, clientPath(id), "", nil)
	if err != nil {
		return err
	}
	switch resp.status {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return app.ErrNotFound
	}
	return resp.unexpected("delete client")
}

// clientStatus is the cached result of a client-status lookup.
type clientStatus struct {
	ok               bool
	scopes           []machine.Scope
	tokensValidAfter int64
}

// status looks a client up for the verifier. A missing, unmanaged or
// mis-configured client is a (cacheable) negative result; transport errors
// and 5xx are ErrDependencyUnavailable.
func (a *Admin) status(ctx context.Context, id string) (clientStatus, error) {
	c, found, err := a.getRaw(ctx, id)
	if err != nil {
		return clientStatus{}, err
	}
	if !found || c.ClientID != id {
		return clientStatus{}, nil
	}
	m, ok := a.managed(c)
	if !ok {
		return clientStatus{}, nil
	}
	return clientStatus{ok: true, scopes: machine.KnownScopes(strings.Fields(c.Scope)), tokensValidAfter: m.TokensValidAfter}, nil
}

// Ready checks Hydra admin readiness.
func (a *Admin) Ready(ctx context.Context) error {
	resp, err := a.c.do(ctx, http.MethodGet, "/health/ready", "", nil)
	if err != nil {
		return err
	}
	if resp.status != http.StatusOK {
		return fmt.Errorf("hydra admin not ready: %d", resp.status)
	}
	return nil
}
