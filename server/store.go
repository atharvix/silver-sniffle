package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// No location, distance or "who met whom" column exists anywhere in this schema, on purpose.
var schema = []string{
	`create table if not exists users (
		id text primary key,
		created_at timestamptz not null default now(),
		hidden boolean not null default false
	)`,
	`create table if not exists sessions (
		token_sha256 text primary key,
		user_id text not null references users(id) on delete cascade,
		created_at timestamptz not null default now()
	)`,
	// shortcut: push tokens stored as-is (they only let our own Firebase key reach the phone); seal them with a key before launch.
	`create table if not exists devices (
		token text primary key,
		user_id text not null references users(id) on delete cascade,
		platform text not null,
		updated_at timestamptz not null default now()
	)`,
	`create table if not exists profiles (
		user_id text primary key references users(id) on delete cascade,
		name text not null,
		role text not null,
		looking text not null,
		photo_key text not null, -- object key in photo storage (S3/R2), never the image itself
		updated_at timestamptz not null default now()
	)`,
	// When the person confirmed they are 18+ and accepted the terms (required to save a card).
	`alter table users add column if not exists agreed_at timestamptz`,
	// A block hides both people from each other for good. The blocked person is never told.
	`create table if not exists blocks (
		blocker text not null references users(id) on delete cascade,
		blocked text not null references users(id) on delete cascade,
		created_at timestamptz not null default now(),
		primary key (blocker, blocked)
	)`,
	`create table if not exists reports (
		id bigserial primary key,
		reporter text not null references users(id) on delete cascade,
		target text not null references users(id) on delete cascade,
		reason text not null,
		created_at timestamptz not null default now()
	)`,
	// Latest health of each install: what keeps Kinjo alive in the background, or stops it. No positions.
	`create table if not exists device_health (
		install_id text primary key,
		user_id text not null references users(id) on delete cascade,
		platform text not null,
		maker text not null,
		model text not null,
		os text not null,
		app text not null,
		battery_free boolean not null,     -- battery optimisation off for Kinjo ("Unrestricted")
		bg_restricted boolean not null,    -- Android restricts Kinjo in the background
		standby_bucket int not null,       -- Android's app standby group: 10 active ... 45 restricted
		power_save boolean not null,
		location_always boolean not null,
		bluetooth_on boolean not null,
		notifications_on boolean not null,
		last_exit text not null,           -- why Android last stopped Kinjo
		killer_apps text not null,         -- cleaner/booster apps from a fixed list, comma separated
		updated_at timestamptz not null default now()
	)`,
	// One row per person per day they opened the app (India time): daily, weekly, monthly active.
	`create table if not exists app_days (
		user_id text not null references users(id) on delete cascade,
		day date not null,
		primary key (user_id, day)
	)`,
	// Daily totals (matches, wake-ups, sign-ups...): counts only, never who.
	`create table if not exists daily_counts (
		day date not null,
		metric text not null,
		count bigint not null,
		primary key (day, metric)
	)`,
}

// today is the reporting day in India time, where Kinjo runs.
const today = `(now() at time zone 'Asia/Kolkata')::date`

// Health is one install's background-health snapshot.
type Health struct {
	InstallID       string `json:"installId"`
	Platform        string `json:"platform"`
	Maker           string `json:"maker"`
	Model           string `json:"model"`
	OS              string `json:"os"`
	App             string `json:"app"`
	BatteryFree     bool   `json:"batteryFree"`
	BgRestricted    bool   `json:"bgRestricted"`
	StandbyBucket   int    `json:"standbyBucket"`
	PowerSave       bool   `json:"powerSave"`
	LocationAlways  bool   `json:"locationAlways"`
	BluetoothOn     bool   `json:"bluetoothOn"`
	NotificationsOn bool   `json:"notificationsOn"`
	LastExit        string `json:"lastExit"`
	KillerApps      string `json:"killerApps"`
}

// saveHealth keeps the latest snapshot per install and marks today as an active day.
func (s *Store) saveHealth(ctx context.Context, uid string, h Health) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			insert into device_health (install_id, user_id, platform, maker, model, os, app, battery_free, bg_restricted,
				standby_bucket, power_save, location_always, bluetooth_on, notifications_on, last_exit, killer_apps)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			on conflict (install_id) do update set user_id = $2, platform = $3, maker = $4, model = $5, os = $6, app = $7,
				battery_free = $8, bg_restricted = $9, standby_bucket = $10, power_save = $11, location_always = $12,
				bluetooth_on = $13, notifications_on = $14, last_exit = $15, killer_apps = $16, updated_at = now()`,
			h.InstallID, uid, h.Platform, h.Maker, h.Model, h.OS, h.App, h.BatteryFree, h.BgRestricted,
			h.StandbyBucket, h.PowerSave, h.LocationAlways, h.BluetoothOn, h.NotificationsOn, h.LastExit, h.KillerApps)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `insert into app_days (user_id, day) values ($1, `+today+`) on conflict do nothing`, uid)
		return err
	})
}

// addCounts adds to today's totals, all or nothing.
func (s *Store) addCounts(ctx context.Context, counts map[string]int64) error {
	if len(counts) == 0 {
		return nil
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		for metric, n := range counts {
			if _, err := tx.Exec(ctx, `
				insert into daily_counts (day, metric, count) values (`+today+`, $1, $2)
				on conflict (day, metric) do update set count = daily_counts.count + $2`, metric, n); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) exists(ctx context.Context, uid string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `select exists (select 1 from users where id = $1)`, uid).Scan(&ok)
	return ok, err
}

// agree records the 18+ and terms confirmation the first time it is given.
func (s *Store) agree(ctx context.Context, uid string) error {
	_, err := s.db.Exec(ctx, `update users set agreed_at = coalesce(agreed_at, now()) where id = $1`, uid)
	return err
}

// block hides the pair from each other; blocking twice is harmless.
func (s *Store) block(ctx context.Context, blocker, blocked string) error {
	_, err := s.db.Exec(ctx, `insert into blocks (blocker, blocked) values ($1, $2) on conflict do nothing`, blocker, blocked)
	return err
}

func (s *Store) report(ctx context.Context, reporter, target, reason string) error {
	_, err := s.db.Exec(ctx, `insert into reports (reporter, target, reason) values ($1, $2, $3)`, reporter, target, reason)
	return err
}

// blockedWith lists everyone this user blocked or was blocked by.
func (s *Store) blockedWith(ctx context.Context, uid string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select blocked from blocks where blocker = $1 union select blocker from blocks where blocked = $1`, uid)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

const sessionCacheTTL = 5 * time.Minute // phones call every few seconds; this keeps that off the database

type cachedSession struct {
	uid   string
	until time.Time
}

type Store struct {
	db *pgxpool.Pool

	mu       sync.Mutex
	sessions map[string]cachedSession // token hash -> user, successful lookups only
}

func openStore(ctx context.Context, url string) (*Store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	for _, stmt := range schema {
		if _, err := db.Exec(ctx, stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db, sessions: map[string]cachedSession{}}, nil
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// newAccount creates a user and a session; only the token's hash is stored.
// shortcut: anonymous accounts until sign-in (Apple, Google, email code) is built.
func (s *Store) newAccount(ctx context.Context) (uid, token string, err error) {
	uid, token = "u_"+randomString(12), randomString(32)
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `insert into users (id) values ($1)`, uid); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `insert into sessions (token_sha256, user_id) values ($1, $2)`, hashToken(token), uid)
		return err
	})
	return uid, token, err
}

var errNoSession = errors.New("no session")

// userFor resolves a session token. errNoSession means "sign in again"; any other error is the database.
// Database errors are never cached.
func (s *Store) userFor(ctx context.Context, token string) (string, error) {
	hash := hashToken(token)
	s.mu.Lock()
	hit, ok := s.sessions[hash]
	s.mu.Unlock()
	if ok && time.Now().Before(hit.until) {
		return hit.uid, nil
	}
	var uid string
	err := s.db.QueryRow(ctx, `select user_id from sessions where token_sha256 = $1`, hash).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNoSession
	}
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.sessions[hash] = cachedSession{uid, time.Now().Add(sessionCacheTTL)}
	s.mu.Unlock()
	return uid, nil
}

// forgetSessions drops a deleted account's cached sessions so it is refused at once, and expired entries with them.
func (s *Store) forgetSessions(uid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for hash, c := range s.sessions {
		if c.uid == uid || now.After(c.until) {
			delete(s.sessions, hash)
		}
	}
}

// saveProfile stores the card and returns the photo key it replaced ("" if none), to delete from storage.
func (s *Store) saveProfile(ctx context.Context, uid string, p Profile, photoKey string) (oldKey string, err error) {
	err = s.db.QueryRow(ctx, `
		with old as (select photo_key from profiles where user_id = $1)
		insert into profiles (user_id, name, role, looking, photo_key) values ($1, $2, $3, $4, $5)
		on conflict (user_id) do update set name = $2, role = $3, looking = $4, photo_key = $5, updated_at = now()
		returning coalesce((select photo_key from old), '')`, uid, p.Name, p.Role, p.Looking, photoKey).Scan(&oldKey)
	return oldKey, err
}

// profile returns the user's card fields; ok is false when they have no profile or are hidden.
func (s *Store) profile(ctx context.Context, uid string) (p Profile, photoKey string, ok bool, err error) {
	err = s.db.QueryRow(ctx, `
		select p.name, p.role, p.looking, p.photo_key from profiles p join users u on u.id = p.user_id
		where p.user_id = $1 and not u.hidden`, uid).Scan(&p.Name, &p.Role, &p.Looking, &photoKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, "", false, nil
	}
	return p, photoKey, err == nil, err
}

func (s *Store) hidden(ctx context.Context, uid string) (bool, error) {
	var hidden bool
	err := s.db.QueryRow(ctx, `select hidden from users where id = $1`, uid).Scan(&hidden)
	return hidden, err
}

func (s *Store) setHidden(ctx context.Context, uid string, hidden bool) error {
	_, err := s.db.Exec(ctx, `update users set hidden = $2 where id = $1`, uid, hidden)
	return err
}

// deleteAccount removes the user; sessions and profile go with it (on delete cascade).
func (s *Store) deleteAccount(ctx context.Context, uid string) error {
	_, err := s.db.Exec(ctx, `delete from users where id = $1`, uid)
	return err
}

// saveDevice records a phone's push token; a token moving to another account follows it.
func (s *Store) saveDevice(ctx context.Context, uid, token, platform string) error {
	_, err := s.db.Exec(ctx, `
		insert into devices (token, user_id, platform) values ($1, $2, $3)
		on conflict (token) do update set user_id = $2, platform = $3, updated_at = now()`, token, uid, platform)
	return err
}

func (s *Store) deviceTokens(ctx context.Context, uid string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select token from devices where user_id = $1`, uid)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) dropDevice(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `delete from devices where token = $1`, token)
	return err
}
