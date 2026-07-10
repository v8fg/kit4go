package email

import (
	"context"
	"strings"
	"testing"
	"time"

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

	// Invoking the closure must delegate to DialAndSendWithContext and surface
	// an error (no SMTP server on 127.0.0.1:1). This covers the default-branch
	// closure body that calls c.DialAndSendWithContext(ctx, m).
	m := gomail.NewMsg()
	require.NoError(t, m.From("from@example.com"))
	require.NoError(t, m.To("to@example.com"))
	m.Subject("t")
	m.SetBodyString(gomail.TypeTextPlain, "body")
	err = s.sendFunc(context.Background(), s.client, m)
	require.Error(t, err, "default sendFunc should attempt DialAndSendWithContext and fail")
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
		WithSendFunc(func(context.Context, *gomail.Client, *gomail.Msg) error { return nil }),
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
		WithSendFunc(func(context.Context, *gomail.Client, *gomail.Msg) error { return nil }),
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
		WithSendFunc(func(context.Context, *gomail.Client, *gomail.Msg) error { return nil }),
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
		WithSendFunc(func(context.Context, *gomail.Client, *gomail.Msg) error { return nil }),
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
		WithSendFunc(func(context.Context, *gomail.Client, *gomail.Msg) error {
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
		WithSendFunc(func(context.Context, *gomail.Client, *gomail.Msg) error {
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

// TestSend_PreCancelledContext is the R24 regression test: Send must thread the
// caller's ctx into the send func. With a custom sendFunc that blocks until ctx
// is done, a pre-cancelled ctx must let Send return promptly (propagating the
// context error) rather than blocking for the full SMTP dial timeout. This
// proves the ctx is actually forwarded instead of being discarded.
func TestSend_PreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(c context.Context, _ *gomail.Client, _ *gomail.Msg) error {
			// Block until ctx is done; mirror a dial that honors cancellation.
			<-c.Done()
			return c.Err()
		}),
	)

	start := time.Now()
	err := s.Send(ctx, &Message{
		To:      []string{"to@example.com"},
		Subject: "s",
		Text:    "b",
	})
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.Canceled,
		"a pre-cancelled ctx must surface context.Canceled, not block")
	// The send func returns the instant ctx is observed done. Allow generous
	// slack for the scheduler; the point is "not the full dial timeout (~15s)".
	require.Less(t, elapsed, time.Second,
		"Send must return promptly on a pre-cancelled ctx, took %v", elapsed)
}

// TestSend_ThreadsContextToDefaultSendFunc verifies the production default
// sendFunc closure actually receives the caller's ctx (regression guard that
// the default branch wiring passes ctx through, not a throwaway).
func TestSend_ThreadsContextToDefaultSendFunc(t *testing.T) {
	s, err := NewSMTPSender(WithHost("smtp.example.com"))
	require.NoError(t, err)
	require.NotNil(t, s.sendFunc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Invoke the default closure with a pre-cancelled ctx against a
	// non-listening host. The result must be a fast failure (ctx cancellation
	// or dial error), never a clean nil — proving ctx is wired into the call.
	m := gomail.NewMsg()
	require.NoError(t, m.From("from@example.com"))
	require.NoError(t, m.To("to@example.com"))
	m.Subject("t")
	m.SetBodyString(gomail.TypeTextPlain, "b")

	start := time.Now()
	err = s.sendFunc(ctx, s.client, m)
	elapsed := time.Since(start)

	require.Error(t, err, "default sendFunc with a cancelled ctx must not succeed")
	require.Less(t, elapsed, time.Second,
		"default sendFunc must honor a cancelled ctx promptly, took %v", elapsed)
}
