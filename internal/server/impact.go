package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mermaid"
)

func impactError(err error) error {
	if errors.Is(err, cstore.ErrBusy) {
		return connect.NewError(connect.CodeAlreadyExists, err)
	}
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, err)
	}
	return connect.NewError(connect.CodeFailedPrecondition, err)
}

func impactLookupError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

// impactService builds the reusable comparison service shared with the CLI.
func (s *codeIndexService) impactService() impact.Service {
	return impact.Service{Workspace: s.ws, Index: s.store, Config: configbridge.FromGlobal(s.config)}
}

func (s *codeIndexService) CompareRepository(ctx context.Context, req *connect.Request[pb.CompareRepositoryRequest], stream *connect.ServerStream[pb.CompareRepositoryEvent]) error {
	input := req.Msg
	if input.RepositoryId == "" || input.Base == nil || input.Head == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository and both comparison targets are required"))
	}
	diagram, err := s.impactService().Compare(ctx, impact.CompareRequest{
		RepositoryID: input.RepositoryId,
		Base:         input.Base,
		Head:         input.Head,
		ContextDepth: input.ContextDepth,
		Progress: func(p indexer.Progress) {
			_ = stream.Send(&pb.CompareRepositoryEvent{Event: &pb.CompareRepositoryEvent_Progress{Progress: &pb.Progress{Stage: p.Stage, Current: uint32(p.Current), Total: uint32(p.Total), Detail: p.Detail}}})
		},
	})
	if err != nil {
		return impactError(err)
	}
	return stream.Send(&pb.CompareRepositoryEvent{Event: &pb.CompareRepositoryEvent_Result{Result: diagram}})
}

func (s *codeIndexService) GetLiveImpact(ctx context.Context, req *connect.Request[pb.ID]) (*connect.Response[pb.LiveImpact], error) {
	if _, err := s.store.Repository(ctx, req.Msg.GetId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	result, err := s.store.LiveImpact(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(result), nil
}

func (s *codeIndexService) ExportImpactMermaid(ctx context.Context, req *connect.Request[pb.ExportImpactMermaidRequest]) (*connect.Response[pb.ExportImpactMermaidResponse], error) {
	if req.Msg.GetRepositoryId() == "" || req.Msg.GetComparisonKey() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository id and comparison key are required"))
	}
	diagram, err := s.store.Impact(ctx, req.Msg.GetRepositoryId(), req.Msg.GetComparisonKey())
	if err != nil {
		return nil, impactLookupError(err)
	}
	scoped := impact.Scope(diagram, req.Msg.GetRadius())
	code := mermaid.ExportImpactDiagram(scoped, mermaid.ImpactExportOptions{IncludeMetadata: true, Radius: req.Msg.GetRadius()})
	response := &pb.ExportImpactMermaidResponse{Code: code}
	if req.Msg.GetMarkdown() {
		response.Markdown = mermaid.MermaidBlock(code)
	}
	return connect.NewResponse(response), nil
}

func (s *codeIndexService) GetImpactScene(ctx context.Context, req *connect.Request[pb.GetImpactSceneRequest]) (*connect.Response[pb.GetImpactSceneResponse], error) {
	if req.Msg.GetRepositoryId() == "" || req.Msg.GetComparisonKey() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository id and comparison key are required"))
	}
	diagram, err := s.store.Impact(ctx, req.Msg.GetRepositoryId(), req.Msg.GetComparisonKey())
	if err != nil {
		return nil, impactLookupError(err)
	}
	scene, err := s.impactService().Scene(ctx, diagram)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.GetImpactSceneResponse{Scene: scene}), nil
}
