package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// Pagination limits (docs/principles/05-api-guidelines.md §4).
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
	// maxKratosPagesPerList bounds how many Kratos pages one customer list
	// request may scan while filtering by schema/state.
	maxKratosPagesPerList = 5
)

// ClampPageSize applies the default and maximum page size.
func ClampPageSize(n int) int {
	switch {
	case n <= 0:
		return DefaultPageSize
	case n > MaxPageSize:
		return MaxPageSize
	}
	return n
}

// CustomerView is a customer as shown to admins.
type CustomerView struct {
	Identity    identity.Identity
	DisplayName *string
	// Login is the masked login identifier (PLI-FR-10); nil when it could
	// not be read. LoginUnavailable reports a key-manager failure.
	Login            *MaskedLogin
	LoginUnavailable bool
}

// CustomerQuery filters the customer list.
type CustomerQuery struct {
	State     *identity.State
	PageSize  int
	PageToken string
}

// CustomerPage is one page of customers.
type CustomerPage struct {
	Items         []CustomerView
	NextPageToken string
}

// CustomerService implements the admin customer use cases (FR-11, FR-12).
type CustomerService struct {
	Authz      Authorizer
	Identities IdentityAdmin
	Profiles   ProfileRepo
	Tx         TxRunner
	Sessions   SessionVerifier
	Log        *slog.Logger
	// Logins masks login identifiers (ADR-0013); nil leaves Login unset.
	Logins *LoginIdentifierService
}

// List lists customers. Permission: view_customers.
//
// Kratos cannot filter by schema or state, so this scans up to a few Kratos
// pages and filters in memory; a page may hold fewer than PageSize items.
func (s *CustomerService) List(ctx context.Context, a Actor, q CustomerQuery) (CustomerPage, error) {
	if err := require(ctx, s.Authz, a, identity.PermViewCustomers); err != nil {
		return CustomerPage{}, err
	}
	size := ClampPageSize(q.PageSize)
	token := q.PageToken
	var out []identity.Identity
	for range maxKratosPagesPerList {
		items, next, err := s.Identities.ListIdentities(ctx, IdentityQuery{PageSize: size, PageToken: token})
		if err != nil {
			return CustomerPage{}, fmt.Errorf("list identities: %w", err)
		}
		for _, it := range items {
			if it.SchemaID != string(identity.KindCustomer) {
				continue
			}
			if q.State != nil && it.State != *q.State {
				continue
			}
			out = append(out, it)
		}
		token = next
		if token == "" || len(out) > 0 {
			break
		}
	}
	views, err := s.withProfiles(ctx, out)
	if err != nil {
		return CustomerPage{}, err
	}
	return CustomerPage{Items: views, NextPageToken: token}, nil
}

// Get returns one customer. Permission: view_customers.
func (s *CustomerService) Get(ctx context.Context, a Actor, id uuid.UUID) (CustomerView, error) {
	if err := require(ctx, s.Authz, a, identity.PermViewCustomers); err != nil {
		return CustomerView{}, err
	}
	ident, err := s.customer(ctx, id)
	if err != nil {
		return CustomerView{}, err
	}
	views, err := s.withProfiles(ctx, []identity.Identity{ident})
	if err != nil {
		return CustomerView{}, err
	}
	return views[0], nil
}

// Disable sets the customer inactive, revokes all sessions and purges the
// local session cache. Idempotent. Audited (see audited). Permission:
// manage_customers.
func (s *CustomerService) Disable(ctx context.Context, a Actor, id uuid.UUID, reason *string) (identity.State, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageCustomers); err != nil {
		return "", err
	}
	if _, err := s.customer(ctx, id); err != nil {
		return "", err
	}
	ev := s.event(a, audit.ActionCustomerDisabled, id, reason)
	err := audited(ctx, s.Tx, s.Log, &ev, nil, func(ctx context.Context) error {
		if err := s.Identities.SetState(ctx, id, identity.StateInactive); err != nil {
			return fmt.Errorf("set inactive: %w", err)
		}
		if err := s.Identities.RevokeIdentitySessions(ctx, id); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return nil
	})
	s.Sessions.Invalidate(id)
	if err != nil {
		return "", err
	}
	return identity.StateInactive, nil
}

// Enable sets the customer active. Idempotent. Audited. Permission:
// manage_customers.
func (s *CustomerService) Enable(ctx context.Context, a Actor, id uuid.UUID, reason *string) (identity.State, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageCustomers); err != nil {
		return "", err
	}
	if _, err := s.customer(ctx, id); err != nil {
		return "", err
	}
	ev := s.event(a, audit.ActionCustomerEnabled, id, reason)
	err := audited(ctx, s.Tx, s.Log, &ev, nil, func(ctx context.Context) error {
		if err := s.Identities.SetState(ctx, id, identity.StateActive); err != nil {
			return fmt.Errorf("set active: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return identity.StateActive, nil
}

// RevokeSessions revokes every session of the customer. Audited. Permission:
// manage_customers.
func (s *CustomerService) RevokeSessions(ctx context.Context, a Actor, id uuid.UUID) error {
	if err := require(ctx, s.Authz, a, identity.PermManageCustomers); err != nil {
		return err
	}
	if _, err := s.customer(ctx, id); err != nil {
		return err
	}
	ev := s.event(a, audit.ActionCustomerSessionsRevoked, id, nil)
	err := audited(ctx, s.Tx, s.Log, &ev, nil, func(ctx context.Context) error {
		if err := s.Identities.RevokeIdentitySessions(ctx, id); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return nil
	})
	s.Sessions.Invalidate(id)
	return err
}

// customer loads the identity and hides anything that is not a customer
// behind ErrNotFound (admins are not managed here).
func (s *CustomerService) customer(ctx context.Context, id uuid.UUID) (identity.Identity, error) {
	ident, err := s.Identities.GetIdentity(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return identity.Identity{}, ErrNotFound
		}
		return identity.Identity{}, fmt.Errorf("get identity: %w", err)
	}
	if ident.SchemaID != string(identity.KindCustomer) {
		return identity.Identity{}, ErrNotFound
	}
	return ident, nil
}

func (s *CustomerService) withProfiles(ctx context.Context, ids []identity.Identity) ([]CustomerView, error) {
	views := make([]CustomerView, 0, len(ids))
	if len(ids) == 0 {
		return views, nil
	}
	keys := make([]uuid.UUID, len(ids))
	for i, it := range ids {
		keys[i] = it.ID
	}
	profiles, err := s.Profiles.GetMany(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}
	var masked map[uuid.UUID]MaskedLogin
	unavailable := false
	if s.Logins != nil {
		masked, unavailable = s.Logins.MaskMany(ctx, ids)
	}
	for _, it := range ids {
		v := CustomerView{Identity: it, LoginUnavailable: unavailable}
		if p, ok := profiles[it.ID]; ok {
			v.DisplayName = p.DisplayName
		}
		if m, ok := masked[it.ID]; ok {
			v.Login = &m
		}
		views = append(views, v)
	}
	return views, nil
}

func (s *CustomerService) event(a Actor, action audit.Action, id uuid.UUID, reason *string) audit.Event {
	details := map[string]any{}
	if reason != nil && *reason != "" {
		details["reason"] = *reason
	}
	return audit.Event{
		ActorID: a.Principal.IdentityID, Action: action,
		TargetType: audit.TargetCustomer, TargetID: id.String(),
		RequestID: a.RequestID, ClientIP: a.ClientIP, Details: details,
	}
}
