package presence

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/security"
)

// geoCell rounds a coordinate to 3 decimals (~111m grid) matching the
// profile package's GeoCellRounding and the DB geo-cell indexes.
func geoCell(v float64) float64 {
	return math.Round(v*1000) / 1000
}

type Repository interface {
	UpdateLocation(ctx context.Context, email string, lat, lon float64) error
	RecordHeartbeat(ctx context.Context, email string) error
	MarkOffline(ctx context.Context, email string) error
}

type PostgresRepository struct {
	db *database.DB
	c  *security.Crypto
}

func NewRepository(db *database.DB, c *security.Crypto) *PostgresRepository {
	return &PostgresRepository{db: db, c: c}
}

func (r *PostgresRepository) UpdateLocation(ctx context.Context, email string, lat, lon float64) error {
	now := time.Now()
	latEnc, err := r.c.EncryptFloat(lat)
	if err != nil {
		return fmt.Errorf("failed to encrypt latitude: %w", err)
	}
	lonEnc, err := r.c.EncryptFloat(lon)
	if err != nil {
		return fmt.Errorf("failed to encrypt longitude: %w", err)
	}
	query := `
		UPDATE profiles
		SET lat_enc = $1,
		    lon_enc = $2,
		    geo_lat_cell = $3,
		    geo_lon_cell = $4,
		    last_seen_at = $5,
		    updated_at = $5
		WHERE email_hash = $6;
	`
	cmdTag, err := r.db.Pool.Exec(ctx, query,
		latEnc, lonEnc,
		geoCell(lat), geoCell(lon),
		now, r.c.EmailHash(email),
	)
	if err != nil {
		return fmt.Errorf("failed to update location: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}

	return nil
}

func (r *PostgresRepository) RecordHeartbeat(ctx context.Context, email string) error {
	query := `
		UPDATE profiles
		SET last_seen_at = $1
		WHERE email_hash = $2;
	`
	cmdTag, err := r.db.Pool.Exec(ctx, query, time.Now(), r.c.EmailHash(email))
	if err != nil {
		return fmt.Errorf("failed to record heartbeat: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}

	return nil
}

func (r *PostgresRepository) MarkOffline(ctx context.Context, email string) error {
	query := `
		UPDATE profiles
		SET last_seen_at = NULL
		WHERE email_hash = $1;
	`
	_, err := r.db.Pool.Exec(ctx, query, r.c.EmailHash(email))
	if err != nil {
		return fmt.Errorf("failed to mark offline: %w", err)
	}
	return nil
}
