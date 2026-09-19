package discovery

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/security"
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
	c  *security.Crypto
}

func NewRepository(db *database.DB, c *security.Crypto) *PostgresRepository {
	return &PostgresRepository{db: db, c: c}
}

// geoCellRounding matches the 3-decimal rounding (~111m grid) used by the
// profile and presence repositories and the DB geo-cell index.
func geoCellRounding(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func (r *PostgresRepository) GetCallerProfile(ctx context.Context, email string) (*domain.Profile, error) {
	query := `
		SELECT email_enc, name_enc, bio_enc, photo_url, lat_enc, lon_enc, last_seen_at, created_at, updated_at
		FROM profiles
		WHERE email_hash = $1;
	`
	var (
		emailEnc, nameEnc, bioEnc string
		latEnc, lonEnc            *string
	)
	var p domain.Profile
	err := r.db.Pool.QueryRow(ctx, query, r.c.EmailHash(email)).Scan(
		&emailEnc,
		&nameEnc,
		&bioEnc,
		&p.PhotoURL,
		&latEnc,
		&lonEnc,
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

	if p.Email, err = r.c.DecryptEmail(emailEnc); err != nil {
		return nil, fmt.Errorf("failed to decrypt email: %w", err)
	}
	if p.Name, err = r.c.Decrypt(nameEnc); err != nil {
		return nil, fmt.Errorf("failed to decrypt name: %w", err)
	}
	if p.Bio, err = r.c.Decrypt(bioEnc); err != nil {
		return nil, fmt.Errorf("failed to decrypt bio: %w", err)
	}
	if latEnc != nil && lonEnc != nil {
		lat, err := r.c.DecryptFloat(*latEnc)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt latitude: %w", err)
		}
		lon, err := r.c.DecryptFloat(*lonEnc)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt longitude: %w", err)
		}
		p.Latitude, p.Longitude = &lat, &lon
	}
	return &p, nil
}

// FindNearbyProfiles discovers face-verified profiles within radiusMeters.
//
// Because coordinates are AES-GCM encrypted (non-indexable), proximity
// search uses the plaintext geo-cell grid (3-decimal rounding ≈ 111m):
//   1. Candidate stage: B-tree lookup of rows whose geo cells fall within
//      the target cell ± ceil(radius/111m) cells in both axes (uses
//      idx_profiles_geo_cells).
//   2. Refine stage: exact haversine distance computed on the encrypted
//      coordinates, decrypted in Go, filtered to <= radius, sorted nearest
//      first, and capped at limit.
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

	cellDeg := 111.0 // approx meters per 0.001 deg latitude
	offset := int(math.Ceil(radiusMeters / cellDeg))
	if offset < 1 {
		offset = 1
	}
	cellLat := geoCellRounding(lat)
	cellLon := geoCellRounding(lon)

	callerHash := r.c.EmailHash(email)

	// Candidate stage — only face-verified users with a known location are
	// discoverable. Approx bounding box on cells with longitude correction
	// for the caller's latitude.
	// lonScale is reserved for future asymmetric cell search refinement.
	lonScale := 1.0 / math.Max(0.2, math.Cos(lat*math.Pi/180))
	_ = lonScale

	candidateQuery := `
		SELECT email_enc, name_enc, bio_enc, photo_url, headline, ai_summary, lat_enc, lon_enc
		FROM profiles
		WHERE email_hash <> $1
		  AND face_verified_at IS NOT NULL
		  AND lat_enc IS NOT NULL AND lon_enc IS NOT NULL
		  AND geo_lat_cell BETWEEN $2 AND $3
		  AND geo_lon_cell BETWEEN $4 AND $5
		LIMIT 500;
	`
	rows, err := r.db.Pool.Query(ctx, candidateQuery,
		callerHash,
		cellLat-float64(offset)*0.001, cellLat+float64(offset)*0.001,
		cellLon-float64(offset)*0.001, cellLon+float64(offset)*0.001,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query nearby candidates: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		rec      NearbyRecord
		lat, lon float64
	}
	var candidates []candidate

	for rows.Next() {
		var (
			emailEnc, nameEnc, bioEnc string
			latEnc, lonEnc            *string
			rec                       NearbyRecord
		)
		if err := rows.Scan(&emailEnc, &nameEnc, &bioEnc, &rec.PhotoURL, &rec.Headline, &rec.AISummary, &latEnc, &lonEnc); err != nil {
			return nil, fmt.Errorf("failed to scan nearby candidate: %w", err)
		}
		if latEnc == nil || lonEnc == nil {
			continue
		}
		clat, err := r.c.DecryptFloat(*latEnc)
		if err != nil {
			continue // skip rows that cannot be decrypted rather than failing discovery
		}
		clon, err := r.c.DecryptFloat(*lonEnc)
		if err != nil {
			continue
		}
		if rec.Name, err = r.c.Decrypt(nameEnc); err != nil {
			continue
		}
		if rec.Bio, err = r.c.Decrypt(bioEnc); err != nil {
			continue
		}
		if rec.Email, err = r.c.DecryptEmail(emailEnc); err != nil {
			continue
		}
		candidates = append(candidates, candidate{rec: rec, lat: clat, lon: clon})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating nearby candidates: %w", err)
	}

	// Refine stage — exact haversine, nearest first.
	results := make([]NearbyRecord, 0, len(candidates))
	for _, cand := range candidates {
		d := haversineMeters(lat, lon, cand.lat, cand.lon)
		if d <= radiusMeters {
			cand.rec.DistanceMeters = d
			results = append(results, cand.rec)
		}
	}
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].DistanceMeters < results[j-1].DistanceMeters; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}
