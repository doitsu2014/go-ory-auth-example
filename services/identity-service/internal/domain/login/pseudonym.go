package login

import (
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// Domain is the reserved (RFC 6761 ".invalid") domain of every pseudonym.
// Kratos accepts it as format: email (spike S1); no MTA can deliver to it.
const Domain = "login.invalid"

// PseudonymLen is the size of the HMAC-SHA256 output.
const PseudonymLen = 32

// encodedLen is the unpadded base32 length of 32 bytes.
const encodedLen = 52

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// Pseudonym is the keyed hash of a login identifier. Its string form
// "<base32>@login.invalid" is the only customer identifier Kratos stores.
type Pseudonym [PseudonymLen]byte

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

// PseudonymInput is the domain-separated HMAC input:
// "login-id/v1" ‖ 0x00 ‖ kind ‖ 0x00 ‖ normalised value.
func PseudonymInput(i Identifier) []byte {
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
