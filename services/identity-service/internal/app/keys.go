package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/crypto/envelope"
)

// DefaultRewrapBatch is the `keys rewrap` batch size.
const DefaultRewrapBatch = 100

// RewrapResult summarises a re-wrap run.
type RewrapResult struct {
	Scanned   int
	Rewrapped int
	// Current counts keys already wrapped by the newest KEK version.
	Current int
	// Conflicts counts keys changed or erased concurrently (left as is).
	Conflicts int
	// Failed counts keys that could not be unwrapped (integrity errors).
	Failed int
	// LatestVersion / LatestKEKName identify the newest KEK seen.
	LatestVersion int
	LatestKEKName string
}

// KeyRotationService re-wraps data keys after a KEK rotation (PII-FR-09) and
// re-applies erasures after a restore (§9 B1). Operator CLI only.
type KeyRotationService struct {
	Keys        KeyManager
	SubjectKeys SubjectKeyRepo
	// Logins is the login identifier vault (erasure ledger, ADR-0013).
	Logins LoginIdentifierRepo
	Log    *slog.Logger
}

// Rewrap re-wraps every subject key with the newest KEK version: unwrap and
// wrap again with the same associated data, then an optimistic UPDATE
// (WHERE wrapped_dek = old) so concurrent writers and erasures win.
// Idempotent and resumable; keys already at the newest version (learned from
// the first wrap) are skipped without key-manager calls. Dependency errors
// abort the run; integrity failures are counted and reported.
func (s *KeyRotationService) Rewrap(ctx context.Context, batch int) (RewrapResult, error) {
	if batch <= 0 {
		batch = DefaultRewrapBatch
	}
	var res RewrapResult
	after := uuid.Nil
	for {
		keys, err := s.SubjectKeys.ListForRewrap(ctx, after, batch)
		if err != nil {
			return res, fmt.Errorf("list subject keys: %w", err)
		}
		if len(keys) == 0 {
			break
		}
		for _, k := range keys {
			res.Scanned++
			after = k.KeyID
			if res.LatestVersion > 0 && k.Wrapped.KEKName == res.LatestKEKName && k.Wrapped.KEKVersion >= res.LatestVersion {
				res.Current++
				continue
			}
			if err := s.rewrapOne(ctx, k, &res); err != nil {
				return res, err
			}
		}
	}
	if res.Failed > 0 {
		return res, fmt.Errorf("%d subject keys could not be re-wrapped: %w", res.Failed, ErrDataIntegrity)
	}
	return res, nil
}

func (s *KeyRotationService) rewrapOne(ctx context.Context, k SubjectKey, res *RewrapResult) error {
	dek, err := s.Keys.UnwrapDEK(ctx, k.Context(), k.Wrapped)
	if err != nil {
		if errors.Is(err, ErrDataIntegrity) {
			res.Failed++
			s.log().ErrorContext(ctx, "pii_dek_unwrap_failed", "identity_id", k.IdentityID.String(), "key_id", k.KeyID.String())
			return nil
		}
		return fmt.Errorf("unwrap data key: %w", err)
	}
	w, err := s.Keys.WrapDEK(ctx, k.Context(), dek)
	envelope.Zero(dek)
	if err != nil {
		return fmt.Errorf("wrap data key: %w", err)
	}
	if w.KEKVersion >= res.LatestVersion {
		res.LatestVersion, res.LatestKEKName = w.KEKVersion, w.KEKName
	}
	if w.KEKVersion <= k.Wrapped.KEKVersion && w.KEKName == k.Wrapped.KEKName {
		res.Current++
		return nil
	}
	ok, err := s.SubjectKeys.UpdateWrapped(ctx, k.KeyID, k.Wrapped.Ciphertext, w)
	if err != nil {
		return fmt.Errorf("update wrapped key: %w", err)
	}
	if ok {
		res.Rewrapped++
	} else {
		res.Conflicts++
	}
	return nil
}

// ReapplyErasures deletes the subject key of every customer whose latest
// customer.pii.erased audit event is newer than the key (run after any
// database restore, §9 B1). Returns the number of deleted keys.
func (s *KeyRotationService) ReapplyErasures(ctx context.Context) (int64, error) {
	n, err := s.SubjectKeys.DeleteErased(ctx)
	if err != nil {
		return 0, fmt.Errorf("reapply erasures: %w", err)
	}
	return n, nil
}

// ReapplyLoginErasures deletes the logins of identities with a
// customer.login.erased event (run after any restore, with ReapplyErasures).
func (s *KeyRotationService) ReapplyLoginErasures(ctx context.Context) (int64, error) {
	if s.Logins == nil {
		return 0, nil
	}
	n, err := s.Logins.DeleteErased(ctx)
	if err != nil {
		return 0, fmt.Errorf("reapply login erasures: %w", err)
	}
	return n, nil
}

func (s *KeyRotationService) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
