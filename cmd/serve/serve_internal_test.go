package serve

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
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
		DBDriver:        "sqlite",
	})

	got := out.String()
	if !strings.Contains(got, "Mode:") || !strings.Contains(got, "foreground") {
		t.Fatalf("missing mode in output: %q", got)
	}
	if !strings.Contains(got, "Server status:") || !strings.Contains(got, "initialized new local data") {
		t.Fatalf("missing initialized data status in output: %q", got)
	}
	if !strings.Contains(got, "Bind address:") || !strings.Contains(got, "127.0.0.1:8060") {
		t.Fatalf("missing bind address in output: %q", got)
	}
	if !strings.Contains(got, "DB:") || !strings.Contains(got, dbPath) {
		t.Fatalf("missing database path in output: %q", got)
	}
	if strings.Contains(got, "Data path:") {
		t.Fatalf("data path should not be printed anymore: %q", got)
	}
	if !strings.Contains(got, "tlDiagram available at:") {
		t.Fatalf("webapp line should be present: %q", got)
	}
	if !strings.Contains(got, "http://127.0.0.1:8060") {
		t.Fatalf("webapp url should be present: %q", got)
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
		DBDriver:  "sqlite",
	})

	got := out.String()
	if !strings.Contains(got, "Mode:") || !strings.Contains(got, "background") {
		t.Fatalf("missing background mode in output: %q", got)
	}
	if !strings.Contains(got, "Server status:") || !strings.Contains(got, "using existing local data") {
		t.Fatalf("missing existing data status in output: %q", got)
	}
	if !strings.Contains(got, "Resources:") || !strings.Contains(got, "2 views, 7 elements, 3 connectors") {
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
		DBDriver:        "sqlite",
	})

	if !strings.Contains(out.String(), "DB:") || !strings.Contains(out.String(), dbPath) {
		t.Fatalf("expected db path even when initializing, got %q", out.String())
	}
	if strings.Contains(out.String(), "DB size:") {
		t.Fatalf("new database should not have size metadata yet: %q", out.String())
	}
}

func TestPrintServeInfoUsesPostgresStatus(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tld.db")
	var out bytes.Buffer
	printServeInfo(&out, "https://app.example.com", serveStatus{
		Mode:      "background",
		Resources: localserver.ResourceCounts{Views: 2, Elements: 7, Connectors: 3},
		BindAddr:  "127.0.0.1:8060",
		DBPath:    dbPath,
		DBDriver:  "postgres",
	})

	got := out.String()
	if !strings.Contains(got, "Server status:") || !strings.Contains(got, "using postgres database") {
		t.Fatalf("missing postgres status in output: %q", got)
	}
	if !strings.Contains(got, "Resources:") || !strings.Contains(got, "2 views, 7 elements, 3 connectors") {
		t.Fatalf("missing postgres resource counts in output: %q", got)
	}
	if !strings.Contains(got, "DB:") || !strings.Contains(got, "postgres") {
		t.Fatalf("missing postgres database label in output: %q", got)
	}
	if strings.Contains(got, dbPath) {
		t.Fatalf("postgres output should not show local sqlite path: %q", got)
	}
}

func TestResolveServeOptionsCarriesSelfHostedURLWithoutChangingBindAddress(t *testing.T) {
	cfg := workspace.DefaultConfig()
	cfg.Serve.PublicURL = "https://app.example.com"
	cfg.Serve.AllowedOrigins = []string{"https://admin.example.com"}

	opts := resolveServeOptions(cfg, "0.0.0.0", "9000")
	if got := localserver.ResolveAddr(opts); got != "0.0.0.0:9000" {
		t.Fatalf("ResolveAddr = %q, want explicit bind address", got)
	}
	if got := localserver.DisplayURL(opts, "0.0.0.0:9000"); got != "https://app.example.com" {
		t.Fatalf("DisplayURL = %q, want public URL", got)
	}
	if len(opts.AllowedOrigins) != 1 || opts.AllowedOrigins[0] != "https://admin.example.com" {
		t.Fatalf("AllowedOrigins = %+v", opts.AllowedOrigins)
	}
}

func TestDatabaseWillBeInitializedChecksLocalDatabase(t *testing.T) {
	dir := t.TempDir()
	cfg := workspace.DefaultConfig()
	if !databaseWillBeInitialized(cfg, dir) {
		t.Fatal("empty data dir should initialize a new database")
	}

	if err := os.WriteFile(localserver.DatabasePath(dir), []byte("sqlite placeholder"), 0o644); err != nil {
		t.Fatalf("write db placeholder: %v", err)
	}
	if databaseWillBeInitialized(cfg, filepath.Clean(dir)) {
		t.Fatal("existing database should be reported as reusable data")
	}
}

func TestDatabaseWillBeInitializedSkipsPostgres(t *testing.T) {
	cfg := workspace.DefaultConfig()
	cfg.Database.Driver = "postgres"

	if databaseWillBeInitialized(cfg, t.TempDir()) {
		t.Fatal("postgres mode should not infer initialization from a missing sqlite file")
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
