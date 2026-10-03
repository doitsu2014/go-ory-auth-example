package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// require fails closed unless the actor is an admin holding perm.
func require(ctx context.Context, authz Authorizer, a Actor, perm identity.Permission) error {
	if a.Principal.Kind != identity.KindAdmin {
		return ErrNotAdmin
	}
	ok, err := authz.Check(ctx, a.Principal.IdentityID, perm)
	if err != nil {
		return fmt.Errorf("authorize %s: %w", perm, err)
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

type memoKey struct{}

type memo struct {
	mu sync.Mutex
	m  map[string]bool
}

// WithAuthzMemo returns a context in which MemoAuthorizer caches positive and
// negative decisions, so the route-level and use-case-level checks of one
// request cost a single Keto call.
func WithAuthzMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, memoKey{}, &memo{m: map[string]bool{}})
}

// MemoAuthorizer decorates an Authorizer with a request-scoped memo.
type MemoAuthorizer struct{ Next Authorizer }

// Check implements Authorizer.
func (m MemoAuthorizer) Check(ctx context.Context, subject uuid.UUID, perm identity.Permission) (bool, error) {
	mm, _ := ctx.Value(memoKey{}).(*memo)
	if mm == nil {
		return m.Next.Check(ctx, subject, perm)
	}
	k := subject.String() + "#" + string(perm)
	mm.mu.Lock()
	v, ok := mm.m[k]
	mm.mu.Unlock()
	if ok {
		return v, nil
	}
	v, err := m.Next.Check(ctx, subject, perm)
	if err != nil {
		return false, err
	}
	mm.mu.Lock()
	mm.m[k] = v
	mm.mu.Unlock()
	return v, nil
}
