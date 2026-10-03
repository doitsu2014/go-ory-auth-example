package kratos

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

type traits struct {
	Email string `json:"email"`
	Name  *struct {
		First string `json:"first,omitempty"`
		Last  string `json:"last,omitempty"`
	} `json:"name,omitempty"`
}

type verifiableAddress struct {
	Value    string `json:"value"`
	Verified bool   `json:"verified"`
	Via      string `json:"via"`
}

type kIdentity struct {
	ID                  uuid.UUID             `json:"id"`
	SchemaID            string                `json:"schema_id"`
	State               string                `json:"state"`
	StateChangedAt      *time.Time            `json:"state_changed_at"`
	Traits              traits                `json:"traits"`
	VerifiableAddresses []verifiableAddress   `json:"verifiable_addresses"`
	Credentials         map[string]credential `json:"credentials"`
	CreatedAt           time.Time             `json:"created_at"`
}

// credential decodes only type and timestamps; configs are never needed.
type credential struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

type kSession struct {
	ID              uuid.UUID `json:"id"`
	Active          bool      `json:"active"`
	ExpiresAt       time.Time `json:"expires_at"`
	AuthenticatedAt time.Time `json:"authenticated_at"`
	AAL             string    `json:"authenticator_assurance_level"`
	Identity        kIdentity `json:"identity"`
}

func (k kIdentity) emailVerified() bool {
	for _, a := range k.VerifiableAddresses {
		if a.Via == "email" && strings.EqualFold(a.Value, k.Traits.Email) {
			return a.Verified
		}
	}
	return false
}

func (k kIdentity) name() identity.Name {
	if k.Traits.Name == nil {
		return identity.Name{}
	}
	return identity.Name{First: k.Traits.Name.First, Last: k.Traits.Name.Last}
}

func (k kIdentity) toDomain() identity.Identity {
	totp, hasTOTP := k.Credentials["totp"]
	out := identity.Identity{
		ID:            k.ID,
		SchemaID:      k.SchemaID,
		State:         identity.State(k.State),
		Email:         k.Traits.Email,
		EmailVerified: k.emailVerified(),
		Name:          k.name(),
		HasTOTP:       hasTOTP,
		TOTPCreatedAt: totp.CreatedAt,
		CreatedAt:     k.CreatedAt,
	}
	if k.StateChangedAt != nil {
		out.StateChangedAt = *k.StateChangedAt
	}
	return out
}

func (s kSession) toPrincipal() (identity.Principal, bool) {
	kind, ok := identity.ParseKind(s.Identity.SchemaID)
	if !ok {
		return identity.Principal{}, false
	}
	aal := identity.AAL1
	if s.AAL == string(identity.AAL2) {
		aal = identity.AAL2
	}
	return identity.Principal{
		IdentityID:        s.Identity.ID,
		SessionID:         s.ID,
		Kind:              kind,
		AAL:               aal,
		AuthenticatedAt:   s.AuthenticatedAt,
		ExpiresAt:         s.ExpiresAt,
		Email:             s.Identity.Traits.Email,
		EmailVerified:     s.Identity.emailVerified(),
		Name:              s.Identity.name(),
		IdentityCreatedAt: s.Identity.CreatedAt,
	}, true
}
