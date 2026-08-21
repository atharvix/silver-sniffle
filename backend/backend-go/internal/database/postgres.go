package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	Pool   *pgxpool.Pool
	logger *slog.Logger
}

func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*DB, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is empty")
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdle
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLife

	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		logger.DebugContext(ctx, "new database connection established")
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Ping database
	pingCtx, cancel := context.WithTimeout(ctx, cfg.DBHealthTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	db := &DB{
		Pool:   pool,
		logger: logger,
	}

	return db, nil
}

func (db *DB) Close() {
	if db.Pool != nil {
		db.logger.Info("closing database connection pool")
		db.Pool.Close()
	}
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

// Migrate runs embedded up migrations
func (db *DB) Migrate(ctx context.Context) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations dir: %w", err)
	}

	var upFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			upFiles = append(upFiles, entry.Name())
		}
	}
	sort.Strings(upFiles)

	// Create schema migrations table if not exists
	_, err = db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	for _, file := range upFiles {
		version := strings.TrimSuffix(file, ".up.sql")
		
		var exists bool
		err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", version, err)
		}

		if exists {
			db.logger.DebugContext(ctx, "migration already applied", slog.String("version", version))
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to start transaction for %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed to apply migration %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", version, err)
		}

		db.logger.InfoContext(ctx, "successfully applied database migration", slog.String("version", version))
	}

	return nil
}

// WithTx executes a function within a transaction with automatic rollback on error
func (db *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && rbErr != pgx.ErrTxClosed {
			db.logger.ErrorContext(ctx, "failed to rollback transaction", slog.String("error", rbErr.Error()))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// PoolStats returns current pool metrics
type PoolStats struct {
	TotalConns        int32
	AcquiredConns     int32
	IdleConns         int32
	ConstructingConns int32
	MaxConns          int32
}

func (db *DB) Stats() PoolStats {
	stat := db.Pool.Stat()
	return PoolStats{
		TotalConns:        stat.TotalConns(),
		AcquiredConns:     stat.AcquiredConns(),
		IdleConns:         stat.IdleConns(),
		ConstructingConns: stat.ConstructingConns(),
		MaxConns:          stat.MaxConns(),
	}
}

// Helper interface for queries that can run on either *pgxpool.Pool or pgx.Tx
type Queryable interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
