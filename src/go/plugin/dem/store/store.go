// SPDX-License-Identifier: GPL-3.0-or-later

// Package store owns DEM investigation history. The command owns its lifetime;
// job retirement joins writers and Function calls before closing the handle.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

// Open creates the native history store. An empty path selects a private
// in-memory database for terminal/debug runs, leaving live state untouched.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := ":memory:"
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		dsn = (&url.URL{
			Scheme: "file",
			Path:   abs,
		}).String()
	}
	db, err := sql.Open("sqlite", dsn+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// SQLite has one writer. One connection also keeps a private in-memory
	// database coherent; waiting Function calls remain caller-cancellable.
	db.SetMaxOpenConns(1)
	st := &Store{
		db: db,
	}
	if err := st.migrateRumHistory(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize history: %w", err)
	}
	return st, nil
}
func (s *Store) Close() error { return s.db.Close() }
