package watchcase

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/localserver"
	storepkg "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestRunCaseProducesReviewableObjectDiff(t *testing.T) {
	requireGit(t)
	caseDir := writeBodyEditCase(t)
	opts := runOptions{Config: testConfig()}

	first, err := RunCase(context.Background(), caseDir, ExpectedFile{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Objects) != 1 {
		t.Fatalf("expected one object diff, got %+v", first.Objects)
	}
	object := first.Objects[0]
	if object.Kind != "element" || object.Change != "updated" || object.Review != reviewUnreviewed {
		t.Fatalf("unexpected object diff: %+v", object)
	}
	if !strings.Contains(object.Identity, "CreateUser") {
		t.Fatalf("expected stable function identity, got %+v", object)
	}

	expected := ExpectedFile{Objects: []ExpectedObject{{
		Kind:     object.Kind,
		Identity: object.Identity,
		Change:   object.Change,
		Review:   reviewCorrect,
	}}}
	second, err := RunCase(context.Background(), caseDir, expected, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Objects) != 1 || second.Objects[0].Review != reviewCorrect {
		t.Fatalf("expected saved review to be restored, got %+v", second.Objects)
	}
	if errs := verifyObjects(second.Objects, expected); len(errs) != 0 {
		t.Fatalf("expected verified objects, got %v", errs)
	}
}

func TestReviewAcceptAllSavesGroundTruth(t *testing.T) {
	requireGit(t)
	caseDir := writeBodyEditCase(t)

	var out bytes.Buffer
	if err := reviewCases(context.Background(), strings.NewReader("a\nn\n"), &out, []string{caseDir}, runOptions{Config: testConfig()}); err != nil {
		t.Fatalf("review cases: %v\n%s", err, out.String())
	}
	expected, err := loadExpected(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(expected.Objects) != 1 {
		t.Fatalf("expected one saved object, got %+v\noutput:\n%s", expected.Objects, out.String())
	}
	if expected.Objects[0].Review != reviewCorrect {
		t.Fatalf("expected accept all to mark correct, got %+v", expected.Objects[0])
	}
}

func TestRunCaseMatchesProductionWatchDirtyWorktreeDiffs(t *testing.T) {
	requireGit(t)
	caseDir := writeBodyEditCase(t)
	cfg := testConfig()

	watchcaseResult, err := RunCase(context.Background(), caseDir, ExpectedFile{}, runOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}

	tempRoot := t.TempDir()
	repo := filepath.Join(tempRoot, "repo")
	dataDir := filepath.Join(tempRoot, "data")
	if err := copyDir(filepath.Join(caseDir, "baseline"), repo); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := initBaselineGit(repo); err != nil {
		t.Fatal(err)
	}
	sqliteStore, err := storepkg.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func(db *sql.DB) { _ = db.Close() }(sqliteStore.DB())
	watchStore := watch.NewStore(sqliteStore.DB())
	settings := watch.ResolveSettings(cfg, []string{"go"}, watch.WatcherPoll, "", "", 0, 0, 0, 0, 0)
	embedding := watch.ResolveEmbeddingConfig(cfg, "", "", "", 0)
	events := watch.NewEventQueue()
	ready := make(chan watch.RunnerResult, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, runErr := watch.NewRunner(watchStore).Run(ctx, watch.RunnerOptions{
			Path:              repo,
			Rescan:            true,
			Embedding:         embedding,
			Settings:          settings,
			DataDir:           dataDir,
			Events:            events,
			Ready:             ready,
			PollInterval:      25 * time.Millisecond,
			Debounce:          5 * time.Millisecond,
			HeartbeatInterval: time.Hour,
			SummaryInterval:   time.Hour,
		})
		errCh <- runErr
		events.Close()
	}()
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatalf("watch runner exited before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("watch runner did not become ready")
	}
	if err := runGitApply(repo, filepath.Join(caseDir, "change.patch")); err != nil {
		t.Fatal(err)
	}
	productionDiffs := waitForProductionDiffs(t, watchStore, events)
	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatal(err)
	}
	productionObjects := objectDiffs(productionDiffs, ExpectedFile{})
	if got, want := objectSignatures(productionObjects), objectSignatures(watchcaseResult.Objects); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("watchcase objects differ from production watch dirty-worktree objects\nproduction:\n%s\nwatchcase:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func testConfig() *workspace.Config {
	cfg := workspace.DefaultConfig()
	cfg.Watch.Embedding.Provider = "none"
	cfg.Watch.Languages = []string{"go"}
	cfg.Watch.Watcher = watch.WatcherPoll
	return cfg
}

func waitForProductionDiffs(t *testing.T, store *watch.Store, events *watch.EventQueue) []watch.RepresentationDiff {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-events.Out():
			if !ok {
				t.Fatal("watch events closed before patched representation")
			}
			if event.Type != "representation.updated" || event.RepositoryID == 0 {
				continue
			}
			summary, ok := event.Data.(watch.RepresentResult)
			if !ok || summary.RepresentationHash == "" {
				continue
			}
			diffs, err := store.BuildWatchDiffs(context.Background(), event.RepositoryID, summary.RepresentationHash)
			if err != nil {
				t.Fatal(err)
			}
			var nonRepository []watch.RepresentationDiff
			for _, diff := range diffs {
				if diff.OwnerType != "repository" {
					nonRepository = append(nonRepository, diff)
				}
			}
			if len(nonRepository) > 0 {
				return diffs
			}
		case <-timer.C:
			t.Fatal("timed out waiting for patched production representation")
		}
	}
}

func objectSignatures(objects []ObjectDiff) []string {
	out := make([]string, 0, len(objects))
	for _, object := range objects {
		out = append(out, object.Kind+" "+object.Change+" "+object.Identity)
	}
	return out
}

func writeBodyEditCase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fixture.yaml"), `name: body-edit
baseline: baseline
patch: change.patch
languages:
  - go
`)
	writeFile(t, filepath.Join(dir, "expected.yaml"), "objects: []\n")
	writeFile(t, filepath.Join(dir, "baseline", "go.mod"), `module example.com/watchcase/bodyedit

go 1.22
`)
	writeFile(t, filepath.Join(dir, "baseline", "internal", "service", "user.go"), `package service

import "strings"

type User struct {
	Name string
}

func CreateUser(name string) User {
	clean := strings.TrimSpace(name)
	return User{Name: clean}
}
`)
	writeFile(t, filepath.Join(dir, "change.patch"), `diff --git a/internal/service/user.go b/internal/service/user.go
--- a/internal/service/user.go
+++ b/internal/service/user.go
@@ -6,6 +6,9 @@ type User struct {
 }
 
 func CreateUser(name string) User {
-	clean := strings.TrimSpace(name)
+	clean := strings.ToLower(strings.TrimSpace(name))
+	if clean == "" {
+		clean = "anonymous"
+	}
 	return User{Name: clean}
 }
`)
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for watchcase tests")
	}
}
