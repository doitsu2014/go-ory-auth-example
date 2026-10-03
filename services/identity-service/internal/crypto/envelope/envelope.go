// Package envelope seals personal information columns with a per-subject
// data encryption key (DEK): AES-256-GCM (NIST SP 800-38D), a random 96-bit
// nonce per write, and associated data that binds the ciphertext to its
// column, its row and the format version (technical spec §9 A7).
//
// Ciphertext format (bytea): FormatV1 ‖ nonce(12) ‖ ciphertext ‖ tag(16).
//
// Only the standard library is used. Go's GC may copy key material; zeroing
// is defence in depth only (DD-12).
package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"github.com/google/uuid"
)

// Sizes.
const (
	KeySize   = 32
	NonceSize = 12
	TagSize   = 16
	// FormatV1 is the leading format byte. It is also part of the AAD, so
	// it cannot be changed without failing authentication.
	FormatV1 byte = 0x01
	// Overhead is the ciphertext expansion of FormatV1.
	Overhead = 1 + NonceSize + TagSize
)

// aadPrefix domain-separates this service's AAD (the NUL ends the label).
const aadPrefix = "identity-service/pii/v1\x00"

// Errors. They never carry key or plaintext material.
var (
	// ErrDecrypt means authentication failed: wrong key, wrong AAD (other
	// row or column), tampering, or an unknown format byte.
	ErrDecrypt = errors.New("envelope: authentication failed")
	// ErrKeySize means the DEK is not 32 bytes.
	ErrKeySize = errors.New("envelope: invalid key size")
)

// NewDEK returns 32 random bytes from crypto/rand.
func NewDEK() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, nil
}

// AAD returns the associated data of a column of a subject's row:
// "identity-service/pii/v1\x00" ‖ FormatV1 ‖ column ‖ identity_id.
// The column names are a closed set and the id has a fixed width, so the
// concatenation is unambiguous.
func AAD(column string, id uuid.UUID) []byte {
	s := id.String()
	b := make([]byte, 0, len(aadPrefix)+1+len(column)+len(s))
	b = append(b, aadPrefix...)
	b = append(b, FormatV1)
	b = append(b, column...)
	b = append(b, s...)
	return b
}

func gcm(dek []byte) (cipher.AEAD, error) {
	if len(dek) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, ErrKeySize
	}
	return cipher.NewGCM(block)
}

// Seal encrypts pt under dek with aad and returns FormatV1 ‖ nonce ‖ ct ‖ tag.
func Seal(dek, aad, pt []byte) ([]byte, error) {
	a, err := gcm(dek)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+NonceSize, 1+NonceSize+len(pt)+TagSize)
	out[0] = FormatV1
	if _, err := rand.Read(out[1 : 1+NonceSize]); err != nil {
		return nil, err
	}
	return a.Seal(out, out[1:1+NonceSize], pt, aad), nil
}

// Open authenticates and decrypts a Seal output. Every failure is ErrDecrypt
// (or ErrKeySize for a malformed key).
func Open(dek, aad, ct []byte) ([]byte, error) {
	a, err := gcm(dek)
	if err != nil {
		return nil, err
	}
	if len(ct) < Overhead || ct[0] != FormatV1 {
		return nil, ErrDecrypt
	}
	pt, err := a.Open(nil, ct[1:1+NonceSize], ct[1+NonceSize:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// Zero overwrites b with zeros (best effort, DD-12).
func Zero(b []byte) { clear(b) }
