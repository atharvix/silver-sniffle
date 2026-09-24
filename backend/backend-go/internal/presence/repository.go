package presence

import (
	"context"
	"fmt"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
)

type UpdateLocationResult struct {
	IsFirstLocation bool
	Name            string
}

type Repository interface {
	UpdateLocation(ctx context.Context, email string, lat, lon float64) error
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
		  AND face_verified_at IS NOT NULL
		RETURNING (SELECT latitude IS NULL FROM prev) AS is_first_location,
		          COALESCE((SELECT name FROM prev), '');
	`
	var isFirst bool
	var name string
	err := r.db.Pool.QueryRow(ctx, query, lat, lon, now, email).Scan(&isFirst, &name)
	if err != nil {
		var faceVerifiedAt *time.Time
		if queryErr := r.db.Pool.QueryRow(ctx, "SELECT face_verified_at FROM profiles WHERE email = $1", email).Scan(&faceVerifiedAt); queryErr == nil {
			if faceVerifiedAt == nil {
				return nil, domain.ErrForbidden
			}
		}
		return nil, domain.ErrProfileNotFound
	}

	return &UpdateLocationResult{
		IsFirstLocation: isFirst,
		Name:            name,
	}, nil
}

func (r *PostgresRepository) UpdateLocation(ctx context.Context, email string, lat, lon float64) error {
	_, err := r.UpdateLocationWithResult(ctx, email, lat, lon)
	return err
}

func (r *PostgresRepository) RecordHeartbeat(ctx context.Context, email string) error {
	query := `
		UPDATE profiles
		SET last_seen_at = $1
		WHERE email = $2
		  AND face_verified_at IS NOT NULL;
	`
	cmdTag, err := r.db.Pool.Exec(ctx, query, time.Now(), email)
	if err != nil {
		return fmt.Errorf("failed to record heartbeat: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		var faceVerifiedAt *time.Time
		if err := r.db.Pool.QueryRow(ctx, "SELECT face_verified_at FROM profiles WHERE email = $1", email).Scan(&faceVerifiedAt); err == nil {
			if faceVerifiedAt == nil {
				return domain.ErrForbidden
			}
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
