package discovery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type NearbyRecord struct {
	Email          string
	Name           string
	PhotoURL       string
	Bio            string
	Headline       *string
	AISummary      *string
	DistanceMeters float64
}

type Repository interface {
	GetCallerProfile(ctx context.Context, email string) (*domain.Profile, error)
	UpdateCallerLocation(ctx context.Context, email string, lat, lon float64) error
	FindNearbyProfiles(ctx context.Context, email string, lat, lon, radiusMeters float64, presenceCutoff time.Time, limit int) ([]NearbyRecord, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) UpdateCallerLocation(ctx context.Context, email string, lat, lon float64) error {
	now := time.Now()
	query := `
		UPDATE profiles 
		SET latitude = $1, 
		    longitude = $2, 
		    last_seen_at = $3, 
		    updated_at = $3 
		WHERE LOWER(email) = LOWER($4);
	`
	_, err := r.db.Pool.Exec(ctx, query, lat, lon, now, email)
	return err
}

func (r *PostgresRepository) GetCallerProfile(ctx context.Context, email string) (*domain.Profile, error) {
	query := `
		SELECT email, name, bio, photo_url, latitude, longitude, last_seen_at, created_at, updated_at
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
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProfileNotFound
		}
		return nil, fmt.Errorf("failed to query caller profile: %w", err)
	}

	return &p, nil
}

func (r *PostgresRepository) FindNearbyProfiles(
	ctx context.Context,
	email string,
	lat, lon, radiusMeters float64,
	presenceCutoff time.Time,
	limit int,
) ([]NearbyRecord, error) {
	if limit <= 0 {
		limit = 30
	}
	if radiusMeters <= 0 {
		radiusMeters = 30.0
	}

	// 1. Primary Query: Strictly within 30 meters
	query30m := `
		SELECT email, name, photo_url, bio, headline, ai_summary,
		       (6371000.0 * acos(
		           LEAST(1.0, GREATEST(-1.0,
		               cos(radians($1)) * cos(radians(latitude)) * cos(radians(longitude) - radians($2)) +
		               sin(radians($1)) * sin(radians(latitude))
		           ))
		       )) AS distance_meters
		FROM profiles
		WHERE LOWER(email) != LOWER($3)
		  AND latitude IS NOT NULL 
		  AND longitude IS NOT NULL
		  AND (6371000.0 * acos(
		         LEAST(1.0, GREATEST(-1.0,
		             cos(radians($1)) * cos(radians(latitude)) * cos(radians(longitude) - radians($2)) +
		             sin(radians($1)) * sin(radians(latitude))
		         ))
		     )) <= $4
		ORDER BY distance_meters ASC
		LIMIT $5;
	`

	rows, err := r.db.Pool.Query(ctx, query30m, lat, lon, email, radiusMeters, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query 30m profiles: %w", err)
	}
	defer rows.Close()

	var results []NearbyRecord
	fetchedEmails := make(map[string]bool)
	fetchedEmails[strings.ToLower(email)] = true

	for rows.Next() {
		var rec NearbyRecord
		if err := rows.Scan(&rec.Email, &rec.Name, &rec.PhotoURL, &rec.Bio, &rec.Headline, &rec.AISummary, &rec.DistanceMeters); err != nil {
			return nil, fmt.Errorf("failed to scan profile: %w", err)
		}
		results = append(results, rec)
		fetchedEmails[strings.ToLower(rec.Email)] = true
	}

	if results == nil {
		results = []NearbyRecord{}
	}

	return results, nil
}
