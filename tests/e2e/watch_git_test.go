package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mertcikla/tld/tests/e2e/infra"
)

func TestWatchGitStateTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	tmpRoot := t.TempDir()
	env := infra.NewTldEnv(tmpRoot, 18070)
	
	// 1. Build binary
	cwd, _ := os.Getwd()
	// os.Getwd() might be tld/tests/e2e, we need tld/
	projectRoot := filepath.Join(cwd, "../..")
	if err := env.Build(projectRoot); err != nil {
		t.Fatal(err)
	}

	// 2. Setup Repo with initial commit
	repoPath := filepath.Join(tmpRoot, "git-state-repo")
	actor := infra.NewPolyglotActor(repoPath)
	if err := actor.Init(); err != nil {
		t.Fatal(err)
	}
	if err := actor.GenerateLanguageFile("go", "BaseSymbol"); err != nil {
		t.Fatal(err)
	}
	if err := actor.Commit("base commit"); err != nil {
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

	// 4. Add Untracked file
	if err := actor.GenerateLanguageFile("go", "UntrackedSymbol"); err != nil {
		t.Fatal(err)
	}

	// Wait for rescan
	time.Sleep(2 * time.Second)

	// Verify git:untracked tag in DB
	query := "SELECT COUNT(*) FROM elements WHERE name = 'UntrackedSymbol' AND tags LIKE '%git:untracked%'"
	count, err := client.QueryDB(query)
	if err != nil || count == 0 {
		t.Errorf("expected git:untracked tag on UntrackedSymbol element, got count=%d, err=%v", count, err)
	}

	// 5. Stage the file
	if err := actor.Stage("src/pkg_go/file_UntrackedSymbol.go"); err != nil {
		t.Fatal(err)
	}

	// Wait for rescan
	time.Sleep(2 * time.Second)

	// Verify git:staged tag in DB
	query = "SELECT COUNT(*) FROM elements WHERE name = 'UntrackedSymbol' AND tags LIKE '%git:staged%'"
	count, err = client.QueryDB(query)
	if err != nil || count == 0 {
		t.Errorf("expected git:staged tag on UntrackedSymbol element, got count=%d, err=%v", count, err)
	}

	// 6. Commit and verify tags are removed
	if err := actor.Commit("commit untracked"); err != nil {
		t.Fatal(err)
	}

	// Wait for rescan and version creation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.ListenEvents(ctx, "version.created", 1, 10*time.Second)
	if err != nil {
		t.Logf("Warning: did not receive version.created event: %v", err)
	}

	// Retry check because of async DB updates
	query = "SELECT COUNT(*) FROM elements WHERE name = 'UntrackedSymbol' AND (tags LIKE '%git:staged%' OR tags LIKE '%git:untracked%')"
	success := false
	for i := 0; i < 5; i++ {
		count, err = client.QueryDB(query)
		if err == nil && count == 0 {
			success = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !success {
		t.Errorf("expected git tags to be removed after commit, got count=%d, err=%v", count, err)
		// Debug: print the tags
		var tags string
		dbPath := filepath.Join(env.DataDir, "tld.db")
		db, _ := infra.OpenDB(dbPath)
		_ = db.QueryRow("SELECT tags FROM elements WHERE name = 'UntrackedSymbol'").Scan(&tags)
		t.Logf("Actual tags on UntrackedSymbol: %s", tags)
		_ = db.Close()
	}
}
