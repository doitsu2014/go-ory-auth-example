package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// Invitation defaults.
const (
	DefaultInvitationTTL = 24 * time.Hour
	IdempotencyTTL       = 24 * time.Hour
	maxEmailLen          = 320
	maxNameLen           = 100
)

// AdminMeView is the current admin with roles and effective permissions.
type AdminMeView struct {
	Principal   identity.Principal
	Roles       []identity.Role
	Permissions []identity.Permission
}

// AdminView is an admin as listed to super admins.
type AdminView struct {
	Identity identity.Identity
	Role     identity.Role
	HasRole  bool
}

// AdminPage is one page of admins.
type AdminPage struct {
	Items         []AdminView
	NextPageToken string
}

// InviteRequest is the input of InviteAdmin.
type InviteRequest struct {
	Email string
	Name  identity.Name
	Role  identity.Role
}

// InvitedAdmin is the (replayable) result of InviteAdmin.
type InvitedAdmin struct {
	ID                  uuid.UUID     `json:"id"`
	Email               string        `json:"email"`
	Role                identity.Role `json:"role"`
	InvitationExpiresAt time.Time     `json:"invitation_expires_at"`
}

// AdminService implements the admin management use cases (FR-05..FR-07, FR-12).
type AdminService struct {
	Authz         Authorizer
	Roles         RoleStore
	Identities    IdentityAdmin
	Tx            TxRunner
	Idempotency   IdempotencyRepo
	Mailer        Mailer
	Clock         Clock
	Log           *slog.Logger
	InvitationTTL time.Duration
}

// Me returns the current admin's roles and permissions. Permission: any admin
// principal (the AdminGate ran before).
func (s *AdminService) Me(ctx context.Context, a Actor) (AdminMeView, error) {
	if a.Principal.Kind != identity.KindAdmin {
		return AdminMeView{}, ErrNotAdmin
	}
	roles, err := s.Roles.RolesOf(ctx, a.Principal.IdentityID)
	if err != nil {
		return AdminMeView{}, fmt.Errorf("roles: %w", err)
	}
	perms := make([]identity.Permission, 0, len(identity.AllPermissions))
	for _, p := range identity.AllPermissions {
		ok, err := s.Authz.Check(ctx, a.Principal.IdentityID, p)
		if err != nil {
			return AdminMeView{}, fmt.Errorf("check %s: %w", p, err)
		}
		if ok {
			perms = append(perms, p)
		}
	}
	return AdminMeView{Principal: a.Principal, Roles: roles, Permissions: perms}, nil
}

// List lists admin identities with their role. Permission: manage_admins.
func (s *AdminService) List(ctx context.Context, a Actor, pageSize int, pageToken string) (AdminPage, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageAdmins); err != nil {
		return AdminPage{}, err
	}
	size := ClampPageSize(pageSize)
	token := pageToken
	var found []identity.Identity
	for range maxKratosPagesPerList {
		items, next, err := s.Identities.ListIdentities(ctx, IdentityQuery{PageSize: size, PageToken: token, IncludeTOTP: true})
		if err != nil {
			return AdminPage{}, fmt.Errorf("list identities: %w", err)
		}
		for _, it := range items {
			if it.SchemaID == string(identity.KindAdmin) {
				found = append(found, it)
			}
		}
		token = next
		if token == "" || len(found) > 0 {
			break
		}
	}
	assignments, err := s.Roles.AllAssignments(ctx)
	if err != nil {
		return AdminPage{}, fmt.Errorf("role assignments: %w", err)
	}
	out := make([]AdminView, 0, len(found))
	for _, it := range found {
		v := AdminView{Identity: it}
		v.Role, v.HasRole = identity.HighestRole(assignments[it.ID])
		out = append(out, v)
	}
	return AdminPage{Items: out, NextPageToken: token}, nil
}

// finalizeTimeout bounds the detached steps after the Ory calls (record,
// deliver, compensate), independent of the request deadline.
const finalizeTimeout = 10 * time.Second

// superAdminLockKey serialises super_admin demotions (pg_advisory_xact_lock).
const superAdminLockKey int64 = 0x1d5e_0001

// Invite creates an admin identity, grants the role, creates a 24 h recovery
// code and emails it. The code/link is never returned or logged.
// Permission: manage_admins.
//
// Idempotency-Key: the key is reserved (pending row) before any side effect,
// so concurrent duplicates get 409 and completed ones replay the stored
// response for 24 h. Order of steps:
//
//  1. reserve key → 2. Kratos identity → 3. Keto role → 4. recovery code →
//  5. one detached tx: profile + audit admin.invited + complete key →
//  6. send email (detached).
//
// Failure in 2–5 compensates (tuple + identity removed, key released). A
// failed email after 5 also compensates, audits admin.invitation_failed and
// releases the key, so the caller can retry. If compensation itself fails,
// "orphaned_admin" is logged with ids.
func (s *AdminService) Invite(ctx context.Context, a Actor, idemKey string, req InviteRequest) (InvitedAdmin, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageAdmins); err != nil {
		return InvitedAdmin{}, err
	}
	req, err := normalizeInvite(req)
	if err != nil {
		return InvitedAdmin{}, err
	}
	if strings.TrimSpace(idemKey) == "" {
		return InvitedAdmin{}, NewValidationError("Idempotency-Key", "required")
	}
	hash := requestHash(req)
	actorID := a.Principal.IdentityID
	existing, reserved, err := s.Idempotency.Reserve(ctx, actorID, idemKey, hash, s.Clock.Now().Add(-IdempotencyTTL))
	if err != nil {
		return InvitedAdmin{}, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if !reserved {
		return replay(existing, hash)
	}
	release := func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
		defer cancel()
		if err := s.Idempotency.Release(cctx, actorID, idemKey); err != nil && s.Log != nil {
			s.Log.ErrorContext(ctx, "release idempotency key", "actor_id", actorID.String(), "error", err.Error())
		}
	}

	ident, rc, err := s.provision(ctx, req)
	if err != nil {
		release()
		return InvitedAdmin{}, err
	}
	out := InvitedAdmin{ID: ident.ID, Email: req.Email, Role: req.Role, InvitationExpiresAt: rc.ExpiresAt.UTC()}
	body, err := json.Marshal(out)
	if err != nil {
		s.compensate(ctx, ident.ID)
		release()
		return InvitedAdmin{}, fmt.Errorf("encode response: %w", err)
	}

	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	err = s.Tx.WithinTx(fctx, func(ctx context.Context, r Repos) error {
		if err := r.Profiles.Ensure(ctx, out.ID, identity.KindAdmin); err != nil {
			return err
		}
		if err := r.Audit.Append(ctx, audit.Event{
			ActorID: actorID, Action: audit.ActionAdminInvited,
			TargetType: audit.TargetAdmin, TargetID: out.ID.String(),
			RequestID: a.RequestID, ClientIP: a.ClientIP,
			Details: map[string]any{"role": string(req.Role)},
		}); err != nil {
			return err
		}
		return r.Idempotency.Complete(ctx, actorID, idemKey, 201, body)
	})
	if err != nil {
		s.compensate(ctx, ident.ID)
		release()
		return InvitedAdmin{}, fmt.Errorf("record invitation: %w", err)
	}

	mctx, mcancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer mcancel()
	if err := s.Mailer.SendInvitation(mctx, Invitation{
		To: req.Email, Name: req.Name, Role: req.Role, Link: rc.Link, Code: rc.Code, ExpiresAt: rc.ExpiresAt,
	}); err != nil {
		s.compensate(ctx, ident.ID)
		uerr := s.Tx.WithinTx(fctx, func(ctx context.Context, r Repos) error {
			if err := r.Audit.Append(ctx, audit.Event{
				ActorID: actorID, Action: audit.ActionAdminInvitationFailed,
				TargetType: audit.TargetAdmin, TargetID: out.ID.String(),
				RequestID: a.RequestID, ClientIP: a.ClientIP,
				Details: map[string]any{"reason": "mail_delivery_failed"},
			}); err != nil {
				return err
			}
			return r.Idempotency.Release(ctx, actorID, idemKey)
		})
		if uerr != nil && s.Log != nil {
			s.Log.ErrorContext(ctx, "record invitation failure", "identity_id", out.ID.String(), "error", uerr.Error())
		}
		// The SMTP error is not wrapped: server replies may echo the address.
		return InvitedAdmin{}, fmt.Errorf("%w: invitation email could not be sent", ErrDependencyUnavailable)
	}
	return out, nil
}

func replay(rec *IdempotencyRecord, hash []byte) (InvitedAdmin, error) {
	if rec == nil {
		return InvitedAdmin{}, fmt.Errorf("%w: idempotency key unavailable", ErrConflict)
	}
	if !bytes.Equal(rec.RequestHash, hash) {
		return InvitedAdmin{}, fmt.Errorf("%w: idempotency key reused with a different request", ErrConflict)
	}
	if rec.Pending() {
		return InvitedAdmin{}, fmt.Errorf("%w: a request with this idempotency key is in progress", ErrConflict)
	}
	var out InvitedAdmin
	if err := json.Unmarshal(rec.ResponseBody, &out); err != nil {
		return InvitedAdmin{}, fmt.Errorf("decode stored response: %w", err)
	}
	return out, nil
}

// BootstrapResult is returned to the operator CLI only.
type BootstrapResult struct {
	Admin    InvitedAdmin
	Recovery RecoveryCode
}

// Bootstrap creates the first super_admin from a trusted operator shell and
// returns the recovery code for printing to that terminal only. It fails with
// ErrConflict if the email already exists. If recording fails, the identity
// and tuple are removed again.
func (s *AdminService) Bootstrap(ctx context.Context, email string, name identity.Name) (BootstrapResult, error) {
	req, err := normalizeInvite(InviteRequest{Email: email, Name: name, Role: identity.RoleSuperAdmin})
	if err != nil {
		return BootstrapResult{}, err
	}
	ident, rc, err := s.provision(ctx, req)
	if err != nil {
		return BootstrapResult{}, err
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	err = s.Tx.WithinTx(fctx, func(ctx context.Context, r Repos) error {
		if err := r.Profiles.Ensure(ctx, ident.ID, identity.KindAdmin); err != nil {
			return err
		}
		return r.Audit.Append(ctx, audit.Event{
			ActorID: audit.SystemActor, Action: audit.ActionAdminBootstrapped,
			TargetType: audit.TargetAdmin, TargetID: ident.ID.String(),
			RequestID: "cli-bootstrap", Details: map[string]any{"role": string(identity.RoleSuperAdmin)},
		})
	})
	if err != nil {
		s.compensate(ctx, ident.ID)
		return BootstrapResult{}, fmt.Errorf("record bootstrap: %w", err)
	}
	out := InvitedAdmin{ID: ident.ID, Email: req.Email, Role: req.Role, InvitationExpiresAt: rc.ExpiresAt.UTC()}
	return BootstrapResult{Admin: out, Recovery: rc}, nil
}

// provision runs identity → tuple → recovery code, compensating on failure.
func (s *AdminService) provision(ctx context.Context, req InviteRequest) (identity.Identity, RecoveryCode, error) {
	ident, err := s.Identities.CreateIdentity(ctx, NewIdentity{SchemaID: string(identity.KindAdmin), Email: req.Email, Name: req.Name})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return identity.Identity{}, RecoveryCode{}, fmt.Errorf("%w: identifier already exists", ErrConflict)
		}
		return identity.Identity{}, RecoveryCode{}, fmt.Errorf("create identity: %w", err)
	}
	if err := s.Roles.SetRole(ctx, ident.ID, req.Role); err != nil {
		s.compensate(ctx, ident.ID)
		return identity.Identity{}, RecoveryCode{}, fmt.Errorf("grant role: %w", err)
	}
	rc, err := s.Identities.CreateRecoveryCode(ctx, ident.ID, s.ttl())
	if err != nil {
		s.compensate(ctx, ident.ID)
		return identity.Identity{}, RecoveryCode{}, fmt.Errorf("create recovery code: %w", err)
	}
	return ident, rc, nil
}

// compensate removes the role tuples and the identity of a failed invite.
func (s *AdminService) compensate(ctx context.Context, id uuid.UUID) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	rerr := s.Roles.RemoveAll(cctx, id)
	derr := s.Identities.DeleteIdentity(cctx, id)
	if (rerr != nil || derr != nil) && s.Log != nil {
		s.Log.ErrorContext(ctx, "orphaned_admin", "identity_id", id.String(),
			"remove_tuples_error", errString(rerr), "delete_identity_error", errString(derr))
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ChangeRole replaces the target admin's role. Rules: target must be an
// admin (else ErrNotFound), not the caller (ErrForbidden), and the last
// active super_admin cannot be demoted (ErrConflict). Demotions are
// serialised with a transaction-scoped advisory lock held across the count
// and the Keto write; only active admin identities count. Audited (see
// audited). Permission: manage_admins.
func (s *AdminService) ChangeRole(ctx context.Context, a Actor, target uuid.UUID, role identity.Role) (AdminView, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageAdmins); err != nil {
		return AdminView{}, err
	}
	if _, err := role.Relation(); err != nil {
		return AdminView{}, NewValidationError("role", "invalid")
	}
	if target == a.Principal.IdentityID {
		return AdminView{}, fmt.Errorf("%w: cannot change own role", ErrForbidden)
	}
	ident, err := s.Identities.GetIdentity(ctx, target)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AdminView{}, ErrNotFound
		}
		return AdminView{}, fmt.Errorf("get identity: %w", err)
	}
	if ident.SchemaID != string(identity.KindAdmin) {
		return AdminView{}, ErrNotFound
	}
	ev := audit.Event{
		ActorID: a.Principal.IdentityID, Action: audit.ActionAdminRoleChanged,
		TargetType: audit.TargetAdmin, TargetID: target.String(),
		RequestID: a.RequestID, ClientIP: a.ClientIP, Details: map[string]any{"role": string(role)},
	}
	pre := func(ctx context.Context, r Repos) error {
		if err := r.Locks.XactLock(ctx, superAdminLockKey); err != nil {
			return fmt.Errorf("lock: %w", err)
		}
		current, err := s.Roles.RolesOf(ctx, target)
		if err != nil {
			return fmt.Errorf("roles: %w", err)
		}
		if prev, ok := identity.HighestRole(current); ok {
			ev.Details["previous_role"] = string(prev)
		}
		if hasRole(current, identity.RoleSuperAdmin) && role != identity.RoleSuperAdmin && ident.State == identity.StateActive {
			n, err := s.activeSuperAdmins(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return fmt.Errorf("%w: cannot remove the last super_admin", ErrConflict)
			}
		}
		return nil
	}
	err = audited(ctx, s.Tx, s.Log, &ev, pre, func(ctx context.Context) error {
		if err := s.Roles.SetRole(ctx, target, role); err != nil {
			return fmt.Errorf("set role: %w", err)
		}
		return nil
	})
	if err != nil {
		return AdminView{}, err
	}
	return AdminView{Identity: ident, Role: role, HasRole: true}, nil
}

// activeSuperAdmins counts super_admin tuples whose subject is an existing,
// active admin identity.
func (s *AdminService) activeSuperAdmins(ctx context.Context) (int, error) {
	all, err := s.Roles.AllAssignments(ctx)
	if err != nil {
		return 0, fmt.Errorf("role assignments: %w", err)
	}
	n := 0
	for id, roles := range all {
		if !hasRole(roles, identity.RoleSuperAdmin) {
			continue
		}
		it, err := s.Identities.GetIdentity(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("get identity: %w", err)
		}
		if it.SchemaID == string(identity.KindAdmin) && it.State == identity.StateActive {
			n++
		}
	}
	return n, nil
}

func (s *AdminService) ttl() time.Duration {
	if s.InvitationTTL > 0 && s.InvitationTTL <= DefaultInvitationTTL {
		return s.InvitationTTL
	}
	return DefaultInvitationTTL
}

func hasRole(rs []identity.Role, r identity.Role) bool {
	for _, x := range rs {
		if x == r {
			return true
		}
	}
	return false
}

func normalizeInvite(req InviteRequest) (InviteRequest, error) {
	var errs []FieldError
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		errs = append(errs, FieldError{Field: "email", Code: "required"})
	} else if len(req.Email) > maxEmailLen {
		errs = append(errs, FieldError{Field: "email", Code: "too_long"})
	} else if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email {
		errs = append(errs, FieldError{Field: "email", Code: "invalid_format"})
	}
	if profile.HasControlChars(req.Email) {
		errs = append(errs, FieldError{Field: "email", Code: "invalid_characters"})
	}
	if profile.HasControlChars(req.Name.First) {
		errs = append(errs, FieldError{Field: "name.first", Code: "invalid_characters"})
	}
	if profile.HasControlChars(req.Name.Last) {
		errs = append(errs, FieldError{Field: "name.last", Code: "invalid_characters"})
	}
	if utf8.RuneCountInString(req.Name.First) > maxNameLen {
		errs = append(errs, FieldError{Field: "name.first", Code: "too_long"})
	}
	if utf8.RuneCountInString(req.Name.Last) > maxNameLen {
		errs = append(errs, FieldError{Field: "name.last", Code: "too_long"})
	}
	if _, err := req.Role.Relation(); err != nil {
		errs = append(errs, FieldError{Field: "role", Code: "invalid"})
	}
	if len(errs) > 0 {
		return req, &ValidationError{Fields: errs}
	}
	return req, nil
}

func requestHash(req InviteRequest) []byte {
	b, _ := json.Marshal(struct {
		Email string `json:"email"`
		First string `json:"first"`
		Last  string `json:"last"`
		Role  string `json:"role"`
	}{req.Email, req.Name.First, req.Name.Last, string(req.Role)})
	sum := sha256.Sum256(b)
	return sum[:]
}
