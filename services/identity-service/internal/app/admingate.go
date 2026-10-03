package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// Admin plane defaults (docs/architecture/03-auth-flows.md §3.4).
const (
	DefaultAdminSessionMaxAge = 12 * time.Hour
	DefaultMFAEnrollmentGrace = 24 * time.Hour
)

// AdminGate enforces the admin plane rules that hold for every /admin/v1
// request, before any per-route permission (FR-05, FR-06). The admin
// identity (with its TOTP credential) is loaded on every request, at AAL1
// and AAL2 alike:
//
//   - schema must be admin                            → ErrNotAdmin
//   - authenticated_at older than MaxAge               → session revoked in Kratos, ErrUnauthenticated
//   - TOTP enrolled after the enrolment deadline       → identity deactivated, ErrForbidden
//     (closes the bypass where a password-only attacker enrols TOTP at
//     Kratos after the deadline and arrives at AAL2)
//   - no TOTP, before the deadline                     → ErrMFAEnrollmentRequired
//   - no TOTP, after the deadline                      → identity deactivated, ErrForbidden
//   - TOTP and AAL1 (only /admin/v1/me allows AAL1)    → ErrAAL2Required
//
// The deadline is MFADeadlineAnchor()+MFAGrace: identity creation or the last
// state change (operator reactivation), whichever is later. Re-enrolling TOTP
// after the window therefore needs an operator to reactivate the identity.
type AdminGate struct {
	Identities IdentityAdmin
	Sessions   SessionVerifier
	Tx         TxRunner
	Clock      Clock
	Log        *slog.Logger
	MaxAge     time.Duration
	MFAGrace   time.Duration
}

// Check applies the rules. allowAAL1 is true only for /admin/v1/me.
func (g *AdminGate) Check(ctx context.Context, a Actor, allowAAL1 bool) error {
	p := a.Principal
	if p.Kind != identity.KindAdmin {
		return ErrNotAdmin
	}
	now := g.Clock.Now()
	if now.Sub(p.AuthenticatedAt) > g.maxAge() {
		if err := g.Identities.RevokeSession(ctx, p.SessionID); err != nil {
			return fmt.Errorf("revoke expired admin session: %w", err)
		}
		g.Sessions.Invalidate(p.IdentityID)
		return ErrUnauthenticated
	}
	ident, err := g.Identities.GetIdentity(ctx, p.IdentityID)
	if err != nil {
		return fmt.Errorf("load admin identity: %w", err)
	}
	if ident.State != identity.StateActive {
		g.Sessions.Invalidate(p.IdentityID)
		return ErrUnauthenticated
	}
	deadline := ident.MFADeadline(g.mfaGrace())
	if ident.HasTOTP {
		if ident.TOTPCreatedAt.IsZero() {
			return fmt.Errorf("totp credential of %s has no created_at", ident.ID)
		}
		if ident.TOTPCreatedAt.After(deadline) {
			if err := g.deactivate(ctx, ident.ID, a.RequestID, a, "mfa_enrolled_after_deadline"); err != nil {
				return err
			}
			return ErrForbidden
		}
		if p.AAL == identity.AAL2 || allowAAL1 {
			return nil
		}
		return ErrAAL2Required
	}
	if now.Before(deadline) {
		return ErrMFAEnrollmentRequired
	}
	if err := g.deactivate(ctx, ident.ID, a.RequestID, a, "mfa_enrollment_deadline"); err != nil {
		return err
	}
	return ErrForbidden
}

// mustDeactivate reports whether an active admin identity is past its MFA
// deadline without a valid enrolment.
func (g *AdminGate) mustDeactivate(ident identity.Identity, now time.Time) (bool, string) {
	deadline := ident.MFADeadline(g.mfaGrace())
	switch {
	case ident.HasTOTP && !ident.TOTPCreatedAt.IsZero() && ident.TOTPCreatedAt.After(deadline):
		return true, "mfa_enrolled_after_deadline"
	case !ident.HasTOTP && now.After(deadline):
		return true, "mfa_enrollment_deadline"
	}
	return false, ""
}

// Sweep deactivates every active admin past its MFA deadline without a valid
// TOTP enrolment, so the rule holds even for admins who never call the API.
// Run periodically by `serve`. Returns the number of deactivated identities.
func (g *AdminGate) Sweep(ctx context.Context) (int, error) {
	now := g.Clock.Now()
	n := 0
	token := ""
	for {
		items, next, err := g.Identities.ListIdentities(ctx, IdentityQuery{PageSize: MaxPageSize, PageToken: token, IncludeTOTP: true})
		if err != nil {
			return n, fmt.Errorf("list identities: %w", err)
		}
		for _, it := range items {
			if it.SchemaID != string(identity.KindAdmin) || it.State != identity.StateActive {
				continue
			}
			if ok, reason := g.mustDeactivate(it, now); ok {
				if err := g.deactivate(ctx, it.ID, "mfa-sweeper", Actor{}, reason); err != nil {
					return n, err
				}
				n++
			}
		}
		if next == "" || next == token {
			return n, nil
		}
		token = next
	}
}

// deactivate sets the admin inactive and revokes its sessions, audited.
func (g *AdminGate) deactivate(ctx context.Context, id uuid.UUID, requestID string, a Actor, reason string) error {
	ev := audit.Event{
		ActorID: audit.SystemActor, Action: audit.ActionAdminDeactivated,
		TargetType: audit.TargetAdmin, TargetID: id.String(),
		RequestID: requestID, ClientIP: a.ClientIP,
		Details: map[string]any{"reason": reason},
	}
	err := audited(ctx, g.Tx, g.Log, &ev, nil, func(ctx context.Context) error {
		if err := g.Identities.SetState(ctx, id, identity.StateInactive); err != nil {
			return fmt.Errorf("deactivate admin: %w", err)
		}
		if err := g.Identities.RevokeIdentitySessions(ctx, id); err != nil {
			return fmt.Errorf("revoke admin sessions: %w", err)
		}
		return nil
	})
	g.Sessions.Invalidate(id)
	if err != nil {
		return err
	}
	if g.Log != nil {
		g.Log.WarnContext(ctx, "admin deactivated", "identity_id", id.String(), "reason", reason)
	}
	return nil
}

func (g *AdminGate) maxAge() time.Duration {
	if g.MaxAge > 0 {
		return g.MaxAge
	}
	return DefaultAdminSessionMaxAge
}

func (g *AdminGate) mfaGrace() time.Duration {
	if g.MFAGrace > 0 {
		return g.MFAGrace
	}
	return DefaultMFAEnrollmentGrace
}
