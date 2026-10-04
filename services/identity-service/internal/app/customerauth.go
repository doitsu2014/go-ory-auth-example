package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// FlowClient is the end client a Kratos flow is driven for: Kratos records
// it on the session (devices) instead of identity-service's address.
type FlowClient struct {
	IP        netip.Addr
	UserAgent string
}

// AuthSession is a Kratos session issued to a customer: the session token
// and the session object as Kratos returns it to its owner. It redacts
// itself when formatted or logged.
type AuthSession struct {
	Token   string
	Session json.RawMessage
	// VerificationFlowID is set after a registration when Kratos started a
	// verification flow (code already sent).
	VerificationFlowID string
}

// String implements fmt.Stringer.
func (AuthSession) String() string { return "AuthSession(<redacted>)" }

// LogValue implements slog.LogValuer.
func (AuthSession) LogValue() slog.Value { return slog.StringValue("<redacted>") }

// AuthFlows drives Kratos native (API) self-service flows on a customer's
// behalf (ADR-0014). The identifier is always a Kratos handle (or a legacy
// email during the migration), never what the customer typed. A flow Kratos
// rejects is an *AuthFlowError; transport failures ErrDependencyUnavailable.
// Errors never carry identifiers, passwords or tokens.
type AuthFlows interface {
	Login(ctx context.Context, c FlowClient, identifier, password string) (AuthSession, error)
	Register(ctx context.Context, c FlowClient, loginID, password string) (AuthSession, error)
	// StartRecovery creates a recovery flow (method code) for email and
	// returns its id; Kratos answers the same whether or not it exists.
	StartRecovery(ctx context.Context, c FlowClient, email string) (flowID string, err error)
	// SubmitRecoveryCode submits a recovery code. A wrong code is an
	// *AuthFlowError, an expired flow ErrAuthFlowExpired.
	SubmitRecoveryCode(ctx context.Context, c FlowClient, flowID, code string) (RecoveryGrant, error)
}

// RecoveryGrant is the privileged session Kratos issues for a valid
// recovery code, and the settings flow to set the new password in. It
// redacts itself when formatted or logged.
type RecoveryGrant struct {
	SessionToken   string
	SettingsFlowID string
}

// String implements fmt.Stringer.
func (RecoveryGrant) String() string { return "RecoveryGrant(<redacted>)" }

// LogValue implements slog.LogValuer.
func (RecoveryGrant) LogValue() slog.Value { return slog.StringValue("<redacted>") }

// ErrAuthFlowExpired: the Kratos flow behind a request expired (start over).
var ErrAuthFlowExpired = errors.New("auth flow expired")

// Flow message fields (api-contract: auth_flow_rejected).
const (
	FlowFieldLogin    = "login"
	FlowFieldPassword = "password"
	FlowFieldForm     = "form"
)

// KratosInvalidCredentials is Kratos's "the provided credentials are
// invalid" message id.
const KratosInvalidCredentials = 4000006

// FlowMessage is one error message of a rejected Kratos flow: the stable
// Kratos message id and the field it belongs to. Kratos text, context and
// node values are never kept: the flow echoes the identifier.
type FlowMessage struct {
	Field string
	ID    int
}

// AuthFlowError is a Kratos flow rejection (invalid credentials, password
// policy, duplicate account, …).
type AuthFlowError struct{ Messages []FlowMessage }

func (e *AuthFlowError) Error() string {
	return fmt.Sprintf("auth flow rejected (%d messages)", len(e.Messages))
}

// Has reports whether the rejection carries message id.
func (e *AuthFlowError) Has(id int) bool {
	for _, m := range e.Messages {
		if m.ID == id {
			return true
		}
	}
	return false
}

func invalidCredentials(err error) bool {
	var fe *AuthFlowError
	return errors.As(err, &fe) && fe.Has(KratosInvalidCredentials)
}

// CustomerAuthService signs customers in, registers them and starts
// password recovery through Kratos on their behalf (ADR-0014, PLX-FR-01..03).
// The customer sends the email / phone number; the Kratos handle is found
// (or created) in the vault and never leaves this service, so no endpoint
// maps an address to its handle. Passwords exist only in memory for one
// request and are never logged.
type CustomerAuthService struct {
	Logins *LoginIdentifierService
	Flows  AuthFlows
	// Limiters (nil = unlimited). SignIn (login, recovery) and Register are
	// per IPBucket, Net per NetBucket (all routes), Account counts failed
	// sign-ins per lookup key.
	SignInLimiter   *KeyedLimiter[string]
	RegisterLimiter *KeyedLimiter[string]
	NetLimiter      *KeyedLimiter[string]
	AccountLimiter  *KeyedLimiter[string]
}

// Credentials is the input of Login and Register.
type Credentials struct {
	Client   FlowClient
	Type     string
	Value    string
	Password string
}

// MaxPasswordLength bounds the password accepted from a customer.
const MaxPasswordLength = 1024

func (in Credentials) validatePassword() error {
	switch {
	case in.Password == "":
		return NewValidationError("password", "required")
	case len(in.Password) > MaxPasswordLength:
		return NewValidationError("password", "too_long")
	}
	return nil
}

// limit checks and records the per-IP and per-network limits.
func (s *CustomerAuthService) limit(ip netip.Addr, perIP *KeyedLimiter[string]) error {
	bucket, net := IPBucket(ip), NetBucket(ip)
	if !perIP.Peek(bucket, 1) || !s.NetLimiter.Peek(net, 1) {
		return ErrRateLimited
	}
	perIP.Allow(bucket)
	s.NetLimiter.Allow(net)
	return nil
}

// candidates are the Kratos identifiers to try for an address, in order: its
// handle, then (migration transition) the legacy plaintext email when no
// bound handle exists. An address with neither gets a fresh random decoy, so
// unknown and known addresses take the same path (PLX-NFR-03).
func (s *CustomerAuthService) candidates(ctx context.Context, id login.Identifier) ([]string, login.LookupKey, error) {
	rec, k, err := s.Logins.find(ctx, id)
	found := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, k, err
	}
	var out []string
	if found {
		out = append(out, rec.Pseudonym.String())
	}
	if s.Logins.Phase != PhaseComplete && id.Kind() == login.KindEmail && (!found || rec.IdentityID == nil) {
		out = append(out, id.Value())
	}
	if len(out) == 0 {
		decoy, err := login.NewPseudonym()
		if err != nil {
			return nil, k, err
		}
		out = append(out, decoy.String())
	}
	return out, k, nil
}

// Login signs a customer in (PLX-FR-01). An unknown address, a wrong
// password and a disabled account are all an *AuthFlowError from Kratos.
func (s *CustomerAuthService) Login(ctx context.Context, in Credentials) (AuthSession, error) {
	if err := in.validatePassword(); err != nil {
		return AuthSession{}, err
	}
	if err := s.limit(in.Client.IP, s.SignInLimiter); err != nil {
		return AuthSession{}, err
	}
	id, err := s.Logins.parse(in.Type, in.Value, "login.")
	if err != nil {
		return AuthSession{}, err
	}
	ids, k, err := s.candidates(ctx, id)
	if err != nil {
		return AuthSession{}, err
	}
	// Every attempt is recorded before Kratos is called, so concurrent
	// guesses cannot all pass the check (SEC review #2).
	if !s.AccountLimiter.Allow(k.Hex()) {
		return AuthSession{}, ErrRateLimited
	}
	var sess AuthSession
	for _, identifier := range ids {
		sess, err = s.Flows.Login(ctx, in.Client, identifier, in.Password)
		if !invalidCredentials(err) {
			break
		}
	}
	return sess, err
}

// Register registers a customer (PLX-FR-02): the address is stored sealed
// under its handle first; Kratos's pre-registration webhook checks the
// handle and the after-registration webhook binds it.
func (s *CustomerAuthService) Register(ctx context.Context, in Credentials) (AuthSession, error) {
	if err := in.validatePassword(); err != nil {
		return AuthSession{}, err
	}
	if err := s.limit(in.Client.IP, s.RegisterLimiter); err != nil {
		return AuthSession{}, err
	}
	id, err := s.Logins.parse(in.Type, in.Value, "login.")
	if err != nil {
		return AuthSession{}, err
	}
	p, err := s.Logins.claim(ctx, id, nil, false)
	if err != nil {
		return AuthSession{}, err
	}
	return s.Flows.Register(ctx, in.Client, p.String(), in.Password)
}

// RecoveryRequest is the input of StartRecovery.
type RecoveryRequest struct {
	Client FlowClient
	Type   string
	Value  string
}

// recoveryAD binds a sealed recovery reference to its purpose.
var recoveryAD = []byte("identity-service/recovery-flow/v1")

// maxRecoveryIDLength bounds the sealed reference accepted from a client.
const maxRecoveryIDLength = 1024

// sealedRefRe is the shape of a key-manager ciphertext ("vault:vN:…" from
// OpenBao, "local:vN:…" from the development key manager).
var sealedRefRe = regexp.MustCompile(`^(vault|local):v[0-9]{1,6}:[A-Za-z0-9+/]+={0,2}$`)

// StartRecovery starts a recovery flow (method code) for an address and
// returns an opaque reference to it (PLX-FR-03): the Kratos flow id sealed
// with the login KEK. The flow id itself never leaves identity-service,
// because anyone can read a Kratos recovery flow by id and it carries the
// handle it was started for. The answer has the same shape whether or not
// an account exists; Kratos's courier delivers the code only when it does.
func (s *CustomerAuthService) StartRecovery(ctx context.Context, in RecoveryRequest) (string, error) {
	if err := s.limit(in.Client.IP, s.SignInLimiter); err != nil {
		return "", err
	}
	id, err := s.Logins.parse(in.Type, in.Value, "login.")
	if err != nil {
		return "", err
	}
	target, err := s.recoveryTarget(ctx, id)
	if err != nil {
		return "", err
	}
	flowID, err := s.Flows.StartRecovery(ctx, in.Client, target)
	if err != nil {
		return "", err
	}
	ref, _, err := s.Logins.Keys.SealLogin(ctx, recoveryAD, []byte(flowID))
	if err != nil {
		return "", fmt.Errorf("seal recovery reference: %w", err)
	}
	return ref, nil
}

// recoveryTarget is the Kratos identifier recovery is started for: the
// address's handle; during the transition, the legacy email when the
// address has no handle, or an unbound one no Kratos identity uses (a
// registration attempt for a legacy customer's address leaves such a row,
// SEC review #3); else a random decoy.
func (s *CustomerAuthService) recoveryTarget(ctx context.Context, id login.Identifier) (string, error) {
	rec, _, err := s.Logins.find(ctx, id)
	found := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", err
	}
	legacy := s.Logins.Phase != PhaseComplete && id.Kind() == login.KindEmail
	switch {
	case found && (rec.IdentityID != nil || !legacy):
		return rec.Pseudonym.String(), nil
	case found:
		owners, _, err := s.Logins.Identities.ListIdentities(ctx, IdentityQuery{Email: rec.Pseudonym.String()})
		if err != nil {
			return "", fmt.Errorf("find identity: %w", err)
		}
		for _, it := range owners {
			if it.IsCustomer() {
				return rec.Pseudonym.String(), nil
			}
		}
		return id.Value(), nil
	case legacy:
		return id.Value(), nil
	}
	decoy, err := login.NewPseudonym()
	if err != nil {
		return "", err
	}
	return decoy.String(), nil
}

// RecoveryCodeRequest is the input of SubmitRecoveryCode.
type RecoveryCodeRequest struct {
	Client     FlowClient
	RecoveryID string
	Code       string
}

// SubmitRecoveryCode submits a recovery code on the flow behind an opaque
// reference from StartRecovery (PLX-FR-03) and returns the privileged
// session and settings flow; the new password then goes to Kratos directly.
func (s *CustomerAuthService) SubmitRecoveryCode(ctx context.Context, in RecoveryCodeRequest) (RecoveryGrant, error) {
	if err := s.limit(in.Client.IP, s.SignInLimiter); err != nil {
		return RecoveryGrant{}, err
	}
	if !login.ValidCode(in.Code) {
		return RecoveryGrant{}, NewValidationError("code", "invalid_format")
	}
	if len(in.RecoveryID) > maxRecoveryIDLength || !sealedRefRe.MatchString(in.RecoveryID) {
		return RecoveryGrant{}, NewValidationError("recovery_id", "invalid")
	}
	pts, errs, err := s.Logins.Keys.OpenLogins(ctx, []SealedLogin{{AD: recoveryAD, Ciphertext: in.RecoveryID}})
	if err != nil {
		return RecoveryGrant{}, fmt.Errorf("open recovery reference: %w", err)
	}
	if errs[0] != nil {
		return RecoveryGrant{}, NewValidationError("recovery_id", "invalid")
	}
	flowID := string(pts[0])
	if _, err := uuid.Parse(flowID); err != nil {
		return RecoveryGrant{}, NewValidationError("recovery_id", "invalid")
	}
	return s.Flows.SubmitRecoveryCode(ctx, in.Client, flowID, in.Code)
}
