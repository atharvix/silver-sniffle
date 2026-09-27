package presence

import (
	"context"
	"fmt"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
)

// verifiedQuery reports whether an account completed the current liveness
// flow. Legacy verifications (no face_scan_hash) come back false so the caller
// is told to re-verify rather than getting "profile not found".
const verifiedQuery = `SELECT (face_verified_at IS NOT NULL AND COALESCE(face_scan_hash, '') <> '') FROM profiles WHERE email = $1`

type UpdateLocationResult struct {
	IsFirstLocation bool
	Name            string
}

type Repository interface {
	UpdateLocationWithResult(ctx context.Context, email string, lat, lon float64) (*UpdateLocationResult, error)
	RecordHeartbeat(ctx context.Context, email string) error
	MarkOffline(ctx context.Context, email string) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) UpdateLocationWithResult(ctx context.Context, email string, lat, lon float64) (*UpdateLocationResult, error) {
	now := time.Now()
	query := `
		WITH prev AS (
			SELECT latitude, name FROM profiles WHERE email = $4
		)
		UPDATE profiles
		SET latitude = $1,
		    longitude = $2,
		    last_seen_at = $3,
		    updated_at = $3
		WHERE email = $4
		  AND face_verified_at IS NOT NULL AND COALESCE(face_scan_hash, '') <> ''
		RETURNING (SELECT latitude IS NULL FROM prev) AS is_first_location,
		          COALESCE((SELECT name FROM prev), '');
	`
	var isFirst bool
	var name string
	err := r.db.Pool.QueryRow(ctx, query, lat, lon, now, email).Scan(&isFirst, &name)
	if err != nil {
		var verified bool
		if queryErr := r.db.Pool.QueryRow(ctx, verifiedQuery, email).Scan(&verified); queryErr == nil && !verified {
			return nil, domain.ErrForbidden
		}
		return nil, domain.ErrProfileNotFound
	}

	return &UpdateLocationResult{
		IsFirstLocation: isFirst,
		Name:            name,
	}, nil
}

func (r *PostgresRepository) RecordHeartbeat(ctx context.Context, email string) error {
	query := `
		UPDATE profiles
		SET last_seen_at = $1
		WHERE email = $2
		  AND face_verified_at IS NOT NULL AND COALESCE(face_scan_hash, '') <> '';
	`
	cmdTag, err := r.db.Pool.Exec(ctx, query, time.Now(), email)
	if err != nil {
		return fmt.Errorf("failed to record heartbeat: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		var verified bool
		if err := r.db.Pool.QueryRow(ctx, verifiedQuery, email).Scan(&verified); err == nil && !verified {
			return domain.ErrForbidden
		}
		return domain.ErrProfileNotFound
	}

	return nil
}

func (r *PostgresRepository) MarkOffline(ctx context.Context, email string) error {
	query := `
		UPDATE profiles
		SET last_seen_at = NULL
		WHERE email = $1;
	`
	_, err := r.db.Pool.Exec(ctx, query, email)
	if err != nil {
		return fmt.Errorf("failed to mark offline: %w", err)
	}
	return nil
}
