// Package mail is a thin SMTP sender used by the magic-link login flow. It
// wraps net/smtp so we can support STARTTLS (port 587), implicit TLS (465)
// and plain SMTP (25, for dev) without pulling a third-party dependency.
//
// MIME assembly is intentionally minimal — a multipart/alternative message
// with one text/plain and one text/html part. That covers magic-link emails
// well enough; templated newsletters are out of scope.
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// TLSMode selects how the SMTP session is upgraded. The string values match
// the JSON the admin panel writes into the settings table verbatim so the
// store does not have to translate.
type TLSMode string

const (
	TLSStartTLS TLSMode = "starttls" // submit ports (587), most public SMTP
	TLSImplicit TLSMode = "tls"      // smtps (465), older or AWS SES style
	TLSNone     TLSMode = "none"     // plaintext, mostly for local relays
)

// Config carries everything the sender needs. Values flow from the settings
// table; the admin panel validates them before they reach this struct.
type Config struct {
	Host          string
	Port          int
	User          string
	Password      string
	FromName      string
	FromEmail     string
	ReplyTo       string
	SubjectPrefix string
	BCC           string
	TLSMode       TLSMode
}

// Sender is the package's main type. Keep one per active config; rebuild on
// settings change.
type Sender struct {
	cfg Config
}

func New(cfg Config) *Sender {
	if cfg.TLSMode == "" {
		cfg.TLSMode = TLSStartTLS
	}
	return &Sender{cfg: cfg}
}

// dialTimeout caps how long a misconfigured host can stall the login flow.
const dialTimeout = 15 * time.Second

// Send dispatches one email. ctx is honored only at the dial step — net/smtp
// has no per-call deadline, so once the handshake completes the rest runs to
// completion. That is acceptable: SMTP is fast once you are past TLS.
func (s *Sender) Send(ctx context.Context, to, subject, htmlBody, textBody string) error {
	if s == nil {
		return fmt.Errorf("mail: sender not configured")
	}
	cfg := s.cfg
	if cfg.Host == "" || cfg.FromEmail == "" {
		return fmt.Errorf("mail: missing host or from address")
	}

	subject = strings.TrimSpace(subject)
	if cfg.SubjectPrefix != "" {
		subject = strings.TrimSpace(cfg.SubjectPrefix) + " " + subject
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	var (
		conn net.Conn
		err  error
	)
	dialer := &net.Dialer{Timeout: dialTimeout}
	switch cfg.TLSMode {
	case TLSImplicit:
		tlsCfg := &tls.Config{ServerName: cfg.Host}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	default:
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("mail: smtp client: %w", err)
	}
	defer func() { _ = client.Quit() }()

	if cfg.TLSMode == TLSStartTLS {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}

	if cfg.User != "" && cfg.Password != "" {
		auth := smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}

	fromAddr := cfg.FromEmail
	if err := client.Mail(fromAddr); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO %s: %w", to, err)
	}
	if cfg.BCC != "" {
		if err := client.Rcpt(cfg.BCC); err != nil {
			return fmt.Errorf("mail: RCPT BCC %s: %w", cfg.BCC, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write([]byte(assembleMessage(cfg, to, subject, htmlBody, textBody))); err != nil {
		return fmt.Errorf("mail: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: close data: %w", err)
	}
	return nil
}

// assembleMessage renders RFC 5322 + multipart/alternative. Boundary is a
// fixed string with high enough entropy that it will not collide with the
// body content for our short magic-link emails.
func assembleMessage(cfg Config, to, subject, htmlBody, textBody string) string {
	from := (&mail.Address{Name: cfg.FromName, Address: cfg.FromEmail}).String()
	const boundary = "mc-mail-boundary-7b4a3f8e2c1d"

	h := strings.Builder{}
	h.WriteString("From: ")
	h.WriteString(from)
	h.WriteString("\r\n")
	h.WriteString("To: ")
	h.WriteString(to)
	h.WriteString("\r\n")
	if cfg.ReplyTo != "" {
		h.WriteString("Reply-To: ")
		h.WriteString(cfg.ReplyTo)
		h.WriteString("\r\n")
	}
	h.WriteString("Subject: ")
	h.WriteString(mimeHeaderEncode(subject))
	h.WriteString("\r\n")
	h.WriteString("MIME-Version: 1.0\r\n")
	h.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	h.WriteString("\r\n")

	h.WriteString("--" + boundary + "\r\n")
	h.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	h.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	h.WriteString(textBody)
	h.WriteString("\r\n\r\n")

	h.WriteString("--" + boundary + "\r\n")
	h.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	h.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	h.WriteString(htmlBody)
	h.WriteString("\r\n\r\n")
	h.WriteString("--" + boundary + "--\r\n")
	return h.String()
}

// mimeHeaderEncode wraps non-ASCII subjects in =?utf-8?b?…?= so accents and
// emoji survive servers that ban 8bit headers. Pure ASCII subjects pass
// through untouched to keep the wire format readable in transit.
func mimeHeaderEncode(s string) string {
	for _, r := range s {
		if r > 0x7e {
			return "=?utf-8?b?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}
