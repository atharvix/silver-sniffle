package profile

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/atharvix/kinjo-backend/internal/database"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/security"
	"github.com/jackc/pgx/v5"
)

type Repository interface {
	Upsert(ctx context.Context, p *domain.Profile) error
	GetByEmail(ctx context.Context, email string) (*domain.Profile, error)
	MarkFaceVerified(ctx context.Context, email string, faceScanPhotoURL string) error
	IsFaceVerified(ctx context.Context, email string) (bool, error)
	BackfillEncryption(ctx context.Context, c *security.Crypto) (int, error)
}

type PostgresRepository struct {
	db *database.DB
	c  *security.Crypto
}

func NewRepository(db *database.DB, c *security.Crypto) *PostgresRepository {
	return &PostgresRepository{db: db, c: c}
}

// GeoCellRounding matches the 3-decimal rounding (~111m) used by the DB
// indexes: discovery only ever searches adjacent cells.
func GeoCellRounding(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func (r *PostgresRepository) Upsert(ctx context.Context, p *domain.Profile) error {
	now := time.Now()
	emailHash := r.c.EmailHash(p.Email)
	emailEnc, err := r.c.EncryptEmail(p.Email)
	if err != nil {
		return fmt.Errorf("failed to encrypt email: %w", err)
	}
	nameEnc, err := r.c.Encrypt(p.Name)
	if err != nil {
		return fmt.Errorf("failed to encrypt name: %w", err)
	}
	bioEnc, err := r.c.Encrypt(p.Bio)
	if err != nil {
		return fmt.Errorf("failed to encrypt bio: %w", err)
	}

	var latEnc, lonEnc *string
	var latCell, lonCell *float64
	if p.Latitude != nil && p.Longitude != nil {
		le, err := r.c.EncryptFloat(*p.Latitude)
		if err != nil {
			return fmt.Errorf("failed to encrypt latitude: %w", err)
		}
		lonE, err := r.c.EncryptFloat(*p.Longitude)
		if err != nil {
			return fmt.Errorf("failed to encrypt longitude: %w", err)
		}
		latEnc, lonEnc = &le, &lonE
		lc := GeoCellRounding(*p.Latitude)
		lnc := GeoCellRounding(*p.Longitude)
		latCell, lonCell = &lc, &lnc
	}

	query := `
		INSERT INTO profiles (email_hash, email_enc, name_enc, bio_enc, photo_url,
			lat_enc, lon_enc, geo_lat_cell, geo_lon_cell, last_seen_at, updated_at)
		VALUES ($1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			CASE WHEN $6::text IS NOT NULL THEN $10::timestamptz ELSE NULL END, $10::timestamptz)
		ON CONFLICT (email_hash) DO UPDATE SET
			email_enc = EXCLUDED.email_enc,
			name_enc = EXCLUDED.name_enc,
			bio_enc = EXCLUDED.bio_enc,
			photo_url = EXCLUDED.photo_url,
			lat_enc = COALESCE(EXCLUDED.lat_enc, profiles.lat_enc),
			lon_enc = COALESCE(EXCLUDED.lon_enc, profiles.lon_enc),
			geo_lat_cell = COALESCE(EXCLUDED.geo_lat_cell, profiles.geo_lat_cell),
			geo_lon_cell = COALESCE(EXCLUDED.geo_lon_cell, profiles.geo_lon_cell),
			last_seen_at = CASE WHEN EXCLUDED.lat_enc IS NOT NULL THEN $10::timestamptz ELSE profiles.last_seen_at END,
			updated_at = EXCLUDED.updated_at;
	`
	_, err = r.db.Pool.Exec(ctx, query,
		emailHash, emailEnc, nameEnc, bioEnc, p.PhotoURL,
		latEnc, lonEnc, latCell, lonCell, now,
	)
	if err != nil {
		return fmt.Errorf("failed to save profile: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (*domain.Profile, error) {
	emailHash := r.c.EmailHash(email)
	query := `
		SELECT email_enc, name_enc, bio_enc, photo_url, lat_enc, lon_enc, last_seen_at,
		       ai_summary, headline, face_verified_at, face_scan_photo_url, created_at, updated_at
		FROM profiles
		WHERE email_hash = $1;
	`
	var (
		emailEnc, nameEnc, bioEnc string
		latEnc, lonEnc            *string
	)
	var p domain.Profile
	err := r.db.Pool.QueryRow(ctx, query, emailHash).Scan(
		&emailEnc,
		&nameEnc,
		&bioEnc,
		&p.PhotoURL,
		&latEnc,
		&lonEnc,
		&p.LastSeenAt,
		&p.AISummary,
		&p.Headline,
		&p.FaceVerifiedAt,
		&p.FaceScanPhotoURL,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProfileNotFound
		}
		return nil, fmt.Errorf("failed to query profile: %w", err)
	}

	if p.Email, err = r.c.DecryptEmail(emailEnc); err != nil {
		return nil, fmt.Errorf("failed to decrypt profile email: %w", err)
	}
	if p.Name, err = r.c.Decrypt(nameEnc); err != nil {
		return nil, fmt.Errorf("failed to decrypt profile name: %w", err)
	}
	if p.Bio, err = r.c.Decrypt(bioEnc); err != nil {
		return nil, fmt.Errorf("failed to decrypt profile bio: %w", err)
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

// MarkFaceVerified records a successful live face scan server-side.
func (r *PostgresRepository) MarkFaceVerified(ctx context.Context, email string, faceScanPhotoURL string) error {
	cmd, err := r.db.Pool.Exec(ctx, `
		UPDATE profiles
		SET face_verified_at = NOW(), face_scan_photo_url = $2, updated_at = NOW()
		WHERE email_hash = $1;
	`, r.c.EmailHash(email), faceScanPhotoURL)
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
		SELECT (face_verified_at IS NOT NULL) FROM profiles WHERE email_hash = $1;
	`, r.c.EmailHash(email)).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrProfileNotFound
	}
	return verified, err
}

// BackfillEncryption migrates legacy plaintext rows (keyed by the old
// plaintext email column) into the encrypted columns. Idempotent: rows with
// email_hash already populated in encrypted form (email_enc "v1:") are
// skipped. Returns the number of rows migrated.
func (r *PostgresRepository) BackfillEncryption(ctx context.Context, c *security.Crypto) (int, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT COALESCE(email, ''), COALESCE(name, ''), COALESCE(bio, ''),
		       latitude, longitude
		FROM profiles
		WHERE email IS NOT NULL AND email <> ''
		  AND (email_enc = '' OR email_enc NOT LIKE 'v1:%');
	`)
	if err != nil {
		return 0, fmt.Errorf("backfill select failed: %w", err)
	}
	defer rows.Close()

	type row struct {
		email, name, bio string
		lat, lon         *float64
	}
	var pending []row
	for rows.Next() {
		var rw row
		if err := rows.Scan(&rw.email, &rw.name, &rw.bio, &rw.lat, &rw.lon); err != nil {
			return 0, err
		}
		pending = append(pending, rw)
	}
	rows.Close()

	migrated := 0
	for _, rw := range pending {
		emailHash := c.EmailHash(rw.email)
		emailEnc, err := c.EncryptEmail(rw.email)
		if err != nil {
			return migrated, err
		}
		nameEnc, err := c.Encrypt(rw.name)
		if err != nil {
			return migrated, err
		}
		bioEnc, err := c.Encrypt(rw.bio)
		if err != nil {
			return migrated, err
		}
		var latEnc, lonEnc *string
		var latCell, lonCell *float64
		if rw.lat != nil && rw.lon != nil {
			le, err := c.EncryptFloat(*rw.lat)
			if err != nil {
				return migrated, err
			}
			lne, err := c.EncryptFloat(*rw.lon)
			if err != nil {
				return migrated, err
			}
			latEnc, lonEnc = &le, &lne
			lc := GeoCellRounding(*rw.lat)
			lnc := GeoCellRounding(*rw.lon)
			latCell, lonCell = &lc, &lnc
		}

		// Handle duplicate email_hash (case-variant duplicates of the same
		// address): keep one row, drop the extras.
		tag, err := r.db.Pool.Exec(ctx, `
			UPDATE profiles SET email_hash = $1, email_enc = $2, name_enc = $3, bio_enc = $4,
			       lat_enc = $5, lon_enc = $6, geo_lat_cell = $7, geo_lon_cell = $8
			WHERE email = $9
			  AND NOT EXISTS (SELECT 1 FROM profiles p2 WHERE p2.email_hash = $1 AND p2.email <> $9);
		`, emailHash, emailEnc, nameEnc, bioEnc, latEnc, lonEnc, latCell, lonCell, rw.email)
		if err != nil {
			return migrated, fmt.Errorf("backfill update failed for %s: %w", rw.email, err)
		}
		if tag.RowsAffected() == 0 {
			// Duplicate of an already-migrated row; delete legacy duplicate.
			_, _ = r.db.Pool.Exec(ctx, `DELETE FROM profiles WHERE email = $1 AND email_hash IS DISTINCT FROM $2`, rw.email, emailHash)
		} else {
			migrated++
		}
	}
	return migrated, nil
}
