//go:build cliworkflow

package cliworkflow

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/tld/v2/internal/hashids"
	"github.com/uptrace/bun/driver/pgdriver"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tld-workflow-binary-")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "tld")
	tags, err := os.ReadFile("../../.go-build-tags")
	if err != nil {
		_ = os.RemoveAll(dir)
		panic(err)
	}
	build := exec.Command("go", "build", "-tags", strings.TrimSpace(string(tags)), "-o", binary, "./cmd/tld")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		fmt.Fprintf(os.Stderr, "build CLI: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// Both testing.T and rapid.T implement this. Each Rapid attempt owns its files
// and cleanup, so shrinking never inherits state from a previous attempt.
type testContext interface {
	Fatalf(string, ...any)
	Logf(string, ...any)
	Cleanup(func())
	Failed() bool
}

type harness struct {
	t              testContext
	dir, workspace string
	env            []string
	local, target  graph
	ids            identities
	baselineIDs    identities
	baseline       string
	dirty          bool
	trace          []string
	targetName     string
	dsn            string
	faults         *rpcFaults
}

func newHarness(t testContext) *harness {
	return newHarnessFor(t, "sqlite")
}

func newHarnessFor(t testContext, backend string) *harness {
	dir, err := os.MkdirTemp("", "tld-workflow-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	h := &harness{t: t, dir: dir, workspace: filepath.Join(dir, ".tld"), local: emptyGraph(), target: emptyGraph(), ids: newIdentities(), targetName: "local"}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TLD_") {
			h.env = append(h.env, entry)
		}
	}
	h.env = append(h.env, "TLD_CONFIG_DIR="+filepath.Join(dir, "config"), "TLD_DATA_DIR="+filepath.Join(dir, "data"), "TLD_DB_DRIVER=sqlite", "TLD_UPDATES_AUTO=false", "NO_COLOR=1")
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("workflow trace:\n%s", strings.Join(h.trace, "\n"))
			// Opt in to retaining workspaces (including SQLite and YAML) during
			// local debugging. Rapid also saves its minimized replay separately.
			if dest := os.Getenv("TLD_WORKFLOW_ARTIFACT_DIR"); dest != "" {
				if err := os.MkdirAll(dest, 0o700); err == nil {
					_ = os.WriteFile(filepath.Join(dir, "trace.txt"), []byte(strings.Join(h.trace, "\n")), 0o600)
					path := filepath.Join(dest, filepath.Base(dir))
					if err := os.Rename(dir, path); err == nil {
						t.Logf("failure artifacts: %s", path)
						return
					}
				}
			}
		}
		_ = os.RemoveAll(dir)
	})
	h.command(false, "init", dir)
	// Bootstrap the real database and an empty synchronized cache.
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o700); err != nil {
		h.fail("create data dir: %v", err)
	}
	h.configureBackend(backend)
	h.command(false, "pull", "--force")
	h.acceptBaseline()
	h.check()
	return h
}

// command always starts a new process, with explicit destinations and a bounded
// lifetime. JSON status and the OS exit status must agree.
func (h *harness) command(wantFailure bool, args ...string) map[string]any {
	return h.commandInput(wantFailure, "", args...)
}

func (h *harness) runRaw(wdir, format, input string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	flags := []string{"--workspace", wdir, "--target", h.targetName, "--data-dir", filepath.Join(h.dir, "data"), "--format", format, "--compact"}
	c := exec.CommandContext(ctx, binary, append(flags, args...)...)
	c.Dir, c.Env = h.dir, h.env
	c.Stdin = strings.NewReader(input)
	var out, stderr bytes.Buffer
	c.Stdout, c.Stderr = &out, &stderr
	err := c.Run()
	h.trace = append(h.trace, fmt.Sprintf("%03d tld %q\nexit=%v stdout=%s stderr=%s", len(h.trace)+1, args, err, out.String(), stderr.String()))
	if ctx.Err() != nil {
		h.fail("command timeout: %q", args)
	}
	return out.String(), stderr.String(), err
}

func (h *harness) commandInput(wantFailure bool, input string, args ...string) map[string]any {
	out, stderr, err := h.runRaw(h.dir, "json", input, args...)
	if (err != nil) != wantFailure {
		h.fail("command %q: error=%v, expected failure=%t\nstdout=%s\nstderr=%s", args, err, wantFailure, out, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		h.fail("command %q produced invalid JSON: %v\n%s", args, err, out)
	}
	if (result["status"] == "error") != wantFailure {
		h.fail("command %q: JSON status disagrees with exit status: %s", args, out)
	}
	return result
}

func (h *harness) fail(format string, args ...any) {
	// Rapid compares failure messages while shrinking. Keep volatile paths,
	// timestamps, and command output in diagnostics, outside the stable error.
	h.t.Logf(format, args...)
	h.t.Fatalf("workflow assertion failed: %s", strings.SplitN(format, "\n", 2)[0])
}

func (h *harness) readYAML(name string, dest any) {
	b, err := os.ReadFile(filepath.Join(h.workspace, name))
	if err != nil {
		h.fail("read %s: %v", name, err)
	}
	if err := yaml.Unmarshal(b, dest); err != nil {
		h.fail("parse %s: %v", name, err)
	}
}

func (h *harness) lock() lockFile {
	var lock lockFile
	h.readYAML(".tld.lock", &lock)
	return lock
}

func (h *harness) db() *sql.DB {
	if h.dsn != "" {
		return sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(h.dsn)))
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(h.dir, "data", "tld.db"))+"?mode=ro")
	if err != nil {
		h.fail("open observer: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func (h *harness) query(db *sql.DB, query string, visit func(*sql.Rows)) {
	rows, err := db.Query(query)
	if err != nil {
		h.fail("query %s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		visit(rows)
	}
	if err := rows.Err(); err != nil {
		h.fail("query rows: %v", err)
	}
}

func (h *harness) scan(rows *sql.Rows, dest ...any) {
	if err := rows.Scan(dest...); err != nil {
		h.fail("scan: %v", err)
	}
}

// Capture all managed files byte-for-byte and all domain rows, including IDs
// and timestamps. Comparing logical SQL rows avoids WAL/checkpoint noise.
func (h *harness) fingerprint() string {
	files := map[string]string{}
	err := filepath.WalkDir(h.workspace, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[strings.TrimPrefix(path, h.workspace)] = string(b)
		}
		return nil
	})
	if err != nil {
		h.fail("snapshot files: %v", err)
	}
	db := h.db()
	defer func() { _ = db.Close() }()
	tables := []string{"elements", "views", "placements", "connectors", "view_visibility_overrides", "tags"}
	if h.dsn == "" {
		tables = append(tables, "sqlite_sequence")
	}
	for _, table := range tables {
		var records [][]any
		h.query(db, "SELECT * FROM "+table+" ORDER BY 1, 2", func(rows *sql.Rows) {
			cols, err := rows.Columns()
			if err != nil {
				h.fail("columns: %v", err)
			}
			values, pointers := make([]any, len(cols)), make([]any, len(cols))
			for i := range pointers {
				pointers[i] = &values[i]
			}
			h.scan(rows, pointers...)
			records = append(records, values)
		})
		files["sql/"+table] = asJSON(records)
	}
	return asJSON(files)
}

func (h *harness) unchanged(failure bool, args ...string) map[string]any {
	before := h.fingerprint()
	result := h.command(failure, args...)
	if after := h.fingerprint(); before != after {
		h.fail("read/dry-run/rejected command %q changed state\nbefore=%s\nafter=%s", args, before, after)
	}
	h.check()
	return result
}

func asJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}

type metadata struct {
	ID string `yaml:"id"`
}
type metadataSet struct {
	Elements   map[string]metadata `yaml:"elements"`
	Views      map[string]metadata `yaml:"views"`
	Connectors map[string]metadata `yaml:"connectors"`
}
type lockFile struct {
	Hash       string              `yaml:"workspace_hash"`
	Metadata   metadataSet         `yaml:"metadata"`
	Elements   map[string]metadata `yaml:"current_elements"`
	Views      map[string]metadata `yaml:"current_views"`
	Connectors map[string]metadata `yaml:"current_connectors"`
	Resources  struct {
		Elements   int `yaml:"elements"`
		Views      int `yaml:"views"`
		Connectors int `yaml:"connectors"`
	} `yaml:"resources"`
}

func (h *harness) reject(code string, args ...string) {
	result := h.unchanged(true, args...)
	detail, ok := result["error"].(map[string]any)
	if !ok || detail["code"] != code {
		h.fail("expected rejection code %q for %q, got %s", code, args, asJSON(result))
	}
}

type identities struct{ Elements, Views, Connectors map[string]int32 }

func newIdentities() identities {
	return identities{map[string]int32{}, map[string]int32{}, map[string]int32{}}
}

func (h *harness) decodeIDs(m map[string]metadata) map[string]int32 {
	ids := map[string]int32{}
	for ref, meta := range m {
		id, err := hashidlib.Decode(meta.ID)
		if err != nil || id == 0 {
			h.fail("invalid metadata ID %s: %v", ref, err)
		}
		ids[ref] = id
	}
	return ids
}

// Only explicit creation actions may learn newly allocated IDs. Other actions
// must preserve the registry, so a delete/recreate implementation cannot pass.
func (h *harness) learnID(kind, ref string) {
	lock := h.lock()
	current, known := lock.Elements, h.ids.Elements
	switch kind {
	case "view":
		current, known = lock.Views, h.ids.Views
	case "connector":
		current, known = lock.Connectors, h.ids.Connectors
	}
	ids := h.decodeIDs(current)
	id, ok := ids[ref]
	if !ok {
		h.fail("new %s %q has no ID", kind, ref)
	}
	if old, exists := known[ref]; exists && old != id {
		h.fail("%s %q changed ID: %d -> %d", kind, ref, old, id)
	}
	known[ref] = id
}

func (h *harness) acceptBaseline() {
	h.baseline = h.lock().Hash
	h.baselineIDs = newIdentities()
	if err := json.Unmarshal([]byte(asJSON(h.ids)), &h.baselineIDs); err != nil {
		panic(err)
	}
}
