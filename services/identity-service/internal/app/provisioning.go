package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// Kratos flow types as sent by the webhook Jsonnet templates.
const (
	FlowAPI     = "api"
	FlowBrowser = "browser"
)

// ProvisioningService handles Kratos webhooks.
type ProvisioningService struct {
	Profiles ProfileRepo
}

// HandleRegistration provisions the profile of a newly registered identity
// (FR-09). Idempotent for webhook retries. Caller: Kratos (webhook key).
func (s *ProvisioningService) HandleRegistration(ctx context.Context, identityID uuid.UUID, schemaID string) error {
	kind, ok := identity.ParseKind(schemaID)
	if !ok {
		return NewValidationError("schema_id", "unknown")
	}
	if identityID == uuid.Nil {
		return NewValidationError("identity_id", "required")
	}
	if err := s.Profiles.Ensure(ctx, identityID, kind); err != nil {
		return fmt.Errorf("ensure profile: %w", err)
	}
	return nil
}

// LoginAllowed is the population guard (ADR-0004): customers only on API
// (native) flows, admins only on browser flows.
func LoginAllowed(schemaID, flowType string) bool {
	switch identity.Kind(schemaID) {
	case identity.KindCustomer:
		return flowType == FlowAPI
	case identity.KindAdmin:
		return flowType == FlowBrowser
	}
	return false
}
