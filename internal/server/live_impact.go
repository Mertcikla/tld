package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/types/known/emptypb"
)

// subscribeLive registers before the initial database read so a publication
// racing with connection establishment cannot be lost. A buffered invalidation
// coalesces bursts; each subscriber reads the latest committed scene itself.
func (s *codeIndexService) subscribeLive(ctx context.Context, repositoryID string) (<-chan struct{}, func()) {
	key := app.TenantOrgIDFromCtx(ctx).String() + ":" + repositoryID
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	if s.liveSubscribers == nil {
		s.liveSubscribers = make(map[string]map[chan struct{}]struct{})
	}
	if s.liveSubscribers[key] == nil {
		s.liveSubscribers[key] = make(map[chan struct{}]struct{})
	}
	s.liveSubscribers[key][ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.liveSubscribers[key], ch)
		if len(s.liveSubscribers[key]) == 0 {
			delete(s.liveSubscribers, key)
		}
	}
}

func (s *codeIndexService) PublishLiveImpact(ctx context.Context, req *connect.Request[pb.PublishLiveImpactRequest]) (*connect.Response[emptypb.Empty], error) {
	id := strings.TrimSpace(req.Msg.GetRepositoryId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	if _, err := s.store.Repository(ctx, id); err != nil {
		return nil, impactLookupError(err)
	}
	key := app.TenantOrgIDFromCtx(ctx).String() + ":" + id
	s.mu.Lock()
	for ch := range s.liveSubscribers[key] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *codeIndexService) liveImpactMessage(ctx context.Context, req *pb.WatchLiveImpactRequest) (*pb.LiveImpact, error) {
	result, err := s.store.LiveImpact(ctx, req.GetRepositoryId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := &pb.LiveImpact{Watching: result.Watching, Error: result.Error, GitBranch: result.GitBranch, GitRevision: result.GitRevision}
	if result.Diagram != nil {
		if result.Diagram.GetScene().GetSchemaVersion() != impact.PipelineVersion {
			out.Error = "Comparison schema is obsolete; run compare again"
		} else {
			scene := impact.SelectScene(result.Diagram.Scene, impact.DiagramRadius(result.Diagram), req.GetPlain())
			out.Scene = impact.PublicScene(scene, req.GetDebugProvenance())
		}
	}
	s.refreshLiveStatus(ctx, req.GetRepositoryId(), out)
	return out, nil
}

func (s *codeIndexService) refreshLiveStatus(ctx context.Context, id string, out *pb.LiveImpact) {
	if s.repositories == nil {
		return
	}
	key, err := s.repositories.watchKey(ctx, id)
	if err != nil {
		return
	}
	status := s.repositories.status(ctx, id, key)
	out.WatchStatus = status
	out.Watching, out.GitBranch, out.GitRevision = status.Running, status.GitBranch, status.GitRevision
	if out.Error != "Comparison schema is obsolete; run compare again" {
		out.Error = status.Error
	}
}

func (s *codeIndexService) WatchLiveImpact(ctx context.Context, req *connect.Request[pb.WatchLiveImpactRequest], stream *connect.ServerStream[pb.LiveImpact]) error {
	id := strings.TrimSpace(req.Msg.GetRepositoryId())
	if id == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	if _, err := s.store.Repository(ctx, id); err != nil {
		return impactLookupError(err)
	}
	req.Msg.RepositoryId = id
	updates, unsubscribe := s.subscribeLive(ctx, id)
	defer unsubscribe()
	// Heartbeats keep idle connections active and detect crashed watchers. They
	// only refresh watcher status, never poll for scene changes.
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	latest, err := s.liveImpactMessage(ctx, req.Msg)
	if err != nil {
		return err
	}
	lastVersion := ""
	for {
		// Status heartbeats do not retransmit a potentially large scene.
		message := &pb.LiveImpact{Watching: latest.Watching, Error: latest.Error, GitBranch: latest.GitBranch, GitRevision: latest.GitRevision, WatchStatus: latest.WatchStatus}
		version := latest.GetScene().GetComparison().GetVersion()
		if version != lastVersion {
			message.Scene = latest.Scene
		}
		if err := stream.Send(message); err != nil {
			return err
		}
		lastVersion = version
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-updates:
			latest, err = s.liveImpactMessage(ctx, req.Msg)
			if err != nil {
				return err
			}
		case <-heartbeat.C:
			s.refreshLiveStatus(ctx, id, latest)
		}
	}
}
