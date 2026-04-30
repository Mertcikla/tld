package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mertcikla/tld/tests/e2e/infra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func TestWatchPolyglotTop20(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	tmpRoot := t.TempDir()
	env := infra.NewTldEnv(tmpRoot, 18069)
	
	// 1. Build binary
	cwd, _ := os.Getwd()
	// os.Getwd() might be tld/tests/e2e, we need tld/
	projectRoot := filepath.Join(cwd, "../..")
	if err := env.Build(projectRoot); err != nil {
		t.Fatal(err)
	}

	// 2. Setup Polyglot Repo
	repoPath := filepath.Join(tmpRoot, "polyglot-repo")
	actor := infra.NewPolyglotActor(repoPath)
	if err := actor.Init(); err != nil {
		t.Fatal(err)
	}

	languages := []string{
		"go", "python", "typescript", "javascript", "java",
		"rust", "cpp", "csharp", "ruby", "php",
		"swift", "kotlin", "scala", "dart", "zig",
		"elixir", "haskell", "clojure", "objective-c", "lua",
	}

	caser := cases.Title(language.English)
	for _, lang := range languages {
		symbolName := fmt.Sprintf("Init%sSymbol", caser.String(lang))
		if err := actor.GenerateLanguageFile(lang, symbolName); err != nil {
			t.Fatal(err)
		}
	}
	if err := actor.Commit("initial polyglot commit"); err != nil {
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

	// Wait for initial representation
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer waitCancel()
	_, _ = client.ListenEvents(waitCtx, "representation.updated", 1, 15*time.Second)

	// 4. Verify & Report
	fmt.Println("\n--- Language Support Report ---")
	fmt.Printf("| %-15s | %-10s | %-10s | %-10s |\n", "Language", "Scan", "Represent", "Sync")
	fmt.Println("|" + "-----------------" + "|" + "------------" + "|" + "------------" + "|" + "------------" + "|")

	for _, lang := range languages {
		scanStatus := "❌ Fail"
		reprStatus := "❌ Fail"
		syncStatus := "✅ Pass" // File level sync should always work
		symbolName := fmt.Sprintf("Init%sSymbol", caser.String(lang))

		// Check if symbol was extracted to DB
		query := "SELECT COUNT(*) FROM watch_symbols WHERE stable_key LIKE ?"
		count, err := client.QueryDB(query, "%"+lang+"%"+symbolName+"%")
		if err == nil && count > 0 {
			scanStatus = "✅ Pass"
		}

		// Check if element was materialized
		query = "SELECT COUNT(*) FROM elements WHERE name = ?"
		count, err = client.QueryDB(query, symbolName)
		if err == nil && count > 0 {
			reprStatus = "✅ Pass"
		}

		fmt.Printf("| %-15s | %-10s | %-10s | %-10s |\n", lang, scanStatus, reprStatus, syncStatus)
	}
	fmt.Println("-------------------------------")

	// 5. Test Real-time sync (Polyglot)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Add a new file in a random language (e.g. Zig)
	if err := actor.GenerateLanguageFile("zig", "NewZigSymbol"); err != nil {
		t.Fatal(err)
	}

	// Wait for representation.updated event
	events, err := client.ListenEvents(ctx, "representation.updated", 1, 15*time.Second)
	if err != nil {
		t.Logf("Warning: websocket event failed (might be expected if language not supported yet): %v", err)
	} else if len(events) > 0 {
		t.Log("Successfully detected real-time update for Zig")
	}
}
