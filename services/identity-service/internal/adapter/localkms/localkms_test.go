package localkms

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

func TestLocalKMS_WrapUnwrapBoundToSubject(t *testing.T) {
	ctx := context.Background()
	k := NewRandom()
	kc := app.KeyContext{IdentityID: uuid.New(), KeyID: uuid.New()}
	dek := bytes.Repeat([]byte{7}, 32)
	w, err := k.WrapDEK(ctx, kc, dek)
	if err != nil || w.KEKVersion != 1 || w.KEKName != KEKName || !strings.HasPrefix(w.Ciphertext, "local:v1:") {
		t.Fatalf("wrap: %+v %v", w, err)
	}
	if strings.Contains(w.Ciphertext, string(dek)) {
		t.Fatal("wrapped contains the DEK")
	}
	got, err := k.UnwrapDEK(ctx, kc, w)
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %v", err)
	}
	for name, other := range map[string]app.KeyContext{
		"other subject": {IdentityID: uuid.New(), KeyID: kc.KeyID},
		"other key id":  {IdentityID: kc.IdentityID, KeyID: uuid.New()},
	} {
		if _, err := k.UnwrapDEK(ctx, other, w); !errors.Is(err, app.ErrDataIntegrity) {
			t.Fatalf("%s: want ErrDataIntegrity, got %v", name, err)
		}
	}
	if _, err := NewRandom().UnwrapDEK(ctx, kc, w); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("other KEK: %v", err)
	}
	for _, bad := range []string{"vault:v1:abc", "local:vX:abc", "local:v1:!!", "local:v9:AAAA", "local:v1:AAAA"} {
		if _, err := k.UnwrapDEK(ctx, kc, app.Wrapped{Ciphertext: bad}); !errors.Is(err, app.ErrDataIntegrity) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
}

func TestLocalKMS_RotateKeepsOldVersionsReadable(t *testing.T) {
	ctx := context.Background()
	k := NewRandom()
	kc := app.KeyContext{IdentityID: uuid.New(), KeyID: uuid.New()}
	dek := bytes.Repeat([]byte{9}, 32)
	w1, _ := k.WrapDEK(ctx, kc, dek)
	if v := k.Rotate(); v != 2 {
		t.Fatalf("rotate: %d", v)
	}
	w2, _ := k.WrapDEK(ctx, kc, dek)
	if w2.KEKVersion != 2 {
		t.Fatalf("new wraps use the newest version: %d", w2.KEKVersion)
	}
	for _, w := range []app.Wrapped{w1, w2} {
		if got, err := k.UnwrapDEK(ctx, kc, w); err != nil || !bytes.Equal(got, dek) {
			t.Fatalf("v%d: %v", w.KEKVersion, err)
		}
	}
}

func TestLocalKMS_BlindIndexDeterministicAndKeyed(t *testing.T) {
	ctx := context.Background()
	k := NewRandom()
	a, _ := k.BlindIndex(ctx, []byte("phone_number:+84901234567"))
	b, _ := k.BlindIndex(ctx, []byte("phone_number:+84901234567"))
	c, _ := k.BlindIndex(ctx, []byte("phone_number:+84901234568"))
	d, _ := NewRandom().BlindIndex(ctx, []byte("phone_number:+84901234567"))
	if len(a.Sum) != 32 || a.KeyVersion != 1 || !bytes.Equal(a.Sum, b.Sum) || bytes.Equal(a.Sum, c.Sum) || bytes.Equal(a.Sum, d.Sum) {
		t.Fatal("blind index must be a deterministic keyed 32-byte hash")
	}
}

func TestLocalKMS_KeysAndFailure(t *testing.T) {
	if _, err := New(make([]byte, 16), make([]byte, 32)); !errors.Is(err, ErrKeySize) {
		t.Fatal("short KEK accepted")
	}
	if _, err := NewFromBase64("not base64", "x"); !errors.Is(err, ErrKeySize) {
		t.Fatal("bad base64 accepted")
	}
	k := NewRandom()
	k.SetFailure(app.ErrDependencyUnavailable)
	if _, err := k.BlindIndex(context.Background(), []byte("x")); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatal("failure injection")
	}
}
