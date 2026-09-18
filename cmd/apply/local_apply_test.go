package apply_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestCRUDManualApplyCreatesSQLiteAndPrunes(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("TLD_DATA_DIR", dataDir)
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace", "--yaml-only")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service", "--yaml-only")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database", "--yaml-only")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads", "--yaml-only")
	cmd.MustRunCmd(t, dir, "apply", "--force", "--target", "local", "--data-dir", dataDir)

	db := openLocalDB(t, dataDir)
	assertCount(t, db, "elements", 3)
	assertCount(t, db, "views", 2)
	assertCount(t, db, "placements", 3)
	assertCount(t, db, "connectors", 1)

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	if ws.Meta == nil || len(ws.Meta.Elements) != 3 || len(ws.Meta.Views) != 1 || len(ws.Meta.Connectors) != 1 {
		t.Fatalf("metadata not updated: %+v", ws.Meta)
	}
	lockFile, err := workspace.LoadLockFile(dir)
	if err != nil {
		t.Fatalf("load lock file: %v", err)
	}
	if lockFile == nil || lockFile.Metadata == nil || len(lockFile.Metadata.Elements) != 3 {
		t.Fatalf("lockfile metadata not updated: %+v", lockFile)
	}

	cmd.MustRunCmd(t, dir, "remove", "connector", "--view", "platform", "--from", "api", "--to", "db", "--yaml-only")
	cmd.MustRunCmd(t, dir, "apply", "--force", "--target", "local", "--data-dir", dataDir)
	assertCount(t, db, "connectors", 0)

	cmd.MustRunCmd(t, dir, "remove", "element", "db", "--yaml-only")
	cmd.MustRunCmd(t, dir, "apply", "--force", "--target", "local", "--data-dir", dataDir)
	assertCount(t, db, "elements", 2)
	assertCount(t, db, "views", 2)
}

func TestAddAutoAppliesByDefaultAndYamlOnlyOptsOut(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("TLD_DATA_DIR", dataDir)
	t.Setenv("TLD_APPLY_TARGET", "local")
	cmd.MustInitWorkspace(t, dir)

	// Default: synchronous auto-apply creates the local DB instantly.
	stdout, _, err := cmd.RunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	if err != nil {
		t.Fatalf("add with auto-apply: %v", err)
	}
	if !strings.Contains(stdout, "applied:") {
		t.Fatalf("expected immediate apply feedback, got %q", stdout)
	}
	if _, err := os.Stat(localserver.DatabasePath(dataDir)); err != nil {
		t.Fatalf("add should auto-apply to local DB by default: %v", err)
	}

	// Opt-out: --yaml-only leaves the DB untouched.
	yamlDir := t.TempDir()
	yamlDataDir := t.TempDir()
	t.Setenv("TLD_DATA_DIR", yamlDataDir)
	cmd.MustInitWorkspace(t, yamlDir)
	cmd.MustRunCmd(t, yamlDir, "add", "API", "--ref", "api", "--kind", "service", "--yaml-only")
	if _, err := os.Stat(localserver.DatabasePath(yamlDataDir)); !os.IsNotExist(err) {
		t.Fatalf("add --yaml-only should not create local DB, stat error: %v", err)
	}

	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	remoteDir := t.TempDir()
	cmd.MustInitWorkspace(t, remoteDir)
	cmd.WriteConfig(t, remoteDir, serverURL, "remote-key")

	cmd.MustRunCmd(t, remoteDir, "add", "API", "--ref", "api", "--kind", "service", "--yaml-only")

	svc.Mu.Lock()
	defer svc.Mu.Unlock()
	if svc.LastRequest != nil {
		t.Fatal("add --yaml-only should only update YAML; expected no remote apply request")
	}
}

func openLocalDB(t *testing.T, dataDir string) *sql.DB {
	t.Helper()
	sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqliteStore.Legacy().Close() })
	return sqliteStore.DB()
}

func assertCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	queryByTable := map[string]string{
		"elements":   "SELECT COUNT(*) FROM elements",
		"views":      "SELECT COUNT(*) FROM views",
		"placements": "SELECT COUNT(*) FROM placements",
		"connectors": "SELECT COUNT(*) FROM connectors",
	}
	query, ok := queryByTable[table]
	if !ok {
		t.Fatalf("unknown table %q", table)
	}
	var got int
	if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func TestApplyLocalTargetUsesDataDirFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TLD_DATA_DIR", t.TempDir())
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--yaml-only")

	cmd.MustRunCmd(t, dir, "apply", "--force", "--target", "local", "--data-dir", dataDir)

	sqliteStore, err := store.Open(filepath.Join(dataDir, "tld.db"), assets.FS)
	if err != nil {
		t.Fatalf("expected local db in data dir: %v", err)
	}
	defer func() { _ = sqliteStore.Legacy().Close() }()
	assertCount(t, sqliteStore.DB(), "elements", 1)
}

func TestApplyLocalTargetPrintsServeCommandWhenServerIsStopped(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--yaml-only")

	stdout, _, err := cmd.RunCmd(t, dir, "apply", "--force", "--target", "local", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("apply local: %v", err)
	}
	if !strings.Contains(stdout, "Target:") || !strings.Contains(stdout, "local") {
		t.Fatalf("stdout %q does not contain local target", stdout)
	}
	if !strings.Contains(stdout, "Start app:") || !strings.Contains(stdout, "tld serve --data-dir "+dataDir) {
		t.Fatalf("stdout %q does not contain serve command", stdout)
	}
}

func TestApplyLocalTargetPrintsRunningServerURL(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--yaml-only")
	if err := localserver.SaveProcessRegistry(localserver.ProcessRegistry{Processes: []localserver.ProcessRecord{
		{Kind: localserver.ProcessKindServer, PID: os.Getpid(), DataDir: dataDir, Addr: "127.0.0.1:9999"},
	}}); err != nil {
		t.Fatalf("save process registry: %v", err)
	}

	stdout, _, err := cmd.RunCmd(t, dir, "apply", "--force", "--target", "local", "--data-dir", dataDir)
	if err != nil {
		t.Fatalf("apply local: %v", err)
	}
	if !strings.Contains(stdout, "View at:") || !strings.Contains(stdout, "http://127.0.0.1:9999") {
		t.Fatalf("stdout %q does not contain running server URL", stdout)
	}
}
