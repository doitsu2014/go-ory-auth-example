package login

// Purpose is why a client resolves an identifier (api-contract §3).
type Purpose string

// Purposes.
const (
	PurposeRegistration Purpose = "registration"
	PurposeSignIn       Purpose = "sign_in"
	PurposeRecovery     Purpose = "recovery"
	PurposeVerification Purpose = "verification"
)

// ParsePurpose validates a purpose.
func ParsePurpose(s string) (Purpose, bool) {
	switch Purpose(s) {
	case PurposeRegistration, PurposeSignIn, PurposeRecovery, PurposeVerification:
		return Purpose(s), true
	}
	return "", false
}

// Persists reports whether resolving for this purpose stores the encrypted
// address (only registration does, PLI-FR-02).
func (p Purpose) Persists() bool { return p == PurposeRegistration }
