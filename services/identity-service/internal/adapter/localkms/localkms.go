// Package localkms is an in-process KeyManager for tests and local
// development (PII_KMS_PROVIDER=local, only with APP_ENV local|test, §9 A15):
// AES-256-GCM wrapping with a local KEK (versioned, rotatable) and
// HMAC-SHA256 with a local index key. It mirrors the OpenBao adapter's
// semantics: associated data binds a wrapped DEK to its subject, and
// authentication failures are app.ErrDataIntegrity.
//
// It also implements app.LoginKeys. The login HMAC key and the first login
// KEK are derived from the configured keys with HMAC-SHA256 and fixed labels,
// so they are distinct from the PII keys without extra configuration.
package localkms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// KEKName is the reported KEK name.
const KEKName = "local-kek"

const prefix = "local:v"

// ErrKeySize is returned for keys that are not 32 bytes.
var ErrKeySize = errors.New("localkms: keys must be 32 bytes")

// KMS implements app.KeyManager.
type KMS struct {
	mu      sync.RWMutex
	keks    [][]byte // index = version-1
	bidxKey []byte
	// loginHMAC and loginKEKs back app.LoginKeys (index = version-1).
	loginHMAC []byte
	loginKEKs [][]byte
	failWith  error
}

var (
	_ app.KeyManager = (*KMS)(nil)
	_ app.LoginKeys  = (*KMS)(nil)
)

func derive(key []byte, label string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(label))
	return h.Sum(nil)
}

// New creates a KMS from a 32-byte KEK and a 32-byte index key.
func New(kek, bidxKey []byte) (*KMS, error) {
	if len(kek) != 32 || len(bidxKey) != 32 {
		return nil, ErrKeySize
	}
	return &KMS{
		keks: [][]byte{append([]byte(nil), kek...)}, bidxKey: append([]byte(nil), bidxKey...),
		loginHMAC: derive(bidxKey, "localkms/login-pseudonym/v1"),
		loginKEKs: [][]byte{derive(kek, "localkms/login-kek/v1")},
	}, nil
}

// NewFromBase64 decodes standard base64 keys (PII_LOCAL_KEK, PII_LOCAL_BIDX_KEY).
func NewFromBase64(kek, bidxKey string) (*KMS, error) {
	k, err1 := base64.StdEncoding.DecodeString(kek)
	b, err2 := base64.StdEncoding.DecodeString(bidxKey)
	if err1 != nil || err2 != nil {
		return nil, ErrKeySize
	}
	return New(k, b)
}

// NewRandom creates a KMS with random keys (tests).
func NewRandom() *KMS {
	k, b := make([]byte, 32), make([]byte, 32)
	_, _ = rand.Read(k)
	_, _ = rand.Read(b)
	m, _ := New(k, b)
	return m
}

// Rotate adds a new KEK version (and a new login KEK version); new wraps use
// it, old versions still unwrap.
func (m *KMS) Rotate() int {
	k, lk := make([]byte, 32), make([]byte, 32)
	_, _ = rand.Read(k)
	_, _ = rand.Read(lk)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keks = append(m.keks, k)
	m.loginKEKs = append(m.loginKEKs, lk)
	return len(m.keks)
}

// SetFailure makes every call fail with err (nil restores); tests use it to
// simulate an unavailable key manager.
func (m *KMS) SetFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failWith = err
}

func (m *KMS) aead(version int) (cipher.AEAD, error) {
	if version < 1 || version > len(m.keks) {
		return nil, app.ErrDataIntegrity
	}
	block, err := aes.NewCipher(m.keks[version-1])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// WrapDEK implements app.KeyManager.
func (m *KMS) WrapDEK(_ context.Context, kc app.KeyContext, dek []byte) (app.Wrapped, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failWith != nil {
		return app.Wrapped{}, m.failWith
	}
	v := len(m.keks)
	a, err := m.aead(v)
	if err != nil {
		return app.Wrapped{}, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return app.Wrapped{}, err
	}
	ct := a.Seal(nonce, nonce, dek, kc.AssociatedData())
	return app.Wrapped{
		Ciphertext: prefix + strconv.Itoa(v) + ":" + base64.StdEncoding.EncodeToString(ct),
		KEKName:    KEKName, KEKVersion: v,
	}, nil
}

// UnwrapDEK implements app.KeyManager.
func (m *KMS) UnwrapDEK(_ context.Context, kc app.KeyContext, w app.Wrapped) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failWith != nil {
		return nil, m.failWith
	}
	rest, ok := strings.CutPrefix(w.Ciphertext, prefix)
	if !ok {
		return nil, app.ErrDataIntegrity
	}
	vs, b64, ok := strings.Cut(rest, ":")
	v, err := strconv.Atoi(vs)
	if !ok || err != nil {
		return nil, app.ErrDataIntegrity
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, app.ErrDataIntegrity
	}
	a, err := m.aead(v)
	if err != nil {
		return nil, err
	}
	if len(raw) < a.NonceSize() {
		return nil, app.ErrDataIntegrity
	}
	dek, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], kc.AssociatedData())
	if err != nil {
		return nil, fmt.Errorf("localkms unwrap: %w", app.ErrDataIntegrity)
	}
	return dek, nil
}

// BlindIndex implements app.KeyManager (HMAC-SHA256, key version 1).
func (m *KMS) BlindIndex(_ context.Context, input []byte) (app.BlindIndex, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failWith != nil {
		return app.BlindIndex{}, m.failWith
	}
	h := hmac.New(sha256.New, m.bidxKey)
	h.Write(input)
	return app.BlindIndex{Sum: h.Sum(nil), KeyVersion: 1}, nil
}

// Pseudonym implements app.LoginKeys (HMAC-SHA256, key version 1).
func (m *KMS) Pseudonym(_ context.Context, input []byte) (login.Pseudonym, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failWith != nil {
		return login.Pseudonym{}, m.failWith
	}
	h := hmac.New(sha256.New, m.loginHMAC)
	h.Write(input)
	var p login.Pseudonym
	copy(p[:], h.Sum(nil))
	return p, nil
}

func loginAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealLogin implements app.LoginKeys (AES-256-GCM with ad, newest version).
func (m *KMS) SealLogin(_ context.Context, ad, plaintext []byte) (string, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failWith != nil {
		return "", 0, m.failWith
	}
	v := len(m.loginKEKs)
	a, err := loginAEAD(m.loginKEKs[v-1])
	if err != nil {
		return "", 0, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", 0, err
	}
	ct := a.Seal(nonce, nonce, plaintext, ad)
	return prefix + strconv.Itoa(v) + ":" + base64.StdEncoding.EncodeToString(ct), v, nil
}

// OpenLogins implements app.LoginKeys.
func (m *KMS) OpenLogins(_ context.Context, items []app.SealedLogin) ([][]byte, []error, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.failWith != nil {
		return nil, nil, m.failWith
	}
	pts := make([][]byte, len(items))
	errs := make([]error, len(items))
	for i, it := range items {
		pts[i], errs[i] = m.openLogin(it)
	}
	return pts, errs, nil
}

func (m *KMS) openLogin(it app.SealedLogin) ([]byte, error) {
	fail := fmt.Errorf("localkms open login: %w", app.ErrDataIntegrity)
	rest, ok := strings.CutPrefix(it.Ciphertext, prefix)
	if !ok {
		return nil, fail
	}
	vs, b64, ok := strings.Cut(rest, ":")
	v, err := strconv.Atoi(vs)
	if !ok || err != nil || v < 1 || v > len(m.loginKEKs) {
		return nil, fail
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fail
	}
	a, err := loginAEAD(m.loginKEKs[v-1])
	if err != nil || len(raw) < a.NonceSize() {
		return nil, fail
	}
	pt, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], it.AD)
	if err != nil {
		return nil, fail
	}
	return pt, nil
}
