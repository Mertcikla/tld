package maprun

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	cgraph "github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	wsstore "github.com/mertcikla/tld/v2/internal/store"
)

func TestFullMapRollsSymbolsUpToFileDependencies(t *testing.T) {
	ctx := context.Background()
	ws, err := wsstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	snap := &pb.Snapshot{Id: "snapshot", RepositoryId: "repo", IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions()}
	g := cgraph.NewGraph("repo", snap.Id)
	for _, p := range []string{"src/a.go", "lib/b.go"} {
		g.AddFact(pb.FactKind_FACT_KIND_FILE, p, "go", &pb.SourceAnchor{Path: p}, "", "", nil)
	}
	a := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", &pb.SourceAnchor{Path: "src/a.go", StartByte: 1, StartLine: 4}, "", "", nil)
	b := g.AddFact(pb.FactKind_FACT_KIND_METHOD, "B", "go", &pb.SourceAnchor{Path: "lib/b.go", StartByte: 1, StartLine: 20}, "", "", nil)
	a2 := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A2", "go", &pb.SourceAnchor{Path: "src/a.go", StartByte: 100, StartLine: 12}, "", "", nil)
	b2 := g.AddFact(pb.FactKind_FACT_KIND_METHOD, "B2", "go", &pb.SourceAnchor{Path: "lib/b.go", StartByte: 100, StartLine: 30}, "", "", nil)
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_REFERENCES, a2.Id, b2.Id, "", &pb.SourceAnchor{Path: "src/a.go", StartByte: 120})
	g.AddFact(pb.FactKind_FACT_KIND_ENV, "TOKEN", "go", &pb.SourceAnchor{Path: "src/a.go", StartByte: 2}, "", "", nil)
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", &pb.SourceAnchor{Path: "src/a.go", StartByte: 10})
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", &pb.SourceAnchor{Path: "src/a.go", StartByte: 20})
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_REFERENCES, a.Id, b.Id, "", &pb.SourceAnchor{Path: "src/a.go", StartByte: 30})
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, b.Id, a.Id, "", &pb.SourceAnchor{Path: "lib/b.go", StartByte: 10})
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, a.Id, "", &pb.SourceAnchor{Path: "src/a.go", StartByte: 40})
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_REFERENCES, a.Id, "", "external.Print", &pb.SourceAnchor{Path: "src/a.go", StartByte: 50})
	if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
		t.Fatal(err)
	}
	deps := Deps{Workspace: ws, Codeindex: idx}
	req := Request{Full: true, RepositoryID: "repo", SnapshotID: snap.Id}
	result, cached, err := Run(ctx, deps, req, nil)
	if err != nil || cached {
		t.Fatalf("run cached=%v err=%v", cached, err)
	}
	if result.Facts != 2 {
		t.Fatalf("materialized facts=%d want 2 files", result.Facts)
	}
	mappings, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	connections := map[string]int{}
	folderConnections := map[string]int{}
	for _, m := range mappings {
		switch m.Kind {
		case cstore.MappingElement:
			e, err := ws.ElementByID(ctx, m.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			if e.Kind == nil || (*e.Kind != "file" && *e.Kind != "directory" && *e.Kind != "repository") {
				t.Fatalf("symbol materialized: %+v", e)
			}
		case cstore.MappingConnector:
			c, err := ws.ConnectorByID(ctx, m.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			from, err := ws.ElementByID(ctx, c.SourceElementID)
			if err != nil {
				t.Fatal(err)
			}
			to, err := ws.ElementByID(ctx, c.TargetElementID)
			if err != nil {
				t.Fatal(err)
			}
			if from.Kind != nil && to.Kind != nil && *from.Kind == "directory" && *to.Kind == "directory" {
				if c.ViewID != result.ViewId {
					t.Fatalf("folder connector is not in repository view: %+v", c)
				}
				folderConnections[from.Name+"->"+to.Name]++
				continue
			}
			if from.FilePath == nil || to.FilePath == nil {
				t.Fatalf("not a file connector: %+v", c)
			}
			if (c.Label != nil && *c.Label != "") || (c.Relationship != nil && *c.Relationship != "") {
				t.Fatalf("dependency still labeled: %+v", c)
			}
			if *from.FilePath == "src/a.go" {
				if c.SourceHandle == nil || !strings.HasSuffix(*c.SourceHandle, "-line-5,13") || c.TargetHandle == nil || !strings.HasSuffix(*c.TargetHandle, "-line-21,31") {
					t.Fatalf("merged connector lost declaration anchors: %+v", c)
				}
			}
			connections[*from.FilePath+"->"+*to.FilePath]++
		}
	}
	if len(connections) != 2 || connections["src/a.go->lib/b.go"] != 1 || connections["lib/b.go->src/a.go"] != 1 {
		t.Fatalf("file dependencies=%v", connections)
	}
	if len(folderConnections) != 2 || folderConnections["src->lib"] != 1 || folderConnections["lib->src"] != 1 {
		t.Fatalf("folder dependencies=%v", folderConnections)
	}
	facts, err := idx.AllFacts(ctx, snap.Id, pb.FactKind_FACT_KIND_UNSPECIFIED)
	if err != nil || len(facts) != 7 {
		t.Fatalf("indexed facts changed: %d %v", len(facts), err)
	}
	again, cached, err := Run(ctx, deps, req, nil)
	if err != nil || !cached || again.ViewId != result.ViewId {
		t.Fatalf("rerun cached=%v err=%v", cached, err)
	}
}
