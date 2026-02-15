package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// DBSchemaVersion is the current database schema version.
// Bump this when adding migrations that change the schema.
const DBSchemaVersion = 1

// alterColumn runs an ALTER TABLE ADD COLUMN and silently ignores
// "duplicate column name" errors, making the migration idempotent.
func alterColumn(db *sql.DB, stmt string) error {
	_, err := db.Exec(stmt)
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}

// migrations is an ordered list of SQL statements applied to the database.
// Each statement is idempotent (uses IF NOT EXISTS where possible).
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS repos (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		owner            TEXT NOT NULL,
		name             TEXT NOT NULL,
		poll_interval_ms INTEGER DEFAULT 5000,
		last_sync_at     TEXT,
		issues_etag      TEXT DEFAULT '',
		created_at       TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(owner, name)
	)`,

	`CREATE TABLE IF NOT EXISTS issues (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		repo_id     INTEGER NOT NULL REFERENCES repos(id),
		github_id   INTEGER,
		title       TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'open',
		priority    INTEGER NOT NULL DEFAULT 2,
		issue_type  TEXT NOT NULL DEFAULT 'task',
		description TEXT DEFAULT '',
		owner       TEXT DEFAULT '',
		labels      TEXT DEFAULT '[]',
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL,
		closed_at   TEXT
	)`,

	`CREATE INDEX IF NOT EXISTS idx_issues_repo_status ON issues(repo_id, status)`,
	`CREATE INDEX IF NOT EXISTS idx_issues_repo_priority ON issues(repo_id, priority)`,
	`CREATE INDEX IF NOT EXISTS idx_issues_github_id ON issues(repo_id, github_id)`,

	`CREATE TABLE IF NOT EXISTS events (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		repo_id             INTEGER NOT NULL REFERENCES repos(id),
		github_comment_id   INTEGER,
		issue_id            INTEGER NOT NULL,
		github_issue_number INTEGER,
		timestamp           TEXT NOT NULL,
		action              TEXT NOT NULL,
		payload             TEXT NOT NULL,
		agent               TEXT DEFAULT '',
		synced              INTEGER DEFAULT 0,
		created_at          TEXT NOT NULL DEFAULT (datetime('now'))
	)`,

	`CREATE INDEX IF NOT EXISTS idx_events_repo_issue ON events(repo_id, issue_id)`,
	`CREATE INDEX IF NOT EXISTS idx_events_synced ON events(synced)`,

	`CREATE TABLE IF NOT EXISTS issue_sync_state (
		repo_id              INTEGER NOT NULL,
		github_issue_number  INTEGER NOT NULL,
		last_comment_id      INTEGER NOT NULL DEFAULT 0,
		last_comment_at      TEXT,
		PRIMARY KEY (repo_id, github_issue_number)
	)`,
}

// alterMigrations are ALTER TABLE statements that are run after the main
// CREATE TABLE migrations. They use alterColumn to be idempotent.
var alterMigrations = []string{
	`ALTER TABLE repos ADD COLUMN issues_since TEXT DEFAULT ''`,
}

// runMigrations applies all migration statements in order.
// It checks the database schema version and refuses to proceed if the
// database was created by a newer binary (to prevent data corruption
// on rollback).
func runMigrations(db *sql.DB) error {
	var dbVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&dbVersion); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if dbVersion > DBSchemaVersion {
		return fmt.Errorf(
			"database schema version %d is newer than this binary supports (max %d); upgrade the binary or use a different database",
			dbVersion, DBSchemaVersion)
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}
	for _, m := range alterMigrations {
		if err := alterColumn(db, m); err != nil {
			return err
		}
	}

	if dbVersion < DBSchemaVersion {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", DBSchemaVersion)); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	}

	return nil
}
