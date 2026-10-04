package kratos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// SelfServiceTimeout bounds each self-service call: Kratos hashes passwords
// (bcrypt) and checks registrations against HaveIBeenPwned.
const SelfServiceTimeout = 10 * time.Second

// SelfService implements app.AuthFlows over the Kratos public API, native
// (API) flows only (ADR-0014): create the flow, then submit it. Requests and
// responses carry passwords, handles and session tokens; none of them is
// ever logged or put into an error.
type SelfService struct{ c client }

var _ app.AuthFlows = (*SelfService)(nil)

// NewSelfService creates the adapter for the Kratos public URL. hc nil
// uses a client with SelfServiceTimeout.
func NewSelfService(publicURL string, hc *http.Client) *SelfService {
	if hc == nil {
		hc = &http.Client{Timeout: SelfServiceTimeout}
	}
	return &SelfService{c: newClient(publicURL, hc)}
}

// clientHeader forwards the end client so Kratos records it on the session
// (devices) instead of identity-service.
func clientHeader(c app.FlowClient) http.Header {
	h := http.Header{}
	if c.IP.IsValid() {
		h.Set("X-Forwarded-For", c.IP.String())
	}
	if c.UserAgent != "" {
		h.Set("User-Agent", c.UserAgent)
	}
	return h
}

// kFlow decodes what is needed from a self-service flow.
type kFlow struct {
	ID string `json:"id"`
	UI struct {
		Messages []kMessage `json:"messages"`
		Nodes    []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
			Messages []kMessage `json:"messages"`
		} `json:"nodes"`
	} `json:"ui"`
}

type kMessage struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
}

// fieldOf maps a Kratos node name to the field of the public contract.
func fieldOf(node string) string {
	switch node {
	case "identifier", "traits.login_id", "traits.email", "email":
		return app.FlowFieldLogin
	case "password":
		return app.FlowFieldPassword
	}
	return app.FlowFieldForm
}

// rejection keeps only the error message ids of a rejected flow and the
// field they belong to: never Kratos text, context or node values (the
// identifier node echoes the handle).
func (f kFlow) rejection() *app.AuthFlowError {
	e := &app.AuthFlowError{}
	for _, m := range f.UI.Messages {
		if m.Type == "error" {
			e.Messages = append(e.Messages, app.FlowMessage{Field: app.FlowFieldForm, ID: m.ID})
		}
	}
	for _, n := range f.UI.Nodes {
		for _, m := range n.Messages {
			if m.Type == "error" {
				e.Messages = append(e.Messages, app.FlowMessage{Field: fieldOf(n.Attributes.Name), ID: m.ID})
			}
		}
	}
	return e
}

// createFlow starts a native flow of kind (login, registration, recovery).
func (s *SelfService) createFlow(ctx context.Context, kind string, h http.Header) (string, error) {
	resp, err := s.c.do(ctx, http.MethodGet, "/self-service/"+kind+"/api", h, nil)
	if err != nil {
		return "", err
	}
	if resp.status != http.StatusOK {
		return "", resp.unexpected("create " + kind + " flow")
	}
	var f kFlow
	if err := resp.decode(&f); err != nil {
		return "", err
	}
	if f.ID == "" {
		return "", fmt.Errorf("kratos create %s flow: no flow id", kind)
	}
	return f.ID, nil
}

// submit posts body to the flow. A 400 with a flow is the flow's rejection.
func (s *SelfService) submit(ctx context.Context, kind, flowID string, h http.Header, body any) (response, error) {
	resp, err := s.c.do(ctx, http.MethodPost, "/self-service/"+kind+"?flow="+url.QueryEscape(flowID), h, body)
	if err != nil {
		return response{}, err
	}
	switch resp.status {
	case http.StatusOK:
		return resp, nil
	case http.StatusGone:
		return response{}, app.ErrAuthFlowExpired
	case http.StatusBadRequest:
		if resp.errorID() == "self_service_flow_expired" {
			return response{}, app.ErrAuthFlowExpired
		}
		var f kFlow
		if json.Unmarshal(resp.body, &f) == nil && f.ID != "" {
			if e := f.rejection(); len(e.Messages) > 0 {
				return response{}, e
			}
		}
	}
	return response{}, resp.unexpected("submit " + kind + " flow")
}

type kAuthResult struct {
	SessionToken string          `json:"session_token"`
	Session      json.RawMessage `json:"session"`
	ContinueWith []struct {
		Action string `json:"action"`
		Flow   struct {
			ID string `json:"id"`
		} `json:"flow"`
	} `json:"continue_with"`
}

func (r kAuthResult) toSession() (app.AuthSession, error) {
	if r.SessionToken == "" || len(r.Session) == 0 {
		return app.AuthSession{}, errors.New("kratos: no session in the response")
	}
	out := app.AuthSession{Token: r.SessionToken, Session: r.Session}
	for _, c := range r.ContinueWith {
		if c.Action == "show_verification_ui" && c.Flow.ID != "" {
			out.VerificationFlowID = c.Flow.ID
		}
	}
	return out, nil
}

// Login implements app.AuthFlows.
func (s *SelfService) Login(ctx context.Context, c app.FlowClient, identifier, password string) (app.AuthSession, error) {
	h := clientHeader(c)
	flow, err := s.createFlow(ctx, "login", h)
	if err != nil {
		return app.AuthSession{}, err
	}
	resp, err := s.submit(ctx, "login", flow, h, map[string]string{
		"method": "password", "identifier": identifier, "password": password,
	})
	if err != nil {
		return app.AuthSession{}, err
	}
	var r kAuthResult
	if err := resp.decode(&r); err != nil {
		return app.AuthSession{}, err
	}
	return r.toSession()
}

// Register implements app.AuthFlows. The customer schema has login_id only.
func (s *SelfService) Register(ctx context.Context, c app.FlowClient, loginID, password string) (app.AuthSession, error) {
	h := clientHeader(c)
	flow, err := s.createFlow(ctx, "registration", h)
	if err != nil {
		return app.AuthSession{}, err
	}
	resp, err := s.submit(ctx, "registration", flow, h, map[string]any{
		"method": "password", "password": password, "traits": map[string]string{"login_id": loginID},
	})
	if err != nil {
		return app.AuthSession{}, err
	}
	var r kAuthResult
	if err := resp.decode(&r); err != nil {
		return app.AuthSession{}, err
	}
	return r.toSession()
}

// StartRecovery implements app.AuthFlows.
func (s *SelfService) StartRecovery(ctx context.Context, c app.FlowClient, email string) (string, error) {
	h := clientHeader(c)
	flow, err := s.createFlow(ctx, "recovery", h)
	if err != nil {
		return "", err
	}
	if _, err := s.submit(ctx, "recovery", flow, h, map[string]string{"method": "code", "email": email}); err != nil {
		return "", err
	}
	return flow, nil
}

// SubmitRecoveryCode implements app.AuthFlows. A valid code answers 200
// with continue_with [set_ory_session_token, show_settings_ui]
// (feature_flags.use_continue_with_transitions); a wrong one 200 with error
// messages on the same flow.
func (s *SelfService) SubmitRecoveryCode(ctx context.Context, c app.FlowClient, flowID, code string) (app.RecoveryGrant, error) {
	resp, err := s.submit(ctx, "recovery", flowID, clientHeader(c), map[string]string{"method": "code", "code": code})
	if err != nil {
		return app.RecoveryGrant{}, err
	}
	var r struct {
		kFlow
		ContinueWith []struct {
			Action          string `json:"action"`
			OrySessionToken string `json:"ory_session_token"`
			Flow            struct {
				ID string `json:"id"`
			} `json:"flow"`
		} `json:"continue_with"`
	}
	if err := resp.decode(&r); err != nil {
		return app.RecoveryGrant{}, err
	}
	var g app.RecoveryGrant
	for _, cw := range r.ContinueWith {
		switch cw.Action {
		case "set_ory_session_token":
			g.SessionToken = cw.OrySessionToken
		case "show_settings_ui":
			g.SettingsFlowID = cw.Flow.ID
		}
	}
	if g.SessionToken != "" {
		return g, nil
	}
	if e := r.rejection(); len(e.Messages) > 0 {
		return app.RecoveryGrant{}, e
	}
	return app.RecoveryGrant{}, errors.New("kratos recovery: no session in the response")
}
