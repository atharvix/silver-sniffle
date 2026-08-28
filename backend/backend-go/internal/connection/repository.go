package connection

import (
	"context"
	"fmt"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
)

type Repository interface {
	Create(ctx context.Context, requesterEmail, recipientEmail string) error
	ListIncoming(ctx context.Context, recipientEmail string) ([]domain.ConnectionNotification, error)
}

type PostgresRepository struct { db *database.DB }

func NewRepository(db *database.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (r *PostgresRepository) Create(ctx context.Context, requesterEmail, recipientEmail string) error {
	result, err := r.db.Pool.Exec(ctx, `
		INSERT INTO connections (requester_email, recipient_email)
		SELECT $1, p.email FROM profiles p
		WHERE p.email = $2
		ON CONFLICT (requester_email, recipient_email) DO NOTHING;
	`, requesterEmail, recipientEmail)
	if err != nil { return fmt.Errorf("failed to create connection: %w", err) }
	if result.RowsAffected() == 0 { return domain.ErrProfileNotFound }
	return nil
}

func (r *PostgresRepository) ListIncoming(ctx context.Context, recipientEmail string) ([]domain.ConnectionNotification, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT c.requester_email, p.name, c.created_at
		FROM connections c JOIN profiles p ON p.email = c.requester_email
		WHERE c.recipient_email = $1 ORDER BY c.created_at DESC;
	`, recipientEmail)
	if err != nil { return nil, fmt.Errorf("failed to list connection notifications: %w", err) }
	defer rows.Close()
	result := []domain.ConnectionNotification{}
	for rows.Next() {
		var notification domain.ConnectionNotification
		if err := rows.Scan(&notification.RequesterEmail, &notification.RequesterName, &notification.CreatedAt); err != nil { return nil, err }
		result = append(result, notification)
	}
	return result, rows.Err()
}