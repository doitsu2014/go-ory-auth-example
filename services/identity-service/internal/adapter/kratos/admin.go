package kratos

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// Admin implements app.IdentityAdmin over the Kratos admin API (private).
type Admin struct{ c client }

var _ app.IdentityAdmin = (*Admin)(nil)

// NewAdmin creates the adapter for the Kratos admin URL.
func NewAdmin(adminURL string, hc *http.Client) *Admin {
	return &Admin{c: newClient(adminURL, hc)}
}

// GetIdentity implements app.IdentityAdmin.
func (a *Admin) GetIdentity(ctx context.Context, id uuid.UUID) (identity.Identity, error) {
	resp, err := a.c.do(ctx, http.MethodGet, "/admin/identities/"+id.String()+"?include_credential=totp", nil, nil)
	if err != nil {
		return identity.Identity{}, err
	}
	switch resp.status {
	case http.StatusOK:
	case http.StatusNotFound:
		return identity.Identity{}, app.ErrNotFound
	default:
		return identity.Identity{}, resp.unexpected("get identity")
	}
	var k kIdentity
	if err := resp.decode(&k); err != nil {
		return identity.Identity{}, err
	}
	return k.toDomain(), nil
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// ListIdentities implements app.IdentityQuery. Page tokens are opaque
// (base64url of the Kratos token).
func (a *Admin) ListIdentities(ctx context.Context, q app.IdentityQuery) ([]identity.Identity, string, error) {
	v := url.Values{}
	v.Set("page_size", strconv.Itoa(app.ClampPageSize(q.PageSize)))
	if q.PageToken != "" {
		raw, err := base64.RawURLEncoding.DecodeString(q.PageToken)
		if err != nil {
			return nil, "", app.NewValidationError("page_token", "invalid")
		}
		v.Set("page_token", string(raw))
	}
	if q.Email != "" {
		v.Set("credentials_identifier", q.Email)
	}
	if q.IncludeTOTP {
		v.Set("include_credential", "totp")
	}
	resp, err := a.c.do(ctx, http.MethodGet, "/admin/identities?"+v.Encode(), nil, nil)
	if err != nil {
		return nil, "", err
	}
	switch resp.status {
	case http.StatusOK:
	case http.StatusBadRequest:
		if q.PageToken != "" {
			return nil, "", app.NewValidationError("page_token", "invalid")
		}
		return nil, "", resp.unexpected("list identities")
	default:
		return nil, "", resp.unexpected("list identities")
	}
	var ks []kIdentity
	if err := resp.decode(&ks); err != nil {
		return nil, "", err
	}
	out := make([]identity.Identity, 0, len(ks))
	for _, k := range ks {
		out = append(out, k.toDomain())
	}
	next := ""
	// credentials_identifier is an exact match; no further pages.
	if q.Email == "" && len(ks) > 0 {
		next = nextToken(resp.header.Get("Link"))
	}
	return out, next, nil
}

func nextToken(link string) string {
	m := nextLinkRe.FindStringSubmatch(link)
	if m == nil {
		return ""
	}
	u, err := url.Parse(m[1])
	if err != nil {
		return ""
	}
	t := u.Query().Get("page_token")
	if t == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(t))
}

type createIdentityBody struct {
	SchemaID string `json:"schema_id"`
	State    string `json:"state"`
	Traits   traits `json:"traits"`
}

// CreateIdentity implements app.IdentityAdmin.
func (a *Admin) CreateIdentity(ctx context.Context, in app.NewIdentity) (identity.Identity, error) {
	body := createIdentityBody{SchemaID: in.SchemaID, State: string(identity.StateActive), Traits: traits{Email: in.Email}}
	if in.Name.First != "" || in.Name.Last != "" {
		body.Traits.Name = &struct {
			First string `json:"first,omitempty"`
			Last  string `json:"last,omitempty"`
		}{First: in.Name.First, Last: in.Name.Last}
	}
	resp, err := a.c.do(ctx, http.MethodPost, "/admin/identities", nil, body)
	if err != nil {
		return identity.Identity{}, err
	}
	switch resp.status {
	case http.StatusCreated, http.StatusOK:
	case http.StatusConflict:
		return identity.Identity{}, app.ErrConflict
	case http.StatusBadRequest:
		return identity.Identity{}, app.NewValidationError("traits", "invalid")
	default:
		return identity.Identity{}, resp.unexpected("create identity")
	}
	var k kIdentity
	if err := resp.decode(&k); err != nil {
		return identity.Identity{}, err
	}
	return k.toDomain(), nil
}

// DeleteIdentity implements app.IdentityAdmin.
func (a *Admin) DeleteIdentity(ctx context.Context, id uuid.UUID) error {
	resp, err := a.c.do(ctx, http.MethodDelete, "/admin/identities/"+id.String(), nil, nil)
	if err != nil {
		return err
	}
	if resp.status != http.StatusNoContent && resp.status != http.StatusNotFound {
		return resp.unexpected("delete identity")
	}
	return nil
}

type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// SetState implements app.IdentityAdmin (JSON Patch on /state).
func (a *Admin) SetState(ctx context.Context, id uuid.UUID, state identity.State) error {
	resp, err := a.c.do(ctx, http.MethodPatch, "/admin/identities/"+id.String(), nil,
		[]jsonPatchOp{{Op: "replace", Path: "/state", Value: string(state)}})
	if err != nil {
		return err
	}
	switch resp.status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return app.ErrNotFound
	}
	return resp.unexpected("patch identity state")
}

// RevokeIdentitySessions implements app.IdentityAdmin. Kratos answers 404
// when the identity has no sessions; that is success here.
func (a *Admin) RevokeIdentitySessions(ctx context.Context, id uuid.UUID) error {
	resp, err := a.c.do(ctx, http.MethodDelete, "/admin/identities/"+id.String()+"/sessions", nil, nil)
	if err != nil {
		return err
	}
	if resp.status != http.StatusNoContent && resp.status != http.StatusNotFound {
		return resp.unexpected("revoke identity sessions")
	}
	return nil
}

// RevokeSession implements app.IdentityAdmin.
func (a *Admin) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	resp, err := a.c.do(ctx, http.MethodDelete, "/admin/sessions/"+sessionID.String(), nil, nil)
	if err != nil {
		return err
	}
	if resp.status != http.StatusNoContent && resp.status != http.StatusNotFound {
		return resp.unexpected("revoke session")
	}
	return nil
}

type recoveryCodeBody struct {
	IdentityID uuid.UUID `json:"identity_id"`
	ExpiresIn  string    `json:"expires_in"`
}

type recoveryCodeResp struct {
	RecoveryLink string    `json:"recovery_link"`
	RecoveryCode string    `json:"recovery_code"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// CreateRecoveryCode implements app.IdentityAdmin. The result is a secret.
func (a *Admin) CreateRecoveryCode(ctx context.Context, id uuid.UUID, ttl time.Duration) (app.RecoveryCode, error) {
	resp, err := a.c.do(ctx, http.MethodPost, "/admin/recovery/code", nil,
		recoveryCodeBody{IdentityID: id, ExpiresIn: fmt.Sprintf("%ds", int(ttl.Seconds()))})
	if err != nil {
		return app.RecoveryCode{}, err
	}
	switch resp.status {
	case http.StatusCreated, http.StatusOK:
	case http.StatusNotFound:
		return app.RecoveryCode{}, app.ErrNotFound
	default:
		return app.RecoveryCode{}, resp.unexpected("create recovery code")
	}
	var r recoveryCodeResp
	if err := resp.decode(&r); err != nil {
		return app.RecoveryCode{}, err
	}
	return app.RecoveryCode{Link: r.RecoveryLink, Code: r.RecoveryCode, ExpiresAt: r.ExpiresAt}, nil
}

// Ready checks Kratos admin readiness.
func (a *Admin) Ready(ctx context.Context) error {
	resp, err := a.c.do(ctx, http.MethodGet, "/admin/health/ready", nil, nil)
	if err != nil {
		return err
	}
	if resp.status != http.StatusOK {
		return fmt.Errorf("kratos admin not ready: %d", resp.status)
	}
	return nil
}

var _ app.NameTraitAdmin = (*Admin)(nil)

type jsonPatchRemove struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// RemoveTraitName implements app.NameTraitAdmin. Kratos v26 rejects the JSON
// Patch "test" operation ("unsupported operation: test"), so the check is a
// read-compare-remove: the identity is re-read, the name trait must still
// equal old (else ErrConflict), then PATCH [remove /traits/name]. The window
// between read and patch is not atomic; the customer schema no longer
// accepts a name, so only an operator could change it meanwhile. The name is
// never logged or put into errors.
func (a *Admin) RemoveTraitName(ctx context.Context, id uuid.UUID, old identity.Name) error {
	for attempt := 0; ; attempt++ {
		present, cur, err := a.nameTrait(ctx, id)
		if err != nil || !present {
			return err
		}
		if cur != old {
			return fmt.Errorf("%w: name trait changed", app.ErrConflict)
		}
		resp, err := a.c.do(ctx, http.MethodPatch, "/admin/identities/"+id.String(), nil,
			[]jsonPatchRemove{{Op: "remove", Path: "/traits/name"}})
		if err != nil {
			return err
		}
		switch resp.status {
		case http.StatusOK:
			return nil
		case http.StatusNotFound:
			return app.ErrNotFound
		case http.StatusBadRequest, http.StatusConflict:
			if attempt == 0 { // e.g. removed concurrently: re-read once
				continue
			}
		}
		return resp.unexpected("patch identity name")
	}
}

// nameTrait reads whether traits.name is present (even as an empty object)
// and its value.
func (a *Admin) nameTrait(ctx context.Context, id uuid.UUID) (bool, identity.Name, error) {
	resp, err := a.c.do(ctx, http.MethodGet, "/admin/identities/"+id.String(), nil, nil)
	if err != nil {
		return false, identity.Name{}, err
	}
	switch resp.status {
	case http.StatusOK:
	case http.StatusNotFound:
		return false, identity.Name{}, app.ErrNotFound
	default:
		return false, identity.Name{}, resp.unexpected("get identity")
	}
	var k kIdentity
	if err := resp.decode(&k); err != nil {
		return false, identity.Name{}, err
	}
	return k.Traits.Name != nil, k.name(), nil
}
