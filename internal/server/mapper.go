package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/maprun"
)

// MapRepository runs the repository mapping pipeline and materializes its
// dependency-graph community hierarchy into the workspace.
func (s *codeIndexService) MapRepository(ctx context.Context, req *connect.Request[codeindexv1.MapRepositoryRequest], stream *connect.ServerStream[codeindexv1.MapRepositoryEvent]) error {
	repositoryID := strings.TrimSpace(req.Msg.GetRepositoryId())
	if repositoryID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	req.Msg.RepositoryId = repositoryID
	revision := req.Msg.GetRevision()
	if revision == nil {
		revision = &codeindexv1.Revision{}
		req.Msg.Revision = revision
	}
	revision.SnapshotId = strings.TrimSpace(revision.GetSnapshotId())
	revision.GitRevision = strings.TrimSpace(revision.GetGitRevision())
	targets := 0
	if revision.GetSnapshotId() != "" {
		targets++
	}
	if revision.GetGitRevision() != "" {
		targets++
	}
	if revision.GetWorkingTree() {
		targets++
	}
	if targets > 1 {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot_id, git_revision, and working_tree are mutually exclusive"))
	}
	if !s.begin(repositoryID) {
		return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a map is already running for this repository"))
	}
	defer s.end(repositoryID)
	ctx, release, err := s.store.AcquireLease(ctx, repositoryID)
	if err != nil {
		return impactError(err)
	}
	defer release()

	send := func(progress *codeindexv1.Progress) {
		_ = stream.Send(&codeindexv1.MapRepositoryEvent{Event: &codeindexv1.MapRepositoryEvent_Progress{Progress: progress}})
	}

	snapshot, err := s.prepareSnapshot(ctx, req.Msg, send)
	if err != nil {
		return err
	}
	result, err := s.mapWithCommunities(ctx, req, repositoryID, snapshot, send)
	if err != nil {
		return err
	}
	return stream.Send(&codeindexv1.MapRepositoryEvent{Event: &codeindexv1.MapRepositoryEvent_Result{Result: result}})
}

func (s *codeIndexService) begin(repositoryID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[repositoryID]; ok {
		return false
	}
	s.running[repositoryID] = struct{}{}
	return true
}

func (s *codeIndexService) end(repositoryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, repositoryID)
}

func (s *codeIndexService) ListMaps(ctx context.Context, req *connect.Request[codeindexv1.ID]) (*connect.Response[codeindexv1.ListMapsResponse], error) {
	if _, err := s.store.Repository(ctx, req.Msg.GetId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	maps, err := s.store.CompletedMaps(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&codeindexv1.ListMapsResponse{Maps: maps}), nil
}

// mapWithCommunities groups a snapshot's files by their resolved dependency
// graph, materializes the hierarchy and persists the run as communities.
func (s *codeIndexService) mapWithCommunities(ctx context.Context, req *connect.Request[codeindexv1.MapRepositoryRequest], repositoryID string, snapshot *codeindexv1.Snapshot, send func(*codeindexv1.Progress)) (*codeindexv1.MapResult, error) {
	result, _, err := maprun.Run(ctx, maprun.Deps{
		Workspace: s.ws,
		Codeindex: s.store,
		Options:   mapconfig.FromGlobal(s.config),
	}, maprun.Request{
		RepositoryID: repositoryID,
		SnapshotID:   snapshot.Id,
	}, func(stage string, current, total int, detail string) {
		send(&codeindexv1.Progress{Stage: stage, Current: uint32(current), Total: uint32(total), Detail: detail})
	})
	if err != nil {
		if errors.Is(err, maprun.ErrNoFileFacts) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return result, nil
}
