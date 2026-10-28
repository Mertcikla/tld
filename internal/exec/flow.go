package exec

import (
	"context"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/pkg/api"
)

// FlowRunner extends target operations without expanding the workspace runner
// contract used by other CLI commands and their mocks.
type FlowRunner interface {
	Runner
	ListFlows(context.Context, int32) ([]*diagv1.Flow, error)
	SaveFlow(context.Context, *diagv1.SaveFlowRequest) (*diagv1.Flow, error)
	DeleteFlow(context.Context, *diagv1.DeleteFlowRequest) error
	ViewContent(context.Context, int32) (*diagv1.ViewContent, error)
}

var _ FlowRunner = (*localRunner)(nil)
var _ FlowRunner = (*remoteRunner)(nil)

func (r *remoteRunner) ListFlows(ctx context.Context, viewID int32) ([]*diagv1.Flow, error) {
	resp, err := client.NewFlow(r.serverURL, r.apiKey).ListFlows(ctx, connect.NewRequest(&diagv1.ListFlowsRequest{OrgId: r.orgID, ViewId: viewID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Flows, nil
}
func (r *remoteRunner) SaveFlow(ctx context.Context, req *diagv1.SaveFlowRequest) (*diagv1.Flow, error) {
	req.OrgId = r.orgID
	resp, err := client.NewFlow(r.serverURL, r.apiKey).SaveFlow(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Flow, nil
}
func (r *remoteRunner) DeleteFlow(ctx context.Context, req *diagv1.DeleteFlowRequest) error {
	req.OrgId = r.orgID
	_, err := client.NewFlow(r.serverURL, r.apiKey).DeleteFlow(ctx, connect.NewRequest(req))
	return err
}
func (r *remoteRunner) ViewContent(ctx context.Context, viewID int32) (*diagv1.ViewContent, error) {
	resp, err := r.client().GetView(ctx, connect.NewRequest(&diagv1.GetViewRequest{ViewId: viewID, IncludeContent: true}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Content, nil
}
func (r *localRunner) ListFlows(ctx context.Context, viewID int32) ([]*diagv1.Flow, error) {
	svc := &api.FlowService{Store: r.adapter}
	resp, err := svc.ListFlows(r.ctx(ctx), connect.NewRequest(&diagv1.ListFlowsRequest{ViewId: viewID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Flows, nil
}
func (r *localRunner) SaveFlow(ctx context.Context, req *diagv1.SaveFlowRequest) (*diagv1.Flow, error) {
	svc := &api.FlowService{Store: r.adapter}
	resp, err := svc.SaveFlow(r.ctx(ctx), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Flow, nil
}
func (r *localRunner) DeleteFlow(ctx context.Context, req *diagv1.DeleteFlowRequest) error {
	svc := &api.FlowService{Store: r.adapter}
	_, err := svc.DeleteFlow(r.ctx(ctx), connect.NewRequest(req))
	return err
}
func (r *localRunner) ViewContent(ctx context.Context, viewID int32) (*diagv1.ViewContent, error) {
	return r.adapter.GetProjectedViewContent(r.ctx(ctx), viewID, uuid.Nil, nil)
}
