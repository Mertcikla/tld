package watch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func openSettingsTestStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(openTestDB(t))
	repo, err := store.EnsureRepository(context.Background(), RepositoryInput{
		RepoRoot:     t.TempDir(),
		DisplayName:  "repo",
		SettingsHash: "settings",
	})
	if err != nil {
		t.Fatalf("ensure repository: %v", err)
	}
	if repo.ID == 0 {
		t.Fatal("repository id was not assigned")
	}
	return store
}

func TestRepositorySettingsRoundTrip(t *testing.T) {
	store := openSettingsTestStore(t)
	ctx := context.Background()
	repo, err := store.Repositories(ctx)
	if err != nil || len(repo) == 0 {
		t.Fatalf("list repositories: %v", err)
	}
	repositoryID := repo[0].ID

	if _, found, err := store.RepositorySettings(ctx, repositoryID); err != nil || found {
		t.Fatalf("expected no override initially, found=%v err=%v", found, err)
	}

	settings := DefaultSettings()
	settings.Watcher = WatcherPoll
	settings.PollInterval = 42 * time.Second
	settings.Languages = []string{"go"}
	settings.Scale.Strategy = ScanStrategyLimited
	settings.LSP.Enabled = false
	embedding := EmbeddingConfig{Provider: "local-lexical", Dimension: 256}

	saved, err := store.SaveRepositorySettings(ctx, repositoryID, settings, embedding)
	if err != nil {
		t.Fatalf("save repository settings: %v", err)
	}
	if saved.Settings.Watcher != WatcherPoll {
		t.Fatalf("saved watcher = %q, want %q", saved.Settings.Watcher, WatcherPoll)
	}

	loaded, found, err := store.RepositorySettings(ctx, repositoryID)
	if err != nil || !found {
		t.Fatalf("load repository settings: found=%v err=%v", found, err)
	}
	if loaded.Settings.Watcher != WatcherPoll {
		t.Fatalf("loaded watcher = %q, want %q", loaded.Settings.Watcher, WatcherPoll)
	}
	if loaded.Settings.PollInterval != 42*time.Second {
		t.Fatalf("loaded poll interval = %s, want 42s", loaded.Settings.PollInterval)
	}
	if loaded.Embedding.Provider != "local-lexical" || loaded.Embedding.Dimension != 256 {
		t.Fatalf("loaded embedding = %+v, want local-lexical/256", loaded.Embedding)
	}

	// Upsert must not create a duplicate row.
	if _, err := store.SaveRepositorySettings(ctx, repositoryID, settings, embedding); err != nil {
		t.Fatalf("second save: %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_repository_settings WHERE repository_id = ?`, repositoryID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("watch_repository_settings rows = %d, want 1", count)
	}

	if err := store.DeleteRepositorySettings(ctx, repositoryID); err != nil {
		t.Fatalf("delete repository settings: %v", err)
	}
	if _, found, err := store.RepositorySettings(ctx, repositoryID); err != nil || found {
		t.Fatalf("expected override removed, found=%v err=%v", found, err)
	}
}

func TestSaveRepositorySettingsRejectsInvalid(t *testing.T) {
	store := openSettingsTestStore(t)
	ctx := context.Background()
	repos, _ := store.Repositories(ctx)
	repositoryID := repos[0].ID

	settings := DefaultSettings()
	settings.Watcher = "bogus"
	if _, err := store.SaveRepositorySettings(ctx, repositoryID, settings, EmbeddingConfig{Provider: "none"}); err == nil {
		t.Fatal("expected invalid watcher to be rejected")
	} else if !strings.Contains(err.Error(), "watch.watcher") {
		t.Fatalf("error = %v, want watch.watcher validation", err)
	}
}

func TestValidateSettingsAndEmbedding(t *testing.T) {
	if err := ValidateSettings(DefaultSettings()); err != nil {
		t.Fatalf("default settings should validate: %v", err)
	}
	if err := ValidateEmbeddingConfig(EmbeddingConfig{Provider: "local-lexical"}); err != nil {
		t.Fatalf("local-lexical embedding should validate: %v", err)
	}
	invalid := DefaultSettings()
	invalid.Languages = []string{"klingon"}
	if err := ValidateSettings(invalid); err == nil {
		t.Fatal("expected unsupported language to fail validation")
	}
	badEmbedding := EmbeddingConfig{Provider: "openai", Model: "", Endpoint: "", HealthThreshold: 2}
	if err := ValidateEmbeddingConfig(badEmbedding); err == nil {
		t.Fatal("expected invalid openai embedding config to fail")
	}
}

func TestResolveRepositorySettingsLayersOverride(t *testing.T) {
	store := openSettingsTestStore(t)
	ctx := context.Background()
	repos, _ := store.Repositories(ctx)
	repositoryID := repos[0].ID

	base := DefaultSettings()
	baseEmbedding := EmbeddingConfig{Provider: "local-lexical", Dimension: 512}

	resolved, resolvedEmbedding := ResolveRepositorySettings(ctx, store, nil, repositoryID, &base, &baseEmbedding)
	if resolved.Watcher != base.Watcher {
		t.Fatalf("without override watcher = %q, want base %q", resolved.Watcher, base.Watcher)
	}

	override := DefaultSettings()
	override.Watcher = WatcherPoll
	override.PollInterval = 7 * time.Second
	if _, err := store.SaveRepositorySettings(ctx, repositoryID, override, EmbeddingConfig{Provider: "local-lexical", Dimension: 64}); err != nil {
		t.Fatalf("save override: %v", err)
	}

	resolved, resolvedEmbedding = ResolveRepositorySettings(ctx, store, nil, repositoryID, &base, &baseEmbedding)
	if resolved.Watcher != WatcherPoll {
		t.Fatalf("with override watcher = %q, want %q", resolved.Watcher, WatcherPoll)
	}
	if resolvedEmbedding.Dimension != 64 {
		t.Fatalf("with override embedding dimension = %d, want 64", resolvedEmbedding.Dimension)
	}
}

func TestWatchSettingsEndpoints(t *testing.T) {
	store := openSettingsTestStore(t)
	ctx := context.Background()
	repos, _ := store.Repositories(ctx)
	repositoryID := repos[0].ID

	handler := NewHandler(store)
	handler.DataDir = t.TempDir()

	mux := http.NewServeMux()
	handler.Register(mux)

	// Descriptors endpoint lists watch.* keys.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/watch/settings/descriptors", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("descriptors status = %d", rec.Code)
	}
	var descriptors []SettingsDescriptor
	if err := json.Unmarshal(rec.Body.Bytes(), &descriptors); err != nil {
		t.Fatalf("decode descriptors: %v", err)
	}
	if len(descriptors) == 0 {
		t.Fatal("expected watch settings descriptors")
	}
	for _, d := range descriptors {
		if !strings.HasPrefix(d.Key, "watch.") {
			t.Fatalf("descriptor %q is not a watch setting", d.Key)
		}
	}

	// Settings GET returns effective values.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/watch/repositories/"+itoa(repositoryID)+"/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings status = %d: %s", rec.Code, rec.Body.String())
	}
	var current repositorySettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if current.Overridden {
		t.Fatal("expected no override initially")
	}

	// PUT saves an override.
	payload := `{"settings":{"watcher":"poll","poll_interval":15000000000},"embedding":{"provider":"local-lexical","dimension":128}}`
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/watch/repositories/"+itoa(repositoryID)+"/settings", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings status = %d: %s", rec.Code, rec.Body.String())
	}
	var saved repositorySettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode saved settings: %v", err)
	}
	if saved.Settings.Watcher != WatcherPoll || !saved.Overridden {
		t.Fatalf("saved settings = %+v", saved)
	}

	// Invalid PUT returns 400.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/watch/repositories/"+itoa(repositoryID)+"/settings", strings.NewReader(`{"settings":{"watcher":"nope"}}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid put status = %d, want 400", rec.Code)
	}

	// DELETE clears the override.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/watch/repositories/"+itoa(repositoryID)+"/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete settings status = %d", rec.Code)
	}
	if _, found, _ := store.RepositorySettings(ctx, repositoryID); found {
		t.Fatal("expected override removed after DELETE")
	}
}

func TestEmbeddingHealthcheckDisabled(t *testing.T) {
	store := openSettingsTestStore(t)
	ctx := context.Background()
	repos, _ := store.Repositories(ctx)
	repositoryID := repos[0].ID

	handler := NewHandler(store)
	if _, err := store.SaveRepositorySettings(ctx, repositoryID, DefaultSettings(), EmbeddingConfig{Provider: "none"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/watch/repositories/"+itoa(repositoryID)+"/healthcheck/embedding", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("embedding healthcheck status = %d: %s", rec.Code, rec.Body.String())
	}
	var body healthcheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode healthcheck: %v", err)
	}
	if body.OK {
		t.Fatal("expected disabled embedding provider to report not-ok")
	}
	if body.Embedding == nil || body.Embedding.Provider != "none" {
		t.Fatalf("embedding healthcheck payload = %+v", body.Embedding)
	}
}

func TestLSPHealthcheckDisabled(t *testing.T) {
	store := openSettingsTestStore(t)
	ctx := context.Background()
	repos, _ := store.Repositories(ctx)
	repositoryID := repos[0].ID

	handler := NewHandler(store)
	settings := DefaultSettings()
	settings.LSP.Enabled = false
	if _, err := store.SaveRepositorySettings(ctx, repositoryID, settings, EmbeddingConfig{Provider: "none"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/watch/repositories/"+itoa(repositoryID)+"/healthcheck/lsp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("lsp healthcheck status = %d: %s", rec.Code, rec.Body.String())
	}
	var body healthcheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode healthcheck: %v", err)
	}
	if body.LSP == nil {
		t.Fatal("expected LSP status in healthcheck response")
	}
	if body.LSP.Enabled {
		t.Fatal("expected disabled LSP status")
	}
}

func TestSupervisorStartStopWithFakeBinary(t *testing.T) {
	supervisor := NewSupervisor()
	supervisor.executable = func() (string, error) { return "/bin/sleep", nil }
	process, err := supervisor.Start(context.Background(), StartOptions{
		RepositoryID: 1,
		RepoRoot:     t.TempDir(),
		DataDir:      t.TempDir(),
	})
	if err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	if process.PID <= 0 {
		t.Fatalf("expected a pid, got %d", process.PID)
	}
	if _, ok := supervisor.Status(1); !ok {
		t.Fatal("expected tracked process")
	}
	stopped, err := supervisor.Stop(1)
	if err != nil || !stopped {
		t.Fatalf("stop supervisor: stopped=%v err=%v", stopped, err)
	}
	if _, ok := supervisor.Status(1); ok {
		t.Fatal("expected process to be untracked after stop")
	}
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
