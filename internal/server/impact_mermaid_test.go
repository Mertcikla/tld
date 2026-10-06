package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func seedImpactComparison(t *testing.T) (string, string) {
	t.Helper()
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	root := "/missing/repo-mermaid"
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
	t.Cleanup(ts.Close)
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{RepositoryId: repoID, Base: &pb.ComparisonTarget{SnapshotId: "base"}, Head: &pb.ComparisonTarget{SnapshotId: "head"}}))
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
	if result == nil || result.ComparisonKey == "" {
		t.Fatalf("comparison: %+v", result)
	}
	return ts.URL, repoID + "|" + result.ComparisonKey
}

func TestImpactMermaidEndpoint(t *testing.T) {
	baseURL, joined := seedImpactComparison(t)
	repoID, comparisonKey, _ := strings.Cut(joined, "|")

	res, err := http.Get(baseURL + "/api/repositories/" + repoID + "/impact/mermaid?markdown=1&comparisonKey=" + url.QueryEscape(comparisonKey))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}
	var payload struct {
		Code     string   `json:"code"`
		Markdown string   `json:"markdown"`
		Warnings []string `json:"warnings"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload.Code, "flowchart LR") {
		t.Fatalf("code missing flowchart: %q", payload.Code)
	}
	if !strings.HasPrefix(payload.Markdown, "```mermaid\n") {
		t.Fatalf("markdown is not a mermaid block: %q", payload.Markdown)
	}
}

func TestImpactMermaidEndpointRequiresKey(t *testing.T) {
	baseURL, joined := seedImpactComparison(t)
	repoID, _, _ := strings.Cut(joined, "|")

	res, err := http.Get(baseURL + "/api/repositories/" + repoID + "/impact/mermaid")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}

func TestImpactMermaidEndpointMissingImpact(t *testing.T) {
	baseURL, joined := seedImpactComparison(t)
	repoID, _, _ := strings.Cut(joined, "|")

	res, err := http.Get(baseURL + "/api/repositories/" + repoID + "/impact/mermaid?comparisonKey=does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", res.StatusCode)
	}
}

// TestImpactMermaidAlwaysRadiusZero verifies the change diagram never includes
// neighbours the blast radius pulled into the canvas overlay.
func TestImpactMermaidAlwaysRadiusZero(t *testing.T) {
	workspaceID := uuid.New()
	ws, routes := newTestServer(t, workspaceID, nil)
	idx := cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
	ctx := app.WithTenantOrgID(context.Background(), workspaceID)
	root := "/missing/repo-mermaid-radius"
	repoID := graph.RepositoryID(root)

	contextElement, err := ws.CreateElement(ctx, core.LibraryElement{Name: "b.go", FilePath: ptrString("b.go")})
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: "map|file|b", Kind: cstore.MappingElement, ResourceID: contextElement.ID, RepositoryID: repoID, SnapshotID: "head"}}); err != nil {
		t.Fatal(err)
	}

	publish := func(id, aText string) {
		g := graph.NewGraph(repoID, id)
		snap := &pb.Snapshot{Id: id, RepositoryId: repoID, IngestionStatus: "complete"}
		a := &graph.Source{Path: "a.go", Text: []byte(aText), Hash: graph.Hash([]byte(aText)), Language: "go"}
		bText := "func B() {}"
		b := &graph.Source{Path: "b.go", Text: []byte(bText), Hash: graph.Hash([]byte(bText)), Language: "go"}
		g.Sources[a.Path] = a
		g.Sources[b.Path] = b
		snap.Sources = []*pb.SourceFile{{Path: a.Path, Hash: a.Hash, Size: uint64(len(a.Text))}, {Path: b.Path, Hash: b.Hash, Size: uint64(len(b.Text))}}
		aFact := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", a.Anchor(0, len(a.Text)), aText, "", nil)
		bFact := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", b.Anchor(0, len(b.Text)), bText, "", nil)
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, aFact.Id, bFact.Id, "", aFact.Anchor, nil)
		if err := idx.Publish(ctx, root, snap, g); err != nil {
			t.Fatal(err)
		}
	}
	publish("base", "func A() { return 1 }")
	publish("head", "func A() { return 2 }")

	ts := httptest.NewServer(routes)
	t.Cleanup(ts.Close)
	client := codeindexv1connect.NewMapperServiceClient(ts.Client(), ts.URL+"/api")
	stream, err := client.CompareRepository(ctx, connect.NewRequest(&pb.CompareRepositoryRequest{
		RepositoryId: repoID,
		Base:         &pb.ComparisonTarget{SnapshotId: "base"},
		Head:         &pb.ComparisonTarget{SnapshotId: "head"},
		Radius:       1,
	}))
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
	if result == nil {
		t.Fatal("comparison produced no diagram")
	}
	hasContext := false
	for _, node := range result.Nodes {
		if node.Context {
			hasContext = true
		}
	}
	if !hasContext {
		t.Fatalf("radius-1 overlay missing context neighbour: %+v", result.Nodes)
	}

	res, err := http.Get(ts.URL + "/api/repositories/" + repoID + "/impact/mermaid?comparisonKey=" + url.QueryEscape(result.ComparisonKey))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload.Code, "(context)") || strings.Contains(payload.Code, "b.go") {
		t.Fatalf("mermaid included radius neighbours:\n%s", payload.Code)
	}
	if !strings.Contains(payload.Code, "a.go") {
		t.Fatalf("mermaid missing direct change:\n%s", payload.Code)
	}
}

func ptrString(value string) *string { return &value }
