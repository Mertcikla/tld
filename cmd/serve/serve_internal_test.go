package serve

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/term"
)

func TestPrintServeInfoIncludesCoreFields(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tld.db")
	var out bytes.Buffer
	printServeInfo(&out, "http://127.0.0.1:8060", serveStatus{
		Mode:            "foreground",
		InitializedData: true,
		BindAddr:        "127.0.0.1:8060",
		DBPath:          dbPath,
	})

	got := out.String()
	if !strings.Contains(got, "Mode:                foreground") {
		t.Fatalf("missing mode in output: %q", got)
	}
	if !strings.Contains(got, "Server status:       initialized new local data") {
		t.Fatalf("missing initialized data status in output: %q", got)
	}
	if !strings.Contains(got, "Bind address:        127.0.0.1:8060") {
		t.Fatalf("missing bind address in output: %q", got)
	}
	if !strings.Contains(got, "DB:                  "+dbPath) {
		t.Fatalf("missing database path in output: %q", got)
	}
	if strings.Contains(got, "Data path:") {
		t.Fatalf("data path should not be printed anymore: %q", got)
	}
	if !strings.Contains(got, "\n\ntlDiagram available at:") {
		t.Fatalf("webapp line should be preceded by a blank line: %q", got)
	}
	if !strings.Contains(got, "tlDiagram available at: http://127.0.0.1:8060\n\nRun 'tld stop'") {
		t.Fatalf("webapp line should be followed by a blank line before stop hint: %q", got)
	}
}

func TestPrintServeInfoIncludesExistingDataCounts(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tld.db")
	if err := os.WriteFile(dbPath, []byte("sqlite placeholder"), 0o644); err != nil {
		t.Fatalf("write db placeholder: %v", err)
	}
	var out bytes.Buffer
	printServeInfo(&out, "http://127.0.0.1:8060", serveStatus{
		Mode:      "background",
		Resources: localserver.ResourceCounts{Views: 2, Elements: 7, Connectors: 3},
		BindAddr:  "127.0.0.1:8060",
		DBPath:    dbPath,
	})

	got := out.String()
	if !strings.Contains(got, "Mode:                background") {
		t.Fatalf("missing background mode in output: %q", got)
	}
	if !strings.Contains(got, "Server status:       using existing local data") {
		t.Fatalf("missing existing data status in output: %q", got)
	}
	if !strings.Contains(got, "Resource counts:     2 views, 7 elements, 3 connectors") {
		t.Fatalf("missing resource counts in output: %q", got)
	}
	if !strings.Contains(got, "DB:") {
		t.Fatalf("missing database line in output: %q", got)
	}
	if strings.Contains(got, "Data path:") {
		t.Fatalf("data path should not be printed anymore: %q", got)
	}
}

func TestPrintServeInfoShowsDBPathWhileInitializing(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tld.db")
	var out bytes.Buffer
	printServeInfo(&out, "http://127.0.0.1:8060", serveStatus{
		Mode:            "foreground",
		InitializedData: true,
		BindAddr:        "127.0.0.1:8060",
		DBPath:          dbPath,
	})

	if !strings.Contains(out.String(), "DB:                  "+dbPath) {
		t.Fatalf("expected db path even when initializing, got %q", out.String())
	}
	if strings.Contains(out.String(), "DB size:") {
		t.Fatalf("new database should not have size metadata yet: %q", out.String())
	}
}

func TestDatabaseWillBeInitializedChecksLocalDatabase(t *testing.T) {
	dir := t.TempDir()
	if !databaseWillBeInitialized(dir) {
		t.Fatal("empty data dir should initialize a new database")
	}

	if err := os.WriteFile(localserver.DatabasePath(dir), []byte("sqlite placeholder"), 0o644); err != nil {
		t.Fatalf("write db placeholder: %v", err)
	}
	if databaseWillBeInitialized(filepath.Clean(dir)) {
		t.Fatal("existing database should be reported as reusable data")
	}
}

func TestFormatHelpersApplyAnsiWhenEnabled(t *testing.T) {
	local := formatLocalPath("/tmp/tld", true)
	if !strings.Contains(local, term.ColorBlue) || !strings.Contains(local, term.ColorReset) {
		t.Fatalf("expected blue local path, got %q", local)
	}

	webapp := formatWebappURL("http://127.0.0.1:8060", true)
	if !strings.Contains(webapp, term.ColorGreen) || !strings.Contains(webapp, term.ColorUnderline) || !strings.Contains(webapp, term.ColorReset) {
		t.Fatalf("expected styled webapp url, got %q", webapp)
	}
}
