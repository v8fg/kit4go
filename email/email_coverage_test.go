package email

import (
	"context"
	"errors"
	"testing"

	gomail "github.com/wneessen/go-mail"
)

func TestNewSMTPSender_WithTLS(t *testing.T) {
	s, err := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithTLS(),
		WithSendFunc(func(_ context.Context, _ *gomail.Client, _ *gomail.Msg) error { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("nil sender")
	}
}

func TestNewSMTPSender_WithSSL(t *testing.T) {
	_, err := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithSSL(),
		WithSendFunc(func(_ context.Context, _ *gomail.Client, _ *gomail.Msg) error { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewSMTPSender_WithAuth(t *testing.T) {
	_, err := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithAuth("user", "pass"),
		WithSendFunc(func(_ context.Context, _ *gomail.Client, _ *gomail.Msg) error { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSend_NoFromAddress(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithSendFunc(func(_ context.Context, _ *gomail.Client, _ *gomail.Msg) error { return nil }),
	)
	err := s.Send(context.Background(), &Message{
		To:      []string{"to@example.com"},
		Subject: "test",
		Text:    "hello",
	})
	if err == nil {
		t.Fatal("should error on missing From")
	}
}

func TestSend_SendFuncError(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(_ context.Context, _ *gomail.Client, _ *gomail.Msg) error {
			return errors.New("SMTP rejected")
		}),
	)
	err := s.Send(context.Background(), &Message{
		To:      []string{"to@example.com"},
		Subject: "test",
		Text:    "hello",
	})
	if err == nil {
		t.Fatal("should propagate sendFunc error")
	}
}

func TestSend_Success(t *testing.T) {
	s, _ := NewSMTPSender(
		WithHost("smtp.example.com"),
		WithDefaultFrom("from@example.com"),
		WithSendFunc(func(_ context.Context, _ *gomail.Client, _ *gomail.Msg) error { return nil }),
	)
	err := s.Send(context.Background(), &Message{
		To:      []string{"to@example.com"},
		Subject: "test",
		Text:    "hello",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
}
