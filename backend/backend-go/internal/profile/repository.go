package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repository interface {
	Upsert(ctx context.Context, p *domain.Profile) error
	GetByEmail(ctx context.Context, email string) (*domain.Profile, error)
	MarkFaceVerified(ctx context.Context, email string, faceScanPhotoURL string) error
	IsFaceVerified(ctx context.Context, email string) (bool, error)
	SaveFaceChallenge(ctx context.Context, email, challengeHash string, expiresAt time.Time) error
	ConsumeFaceChallenge(ctx context.Context, email, challengeHash string) error
	SetFaceScanHash(ctx context.Context, email, faceScanHash string) error
}

type PostgresRepository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Upsert(ctx context.Context, p *domain.Profile) error {
	now := time.Now()

	query := `
		INSERT INTO profiles (email, name, bio, photo_url, latitude, longitude, last_seen_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6,
			CASE WHEN $5::double precision IS NOT NULL THEN $7::timestamptz ELSE NULL END, $7::timestamptz)
		ON CONFLICT (email) DO UPDATE SET
			name = EXCLUDED.name,
			bio = EXCLUDED.bio,
			photo_url = CASE WHEN EXCLUDED.photo_url <> '' THEN EXCLUDED.photo_url ELSE profiles.photo_url END,
			latitude = COALESCE(EXCLUDED.latitude, profiles.latitude),
			longitude = COALESCE(EXCLUDED.longitude, profiles.longitude),
			last_seen_at = CASE WHEN EXCLUDED.latitude IS NOT NULL THEN $7::timestamptz ELSE profiles.last_seen_at END,
			updated_at = EXCLUDED.updated_at;
	`
	_, err := r.db.Pool.Exec(ctx, query,
		p.Email, p.Name, p.Bio, p.PhotoURL,
		p.Latitude, p.Longitude, now,
	)
	if err != nil {
		return fmt.Errorf("failed to save profile: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (*domain.Profile, error) {
	query := `
		SELECT email, name, bio, photo_url, latitude, longitude, last_seen_at,
		       face_verified_at, face_scan_photo_url, COALESCE(face_scan_hash, ''), created_at, updated_at
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
		&p.FaceScanPhotoURL,
		&p.FaceScanHash,
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

// SaveFaceChallenge replaces the caller's pending challenge. Only one is live
// per account, so issuing a new one invalidates the previous scan.
func (r *PostgresRepository) SaveFaceChallenge(ctx context.Context, email, challengeHash string, expiresAt time.Time) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO face_challenges (email, challenge_hash, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (email) DO UPDATE SET
			challenge_hash = EXCLUDED.challenge_hash,
			expires_at = EXCLUDED.expires_at,
			created_at = NOW();
	`, email, challengeHash, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to save face challenge: %w", err)
	}
	return nil
}

// ConsumeFaceChallenge deletes the caller's challenge if it matches and is
// still within its TTL. Deleting is what makes the nonce single-use.
func (r *PostgresRepository) ConsumeFaceChallenge(ctx context.Context, email, challengeHash string) error {
	cmd, err := r.db.Pool.Exec(ctx, `
		DELETE FROM face_challenges
		WHERE email = $1 AND challenge_hash = $2 AND expires_at > NOW();
	`, email, challengeHash)
	if err != nil {
		return fmt.Errorf("failed to consume face challenge: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrForbidden
	}
	return nil
}

// SetFaceScanHash binds a captured selfie to this account. The unique index on
// face_scan_hash is what stops one photo verifying several accounts.
func (r *PostgresRepository) SetFaceScanHash(ctx context.Context, email, faceScanHash string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE profiles SET face_scan_hash = $2 WHERE email = $1;`, email, faceScanHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrFaceScanReused
		}
		return fmt.Errorf("failed to store face scan hash: %w", err)
	}
	return nil
}

// MarkFaceVerified records a successful live face scan server-side.
func (r *PostgresRepository) MarkFaceVerified(ctx context.Context, email string, faceScanPhotoURL string) error {
	cmd, err := r.db.Pool.Exec(ctx, `
		UPDATE profiles
		SET face_verified_at = NOW(), face_scan_photo_url = $2, updated_at = NOW()
		WHERE email = $1;
	`, email, faceScanPhotoURL)
	if err != nil {
		return fmt.Errorf("failed to mark face verified: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}
	return nil
}

func (r *PostgresRepository) IsFaceVerified(ctx context.Context, email string) (bool, error) {
	var verified bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT (face_verified_at IS NOT NULL AND COALESCE(face_scan_hash, '') <> '')
		FROM profiles WHERE email = $1;
	`, email).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrProfileNotFound
	}
	return verified, err
}
