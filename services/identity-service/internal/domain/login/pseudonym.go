package login

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
)

// Domain is the reserved (RFC 6761 ".invalid") domain of every pseudonym.
// Kratos accepts it as format: email (spike S1); no MTA can deliver to it.
const Domain = "login.invalid"

// PseudonymLen is the size of a handle and of the HMAC-SHA256 lookup key.
const PseudonymLen = 32

// encodedLen is the unpadded base32 length of 32 bytes.
const encodedLen = 52

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// Pseudonym is the opaque handle that replaces a login identifier in Kratos.
// Its string form "<base32>@login.invalid" is the only customer identifier
// Kratos stores. Handles created since ADR-0014 are random (NewPseudonym);
// older ones equal the LookupKey of their address (ADR-0013). Nothing derives
// a handle from an address: identity-service finds it through the vault.
type Pseudonym [PseudonymLen]byte

// NewPseudonym returns a random handle.
func NewPseudonym() (Pseudonym, error) {
	var p Pseudonym
	if _, err := rand.Read(p[:]); err != nil {
		return Pseudonym{}, fmt.Errorf("random login handle: %w", err)
	}
	return p, nil
}

// LookupKey is the keyed hash (HMAC-SHA256, OpenBao Transit) of a
// normalised login identifier. It finds the vault entry of an address and
// never leaves identity-service (ADR-0014).
type LookupKey [PseudonymLen]byte

// Hex returns the lower-case hex form (rate-limit keys, never logged).
func (k LookupKey) Hex() string { return hex.EncodeToString(k[:]) }

// String returns "<lower base32>@login.invalid".
func (p Pseudonym) String() string {
	return strings.ToLower(b32.EncodeToString(p[:])) + "@" + Domain
}

// Hex returns the lower-case hex form (used in the AAD).
func (p Pseudonym) Hex() string { return hex.EncodeToString(p[:]) }

// Short is the first 8 hex characters, safe for correlation in logs of
// operator tools only (never in request logs).
func (p Pseudonym) Short() string { return p.Hex()[:8] }

// ParsePseudonym parses the string form strictly. Kratos lower-cases
// identifiers, so any case is accepted.
func ParsePseudonym(s string) (Pseudonym, bool) {
	var p Pseudonym
	s = strings.ToLower(s)
	local, domain, ok := strings.Cut(s, "@")
	if !ok || domain != Domain || len(local) != encodedLen {
		return p, false
	}
	raw, err := b32.DecodeString(strings.ToUpper(local))
	if err != nil || len(raw) != PseudonymLen {
		return p, false
	}
	copy(p[:], raw)
	// Reject non-canonical encodings (trailing bits set).
	if p.String() != s {
		return Pseudonym{}, false
	}
	return p, true
}

// IsPseudonym reports whether s has the pseudonym form.
func IsPseudonym(s string) bool {
	_, ok := ParsePseudonym(s)
	return ok
}

// LookupInput is the domain-separated HMAC input of the lookup key:
// "login-id/v1" ‖ 0x00 ‖ kind ‖ 0x00 ‖ normalised value. The bytes are those
// ADR-0013 hashed into the handle, so lookup keys of existing rows match.
func LookupInput(i Identifier) []byte {
	b := make([]byte, 0, 16+len(i.kind)+len(i.value))
	b = append(b, "login-id/v1\x00"...)
	b = append(b, i.kind...)
	b = append(b, 0)
	return append(b, i.value...)
}

// AAD binds a vault ciphertext to its kind and pseudonym (PLI-NFR-03):
// "identity-service/login/v1" ‖ 0x00 ‖ kind ‖ 0x00 ‖ hex(pseudonym).
func AAD(kind Kind, p Pseudonym) []byte {
	return []byte("identity-service/login/v1\x00" + string(kind) + "\x00" + p.Hex())
}
