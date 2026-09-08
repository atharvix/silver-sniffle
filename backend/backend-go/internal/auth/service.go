package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/email"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type GoogleTokenClaims struct {
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"`
	Audience      string          `json:"aud"`
	Issuer        string          `json:"iss"`
	Expiry        json.RawMessage `json:"exp"`
	Name          string          `json:"name"`
	Picture       string          `json:"picture"`
}

func (claims GoogleTokenClaims) IsEmailVerified() bool {
	var verified bool
	if json.Unmarshal(claims.EmailVerified, &verified) == nil {
		return verified
	}

	var verifiedString string
	return json.Unmarshal(claims.EmailVerified, &verifiedString) == nil && strings.EqualFold(verifiedString, "true")
}

func (claims GoogleTokenClaims) ExpiryUnix() (int64, error) {
	var expiry int64
	if json.Unmarshal(claims.Expiry, &expiry) == nil {
		return expiry, nil
	}

	var expiryString string
	if err := json.Unmarshal(claims.Expiry, &expiryString); err != nil {
		return 0, err
	}
	return strconv.ParseInt(expiryString, 10, 64)
}

var GoogleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

type failedAttemptInfo struct {
	count    int
	lockedUntil time.Time
}

type Service struct {
	repo           Repository
	emailService   email.Service
	cfg            *config.Config
	logger         *slog.Logger
	metrics        *observability.Metrics
	failedAttempts map[string]failedAttemptInfo
	attemptMu      sync.Mutex
}

func NewService(
	repo Repository,
	emailService email.Service,
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *Service {
	return &Service{
		repo:           repo,
		emailService:   emailService,
		cfg:            cfg,
		logger:         logger,
		metrics:        metrics,
		failedAttempts: make(map[string]failedAttemptInfo),
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

func validatePassword(password string) error {
	if len(password) < 8 {
		return domain.NewAppError(400, "Password must be at least 8 characters.", domain.ErrBadRequest)
	}
	return nil
}

func VerifyGoogleToken(ctx context.Context, idToken string, expectedAudience string) (*GoogleTokenClaims, error) {
	if strings.TrimSpace(idToken) == "" {
		return nil, domain.NewAppError(400, "Google token is required.", domain.ErrBadRequest)
	}
	if strings.TrimSpace(expectedAudience) == "" {
		return nil, domain.NewAppError(400, "Google client ID is not configured.", domain.ErrBadRequest)
	}

	endpoint := GoogleTokenInfoURL + "?id_token=" + strings.TrimSpace(idToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build Google verification request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Google token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, domain.NewAppError(401, "Google authentication failed.", domain.ErrUnauthorized)
	}

	var claims GoogleTokenClaims
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, fmt.Errorf("failed to decode Google token response: %w", err)
	}
	if strings.TrimSpace(claims.Email) == "" {
		return nil, domain.NewAppError(401, "Google account email is missing.", domain.ErrUnauthorized)
	}
	if !claims.IsEmailVerified() {
		return nil, domain.NewAppError(401, "Google account email is not verified.", domain.ErrUnauthorized)
	}
	if claims.Audience != expectedAudience {
		return nil, domain.NewAppError(401, "Google token audience mismatch.", domain.ErrUnauthorized)
	}
	expiry, err := claims.ExpiryUnix()
	if err != nil {
		return nil, fmt.Errorf("failed to parse Google token expiry: %w", err)
	}
	if expiry > 0 && time.Now().Unix() >= expiry {
		return nil, domain.NewAppError(401, "Google token is expired.", domain.ErrUnauthorized)
	}
	return &claims, nil
}

func (s *Service) GoogleSignIn(ctx context.Context, idToken string) (*domain.GoogleSignInResponse, error) {
	if s.cfg == nil || strings.TrimSpace(s.cfg.GoogleClientID) == "" {
		return nil, domain.NewAppError(503, "Google sign-in is not configured on the server.", domain.ErrServiceUnavailable)
	}

	claims, err := VerifyGoogleToken(ctx, idToken, s.cfg.GoogleClientID)
	if err != nil {
		return nil, err
	}

	cleanEmail, err := ValidateEmail(claims.Email)
	if err != nil {
		return nil, err
	}

	rawToken, err := GenerateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}
	if err := s.repo.IssueToken(ctx, cleanEmail, HashString(rawToken), time.Now().Add(s.cfg.TokenTTL)); err != nil {
		return nil, fmt.Errorf("failed to issue Google session token: %w", err)
	}

	// Mark email as verified for Google-authenticated users.
	// Google already verified the email via its own OAuth flow (IsEmailVerified() check above).
	if err := s.repo.MarkPasswordAccountVerified(ctx, cleanEmail); err != nil {
		s.logger.WarnContext(ctx, "failed to mark Google user email_verified", slog.String("email", cleanEmail), slog.String("error", err.Error()))
	}

	return &domain.GoogleSignInResponse{
		Success:           true,
		Message:           "Google sign-in successful.",
		VerificationToken: rawToken,
		Email:             cleanEmail,
	}, nil
}

func (s *Service) SignUp(ctx context.Context, emailStr, password string) (*domain.SendOTPResponse, error) {
	cleanEmail, err := ValidateEmail(emailStr)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	if err := s.repo.CreatePasswordAccount(ctx, cleanEmail, string(passwordHash)); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, domain.NewAppError(409, "An account with this email already exists.", domain.ErrConflict)
		}
		return nil, fmt.Errorf("failed to create account: %w", err)
	}
	return s.SendOTP(ctx, cleanEmail)
}

func (s *Service) SignIn(ctx context.Context, emailStr, password string) (*domain.AuthResponse, error) {
	cleanEmail, err := ValidateEmail(emailStr)
	if err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	// Account Lockout Check
	s.attemptMu.Lock()
	if info, exists := s.failedAttempts[cleanEmail]; exists {
		if time.Now().Before(info.lockedUntil) {
			s.attemptMu.Unlock()
			return nil, domain.NewAppError(429, "Too many failed sign-in attempts. Account temporarily locked for 15 minutes.", domain.ErrRateLimited)
		}
	}
	s.attemptMu.Unlock()

	passwordHash, verified, err := s.repo.GetPasswordAccount(ctx, cleanEmail)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		s.attemptMu.Lock()
		info := s.failedAttempts[cleanEmail]
		info.count++
		if info.count >= 5 {
			info.lockedUntil = time.Now().Add(15 * time.Minute)
			s.logger.WarnContext(ctx, "account temporarily locked due to repeated failed password attempts", slog.String("email", cleanEmail))
		}
		s.failedAttempts[cleanEmail] = info
		s.attemptMu.Unlock()

		return nil, domain.NewAppError(401, "Email or password is incorrect.", domain.ErrUnauthorized)
	}

	// Reset failed attempts on successful sign-in
	s.attemptMu.Lock()
	delete(s.failedAttempts, cleanEmail)
	s.attemptMu.Unlock()

	if !verified {
		return nil, domain.NewAppError(403, "Verify your email before signing in.", domain.ErrEmailNotVerified)
	}

	rawToken, err := GenerateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	if err := s.repo.IssueToken(ctx, cleanEmail, HashString(rawToken), time.Now().Add(s.cfg.TokenTTL)); err != nil {
		return nil, fmt.Errorf("failed to issue session: %w", err)
	}
	return &domain.AuthResponse{Success: true, Message: "Signed in successfully.", VerificationToken: rawToken, Email: cleanEmail}, nil
}

func (s *Service) VerifyOTP(ctx context.Context, emailStr, plainOTP string) (*domain.VerifyOTPResponse, error) {
	cleanEmail, err := ValidateEmail(emailStr)
	if err != nil {
		return nil, err
	}

	plainOTP = strings.TrimSpace(plainOTP)
	if len(plainOTP) != 4 || strings.IndexFunc(plainOTP, func(r rune) bool {
		return r < '0' || r > '9'
	}) != -1 {
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
	if err := s.repo.MarkPasswordAccountVerified(ctx, cleanEmail); err != nil {
		return nil, fmt.Errorf("failed to mark email verified: %w", err)
	}
	if _, _, accountErr := s.repo.GetPasswordAccount(ctx, cleanEmail); accountErr == nil {
		name := strings.Split(cleanEmail, "@")[0]
		if _, welcomeErr := s.SendWelcome(ctx, cleanEmail, name, ""); welcomeErr != nil {
			s.logger.WarnContext(ctx, "email verified but welcome email failed", slog.String("email", cleanEmail), slog.String("error", welcomeErr.Error()))
		}
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

func (s *Service) DeleteAccount(ctx context.Context, email string) error {
	cleanEmail, err := ValidateEmail(email)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteAccount(ctx, cleanEmail); err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}
	return nil
}

func (s *Service) GetEmailFromToken(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", domain.ErrUnauthorized
	}

	tokenHash := HashString(token)
	return s.repo.GetEmailFromToken(ctx, tokenHash)
}
