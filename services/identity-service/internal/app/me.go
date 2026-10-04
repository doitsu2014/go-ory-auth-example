package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// MeView joins the session (Kratos traits) with the profile (identity DB).
type MeView struct {
	Principal identity.Principal
	Profile   profile.Profile
	// Login is the caller's own login identifier (PLI-FR-07).
	Login login.Identifier
}

// MeService implements the customer self-service use cases (FR-09, FR-10).
type MeService struct {
	Profiles ProfileRepo
	// Logins reads the caller's login identifier (ADR-0013); nil falls back
	// to the session's legacy email.
	Logins *LoginIdentifierService
}

// GetMe returns the caller's profile, creating it lazily (ADR-0008).
// Permission: self (customer principal).
func (s *MeService) GetMe(ctx context.Context, p identity.Principal) (MeView, error) {
	if p.Kind != identity.KindCustomer {
		return MeView{}, ErrForbidden
	}
	pr, err := s.ensure(ctx, p)
	if err != nil {
		return MeView{}, err
	}
	l, err := s.login(ctx, p)
	if err != nil {
		return MeView{}, err
	}
	return MeView{Principal: p, Profile: pr, Login: l}, nil
}

func (s *MeService) login(ctx context.Context, p identity.Principal) (login.Identifier, error) {
	if s.Logins != nil {
		return s.Logins.Own(ctx, p)
	}
	return legacyIdentifier(p.LoginID)
}

// UpdateMe applies a partial profile update. Permission: self (customer
// principal) with a verified email address.
func (s *MeService) UpdateMe(ctx context.Context, p identity.Principal, patch profile.Patch) (MeView, error) {
	if p.Kind != identity.KindCustomer {
		return MeView{}, ErrForbidden
	}
	if !p.EmailVerified {
		return MeView{}, ErrEmailNotVerified
	}
	if err := patch.Validate(); err != nil {
		return MeView{}, err
	}
	cur, err := s.ensure(ctx, p)
	if err != nil {
		return MeView{}, err
	}
	updated, err := s.Profiles.Update(ctx, cur.Apply(patch))
	if err != nil {
		return MeView{}, fmt.Errorf("update profile: %w", err)
	}
	l, err := s.login(ctx, p)
	if err != nil {
		return MeView{}, err
	}
	return MeView{Principal: p, Profile: updated, Login: l}, nil
}

func (s *MeService) ensure(ctx context.Context, p identity.Principal) (profile.Profile, error) {
	pr, err := s.Profiles.Get(ctx, p.IdentityID)
	if err == nil {
		return pr, nil
	}
	if !errors.Is(err, profile.ErrNotFound) {
		return profile.Profile{}, fmt.Errorf("get profile: %w", err)
	}
	if err := s.Profiles.Ensure(ctx, p.IdentityID, p.Kind); err != nil {
		return profile.Profile{}, fmt.Errorf("ensure profile: %w", err)
	}
	pr, err = s.Profiles.Get(ctx, p.IdentityID)
	if err != nil {
		return profile.Profile{}, fmt.Errorf("get profile after ensure: %w", err)
	}
	return pr, nil
}
