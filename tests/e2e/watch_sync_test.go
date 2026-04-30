package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mertcikla/tld/tests/e2e/infra"
)

func TestWatchRealTimeSyncAndDebouncing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	tmpRoot := t.TempDir()
	env := infra.NewTldEnv(tmpRoot, 18071)
	
	// 1. Build binary
	cwd, _ := os.Getwd()
	// os.Getwd() might be tld/tests/e2e, we need tld/
	projectRoot := filepath.Join(cwd, "../..")
	if err := env.Build(projectRoot); err != nil {
		t.Fatal(err)
	}

	// 2. Setup Repo
	repoPath := filepath.Join(tmpRoot, "sync-repo")
	actor := infra.NewPolyglotActor(repoPath)
	if err := actor.Init(); err != nil {
		t.Fatal(err)
	}
	if err := actor.GenerateLanguageFile("go", "SyncSymbol"); err != nil {
		t.Fatal(err)
	}
	if err := actor.Commit("initial commit"); err != nil {
		t.Fatal(err)
	}

	// 3. Start Watch
	if err := env.Watch(repoPath); err != nil {
		t.Fatal(err)
	}
	defer env.Cleanup()

	client := infra.NewTldClient(env.Port, env.DataDir)
	if err := client.WaitForStatus(true, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// 4. Test Debouncing: Perform multiple rapid writes
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		_ = actor.AddFile("src/pkg_go/file_SyncSymbol.go", 
			"package pkg\nfunc SyncSymbol() {\n\t// iteration "+string(rune('0'+i))+"\n}\n")
		time.Sleep(100 * time.Millisecond)
	}

	// We expect ONE representation.updated event after debouncing
	events, err := client.ListenEvents(ctx, "representation.updated", 1, 15*time.Second)
	if err != nil {
		t.Fatalf("failed to receive debounced update event: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 debounced event, got %d", len(events))
	}

	// 5. Verify the symbol exists in DB
	query := "SELECT COUNT(*) FROM watch_symbols WHERE stable_key LIKE '%SyncSymbol%'"
	count, err := client.QueryDB(query)
	if err != nil || count == 0 {
		t.Errorf("SyncSymbol missing from DB after sync, count=%d, err=%v", count, err)
	}
}
