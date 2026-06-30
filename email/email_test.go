package email

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	gomail "github.com/wneessen/go-mail"
)

// capturingSender records messages without sending.
type capturingSender struct {
	mu   sync.Mutex
	msgs []*Message
}

func (c *capturingSender) Send(_ context.Context, msg *Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, msg)
	return nil
}

func TestMessageValidation(t *testing.T) {
	require.ErrorIs(t, (&Message{}).Validate(), ErrMissingRecipient)
	require.ErrorIs(t, (&Message{To: []string{"x@y.com"}}).Validate(), ErrMissingSubject)
	require.ErrorIs(t, (&Message{To: []string{"x@y.com"}, Subject: "s"}).Validate(), ErrMissingBody)
	require.NoError(t, (&Message{To: []string{"x@y.com"}, Subject: "s", Text: "body"}).Validate())
	require.NoError(t, (&Message{To: []string{"x@y.com"}, Subject: "s", HTML: "<p>body</p>"}).Validate())
}

func TestCapturingSender(t *testing.T) {
	var cap capturingSender
	require.NoError(t, cap.Send(context.Background(), &Message{
		To: []string{"user@example.com"}, Subject: "hi", Text: "hello",
	}))
	require.Len(t, cap.msgs, 1)
	require.Equal(t, "hi", cap.msgs[0].Subject)
}

func TestSMTPSenderBuildsWithOptions(t *testing.T) {
	// Build with an injected sendFunc that captures the go-mail Msg (no SMTP).
	var capturedMsg *gomail.Msg
	s, err := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithPort(587),
		WithAuth("user", "pass"),
		WithDefaultFrom("noreply@example.com"),
		WithSendFunc(func(_ *gomail.Client, m *gomail.Msg) error {
			capturedMsg = m
			return nil
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, s)

	err = s.Send(context.Background(), &Message{
		To:      []string{"dest@example.com"},
		Cc:      []string{"cc@example.com"},
		ReplyTo: "reply@example.com",
		Subject: "Test Subject",
		Text:    "plain body",
		HTML:    "<p>html body</p>",
	})
	require.NoError(t, err)
	require.NotNil(t, capturedMsg)
}

func TestSMTPSenderDefaultFromUsedWhenEmpty(t *testing.T) {
	s, err := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("default@example.com"),
		WithSendFunc(func(_ *gomail.Client, m *gomail.Msg) error { return nil }),
	)
	require.NoError(t, err)
	err = s.Send(context.Background(), &Message{
		To: []string{"x@y.com"}, Subject: "s", Text: "b",
		// From intentionally empty → uses defaultFrom
	})
	require.NoError(t, err)
}

func TestSMTPSenderRequiresFrom(t *testing.T) {
	s, err := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithSendFunc(func(_ *gomail.Client, m *gomail.Msg) error { return nil }),
	)
	require.NoError(t, err)
	err = s.Send(context.Background(), &Message{
		To: []string{"x@y.com"}, Subject: "s", Text: "b",
		// No From and no defaultFrom → error
	})
	require.Error(t, err)
}

func TestSMTPSenderRequiresHost(t *testing.T) {
	_, err := NewSMTPSender()
	require.Error(t, err)
}

func TestSMTPSenderSendValidation(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithSendFunc(func(_ *gomail.Client, m *gomail.Msg) error { return nil }),
	)
	// Empty To → validation error before send.
	err := s.Send(context.Background(), &Message{Subject: "s", Text: "b"})
	require.ErrorIs(t, err, ErrMissingRecipient)
}

func TestSenderInterface(t *testing.T) {
	var _ Sender = (*SMTPSender)(nil)
	var _ Sender = (*capturingSender)(nil)
}

func TestSendFuncError(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("test@example.com"),
		WithSendFunc(func(_ *gomail.Client, _ *gomail.Msg) error {
			return context.DeadlineExceeded
		}),
	)
	err := s.Send(context.Background(), &Message{
		To: []string{"x@y.com"}, Subject: "s", Text: "b",
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
