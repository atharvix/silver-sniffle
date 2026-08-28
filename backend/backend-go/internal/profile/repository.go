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
	query := `
		INSERT INTO profiles (email, name, about, photo_url, social_links, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (email) DO UPDATE SET
			name = EXCLUDED.name,
			about = EXCLUDED.about,
			photo_url = EXCLUDED.photo_url,
			social_links = EXCLUDED.social_links,
			updated_at = EXCLUDED.updated_at;
	`
	now := time.Now()
	_, err := r.db.Pool.Exec(ctx, query, p.Email, p.Name, p.About, p.PhotoURL, p.SocialLinks, now)
	if err != nil {
		return fmt.Errorf("failed to upsert profile: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (*domain.Profile, error) {
	query := `
		SELECT email, name, about, photo_url, latitude, longitude, last_seen_at, social_links,
		       ai_summary, ai_summary_about, headline, headline_about, created_at, updated_at
		FROM profiles
		WHERE email = $1;
	`
	var p domain.Profile
	err := r.db.Pool.QueryRow(ctx, query, email).Scan(
		&p.Email,
		&p.Name,
		&p.About,
		&p.PhotoURL,
		&p.Latitude,
		&p.Longitude,
		&p.LastSeenAt,
			&p.SocialLinks,
		&p.AISummary,
		&p.AISummaryAbout,
		&p.Headline,
		&p.HeadlineAbout,
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
