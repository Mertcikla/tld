//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun/driver/pgdriver"
)

func TestPostgresIndexE2E(t *testing.T) {
	dsn := os.Getenv("TLD_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("TLD_TEST_POSTGRES_URL not set")
	}
	repoRoot := repositoryRoot(t)
	resetPostgresSchema(t, dsn)

	tmp := t.TempDir()
	src := filepath.Join(tmp, "repo")
	dataDir := filepath.Join(tmp, "data")
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "main.go"), `package main

func main() { println(message()) }
func message() string { return "hello" }
`)
	run(t, src, nil, "git", "init", "-q")
	run(t, src, nil, "git", "config", "user.email", "test@example.com")
	run(t, src, nil, "git", "config", "user.name", "Test")
	run(t, src, nil, "git", "add", ".")
	run(t, src, nil, "git", "commit", "-q", "-m", "init")

	env := postgresEnv(configDir, dsn)
	indexOut := run(t, repoRoot, env, "go", "run", "./cmd/tld", "index", src, "--data-dir", dataDir, "--json", "--map")
	var result struct {
		Snapshot struct {
			Id string `json:"id"`
		} `json:"snapshot"`
		Report struct {
			Facts  int `json:"facts"`
			Chunks int `json:"chunks"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(indexOut), &result); err != nil {
		t.Fatalf("parse index output: %v\n%s", err, indexOut)
	}
	if result.Snapshot.Id == "" || result.Report.Facts < 2 {
		t.Fatalf("index result = %+v:\n%s", result, indexOut)
	}

	db := openPostgres(t, dsn)
	defer func() { _ = db.Close() }()
	assertCountAtLeast(t, db, `SELECT COUNT(*) FROM codeindex_facts`, 2)
	assertCountAtLeast(t, db, `SELECT COUNT(*) FROM codeindex_chunks`, 1)
	assertCountAtLeast(t, db, `SELECT COUNT(*) FROM codeindex_snapshots`, 1)
	assertCountAtLeast(t, db, `SELECT COUNT(*) FROM elements`, 1)
	assertCountAtLeast(t, db, `SELECT COUNT(*) FROM views`, 1)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "../.."))
}

func postgresEnv(configDir, dsn string) []string {
	return append(os.Environ(),
		"TLD_CONFIG_DIR="+configDir,
		"TLD_DB_DRIVER=postgres",
		"TLD_DATABASE_URL="+dsn,
	)
}

func resetPostgresSchema(t *testing.T, dsn string) {
	t.Helper()
	db := openPostgres(t, dsn)
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE`); err != nil {
		t.Fatalf("drop public schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("create public schema: %v", err)
	}
}

func openPostgres(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func assertCountAtLeast(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if got < want {
		t.Fatalf("%s = %d, want at least %d", query, got, want)
	}
}

func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
