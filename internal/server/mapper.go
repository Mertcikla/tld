package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/maprun"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// mapperService runs the repository mapping pipeline and materializes its
// dependency-graph community hierarchy into the workspace.
type mapperService struct {
	codeindexv1connect.UnimplementedMapperServiceHandler
	ws     *store.SQLiteStore
	idx    *cstore.Store
	config *workspace.Config

	mu      sync.Mutex
	running map[string]struct{}
}

func registerMapperHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore, configs ...*workspace.Config) {
	svc := &mapperService{
		ws:      sqliteStore,
		idx:     cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect()),
		running: map[string]struct{}{},
	}
	if len(configs) > 0 {
		svc.config = configs[0]
	}
	path, handler := codeindexv1connect.NewMapperServiceHandler(svc)
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
}

func (s *mapperService) MapRepository(ctx context.Context, req *connect.Request[codeindexv1.MapRepositoryRequest], stream *connect.ServerStream[codeindexv1.MapRepositoryEvent]) error {
	repositoryID := strings.TrimSpace(req.Msg.GetRepositoryId())
	if repositoryID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	req.Msg.RepositoryId = repositoryID
	req.Msg.SnapshotId = strings.TrimSpace(req.Msg.SnapshotId)
	req.Msg.GitRevision = strings.TrimSpace(req.Msg.GitRevision)
	targets := 0
	if req.Msg.SnapshotId != "" {
		targets++
	}
	if req.Msg.GitRevision != "" {
		targets++
	}
	if req.Msg.WorkingTree {
		targets++
	}
	if targets > 1 {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot_id, git_revision, and working_tree are mutually exclusive"))
	}
	if !s.begin(repositoryID) {
		return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a map is already running for this repository"))
	}
	defer s.end(repositoryID)
	ctx, release, err := s.idx.AcquireLease(ctx, repositoryID)
	if err != nil {
		return impactError(err)
	}
	defer release()

	send := func(progress *codeindexv1.MapProgress) {
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

func (s *mapperService) begin(repositoryID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[repositoryID]; ok {
		return false
	}
	s.running[repositoryID] = struct{}{}
	return true
}

func (s *mapperService) end(repositoryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, repositoryID)
}

func (s *mapperService) ListMaps(ctx context.Context, req *connect.Request[codeindexv1.ListMapsRequest]) (*connect.Response[codeindexv1.ListMapsResponse], error) {
	if _, err := s.idx.Repository(ctx, req.Msg.GetRepositoryId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	maps, err := s.idx.CompletedMaps(ctx, req.Msg.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&codeindexv1.ListMapsResponse{Maps: maps}), nil
}

// mapWithCommunities groups a snapshot's files by their resolved dependency
// graph, materializes the hierarchy and persists the run as communities.
func (s *mapperService) mapWithCommunities(ctx context.Context, req *connect.Request[codeindexv1.MapRepositoryRequest], repositoryID string, snapshot *codeindexv1.Snapshot, send func(*codeindexv1.MapProgress)) (*codeindexv1.MapResult, error) {
	result, _, err := maprun.Run(ctx, maprun.Deps{
		Workspace: s.ws,
		Codeindex: s.idx,
		Options:   mapconfig.FromGlobal(s.config),
	}, maprun.Request{
		RepositoryID: repositoryID,
		SnapshotID:   snapshot.Id,
	}, func(stage string, current, total int, detail string) {
		send(&codeindexv1.MapProgress{Stage: stage, Current: uint32(current), Total: uint32(total), Detail: detail})
	})
	if err != nil {
		if errors.Is(err, maprun.ErrNoFileFacts) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return result, nil
}
