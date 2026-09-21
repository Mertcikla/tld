package watch

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

func repositoryRootElementID(t *testing.T, db *sql.DB, repositoryID int64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		SELECT resource_id FROM watch_materialization
		WHERE repository_id = ? AND owner_type = 'repository' AND owner_key = ? AND resource_type = 'element'`,
		repositoryID, fmt.Sprintf("repository:%d", repositoryID)).Scan(&id)
	if err != nil {
		t.Fatalf("repository root element mapping: %v", err)
	}
	return id
}

func TestEnsureRepositoryCreatesRootElementAndView(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store := NewStore(db)
	ctx := context.Background()

	repo, err := store.EnsureRepository(ctx, RepositoryInput{RepoRoot: t.TempDir(), DisplayName: "checkout"})
	if err != nil {
		t.Fatal(err)
	}

	elementID := repositoryRootElementID(t, db, repo.ID)
	var name, kind string
	if err := db.QueryRow(`SELECT name, kind FROM elements WHERE id = ?`, elementID).Scan(&name, &kind); err != nil {
		t.Fatalf("load repository element: %v", err)
	}
	if name != "checkout" || kind != "repository" {
		t.Fatalf("repository element = (%q, %q), want (checkout, repository)", name, kind)
	}

	var rootViewID int64
	if err := db.QueryRow(`SELECT id FROM views WHERE owner_element_id IS NULL ORDER BY id LIMIT 1`).Scan(&rootViewID); err != nil {
		t.Fatal(err)
	}
	var placementCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM placements WHERE view_id = ? AND element_id = ?`, rootViewID, elementID).Scan(&placementCount); err != nil {
		t.Fatal(err)
	}
	if placementCount != 1 {
		t.Fatalf("root placement count = %d, want 1", placementCount)
	}

	var childViewCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM views WHERE owner_element_id = ?`, elementID).Scan(&childViewCount); err != nil {
		t.Fatal(err)
	}
	if childViewCount != 1 {
		t.Fatalf("repository child view count = %d, want 1", childViewCount)
	}
}

func TestDeletingRepositoryRootElementKeepsWatchRepository(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	store := NewStore(db)
	ctx := context.Background()

	repo, err := store.EnsureRepository(ctx, RepositoryInput{RepoRoot: t.TempDir(), DisplayName: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveRepositorySettings(ctx, repo.ID, DefaultSettings(), EmbeddingConfig{Provider: "none"}); err != nil {
		t.Fatalf("save repository settings: %v", err)
	}
	elementID := repositoryRootElementID(t, db, repo.ID)

	// Simulate the workspace delete path (pkg/app.Store.DeleteElement), which
	// cascades to the owned view and placements.
	if _, err := db.Exec(`DELETE FROM elements WHERE id = ?`, elementID); err != nil {
		t.Fatal(err)
	}

	var repoCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM watch_repositories WHERE id = ?`, repo.ID).Scan(&repoCount); err != nil {
		t.Fatal(err)
	}
	if repoCount != 1 {
		t.Fatalf("watch repository count = %d, want 1 after element deletion", repoCount)
	}
	if _, found, err := store.RepositorySettings(ctx, repo.ID); err != nil || !found {
		t.Fatalf("repository settings lost after element deletion: found=%v err=%v", found, err)
	}

	// Re-materializing the root element recreates it without a duplicate repo.
	if err := store.ensureRepositoryRootElement(ctx, repo); err != nil {
		t.Fatal(err)
	}
	recreatedID := repositoryRootElementID(t, db, repo.ID)
	if recreatedID == elementID {
		t.Fatalf("expected a new element id after deletion, got %d", recreatedID)
	}
	var elementCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM elements WHERE id = ?`, recreatedID).Scan(&elementCount); err != nil || elementCount != 1 {
		t.Fatalf("recreated element missing: count=%d err=%v", elementCount, err)
	}
}
