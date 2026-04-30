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
	if _, err := store.CreateWatchVersion(context.Background(), scan.RepositoryID, "commit1", "", "main", rep.RepresentationHash, nil, firstDiffs); err != nil {
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
	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, 1234, "token", LockHeartbeatTimeout); err != nil {
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
	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, 1234, "token", LockHeartbeatTimeout); err != nil {
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
	if _, err := store.AcquireLock(context.Background(), scanResult.RepositoryID, 1234, "token", LockHeartbeatTimeout); err != nil {
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
	for _, migration := range []string{"001_init.sql", "002_watch_raw_code_graph.sql", "003_watch_materialized_workspace.sql", "004_watch_runtime_git_versions.sql", "005_watch_embeddings_identity_vec.sql", "006_workspace_read_indexes.sql", "007_watch_version_resources.sql"} {
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

func hasDiff(diffs []RepresentationDiff, resourceType, changeType string) bool {
	for _, diff := range diffs {
		if diff.ResourceType != nil && *diff.ResourceType == resourceType && diff.ChangeType == changeType {
			return true
		}
	}
	return false
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
