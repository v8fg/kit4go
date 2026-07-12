package email_test

import (
	"context"
	"fmt"

	"github.com/v8fg/kit4go/email"
)

// ExampleSender demonstrates the Sender interface with a capturing fake — the
// pattern the package itself is tested with (no SMTP server needed). In
// production use SMTPSender: email.NewSMTPSender(email.WithHost(...),
// email.WithPort(...), email.WithAuth(user, pass), email.WithTLS()).
func ExampleSender() {
	var sent []string
	sender := fakeSender{onSend: func(m *email.Message) { sent = append(sent, m.To...) }}

	_ = sender.Send(context.Background(), &email.Message{
		From:    "bot@example.com",
		To:      []string{"alice@example.com", "bob@example.com"},
		Subject: "deploy complete",
		Text:    "build #42 deployed to prod",
	})

	fmt.Println(sent)
	// Output: [alice@example.com bob@example.com]
}

type fakeSender struct{ onSend func(*email.Message) }

func (f fakeSender) Send(_ context.Context, m *email.Message) error { f.onSend(m); return nil }
