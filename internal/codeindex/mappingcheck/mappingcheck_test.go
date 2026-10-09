package mappingcheck

import (
	"context"
	"path/filepath"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func TestClassifierUsesLiveMappings(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())

	handle, err := dbrepo.OpenSQLite(ctx, dbrepo.DBOptions{
		SQLitePath: filepath.Join(dataDir, "tld.db"),
		Migrations: assets.FS,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `INSERT INTO elements (id, name, kind, repository_id, file_path, created_at, updated_at)
		VALUES (11, 'a.go', 'file', 'repo', 'src/a.go', 'now', 'now')`); err != nil {
		t.Fatalf("insert element: %v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `INSERT INTO elements (id, name, kind, created_at, updated_at)
		VALUES (12, 'frontend/src', 'component', 'now', 'now')`); err != nil {
		t.Fatalf("insert component: %v", err)
	}
	idx := cstore.NewStoreFromHandle(handle)
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|fact|repo|1", Kind: cstore.MappingElement, ResourceID: 11, RepositoryID: "repo", SnapshotID: "snap"},
		{LogicalKey: "map|group|repo|1", Kind: cstore.MappingElement, ResourceID: 12, RepositoryID: "repo", SnapshotID: "snap"},
	}); err != nil {
		t.Fatalf("save mappings: %v", err)
	}
	_ = handle.Close()

	classify := Classifier(ctx, dataDir)
	if classify == nil {
		t.Fatal("expected a classifier backed by the database")
	}
	if !classify(&workspace.Element{RepositoryID: "repo", FilePath: "src/a.go"}) {
		t.Fatal("expected mapped element to be classified as codeindex-owned")
	}
	if classify(&workspace.Element{RepositoryID: "repo", FilePath: "src/other.go"}) {
		t.Fatal("element without a mapping must not be classified as codeindex-owned")
	}
	if classify(&workspace.Element{RepositoryID: "repo"}) {
		t.Fatal("repository_id alone must not classify an element")
	}
	if !classify(&workspace.Element{Name: "frontend/src", Kind: "component"}) {
		t.Fatal("expected path-less mapped component to be classified by name")
	}
	if classify(&workspace.Element{Name: "hand-authored", Kind: "component"}) {
		t.Fatal("unmapped component must not be classified as codeindex-owned")
	}
}

func TestClassifierWithoutDatabase(t *testing.T) {
	if classify := Classifier(context.Background(), t.TempDir()); classify != nil {
		t.Fatal("expected nil classifier when no database exists")
	}
}
