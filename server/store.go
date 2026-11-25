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
func (s *Store) saveProfile(ctx context.Context, uid string, p Profile, photoKey string) (updated time.Time, oldKey string, err error) {
	err = s.db.QueryRow(ctx, `
		with old as (select photo_key from profiles where user_id = $1)
		insert into profiles (user_id, name, role, looking, photo_key) values ($1, $2, $3, $4, $5)
		on conflict (user_id) do update set name = $2, role = $3, looking = $4, photo_key = $5, updated_at = now()
		returning updated_at, coalesce((select photo_key from old), '')`, uid, p.Name, p.Role, p.Looking, photoKey).Scan(&updated, &oldKey)
	return updated, oldKey, err
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
