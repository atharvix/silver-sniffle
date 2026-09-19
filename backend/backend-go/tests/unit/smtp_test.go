package unit

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/atharvix/kinjo-backend/internal/email"
)

func TestSMTPServiceConfiguration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Not configured
	svcUnconf := email.NewSMTPService("", 587, "", "", "", "", "tls", logger, nil)
	if svcUnconf.IsConfigured() {
		t.Errorf("expected IsConfigured() to be false when host is empty")
	}

	err := svcUnconf.SendOTP(context.Background(), "test@example.com", "1234")
	if err == nil {
		t.Errorf("expected SendOTP to return error when not configured")
	}

	err = svcUnconf.SendWelcome(context.Background(), "test@example.com", "John Doe", "Engineer")
	if err == nil {
		t.Errorf("expected SendWelcome to return error when not configured")
	}

	// Configured
	svcConf := email.NewSMTPService("smtp.example.com", 587, "user", "pass", "hello@kinjo.world", "Kinjo", "tls", logger, nil)
	if !svcConf.IsConfigured() {
		t.Errorf("expected IsConfigured() to be true when host and sender are set")
	}
}
