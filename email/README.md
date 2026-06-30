# email

Sends transactional email via SMTP (wrapping go-mail). Own module so the go-mail
dependency stays isolated.

## Why

The design separates the message model from the transport: callers build a
`Message` and send it through a `Sender` interface. `SMTPSender` implements
`Sender` via go-mail's SMTP client; tests inject a `WithSendFunc` to exercise the
full message→MIME conversion without a real SMTP server.

## API

```go
sender, _ := email.NewSMTPSender(
    email.WithHost("smtp.example.com"),
    email.WithPort(587),
    email.WithAuth("user", "pass"),
    email.WithDefaultFrom("noreply@example.com"),
)
err := sender.Send(ctx, &email.Message{
    To:      []string{"user@example.com"},
    Subject: "Welcome",
    Text:    "Welcome aboard!",
    HTML:    "<p>Welcome aboard!</p>",
})
```

| Symbol | Behavior |
|---|---|
| `Message` | From/To/Cc/ReplyTo/Subject/Text/HTML |
| `Message.Validate()` | Checks required fields (To, Subject, Text or HTML) |
| `Sender` interface | `Send(ctx, *Message) error` |
| `NewSMTPSender(opts...)` | Build via go-mail; host required |
| `WithHost/Port/Auth/TLS/SSL/DefaultFrom` | SMTP config options |
| `WithSendFunc(fn)` | Inject a custom send (tests) |

## Testing

81% statement coverage, `-race` clean. Tests inject a `WithSendFunc` that
captures the go-mail Msg, exercising the full Message→MIME conversion without a
real SMTP server. Covers message validation (all required-field errors), Sender
interface satisfaction, default-From fallback, host-required error, sendFunc
error propagation, and capturing-sender round-trip.

```bash
go test -race -cover ./...
```
