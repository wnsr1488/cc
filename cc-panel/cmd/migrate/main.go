package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/example/cc-panel/internal/config"
	"github.com/example/cc-panel/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func main() {
	ctx := context.Background()
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	if err := ensureMigrationTable(ctx, pool); err != nil {
		log.Fatalf("ensure migration table: %v", err)
	}

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		log.Fatalf("list migrations: %v", err)
	}
	sort.Strings(files)

	for _, path := range files {
		name := filepath.Base(path)
		applied, err := isApplied(ctx, pool, name)
		if err != nil {
			log.Fatalf("check migration %s: %v", name, err)
		}
		if applied {
			log.Printf("skip %s", name)
			continue
		}
		if err := applyMigration(ctx, pool, name, path); err != nil {
			log.Fatalf("apply migration %s: %v", name, err)
		}
		log.Printf("applied %s", name)
	}
}

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func ensureMigrationTable(ctx context.Context, db execer) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func isApplied(ctx context.Context, db execer, version string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
	return exists, err
}

func applyMigration(ctx context.Context, pool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}, version, path string) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	sql := strings.TrimSpace(string(sqlBytes))
	if sql == "" {
		return fmt.Errorf("migration is empty")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("exec sql: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return tx.Commit(ctx)
}
