package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type Repository interface {
	SaveOTP(ctx context.Context, email, otpHash string, expiresAt time.Time) error
	VerifyAndIssueToken(ctx context.Context, email, plainOTP string, tokenHash string, tokenExpiresAt time.Time, maxAttempts int) error
	GetEmailFromToken(ctx context.Context, tokenHash string) (string, error)
	IsEmailVerified(ctx context.Context, email string) (bool, error)
	ConsumeVerifiedEmail(ctx context.Context, email string) error
	CleanupExpired(ctx context.Context) error
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

func (r *PostgresRepository) SaveOTP(ctx context.Context, email, otpHash string, expiresAt time.Time) error {
	query := `
		INSERT INTO otp_codes (email, otp_hash, expiresAt, attempts)
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
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	if time.Now().After(expiresAt) {
		return false, nil
	}

	return true, nil
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
