package email

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	gomail "github.com/wneessen/go-mail"
)

// TestNewSMTPSender_DefaultSendFunc exercises the production default-sendFunc
// branch (no WithSendFunc injected). We cannot dial a real SMTP server in a
// unit test, but constructing the sender hits the else-branch that assigns the
// DialAndSend closure, and we can confirm the closure is non-nil and that
// invoking it against a non-listening host surfaces a network error (proving it
// really calls DialAndSend rather than panicking).
func TestNewSMTPSender_DefaultSendFunc(t *testing.T) {
	s, err := NewSMTPSender(
		WithHost("127.0.0.1"),
		WithPort(1), // nothing listening on port 1
	)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.NotNil(t, s.sendFunc, "default sendFunc (DialAndSend) must be wired")

	// Invoking the closure must delegate to DialAndSend and surface an error
	// (no SMTP server on 127.0.0.1:1). This covers the default-branch closure
	// body that just calls c.DialAndSend(m).
	m := gomail.NewMsg()
	require.NoError(t, m.From("from@example.com"))
	require.NoError(t, m.To("to@example.com"))
	m.Subject("t")
	m.SetBodyString(gomail.TypeTextPlain, "body")
	err = s.sendFunc(s.client, m)
	require.Error(t, err, "default sendFunc should attempt DialAndSend and fail")
}

// TestNewSMTPSender_BadPort covers the gomail.NewClient error path by passing
// an invalid port (negative), which gomail rejects during construction.
func TestNewSMTPSender_BadPort(t *testing.T) {
	_, err := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithPort(-1),
	)
	require.Error(t, err, "gomail should reject an invalid port")
	require.Contains(t, err.Error(), "SMTP client",
		"error should be wrapped by the NewSMTPSender fmt.Errorf")
}

// TestSend_FromError exercises the m.From error branch in Send. An empty-string
// From after defaults cannot be passed (Send guards empty-from earlier), so we
// use an invalid address format that gomail.From rejects.
func TestSend_FromError(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithSendFunc(func(*gomail.Client, *gomail.Msg) error { return nil }),
	)
	err := s.Send(context.Background(), &Message{
		From:    "not an email",
		To:      []string{"to@example.com"},
		Subject: "s",
		Text:    "b",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "From:")
}

// TestSend_ToError exercises the m.To error branch.
func TestSend_ToError(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(*gomail.Client, *gomail.Msg) error { return nil }),
	)
	err := s.Send(context.Background(), &Message{
		To:      []string{"not an email"},
		Subject: "s",
		Text:    "b",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "To:")
}

// TestSend_CcError exercises the m.Cc error branch.
func TestSend_CcError(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(*gomail.Client, *gomail.Msg) error { return nil }),
	)
	err := s.Send(context.Background(), &Message{
		To:      []string{"to@example.com"},
		Cc:      []string{"bad cc"},
		Subject: "s",
		Text:    "b",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cc:")
}

// TestSend_ReplyToError exercises the m.ReplyTo error branch.
func TestSend_ReplyToError(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(*gomail.Client, *gomail.Msg) error { return nil }),
	)
	err := s.Send(context.Background(), &Message{
		To:      []string{"to@example.com"},
		ReplyTo: "bad reply-to",
		Subject: "s",
		Text:    "b",
	})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "ReplyTo:"),
		"want ReplyTo error, got %v", err)
}

// TestSend_HTMLOnly exercises the HTML-only (no Text) SetBodyString branch.
func TestSend_HTMLOnly(t *testing.T) {
	var sent bool
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(*gomail.Client, *gomail.Msg) error {
			sent = true
			return nil
		}),
	)
	err := s.Send(context.Background(), &Message{
		To:      []string{"to@example.com"},
		Subject: "s",
		HTML:    "<p>hi</p>",
	})
	require.NoError(t, err)
	require.True(t, sent)
}

// TestSend_ValidationError confirms Send short-circuits on Validate errors
// without ever invoking the From/To parsing.
func TestSend_ValidationError(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(*gomail.Client, *gomail.Msg) error {
			t.Fatal("sendFunc must not be called on validation failure")
			return nil
		}),
	)
	require.ErrorIs(t, s.Send(context.Background(), &Message{
		Subject: "s", Text: "b",
	}), ErrMissingRecipient)
	require.ErrorIs(t, s.Send(context.Background(), &Message{
		To: []string{"x@y.com"}, Text: "b",
	}), ErrMissingSubject)
	require.ErrorIs(t, s.Send(context.Background(), &Message{
		To: []string{"x@y.com"}, Subject: "s",
	}), ErrMissingBody)
}

