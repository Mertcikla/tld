package exec

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

type testFlowService struct {
	diagv1connect.UnimplementedFlowServiceHandler
	flow *diagv1.Flow
	org  string
}

func (s *testFlowService) ListFlows(_ context.Context, req *connect.Request[diagv1.ListFlowsRequest]) (*connect.Response[diagv1.ListFlowsResponse], error) {
	if req.Msg.OrgId != s.org {
		return nil, fmt.Errorf("missing workspace scope")
	}
	out := &diagv1.ListFlowsResponse{}
	if s.flow != nil {
		out.Flows = []*diagv1.Flow{s.flow}
	}
	return connect.NewResponse(out), nil
}
func (s *testFlowService) SaveFlow(_ context.Context, req *connect.Request[diagv1.SaveFlowRequest]) (*connect.Response[diagv1.SaveFlowResponse], error) {
	if req.Msg.OrgId != s.org {
		return nil, fmt.Errorf("missing workspace scope")
	}
	if req.Msg.Id != "" && req.Msg.ExpectedRevision != s.flow.Revision {
		return nil, connect.NewError(connect.CodeAborted, fmt.Errorf("stale flow"))
	}
	id := req.Msg.Id
	if id == "" {
		id = "flow-id"
	}
	s.flow = &diagv1.Flow{Id: id, ViewId: req.Msg.ViewId, Revision: req.Msg.ExpectedRevision + 1, Content: req.Msg.Content}
	return connect.NewResponse(&diagv1.SaveFlowResponse{Flow: s.flow}), nil
}
func (s *testFlowService) DeleteFlow(_ context.Context, req *connect.Request[diagv1.DeleteFlowRequest]) (*connect.Response[diagv1.DeleteFlowResponse], error) {
	if req.Msg.OrgId != s.org || req.Msg.ExpectedRevision != s.flow.Revision {
		return nil, fmt.Errorf("incorrect delete scope or revision")
	}
	s.flow = nil
	return connect.NewResponse(&diagv1.DeleteFlowResponse{}), nil
}
func TestRemoteFlowRunnerUsesExistingService(t *testing.T) {
	svc := &testFlowService{org: "workspace-id"}
	path, handler := diagv1connect.NewFlowServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing bearer authentication")
		}
		mux.ServeHTTP(w, r)
	}))
	defer server.Close()
	runner, err := NewRunner(workspace.Config{ServerURL: server.URL, APIKey: "secret", WorkspaceID: svc.org}, "remote", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	fr := runner.(FlowRunner)
	ctx := context.Background()
	created, err := fr.SaveFlow(ctx, &diagv1.SaveFlowRequest{ViewId: 1, Content: &diagv1.FlowContent{Name: "Checkout"}})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := fr.ListFlows(ctx, 0)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list failed: %v %v", listed, err)
	}
	_, err = fr.SaveFlow(ctx, &diagv1.SaveFlowRequest{Id: created.Id, ViewId: 1, ExpectedRevision: 99, Content: created.Content})
	if connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("expected revision conflict: %v", err)
	}
	if err = fr.DeleteFlow(ctx, &diagv1.DeleteFlowRequest{Id: created.Id, ViewId: 1, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("unexpected HTTP calls: %d", calls)
	}
	if !strings.HasSuffix(fr.TargetLabel(), "/api") {
		t.Fatal("target URL not normalized")
	}
}
