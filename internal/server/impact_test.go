package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/mermaid"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/proto"
)

func TestCompareRepositoryNoCheckout(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	root := "/missing/repo"
	repoID := graph.RepositoryID(root)
	for _, id := range []string{"base", "head"} {
		text := "func A() { return " + id + " }"
		g := graph.NewGraph(repoID, id)
		src := &graph.Source{Path: "a.go", Text: []byte(text), Hash: graph.Hash([]byte(text)), Language: "go"}
		g.Sources[src.Path] = src
		g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, len(text)), text, "", nil)
		snap := &pb.Snapshot{Id: id, RepositoryId: repoID, IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions(), Sources: []*pb.SourceFile{{Path: "a.go", Hash: src.Hash, Size: uint64(len(text))}}}
		if err := idx.Publish(ctx, root, snap, g); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{RepositoryId: repoID, Base: &pb.Revision{SnapshotId: "base"}, Head: &pb.Revision{SnapshotId: "head"}}))
	if err != nil {
		t.Fatal(err)
	}
	var result *pb.ImpactScene
	for stream.Receive() {
		if found := stream.Msg().GetResult(); found != nil {
			result = found
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Files) != 1 || len(result.Symbols) != 1 {
		t.Fatalf("comparison: %+v", result)
	}
	for _, view := range result.Views {
		for _, p := range view.Placements {
			if p.Distance != 0 {
				t.Fatal("direct change distance is not zero")
			}
		}
	}

	mermaid, err := client.ExportImpactMermaid(ctx, connect.NewRequest(&pb.ExportImpactMermaidRequest{
		RepositoryId: repoID, ComparisonKey: result.Comparison.Key, Markdown: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mermaid.Msg.GetCode(), "flowchart LR") || !strings.Contains(mermaid.Msg.GetCode(), "a.go") {
		t.Fatalf("mermaid code: %q", mermaid.Msg.GetCode())
	}
	// The docked diagram renders the scene, not the file graph: the uncovered
	// file surfaces as a transient in the fallback view, exactly as drawn.
	if !strings.Contains(mermaid.Msg.GetCode(), `subgraph view_neg1["Changes"]`) {
		t.Fatalf("mermaid code missing fallback view: %q", mermaid.Msg.GetCode())
	}
	plain, err := client.ExportImpactMermaid(ctx, connect.NewRequest(&pb.ExportImpactMermaidRequest{
		RepositoryId: repoID, ComparisonKey: result.Comparison.Key, Plain: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.Msg.GetCode(), "flowchart LR") || !strings.Contains(plain.Msg.GetCode(), "a.go") {
		t.Fatalf("plain mermaid code: %q", plain.Msg.GetCode())
	}
	if !strings.HasPrefix(mermaid.Msg.GetMarkdown(), "```mermaid\n") {
		t.Fatalf("mermaid markdown: %q", mermaid.Msg.GetMarkdown())
	}
	scene, err := client.GetImpactScene(ctx, connect.NewRequest(&pb.GetImpactSceneRequest{RepositoryId: repoID, ComparisonKey: result.Comparison.Key}))
	if err != nil {
		t.Fatal(err)
	}
	if scene.Msg == nil {
		t.Fatal("scene missing")
	}
	snapshots, err := idx.Snapshots(ctx, repoID)
	if err != nil || len(snapshots) != 2 {
		t.Fatal("scene or mermaid indexed sources")
	}
}

// Every supported RPC radius must match the shared service cache and survive
// scene and Mermaid export without being narrowed to a server default.
func TestCompareRepositoryUsesRequestedBlastRadius(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID("/repo")
	view, err := ws.CreateView(ctx, "Context", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"b.go", "c.go"} {
		filePath, name, owner := path, path, repoID
		element, err := ws.CreateElement(ctx, core.LibraryElement{Name: name, FilePath: &filePath, RepositoryID: &owner})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ws.AddPlacement(ctx, view.ID, element.ID, 0, 0); err != nil {
			t.Fatal(err)
		}
		if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
			{LogicalKey: "map|file|" + path, Kind: cstore.MappingElement, ResourceID: element.ID, RepositoryID: repoID, SnapshotID: "head"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	publish := func(id, aText string) {
		g := graph.NewGraph(repoID, id)
		files := map[string]string{"a.go": aText, "b.go": "func B() {}", "c.go": "func C() {}"}
		snap := &pb.Snapshot{Id: id, RepositoryId: repoID, IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions()}
		facts := map[string]*pb.CodeFact{}
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "Fn", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		// Chain a.go -> b.go -> c.go: c.go sits two hops out.
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts["a.go"].Id, facts["b.go"].Id, "", facts["a.go"].Anchor)
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts["b.go"].Id, facts["c.go"].Id, "", facts["b.go"].Anchor)
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", "func A() { return 1 }")
	publish("head", "func A() { return 2 }")
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	for _, radius := range []uint32{0, 1, 2, 3} {
		t.Run(fmt.Sprint(radius), func(t *testing.T) {
			stream, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{
				RepositoryId: repoID, Base: &pb.Revision{SnapshotId: "base"}, Head: &pb.Revision{SnapshotId: "head"}, ContextDepth: radius,
			}))
			var result *pb.ImpactScene
			if err == nil {
				for stream.Receive() {
					if found := stream.Msg().GetResult(); found != nil {
						result = found
					}
				}
				err = stream.Err()
			}
			if radius > impact.MaxBlastRadius {
				if connect.CodeOf(err) != connect.CodeInvalidArgument {
					t.Fatalf("invalid radius: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result == nil {
				t.Fatal("missing canonical scene")
			}
			placementsInResult := 0
			for _, view := range result.Views {
				for _, p := range view.Placements {
					placementsInResult++
					if p.Distance > radius {
						t.Fatal("node beyond radius")
					}
				}
			}
			if placementsInResult != int(radius)+1 {
				t.Fatalf("radius %d: %d placements", radius, placementsInResult)
			}

			cached, err := (impact.Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, repoID, "base", "head", radius)
			if err != nil || cached.Version != result.GetComparison().GetVersion() || cached.GetComparisonKey() != result.GetComparison().GetKey() {
				t.Fatalf("comparison cache not shared: %v", err)
			}
			expected := impact.PublicScene(impact.SelectScene(cached.Scene, radius, false), false)
			if !proto.Equal(expected, result) {
				t.Fatal("RPC differs from the canonical CLI/service projection")
			}

			scene, err := client.GetImpactScene(ctx, connect.NewRequest(&pb.GetImpactSceneRequest{RepositoryId: repoID, ComparisonKey: result.Comparison.Key}))
			if err != nil {
				t.Fatal(err)
			}
			var placements int
			for _, view := range scene.Msg.GetViews() {
				placements += len(view.GetPlacements())
			}
			if placements != int(radius)+1 {
				t.Fatalf("scene radius %d: %d placements", radius, placements)
			}
			code, err := client.ExportImpactMermaid(ctx, connect.NewRequest(&pb.ExportImpactMermaidRequest{RepositoryId: repoID, ComparisonKey: result.Comparison.Key}))
			if err != nil {
				t.Fatal(err)
			}
			if code.Msg.Code != mermaid.ExportImpactScene(expected, mermaid.SceneExportOptions{IncludeMetadata: true}) {
				t.Fatal("RPC Mermaid differs from canonical projection")
			}

			for hop, path := range []string{"a.go", "b.go", "c.go"} {
				if strings.Contains(code.Msg.GetCode(), path) != (uint32(hop) <= radius) {
					t.Fatalf("radius %d, path %s: %s", radius, path, code.Msg.GetCode())
				}
			}
		})
	}
}

// TestExportImpactMermaidKeepsBlastRadius proves the served scene and diagram
// carry the blast radius only: the impacted file and its one-hop neighbour
// render, while far unimpacted members never reach the payload.
func TestExportImpactMermaidKeepsBlastRadius(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID("/repo")
	mine, err := ws.CreateView(ctx, "Mine", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	link := func(name, filePath string) int64 {
		t.Helper()
		element, err := ws.CreateElement(ctx, core.LibraryElement{Name: name, FilePath: &filePath, RepositoryID: &repoID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ws.AddPlacement(ctx, mine.ID, element.ID, 0, 0); err != nil {
			t.Fatal(err)
		}
		return element.ID
	}
	link("Changed File", "a.go")
	neighbourID := link("Neighbour File", "b.go")
	link("Far File", "c.go")
	// Only the neighbour is mapped, so only it can surface as diagram
	// context; the far file stays a plain authored member with no overlay.
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|file|b", Kind: cstore.MappingElement, ResourceID: neighbourID, RepositoryID: repoID, SnapshotID: "head"},
	}); err != nil {
		t.Fatal(err)
	}
	publish := func(id, aText string) {
		g := graph.NewGraph(repoID, id)
		files := map[string]string{"a.go": aText, "b.go": "func B() {}", "c.go": "func C() {}"}
		snap := &pb.Snapshot{Id: id, RepositoryId: repoID, IngestionStatus: "complete", ToolVersions: indexer.SnapshotVersions()}
		facts := map[string]*pb.CodeFact{}
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts["a.go"].Id, facts["b.go"].Id, "", facts["a.go"].Anchor)
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", "func A() { return 1 }")
	publish("head", "func A() { return 2 }")
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{RepositoryId: repoID, Base: &pb.Revision{SnapshotId: "base"}, Head: &pb.Revision{SnapshotId: "head"}, ContextDepth: 1}))
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for stream.Receive() {
		if found := stream.Msg().GetResult(); found != nil {
			key = found.GetComparison().GetKey()
		}
	}
	if err := stream.Err(); err != nil || key == "" {
		t.Fatalf("comparison: %v", err)
	}
	scene, err := client.GetImpactScene(ctx, connect.NewRequest(&pb.GetImpactSceneRequest{RepositoryId: repoID, ComparisonKey: key}))
	if err != nil {
		t.Fatal(err)
	}
	var placements int
	for _, content := range scene.Msg.GetViews() {
		placements += len(content.GetPlacements())
	}
	if placements != 3 {
		t.Fatalf("standard scene must preserve authored context, got %d placements: %+v", placements, scene.Msg.GetViews())
	}
	for _, plain := range []bool{false, true} {
		code, err := client.ExportImpactMermaid(ctx, connect.NewRequest(&pb.ExportImpactMermaidRequest{RepositoryId: repoID, ComparisonKey: key, Plain: plain}))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Changed File", "Neighbour File"} {
			if !strings.Contains(code.Msg.GetCode(), want) {
				t.Fatalf("plain=%v diagram lost %q:\n%s", plain, want, code.Msg.GetCode())
			}
		}
		if strings.Contains(code.Msg.GetCode(), "Far File") != !plain {
			t.Fatalf("plain=%v diagram kept a far unimpacted member:\n%s", plain, code.Msg.GetCode())
		}
	}
}
