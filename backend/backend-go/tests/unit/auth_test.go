package unit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atharvix/kinjo-backend/internal/auth"
	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/email"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type MockAuthRepo struct {
	SavedOTPs        map[string]string
	SavedTokens      map[string]string
	VerifiedEmails   map[string]bool
	Passwords        map[string]string
	PasswordVerified map[string]bool
}

func NewMockAuthRepo() *MockAuthRepo {
	return &MockAuthRepo{
		SavedOTPs:        make(map[string]string),
		SavedTokens:      make(map[string]string),
		VerifiedEmails:   make(map[string]bool),
		Passwords:        make(map[string]string),
		PasswordVerified: make(map[string]bool),
	}
}

func (m *MockAuthRepo) CreatePasswordAccount(ctx context.Context, email, passwordHash string) error {
	if _, exists := m.Passwords[email]; exists {
		return domain.ErrConflict
	}
	m.Passwords[email] = passwordHash
	return nil
}

func (m *MockAuthRepo) GetPasswordAccount(ctx context.Context, email string) (string, bool, error) {
	hash, ok := m.Passwords[email]
	if !ok {
		return "", false, domain.ErrUnauthorized
	}
	return hash, m.PasswordVerified[email], nil
}

func (m *MockAuthRepo) MarkPasswordAccountVerified(ctx context.Context, email string) error {
	m.PasswordVerified[email] = true
	return nil
}

func (m *MockAuthRepo) IssueToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) error {
	m.SavedTokens[tokenHash] = email
	return nil
}

func (m *MockAuthRepo) SaveOTP(ctx context.Context, email, otpHash string, expiresAt time.Time) error {
	m.SavedOTPs[email] = otpHash
	return nil
}

func (m *MockAuthRepo) VerifyAndIssueToken(ctx context.Context, email, plainOTP string, tokenHash string, tokenExpiresAt time.Time, maxAttempts int) error {
	expectedHash := auth.HashString(plainOTP)
	if m.SavedOTPs[email] != expectedHash {
		return domain.NewAppError(400, "Incorrect OTP", nil)
	}
	delete(m.SavedOTPs, email)
	m.SavedTokens[tokenHash] = email
	m.VerifiedEmails[email] = true
	return nil
}

func (m *MockAuthRepo) GetEmailFromToken(ctx context.Context, tokenHash string) (string, error) {
	if email, ok := m.SavedTokens[tokenHash]; ok {
		return email, nil
	}
	return "", domain.NewAppError(401, "Invalid token", nil)
}

func (m *MockAuthRepo) IsEmailVerified(ctx context.Context, email string) (bool, error) {
	return m.VerifiedEmails[email], nil
}

func (m *MockAuthRepo) ConsumeVerifiedEmail(ctx context.Context, email string) error {
	delete(m.VerifiedEmails, email)
	return nil
}

func (m *MockAuthRepo) CleanupExpired(ctx context.Context) error {
	return nil
}

func (m *MockAuthRepo) DeleteAccount(ctx context.Context, email string) error {
	return nil
}

func (m *MockAuthRepo) EnsureGoogleProfile(ctx context.Context, email, name string) error {
	return nil
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		input   string
		valid   bool
		cleaned string
	}{
		{"test@example.com", true, "test@example.com"},
		{"  USER@Domain.COM  ", true, "user@domain.com"},
		{"invalid-email", false, ""},
		{"@nodomain.com", false, ""},
		{"noatsign.com", false, ""},
		{"", false, ""},
	}

	for _, tt := range tests {
		got, err := auth.ValidateEmail(tt.input)
		if (err == nil) != tt.valid {
			t.Errorf("ValidateEmail(%q) err = %v, want valid=%v", tt.input, err, tt.valid)
		}
		if tt.valid && got != tt.cleaned {
			t.Errorf("ValidateEmail(%q) = %q, want %q", tt.input, got, tt.cleaned)
		}
	}
}

func TestOTPGeneration(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		otp, err := auth.GenerateSecureOTP()
		if err != nil {
			t.Fatalf("GenerateSecureOTP() error: %v", err)
		}
		if len(otp) != 4 {
			t.Errorf("GenerateSecureOTP() length = %d, want 4", len(otp))
		}
		seen[otp] = true
	}
	if len(seen) < 80 {
		t.Errorf("GenerateSecureOTP() entropy too low, only %d unique values in 100 runs", len(seen))
	}
}

func TestGoogleTokenValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id_token") != "fake-google-token" {
			t.Fatalf("unexpected token: %s", r.URL.Query().Get("id_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"aud":"test-google-client-id","email":"google-user@example.com","email_verified":true,"iss":"https://accounts.google.com","exp":4102444800}`))
	}))
	defer server.Close()

	oldURL := auth.GoogleTokenInfoURL
	auth.GoogleTokenInfoURL = server.URL + "/tokeninfo"
	defer func() { auth.GoogleTokenInfoURL = oldURL }()

	claims, err := auth.VerifyGoogleToken(context.Background(), "fake-google-token", "test-google-client-id")
	if err != nil {
		t.Fatalf("VerifyGoogleToken returned error: %v", err)
	}
	if claims.Email != "google-user@example.com" {
		t.Fatalf("claims.Email = %q, want google-user@example.com", claims.Email)
	}
}

func TestAuthService_FullFlow(t *testing.T) {
	repo := NewMockAuthRepo()
	logger := observability.NewNopLogger()
	mockEmail := email.NewMockService(logger)
	cfg := &config.Config{
		OtpTTL:         10 * time.Minute,
		TokenTTL:       24 * time.Hour,
		MaxOtpAttempts: 5,
		Environment:    "development",
	}

	svc := auth.NewService(repo, mockEmail, cfg, logger, nil)
	ctx := context.Background()

	// 1. Send OTP
	sendResp, err := svc.SendOTP(ctx, "user@kinjo.world")
	if err != nil {
		t.Fatalf("SendOTP error: %v", err)
	}
	if !sendResp.Success {
		t.Errorf("SendOTP success = false, want true")
	}

	otp := mockEmail.SentOTPs["user@kinjo.world"]
	if otp == "" {
		t.Fatalf("No OTP sent to user@kinjo.world")
	}

	// 2. Verify OTP
	verifyResp, err := svc.VerifyOTP(ctx, "user@kinjo.world", otp)
	if err != nil {
		t.Fatalf("VerifyOTP error: %v", err)
	}
	if verifyResp.VerificationToken == "" {
		t.Fatalf("VerificationToken is empty")
	}

	// 3. Validate Token
	emailFromToken, err := svc.GetEmailFromToken(ctx, verifyResp.VerificationToken)
	if err != nil {
		t.Fatalf("GetEmailFromToken error: %v", err)
	}
	if emailFromToken != "user@kinjo.world" {
		t.Errorf("GetEmailFromToken = %q, want user@kinjo.world", emailFromToken)
	}

	// 4. Send Welcome
	welcomeResp, err := svc.SendWelcome(ctx, "user@kinjo.world", "Kinjo User", "Software Engineer")
	if err != nil {
		t.Fatalf("SendWelcome error: %v", err)
	}
	if !welcomeResp.Success {
		t.Errorf("SendWelcome success = false, want true")
	}
}

func TestAccountLockout(t *testing.T) {
	repo := NewMockAuthRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockEmail := email.NewMockService(logger)
	cfg := &config.Config{TokenTTL: 1 * time.Hour}
	svc := auth.NewService(repo, mockEmail, cfg, logger, nil)

	ctx := context.Background()
	emailStr := "lockout@test.com"

	// Create an account
	_, err := svc.SignUp(ctx, emailStr, "Password123!")
	if err != nil {
		t.Fatalf("SignUp error: %v", err)
	}

	// 5 failed password attempts
	for i := 0; i < 5; i++ {
		_, _ = svc.SignIn(ctx, emailStr, "WrongPassword999!")
	}

	// 6th attempt should return 429 Too Many Requests (RateLimited)
	_, err = svc.SignIn(ctx, emailStr, "WrongPassword999!")
	if err == nil {
		t.Fatalf("expected rate limit error after 5 failed attempts, got nil")
	}

	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.StatusCode != 429 {
		t.Errorf("expected status 429, got %v", err)
	}
}
