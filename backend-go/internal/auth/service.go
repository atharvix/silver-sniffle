package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/email"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type Service struct {
	repo         Repository
	emailService email.Service
	cfg          *config.Config
	logger       *slog.Logger
	metrics      *observability.Metrics
}

func NewService(
	repo Repository,
	emailService email.Service,
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *Service {
	return &Service{
		repo:         repo,
		emailService: emailService,
		cfg:          cfg,
		logger:       logger,
		metrics:      metrics,
	}
}

// GenerateSecureOTP generates a cryptographically random numeric OTP
func GenerateSecureOTP() (string, error) {
	// Standard 4-digit OTP for 100% frontend compatibility (1000 - 9999)
	n, err := rand.Int(rand.Reader, big.NewInt(9000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d", n.Int64()+1000), nil
}

// GenerateSecureToken generates a 32-byte cryptographically secure hex token
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func ValidateEmail(emailStr string) (string, error) {
	emailStr = strings.TrimSpace(strings.ToLower(emailStr))
	if emailStr == "" {
		return "", domain.NewAppError(400, "Please enter a valid email address.", domain.ErrBadRequest)
	}

	addr, err := mail.ParseAddress(emailStr)
	if err != nil || addr.Address != emailStr || !strings.Contains(emailStr, "@") || !strings.Contains(emailStr, ".") {
		return "", domain.NewAppError(400, "Please enter a valid email address.", domain.ErrBadRequest)
	}

	return emailStr, nil
}

func (s *Service) SendOTP(ctx context.Context, emailStr string) (*domain.SendOTPResponse, error) {
	cleanEmail, err := ValidateEmail(emailStr)
	if err != nil {
		return nil, err
	}

	otp, err := GenerateSecureOTP()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure OTP: %w", err)
	}

	otpHash := HashString(otp)
	expiresAt := time.Now().Add(s.cfg.OtpTTL)

	if err := s.repo.SaveOTP(ctx, cleanEmail, otpHash, expiresAt); err != nil {
		return nil, fmt.Errorf("failed to save OTP: %w", err)
	}

	// Opportunistic cleanup of expired rows
	go func() {
		_ = s.repo.CleanupExpired(context.Background())
	}()

	var devOTP *string
	if s.emailService.IsConfigured() {
		if err := s.emailService.SendOTP(ctx, cleanEmail, otp); err != nil {
			s.logger.ErrorContext(ctx, "failed to send OTP email via provider", slog.String("email", cleanEmail), slog.String("error", err.Error()))
			if s.metrics != nil {
				s.metrics.OTPSentTotal.WithLabelValues("failed").Inc()
			}
			return nil, domain.NewAppError(502, "Failed to send verification email. Please try again.", domain.ErrServiceUnavailable)
		}
		if s.metrics != nil {
			s.metrics.OTPSentTotal.WithLabelValues("success").Inc()
		}
	} else if !s.cfg.IsProduction() {
		// Development mode fallback: return OTP in response for demo / test
		devOTP = &otp
		s.logger.WarnContext(ctx, "email provider not configured; exposing devOtp in response (dev only)", slog.String("email", cleanEmail))
		if s.metrics != nil {
			s.metrics.OTPSentTotal.WithLabelValues("dev_mode").Inc()
		}
	} else {
		// Production without email provider -> fail closed
		s.logger.ErrorContext(ctx, "email delivery is not configured in production")
		return nil, domain.NewAppError(503, "Email delivery is not configured. Please contact support.", domain.ErrServiceUnavailable)
	}

	return &domain.SendOTPResponse{
		Success: true,
		Message: fmt.Sprintf("Verification code sent to %s", cleanEmail),
		DevOTP:  devOTP,
	}, nil
}

func (s *Service) VerifyOTP(ctx context.Context, emailStr, plainOTP string) (*domain.VerifyOTPResponse, error) {
	cleanEmail, err := ValidateEmail(emailStr)
	if err != nil {
		return nil, err
	}

	plainOTP = strings.TrimSpace(plainOTP)
	if plainOTP == "" {
		return nil, domain.NewAppError(400, "Enter all 4 digits", domain.ErrBadRequest)
	}

	rawToken, err := GenerateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	tokenHash := HashString(rawToken)
	tokenExpiresAt := time.Now().Add(s.cfg.TokenTTL)

	err = s.repo.VerifyAndIssueToken(ctx, cleanEmail, plainOTP, tokenHash, tokenExpiresAt, s.cfg.MaxOtpAttempts)
	if err != nil {
		if s.metrics != nil {
			s.metrics.OTPVerifiedTotal.WithLabelValues("failed").Inc()
		}
		return nil, err
	}

	if s.metrics != nil {
		s.metrics.OTPVerifiedTotal.WithLabelValues("success").Inc()
	}

	s.logger.InfoContext(ctx, "OTP verified successfully", slog.String("email", cleanEmail))

	return &domain.VerifyOTPResponse{
		Success:           true,
		Message:           "Email verified successfully!",
		VerificationToken: rawToken,
	}, nil
}

func (s *Service) SendWelcome(ctx context.Context, emailStr, name, about string) (*domain.SendWelcomeResponse, error) {
	cleanEmail, err := ValidateEmail(emailStr)
	if err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.NewAppError(400, "Name is required.", domain.ErrBadRequest)
	}

	// Verify proof of OTP verification
	isVerified, err := s.repo.IsEmailVerified(ctx, cleanEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to check verification status: %w", err)
	}
	if !isVerified {
		return nil, domain.NewAppError(403, "Email not verified. Please complete OTP verification first.", domain.ErrEmailNotVerified)
	}

	// Consume verification record (one welcome email per flow)
	_ = s.repo.ConsumeVerifiedEmail(ctx, cleanEmail)

	if !s.emailService.IsConfigured() {
		if !s.cfg.IsProduction() {
			s.logger.WarnContext(ctx, "email provider not configured; skipping welcome email (dev mode)", slog.String("email", cleanEmail))
			return &domain.SendWelcomeResponse{Success: true, Message: "Welcome email skipped (dev mode)."}, nil
		}
		return &domain.SendWelcomeResponse{Success: true, Message: "Profile saved. Welcome email could not be sent."}, nil
	}

	if err := s.emailService.SendWelcome(ctx, cleanEmail, name, about); err != nil {
		s.logger.ErrorContext(ctx, "failed to send welcome email", slog.String("email", cleanEmail), slog.String("error", err.Error()))
		return &domain.SendWelcomeResponse{Success: true, Message: "Profile saved. Welcome email could not be sent."}, nil
	}

	return &domain.SendWelcomeResponse{
		Success: true,
		Message: "Welcome email sent.",
	}, nil
}

func (s *Service) GetEmailFromToken(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", domain.ErrUnauthorized
	}

	tokenHash := HashString(token)
	return s.repo.GetEmailFromToken(ctx, tokenHash)
}
