package discovery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type NearbyRecord struct {
	Name                string
	PhotoURL            string
	About               string
	Headline            *string
	AISummary           *string
	DistanceMeters      float64
}

type Repository interface {
	GetCallerProfile(ctx context.Context, email string) (*domain.Profile, error)
	FindNearbyProfiles(ctx context.Context, email string, lat, lon, radiusMeters float64, presenceCutoff time.Time, limit int) ([]NearbyRecord, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) GetCallerProfile(ctx context.Context, email string) (*domain.Profile, error) {
	query := `
		SELECT email, name, about, photo_url, latitude, longitude, last_seen_at, created_at, updated_at
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
	const earthRadiusM = 6371000.0

	// Calculate bounding box in degrees
	latDelta := (radiusMeters / earthRadiusM) * (180.0 / math.Pi)
	cosLat := math.Cos(lat * math.Pi / 180.0)
	if cosLat < 0.0001 {
		cosLat = 0.0001
	}
	lonDelta := latDelta / cosLat

	minLat := lat - latDelta
	maxLat := lat + latDelta
	minLon := lon - lonDelta
	maxLon := lon + lonDelta

	query := `
		SELECT name, photo_url, about, headline, ai_summary,
		       (6371000.0 * acos(
		           LEAST(1.0, GREATEST(-1.0,
		               cos(radians($1)) * cos(radians(latitude)) * cos(radians(longitude) - radians($2)) +
		               sin(radians($1)) * sin(radians(latitude))
		           ))
		       )) AS distance_meters
		FROM profiles
		WHERE email != $3
		  AND last_seen_at >= $4
		  AND latitude BETWEEN $5 AND $6
		  AND longitude BETWEEN $7 AND $8
		  AND (6371000.0 * acos(
		           LEAST(1.0, GREATEST(-1.0,
		               cos(radians($1)) * cos(radians(latitude)) * cos(radians(longitude) - radians($2)) +
		               sin(radians($1)) * sin(radians(latitude))
		           ))
		       )) <= $9
		ORDER BY distance_meters ASC
		LIMIT $10;
	`

	rows, err := r.db.Pool.Query(ctx, query, lat, lon, email, presenceCutoff, minLat, maxLat, minLon, maxLon, radiusMeters, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query nearby profiles: %w", err)
	}
	defer rows.Close()

	var results []NearbyRecord
	for rows.Next() {
		var rec NearbyRecord
		if err := rows.Scan(&rec.Name, &rec.PhotoURL, &rec.About, &rec.Headline, &rec.AISummary, &rec.DistanceMeters); err != nil {
			return nil, fmt.Errorf("failed to scan nearby profile: %w", err)
		}
		results = append(results, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("nearby rows error: %w", err)
	}

	if results == nil {
		results = []NearbyRecord{}
	}

	return results, nil
}
