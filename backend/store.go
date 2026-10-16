package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the Postgres pool and all queries. This is the entire data layer
// that used to be Firestore.
type Store struct {
	pool *pgxpool.Pool

	sessMu sync.Mutex
	sess   map[string]sessEntry // token hash -> recently checked session (see uidForToken)
}

// sessionCacheTTL: how long a checked session is trusted from memory. Phones call
// the API every 5-10 s each; this turns one session query per call into one per
// minute. Logout and account deletion drop the entry at once; anything else that
// ends a session (the 90-day expiry) takes effect within this.
const sessionCacheTTL = time.Minute

type sessEntry struct {
	uid string
	at  time.Time
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

// User is the account backing a profile (signed in with LinkedIn, Google or an email code).
type User struct {
	ID       string
	Email    string
	Verified bool
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

-- email verification: providers vouch for their address (email_verified claim),
-- this also verifies a changed address via a link the backend emails out.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at timestamptz;
-- Continue with Google: a user has either or both provider subjects
ALTER TABLE users ADD COLUMN IF NOT EXISTS google_sub text UNIQUE;
ALTER TABLE users ALTER COLUMN linkedin_sub DROP NOT NULL;
-- LinkedIn photos are no longer used. Raw LinkedIn links were signed URLs that have
-- expired anyway, and clearing them sends those people to upload their own photo.
-- (No semicolons in comments here: the schema is split on them.)
UPDATE profiles SET photo = '' WHERE photo LIKE 'https://media.licdn.com/%';
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
-- the session that registered the device: logging out stops its pushes
ALTER TABLE devices ADD COLUMN IF NOT EXISTS session text;
-- the app's own install id (random, made once per install), android or ios
ALTER TABLE devices ADD COLUMN IF NOT EXISTS device_id text;

-- app opens per user per day (India time), for the admin usage view
CREATE TABLE IF NOT EXISTS app_opens (
  uid      text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  day      date NOT NULL,
  opens    int NOT NULL DEFAULT 1,
  platform text NOT NULL DEFAULT '',
  first_at timestamptz NOT NULL DEFAULT now(),
  last_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (uid, day)
);
CREATE INDEX IF NOT EXISTS app_opens_day_idx ON app_opens (day);

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

-- Bluetooth tokens, so phones stay recognisable across a restart (a sleeping phone
-- keeps advertising the token it has). Random, never derived from the uid.
CREATE TABLE IF NOT EXISTS ble_tokens (
  token text PRIMARY KEY,
  uid   text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  exp   timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS ble_tokens_uid_idx ON ble_tokens (uid);
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
	return &Store{pool: pool, sess: map[string]sessEntry{}}, nil
}

func (s *Store) Close() { s.pool.Close() }

// upsertUserFromLinkedIn creates or refreshes the account for a LinkedIn login
// and returns our internal uid plus whether this login created a brand-new
// account (so the caller can send a welcome email). The uid is stable.
// upsertUser finds or creates the account for a provider login. A new
// provider subject joins an existing account only when the provider vouches
// for the email AND that account's email is verified too; otherwise anyone
// could claim an account by registering its address somewhere unverified.
func (s *Store) upsertUser(ctx context.Context, p *provider, in userinfo) (uid string, created bool, err error) {
	col := p.col // fixed per provider, never user input
	if in.EmailVerified && in.Email != "" {
		err = s.pool.QueryRow(ctx, `
			UPDATE users SET `+col+` = $1 WHERE id = (
			  SELECT id FROM users WHERE lower(email) = lower($2) AND email_verified AND `+col+` IS NULL
			  AND NOT EXISTS (SELECT 1 FROM users WHERE `+col+` = $1)
			  ORDER BY created_at LIMIT 1)
			RETURNING id`, in.Sub, in.Email).Scan(&uid)
		if err == nil {
			return uid, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", false, err
		}
	}
	prefix := map[string]string{"linkedin": "li_", "google": "g_"}[p.name]
	// A returning user keeps the email they have (they may have changed and verified
	// another); the provider's "verified" only counts for that same address.
	// RETURNING (xmax = 0): true for a freshly inserted row, false for an update.
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (id, `+col+`, email, email_verified, email_verified_at)
		VALUES ($1, $2, $3, $4, CASE WHEN $4 THEN now() END)
		ON CONFLICT (`+col+`) DO UPDATE SET
		  email = CASE WHEN users.email = '' THEN EXCLUDED.email ELSE users.email END,
		  email_verified = users.email_verified OR (EXCLUDED.email_verified AND lower(users.email) IN ('', lower(EXCLUDED.email))),
		  email_verified_at = CASE WHEN NOT users.email_verified AND EXCLUDED.email_verified
		    AND lower(users.email) IN ('', lower(EXCLUDED.email)) THEN now() ELSE users.email_verified_at END
		RETURNING id, (xmax = 0)`,
		prefix+in.Sub, in.Sub, in.Email, in.EmailVerified).Scan(&uid, &created)
	return uid, created, err
}

// emailUser signs in by an address the person just proved they read: the oldest
// account with that address verified, else a new email-only account.
func (s *Store) emailUser(ctx context.Context, email string) (uid string, created bool, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE lower(email) = $1 AND email_verified ORDER BY created_at LIMIT 1`, email).Scan(&uid)
	if !errors.Is(err, pgx.ErrNoRows) {
		return uid, false, err
	}
	id, err := randToken(12)
	if err != nil {
		return "", false, err
	}
	uid = "em_" + id
	_, err = s.pool.Exec(ctx,
		`INSERT INTO users (id, email, email_verified, email_verified_at) VALUES ($1, $2, true, now())`, uid, email)
	return uid, err == nil, err
}

// hasUserWithEmail reports whether any account already exists with this email address.
func (s *Store) hasUserWithEmail(ctx context.Context, email string) (bool, error) {
	var uid string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE lower(email) = $1 LIMIT 1`, email).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// seedProfile pre-fills a new profile with the sign-in provider's name. An
// existing profile is never touched.
func (s *Store) seedProfile(ctx context.Context, uid, name string) error {
	if name == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO profiles (uid, name, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (uid) DO NOTHING`, uid, name)
	return err
}

func (s *Store) getUser(ctx context.Context, uid string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, email_verified FROM users WHERE id = $1`, uid).
		Scan(&u.ID, &u.Email, &u.Verified)
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

// getProfiles loads several profiles in one query; uids without one are absent.
func (s *Store) getProfiles(ctx context.Context, uids []string) (map[string]Profile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT uid, name, role, look, photo, updated_at FROM profiles WHERE uid = ANY($1)`, uids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]Profile, len(uids))
	for rows.Next() {
		var uid string
		var p Profile
		if err := rows.Scan(&uid, &p.Name, &p.Role, &p.Look, &p.Photo, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out[uid] = p
	}
	return out, rows.Err()
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

// upsertPresences writes many positions in one statement: one array per column
// (UNNEST), so the statement text and parameter count don't grow with the batch.
// Users deleted since the fix are skipped (the join), so one gone account can't
// fail the batch on the foreign key. An older fix never overwrites a newer one.
func (s *Store) upsertPresences(ctx context.Context, ps []Pres) error {
	n := len(ps)
	uids, cells := make([]string, n), make([]string, n)
	lats, lngs := make([]float64, n), make([]float64, n)
	accs, ts := make([]int32, n), make([]time.Time, n)
	for i, p := range ps {
		uids[i], cells[i], lats[i], lngs[i], accs[i], ts[i] = p.UID, p.Cell, p.Lat, p.Lng, int32(p.Acc), p.T
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO presence (uid, cell, lat, lng, acc, t)
		SELECT v.uid, v.cell, v.lat, v.lng, v.acc, v.t
		FROM unnest($1::text[], $2::text[], $3::float8[], $4::float8[], $5::int[], $6::timestamptz[])
		  AS v(uid, cell, lat, lng, acc, t)
		JOIN users u ON u.id = v.uid
		ON CONFLICT (uid) DO UPDATE SET
		  cell = EXCLUDED.cell, lat = EXCLUDED.lat,
		  lng = EXCLUDED.lng, acc = EXCLUDED.acc, t = EXCLUDED.t
		WHERE presence.t <= EXCLUDED.t`,
		uids, cells, lats, lngs, accs, ts)
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

type bleStored struct {
	tok, uid string
	exp      time.Time
}

// saveBLE stores a user's current and previous token (with its shortened grace)
// and drops older ones, in one statement.
func (s *Store) saveBLE(ctx context.Context, uid string, r bleRow) error {
	_, err := s.pool.Exec(ctx, `
		WITH d AS (DELETE FROM ble_tokens WHERE uid = $1 AND token <> $2 AND token <> $3),
		     u AS (UPDATE ble_tokens SET exp = $5 WHERE uid = $1 AND token = $3)
		INSERT INTO ble_tokens (token, uid, exp) VALUES ($2, $1, $4)
		ON CONFLICT (token) DO UPDATE SET exp = EXCLUDED.exp`,
		uid, r.cur, r.prev, r.curExp, r.prevExp)
	return err
}

func (s *Store) deleteBLE(ctx context.Context, uid string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ble_tokens WHERE uid = $1`, uid)
	return err
}

// loadBLE returns unexpired tokens, oldest expiry first (each user's latest comes last).
func (s *Store) loadBLE(ctx context.Context) ([]bleStored, error) {
	rows, err := s.pool.Query(ctx, `SELECT token, uid, exp FROM ble_tokens WHERE exp > now() ORDER BY exp`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bleStored
	for rows.Next() {
		var b bleStored
		if err := rows.Scan(&b.tok, &b.uid, &b.exp); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// loadRecentPresence rebuilds the in-memory presence set on startup so a
// backend restart doesn't drop everyone off the map.
func (s *Store) loadRecentPresence(ctx context.Context) (map[string]*Pres, error) {
	cutoff := time.Now().Add(-freshFor)
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
	s.sessMu.Lock()
	e, ok := s.sess[h]
	s.sessMu.Unlock()
	if ok && time.Since(e.at) < sessionCacheTTL {
		return e.uid, nil
	}
	var uid string
	var bump bool
	err := s.pool.QueryRow(ctx, `
		SELECT uid, seen_at < now() - interval '1 day' FROM sessions
		WHERE token = $1 AND seen_at > now() - interval '90 days'`, h).Scan(&uid, &bump)
	if errors.Is(err, pgx.ErrNoRows) {
		s.sessMu.Lock()
		delete(s.sess, h)
		s.sessMu.Unlock()
		return "", nil
	}
	if err != nil {
		return "", err // never cached: a DB error is a 503, and the next call asks again
	}
	if bump {
		_, _ = s.pool.Exec(ctx, `UPDATE sessions SET seen_at = now() WHERE token = $1`, h)
	}
	s.sessMu.Lock()
	s.sess[h] = sessEntry{uid: uid, at: time.Now()}
	s.sessMu.Unlock()
	return uid, nil
}

func (s *Store) deleteSession(ctx context.Context, token string) error {
	h := hashToken(token)
	s.sessMu.Lock()
	delete(s.sess, h) // before the DELETE: no request may pass on the cached copy after logout
	s.sessMu.Unlock()
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, h)
	return err
}

// forgetSessions drops every cached session of a user (account deleted).
func (s *Store) forgetSessions(uid string) {
	s.sessMu.Lock()
	for h, e := range s.sess {
		if e.uid == uid {
			delete(s.sess, h)
		}
	}
	s.sessMu.Unlock()
}

// purgeSessions drops expired cache entries (called once a minute).
func (s *Store) purgeSessions() {
	s.sessMu.Lock()
	for h, e := range s.sess {
		if time.Since(e.at) >= sessionCacheTTL {
			delete(s.sess, h)
		}
	}
	s.sessMu.Unlock()
}

// deleteAccount removes the user and everything that cascades from it.
func (s *Store) deleteAccount(ctx context.Context, uid string) error {
	s.forgetSessions(uid)
	_, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid)
	s.forgetSessions(uid) // a request that cached the session mid-delete
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
func (s *Store) upsertDevice(ctx context.Context, uid, pushToken, platform, session, deviceID string) error {
	if platform == "" {
		platform = "android"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO devices (push_token, uid, platform, session, device_id, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), now())
		ON CONFLICT (push_token) DO UPDATE SET
		  uid = EXCLUDED.uid, platform = EXCLUDED.platform, session = EXCLUDED.session,
		  device_id = COALESCE(EXCLUDED.device_id, devices.device_id), updated_at = now()`,
		pushToken, uid, platform, hashToken(session), deviceID)
	return err
}

// usageTZ: the day an open counts on. ponytail: one zone for everyone, per-user zones if users go global.
const usageTZ = "Asia/Kolkata"

// recordOpen counts an app open for today. Calls within 30 s are one open (the app
// reports both its start and its first resume).
func (s *Store) recordOpen(ctx context.Context, uid, platform string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO app_opens (uid, day, platform) VALUES ($1, (now() AT TIME ZONE '`+usageTZ+`')::date, $2)
		ON CONFLICT (uid, day) DO UPDATE SET
		  opens = app_opens.opens + CASE WHEN app_opens.last_at < now() - interval '30 seconds' THEN 1 ELSE 0 END,
		  platform = EXCLUDED.platform, last_at = now()`, uid, platform)
	return err
}

type UsageRow struct {
	UID      string    `json:"uid"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Opens    int       `json:"opens"`
	Platform string    `json:"platform"`
	FirstAt  time.Time `json:"firstAt"`
	LastAt   time.Time `json:"lastAt"`
}

type UsageDay struct {
	Day   string `json:"day"`
	Users int    `json:"users"`
	Opens int    `json:"opens"`
}

// usage lists who opened the app on day (YYYY-MM-DD, "" = today), most opens first,
// and the totals for the last 14 days.
func (s *Store) usage(ctx context.Context, day string) (string, []UsageRow, []UsageDay, error) {
	if day == "" {
		if err := s.pool.QueryRow(ctx, `SELECT to_char((now() AT TIME ZONE '`+usageTZ+`')::date, 'YYYY-MM-DD')`).Scan(&day); err != nil {
			return "", nil, nil, err
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT o.uid, COALESCE(p.name, ''), u.email, o.opens, o.platform, o.first_at, o.last_at
		FROM app_opens o JOIN users u ON u.id = o.uid LEFT JOIN profiles p ON p.uid = o.uid
		WHERE o.day = $1::date ORDER BY o.opens DESC, o.last_at DESC LIMIT 1000`, day)
	if err != nil {
		return "", nil, nil, err
	}
	defer rows.Close()
	users := []UsageRow{}
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.UID, &r.Name, &r.Email, &r.Opens, &r.Platform, &r.FirstAt, &r.LastAt); err != nil {
			return "", nil, nil, err
		}
		users = append(users, r)
	}
	if err := rows.Err(); err != nil {
		return "", nil, nil, err
	}
	drows, err := s.pool.Query(ctx, `
		SELECT to_char(day, 'YYYY-MM-DD'), count(*), sum(opens) FROM app_opens
		WHERE day > (now() AT TIME ZONE '`+usageTZ+`')::date - 14 GROUP BY day ORDER BY day DESC`)
	if err != nil {
		return "", nil, nil, err
	}
	defer drows.Close()
	days := []UsageDay{}
	for drows.Next() {
		var d UsageDay
		if err := drows.Scan(&d.Day, &d.Users, &d.Opens); err != nil {
			return "", nil, nil, err
		}
		days = append(days, d)
	}
	return day, users, days, drows.Err()
}

// deleteSessionDevices stops pushes to the devices a session registered (logout).
// Other devices still signed in to the account keep theirs.
// Rows from before devices carried a session (NULL) go too: the account's other
// phones register again the next time they open the app.
func (s *Store) deleteSessionDevices(ctx context.Context, uid, session string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE session = $1 OR (uid = $2 AND session IS NULL)`, hashToken(session), uid)
	return err
}

func (s *Store) deleteDevice(ctx context.Context, uid, pushToken string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE push_token = $1 AND uid = $2`, pushToken, uid)
	return err
}

// pushTokensForUser returns every FCM token registered to a user, so a push
// sender can fan a notification out to all their installs. iPhone location-push
// tokens are APNs tokens: FCM would reject them (and send() would delete the row).
func (s *Store) pushTokensForUser(ctx context.Context, uid string) ([]string, error) {
	return s.tokens(ctx, `SELECT push_token FROM devices WHERE uid = $1 AND platform <> 'ioslocation'`, uid)
}

// locationTokensForUser returns the user's iPhone location-push tokens (APNs).
func (s *Store) locationTokensForUser(ctx context.Context, uid string) ([]string, error) {
	return s.tokens(ctx, `SELECT push_token FROM devices WHERE uid = $1 AND platform = 'ioslocation'`, uid)
}

func (s *Store) tokens(ctx context.Context, q, uid string) ([]string, error) {
	rows, err := s.pool.Query(ctx, q, uid)
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
		  (SELECT count(*) FROM devices WHERE platform <> 'ioslocation'),
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

// listUsers returns the newest users; q (optional) narrows it to names, emails
// or uids containing it, across every account.
func (s *Store) listUsers(ctx context.Context, limit int, q string) ([]AdminUser, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(q)) + "%"
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.email_verified, u.created_at,
		       COALESCE(p.name,''), COALESCE(p.role,''),
		       COALESCE(p.name,'') <> '' AND COALESCE(p.role,'') <> '' AND COALESCE(p.look,'') <> '' AND COALESCE(p.photo,'') <> '',
		       (SELECT count(*) FROM devices d WHERE d.uid = u.id AND d.platform <> 'ioslocation')
		FROM users u LEFT JOIN profiles p ON p.uid = u.id
		WHERE $2 = '%%' OR u.email ILIKE $2 OR p.name ILIKE $2 OR u.id ILIKE $2
		ORDER BY u.created_at DESC LIMIT $1`, limit, like)
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

// uidsForEmail returns every account with this email (an unverified address can
// sit on more than one), for admin notifications addressed by email.
func (s *Store) uidsForEmail(ctx context.Context, email string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, strings.TrimSpace(email))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
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

// claimNotification marks a notification sent before it is pushed, and reports
// whether this caller won it. deliver() and a device registration's catch-up can
// run at once: only one of them may push it (two pushes = the doubled welcome).
func (s *Store) claimNotification(ctx context.Context, id int64) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE notifications SET sent_at = now() WHERE id = $1 AND sent_at IS NULL`, id)
	return err == nil && tag.RowsAffected() == 1, err
}

// unclaimNotification hands a notification back when no device took it, so the
// next device registration delivers it.
func (s *Store) unclaimNotification(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE notifications SET sent_at = NULL WHERE id = $1`, id)
	return err
}

// claimUndelivered claims a user's notifications that were never pushed and are
// still unread (e.g. a welcome created before any device existed), in one
// statement, so two registrations at once can't both get the same row.
func (s *Store) claimUndelivered(ctx context.Context, uid string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		WITH c AS (UPDATE notifications SET sent_at = now() WHERE id IN (
			SELECT id FROM notifications
			WHERE uid = $1 AND sent_at IS NULL AND read_at IS NULL
			ORDER BY created_at ASC LIMIT $2 FOR UPDATE SKIP LOCKED)
		RETURNING id, title, body, data, created_at, read_at)
		SELECT * FROM c ORDER BY created_at ASC`, uid, limit) // RETURNING alone loses the order
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
		`DELETE FROM ble_tokens WHERE exp < now()`,
		`DELETE FROM app_opens WHERE day < now()::date - 180`,
		`DELETE FROM devices WHERE session IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sessions WHERE sessions.token = devices.session)`,
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
