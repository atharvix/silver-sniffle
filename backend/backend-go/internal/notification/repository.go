package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/security"
)

type Repository interface {
	SaveToken(ctx context.Context, email, token, platform string) error
	GetTokensByEmail(ctx context.Context, email string) ([]domain.DeviceToken, error)
	GetTokensByEmails(ctx context.Context, emails []string) ([]domain.DeviceToken, error)
	GetAllTokens(ctx context.Context) ([]domain.DeviceToken, error)
	GetRecentTokens(ctx context.Context, duration time.Duration) ([]domain.DeviceToken, error)
}

type PostgresRepository struct {
	db *database.DB
	c  *security.Crypto
}

func NewRepository(db *database.DB, c *security.Crypto) *PostgresRepository {
	return &PostgresRepository{db: db, c: c}
}

func (r *PostgresRepository) SaveToken(ctx context.Context, email, token, platform string) error {
	now := time.Now()
	query := `
		INSERT INTO device_tokens (email, token, platform, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email, token) DO UPDATE SET
			platform = EXCLUDED.platform,
			updated_at = EXCLUDED.updated_at;
	`
	_, err := r.db.Pool.Exec(ctx, query, r.c.EmailHash(email), token, platform, now)
	if err != nil {
		return fmt.Errorf("failed to save device token: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetTokensByEmail(ctx context.Context, email string) ([]domain.DeviceToken, error) {
	query := `
		SELECT COALESCE(email, ''), token, platform, updated_at
		FROM device_tokens
		WHERE email_hash = $1;
	`
	rows, err := r.db.Pool.Query(ctx, query, r.c.EmailHash(email))
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []domain.DeviceToken
	for rows.Next() {
		var t domain.DeviceToken
		if err := rows.Scan(&t.Email, &t.Token, &t.Platform, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (r *PostgresRepository) GetAllTokens(ctx context.Context) ([]domain.DeviceToken, error) {
	query := `
		SELECT COALESCE(email, ''), token, platform, updated_at
		FROM device_tokens;
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all tokens: %w", err)
	}
	defer rows.Close()

	var tokens []domain.DeviceToken
	for rows.Next() {
		var t domain.DeviceToken
		if err := rows.Scan(&t.Email, &t.Token, &t.Platform, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (r *PostgresRepository) GetTokensByEmails(ctx context.Context, emails []string) ([]domain.DeviceToken, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	query := `
		SELECT COALESCE(email, ''), token, platform, updated_at
		FROM device_tokens
		WHERE email_hash = ANY($1);
	`
	lowerEmails := make([]string, len(emails))
	for i, e := range emails {
		lowerEmails[i] = r.c.EmailHash(e)
	}
	rows, err := r.db.Pool.Query(ctx, query, lowerEmails)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens by emails: %w", err)
	}
	defer rows.Close()

	var tokens []domain.DeviceToken
	for rows.Next() {
		var t domain.DeviceToken
		if err := rows.Scan(&t.Email, &t.Token, &t.Platform, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (r *PostgresRepository) GetRecentTokens(ctx context.Context, duration time.Duration) ([]domain.DeviceToken, error) {
	since := time.Now().Add(-duration)
	query := `
		SELECT COALESCE(email, ''), token, platform, updated_at
		FROM device_tokens
		WHERE updated_at >= $1;
	`
	rows, err := r.db.Pool.Query(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent tokens: %w", err)
	}
	defer rows.Close()

	var tokens []domain.DeviceToken
	for rows.Next() {
		var t domain.DeviceToken
		if err := rows.Scan(&t.Email, &t.Token, &t.Platform, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}
