package api

import (
	"context"
	"fmt"
	"strings"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
)

type OrgService struct {
	diagv1connect.UnimplementedOrgServiceHandler
	Store Store
	Hooks WorkspaceHooks
}

func (s *OrgService) hooks() WorkspaceHooks {
	if s.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return s.Hooks
}

func (s *OrgService) ListTagColors(ctx context.Context, _ *connect.Request[diagv1.ListTagColorsRequest]) (*connect.Response[diagv1.ListTagColorsResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}
	tags, err := s.Store.Tags(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&diagv1.ListTagColorsResponse{Tags: tags}), nil
}

func (s *OrgService) UpdateTag(ctx context.Context, req *connect.Request[diagv1.UpdateTagRequest]) (*connect.Response[diagv1.UpdateTagResponse], error) {
	m := req.Msg
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "tags"); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(m.GetTag())
	color := strings.TrimSpace(m.GetColor())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("tag is required"))
	}
	if color == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("color is required"))
	}
	if err := s.Store.UpdateTag(ctx, workspaceID, name, color, m.Description); err != nil {
		return nil, err
	}
	resp := &diagv1.UpdateTagResponse{}
	s.hooks().AfterWrite(ctx, workspaceID, "update", "tag", name, map[string]any{"color": color}, resp)
	return connect.NewResponse(resp), nil
}
