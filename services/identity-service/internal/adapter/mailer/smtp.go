// Package mailer sends identity-service's own emails (admin invitations) over
// SMTP. Kratos does not email admin-created recovery codes.
package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// SMTP implements app.Mailer.
//
// SMTP_URL forms: smtp://host:port (plain, local dev), smtp://user:pass@host:587
// (STARTTLS required when credentials are set), smtps://host:465 (implicit TLS).
type SMTP struct {
	addr      string
	host      string
	implicit  bool
	auth      smtp.Auth
	from      mail.Address
	timeout   time.Duration
	tlsConfig *tls.Config
}

var _ app.Mailer = (*SMTP)(nil)

// New parses SMTP_URL.
func New(smtpURL, from string) (*SMTP, error) {
	u, err := url.Parse(smtpURL)
	if err != nil {
		return nil, errors.New("SMTP_URL is not a valid URL") // never echo it: may hold credentials
	}
	if u.Scheme != "smtp" && u.Scheme != "smtps" {
		return nil, fmt.Errorf("SMTP_URL: unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = map[string]string{"smtp": "25", "smtps": "465"}[u.Scheme]
	}
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("SMTP_FROM: %w", err)
	}
	s := &SMTP{
		addr: net.JoinHostPort(host, port), host: host, implicit: u.Scheme == "smtps",
		from: *fromAddr, timeout: 10 * time.Second,
		tlsConfig: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12},
	}
	if u.User != nil {
		pw, _ := u.User.Password()
		s.auth = smtp.PlainAuth("", u.User.Username(), pw, host)
	}
	return s, nil
}

var invitationTmpl = template.Must(template.New("inv").Parse(`Hello{{if .Name}} {{.Name}}{{end}},

You have been invited to the go-ory-auth-example admin console with the role "{{.Role}}".

1. Open this link: {{.Link}}
2. Enter this code: {{.Code}}
3. Set a password and enrol an authenticator app (TOTP) in the same session.

The invitation expires at {{.ExpiresAt}}. If you did not expect this email, ignore it.
`))

// SendInvitation implements app.Mailer. The link and code are secrets: they
// go only into the message body, never into logs or errors.
func (s *SMTP) SendInvitation(ctx context.Context, inv app.Invitation) error {
	to, err := mail.ParseAddress(inv.To)
	if err != nil {
		return fmt.Errorf("invalid recipient")
	}
	var body bytes.Buffer
	name := strings.TrimSpace(inv.Name.First + " " + inv.Name.Last)
	if err := invitationTmpl.Execute(&body, map[string]string{
		"Name": name, "Role": string(inv.Role), "Link": inv.Link, "Code": inv.Code,
		"ExpiresAt": inv.ExpiresAt.UTC().Format(time.RFC1123),
	}); err != nil {
		return fmt.Errorf("render invitation: %w", err)
	}
	msg := s.compose(to.Address, "Your admin console invitation", body.Bytes())
	return s.send(ctx, to.Address, msg)
}

func (s *SMTP) compose(to, subject string, body []byte) []byte {
	var b bytes.Buffer
	idBytes := make([]byte, 12)
	_, _ = rand.Read(idBytes)
	fmt.Fprintf(&b, "From: %s\r\n", s.from.String())
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(idBytes), s.from.Address[strings.LastIndex(s.from.Address, "@")+1:])
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.Write(bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n")))
	return b.Bytes()
}

func (s *SMTP) send(ctx context.Context, to string, msg []byte) error {
	d := net.Dialer{Timeout: s.timeout}
	var conn net.Conn
	var err error
	if s.implicit {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: s.tlsConfig}).DialContext(ctx, "tcp", s.addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", s.addr)
	}
	if err != nil {
		return smtpErr("dial", err)
	}
	deadline := time.Now().Add(s.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return smtpErr("handshake", err)
	}
	defer func() { _ = c.Close() }()
	if !s.implicit {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(s.tlsConfig); err != nil {
				return smtpErr("starttls", err)
			}
		} else if s.auth != nil {
			return fmt.Errorf("smtp: refusing to send credentials without TLS")
		}
	}
	if s.auth != nil {
		if err := c.Auth(s.auth); err != nil {
			return smtpErr("auth", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return smtpErr("mail from", err)
	}
	if err := c.Rcpt(to); err != nil {
		return smtpErr("rcpt", err)
	}
	w, err := c.Data()
	if err != nil {
		return smtpErr("data", err)
	}
	if _, err := w.Write(msg); err != nil {
		return smtpErr("write", err)
	}
	if err := w.Close(); err != nil {
		return smtpErr("close data", err)
	}
	if err := c.Quit(); err != nil {
		return smtpErr("quit", err)
	}
	return nil
}

// smtpErr never includes the server's reply text, which may echo the
// recipient address; only the stage and the SMTP status code are kept.
func smtpErr(stage string, err error) error {
	var tp *textproto.Error
	if errors.As(err, &tp) {
		return fmt.Errorf("smtp %s: status %d", stage, tp.Code)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Errorf("smtp %s: timeout", stage)
	}
	return fmt.Errorf("smtp %s failed", stage)
}
