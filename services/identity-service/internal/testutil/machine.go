package testutil

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

// MachineVerifier is a fake MachineTokenVerifier keyed by token value.
type MachineVerifier struct {
	mu          sync.Mutex
	Tokens      map[string]machine.Principal
	Errs        map[string]error
	Invalidated []string
	Calls       int
}

// NewMachineVerifier creates an empty fake.
func NewMachineVerifier() *MachineVerifier {
	return &MachineVerifier{Tokens: map[string]machine.Principal{}, Errs: map[string]error{}}
}

// Verify implements app.MachineTokenVerifier.
func (v *MachineVerifier) Verify(_ context.Context, tok string) (machine.Principal, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Calls++
	if err, ok := v.Errs[tok]; ok {
		return machine.Principal{}, err
	}
	p, ok := v.Tokens[tok]
	if !ok {
		return machine.Principal{}, app.ErrInvalidToken
	}
	return p, nil
}

// Invalidate implements app.MachineTokenVerifier.
func (v *MachineVerifier) Invalidate(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Invalidated = append(v.Invalidated, id)
}

// ServiceClient is a stored fake client.
type ServiceClient struct {
	Client           machine.ServiceClient
	Secret           string
	TokensValidAfter time.Time
}

// ServiceClients is a fake Hydra admin API.
type ServiceClients struct {
	mu        sync.Mutex
	M         map[string]ServiceClient
	Deleted   []string
	Err       error
	DeleteErr error
	// SetSecretErr fails SetSecret only; OnSetSecret runs after a
	// successful SetSecret (e.g. to fail the next transaction).
	SetSecretErr error
	OnSetSecret  func()
	// CreateStoresThenFails models an ambiguous failure: the client is
	// stored at "Hydra" but the call returns this error.
	CreateStoresThenFails error
	// CreateCtxErr is ctx.Err() as seen by the last Create.
	CreateCtxErr error
	Clock        app.Clock
	seq          int
}

// NewServiceClients creates an empty fake.
func NewServiceClients(c app.Clock) *ServiceClients {
	return &ServiceClients{M: map[string]ServiceClient{}, Clock: c}
}

// Create implements app.ServiceClientAdmin.
func (f *ServiceClients) Create(ctx context.Context, in app.NewServiceClient) (machine.ServiceClient, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CreateCtxErr = ctx.Err()
	if f.Err != nil {
		return machine.ServiceClient{}, "", f.Err
	}
	f.seq++
	by := in.CreatedBy
	now := time.Now()
	if f.Clock != nil {
		now = f.Clock.Now()
	}
	id := in.ClientID
	if id == "" {
		id = uuid.NewString()
	}
	if _, dup := f.M[id]; dup {
		return machine.ServiceClient{}, "", app.ErrConflict
	}
	c := machine.ServiceClient{
		ClientID: id, Name: in.Registration.Name, Owner: in.Registration.Owner,
		Scopes: in.Registration.Scopes, CreatedAt: now, CreatedBy: &by,
	}
	secret := fmt.Sprintf("hydra-secret-%d", f.seq)
	f.M[c.ClientID] = ServiceClient{Client: c, Secret: secret}
	if f.CreateStoresThenFails != nil {
		return machine.ServiceClient{}, "", f.CreateStoresThenFails
	}
	return c, secret, nil
}

// Get implements app.ServiceClientAdmin.
func (f *ServiceClients) Get(_ context.Context, id string) (machine.ServiceClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return machine.ServiceClient{}, f.Err
	}
	c, ok := f.M[id]
	if !ok {
		return machine.ServiceClient{}, app.ErrNotFound
	}
	return c.Client, nil
}

// List implements app.ServiceClientAdmin.
func (f *ServiceClients) List(context.Context) ([]machine.ServiceClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]machine.ServiceClient, 0, len(f.M))
	for _, c := range f.M {
		out = append(out, c.Client)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClientID < out[j].ClientID })
	return out, nil
}

// SetSecret implements app.ServiceClientAdmin.
func (f *ServiceClients) SetSecret(_ context.Context, id, secret string, tva time.Time) error {
	f.mu.Lock()
	if f.Err != nil || f.SetSecretErr != nil {
		defer f.mu.Unlock()
		return errors.Join(f.Err, f.SetSecretErr)
	}
	c, ok := f.M[id]
	if !ok {
		f.mu.Unlock()
		return app.ErrNotFound
	}
	c.Secret, c.TokensValidAfter = secret, tva
	f.M[id] = c
	hook := f.OnSetSecret
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

// Delete implements app.ServiceClientAdmin.
func (f *ServiceClients) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.DeleteErr != nil {
		return f.DeleteErr
	}
	if _, ok := f.M[id]; !ok {
		return app.ErrNotFound
	}
	delete(f.M, id)
	f.Deleted = append(f.Deleted, id)
	return nil
}

// Secret returns the current secret of a client (test inspection).
func (f *ServiceClients) Secret(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.M[id].Secret
}
