package envelope

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func mustDEK(t *testing.T) []byte {
	t.Helper()
	k, err := NewDEK()
	if err != nil || len(k) != KeySize {
		t.Fatalf("NewDEK: %v %d", err, len(k))
	}
	return k
}

func TestPIINFR02_RoundTrip(t *testing.T) {
	dek, id := mustDEK(t), uuid.New()
	aad := AAD("phone_number", id)
	pt := []byte("+84901234567")
	ct, err := Seal(dek, aad, pt)
	if err != nil {
		t.Fatal(err)
	}
	if ct[0] != FormatV1 || len(ct) != len(pt)+Overhead {
		t.Fatalf("format: first byte %x len %d", ct[0], len(ct))
	}
	if bytes.Contains(ct, pt) {
		t.Fatal("ciphertext contains the plaintext")
	}
	got, err := Open(dek, aad, ct)
	if err != nil || !bytes.Equal(got, pt) {
		t.Fatalf("open: %q %v", got, err)
	}
	ct2, _ := Seal(dek, aad, pt)
	if bytes.Equal(ct[1:1+NonceSize], ct2[1:1+NonceSize]) || bytes.Equal(ct, ct2) {
		t.Fatal("nonce must be random per write")
	}
	empty, err := Seal(dek, aad, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Open(dek, aad, empty); err != nil || len(got) != 0 {
		t.Fatalf("empty plaintext: %v", err)
	}
}

func TestPIINFR02_OpenFailures(t *testing.T) {
	dek, id := mustDEK(t), uuid.New()
	aad := AAD("address", id)
	ct, err := Seal(dek, aad, []byte(`{"line1":"1 Nguyen Trai"}`))
	if err != nil {
		t.Fatal(err)
	}
	flip := func(i int) []byte {
		c := append([]byte(nil), ct...)
		c[i] ^= 0x01
		return c
	}
	other := mustDEK(t)
	cases := map[string]struct {
		dek, aad, ct []byte
	}{
		"wrong key":             {other, aad, ct},
		"other row (AAD)":       {dek, AAD("address", uuid.New()), ct},
		"other column (AAD)":    {dek, AAD("national_id", id), ct},
		"tampered ciphertext":   {dek, aad, flip(len(ct) - TagSize - 1)},
		"tampered tag":          {dek, aad, flip(len(ct) - 1)},
		"tampered nonce":        {dek, aad, flip(1)},
		"unknown format byte":   {dek, aad, append([]byte{0x02}, ct[1:]...)},
		"truncated":             {dek, aad, ct[:Overhead-1]},
		"empty":                 {dek, aad, nil},
		"format byte zero only": {dek, aad, []byte{FormatV1}},
	}
	for name, tc := range cases {
		if _, err := Open(tc.dek, tc.aad, tc.ct); !errors.Is(err, ErrDecrypt) {
			t.Errorf("%s: want ErrDecrypt, got %v", name, err)
		}
	}
	if _, err := Open([]byte("short"), aad, ct); !errors.Is(err, ErrKeySize) {
		t.Fatalf("bad key size: %v", err)
	}
	if _, err := Seal(make([]byte, 16), aad, []byte("x")); !errors.Is(err, ErrKeySize) {
		t.Fatalf("AES-128 key must be refused: %v", err)
	}
}

func TestA7_AADBindsFormatColumnAndRow(t *testing.T) {
	id := uuid.MustParse("5d9c2c61-6a1e-4b8f-9b8a-2f9d6f0c1e11")
	got := string(AAD("phone_number", id))
	want := "identity-service/pii/v1\x00\x01phone_number5d9c2c61-6a1e-4b8f-9b8a-2f9d6f0c1e11"
	if got != want {
		t.Fatalf("AAD %q want %q", got, want)
	}
}

func TestZero(t *testing.T) {
	b := []byte{1, 2, 3}
	Zero(b)
	if !bytes.Equal(b, []byte{0, 0, 0}) {
		t.Fatal("not zeroed")
	}
}
