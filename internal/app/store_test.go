package app

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureSQLiteDBEnablesBusyTimeoutAndWAL(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := configureSQLiteDB(db); err != nil {
		t.Fatal(err)
	}

	var busyTimeout int
	if err := db.QueryRow(`PRAGMA busy_timeout;`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode;`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}
