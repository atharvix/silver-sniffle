package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type Repository interface {
	Upsert(ctx context.Context, p *domain.Profile) error
	GetByEmail(ctx context.Context, email string) (*domain.Profile, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Upsert(ctx context.Context, p *domain.Profile) error {
	now := time.Now()
	// Atomically upsert profile AND ensure password_account row exists (using industry-standard OAuth hash if absent)
	txErr := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		profileQuery := `
			INSERT INTO profiles (email, name, bio, photo_url, latitude, longitude, last_seen_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::double precision, $6::double precision, CASE WHEN $5::double precision IS NOT NULL AND $6::double precision IS NOT NULL THEN $7::timestamptz ELSE NULL END, $7::timestamptz)
			ON CONFLICT (email) DO UPDATE SET
				name = EXCLUDED.name,
				bio = EXCLUDED.bio,
				photo_url = EXCLUDED.photo_url,
				latitude = COALESCE(EXCLUDED.latitude, profiles.latitude),
				longitude = COALESCE(EXCLUDED.longitude, profiles.longitude),
				last_seen_at = CASE WHEN EXCLUDED.latitude IS NOT NULL THEN $7::timestamptz ELSE profiles.last_seen_at END,
				updated_at = EXCLUDED.updated_at;
		`
		if _, err := tx.Exec(ctx, profileQuery, p.Email, p.Name, p.Bio, p.PhotoURL, p.Latitude, p.Longitude, now); err != nil {
			return fmt.Errorf("failed to upsert profile: %w", err)
		}

		accountQuery := `
			INSERT INTO password_accounts (email, password_hash, email_verified, created_at, updated_at)
			VALUES ($1, '$google_oauth$' || md5(random()::text), TRUE, $2, $2)
			ON CONFLICT (email) DO UPDATE SET email_verified = TRUE;
		`
		if _, err := tx.Exec(ctx, accountQuery, p.Email, now); err != nil {
			return fmt.Errorf("failed to ensure account: %w", err)
		}

		return nil
	})

	if txErr != nil {
		return fmt.Errorf("failed to save profile: %w", txErr)
	}
	return nil
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (*domain.Profile, error) {
	query := `
		SELECT email, name, bio, photo_url, latitude, longitude, last_seen_at,
		       ai_summary, headline, created_at, updated_at
		FROM profiles
		WHERE LOWER(email) = LOWER($1);
	`
	var p domain.Profile
	err := r.db.Pool.QueryRow(ctx, query, email).Scan(
		&p.Email,
		&p.Name,
		&p.Bio,
		&p.PhotoURL,
		&p.Latitude,
		&p.Longitude,
		&p.LastSeenAt,
		&p.AISummary,
		&p.Headline,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProfileNotFound
		}
		return nil, fmt.Errorf("failed to query profile: %w", err)
	}

	return &p, nil
}
