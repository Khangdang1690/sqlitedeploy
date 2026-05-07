// Package sqlitex creates and verifies SQLite database files for use with
// Litestream. The single hard requirement Litestream imposes is WAL mode, so
// everything in this package exists to enforce that.
package sqlitex

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

// InitDB ensures a SQLite database exists at path with WAL mode enabled and
// sane defaults applied. Safe to call repeatedly (idempotent).
func InitDB(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open(driverName, path)
	if err != nil {
		return fmt.Errorf("open sqlite at %s: %w", path, err)
	}
	defer db.Close()

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("apply %q: %w", p, err)
		}
	}

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("verify journal_mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("expected journal_mode=wal, got %q", mode)
	}
	return nil
}

// VerifyWAL confirms an existing SQLite database is in WAL mode. Used before
// starting Litestream replication on a database the CLI didn't create.
func VerifyWAL(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("database file %s: %w", path, err)
	}
	db, err := sql.Open(driverName, path)
	if err != nil {
		return err
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return err
	}
	if mode != "wal" {
		return fmt.Errorf("database at %s is in %q mode; Litestream requires WAL. Run: sqlite3 %s 'PRAGMA journal_mode=WAL;'", path, mode, path)
	}
	return nil
}
