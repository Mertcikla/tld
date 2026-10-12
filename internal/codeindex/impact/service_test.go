package impact

import (
	"context"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	localstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestForTargetTagsProgressUpdates(t *testing.T) {
	var got []indexer.Progress
	record := func(update indexer.Progress) { got = append(got, update) }

	forTarget(record, TargetBase)(indexer.Progress{Stage: "discover"})
	forTarget(record, TargetHead)(indexer.Progress{Stage: "discover"})
	forTarget(record, TargetHead)(indexer.Progress{Stage: "overlay"})

	want := []indexer.Progress{
		{Stage: "discover", Target: TargetBase},
		{Stage: "discover", Target: TargetHead},
		{Stage: "overlay", Target: TargetHead},
	}
	if len(got) != len(want) {
		t.Fatalf("updates = %+v", got)
	}
	for i, expected := range want {
		if got[i] != expected {
			t.Fatalf("update %d = %+v, want %+v", i, got[i], expected)
		}
	}
	if forTarget(nil, TargetBase) != nil {
		t.Fatal("nil progress must stay nil")
	}
}

func TestComparisonKeyIncludesRadius(t *testing.T) {
	shallow := ComparisonKey("base", "head", 0)
	deep := ComparisonKey("base", "head", 1)
	if shallow == "" || shallow == deep {
		t.Fatalf("radius must qualify the key: %q vs %q", shallow, deep)
	}
	if ComparisonKey("base", "head", 0) != shallow {
		t.Fatal("key must be deterministic")
	}
}

// TestCompareServesStoredComparison proves a repeat comparison is a cache
// lookup, not a rebuild: after tampering with the stored payload, Compare
// returns the tampered copy.
func TestCompareServesStoredComparison(t *testing.T) {
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	for _, id := range []string{"base", "head"} {
		text := "func A() { return " + id + " }"
		g := graph.NewGraph("repo", id)
		src := &graph.Source{Path: "a.go", Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
		g.Sources[src.Path] = src
		g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, len(text)), text, "", nil)
		snap := &pb.Snapshot{Id: id, RepositoryId: "repo", IngestionStatus: "complete",
			Sources: []*pb.SourceFile{{Path: "a.go", Hash: src.Hash, Size: uint64(len(text))}}}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	service := Service{Workspace: ws, Index: idx}
	request := CompareRequest{
		RepositoryID: "repo",
		Base:         &pb.Revision{SnapshotId: "base"},
		Head:         &pb.Revision{SnapshotId: "head"},
		BlastRadius:  0,
	}
	first, err := service.Compare(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetComparisonKey() != ComparisonKey("base", "head", 0, first.WorkspaceFingerprint) {
		t.Fatalf("comparison key = %q", first.GetComparisonKey())
	}
	if len(first.GetNodes()) != 1 {
		t.Fatalf("nodes = %d, want the single changed file", len(first.GetNodes()))
	}
	marker := model.Clone(first)
	marker.Version = "cached-marker"
	if err := idx.SaveImpact(ctx, marker); err != nil {
		t.Fatal(err)
	}
	second, err := service.CompareSnapshots(ctx, request.RepositoryID, "base", "head", DefaultBlastRadius)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != "cached-marker" {
		t.Fatalf("repeat comparison rebuilt instead of serving the stored payload (version %q)", second.Version)
	}
}

func TestCompareRejectsInvalidRadiusBeforeIndexing(t *testing.T) {
	service := Service{}
	if _, err := service.Compare(context.Background(), CompareRequest{BlastRadius: MaxBlastRadius + 1}); err == nil {
		t.Fatal("invalid comparison radius accepted")
	}
	if _, err := service.CompareSnapshots(context.Background(), "repo", "base", "head", MaxBlastRadius+1); err == nil {
		t.Fatal("invalid snapshot radius accepted")
	}
}

func TestSceneReturnsFrozenCanonicalArtifactWithoutStores(t *testing.T) {
	frozen := &model.Assembly{SchemaVersion: PipelineVersion, RepositoryId: "repo", Warnings: []string{"captured warning"}}
	scene, err := (Service{}).Scene(context.Background(), &model.Diagram{Scene: frozen})
	if err != nil {
		t.Fatal(err)
	}
	scene.Warnings[0] = "reader changed its copy"
	if frozen.Warnings[0] != "captured warning" {
		t.Fatal("scene reader mutated the stored artifact")
	}
}

func TestSceneRejectsObsoleteArtifactsWithoutReadingWorkspace(t *testing.T) {
	for _, scene := range []*model.Assembly{nil, {SchemaVersion: "3"}} {
		if _, err := (Service{}).Scene(context.Background(), &model.Diagram{Scene: scene}); err == nil {
			t.Fatal("obsolete scene accepted")
		}
	}
}
