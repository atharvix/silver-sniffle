package unit

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/atharvix/kinjo-backend/internal/notification"
)

func TestFCMServiceConfiguration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Mock mode (no credentials)
	fcmMock := notification.NewFCMService("", "", "", logger)
	if fcmMock.IsConfigured() {
		t.Errorf("expected IsConfigured() to be false when no credentials provided")
	}

	// Sending in mock mode should succeed gracefully without error
	err := fcmMock.Send(context.Background(), "mock-device-token", "Test Title", "Test Body", nil)
	if err != nil {
		t.Errorf("expected Send in mock mode to return nil, got: %v", err)
	}

	// Legacy server key mode
	fcmLegacy := notification.NewFCMService("", "", "sample-server-key", logger)
	if !fcmLegacy.IsConfigured() {
		t.Errorf("expected IsConfigured() to be true when serverKey is provided")
	}
}
