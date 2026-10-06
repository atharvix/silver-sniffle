package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the Postgres pool and all queries. This is the entire data layer
// that used to be Firestore.
type Store struct {
	pool *pgxpool.Pool
}

// Profile is the public card shown to people nearby.
type Profile struct {
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Look      string    `json:"look"`
	Photo     string    `json:"photo"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (p Profile) complete() bool {
	return p.Name != "" && p.Role != "" && p.Look != "" && p.Photo != ""
}

// User is the account backing a profile, keyed by the LinkedIn subject id.
type User struct {
	ID          string
	LinkedInSub string
	Email       string
	Verified    bool
	Name        string // name as returned by LinkedIn, used to seed the profile
	Picture     string
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id           text PRIMARY KEY,
  linkedin_sub text UNIQUE NOT NULL,
  email        text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS profiles (
  uid        text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  name       text NOT NULL,
  role       text NOT NULL DEFAULT '',
  look       text NOT NULL DEFAULT '',
  photo      text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS presence (
  uid  text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  cell text NOT NULL,
  lat  double precision NOT NULL,
  lng  double precision NOT NULL,
  acc  int NOT NULL DEFAULT 0,
  t    timestamptz NOT NULL DEFAULT now()
);
-- matching runs in memory, so a cell index was write cost with no reader (and it
-- blocked HOT updates on the hottest table). Same reason there's no index on t.
DROP INDEX IF EXISTS presence_cell_idx;
CREATE TABLE IF NOT EXISTS sessions (
  token      text PRIMARY KEY,
  uid        text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_uid_idx ON sessions (uid);
-- tokens are stored as sha256 hex (a DB leak must not hand out live sessions);
-- this one-time rewrite of older plaintext rows is a no-op once done (43 vs 64 chars).
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS seen_at timestamptz NOT NULL DEFAULT now();
UPDATE sessions SET token = encode(sha256(convert_to(token, 'UTF8')), 'hex') WHERE length(token) <> 64;

-- email verification: LinkedIn already returns a verified email, but this
-- supports verifying a changed address via a link the backend emails out.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at timestamptz;
CREATE TABLE IF NOT EXISTS email_verifications (
  token       text PRIMARY KEY,
  uid         text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email       text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  expires_at  timestamptz NOT NULL,
  consumed_at timestamptz
);
CREATE INDEX IF NOT EXISTS email_verifications_uid_idx ON email_verifications (uid);

-- push devices: one row per install, keyed by the push token (e.g. FCM token).
CREATE TABLE IF NOT EXISTS devices (
  push_token text PRIMARY KEY,
  uid        text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  platform   text NOT NULL DEFAULT 'android',
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS devices_uid_idx ON devices (uid);

-- custom notifications: the feed the app shows + what was pushed out.
CREATE TABLE IF NOT EXISTS notifications (
  id         bigserial PRIMARY KEY,
  uid        text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title      text NOT NULL,
  body       text NOT NULL DEFAULT '',
  data       jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  sent_at    timestamptz,
  read_at    timestamptz
);
CREATE INDEX IF NOT EXISTS notifications_uid_idx ON notifications (uid, created_at DESC);

-- native advertising: sponsored brand cards interleaved into the feed.
CREATE TABLE IF NOT EXISTS ads (
  id               bigserial PRIMARY KEY,
  brand_name       text NOT NULL,
  logo             text NOT NULL DEFAULT '',
  description      text NOT NULL DEFAULT '',
  campaign_message text NOT NULL DEFAULT '',
  category         text NOT NULL DEFAULT '',
  location         text NOT NULL DEFAULT '',
  image            text NOT NULL DEFAULT '',
  cta_label        text NOT NULL DEFAULT 'Learn More',
  destination_url  text NOT NULL DEFAULT '',
  active           boolean NOT NULL DEFAULT true,
  created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS ad_events (
  id         bigserial PRIMARY KEY,
  ad_id      bigint NOT NULL REFERENCES ads(id) ON DELETE CASCADE,
  uid        text REFERENCES users(id) ON DELETE SET NULL,
  event      text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ad_events_ad_idx ON ad_events (ad_id, event);
`

func openStore(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// pgx defaults to max(4, CPUs) connections: a reconnect burst queues behind
	// four. Override with pool_max_conns in DATABASE_URL; keep under Postgres max_connections.
	if !strings.Contains(dsn, "pool_max_conns") {
		cfg.MaxConns = 20
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// Verify the connection up front so misconfiguration surfaces at boot.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	// Run each DDL statement on its own: pgx's default (extended) protocol
	// rejects multiple statements in one Exec, so we split on ';'.
	for _, stmt := range strings.Split(schema, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := pool.Exec(ctx, stmt); err != nil {
			pool.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// upsertUserFromLinkedIn creates or refreshes the account for a LinkedIn login
// and returns our internal uid plus whether this login created a brand-new
// account (so the caller can send a welcome email). The uid is stable.
func (s *Store) upsertUserFromLinkedIn(ctx context.Context, sub, email, name string) (uid string, created bool, err error) {
	uid = "li_" + sub
	// RETURNING (xmax = 0): true for a freshly inserted row, false for an update.
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (id, linkedin_sub, email) VALUES ($1, $2, $3)
		ON CONFLICT (linkedin_sub) DO UPDATE SET email = EXCLUDED.email
		RETURNING (xmax = 0)`,
		uid, sub, email).Scan(&created)
	if err != nil {
		return "", false, err
	}
	return uid, created, nil
}

// seedProfile pre-fills name and photo from LinkedIn on first login. Later
// logins only replace a photo that is still a raw LinkedIn link (those expire);
// a name or photo the user set is never touched.
func (s *Store) seedProfile(ctx context.Context, uid, name, photo string) error {
	if name == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO profiles (uid, name, photo, updated_at) VALUES ($1, $2, $3, now())
		ON CONFLICT (uid) DO UPDATE SET photo = EXCLUDED.photo, updated_at = now()
		WHERE profiles.photo LIKE 'https://media.licdn%'`, uid, name, photo)
	return err
}

func (s *Store) getUser(ctx context.Context, uid string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, linkedin_sub, email, email_verified FROM users WHERE id = $1`, uid).
		Scan(&u.ID, &u.LinkedInSub, &u.Email, &u.Verified)
	return u, err
}

// getProfile returns the profile or (zero, false) when none exists yet.
func (s *Store) getProfile(ctx context.Context, uid string) (Profile, bool, error) {
	var p Profile
	err := s.pool.QueryRow(ctx,
		`SELECT name, role, look, photo, updated_at FROM profiles WHERE uid = $1`, uid).
		Scan(&p.Name, &p.Role, &p.Look, &p.Photo, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	return p, true, nil
}

func (s *Store) saveProfile(ctx context.Context, uid string, p Profile) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO profiles (uid, name, role, look, photo, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (uid) DO UPDATE SET
		  name = EXCLUDED.name, role = EXCLUDED.role,
		  look = EXCLUDED.look, photo = EXCLUDED.photo, updated_at = now()`,
		uid, p.Name, p.Role, p.Look, p.Photo)
	return err
}

func (s *Store) upsertPresence(ctx context.Context, uid string, p Pres) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO presence (uid, cell, lat, lng, acc, t)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (uid) DO UPDATE SET
		  cell = EXCLUDED.cell, lat = EXCLUDED.lat,
		  lng = EXCLUDED.lng, acc = EXCLUDED.acc, t = now()`,
		uid, p.Cell, p.Lat, p.Lng, p.Acc)
	return err
}

func (s *Store) deletePresence(ctx context.Context, uid string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM presence WHERE uid = $1`, uid)
	return err
}

// deleteStalePresence drops positions older than cutoff. Atomic per row: a
// user who reports again in the meantime has a fresh t and is kept.
func (s *Store) deleteStalePresence(ctx context.Context, cutoff time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM presence WHERE t < $1`, cutoff)
	return err
}

// loadRecentPresence rebuilds the in-memory presence set on startup so a
// backend restart doesn't drop everyone off the map.
func (s *Store) loadRecentPresence(ctx context.Context) (map[string]*Pres, error) {
	cutoff := time.Now().Add(-time.Duration(staleMS) * time.Millisecond)
	rows, err := s.pool.Query(ctx,
		`SELECT uid, cell, lat, lng, acc, t FROM presence WHERE t > $1`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]*Pres)
	for rows.Next() {
		p := &Pres{}
		if err := rows.Scan(&p.UID, &p.Cell, &p.Lat, &p.Lng, &p.Acc, &p.T); err != nil {
			return nil, err
		}
		out[p.UID] = p
	}
	return out, rows.Err()
}

// hashToken is what's stored for a session: the client holds the only copy of
// the raw token.
func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func (s *Store) createSession(ctx context.Context, token, uid string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (token, uid) VALUES ($1, $2)`, hashToken(token), uid)
	return err
}

// uidForToken resolves a session. Sessions slide: valid while used at least once
// every 90 days. seen_at is bumped at most daily, so the hot path stays one read.
func (s *Store) uidForToken(ctx context.Context, token string) (string, error) {
	h := hashToken(token)
	var uid string
	var bump bool
	err := s.pool.QueryRow(ctx, `
		SELECT uid, seen_at < now() - interval '1 day' FROM sessions
		WHERE token = $1 AND seen_at > now() - interval '90 days'`, h).Scan(&uid, &bump)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err == nil && bump {
		_, _ = s.pool.Exec(ctx, `UPDATE sessions SET seen_at = now() WHERE token = $1`, h)
	}
	return uid, err
}

func (s *Store) deleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, hashToken(token))
	return err
}

// deleteAccount removes the user and everything that cascades from it.
func (s *Store) deleteAccount(ctx context.Context, uid string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid)
	return err
}

/* ---------------- email verification ---------------- */

// createEmailVerification stores a one-time token for verifying an address.
func (s *Store) createEmailVerification(ctx context.Context, token, uid, email string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO email_verifications (token, uid, email, expires_at)
		VALUES ($1, $2, $3, $4)`, token, uid, email, time.Now().Add(ttl))
	return err
}

// consumeEmailVerification validates an unexpired token and flips the user's email
// to verified. Idempotent until expiry on purpose: corporate mail scanners (e.g.
// Microsoft SafeLinks) open links before the person does, and the real click
// must still say "verified", not "already used". Returns the uid on success.
func (s *Store) consumeEmailVerification(ctx context.Context, token string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var uid, email string
	err = tx.QueryRow(ctx, `
		UPDATE email_verifications SET consumed_at = coalesce(consumed_at, now())
		WHERE token = $1 AND expires_at > now()
		RETURNING uid, email`, token).Scan(&uid, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil // invalid / expired / already used
	}
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE users SET email = $2, email_verified = true, email_verified_at = now()
		WHERE id = $1`, uid, email); err != nil {
		return "", err
	}
	return uid, tx.Commit(ctx)
}

/* ---------------- push devices ---------------- */

// upsertDevice records (or refreshes) a push token for a user. One row per
// token; if the token moves to another account, it's reassigned.
func (s *Store) upsertDevice(ctx context.Context, uid, pushToken, platform string) error {
	if platform == "" {
		platform = "android"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO devices (push_token, uid, platform, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (push_token) DO UPDATE SET
		  uid = EXCLUDED.uid, platform = EXCLUDED.platform, updated_at = now()`,
		pushToken, uid, platform)
	return err
}

func (s *Store) deleteDevice(ctx context.Context, uid, pushToken string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE push_token = $1 AND uid = $2`, pushToken, uid)
	return err
}

// pushTokensForUser returns every device token registered to a user, so a
// push sender (e.g. FCM) can fan a notification out to all their installs.
func (s *Store) pushTokensForUser(ctx context.Context, uid string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT push_token FROM devices WHERE uid = $1`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

/* ---------------- notifications ---------------- */

// Notification is one entry in a user's feed.
type Notification struct {
	ID        int64           `json:"id"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data,omitempty"` // raw JSON payload, or nil
	CreatedAt time.Time       `json:"createdAt"`
	ReadAt    *time.Time      `json:"readAt,omitempty"`
}

// insertNotification saves a notification and returns its id. Sending it out
// (WebSocket while open, FCM while backgrounded) is layered on top.
func (s *Store) insertNotification(ctx context.Context, uid, title, body string, data []byte) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notifications (uid, title, body, data)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		uid, title, body, data).Scan(&id)
	return id, err
}

// listNotifications returns a user's feed, newest first.
func (s *Store) listNotifications(ctx context.Context, uid string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, body, data, created_at, read_at
		FROM notifications WHERE uid = $1 ORDER BY created_at DESC LIMIT $2`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var raw []byte // scan jsonb as raw bytes, then hand back as-is (no base64)
		if err := rows.Scan(&n.ID, &n.Title, &n.Body, &raw, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		n.Data = json.RawMessage(raw)
		out = append(out, n)
	}
	return out, rows.Err()
}

/* ---------------- admin: metrics + listings ---------------- */

// Metrics is the roll-up shown on the admin overview.
type Metrics struct {
	Users            int `json:"users"`
	NewUsers1d       int `json:"newUsers1d"`
	NewUsers7d       int `json:"newUsers7d"`
	VerifiedEmails   int `json:"verifiedEmails"`
	Profiles         int `json:"profiles"`
	CompleteProfiles int `json:"completeProfiles"`
	PresenceRows     int `json:"presenceRows"`
	Devices          int `json:"devices"`
	Notifications    int `json:"notifications"`
}

func (s *Store) metrics(ctx context.Context) (Metrics, error) {
	var m Metrics
	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM users),
		  (SELECT count(*) FROM users WHERE created_at >= now() - interval '1 day'),
		  (SELECT count(*) FROM users WHERE created_at >= now() - interval '7 days'),
		  (SELECT count(*) FROM users WHERE email_verified),
		  (SELECT count(*) FROM profiles),
		  (SELECT count(*) FROM profiles WHERE name <> '' AND role <> '' AND look <> '' AND photo <> ''),
		  (SELECT count(*) FROM presence),
		  (SELECT count(*) FROM devices),
		  (SELECT count(*) FROM notifications)`).
		Scan(&m.Users, &m.NewUsers1d, &m.NewUsers7d, &m.VerifiedEmails,
			&m.Profiles, &m.CompleteProfiles, &m.PresenceRows, &m.Devices, &m.Notifications)
	return m, err
}

// AdminUser is a row in the admin users table.
type AdminUser struct {
	UID       string    `json:"uid"`
	Email     string    `json:"email"`
	Verified  bool      `json:"verified"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Complete  bool      `json:"complete"`
	Devices   int       `json:"devices"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) listUsers(ctx context.Context, limit int) ([]AdminUser, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.email_verified, u.created_at,
		       COALESCE(p.name,''), COALESCE(p.role,''),
		       COALESCE(p.name,'') <> '' AND COALESCE(p.role,'') <> '' AND COALESCE(p.look,'') <> '' AND COALESCE(p.photo,'') <> '',
		       (SELECT count(*) FROM devices d WHERE d.uid = u.id)
		FROM users u LEFT JOIN profiles p ON p.uid = u.id
		ORDER BY u.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.UID, &u.Email, &u.Verified, &u.CreatedAt,
			&u.Name, &u.Role, &u.Complete, &u.Devices); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// emailForUID looks up a user's email by uid (for targeted admin emails).
func (s *Store) emailForUID(ctx context.Context, uid string) (string, error) {
	var email string
	err := s.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, uid).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return email, err
}

// allEmails returns every non-empty email, for broadcast emails.
func (s *Store) allEmails(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT email FROM users WHERE email <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// markNotificationSent records that a notification was pushed to a device, so
// it is never re-pushed on a later device registration.
func (s *Store) markNotificationSent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE notifications SET sent_at = now() WHERE id = $1 AND sent_at IS NULL`, id)
	return err
}

// undeliveredNotifications returns a user's notifications that were never pushed
// to a device and are still unread — delivered exactly once when a device first
// registers (e.g. a welcome notification created before any device existed).
func (s *Store) undeliveredNotifications(ctx context.Context, uid string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, body, data, created_at, read_at
		FROM notifications
		WHERE uid = $1 AND sent_at IS NULL AND read_at IS NULL
		ORDER BY created_at ASC LIMIT $2`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var raw []byte
		if err := rows.Scan(&n.ID, &n.Title, &n.Body, &raw, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		n.Data = raw
		out = append(out, n)
	}
	return out, rows.Err()
}

// reserveVerification records a new verification token unless one was issued in
// the last minute (repeat taps: the first email is already on its way) or 3 in
// the last hour (abuse). A per-user advisory lock makes check-then-insert atomic,
// so simultaneous taps can't all pass the check — that race sent duplicate emails.
// Returns "ok", "recent" or "limited".
func (s *Store) reserveVerification(ctx context.Context, token, uid, email string, ttl time.Duration) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, uid); err != nil {
		return "", err
	}
	var hour, minute int
	if err := tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE created_at > now() - interval '1 minute')
		FROM email_verifications WHERE uid = $1 AND created_at > now() - interval '1 hour'`, uid).
		Scan(&hour, &minute); err != nil {
		return "", err
	}
	switch {
	case minute > 0:
		return "recent", nil
	case hour >= 3:
		return "limited", nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO email_verifications (token, uid, email, expires_at)
		VALUES ($1, $2, $3, $4)`, token, uid, email, time.Now().Add(ttl)); err != nil {
		return "", err
	}
	return "ok", tx.Commit(ctx)
}

type notifTarget struct {
	id  int64
	uid string
}

// broadcastNotification writes one row per user in a single statement, so a
// broadcast is durable before any push goes out: if the server dies mid-send,
// the undelivered rows still reach phones via the device catch-up path.
func (s *Store) broadcastNotification(ctx context.Context, title, body string, data []byte) ([]notifTarget, error) {
	rows, err := s.pool.Query(ctx, `
		INSERT INTO notifications (uid, title, body, data)
		SELECT id, $1, $2, $3 FROM users RETURNING id, uid`, title, body, data)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []notifTarget
	for rows.Next() {
		var t notifTarget
		if err := rows.Scan(&t.id, &t.uid); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// applyRetention deletes data past its useful life so tables stop growing
// forever. Runs daily from the hub's sweeper.
func (s *Store) applyRetention(ctx context.Context) error {
	for _, q := range []string{
		`DELETE FROM sessions WHERE seen_at < now() - interval '90 days'`,
		`DELETE FROM notifications WHERE created_at < now() - interval '90 days'`,
		`DELETE FROM ad_events WHERE created_at < now() - interval '180 days'`,
		`DELETE FROM email_verifications WHERE expires_at < now() - interval '7 days'`,
	} {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// markNotificationsRead marks a user's notifications read (all, or one by id).
func (s *Store) markNotificationsRead(ctx context.Context, uid string, id int64) error {
	if id > 0 {
		_, err := s.pool.Exec(ctx,
			`UPDATE notifications SET read_at = now() WHERE uid = $1 AND id = $2 AND read_at IS NULL`, uid, id)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE uid = $1 AND read_at IS NULL`, uid)
	return err
}
