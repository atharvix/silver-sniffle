package email

import (
	"context"
	"log/slog"
	"sync"
)

type MockService struct {
	mu          sync.Mutex
	SentOTPs    map[string]string
	SentWelcome []string
	logger      *slog.Logger
}

func NewMockService(logger *slog.Logger) *MockService {
	return &MockService{
		SentOTPs: make(map[string]string),
		logger:   logger,
	}
}

func (m *MockService) IsConfigured() bool {
	return true
}

func (m *MockService) SendOTP(ctx context.Context, toEmail, otp string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SentOTPs[toEmail] = otp
	m.logger.InfoContext(ctx, "[MOCK EMAIL] OTP Sent", slog.String("email", toEmail), slog.String("otp", otp))
	return nil
}

func (m *MockService) SendWelcome(ctx context.Context, toEmail, name, about string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SentWelcome = append(m.SentWelcome, toEmail)
	m.logger.InfoContext(ctx, "[MOCK EMAIL] Welcome Sent", slog.String("email", toEmail), slog.String("name", name))
	return nil
}
