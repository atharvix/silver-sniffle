package presence

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
	UpdateLocation(ctx context.Context, email string, lat, lon float64) error
	RecordHeartbeat(ctx context.Context, email string) error
	MarkOffline(ctx context.Context, email string) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) UpdateLocation(ctx context.Context, email string, lat, lon float64) error {
	now := time.Now()
	query := `
		UPDATE profiles 
		SET latitude = $1, 
		    longitude = $2, 
		    last_seen_at = $3, 
		    updated_at = $3 
		WHERE email = $4;
	`
	cmdTag, err := r.db.Pool.Exec(ctx, query, lat, lon, now, email)
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
		WHERE email = $2;
	`
	cmdTag, err := r.db.Pool.Exec(ctx, query, time.Now(), email)
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
		WHERE email = $1;
	`
	_, err := r.db.Pool.Exec(ctx, query, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("failed to mark offline: %w", err)
	}
	return nil
}
