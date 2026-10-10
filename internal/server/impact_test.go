package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/pkg/app"
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
		snap := &pb.Snapshot{Id: id, RepositoryId: repoID, IngestionStatus: "complete", Sources: []*pb.SourceFile{{Path: "a.go", Hash: src.Hash, Size: uint64(len(text))}}}
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
	var result *pb.ImpactDiagram
	for stream.Receive() {
		if found := stream.Msg().GetResult(); found != nil {
			result = found
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if result == nil || result.ViewId != 0 || len(result.Nodes) != 1 || len(result.Nodes[0].Symbols.Modified) != 1 {
		t.Fatalf("comparison: %+v", result)
	}
	if result.Nodes[0].Distance != 0 {
		t.Fatalf("direct change distance = %d, want 0", result.Nodes[0].Distance)
	}
	mermaid, err := client.ExportImpactMermaid(ctx, connect.NewRequest(&pb.ExportImpactMermaidRequest{
		RepositoryId: repoID, ComparisonKey: result.ComparisonKey, Markdown: true,
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
		RepositoryId: repoID, ComparisonKey: result.ComparisonKey, Plain: true,
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
	scene, err := client.GetImpactScene(ctx, connect.NewRequest(&pb.GetImpactSceneRequest{RepositoryId: repoID, ComparisonKey: result.ComparisonKey}))
	if err != nil {
		t.Fatal(err)
	}
	if scene.Msg.GetScene() == nil {
		t.Fatal("scene missing")
	}
	snapshots, err := idx.Snapshots(ctx, repoID)
	if err != nil || len(snapshots) != 2 {
		t.Fatal("scene or mermaid indexed sources")
	}
}

// TestExportImpactMermaidPlainHidesUnimpacted proves the plain request prunes
// placements without change impact while the standard request keeps the full
// authored membership.
func TestExportImpactMermaidPlainHidesUnimpacted(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	repoID := graph.RepositoryID("/repo")
	mine, err := ws.CreateView(ctx, "Mine", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	link := func(name, filePath string) {
		t.Helper()
		element, err := ws.CreateElement(ctx, core.LibraryElement{Name: name, FilePath: &filePath, RepositoryID: &repoID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ws.AddPlacement(ctx, mine.ID, element.ID, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	link("Changed File", "a.go")
	link("Quiet File", "b.go")
	publish := func(id, aText string) {
		g := graph.NewGraph(repoID, id)
		files := map[string]string{"a.go": aText, "b.go": "func B() {}"}
		snap := &pb.Snapshot{Id: id, RepositoryId: repoID, IngestionStatus: "complete"}
		for path, text := range files {
			src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
			g.Sources[path] = src
			snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash, Size: uint64(len(src.Text))})
			g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, len(src.Text)), text, "", nil)
		}
		if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", "func A() { return 1 }")
	publish("head", "func A() { return 2 }")
	ts := httptest.NewServer(routes)
	defer ts.Close()
	client := codeindexv1connect.NewCodeIndexServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{RepositoryId: repoID, Base: &pb.Revision{SnapshotId: "base"}, Head: &pb.Revision{SnapshotId: "head"}}))
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for stream.Receive() {
		if found := stream.Msg().GetResult(); found != nil {
			key = found.GetComparisonKey()
		}
	}
	if err := stream.Err(); err != nil || key == "" {
		t.Fatalf("comparison: %v", err)
	}
	standard, err := client.ExportImpactMermaid(ctx, connect.NewRequest(&pb.ExportImpactMermaidRequest{RepositoryId: repoID, ComparisonKey: key}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(standard.Msg.GetCode(), "Quiet File") {
		t.Fatalf("standard diagram must keep unimpacted members:\n%s", standard.Msg.GetCode())
	}
	plain, err := client.ExportImpactMermaid(ctx, connect.NewRequest(&pb.ExportImpactMermaidRequest{RepositoryId: repoID, ComparisonKey: key, Plain: true}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.Msg.GetCode(), "Changed File") {
		t.Fatalf("plain diagram lost the impacted member:\n%s", plain.Msg.GetCode())
	}
	if strings.Contains(plain.Msg.GetCode(), "Quiet File") {
		t.Fatalf("plain diagram kept an unimpacted member:\n%s", plain.Msg.GetCode())
	}
}
