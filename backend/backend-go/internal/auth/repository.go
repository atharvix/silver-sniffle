package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type Repository interface {
	CreatePasswordAccount(ctx context.Context, email, passwordHash string) error
	GetPasswordAccount(ctx context.Context, email string) (passwordHash string, verified bool, err error)
	MarkPasswordAccountVerified(ctx context.Context, email string) error
	IssueToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) error
	SaveOTP(ctx context.Context, email, otpHash string, expiresAt time.Time) error
	VerifyAndIssueToken(ctx context.Context, email, plainOTP string, tokenHash string, tokenExpiresAt time.Time, maxAttempts int) error
	GetEmailFromToken(ctx context.Context, tokenHash string) (string, error)
	IsEmailVerified(ctx context.Context, email string) (bool, error)
	ConsumeVerifiedEmail(ctx context.Context, email string) error
	CleanupExpired(ctx context.Context) error
	DeleteAccount(ctx context.Context, email string) error
	EnsureGoogleProfile(ctx context.Context, email, name string) error
	CheckEmail(ctx context.Context, email string) (exists bool, hasPassword bool, err error)
	IsLoginLocked(ctx context.Context, email string) (locked bool, lockedUntil time.Time, err error)
	IncrementFailedLogin(ctx context.Context, email string, maxAttempts int, lockFor time.Duration) error
	ResetFailedLogin(ctx context.Context, email string) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func HashString(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func (r *PostgresRepository) CreatePasswordAccount(ctx context.Context, email, passwordHash string) error {
	name := strings.Split(email, "@")[0]
	var verified bool
	err := r.db.Pool.QueryRow(ctx, `SELECT email_verified FROM profiles WHERE email = $1`, email).Scan(&verified)
	if err == nil && verified {
		return domain.ErrConflict
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO profiles (email, name, bio, photo_url, password_hash, email_verified, created_at, updated_at)
		VALUES ($1, $2, '', '', $3, FALSE, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			updated_at = NOW()
	`, email, name, passwordHash)
	return err
}

func (r *PostgresRepository) GetPasswordAccount(ctx context.Context, email string) (string, bool, error) {
	var passwordHash string
	var verified bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT password_hash, email_verified FROM profiles WHERE email = $1 AND password_hash IS NOT NULL
	`, email).Scan(&passwordHash, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, domain.ErrUnauthorized
	}
	return passwordHash, verified, err
}

func (r *PostgresRepository) MarkPasswordAccountVerified(ctx context.Context, email string) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE profiles SET email_verified = TRUE, updated_at = NOW() WHERE email = $1`, email)
	return err
}

func (r *PostgresRepository) IssueToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO verification_tokens (token_hash, email, expires_at, created_at)
		VALUES ($1, $2, $3, NOW())
	`, tokenHash, email, expiresAt)
	return err
}

func (r *PostgresRepository) SaveOTP(ctx context.Context, email, otpHash string, expiresAt time.Time) error {
	query := `
		INSERT INTO otp_codes (email, otp_hash, expires_at, attempts)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (email) DO UPDATE
		SET otp_hash = EXCLUDED.otp_hash,
		    expires_at = EXCLUDED.expires_at,
		    attempts = 0;
	`
	_, err := r.db.Pool.Exec(ctx, query, email, otpHash, expiresAt)
	return err
}

func (r *PostgresRepository) VerifyAndIssueToken(
	ctx context.Context,
	email, plainOTP string,
	tokenHash string,
	tokenExpiresAt time.Time,
	maxAttempts int,
) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var storedHash string
		var expiresAt time.Time
		var attempts int

		// Select row FOR UPDATE to prevent race conditions on concurrent verification attempts
		query := `
			SELECT otp_hash, expires_at, attempts
			FROM otp_codes
			WHERE email = $1
			FOR UPDATE;
		`
		err := tx.QueryRow(ctx, query, email).Scan(&storedHash, &expiresAt, &attempts)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidOTP
			}
			return fmt.Errorf("failed to query otp: %w", err)
		}

		if time.Now().After(expiresAt) {
			_, _ = tx.Exec(ctx, "DELETE FROM otp_codes WHERE email = $1", email)
			return domain.ErrInvalidOTP
		}

		attempts++
		if attempts > maxAttempts {
			_, _ = tx.Exec(ctx, "DELETE FROM otp_codes WHERE email = $1", email)
			return domain.ErrTooManyAttempts
		}

		expectedHash := HashString(plainOTP)
		if storedHash != expectedHash {
			_, _ = tx.Exec(ctx, "UPDATE otp_codes SET attempts = $1 WHERE email = $2", attempts, email)
			return domain.NewAppError(400, fmt.Sprintf("Incorrect OTP. %d attempts remaining.", maxAttempts-attempts), domain.ErrInvalidOTP)
		}

		// Success: Delete OTP code
		if _, err := tx.Exec(ctx, "DELETE FROM otp_codes WHERE email = $1", email); err != nil {
			return fmt.Errorf("failed to delete otp: %w", err)
		}

		// Delete any existing token for this email
		if _, err := tx.Exec(ctx, "DELETE FROM verification_tokens WHERE email = $1", email); err != nil {
			return fmt.Errorf("failed to remove old tokens: %w", err)
		}

		// Insert new verification token
		insertTokenQuery := `
			INSERT INTO verification_tokens (token_hash, email, expires_at, created_at)
			VALUES ($1, $2, $3, NOW());
		`
		if _, err := tx.Exec(ctx, insertTokenQuery, tokenHash, email, tokenExpiresAt); err != nil {
			return fmt.Errorf("failed to insert token: %w", err)
		}

		// Upsert verified_emails record for welcome email gate
		insertVerifiedEmailQuery := `
			INSERT INTO verified_emails (email, expires_at)
			VALUES ($1, $2)
			ON CONFLICT (email) DO UPDATE SET expires_at = EXCLUDED.expires_at;
		`
		if _, err := tx.Exec(ctx, insertVerifiedEmailQuery, email, tokenExpiresAt); err != nil {
			return fmt.Errorf("failed to insert verified email: %w", err)
		}

		return nil
	})
}

func (r *PostgresRepository) GetEmailFromToken(ctx context.Context, tokenHash string) (string, error) {
	var email string
	var expiresAt time.Time

	query := `
		SELECT email, expires_at
		FROM verification_tokens
		WHERE token_hash = $1;
	`
	err := r.db.Pool.QueryRow(ctx, query, tokenHash).Scan(&email, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrUnauthorized
		}
		return "", err
	}

	if time.Now().After(expiresAt) {
		// Clean up expired token
		_, _ = r.db.Pool.Exec(ctx, "DELETE FROM verification_tokens WHERE token_hash = $1", tokenHash)
		return "", domain.ErrTokenExpired
	}

	return email, nil
}

func (r *PostgresRepository) IsEmailVerified(ctx context.Context, email string) (bool, error) {
	var expiresAt time.Time
	query := `SELECT expires_at FROM verified_emails WHERE email = $1;`
	err := r.db.Pool.QueryRow(ctx, query, email).Scan(&expiresAt)
	if err == nil && time.Now().Before(expiresAt) {
		return true, nil
	}

	var profileVerified bool
	err = r.db.Pool.QueryRow(ctx, `SELECT email_verified FROM profiles WHERE email = $1;`, email).Scan(&profileVerified)
	if err == nil && profileVerified {
		return true, nil
	}

	return false, nil
}

func (r *PostgresRepository) ConsumeVerifiedEmail(ctx context.Context, email string) error {
	_, err := r.db.Pool.Exec(ctx, "DELETE FROM verified_emails WHERE email = $1", email)
	return err
}

func (r *PostgresRepository) CleanupExpired(ctx context.Context) error {
	now := time.Now()
	_, _ = r.db.Pool.Exec(ctx, "DELETE FROM otp_codes WHERE expires_at < $1", now)
	_, _ = r.db.Pool.Exec(ctx, "DELETE FROM verification_tokens WHERE expires_at < $1", now)
	_, _ = r.db.Pool.Exec(ctx, "DELETE FROM verified_emails WHERE expires_at < $1", now)
	return nil
}

func (r *PostgresRepository) DeleteAccount(ctx context.Context, email string) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin delete account transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Statements must be executed individually: PostgreSQL's extended protocol
	// (used by pgx whenever parameters are bound) rejects a prepared statement
	// that contains multiple commands.
	for _, query := range []string{
		`DELETE FROM verification_tokens WHERE email = $1;`,
		`DELETE FROM otp_codes WHERE email = $1;`,
		`DELETE FROM verified_emails WHERE email = $1;`,
		`DELETE FROM device_tokens WHERE email = $1;`,
		`DELETE FROM profiles WHERE email = $1;`,
	} {
		if _, err := tx.Exec(ctx, query, email); err != nil {
			return fmt.Errorf("failed to delete account data: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// IsLoginLocked reports whether the account is currently in a failed-login
// cooldown, along with the time that cooldown ends.
func (r *PostgresRepository) IsLoginLocked(ctx context.Context, email string) (bool, time.Time, error) {
	var lockedUntil *time.Time
	err := r.db.Pool.QueryRow(ctx, `SELECT locked_until FROM login_attempts WHERE email = $1;`, email).Scan(&lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to query login lock: %w", err)
	}
	if lockedUntil != nil && time.Now().Before(*lockedUntil) {
		return true, *lockedUntil, nil
	}
	return false, time.Time{}, nil
}

// IncrementFailedLogin records one failed sign-in and starts a cooldown once
// maxAttempts is reached. An already-expired cooldown starts a fresh window.
func (r *PostgresRepository) IncrementFailedLogin(ctx context.Context, email string, maxAttempts int, lockFor time.Duration) error {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var count int
		var lockedUntil *time.Time

		err := tx.QueryRow(ctx, `
			SELECT failed_count, locked_until
			FROM login_attempts
			WHERE email = $1
			FOR UPDATE;
		`, email).Scan(&count, &lockedUntil)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to query login attempts: %w", err)
		}

		// A cooldown that has already elapsed starts a fresh counting window.
		if lockedUntil != nil && !time.Now().Before(*lockedUntil) {
			count = 0
		}
		count++

		var newLock *time.Time
		if count >= maxAttempts {
			until := time.Now().Add(lockFor)
			newLock = &until
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO login_attempts (email, failed_count, locked_until, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (email) DO UPDATE SET
				failed_count = EXCLUDED.failed_count,
				locked_until = EXCLUDED.locked_until,
				updated_at = NOW();
		`, email, count, newLock)
		if err != nil {
			return fmt.Errorf("failed to record login attempt: %w", err)
		}
		return nil
	})
}

// ResetFailedLogin clears the failure counter after a successful sign-in.
func (r *PostgresRepository) ResetFailedLogin(ctx context.Context, email string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM login_attempts WHERE email = $1;`, email)
	return err
}

func (r *PostgresRepository) EnsureGoogleProfile(ctx context.Context, email, name string) error {
	if strings.TrimSpace(name) == "" {
		name = strings.Split(email, "@")[0]
	}
	query := `
		INSERT INTO profiles (email, name, bio, photo_url, email_verified, created_at, updated_at)
		VALUES ($1, $2, '', '', TRUE, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET
			email_verified = TRUE,
			name = CASE WHEN profiles.name = '' THEN EXCLUDED.name ELSE profiles.name END,
			updated_at = NOW();
	`
	_, err := r.db.Pool.Exec(ctx, query, email, name)
	return err
}

func (r *PostgresRepository) CheckEmail(ctx context.Context, email string) (bool, bool, error) {
	var passwordHash *string
	var emailVerified bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT password_hash, email_verified FROM profiles WHERE email = $1
	`, email).Scan(&passwordHash, &emailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	hasPassword := passwordHash != nil && *passwordHash != ""
	return true, hasPassword, nil
}
