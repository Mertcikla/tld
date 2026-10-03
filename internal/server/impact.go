package server

import (
	"context"
	"errors"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/ingest"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
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

func (s *mapperService) CompareRepository(ctx context.Context, req *connect.Request[pb.CompareRepositoryRequest], stream *connect.ServerStream[pb.CompareRepositoryEvent]) error {
	input := req.Msg
	if input.RepositoryId == "" || input.Base == nil || input.Head == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository and both comparison targets are required"))
	}
	repo, err := s.idx.Repository(ctx, input.RepositoryId)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}
	ctx, release, err := s.idx.AcquireLease(ctx, repo.Id)
	if err != nil {
		return impactError(err)
	}
	defer release()
	send := func(p indexer.Progress) {
		_ = stream.Send(&pb.CompareRepositoryEvent{Event: &pb.CompareRepositoryEvent_Progress{Progress: &pb.MapProgress{Stage: p.Stage, Current: uint32(p.Current), Total: uint32(p.Total), Detail: p.Detail}}})
	}
	engine := ingest.Engine{Store: s.idx, Config: configbridge.FromGlobal(s.config), Root: repo.Root, RepositoryID: repo.Id, Progress: send}
	before, err := engine.Prepare(ctx, input.Base)
	if err != nil {
		return impactError(err)
	}
	after, err := engine.Prepare(ctx, input.Head)
	if err != nil {
		return impactError(err)
	}
	send(indexer.Progress{Stage: "overlay", Detail: "preparing change overlay"})
	key := graph.ID(before.Id, after.Id)
	diagram, err := impact.Save(ctx, s.ws, s.idx, repo.Id, key, before.Id, after.Id, input.Radius)
	if err != nil {
		return impactError(err)
	}
	return stream.Send(&pb.CompareRepositoryEvent{Event: &pb.CompareRepositoryEvent_Result{Result: diagram}})
}

func (s *mapperService) GetLiveImpact(ctx context.Context, req *connect.Request[pb.GetLiveImpactRequest]) (*connect.Response[pb.LiveImpact], error) {
	if _, err := s.idx.Repository(ctx, req.Msg.RepositoryId); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	result, err := s.idx.LiveImpact(ctx, req.Msg.RepositoryId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(result), nil
}

func (s *mapperService) SetImpactRadius(ctx context.Context, req *connect.Request[pb.SetImpactRadiusRequest]) (*connect.Response[pb.ImpactDiagram], error) {
	ctx, release, err := s.idx.AcquireLease(ctx, req.Msg.RepositoryId)
	if err != nil {
		return nil, impactError(err)
	}
	defer release()
	recorded, err := s.idx.Impact(ctx, req.Msg.RepositoryId, req.Msg.ComparisonKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	result, err := impact.Save(ctx, s.ws, s.idx, recorded.RepositoryId, recorded.ComparisonKey, recorded.Diff.FromSnapshotId, recorded.Diff.ToSnapshotId, req.Msg.Radius)
	if err != nil {
		return nil, impactError(err)
	}
	return connect.NewResponse(result), nil
}
