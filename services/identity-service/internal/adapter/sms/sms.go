// Package sms implements app.SMSSender (PLI DD-06):
//
//   - Sink (SMS_PROVIDER=sink, local/test only): writes the text as an email
//     to "<digits>@sms.local" through the SMTP mailer, so it shows up in
//     Mailpit next to the other local mail.
//   - HTTP (SMS_PROVIDER=http): POSTs {"to", "text"} as JSON to a provider
//     URL with a bearer token. Any provider-specific gateway sits behind it.
//
// Errors never carry the number or the text.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// Sink delivers SMS to the local mail catcher.
type Sink struct {
	Mailer app.Mailer
}

var _ app.SMSSender = (*Sink)(nil)

// SendSMS implements app.SMSSender.
func (s *Sink) SendSMS(ctx context.Context, to, text string) error {
	digits := strings.TrimPrefix(to, "+")
	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return fmt.Errorf("%w: sms sink: invalid number", app.ErrPermanentDelivery)
	}
	m := login.Message{Subject: "SMS", Text: text + "\n"}
	if err := s.Mailer.SendLoginMessage(ctx, digits+"@"+login.SMSSinkDomain, m); err != nil {
		return fmt.Errorf("%w: sms sink: %v", app.ErrDependencyUnavailable, err)
	}
	return nil
}

// HTTP delivers SMS through a JSON HTTP provider.
type HTTP struct {
	url   string
	token string
	http  *http.Client
}

var _ app.SMSSender = (*HTTP)(nil)

// NewHTTP validates the provider URL; https is required unless allowHTTP.
func NewHTTP(rawURL, token string, allowHTTP bool, hc *http.Client) (*HTTP, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(allowHTTP && u.Scheme == "http")) {
		return nil, errors.New("SMS_HTTP_URL must be an https URL")
	}
	if token == "" {
		return nil, errors.New("SMS_HTTP_TOKEN is required")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	c := *hc
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &HTTP{url: u.String(), token: token, http: &c}, nil
}

// SendSMS implements app.SMSSender. Any non-2xx is transient.
func (h *HTTP) SendSMS(ctx context.Context, to, text string) error {
	body, err := json.Marshal(map[string]string{"to": to, "text": text})
	if err != nil {
		return errors.New("sms http: encode request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return errors.New("sms http: build request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp, err := h.http.Do(req)
	clear(body)
	if err != nil {
		return fmt.Errorf("%w: sms http: transport", app.ErrDependencyUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		return nil
	case resp.StatusCode >= 400 && resp.StatusCode <= 499 && resp.StatusCode != http.StatusRequestTimeout &&
		resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusUnauthorized &&
		resp.StatusCode != http.StatusForbidden:
		// The provider rejected this message (invalid number, blocked
		// destination): retrying cannot succeed. 401/403 are our
		// credentials and stay transient so an operator can fix them.
		return fmt.Errorf("%w: sms http: status %d", app.ErrPermanentDelivery, resp.StatusCode)
	}
	return fmt.Errorf("%w: sms http: status %d", app.ErrDependencyUnavailable, resp.StatusCode)
}
