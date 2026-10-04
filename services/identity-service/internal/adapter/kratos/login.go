package kratos

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

var _ app.LoginTraitAdmin = (*Admin)(nil)

// getRaw reads the raw identity (traits and verifiable addresses).
func (a *Admin) getRaw(ctx context.Context, id uuid.UUID) (kIdentity, error) {
	resp, err := a.c.do(ctx, http.MethodGet, "/admin/identities/"+id.String(), nil, nil)
	if err != nil {
		return kIdentity{}, err
	}
	switch resp.status {
	case http.StatusOK:
	case http.StatusNotFound:
		return kIdentity{}, app.ErrNotFound
	default:
		return kIdentity{}, resp.unexpected("get identity")
	}
	var k kIdentity
	if err := resp.decode(&k); err != nil {
		return kIdentity{}, err
	}
	return k, nil
}

// ReplaceLoginTrait implements app.LoginTraitAdmin (PLI-FR-13). Kratos v26
// rejects the JSON Patch "test" operation, so this is read-compare-patch:
// the identity is re-read and must still be a legacy customer with
// traits.email == oldEmail (else ErrConflict); then one PATCH removes the
// email and adds login_id. Kratos re-derives the credential identifier and
// the verifiable/recovery addresses and deletes the old rows (spike S4); the
// new verifiable address is unverified until MarkLoginVerified. Values are
// never logged or put into errors.
func (a *Admin) ReplaceLoginTrait(ctx context.Context, id uuid.UUID, oldEmail, loginID string) error {
	k, err := a.getRaw(ctx, id)
	if err != nil {
		return err
	}
	if k.SchemaID != "customer" || k.Traits.LoginID != "" || !strings.EqualFold(k.Traits.Email, oldEmail) {
		return fmt.Errorf("%w: login trait changed", app.ErrConflict)
	}
	resp, err := a.c.do(ctx, http.MethodPatch, "/admin/identities/"+id.String(), nil, []any{
		jsonPatchRemove{Op: "remove", Path: "/traits/email"},
		jsonPatchOp{Op: "add", Path: "/traits/login_id", Value: loginID},
	})
	if err != nil {
		return err
	}
	switch resp.status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return app.ErrNotFound
	case http.StatusConflict:
		return fmt.Errorf("%w: login identifier taken", app.ErrConflict)
	}
	return resp.unexpected("patch identity login")
}

// MarkLoginVerified implements app.LoginTraitAdmin: a second PATCH sets the
// verifiable address of loginID to verified (a single patch together with
// the trait change does not survive Kratos recomputing addresses, S4b).
func (a *Admin) MarkLoginVerified(ctx context.Context, id uuid.UUID, loginID string) error {
	k, err := a.getRaw(ctx, id)
	if err != nil {
		return err
	}
	idx := -1
	for i, va := range k.VerifiableAddresses {
		if va.Via == "email" && strings.EqualFold(va.Value, loginID) {
			idx = i
			if va.Verified {
				return nil
			}
		}
	}
	if idx < 0 {
		return app.ErrNotFound
	}
	base := "/verifiable_addresses/" + strconv.Itoa(idx)
	resp, err := a.c.do(ctx, http.MethodPatch, "/admin/identities/"+id.String(), nil, []jsonPatchOp{
		{Op: "replace", Path: base + "/verified", Value: true},
		{Op: "replace", Path: base + "/status", Value: "completed"},
	})
	if err != nil {
		return err
	}
	switch resp.status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return app.ErrNotFound
	}
	return resp.unexpected("patch verifiable address")
}
