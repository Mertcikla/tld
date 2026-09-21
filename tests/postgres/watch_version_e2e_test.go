//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/v2/internal/watch"
)

// TestPostgresWatchVersionRecording guards the dialect-sensitive version
// recording path: the range-column patch and the watch_version_resources
// upsert must work on Postgres, and re-recording the same version must not
// duplicate diff rows.
func TestPostgresWatchVersionRecording(t *testing.T) {
	dsn := requirePostgresDSN(t)
	store := openPostgresLocalStore(t, dsn)
	ctx := context.Background()
	watchStore := watch.NewStoreWithBun(store.DB(), store.BunDB(), store.Dialect())

	repo, err := watchStore.EnsureRepository(ctx, watch.RepositoryInput{
		RepoRoot:     t.TempDir(),
		DisplayName:  "repo",
		SettingsHash: "settings",
	})
	if err != nil {
		t.Fatalf("ensure repository: %v", err)
	}
	if _, _, err := watchStore.UpsertFile(ctx, repo.ID, "main.go", "go", "blob", "worktree", 12, 1, "parsed", nil); err != nil {
		t.Fatalf("upsert file: %v", err)
	}

	diff := watch.RepresentationDiff{OwnerType: "file", OwnerKey: "main.go", ChangeType: "added"}
	first, err := watchStore.CreateWatchVersion(ctx, repo.ID, "commit-1", "init", "", "main", "rep-1", nil, []watch.RepresentationDiff{diff})
	if err != nil {
		t.Fatalf("first CreateWatchVersion on postgres: %v", err)
	}
	second, err := watchStore.CreateWatchVersion(ctx, repo.ID, "commit-1", "init", "", "main", "rep-1", nil, []watch.RepresentationDiff{diff})
	if err != nil {
		t.Fatalf("second CreateWatchVersion on postgres: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("re-recording created a new version: first=%d second=%d", first.ID, second.ID)
	}

	var resourceCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_version_resources WHERE version_id = $1`, first.ID).Scan(&resourceCount); err != nil {
		t.Fatalf("count watch_version_resources: %v", err)
	}
	if resourceCount != 1 {
		t.Fatalf("watch_version_resources count = %d, want 1 (upsert, not duplicate)", resourceCount)
	}

	var diffCount int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_representation_diffs WHERE version_id = $1`, first.ID).Scan(&diffCount); err != nil {
		t.Fatalf("count watch_representation_diffs: %v", err)
	}
	if diffCount != 1 {
		t.Fatalf("watch_representation_diffs count = %d, want 1 (idempotent re-record)", diffCount)
	}

	for _, column := range []string{"file_path", "start_line", "end_line"} {
		var exists bool
		err := store.DB().QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'watch_version_resources' AND column_name = $1)`, column).Scan(&exists)
		if err != nil {
			t.Fatalf("check column %s: %v", column, err)
		}
		if !exists {
			t.Fatalf("watch_version_resources missing column %s", column)
		}
	}
}
