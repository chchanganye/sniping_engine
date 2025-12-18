package sqlite

import (
	"context"
	"fmt"
	"strings"
)

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL DEFAULT '',
			mobile TEXT NOT NULL UNIQUE,
			token TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			uuid TEXT NOT NULL DEFAULT '',
			proxy TEXT NOT NULL DEFAULT '',
			cookies_json TEXT NOT NULL DEFAULT '[]',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS targets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			image_url TEXT NOT NULL DEFAULT '',
			item_id INTEGER NOT NULL,
			sku_id INTEGER NOT NULL,
			shop_id INTEGER NOT NULL DEFAULT 0,
			mode TEXT NOT NULL,
			target_qty INTEGER NOT NULL,
			per_order_qty INTEGER NOT NULL,
			rush_at_ms INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL DEFAULT '{}',
			updated_at INTEGER NOT NULL
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	// Backward compatible migrations for existing DBs.
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE accounts ADD COLUMN username TEXT NOT NULL DEFAULT ''`); err != nil {
		// SQLite returns "duplicate column name: username" if it already exists.
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return fmt.Errorf("migrate accounts.username: %w", err)
		}
	}

	if _, err := s.db.ExecContext(ctx, `ALTER TABLE targets ADD COLUMN image_url TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return fmt.Errorf("migrate targets.image_url: %w", err)
		}
	}

	return nil
}
