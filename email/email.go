// Package email sends transactional email via SMTP (wrapping go-mail). Own
// module so the go-mail dep stays isolated.
//
// The design separates the message model from the transport: callers build a
// Message and send it through a Sender interface. SMTPSender implements Sender
// via go-mail's SMTP client; tests inject a capturing sender (or a sendFunc) to
// exercise the full message→MIME conversion without a real SMTP server.
//
// Ad-tech / finance uses: transactional emails (welcome, receipt), verification-
// code delivery (pairs with otp/ + random.NumericCode), alert notifications.
package email

import (
	"context"
	"errors"
	"fmt"

	gomail "github.com/wneessen/go-mail"
)

// Message is a transactional email.
type Message struct {
	From    string   // sender address (uses SMTPSender default if empty)
	To      []string // recipients (required)
	Cc      []string // carbon-copy (optional)
	ReplyTo string   // reply-to address (optional)
	Subject string   // subject line
	Text    string   // plain-text body (at least one of Text/HTML required)
	HTML    string   // HTML body (optional)
}

// Validate checks the message has the required fields.
func (m *Message) Validate() error {
	if len(m.To) == 0 {
		return ErrMissingRecipient
	}
	if m.Subject == "" {
		return ErrMissingSubject
	}
	if m.Text == "" && m.HTML == "" {
		return ErrMissingBody
	}
	return nil
}

// Sender is the send abstraction. SMTPSender is the SMTP implementation; tests
// can use a capturing fake or the WithSendFunc option.
type Sender interface {
	Send(ctx context.Context, msg *Message) error
}

var (
	ErrMissingRecipient = errors.New("email: at least one To address required")
	ErrMissingSubject   = errors.New("email: subject required")
	ErrMissingBody      = errors.New("email: text or HTML body required")
)

// Compile-time interface assertion: guard that SMTPSender stays in sync with
// the Sender contract.
var _ Sender = (*SMTPSender)(nil)

// SMTPSender sends via go-mail's SMTP client.
type SMTPSender struct {
	client      *gomail.Client
	defaultFrom string
	sendFunc    func(*gomail.Client, *gomail.Msg) error
}

// Option configures SMTPSender.
type Option func(*config)

type config struct {
	host        string
	port        int
	username    string
	password    string
	tls         bool
	ssl         bool
	defaultFrom string
	sendFunc    func(*gomail.Client, *gomail.Msg) error
}

// WithHost sets the SMTP server host.
func WithHost(host string) Option { return func(c *config) { c.host = host } }

// WithPort sets the SMTP server port (default 587).
func WithPort(port int) Option { return func(c *config) { c.port = port } }

// WithAuth sets SMTP authentication (username + password, PLAIN auth).
func WithAuth(user, pass string) Option {
	return func(c *config) { c.username = user; c.password = pass }
}

// WithTLS forces STARTTLS (recommended for port 587).
func WithTLS() Option { return func(c *config) { c.tls = true } }

// WithSSL uses implicit TLS (for port 465).
func WithSSL() Option { return func(c *config) { c.ssl = true } }

// WithDefaultFrom sets the From address used when Message.From is empty.
func WithDefaultFrom(from string) Option { return func(c *config) { c.defaultFrom = from } }

// WithSendFunc injects a custom send function (for tests — replaces DialAndSend).
func WithSendFunc(fn func(*gomail.Client, *gomail.Msg) error) Option {
	return func(c *config) { c.sendFunc = fn }
}

// NewSMTPSender builds an SMTP-backed Sender. Host is required.
func NewSMTPSender(opts ...Option) (*SMTPSender, error) {
	cfg := config{port: 587}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.host == "" {
		return nil, errors.New("email: host is required (WithHost)")
	}

	mailOpts := []gomail.Option{
		gomail.WithPort(cfg.port),
	}
	if cfg.username != "" {
		mailOpts = append(mailOpts,
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
			gomail.WithUsername(cfg.username),
			gomail.WithPassword(cfg.password),
		)
	}
	if cfg.ssl {
		mailOpts = append(mailOpts, gomail.WithSSLPort(true))
	} else if cfg.tls {
		mailOpts = append(mailOpts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	} else {
		// Default to Mandatory STARTTLS (matches go-mail's own default). The
		// prior Opportunistic default would silently fall back to plaintext on a
		// STARTTLS downgrade (MITM), exposing the mail body. Auth still fails
		// closed (PlainAuth refuses unencrypted), but Mandatory avoids the
		// downgrade and the confusing error. Use WithSSL for implicit-TLS(465).
		mailOpts = append(mailOpts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}

	client, err := gomail.NewClient(cfg.host, mailOpts...)
	if err != nil {
		return nil, fmt.Errorf("email: SMTP client: %w", err)
	}

	s := &SMTPSender{client: client, defaultFrom: cfg.defaultFrom}
	if cfg.sendFunc != nil {
		s.sendFunc = cfg.sendFunc
	} else {
		s.sendFunc = func(c *gomail.Client, m *gomail.Msg) error {
			return c.DialAndSend(m)
		}
	}
	return s, nil
}

// Send sends a message via SMTP. Validates the message, converts to go-mail's
// MIME format, and sends.
func (s *SMTPSender) Send(_ context.Context, msg *Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	from := msg.From
	if from == "" {
		from = s.defaultFrom
	}
	if from == "" {
		return errors.New("email: no From address (set Message.From or WithDefaultFrom)")
	}

	m := gomail.NewMsg()
	if err := m.From(from); err != nil {
		return fmt.Errorf("email: From: %w", err)
	}
	if err := m.To(msg.To...); err != nil {
		return fmt.Errorf("email: To: %w", err)
	}
	if len(msg.Cc) > 0 {
		if err := m.Cc(msg.Cc...); err != nil {
			return fmt.Errorf("email: Cc: %w", err)
		}
	}
	if msg.ReplyTo != "" {
		if err := m.ReplyTo(msg.ReplyTo); err != nil {
			return fmt.Errorf("email: ReplyTo: %w", err)
		}
	}
	m.Subject(msg.Subject)
	if msg.HTML != "" {
		m.SetBodyString(gomail.TypeTextHTML, msg.HTML)
	}
	if msg.Text != "" {
		if msg.HTML != "" {
			m.AddAlternativeString(gomail.TypeTextPlain, msg.Text)
		} else {
			m.SetBodyString(gomail.TypeTextPlain, msg.Text)
		}
	}

	return s.sendFunc(s.client, m)
}
