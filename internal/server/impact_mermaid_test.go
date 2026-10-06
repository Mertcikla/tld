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
