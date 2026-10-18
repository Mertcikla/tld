package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
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
	if err := impact.ValidateBlastRadius(input.GetContextDepth()); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	diagram, err := s.impactService().Compare(ctx, impact.CompareRequest{
		RepositoryID: input.RepositoryId,
		Base:         input.Base,
		Head:         input.Head,
		BlastRadius:  input.GetContextDepth(),
		ViewFilter:   input.GetViewFilter(),
		Progress: func(p indexer.Progress) {
			_ = stream.Send(&pb.CompareRepositoryEvent{Event: &pb.CompareRepositoryEvent_Progress{Progress: &pb.Progress{Stage: p.Stage, Current: uint32(p.Current), Total: uint32(p.Total), Detail: p.Detail}}})
		},
	})
	if err != nil {
		return impactError(err)
	}
	result := impact.PublicScene(diagram.Scene, input.GetDebugProvenance())
	return stream.Send(&pb.CompareRepositoryEvent{Event: &pb.CompareRepositoryEvent_Result{Result: result}})
}

func (s *codeIndexService) GetLiveImpact(ctx context.Context, req *connect.Request[pb.ID]) (*connect.Response[pb.LiveImpact], error) {
	if _, err := s.store.Repository(ctx, req.Msg.GetId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	result, err := s.store.LiveImpact(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	public := &pb.LiveImpact{Watching: result.Watching, Error: result.Error, GitBranch: result.GitBranch, GitRevision: result.GitRevision}
	if result.Diagram != nil {
		if result.Diagram.GetScene().GetSchemaVersion() == impact.PipelineVersion {
			public.Scene = impact.PublicScene(result.Diagram.Scene, false)
		} else {
			public.Error = "Comparison schema is obsolete; run compare again"
		}
	}
	return connect.NewResponse(public), nil
}

func (s *codeIndexService) ExportImpactMermaid(ctx context.Context, req *connect.Request[pb.ExportImpactMermaidRequest]) (*connect.Response[pb.ExportImpactMermaidResponse], error) {
	if req.Msg.GetRepositoryId() == "" || req.Msg.GetComparisonKey() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository id and comparison key are required"))
	}
	diagram, err := s.store.Impact(ctx, req.Msg.GetRepositoryId(), req.Msg.GetComparisonKey())
	if err != nil {
		return nil, impactLookupError(err)
	}
	// Mermaid projects its own bounded membership from the frozen canvas scene.
	scene, err := s.impactService().Scene(ctx, diagram)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	radius := impact.DiagramRadius(diagram)
	scene = impact.SelectPresentation(scene, mermaid.SceneMembership(impact.PublicScene(scene, false), radius, req.Msg.GetPlain()), radius, req.Msg.GetPlain())
	code := mermaid.ExportImpactScene(impact.PublicScene(scene, false), mermaid.SceneExportOptions{IncludeMetadata: true, Plain: req.Msg.GetPlain(), Theme: mermaid.DefaultTheme()})
	response := &pb.ExportImpactMermaidResponse{Code: code}
	if req.Msg.GetMarkdown() {
		response.Markdown = mermaid.MermaidBlock(code)
	}
	return connect.NewResponse(response), nil
}

func (s *codeIndexService) GetImpactScene(ctx context.Context, req *connect.Request[pb.GetImpactSceneRequest]) (*connect.Response[pb.ImpactScene], error) {
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
	// Serve the frozen selection. Standard retains the authored skeleton;
	// Plain applies the canonical canvas policy, independently of Mermaid.
	scene = impact.SelectScene(scene, impact.DiagramRadius(diagram), req.Msg.GetPlain())
	if err := impact.ValidateCoverage(scene); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(impact.PublicScene(scene, req.Msg.GetDebugProvenance())), nil
}
