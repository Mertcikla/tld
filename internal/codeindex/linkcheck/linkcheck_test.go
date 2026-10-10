package linkcheck

import (
	"context"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func TestIndexReadsLatestSnapshotPerRepository(t *testing.T) {
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
	idx := cstore.NewStoreFromHandle(handle)
	publishSnapshot(t, ctx, idx, "snapshot-old", "repo-a", "/tmp/repo-a", []string{"a/old.go"})
	publishSnapshot(t, ctx, idx, "snapshot-new", "repo-a", "/tmp/repo-a", []string{"a/b/new.go", "root.go"})
	publishSnapshot(t, ctx, idx, "snapshot-b", "repo-b", "/tmp/repo-b", []string{"b/one.go"})
	// A repository without an indexed snapshot must be skipped.
	if _, err := handle.DB.ExecContext(ctx, `INSERT INTO codeindex_repositories (id, root, created_at, updated_at, org_id) VALUES ('repo-c', '/tmp/repo-c', 'now', 'now', '00000000-0000-0000-0000-000000000000')`); err != nil {
		t.Fatalf("insert empty repository: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close handle: %v", err)
	}

	targets, ok := Index(ctx, dataDir)
	if !ok {
		t.Fatal("expected an index backed by the database")
	}
	if len(targets) != 2 {
		t.Fatalf("repositories = %d, want 2: %+v", len(targets), targets)
	}
	var found bool
	for _, target := range targets {
		if target.ID != "repo-a" {
			continue
		}
		found = true
		if target.Name != "repo-a" {
			t.Fatalf("name = %q, want %q", target.Name, "repo-a")
		}
		if target.Root != "/tmp/repo-a" {
			t.Fatalf("root = %q", target.Root)
		}
		if len(target.Paths) != 2 || target.Paths[0] != "a/b/new.go" || target.Paths[1] != "root.go" {
			t.Fatalf("paths = %v, want the latest snapshot's files", target.Paths)
		}
	}
	if !found {
		t.Fatalf("repo-a missing from %+v", targets)
	}
}

func TestIndexWithoutDatabase(t *testing.T) {
	if _, ok := Index(context.Background(), t.TempDir()); ok {
		t.Fatal("expected no index when no database exists")
	}
}

func publishSnapshot(t *testing.T, ctx context.Context, idx *cstore.Store, snapshotID, repositoryID, root string, paths []string) {
	t.Helper()
	snapshot := &pb.Snapshot{
		Id:              snapshotID,
		RepositoryId:    repositoryID,
		IngestionStatus: "complete",
	}
	for _, path := range paths {
		snapshot.Sources = append(snapshot.Sources, &pb.SourceFile{Path: path, Hash: "hash-" + path, Size: 1})
	}
	g := graph.NewGraph(repositoryID, snapshotID)
	for _, path := range paths {
		g.Sources[path] = &graph.Source{Path: path}
	}
	if err := idx.Publish(ctx, root, snapshot, g); err != nil {
		t.Fatalf("publish %s: %v", snapshotID, err)
	}
}
