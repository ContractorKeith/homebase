// Package store is the only package that touches SQLite. It holds the hub's history:
// hourly token usage, network samples, speed tests and GitHub days.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver, no cgo
)

// Store wraps one SQLite database file.
type Store struct {
	db *sql.DB
}

const schemaVersion = 1

var schema = []string{
	`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS usage (
		machine TEXT NOT NULL, agent TEXT NOT NULL, model TEXT NOT NULL, hour INTEGER NOT NULL,
		input INTEGER NOT NULL, output INTEGER NOT NULL, cache_read INTEGER NOT NULL,
		cache_write INTEGER NOT NULL, reasoning INTEGER NOT NULL, sessions INTEGER NOT NULL,
		PRIMARY KEY (machine, agent, model, hour))`,
	`CREATE TABLE IF NOT EXISTS net_samples (
		machine TEXT NOT NULL, target TEXT NOT NULL, ts INTEGER NOT NULL,
		rtt_ms REAL, jitter_ms REAL, loss_pct REAL,
		PRIMARY KEY (machine, target, ts))`,
	`CREATE TABLE IF NOT EXISTS speed_tests (
		machine TEXT NOT NULL, ts INTEGER NOT NULL, down_mbps REAL, up_mbps REAL, server TEXT NOT NULL,
		PRIMARY KEY (machine, ts))`,
	`CREATE TABLE IF NOT EXISTS github_days (
		repo TEXT NOT NULL, day TEXT NOT NULL, commits INTEGER NOT NULL, additions INTEGER NOT NULL,
		deletions INTEGER NOT NULL, prs_opened INTEGER NOT NULL, prs_merged INTEGER NOT NULL,
		issues INTEGER NOT NULL, reviews INTEGER NOT NULL,
		PRIMARY KEY (repo, day))`,
}

// Open opens or creates the database at path and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer; SQLite serialises anyway
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: schema: %w", err)
		}
	}
	var v int
	err = db.QueryRow(`SELECT CAST(value AS INTEGER) FROM meta WHERE key = 'schema_version'`).Scan(&v)
	switch {
	case err == sql.ErrNoRows:
		_, err = db.Exec(`INSERT INTO meta (key, value) VALUES ('schema_version', ?)`, schemaVersion)
	case err == nil && v > schemaVersion:
		err = fmt.Errorf("store: database schema %d is newer than this binary (%d)", v, schemaVersion)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// tx runs fn in one transaction.
func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	t, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(t); err != nil {
		t.Rollback()
		return err
	}
	return t.Commit()
}

func unix(t time.Time) int64 { return t.UTC().Unix() }

func fromUnix(n int64) time.Time { return time.Unix(n, 0).UTC() }
