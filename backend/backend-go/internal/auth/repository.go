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
	"github.com/atharvix/kinjo-backend/internal/security"
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
}

type PostgresRepository struct {
	db *database.DB
	c  *security.Crypto
}

func NewRepository(db *database.DB, c *security.Crypto) *PostgresRepository {
	return &PostgresRepository{db: db, c: c}
}

func HashString(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func (r *PostgresRepository) CreatePasswordAccount(ctx context.Context, email, passwordHash string) error {
	emailHash := r.c.EmailHash(email)
	name := strings.Split(email, "@")[0]
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO profiles (email_hash, email_enc, name_enc, bio_enc, password_hash, email_verified)
		VALUES ($1, $2, $3, '', $4, FALSE)
	`, emailHash, mustOrErr(r.c.EncryptEmail(email)), mustOrErr(r.c.Encrypt(name)), passwordHash)
	return err
}

func (r *PostgresRepository) GetPasswordAccount(ctx context.Context, email string) (string, bool, error) {
	var passwordHash string
	var verified bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT password_hash, email_verified FROM profiles WHERE email_hash = $1 AND password_hash IS NOT NULL
	`, r.c.EmailHash(email)).Scan(&passwordHash, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, domain.ErrUnauthorized
	}
	return passwordHash, verified, err
}

func (r *PostgresRepository) MarkPasswordAccountVerified(ctx context.Context, email string) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE profiles SET email_verified = TRUE WHERE email_hash = $1`, r.c.EmailHash(email))
	return err
}

func (r *PostgresRepository) IssueToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) error {
	emailEnc, err := r.c.EncryptEmail(email)
	if err != nil {
		return fmt.Errorf("failed to encrypt email for token: %w", err)
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO verification_tokens (token_hash, email_hash, email_enc, expires_at, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, tokenHash, r.c.EmailHash(email), emailEnc, expiresAt)
	return err
}

func (r *PostgresRepository) SaveOTP(ctx context.Context, email, otpHash string, expiresAt time.Time) error {
	query := `
		INSERT INTO otp_codes (email_hash, otp_hash, expires_at, attempts)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (email_hash) DO UPDATE
		SET otp_hash = EXCLUDED.otp_hash,
		    expires_at = EXCLUDED.expires_at,
		    attempts = 0;
	`
	_, err := r.db.Pool.Exec(ctx, query, r.c.EmailHash(email), otpHash, expiresAt)
	return err
}

func (r *PostgresRepository) VerifyAndIssueToken(
	ctx context.Context,
	email, plainOTP string,
	tokenHash string,
	tokenExpiresAt time.Time,
	maxAttempts int,
) error {
	emailHash := r.c.EmailHash(email)
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var storedHash string
		var expiresAt time.Time
		var attempts int

		// Select row FOR UPDATE to prevent race conditions on concurrent verification attempts
		query := `
			SELECT otp_hash, expires_at, attempts
			FROM otp_codes
			WHERE email_hash = $1
			FOR UPDATE;
		`
		err := tx.QueryRow(ctx, query, emailHash).Scan(&storedHash, &expiresAt, &attempts)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidOTP
			}
			return fmt.Errorf("failed to query otp: %w", err)
		}

		if time.Now().After(expiresAt) {
			_, _ = tx.Exec(ctx, "DELETE FROM otp_codes WHERE email_hash = $1", emailHash)
			return domain.ErrInvalidOTP
		}

		attempts++
		if attempts > maxAttempts {
			_, _ = tx.Exec(ctx, "DELETE FROM otp_codes WHERE email_hash = $1", emailHash)
			return domain.ErrTooManyAttempts
		}

		expectedHash := HashString(plainOTP)
		if storedHash != expectedHash {
			_, _ = tx.Exec(ctx, "UPDATE otp_codes SET attempts = $1 WHERE email_hash = $2", attempts, emailHash)
			return domain.NewAppError(400, fmt.Sprintf("Incorrect OTP. %d attempts remaining.", maxAttempts-attempts), domain.ErrInvalidOTP)
		}

		// Success: Delete OTP code
		if _, err := tx.Exec(ctx, "DELETE FROM otp_codes WHERE email_hash = $1", emailHash); err != nil {
			return fmt.Errorf("failed to delete otp: %w", err)
		}

		// Delete any existing token for this email
		if _, err := tx.Exec(ctx, "DELETE FROM verification_tokens WHERE email_hash = $1", emailHash); err != nil {
			return fmt.Errorf("failed to remove old tokens: %w", err)
		}

		// Insert new verification token
		emailEnc, err := r.c.EncryptEmail(email)
		if err != nil {
			return fmt.Errorf("failed to encrypt email for token: %w", err)
		}
		insertTokenQuery := `
			INSERT INTO verification_tokens (token_hash, email_hash, email_enc, expires_at, created_at)
			VALUES ($1, $2, $3, $4, NOW());
		`
		if _, err := tx.Exec(ctx, insertTokenQuery, tokenHash, emailHash, emailEnc, tokenExpiresAt); err != nil {
			return fmt.Errorf("failed to insert token: %w", err)
		}

		// Upsert verified_emails record for welcome email gate
		insertVerifiedEmailQuery := `
			INSERT INTO verified_emails (email_hash, expires_at)
			VALUES ($1, $2)
			ON CONFLICT (email_hash) DO UPDATE SET expires_at = EXCLUDED.expires_at;
		`
		if _, err := tx.Exec(ctx, insertVerifiedEmailQuery, emailHash, tokenExpiresAt); err != nil {
			return fmt.Errorf("failed to insert verified email: %w", err)
		}

		return nil
	})
}

func (r *PostgresRepository) GetEmailFromToken(ctx context.Context, tokenHash string) (string, error) {
	var emailHash string
	var emailEnc *string
	var expiresAt time.Time

	query := `
		SELECT email_hash, email_enc, expires_at
		FROM verification_tokens
		WHERE token_hash = $1;
	`
	err := r.db.Pool.QueryRow(ctx, query, tokenHash).Scan(&emailHash, &emailEnc, &expiresAt)
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

	if emailEnc != nil && *emailEnc != "" {
		return r.c.DecryptEmail(*emailEnc)
	}

	return r.lookupEmailByHash(ctx, emailHash)
}

// lookupEmailByHash resolves an email_hash back to the plaintext email by
// fetching and decrypting the stored email_enc from profiles.
func (r *PostgresRepository) lookupEmailByHash(ctx context.Context, emailHash string) (string, error) {
	var emailEnc string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT email_enc FROM profiles WHERE email_hash = $1 LIMIT 1;
	`, emailHash).Scan(&emailEnc)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrUnauthorized
		}
		return "", fmt.Errorf("failed to resolve email hash: %w", err)
	}
	email, err := r.c.DecryptEmail(emailEnc)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt email: %w", err)
	}
	return email, nil
}

func (r *PostgresRepository) IsEmailVerified(ctx context.Context, email string) (bool, error) {
	emailHash := r.c.EmailHash(email)
	var expiresAt time.Time
	query := `SELECT expires_at FROM verified_emails WHERE email_hash = $1;`
	err := r.db.Pool.QueryRow(ctx, query, emailHash).Scan(&expiresAt)
	if err == nil && time.Now().Before(expiresAt) {
		return true, nil
	}

	var profileVerified bool
	err = r.db.Pool.QueryRow(ctx, `SELECT email_verified FROM profiles WHERE email_hash = $1;`, emailHash).Scan(&profileVerified)
	if err == nil && profileVerified {
		return true, nil
	}

	return false, nil
}

func (r *PostgresRepository) ConsumeVerifiedEmail(ctx context.Context, email string) error {
	_, err := r.db.Pool.Exec(ctx, "DELETE FROM verified_emails WHERE email_hash = $1", r.c.EmailHash(email))
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
	emailHash := r.c.EmailHash(email)
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM profiles WHERE email_hash = $1;
		DELETE FROM verification_tokens WHERE email_hash = $1;
		DELETE FROM otp_codes WHERE email_hash = $1;
		DELETE FROM verified_emails WHERE email_hash = $1;
		DELETE FROM device_tokens WHERE email_hash = $1;
	`, emailHash)
	return err
}

func (r *PostgresRepository) EnsureGoogleProfile(ctx context.Context, email, name string) error {
	if strings.TrimSpace(name) == "" {
		name = strings.Split(email, "@")[0]
	}
	emailHash := r.c.EmailHash(email)
	emailEnc, err := r.c.EncryptEmail(email)
	if err != nil {
		return fmt.Errorf("failed to encrypt email: %w", err)
	}
	nameEnc, err := r.c.Encrypt(name)
	if err != nil {
		return fmt.Errorf("failed to encrypt name: %w", err)
	}
	query := `
		INSERT INTO profiles (email_hash, email_enc, name_enc, bio_enc, photo_url, email_verified, created_at, updated_at)
		VALUES ($1, $2, $3, '', '', TRUE, NOW(), NOW())
		ON CONFLICT (email_hash) DO UPDATE SET
			email_verified = TRUE,
			name_enc = CASE WHEN profiles.name_enc = '' THEN EXCLUDED.name_enc ELSE profiles.name_enc END,
			updated_at = NOW();
	`
	_, err = r.db.Pool.Exec(ctx, query, emailHash, emailEnc, nameEnc)
	return err
}

func mustOrErr(v string, err error) string {
	if err != nil {
		panic(err)
	}
	return v
}
