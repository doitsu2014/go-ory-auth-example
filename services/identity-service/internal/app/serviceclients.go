package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

// secretBytes is the entropy of a rotated client secret (§6 A10).
const secretBytes = 32

// hydraCallTimeout bounds a detached Hydra mutation: it runs on
// context.WithoutCancel so a client disconnect cannot cut it in half.
const hydraCallTimeout = 10 * time.Second

// TokensValidAfter is the rotation cut-off written to Hydra (§6 B3):
// ceil(now)+1 in unix seconds. Tokens are accepted only when iat ≥ it, so a
// token issued in the rotation second (iat has whole-second resolution) is
// always rejected. Clients must fetch a new token 1–2 s after a rotation
// (retry on 401).
func TokensValidAfter(now time.Time) time.Time {
	secs := now.Unix()
	if now.Nanosecond() > 0 {
		secs++
	}
	return time.Unix(secs+1, 0).UTC()
}

// ServiceClientWithSecret is a client plus its one-time secret. Secret is
// empty on an idempotent replay (secrets are never stored).
type ServiceClientWithSecret struct {
	Client machine.ServiceClient
	Secret string
}

// ServiceClientService manages machine clients at Hydra (M2M-FR-08..11, 13).
// Every mutation is audited; secrets are returned once and never stored or
// logged.
type ServiceClientService struct {
	Authz       Authorizer
	Clients     ServiceClientAdmin
	Verifier    MachineTokenVerifier
	Tx          TxRunner
	Idempotency IdempotencyRepo
	Clock       Clock
	Log         *slog.Logger
	// Random defaults to crypto/rand.
	Random func([]byte) error
}

// List returns the managed clients. Permission: manage_service_clients.
func (s *ServiceClientService) List(ctx context.Context, a Actor) ([]machine.ServiceClient, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageServiceClients); err != nil {
		return nil, err
	}
	out, err := s.Clients.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list service clients: %w", err)
	}
	return out, nil
}

// Get returns one managed client. Permission: manage_service_clients.
func (s *ServiceClientService) Get(ctx context.Context, a Actor, clientID string) (machine.ServiceClient, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageServiceClients); err != nil {
		return machine.ServiceClient{}, err
	}
	return s.get(ctx, clientID)
}

func (s *ServiceClientService) get(ctx context.Context, clientID string) (machine.ServiceClient, error) {
	if !validClientID(clientID) {
		return machine.ServiceClient{}, ErrNotFound
	}
	c, err := s.Clients.Get(ctx, clientID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return machine.ServiceClient{}, ErrNotFound
		}
		return machine.ServiceClient{}, fmt.Errorf("get service client: %w", err)
	}
	return c, nil
}

// validClientID bounds client-supplied ids before they reach Hydra.
func validClientID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		ok := r == '-' || r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return false
		}
	}
	return true
}

// Create registers a client and returns its secret once. Permission:
// manage_service_clients.
//
// Like the admin invite (MD-7): 1. reserve the Idempotency-Key → 2. Hydra
// create → 3. one detached tx: audit service_client.created + complete the
// key (stored response without the secret). If 3 fails the Hydra client is
// deleted again (compensation) and the key released. A replay returns the
// client without client_secret.
func (s *ServiceClientService) Create(ctx context.Context, a Actor, idemKey string, name, owner string, scopes []string) (ServiceClientWithSecret, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageServiceClients); err != nil {
		return ServiceClientWithSecret{}, err
	}
	reg, err := machine.NewRegistration(name, owner, scopes)
	if err != nil {
		return ServiceClientWithSecret{}, err
	}
	if strings.TrimSpace(idemKey) == "" {
		return ServiceClientWithSecret{}, NewValidationError("Idempotency-Key", "required")
	}
	actorID := a.Principal.IdentityID
	hash := registrationHash(reg)
	existing, reserved, err := s.Idempotency.Reserve(ctx, actorID, idemKey, hash, s.Clock.Now().Add(-IdempotencyTTL))
	if err != nil {
		return ServiceClientWithSecret{}, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if !reserved {
		c, err := replayServiceClient(existing, hash)
		return ServiceClientWithSecret{Client: c}, err
	}
	release := func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
		defer cancel()
		if err := s.Idempotency.Release(cctx, actorID, idemKey); err != nil && s.Log != nil {
			s.Log.ErrorContext(ctx, "release idempotency key", "actor_id", actorID.String(), "error", err.Error())
		}
	}
	client, secret, err := s.createAtHydra(ctx, reg, actorID)
	if err != nil {
		release()
		return ServiceClientWithSecret{}, err
	}
	body, err := json.Marshal(toStored(client))
	if err != nil {
		s.compensate(ctx, client.ClientID)
		release()
		return ServiceClientWithSecret{}, fmt.Errorf("encode response: %w", err)
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	err = s.Tx.WithinTx(fctx, func(ctx context.Context, r Repos) error {
		if err := r.Audit.Append(ctx, s.event(actorID, a.RequestID, a, audit.ActionServiceClientCreated, client, true)); err != nil {
			return err
		}
		return r.Idempotency.Complete(ctx, actorID, idemKey, 201, body)
	})
	if err != nil {
		s.compensate(ctx, client.ClientID)
		release()
		return ServiceClientWithSecret{}, fmt.Errorf("record service client: %w", err)
	}
	return ServiceClientWithSecret{Client: client, Secret: secret}, nil
}

// CreateAsSystem registers a client from the operator CLI (M2M-FR-13),
// audited with the system actor. No permission check: the caller is a
// trusted operator shell with DATABASE_URL and the Hydra admin URL.
func (s *ServiceClientService) CreateAsSystem(ctx context.Context, name, owner string, scopes []string) (ServiceClientWithSecret, error) {
	reg, err := machine.NewRegistration(name, owner, scopes)
	if err != nil {
		return ServiceClientWithSecret{}, err
	}
	client, secret, err := s.createAtHydra(ctx, reg, audit.SystemActor)
	if err != nil {
		return ServiceClientWithSecret{}, err
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	err = s.Tx.WithinTx(fctx, func(ctx context.Context, r Repos) error {
		return r.Audit.Append(ctx, s.event(audit.SystemActor, "cli-clients-create", Actor{}, audit.ActionServiceClientCreated, client, true))
	})
	if err != nil {
		s.compensate(ctx, client.ClientID)
		return ServiceClientWithSecret{}, fmt.Errorf("record service client: %w", err)
	}
	return ServiceClientWithSecret{Client: client, Secret: secret}, nil
}

// RotateSecret replaces the secret (the old one stops working at the token
// endpoint immediately) and sets tokens_valid_after (see TokensValidAfter),
// so tokens issued before the rotation are rejected by the verifier (§6 B3).
// Permission: manage_service_clients.
//
// Two-phase audit instead of audited(): a COMMIT failure after the Hydra
// PATCH would otherwise roll back the audit row while the client already has
// a secret nobody knows. So: 1. commit service_client.secret_rotation_started
// → 2. PATCH Hydra (detached) → 3. insert service_client.secret_rotated. If
// 3 fails the new secret is still returned (the started row records the
// attempt) and the gap is logged. If 2 fails, secret_rotation_failed is
// recorded (best effort) and the caller may retry.
func (s *ServiceClientService) RotateSecret(ctx context.Context, a Actor, clientID string) (ServiceClientWithSecret, error) {
	if err := require(ctx, s.Authz, a, identity.PermManageServiceClients); err != nil {
		return ServiceClientWithSecret{}, err
	}
	client, err := s.get(ctx, clientID)
	if err != nil {
		return ServiceClientWithSecret{}, err
	}
	secret, err := s.newSecret()
	if err != nil {
		return ServiceClientWithSecret{}, err
	}
	err = s.twoPhase(ctx, a, client,
		audit.ActionServiceClientSecretRotationStarted, audit.ActionServiceClientSecretRotated, audit.ActionServiceClientSecretRotationFailed,
		func(ctx context.Context) error {
			if err := s.Clients.SetSecret(ctx, client.ClientID, secret, TokensValidAfter(s.Clock.Now())); err != nil {
				return fmt.Errorf("set secret: %w", err)
			}
			return nil
		})
	if err != nil {
		return ServiceClientWithSecret{}, err
	}
	return ServiceClientWithSecret{Client: client, Secret: secret}, nil
}

// Delete removes the client and purges the local status cache; other
// replicas stop accepting its tokens within the cache TTL (≤ 30 s).
// Two-phase audit like RotateSecret (deletion_started → DELETE → deleted).
// Permission: manage_service_clients.
func (s *ServiceClientService) Delete(ctx context.Context, a Actor, clientID string) error {
	if err := require(ctx, s.Authz, a, identity.PermManageServiceClients); err != nil {
		return err
	}
	client, err := s.get(ctx, clientID)
	if err != nil {
		return err
	}
	return s.twoPhase(ctx, a, client,
		audit.ActionServiceClientDeletionStarted, audit.ActionServiceClientDeleted, audit.ActionServiceClientDeletionFailed,
		func(ctx context.Context) error {
			if err := s.Clients.Delete(ctx, client.ClientID); err != nil {
				return fmt.Errorf("delete service client: %w", err)
			}
			return nil
		})
}

// twoPhase commits the started row, runs mutate detached from the request,
// purges the status cache and records done (or failed). Only a failure to
// commit the started row or of mutate itself is returned.
func (s *ServiceClientService) twoPhase(ctx context.Context, a Actor, c machine.ServiceClient,
	started, done, failed audit.Action, mutate func(ctx context.Context) error,
) error {
	actorID := a.Principal.IdentityID
	if err := s.appendEvent(ctx, s.event(actorID, a.RequestID, a, started, c, false)); err != nil {
		return fmt.Errorf("record %s: %w", started, err)
	}
	mctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hydraCallTimeout)
	defer cancel()
	merr := mutate(mctx)
	s.invalidate(c.ClientID) // also after a failure: the outcome may be unknown
	final := done
	if merr != nil {
		final = failed
	}
	if err := s.appendEvent(ctx, s.event(actorID, a.RequestID, a, final, c, false)); err != nil && s.Log != nil {
		s.Log.ErrorContext(ctx, "service_client_audit_incomplete",
			"action", string(final), "started_action", string(started), "client_id", c.ClientID,
			"actor_id", actorID.String(), "request_id", a.RequestID, "error", err.Error())
	}
	return merr
}

// appendEvent commits one audit row in its own detached transaction.
func (s *ServiceClientService) appendEvent(ctx context.Context, ev audit.Event) error {
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	return s.Tx.WithinTx(fctx, func(ctx context.Context, r Repos) error { return r.Audit.Append(ctx, ev) })
}

// createAtHydra registers the client under a fresh client_id, detached from
// the request (a cancelled request must not abandon a half-done create). On
// an ambiguous failure (transport error, timeout, 5xx: the client may exist)
// the id is deleted again; if that fails too, possible_orphaned_service_client
// is logged. A conflict or rejection is not ambiguous and deletes nothing.
func (s *ServiceClientService) createAtHydra(ctx context.Context, reg machine.Registration, by uuid.UUID) (machine.ServiceClient, string, error) {
	id := uuid.NewString()
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hydraCallTimeout)
	defer cancel()
	client, secret, err := s.Clients.Create(hctx, NewServiceClient{ClientID: id, Registration: reg, CreatedBy: by})
	if err == nil {
		return client, secret, nil
	}
	if errors.Is(err, ErrDependencyUnavailable) {
		s.compensateAmbiguous(ctx, id, reg.Name, by)
	}
	return machine.ServiceClient{}, "", fmt.Errorf("create service client: %w", err)
}

func (s *ServiceClientService) compensateAmbiguous(ctx context.Context, id, name string, by uuid.UUID) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	err := s.Clients.Delete(cctx, id)
	if s.Log == nil {
		return
	}
	switch {
	case err == nil:
		s.Log.WarnContext(ctx, "service client created despite an error; deleted again", "client_id", id, "actor_id", by.String())
	case errors.Is(err, ErrNotFound):
		// Not created: nothing to clean up.
	default:
		s.Log.ErrorContext(ctx, "possible_orphaned_service_client", "client_id", id, "name", name, "actor_id", by.String())
	}
}

func (s *ServiceClientService) invalidate(clientID string) {
	if s.Verifier != nil {
		s.Verifier.Invalidate(clientID)
	}
}

func (s *ServiceClientService) newSecret() (string, error) {
	b := make([]byte, secretBytes)
	read := s.Random
	if read == nil {
		read = func(p []byte) error { _, err := rand.Read(p); return err }
	}
	if err := read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// compensate deletes a client whose creation could not be recorded.
func (s *ServiceClientService) compensate(ctx context.Context, clientID string) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	if err := s.Clients.Delete(cctx, clientID); err != nil && !errors.Is(err, ErrNotFound) && s.Log != nil {
		s.Log.ErrorContext(ctx, "orphaned_service_client", "client_id", clientID, "error", err.Error())
	}
}

// event builds a service client audit event. Details hold the name (and the
// scopes on create), never the secret.
func (s *ServiceClientService) event(actorID uuid.UUID, requestID string, a Actor, action audit.Action, c machine.ServiceClient, withScopes bool) audit.Event {
	details := map[string]any{"name": c.Name}
	if withScopes {
		sc := make([]string, len(c.Scopes))
		for i, x := range c.Scopes {
			sc[i] = string(x)
		}
		details["scopes"] = sc
	}
	return audit.Event{
		ActorID: actorID, Action: action, TargetType: audit.TargetServiceClient, TargetID: c.ClientID,
		RequestID: requestID, ClientIP: a.ClientIP, Details: details,
	}
}

// storedServiceClient is the replayable (secret-free) response.
type storedServiceClient struct {
	ClientID  string     `json:"client_id"`
	Name      string     `json:"name"`
	Owner     string     `json:"owner"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
}

func toStored(c machine.ServiceClient) storedServiceClient {
	sc := make([]string, len(c.Scopes))
	for i, x := range c.Scopes {
		sc[i] = string(x)
	}
	return storedServiceClient{ClientID: c.ClientID, Name: c.Name, Owner: c.Owner, Scopes: sc, CreatedAt: c.CreatedAt, CreatedBy: c.CreatedBy}
}

func replayServiceClient(rec *IdempotencyRecord, hash []byte) (machine.ServiceClient, error) {
	if rec == nil {
		return machine.ServiceClient{}, fmt.Errorf("%w: idempotency key unavailable", ErrConflict)
	}
	if !bytes.Equal(rec.RequestHash, hash) {
		return machine.ServiceClient{}, fmt.Errorf("%w: idempotency key reused with a different request", ErrConflict)
	}
	if rec.Pending() {
		return machine.ServiceClient{}, fmt.Errorf("%w: a request with this idempotency key is in progress", ErrConflict)
	}
	var st storedServiceClient
	if err := json.Unmarshal(rec.ResponseBody, &st); err != nil || st.ClientID == "" {
		// A key first used for another operation by the same actor.
		return machine.ServiceClient{}, fmt.Errorf("%w: idempotency key reused with a different request", ErrConflict)
	}
	return machine.ServiceClient{
		ClientID: st.ClientID, Name: st.Name, Owner: st.Owner, Scopes: machine.KnownScopes(st.Scopes),
		CreatedAt: st.CreatedAt, CreatedBy: st.CreatedBy,
	}, nil
}

// registrationHash binds the key to the request; the "service_client" tag
// keeps it distinct from hashes of other operations under the same key.
func registrationHash(r machine.Registration) []byte {
	b, _ := json.Marshal(struct {
		Op     string `json:"op"`
		Name   string `json:"name"`
		Owner  string `json:"owner"`
		Scopes string `json:"scopes"`
	}{"service_client.create", r.Name, r.Owner, machine.ScopeString(r.Scopes)})
	sum := sha256.Sum256(b)
	return sum[:]
}
