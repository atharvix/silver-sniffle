package discovery

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

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
	FindNearbyProfiles(ctx context.Context, email string, lat, lon, radiusMeters float64, limit int) ([]NearbyRecord, error)
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) GetCallerProfile(ctx context.Context, email string) (*domain.Profile, error) {
	query := `
		SELECT email, name, bio, photo_url, latitude, longitude, last_seen_at, face_verified_at, created_at, updated_at
		FROM profiles
		WHERE email = $1;
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
		&p.FaceVerifiedAt,
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

// FindNearbyProfiles discovers face-verified profiles within radiusMeters using
// production-grade PostgreSQL earthdistance GiST spatial indexing, with fallback to indexed bounding-box.
func (r *PostgresRepository) FindNearbyProfiles(
	ctx context.Context,
	email string,
	lat, lon, radiusMeters float64,
	limit int,
) ([]NearbyRecord, error) {
	if limit <= 0 {
		limit = 30
	}
	if radiusMeters <= 0 {
		radiusMeters = 30.0
	}

	// 1. Production-grade earthdistance query leveraging GiST spatial index
	prodQuery := `
		SELECT email, name, bio, photo_url,
		       earth_distance(ll_to_earth($2, $3), ll_to_earth(latitude, longitude)) AS distance_meters
		FROM profiles
		WHERE email <> $1
		  AND face_verified_at IS NOT NULL
		  AND latitude IS NOT NULL AND longitude IS NOT NULL
		  AND earth_box(ll_to_earth($2, $3), $4) @> ll_to_earth(latitude, longitude)
		  AND earth_distance(ll_to_earth($2, $3), ll_to_earth(latitude, longitude)) <= $4
		ORDER BY distance_meters ASC
		LIMIT $5;
	`
	rows, err := r.db.Pool.Query(ctx, prodQuery, email, lat, lon, radiusMeters, limit)
	if err == nil {
		defer rows.Close()
		var results []NearbyRecord
		for rows.Next() {
			var rec NearbyRecord
			if err := rows.Scan(&rec.Email, &rec.Name, &rec.Bio, &rec.PhotoURL, &rec.DistanceMeters); err != nil {
				return nil, fmt.Errorf("failed scanning nearby profile: %w", err)
			}
			results = append(results, rec)
		}
		if err := rows.Err(); err == nil {
			return results, nil
		}
	}

	// 2. Resilient Fallback: Coordinate Bounding Box Scan (uses idx_profiles_discovery)
	latDelta := radiusMeters / 111000.0
	cosLat := math.Cos(lat * math.Pi / 180.0)
	if cosLat < 0.2 {
		cosLat = 0.2
	}
	lonDelta := radiusMeters / (111000.0 * cosLat)

	fallbackQuery := `
		SELECT email, name, bio, photo_url, latitude, longitude
		FROM profiles
		WHERE email <> $1
		  AND face_verified_at IS NOT NULL
		  AND latitude IS NOT NULL AND longitude IS NOT NULL
		  AND latitude BETWEEN $2 AND $3
		  AND longitude BETWEEN $4 AND $5
		LIMIT 500;
	`
	fRows, err := r.db.Pool.Query(ctx, fallbackQuery,
		email,
		lat-latDelta, lat+latDelta,
		lon-lonDelta, lon+lonDelta,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query nearby candidates: %w", err)
	}
	defer fRows.Close()

	type candidate struct {
		rec      NearbyRecord
		lat, lon float64
	}
	var candidates []candidate

	for fRows.Next() {
		var (
			rec      NearbyRecord
			clat, clon float64
		)
		if err := fRows.Scan(&rec.Email, &rec.Name, &rec.Bio, &rec.PhotoURL, &clat, &clon); err != nil {
			return nil, fmt.Errorf("failed to scan nearby candidate: %w", err)
		}
		candidates = append(candidates, candidate{rec: rec, lat: clat, lon: clon})
	}
	if err := fRows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating nearby candidates: %w", err)
	}

	// Refine stage: exact haversine distance, sorted nearest first.
	results := make([]NearbyRecord, 0, len(candidates))
	for _, cand := range candidates {
		d := haversineMeters(lat, lon, cand.lat, cand.lon)
		if d <= radiusMeters {
			cand.rec.DistanceMeters = d
			results = append(results, cand.rec)
		}
	}
	slices.SortFunc(results, func(a, b NearbyRecord) int {
		return cmp.Compare(a.DistanceMeters, b.DistanceMeters)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	rad := math.Pi / 180.0
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}
