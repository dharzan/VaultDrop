package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx connection pool using the provided DSN.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 8
	cfg.MaxConnIdleTime = 5 * time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}

// EnsureSchema creates the documents table if needed. Having the migration in
// code keeps the demo self-contained so docker-compose can bootstrap everything.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	const stmt = `
CREATE TABLE IF NOT EXISTS documents (
	id TEXT PRIMARY KEY,
	file_name TEXT NOT NULL,
	object_key TEXT NOT NULL,
	processed_key TEXT,
	status TEXT NOT NULL,
	content TEXT,
	error_message TEXT,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(status);`
	_, err := pool.Exec(ctx, stmt)
	if err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	return nil
}
