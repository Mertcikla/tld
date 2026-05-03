package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tldgit "github.com/mertcikla/tld/internal/git"
	sqlitevec "github.com/viant/sqlite-vec/vec"
	_ "modernc.org/sqlite"
)

func TestMigrationCreatesWatchTablesAndIndexes(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()

	for _, table := range []string{"watch_repositories", "watch_files", "watch_symbols", "watch_references", "watch_scan_runs", "watch_embedding_models", "watch_embeddings", "watch_filter_runs", "watch_filter_decisions", "watch_clusters", "watch_cluster_members", "watch_materialization", "watch_representation_runs", "watch_locks", "watch_versions", "watch_representation_diffs", "watch_version_resources", "workspace_versions"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("missing table %s: %v", table, err)
		}
	}
	for _, index := range []string{"idx_watch_repositories_remote_url", "idx_watch_repositories_repo_root"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&name); err != nil {
			t.Fatalf("missing index %s: %v", index, err)
		}
	}
}

func TestRepresentMaterializesWorkspaceIdempotently(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "cmd/app/main.go", `package main

func Main() {
	helper()
}

func helper() {}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.ElementsCreated == 0 || first.ViewsCreated == 0 {
		t.Fatalf("expected materialized resources, got %+v", first)
	}
	if first.ConnectorsCreated == 0 {
		t.Fatalf("expected symbol connector, got %+v", first)
	}
	countsAfterFirst := workspaceCounts(t, db)

	second, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	if second.RepresentationHash != first.RepresentationHash {
		t.Fatalf("representation hash changed: %s != %s", second.RepresentationHash, first.RepresentationHash)
	}
	if second.ElementsCreated != 0 || second.ViewsCreated != 0 || second.ConnectorsCreated != 0 {
		t.Fatalf("rerun should reuse resources, got %+v", second)
	}
	if counts := workspaceCounts(t, db); counts != countsAfterFirst {
		t.Fatalf("rerun duplicated resources: before %+v after %+v", countsAfterFirst, counts)
	}

	summary, err := store.RepresentationSummary(context.Background(), scanResult.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.VisibleSymbols != 2 || summary.VisibleReferences != 1 {
		t.Fatalf("unexpected representation summary: %+v", summary)
	}
	decisions, err := store.FilterDecisions(context.Background(), scanResult.RepositoryID, FilterDecisionQuery{Decision: "visible"})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) < 3 {
		t.Fatalf("expected symbol and reference decisions, got %+v", decisions)
	}
}

func TestRepresentCollapsesHighRawReferenceFolderPairs(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "internal/pkg/lib.go", `package pkg

func Target0() {}
func Target1() {}
func Target2() {}
func Target3() {}
func Target4() {}
func Target5() {}
func Target6() {}
func Target7() {}
func Target8() {}
func Target9() {}
`)
	writeFile(t, repo, "cmd/app/main.go", `package main

import "example.com/test/internal/pkg"

func Main() {
	pkg.Target0()
	pkg.Target1()
	pkg.Target2()
	pkg.Target3()
	pkg.Target4()
	pkg.Target5()
	pkg.Target6()
	pkg.Target7()
	pkg.Target8()
	pkg.Target9()
}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	req := RepresentRequest{
		Embedding: EmbeddingConfig{Provider: "none"},
		Thresholds: Thresholds{
			MaxExpandedConnectorsPerGroup: 4,
		},
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, req); err != nil {
		t.Fatal(err)
	}

	var label string
	err = db.QueryRow(`
		SELECT c.label
		FROM connectors c
		JOIN elements s ON s.id = c.source_element_id
		JOIN elements t ON t.id = c.target_element_id
		WHERE s.name = 'cmd' AND t.name = 'internal'`).Scan(&label)
	if err != nil {
		t.Fatalf("expected collapsed cmd -> internal connector: %v", err)
	}
	if label != "10 references" {
		t.Fatalf("expected raw reference count label, got %q", label)
	}
}

func TestRepresentPrioritizesCrossFolderAggregatesOverFilePairs(t *testing.T) {
	groups := map[string][]filePairReference{
		"file:cmd/a.go->cmd/b.go": {
			{Key: "cmd/a.go->cmd/b.go", Count: 500},
		},
		"folder:cmd->internal": {
			{Key: "cmd/a.go->internal/b.go", Count: 20},
		},
		"file:assets.go->internal/a.go": {
			{Key: "assets.go->internal/a.go", Count: 200},
		},
	}

	keys := sortedFileGroupKeys(groups)
	if len(keys) < 3 {
		t.Fatalf("expected sorted keys, got %+v", keys)
	}
	if keys[0] != "folder:cmd->internal" {
		t.Fatalf("expected cross-folder aggregate to be materialized first, got %+v", keys)
	}
}

func TestScanCollectsConfiguredLanguages(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")
	writeFile(t, repo, "src/app.ts", "export function render() { return helper() }\nfunction helper() { return 1 }\n")

	store := NewStore(db)
	scanner := NewScanner(store)
	scanner.Settings = Settings{Languages: []string{"go", "typescript"}}
	result, err := scanner.Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesSeen != 2 || result.FilesParsed != 2 {
		t.Fatalf("expected two parsed source files, got %+v", result)
	}
	symbols, err := store.SymbolsForRepository(context.Background(), result.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	seenLanguages := map[string]bool{}
	for _, sym := range symbols {
		seenLanguages[languageFromStableKey(sym.StableKey)] = true
	}
	if !seenLanguages["go"] || !seenLanguages["typescript"] {
		t.Fatalf("expected go and typescript stable keys, got %#v", seenLanguages)
	}
}

func TestScanRespectsGitIgnore(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, ".gitignore", "ignored.go\nnested/\n")
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")
	writeFile(t, repo, "ignored.go", "package main\nfunc Ignored() {}\n")
	writeFile(t, repo, "nested/ignored.go", "package nested\nfunc NestedIgnored() {}\n")

	store := NewStore(db)
	result, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesSeen != 1 || result.FilesParsed != 1 {
		t.Fatalf("expected only non-ignored source file to be scanned, got %+v", result)
	}
	symbols, err := store.SymbolsForRepository(context.Background(), result.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 || symbols[0].Name != "Main" {
		t.Fatalf("expected only Main symbol, got %+v", symbols)
	}
}

func TestScanForceRescanReparsesCachedFiles(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")

	store := NewStore(db)
	scanner := NewScanner(store)
	first, err := scanner.Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := scanner.Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	forced, err := scanner.ScanWithOptions(context.Background(), repo, ScanOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.FilesParsed != 1 || second.FilesSkipped != 1 || forced.FilesParsed != 1 {
		t.Fatalf("unexpected scan cache behavior: first=%+v second=%+v forced=%+v", first, second, forced)
	}
}

func TestNormalizeSettingsFiltersLanguagesAndDefaultsDurations(t *testing.T) {
	settings := NormalizeSettings(Settings{
		Languages: []string{"TypeScript", "go", "rust", "bogus", "go", ""},
		Watcher:   "unknown",
		Thresholds: Thresholds{
			MaxElementsPerView: 4,
		},
	})
	if strings.Join(settings.Languages, ",") != "go,typescript" {
		t.Fatalf("unexpected normalized languages: %#v", settings.Languages)
	}
	if settings.Watcher != WatcherAuto {
		t.Fatalf("unknown watcher should normalize to auto, got %q", settings.Watcher)
	}
	if settings.PollInterval <= 0 || settings.Debounce <= 0 {
		t.Fatalf("expected default durations, got poll=%s debounce=%s", settings.PollInterval, settings.Debounce)
	}
	if settings.Thresholds.MaxElementsPerView != 4 || settings.Thresholds.MaxConnectorsPerView <= 0 {
		t.Fatalf("expected provided threshold plus defaults, got %+v", settings.Thresholds)
	}

	fallback := NormalizeSettings(Settings{Languages: []string{"rust", "bogus"}})
	if len(fallback.Languages) == 0 || !languageAllowed("go", languageSet(fallback.Languages)) {
		t.Fatalf("invalid-only language list should fall back to defaults, got %#v", fallback.Languages)
	}
}

func TestSourceSnapshotsRespectLanguagesAndReportChangeLanguage(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")
	writeFile(t, repo, "web/app.ts", "export function render() { return 1 }\n")
	writeFile(t, repo, "README.md", "# ignored\n")

	settings := Settings{Languages: []string{"typescript"}}
	snapshot := sourceFileSnapshot(repo, settings, nil)
	if len(snapshot) != 1 || snapshot["web/app.ts"] == "" {
		t.Fatalf("expected only TypeScript source file, got %#v", snapshot)
	}

	changes := diffSourceFileSnapshots(
		map[string]string{"old.py": "python:1:1", "same.ts": "typescript:1:1", "changed.go": "go:1:1"},
		map[string]string{"same.ts": "typescript:1:1", "changed.go": "go:2:1", "new.cpp": "cpp:1:1"},
	)
	if got := changeSummary(changes); got != "changed.go:modified:go,new.cpp:added:cpp,old.py:deleted:python" {
		t.Fatalf("unexpected source changes: %s (%+v)", got, changes)
	}
}

func TestSourceWatcherFiltersRelevantEvents(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")
	writeFile(t, repo, "web/app.ts", "export function render() { return 1 }\n")
	writeFile(t, repo, "README.md", "# ignored\n")
	allowed := languageSet([]string{"typescript"})

	if sourceEventRelevant(repo, filepath.Join(repo, "main.go"), allowed, nil) {
		t.Fatal("Go event should be ignored when only TypeScript is allowed")
	}
	if !sourceEventRelevant(repo, filepath.Join(repo, "web", "app.ts"), allowed, nil) {
		t.Fatal("TypeScript event should be relevant")
	}
	if sourceEventRelevant(repo, filepath.Join(repo, "README.md"), allowed, nil) {
		t.Fatal("non-source event should be ignored")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher := newSourceWatcher(ctx, repo, Settings{Watcher: WatcherPoll}, nil)
	if watcher.Mode != WatcherPoll || watcher.Events != nil {
		t.Fatalf("poll watcher should not create fs event channel, got %+v", watcher)
	}
}

func TestWatchDiffsCaptureWorkspaceResourceChanges(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
}

func helper() {}
`)
	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	firstDiffs, err := store.BuildWatchDiffs(context.Background(), scan.RepositoryID, rep.RepresentationHash)
	if err != nil {
		t.Fatal(err)
	}
	if connector := findDiff(firstDiffs, "connector", "added"); connector == nil || connector.Summary == nil || !strings.Contains(*connector.Summary, "->") {
		t.Fatalf("expected connector diff summary to include endpoint arrow, got %+v", connector)
	}
	if _, err := store.CreateWatchVersion(context.Background(), scan.RepositoryID, "commit1", "first commit", "", "main", rep.RepresentationHash, nil, firstDiffs); err != nil {
		t.Fatal(err)
	}

	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
	other()
}

func helper() {}
func other() {}
`)
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	next, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	diffs, err := store.BuildWatchDiffs(context.Background(), scan.RepositoryID, next.RepresentationHash)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiff(diffs, "symbol", "added") || !hasDiff(diffs, "file", "updated") || !hasDiff(diffs, "element", "added") {
		t.Fatalf("expected symbol/file/element diffs, got %+v", diffs)
	}
}

func TestWatchDiffsIncludeElementLineDeltas(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
}

func helper() {}
`)
	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	firstDiffs, err := store.BuildWatchDiffs(context.Background(), scan.RepositoryID, rep.RepresentationHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWatchVersion(context.Background(), scan.RepositoryID, "commit1", "first commit", "", "main", rep.RepresentationHash, nil, firstDiffs); err != nil {
		t.Fatal(err)
	}

	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
	helper()
}

func helper() {}
`)
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	next, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	diffs, err := store.BuildWatchDiffs(context.Background(), scan.RepositoryID, next.RepresentationHash)
	if err != nil {
		t.Fatal(err)
	}
	for _, diff := range diffs {
		if diff.ResourceType != nil && *diff.ResourceType == "element" && diff.ChangeType == "updated" && diff.AddedLines == 1 {
			return
		}
	}
	t.Fatalf("expected updated element diff with +1 line, got %+v", diffs)
}

func TestCreateVersionForHeadCanBaselineAlreadyRepresentedCommit(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	status, err := gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: store}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, rep.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}

	writeFile(t, repo, "main.go", `package main

func Main() {}
func Other() {}
`)
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	next, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	pendingDiffs, err := store.BuildWatchDiffs(context.Background(), scan.RepositoryID, next.RepresentationHash)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiff(pendingDiffs, "element", "added") {
		t.Fatalf("expected uncommitted representation to have pending element diff, got %+v", pendingDiffs)
	}

	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add other")
	status, err = gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, next.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}
	latest, found, err := store.LatestWatchVersion(context.Background(), scan.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || latest.CommitHash != status.HeadCommit {
		t.Fatalf("expected latest version for committed head, got found=%v version=%+v status=%+v", found, latest, status)
	}
	committedDiffs, err := store.WatchDiffs(context.Background(), latest.ID, "", "", "", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(committedDiffs) != 0 {
		t.Fatalf("expected committed baseline version to have no pending diffs, got %+v", committedDiffs)
	}
	if latest.CommitMessage != "add other" {
		t.Fatalf("expected commit message to be stored, got %q", latest.CommitMessage)
	}
}

func TestCreateVersionForHeadStoresDirtyHeadDiffsAndMetadata(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	status, err := gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: store}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, rep.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}
	firstHead := status.HeadCommit

	writeFile(t, repo, "main.go", `package main

func Main() {}
func Committed() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add committed")
	intermediateHead, err := tldgit.DetectHeadCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, "main.go", `package main

func Main() {}
func Committed() {}
func SecondCommitted() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add second committed")
	writeFile(t, repo, "main.go", `package main

func Main() {}
func Committed() {}
func SecondCommitted() {}
func Dirty() {}
`)
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	dirtyRep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	status, err = gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if gitStatusClean(status) {
		t.Fatalf("test setup should have a dirty worktree: %+v", status)
	}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, dirtyRep.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}

	latest, found, err := store.LatestWatchVersion(context.Background(), scan.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || latest.CommitHash != status.HeadCommit || latest.CommitMessage != "add second committed" || latest.ParentCommitHash != intermediateHead || latest.Branch == "" || latest.WorkspaceVersionID == nil {
		t.Fatalf("dirty head version metadata was not stored correctly: found=%v latest=%+v status=%+v first=%s intermediate=%s", found, latest, status, firstHead, intermediateHead)
	}
	diffs, err := store.WatchDiffs(context.Background(), latest.ID, "", "", "", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiff(diffs, "element", "added") {
		t.Fatalf("expected dirty head snapshot to retain pending representation diffs, got %+v", diffs)
	}
}

func TestCreateWatchVersionRetainsOnlyFiveRecentSnapshots(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)

	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	resourceType := "element"
	resourceID := int64(1)
	for i := 1; i <= 6; i++ {
		after := fmt.Sprintf("after-%d", i)
		summary := fmt.Sprintf("snapshot %d", i)
		diffs := []RepresentationDiff{{
			OwnerType:    "symbol",
			OwnerKey:     fmt.Sprintf("go:main.go:function:Main%d", i),
			ChangeType:   "added",
			AfterHash:    &after,
			ResourceType: &resourceType,
			ResourceID:   &resourceID,
			Summary:      &summary,
			AddedLines:   1,
		}}
		if _, err := store.CreateWatchVersion(context.Background(), scan.RepositoryID, fmt.Sprintf("commit-%d", i), fmt.Sprintf("commit %d", i), "", "main", fmt.Sprintf("%s-%d", rep.RepresentationHash, i), nil, diffs); err != nil {
			t.Fatal(err)
		}
	}

	versions, err := store.WatchVersions(context.Background(), scan.RepositoryID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 5 {
		t.Fatalf("expected only five retained watch versions, got %d: %+v", len(versions), versions)
	}
	for i, version := range versions {
		expected := fmt.Sprintf("commit-%d", 6-i)
		if version.CommitHash != expected {
			t.Fatalf("expected retained version %d to be %s, got %+v", i, expected, version)
		}
	}
	var oldestDiffs int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM watch_representation_diffs d
		JOIN watch_versions v ON v.id = d.version_id
		WHERE v.repository_id = ? AND v.commit_hash = 'commit-1'`, scan.RepositoryID).Scan(&oldestDiffs); err != nil {
		t.Fatal(err)
	}
	if oldestDiffs != 0 {
		t.Fatalf("expected oldest snapshot diffs to be pruned, found %d", oldestDiffs)
	}
	var resources int
	if err := db.QueryRow(`SELECT COUNT(*) FROM watch_version_resources`).Scan(&resources); err != nil {
		t.Fatal(err)
	}
	if resources == 0 {
		t.Fatal("expected retained snapshots to keep version resources")
	}
	var materializedElements int
	if err := db.QueryRow(`SELECT COUNT(*) FROM elements`).Scan(&materializedElements); err != nil {
		t.Fatal(err)
	}
	if materializedElements == 0 {
		t.Fatal("snapshot pruning should not delete current materialized workspace resources")
	}
}

func TestDeletedFileTombstonesMaterializedResourcesAndDiffs(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
}

func helper() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWatchVersion(context.Background(), scan.RepositoryID, "commit1", "initial", "", "main", rep.RepresentationHash, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, "main.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	status, err := gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyGitTags(context.Background(), scan.RepositoryID, status); err != nil {
		t.Fatal(err)
	}

	summary, err := store.Summary(context.Background(), scan.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Files != 0 || summary.Symbols != 0 {
		t.Fatalf("raw graph should remove deleted file and symbols, got %+v", summary)
	}
	if tagged := countElementTag(t, db, "watch:deleted"); tagged == 0 {
		t.Fatal("expected tombstoned materialized resources to receive watch:deleted")
	}
	if count := materializationOwnerTypeCount(t, db, "file"); count == 0 {
		t.Fatal("expected deleted file materialization mapping to be retained as tombstone")
	}
	diffs, err := store.BuildWatchDiffs(context.Background(), scan.RepositoryID, rep.RepresentationHash)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiff(diffs, "file", "deleted") || !hasDiff(diffs, "symbol", "deleted") || !hasDiff(diffs, "element", "deleted") {
		t.Fatalf("expected deleted raw and materialized diffs, got %+v", diffs)
	}
}

func TestRestoredDeletedFileRemovesTombstoneTagsAndReusesResources(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	source := `package main

func Main() {}
`
	writeFile(t, repo, "main.go", source)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	fileElementID, ok, err := store.MappingResourceID(context.Background(), scan.RepositoryID, "file", "file:main.go", "element")
	if err != nil || !ok {
		t.Fatalf("expected file element mapping, ok=%v err=%v", ok, err)
	}
	if err := os.Remove(filepath.Join(repo, "main.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	status, err := gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyGitTags(context.Background(), scan.RepositoryID, status); err != nil {
		t.Fatal(err)
	}
	if tagged := countElementTag(t, db, "watch:deleted"); tagged == 0 {
		t.Fatal("expected deletion to create tombstone tag")
	}

	writeFile(t, repo, "main.go", source)
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	status, err = gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyGitTags(context.Background(), scan.RepositoryID, status); err != nil {
		t.Fatal(err)
	}
	nextFileElementID, ok, err := store.MappingResourceID(context.Background(), scan.RepositoryID, "file", "file:main.go", "element")
	if err != nil || !ok {
		t.Fatalf("expected restored file element mapping, ok=%v err=%v", ok, err)
	}
	if nextFileElementID != fileElementID {
		t.Fatalf("expected restored file to reuse element %d, got %d", fileElementID, nextFileElementID)
	}
	if tagged := countElementTag(t, db, "watch:deleted"); tagged != 0 {
		t.Fatalf("expected restore to remove watch:deleted, found %d tagged elements", tagged)
	}
}

func TestCleanHeadPrunesDeletedMaterializedTombstones(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	status, err := gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: store}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, rep.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(repo, "main.go")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "-u")
	runGit(t, repo, "commit", "-m", "delete main")
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	next, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	status, err = gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !gitStatusClean(status) {
		t.Fatalf("test setup should have clean status after deletion commit: %+v", status)
	}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, next.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}

	if count := materializationOwnerTypeCount(t, db, "file"); count != 0 {
		t.Fatalf("expected clean baseline to prune deleted file mappings, got %d", count)
	}
	if count := materializationOwnerTypeCount(t, db, "symbol"); count != 0 {
		t.Fatalf("expected clean baseline to prune deleted symbol mappings, got %d", count)
	}
	if tagged := countElementTag(t, db, "watch:deleted"); tagged != 0 {
		t.Fatalf("expected clean baseline cleanup to remove tombstone tags with resources, found %d", tagged)
	}
	latest, found, err := store.LatestWatchVersion(context.Background(), scan.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || latest.CommitHash != status.HeadCommit {
		t.Fatalf("expected clean deletion baseline version, found=%v latest=%+v status=%+v", found, latest, status)
	}
	diffs, err := store.WatchDiffs(context.Background(), latest.ID, "", "", "", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected clean baseline to store no pending diffs, got %+v", diffs)
	}
}

func TestDirtyHeadRetainsDeletedMaterializedTombstonesAndDiffs(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "gone.go", `package main

func Gone() {}
`)
	writeFile(t, repo, "keep.go", `package main

func Keep() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")

	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	status, err := gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: store}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, rep.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(repo, "gone.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, "keep.go", `package main

func Keep() {}
func Added() {}
`)
	runGit(t, repo, "add", "keep.go")
	runGit(t, repo, "commit", "-m", "add keep symbol")
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	next, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	status, err = gitStatusSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if gitStatusClean(status) || len(status.Deleted) == 0 {
		t.Fatalf("test setup should retain dirty deleted file after HEAD change: %+v", status)
	}
	if _, err := store.ApplyGitTags(context.Background(), scan.RepositoryID, status); err != nil {
		t.Fatal(err)
	}
	if err := runner.createVersionForHead(context.Background(), scan.RepositoryID, status, next.RepresentationHash, false); err != nil {
		t.Fatal(err)
	}

	if tagged := countElementTag(t, db, "watch:deleted"); tagged == 0 {
		t.Fatal("expected dirty head to retain deleted tombstones")
	}
	if count := materializationOwnerTypeCount(t, db, "file"); count == 0 {
		t.Fatal("expected dirty head to retain deleted file mapping")
	}
	latest, found, err := store.LatestWatchVersion(context.Background(), scan.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || latest.CommitHash != status.HeadCommit {
		t.Fatalf("expected dirty head snapshot, found=%v latest=%+v status=%+v", found, latest, status)
	}
	diffs, err := store.WatchDiffs(context.Background(), latest.ID, "", "", "", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiff(diffs, "file", "deleted") || !hasDiff(diffs, "element", "deleted") {
		t.Fatalf("expected dirty head snapshot to retain deleted diffs, got %+v", diffs)
	}
}

func TestWatchDiffsFilterByResourceTypeAndLanguage(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	store := NewStore(db)
	scan, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := NewRepresenter(store).Represent(context.Background(), scan.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	diffs, err := store.BuildWatchDiffs(context.Background(), scan.RepositoryID, rep.RepresentationHash)
	if err != nil {
		t.Fatal(err)
	}
	version, err := store.CreateWatchVersion(context.Background(), scan.RepositoryID, "commit1", "first commit", "", "main", rep.RepresentationHash, nil, diffs)
	if err != nil {
		t.Fatal(err)
	}

	symbolDiffs, err := store.WatchDiffs(context.Background(), version.ID, "", "added", "symbol", "go", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbolDiffs) == 0 {
		t.Fatalf("expected Go symbol diffs, got none from %+v", diffs)
	}
	for _, diff := range symbolDiffs {
		if diff.ResourceType == nil || *diff.ResourceType != "symbol" || diff.ChangeType != "added" || diff.Language == nil || *diff.Language != "go" {
			t.Fatalf("diff did not satisfy filters: %+v", diff)
		}
	}

	none, err := store.WatchDiffs(context.Background(), version.ID, "", "", "symbol", "python", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no Python symbol diffs, got %+v", none)
	}
}

func TestRepresentInitialLayoutFollowsConnectors(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func A() {
	B()
	C()
}

func B() {}
func C() {}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}

	a := functionPlacement(t, db, "A")
	b := functionPlacement(t, db, "B")
	c := functionPlacement(t, db, "C")
	if b.x <= a.x || c.x <= a.x {
		t.Fatalf("initial layout should place callees to the right of caller: A=%+v B=%+v C=%+v", a, b, c)
	}
	if b.x == c.x && b.y == c.y {
		t.Fatalf("initial layout overlapped connected callees: B=%+v C=%+v", b, c)
	}
}

func TestRepresentRelayoutsFreshPlacementsWithExistingMappings(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func A() {
	B()
	C()
}

func B() {}
func C() {}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM placements`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}

	a := functionPlacement(t, db, "A")
	b := functionPlacement(t, db, "B")
	c := functionPlacement(t, db, "C")
	if b.x <= a.x || c.x <= a.x {
		t.Fatalf("fresh placements with existing mappings should use full layout: A=%+v B=%+v C=%+v", a, b, c)
	}
	if b.x == c.x && b.y == c.y {
		t.Fatalf("fresh placements with existing mappings overlapped connected callees: B=%+v C=%+v", b, c)
	}
}

func TestRepresentIncrementalLayoutPreservesExistingPlacements(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func A() {
	B()
}

func B() {}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	b := functionPlacement(t, db, "B")
	if _, err := db.Exec(`UPDATE placements SET position_x = 780, position_y = 510 WHERE id = ?`, b.placementID); err != nil {
		t.Fatal(err)
	}

	writeFile(t, repo, "main.go", `package main

func A() {
	B()
	C()
}

func B() {}
func C() {}
`)
	if _, err := NewScanner(store).Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}

	b = functionPlacement(t, db, "B")
	c := functionPlacement(t, db, "C")
	if b.x != 780 || b.y != 510 {
		t.Fatalf("incremental layout moved existing placement B: %+v", b)
	}
	if c.x == b.x && c.y == b.y {
		t.Fatalf("incremental layout placed new function on occupied B cell: B=%+v C=%+v", b, c)
	}
}

func TestRepresentDoesNotTouchManualResources(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")
	res, err := db.Exec(`INSERT INTO elements(name, tags, technology_connectors, created_at, updated_at) VALUES ('Manual', '[]', '[]', 'now', 'now')`)
	if err != nil {
		t.Fatal(err)
	}
	manualID, _ := res.LastInsertId()

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}

	var name string
	if err := db.QueryRow(`SELECT name FROM elements WHERE id = ?`, manualID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Manual" {
		t.Fatalf("manual element was changed to %q", name)
	}
}

func TestRepresentAssignsUsefulSemanticTags(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "internal/watch/scan.go", `package watch

func ScanRepository() {
	RepresentRepository()
}

func RepresentRepository() {}
`)
	writeFile(t, repo, "internal/server/http.go", `package server

func ServeAPI() {}
`)
	writeFile(t, repo, "cmd/tld/main.go", `package main

func ExecuteCLI() {}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}

	for _, tag := range []string{"tld:watch", "watch:generated", "watch:go", "lang:go"} {
		if count := countElementTag(t, db, tag); count != 0 {
			t.Fatalf("expected unhelpful tag %q to be omitted, found on %d elements", tag, count)
		}
	}
	for _, tag := range []string{"role:watch", "area:internal", "kind:function", "graph:entrypoint"} {
		if count := countElementTag(t, db, tag); count < 2 {
			t.Fatalf("expected useful tag %q on multiple elements, found %d", tag, count)
		}
	}

	tags := elementTagsByName(t, db, "ScanRepository")
	for _, tag := range []string{"role:watch", "area:internal", "kind:function", "graph:entrypoint"} {
		if !stringSliceContains(tags, tag) {
			t.Fatalf("expected ScanRepository to include %q, got %v", tag, tags)
		}
	}
}

func TestRepresentAssignsCodeownersTags(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "CODEOWNERS", `
/frontend/* @org/web-team:random(2)
/backend/* @backend @org/backend:least_busy(3)
`)
	writeFile(t, repo, "frontend/app.go", `package frontend

func Render() {}
`)
	writeFile(t, repo, "backend/server.go", `package backend

func Serve() {}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"frontend", "app.go", "Render"} {
		tags := elementTagsByName(t, db, name)
		if !stringSliceContains(tags, "owner:@org/web-team") {
			t.Fatalf("expected %s to include CODEOWNERS tag, got %v", name, tags)
		}
		if stringSliceContains(tags, "owner:@org/web-team:random(2)") {
			t.Fatalf("expected %s extended assignment suffix to be stripped, got %v", name, tags)
		}
	}
	backendTags := elementTagsByName(t, db, "Serve")
	for _, tag := range []string{"owner:@backend", "owner:@org/backend"} {
		if !stringSliceContains(backendTags, tag) {
			t.Fatalf("expected backend symbol to include %q, got %v", tag, backendTags)
		}
	}
	if count := countElementTag(t, db, "owner:@org/web-team"); count < 3 {
		t.Fatalf("expected rare CODEOWNERS tag to bypass semantic coverage filtering, found on %d elements", count)
	}
}

func TestLargeRepresentationPrunesDetailedSymbolElements(t *testing.T) {
	previousLimit := maxDetailedSymbolElements
	maxDetailedSymbolElements = 100
	defer func() { maxDetailedSymbolElements = previousLimit }()

	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "pkg/busy.go", `package pkg

func Func0() {}
func Func1() {}
func Func2() {}
func Func3() {}
func Func4() {}
`)

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	req := RepresentRequest{
		Embedding: EmbeddingConfig{Provider: "none"},
		Thresholds: Thresholds{
			MaxElementsPerView:   2,
			MaxConnectorsPerView: 2,
		},
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, req); err != nil {
		t.Fatal(err)
	}
	if count := elementKindCount(t, db, "function"); count != 5 {
		t.Fatalf("expected detailed symbol elements before large-mode pruning, got %d", count)
	}

	maxDetailedSymbolElements = 3
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, req); err != nil {
		t.Fatal(err)
	}
	if count := elementKindCount(t, db, "function"); count != 0 {
		t.Fatalf("expected large-mode rerun to prune detailed symbol elements, got %d", count)
	}
	if count := materializationOwnerTypeCount(t, db, "symbol"); count != 0 {
		t.Fatalf("expected stale symbol materialization mappings to be pruned, got %d", count)
	}
	if count := elementKindCount(t, db, "cluster"); count == 0 {
		t.Fatalf("expected cluster elements to summarize the large file")
	}
}

func TestEmbeddingCandidateSymbolsAreCappedDeterministically(t *testing.T) {
	symbols := map[int64]Symbol{
		3: {ID: 3, StableKey: "go:b.go:function:C", FilePath: "b.go", StartLine: 1},
		1: {ID: 1, StableKey: "go:a.go:function:A", FilePath: "a.go", StartLine: 10},
		2: {ID: 2, StableKey: "go:a.go:function:B", FilePath: "a.go", StartLine: 2},
	}
	candidates := embeddingCandidateSymbols(symbols, 2)
	if len(candidates) != 2 {
		t.Fatalf("expected capped candidates, got %d", len(candidates))
	}
	if candidates[0].ID != 2 || candidates[1].ID != 1 {
		t.Fatalf("unexpected candidate order: %+v", candidates)
	}
}

func TestApplyGitTagsReportsAddedAndRemovedTags(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}

	first, err := store.ApplyGitTags(context.Background(), scanResult.RepositoryID, GitStatus{Untracked: []string{"main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.TagsAdded == 0 || first.TagsRemoved != 0 {
		t.Fatalf("expected untracked tags to be added only, got %+v", first)
	}
	if tagged := countElementTag(t, db, "git:untracked"); tagged == 0 {
		t.Fatalf("expected git:untracked on generated elements")
	}

	second, err := store.ApplyGitTags(context.Background(), scanResult.RepositoryID, GitStatus{})
	if err != nil {
		t.Fatal(err)
	}
	if second.TagsAdded != 0 || second.TagsRemoved != first.TagsAdded {
		t.Fatalf("expected stale git tags to be removed, first=%+v second=%+v", first, second)
	}
	if tagged := countElementTag(t, db, "git:untracked"); tagged != 0 {
		t.Fatalf("expected git:untracked to be removed, found %d tagged elements", tagged)
	}
}

func TestEmbeddingCacheAvoidsProviderCalls(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store := NewStore(db)
	provider := &countingProvider{}
	model := provider.ModelID()
	modelID, err := store.EnsureEmbeddingModel(context.Background(), EmbeddingConfig{Provider: model.Provider, Model: model.Model, Dimension: model.Dimension}, model.ConfigHash)
	if err != nil {
		t.Fatal(err)
	}
	symbols := map[int64]Symbol{
		1: {ID: 1, StableKey: "go:a.go:function:A", QualifiedName: "A", Kind: "function", FilePath: "a.go"},
		2: {ID: 2, StableKey: "go:b.go:function:B", QualifiedName: "B", Kind: "function", FilePath: "b.go"},
	}
	representer := NewRepresenter(store)
	stats, _, err := representer.cacheEmbeddings(context.Background(), modelID, provider, "", []Symbol{
		symbols[1],
		symbols[2],
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != 2 {
		t.Fatalf("expected two embeddings created, got %+v", stats)
	}
	if provider.calls != 1 || provider.inputs != 2 {
		t.Fatalf("expected one batched provider call for two inputs, got calls=%d inputs=%d", provider.calls, provider.inputs)
	}
	stats, _, err = representer.cacheEmbeddings(context.Background(), modelID, provider, "", []Symbol{
		symbols[1],
		symbols[2],
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CacheHits != 2 {
		t.Fatalf("expected two embedding cache hits, got %+v", stats)
	}
	if provider.calls != 1 {
		t.Fatalf("cache miss recomputed embeddings, calls=%d", provider.calls)
	}
}

func TestEmbeddingCacheChunksProviderCallsAndReportsProgress(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store := NewStore(db)
	provider := &countingProvider{}
	model := provider.ModelID()
	modelID, err := store.EnsureEmbeddingModel(context.Background(), EmbeddingConfig{Provider: model.Provider, Model: model.Model, Dimension: model.Dimension}, model.ConfigHash)
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]Symbol, 0, defaultEmbeddingBatchSize*2+1)
	for i := 0; i < defaultEmbeddingBatchSize*2+1; i++ {
		name := fmt.Sprintf("Symbol%d", i)
		symbols = append(symbols, Symbol{ID: int64(i + 1), StableKey: "go:a.go:function:" + name, QualifiedName: name, Kind: "function", FilePath: "a.go"})
	}
	progress := &recordingProgress{}

	stats, _, err := NewRepresenter(store).cacheEmbeddings(context.Background(), modelID, provider, "", symbols, nil, progress)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != len(symbols) {
		t.Fatalf("expected %d embeddings created, got %+v", len(symbols), stats)
	}
	expectedBatchSizes := fmt.Sprintf("%d,%d,1", defaultEmbeddingBatchSize, defaultEmbeddingBatchSize)
	if provider.calls != 3 || strings.Join(provider.batchSizes, ",") != expectedBatchSizes {
		t.Fatalf("expected chunked provider calls %s, got calls=%d batchSizes=%v", expectedBatchSizes, provider.calls, provider.batchSizes)
	}
	expectedProgressTotal := fmt.Sprintf("%d", defaultEmbeddingBatchSize*2+1)
	if len(progress.starts) != 2 || progress.starts[0] != "Preparing symbol embeddings:"+expectedProgressTotal || progress.starts[1] != "Embedding symbols:"+expectedProgressTotal {
		t.Fatalf("unexpected progress starts: %v", progress.starts)
	}
	if progress.advances != len(symbols)*2 {
		t.Fatalf("expected prepare and embed progress advances, got %d", progress.advances)
	}
}

func TestSymbolEmbeddingTextUsesOutdentedCodeBody(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "a.go", `package main

func Outer() {
    if true {
        fmt.Println("body")
    }
}
`)
	end := 6
	text := symbolEmbeddingText(repo, Symbol{
		QualifiedName: "Outer",
		Kind:          "function",
		FilePath:      "a.go",
		StartLine:     3,
		EndLine:       &end,
	})

	if !strings.Contains(text, `fmt.Println("body")`) {
		t.Fatalf("expected embedding text to include code body, got:\n%s", text)
	}
	if strings.Contains(text, "Outer\nfunction\na.go") {
		t.Fatalf("embedding text fell back to metadata instead of source body:\n%s", text)
	}
}

func TestShrinkEmbeddingTextFitsApproximateTokenBudget(t *testing.T) {
	text := shrinkEmbeddingText(strings.Repeat("// comment that should be removed\n", 600) + strings.Repeat("statement := value + otherValue\n", 700))
	if approximateTokenCount(text) > maxEmbeddingInputApproxTokens {
		t.Fatalf("expected text within token budget, got %d", approximateTokenCount(text))
	}
	if strings.Contains(text, "// comment") {
		t.Fatalf("expected low-signal comment lines to be dropped")
	}
}

func TestLocalLexicalProviderKeepsRenamedCodeSimilar(t *testing.T) {
	provider := LexicalProvider{}
	vectors, err := provider.Embed(context.Background(), []EmbeddingInput{
		{Text: `func FetchUser(id string) (*User, error) {
	cacheKey := "user:" + id
	if cached, ok := cache.Get(cacheKey); ok {
		return cached, nil
	}
	return client.Load(id)
}`},
		{Text: `func LoadAccount(accountID string) (*Account, error) {
	cacheKey := "user:" + accountID
	if cached, ok := cache.Get(cacheKey); ok {
		return cached, nil
	}
	return client.Load(accountID)
}`},
		{Text: `func WriteAudit(event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return os.WriteFile("audit.json", data, 0600)
}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	renamed := CosineSimilarity(vectors[0], vectors[1])
	unrelated := CosineSimilarity(vectors[0], vectors[2])
	if renamed < 0.70 {
		t.Fatalf("expected renamed implementation to stay similar, got %.3f", renamed)
	}
	if unrelated >= renamed {
		t.Fatalf("expected unrelated implementation below renamed similarity, renamed=%.3f unrelated=%.3f", renamed, unrelated)
	}
}

func TestDefaultEmbeddingConfigUsesLocalOpenAIEndpoint(t *testing.T) {
	cfg := NormalizeEmbeddingConfig(EmbeddingConfig{})
	if cfg.Provider != "openai" || cfg.Endpoint != DefaultOpenAIEndpoint || cfg.Model != DefaultOpenAIModel {
		t.Fatalf("unexpected default embedding config: %+v", cfg)
	}
}

func TestOpenAIHealthCheckUsesCompatibleEmbeddingsEndpoint(t *testing.T) {
	var requestBody struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth == "" {
			t.Fatalf("expected authorization header for OpenAI-compatible request")
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","model":"text-embedding-embeddinggemma-300m-qat","data":[{"object":"embedding","index":0,"embedding":[1,0,0]},{"object":"embedding","index":1,"embedding":[0.95,0.05,0]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer server.Close()

	cfg, result, err := CheckEmbeddingHealth(context.Background(), EmbeddingConfig{
		Provider: "openai",
		Endpoint: server.URL + "/v1/embeddings",
		Model:    "text-embedding-embeddinggemma-300m-qat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestBody.Model != "text-embedding-embeddinggemma-300m-qat" || len(requestBody.Input) != 2 {
		t.Fatalf("unexpected embeddings request body: %+v", requestBody)
	}
	if cfg.Dimension != 3 || result.Dimension != 3 || result.Similarity < DefaultEmbeddingHealthThreshold {
		t.Fatalf("unexpected health result cfg=%+v result=%+v", cfg, result)
	}
}

func TestOllamaHealthCheckParsesEmbedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"embeddings":[[1,0,0],[0.95,0.05,0]]}`))
	}))
	defer server.Close()

	cfg, result, err := CheckEmbeddingHealth(context.Background(), EmbeddingConfig{
		Provider: "ollama",
		Endpoint: server.URL,
		Model:    "jina/jina-embeddings-v2-base-en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dimension != 3 || result.Dimension != 3 || result.Similarity < DefaultEmbeddingHealthThreshold {
		t.Fatalf("unexpected health result cfg=%+v result=%+v", cfg, result)
	}
}

func TestSQLiteVecStoresAndQueriesEmbeddings(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store := NewStore(db)
	modelID, err := store.EnsureEmbeddingModel(context.Background(), EmbeddingConfig{Provider: "local-deterministic-test", Model: "vec", Dimension: 3}, "vec")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEmbedding(context.Background(), modelID, "symbol", "a", "a", vectorBytes(Vector{1, 0, 0})); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEmbedding(context.Background(), modelID, "symbol", "b", "b", vectorBytes(Vector{0, 1, 0})); err != nil {
		t.Fatal(err)
	}
	var shadowRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM _vec_watch_embedding_vec`).Scan(&shadowRows); err != nil {
		t.Fatal(err)
	}
	if shadowRows != 2 {
		t.Fatalf("expected sqlite-vec shadow rows, got %d", shadowRows)
	}
	ids, err := store.SimilarEmbeddings(context.Background(), modelID, Vector{1, 0, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one sqlite-vec match, got %v", ids)
	}
}

func TestRenamePreservesGeneratedSymbolElementAndConnector(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	FetchUser()
}

func FetchUser() {}
`)
	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	beforeElement := symbolElementID(t, db, "FetchUser")
	beforeConnectors := connectorCount(t, db)

	writeFile(t, repo, "main.go", `package main

func Main() {
	LoadUser()
}

func LoadUser() {}
`)
	scanResult, err = NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	afterElement := symbolElementID(t, db, "LoadUser")
	if afterElement != beforeElement {
		t.Fatalf("rename created a new generated element: before=%d after=%d", beforeElement, afterElement)
	}
	if afterConnectors := connectorCount(t, db); afterConnectors != beforeConnectors {
		t.Fatalf("rename changed connector count: before=%d after=%d", beforeConnectors, afterConnectors)
	}
}

func TestMoveRenamePreservesGeneratedSymbolElement(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	FetchUser()
}

func FetchUser() int {
	value := 41
	return value + 1
}
`)
	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	beforeElement := symbolElementID(t, db, "FetchUser")

	if err := os.Remove(filepath.Join(repo, "main.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, "pkg/users.go", `package pkg

func Main() {
	LoadAccount()
}

func LoadAccount() int {
	value := 41
	return value + 1
}
`)
	scanResult, err = NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepresenter(store).Represent(context.Background(), scanResult.RepositoryID, RepresentRequest{Embedding: EmbeddingConfig{Provider: "none"}}); err != nil {
		t.Fatal(err)
	}
	afterElement := symbolElementID(t, db, "LoadAccount")
	if afterElement != beforeElement {
		t.Fatalf("move+rename created a new generated element: before=%d after=%d", beforeElement, afterElement)
	}
}

func TestClusterStableKeyIsDeterministic(t *testing.T) {
	left := stableClusterKey(42, "pkg", "settings", []string{"c", "a", "b"})
	right := stableClusterKey(42, "pkg", "settings", []string{"b", "c", "a"})
	if left != right {
		t.Fatalf("stable cluster key changed with member order: %s != %s", left, right)
	}
}

func TestScanLocalOnlyRepositoryIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func main() {
	helper()
}

func helper() {}
`)

	scanner := NewScanner(NewStore(db))
	first, err := scanner.Scan(context.Background(), repo)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if first.FilesSeen != 1 || first.FilesParsed != 1 || first.FilesSkipped != 0 || first.SymbolsSeen != 2 || first.ReferencesSeen != 1 {
		t.Fatalf("unexpected first scan counts: %+v", first)
	}

	second, err := scanner.Scan(context.Background(), repo)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if second.FilesSeen != 1 || second.FilesParsed != 0 || second.FilesSkipped != 1 {
		t.Fatalf("unexpected second scan counts: %+v", second)
	}
	third, err := scanner.Scan(context.Background(), repo)
	if err != nil {
		t.Fatalf("third scan: %v", err)
	}
	if third.FilesSeen != 1 || third.FilesParsed != 0 || third.FilesSkipped != 1 {
		t.Fatalf("unexpected third scan counts after prior skipped status: %+v", third)
	}

	store := NewStore(db)
	repos, err := store.Repositories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].IdentityStatus != "local_only" {
		t.Fatalf("expected one local_only repo, got %+v", repos)
	}
	summary, err := store.Summary(context.Background(), first.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Files != 1 || summary.Symbols != 2 || summary.References != 1 {
		t.Fatalf("unexpected summary after idempotent scan: %+v", summary)
	}
}

func TestScanUsesRemoteURLIdentity(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	runGit(t, repo, "remote", "add", "origin", "git@github.com:owner/repo.git")
	writeFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	result, err := NewScanner(NewStore(db)).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := NewStore(db).Repository(context.Background(), result.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.RemoteURL.Valid || stored.RemoteURL.String != "git@github.com:owner/repo.git" || stored.IdentityStatus != "known" {
		t.Fatalf("unexpected repository identity: %+v", stored)
	}
}

func TestScanRemovesDeletedFiles(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "one.go", "package main\nfunc one() {}\n")
	writeFile(t, repo, "two.go", "package main\nfunc two() {}\n")

	scanner := NewScanner(NewStore(db))
	result, err := scanner.Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, "two.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.Scan(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	summary, err := NewStore(db).Summary(context.Background(), result.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Files != 1 || summary.Symbols != 1 {
		t.Fatalf("deleted file was not reconciled: %+v", summary)
	}
}

func TestScanFailsClearlyOutsideGitRepository(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	_, err := NewScanner(NewStore(db)).Scan(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("expected git repository error, got %v", err)
	}
}

func TestStatusEndpointReportsActiveWatch(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, os.Getpid(), "token", LockHeartbeatTimeout); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	NewHandler(store).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/watch/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Active     bool           `json:"active"`
		Repository RepositoryJSON `json:"repository"`
		Lock       Lock           `json:"lock"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Active || body.Repository.ID != scanResult.RepositoryID || body.Lock.RepositoryID != scanResult.RepositoryID {
		t.Fatalf("unexpected status body: %+v", body)
	}
}

func TestAcquireLockReplacesDeadProcessLock(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	originalProcessCheck := watchProcessIsRunning
	t.Cleanup(func() { watchProcessIsRunning = originalProcessCheck })
	watchProcessIsRunning = func(pid int) bool { return pid == os.Getpid() }

	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, 999999, "dead-token", LockHeartbeatTimeout); err != nil {
		t.Fatal(err)
	}
	lock, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, os.Getpid(), "live-token", LockHeartbeatTimeout)
	if err != nil {
		t.Fatalf("expected dead process lock to be replaced: %v", err)
	}
	if lock.Token != "live-token" || lock.PID != os.Getpid() {
		t.Fatalf("unexpected replacement lock: %+v", lock)
	}
}

func TestActiveLiveLockTreatsDeadProcessAsStale(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	originalProcessCheck := watchProcessIsRunning
	t.Cleanup(func() { watchProcessIsRunning = originalProcessCheck })
	watchProcessIsRunning = func(pid int) bool { return false }

	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, 999999, "dead-token", LockHeartbeatTimeout); err != nil {
		t.Fatal(err)
	}
	lock, live, err := store.ActiveLiveLock(context.Background(), LockHeartbeatTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if live || lock.Token != "dead-token" {
		t.Fatalf("expected dead process lock to be non-live: live=%v lock=%+v", live, lock)
	}
	status, err := store.LockStatus(context.Background(), scanResult.RepositoryID, "dead-token")
	if err != nil {
		t.Fatal(err)
	}
	if status != "stale" {
		t.Fatalf("expected stale lock, got %q", status)
	}
}

func TestRequestStopActiveStopsCurrentLock(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, os.Getpid(), "token", LockHeartbeatTimeout); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestStopActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := store.LockStatus(context.Background(), scanResult.RepositoryID, "token")
	if err != nil {
		t.Fatal(err)
	}
	if status != "stopping" {
		t.Fatalf("expected stopping lock, got %q", status)
	}
}

func TestPauseResumeActiveLock(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	store := NewStore(db)
	scanResult, err := NewScanner(store).Scan(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, os.Getpid(), "token", LockHeartbeatTimeout); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestPauseActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := store.LockStatus(context.Background(), scanResult.RepositoryID, "token")
	if err != nil {
		t.Fatal(err)
	}
	if status != "paused" {
		t.Fatalf("expected paused lock, got %q", status)
	}
	if _, err := store.HeartbeatLock(context.Background(), scanResult.RepositoryID, "token"); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestResumeActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err = store.LockStatus(context.Background(), scanResult.RepositoryID, "token")
	if err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("expected active lock, got %q", status)
	}
}

func TestRunnerEmitsChangeCounter(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 32)
	ready := make(chan RunnerResult, 1)
	done := make(chan error, 1)
	go func() {
		_, err := NewRunner(NewStore(db)).Run(ctx, RunnerOptions{
			Path:              repo,
			PollInterval:      time.Hour,
			HeartbeatInterval: time.Hour,
			SummaryInterval:   10 * time.Millisecond,
			Embedding:         EmbeddingConfig{Provider: "none"},
			Events:            events,
			Ready:             ready,
		})
		done <- err
		close(events)
	}()

	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("runner exited before ready: %v", err)
	case <-time.After(time.Second):
		t.Fatal("runner did not become ready")
	}

	for {
		select {
		case event := <-events:
			if event.Type == "watch.changeCounter" {
				counter, ok := event.Data.(ChangeCounter)
				if !ok {
					t.Fatalf("unexpected counter payload: %#v", event.Data)
				}
				if counter.TotalChangesProcessed != 0 || counter.IntervalChangesProcessed != 0 {
					t.Fatalf("unexpected idle counter: %+v", counter)
				}
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("runner did not stop")
				}
				return
			}
		case err := <-done:
			t.Fatalf("runner exited before counter: %v", err)
		case <-time.After(time.Second):
			t.Fatal("runner did not emit change counter")
		}
	}
}

func TestRunnerResolvesSubdirectoryToRepositoryRootBeforeReady(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "cmd/app/main.go", `package main

func Main() {}
`)
	subdir := filepath.Join(repo, "cmd", "app")

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 32)
	ready := make(chan RunnerResult, 1)
	done := make(chan error, 1)
	go func() {
		_, err := NewRunner(NewStore(db)).Run(ctx, RunnerOptions{
			Path:              subdir,
			PollInterval:      time.Hour,
			HeartbeatInterval: time.Hour,
			SummaryInterval:   time.Hour,
			Embedding:         EmbeddingConfig{Provider: "none"},
			Events:            events,
			Ready:             ready,
		})
		done <- err
		close(events)
	}()

	var result RunnerResult
	select {
	case result = <-ready:
	case err := <-done:
		t.Fatalf("runner exited before ready: %v", err)
	case <-time.After(time.Second):
		t.Fatal("runner did not become ready")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not stop")
	}
	expectedRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	actualRoot, err := filepath.EvalSymlinks(result.Repository.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if actualRoot != expectedRoot {
		t.Fatalf("expected runner repository root %q, got %q", expectedRoot, actualRoot)
	}
	if result.InitialScan.RepositoryID == 0 || result.InitialRep.RepositoryID == 0 {
		t.Fatalf("expected initial scan and representation before ready, got %+v", result)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tld.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := sqlitevec.Register(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{"001_init.sql", "002_watch_raw_code_graph.sql", "003_watch_materialized_workspace.sql", "004_watch_runtime_git_versions.sql", "005_watch_embeddings_identity_vec.sql", "006_workspace_read_indexes.sql", "007_watch_version_resources.sql", "008_watch_diff_line_counts.sql", "009_watch_commit_messages.sql"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "migrations", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO views(owner_element_id, name, description, level_label, level, created_at, updated_at) VALUES (NULL, 'Workspace', 'Local offline workspace', 'Root', 1, 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func symbolElementID(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
		SELECT id FROM elements
		WHERE name = ? AND kind = 'function'`, name).Scan(&id); err != nil {
		t.Fatalf("find symbol element %s: %v", name, err)
	}
	return id
}

func connectorCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM connectors`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected at least one generated connector")
	}
	return count
}

func elementKindCount(t *testing.T, db *sql.DB, kind string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM elements WHERE kind = ?`, kind).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func materializationOwnerTypeCount(t *testing.T, db *sql.DB, ownerType string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM watch_materialization WHERE owner_type = ?`, ownerType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

type workspaceCount struct {
	Views      int
	Elements   int
	Placements int
	Connectors int
}

func workspaceCounts(t *testing.T, db *sql.DB) workspaceCount {
	t.Helper()
	var count workspaceCount
	for query, dest := range map[string]*int{
		`SELECT COUNT(*) FROM views`:      &count.Views,
		`SELECT COUNT(*) FROM elements`:   &count.Elements,
		`SELECT COUNT(*) FROM placements`: &count.Placements,
		`SELECT COUNT(*) FROM connectors`: &count.Connectors,
	} {
		if err := db.QueryRow(query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	return count
}

func countElementTag(t *testing.T, db *sql.DB, tag string) int {
	t.Helper()
	rows, err := db.Query(`SELECT tags FROM elements`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var tags []string
		_ = json.Unmarshal([]byte(raw), &tags)
		for _, item := range tags {
			if item == tag {
				count++
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return count
}

func elementTagsByName(t *testing.T, db *sql.DB, name string) []string {
	t.Helper()
	var raw string
	if err := db.QueryRow(`SELECT tags FROM elements WHERE name = ? ORDER BY id LIMIT 1`, name).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		t.Fatal(err)
	}
	return tags
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func hasDiff(diffs []RepresentationDiff, resourceType, changeType string) bool {
	return findDiff(diffs, resourceType, changeType) != nil
}

func findDiff(diffs []RepresentationDiff, resourceType, changeType string) *RepresentationDiff {
	for _, diff := range diffs {
		if diff.ResourceType != nil && *diff.ResourceType == resourceType && diff.ChangeType == changeType {
			return &diff
		}
	}
	return nil
}

func languageSet(languages []string) map[string]struct{} {
	out := make(map[string]struct{}, len(languages))
	for _, language := range languages {
		out[language] = struct{}{}
	}
	return out
}

func changeSummary(changes []SourceFileChange) string {
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		parts = append(parts, change.Path+":"+change.ChangeType+":"+change.Language)
	}
	return strings.Join(parts, ",")
}

type testPlacement struct {
	placementID int64
	elementID   int64
	x           float64
	y           float64
}

func functionPlacement(t *testing.T, db *sql.DB, name string) testPlacement {
	t.Helper()
	row := db.QueryRow(`
		SELECT p.id, p.element_id, p.position_x, p.position_y
		FROM placements p
		JOIN elements e ON e.id = p.element_id
		WHERE e.kind = 'function' AND (e.name = ? OR e.name LIKE ?)
		ORDER BY p.id
		LIMIT 1`, name, "%."+name)
	var p testPlacement
	if err := row.Scan(&p.placementID, &p.elementID, &p.x, &p.y); err != nil {
		t.Fatalf("function placement %q: %v", name, err)
	}
	return p
}

type countingProvider struct {
	calls      int
	inputs     int
	batchSizes []string
	texts      []string
}

func (p *countingProvider) ModelID() ModelID {
	return ModelID{Provider: "local-deterministic-test", Model: "counting", Dimension: 2, ConfigHash: "counting"}
}

func (p *countingProvider) Embed(_ context.Context, inputs []EmbeddingInput) ([]Vector, error) {
	p.calls++
	p.inputs += len(inputs)
	p.batchSizes = append(p.batchSizes, fmt.Sprint(len(inputs)))
	out := make([]Vector, 0, len(inputs))
	for _, input := range inputs {
		p.texts = append(p.texts, input.Text)
		out = append(out, Vector{1, 2})
	}
	return out, nil
}

type recordingProgress struct {
	starts   []string
	advances int
}

func (p *recordingProgress) Start(label string, total int) {
	p.starts = append(p.starts, fmt.Sprintf("%s:%d", label, total))
}

func (p *recordingProgress) Advance(string) {
	p.advances++
}

func (p *recordingProgress) Finish() {}

func initGitRepoNoCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
