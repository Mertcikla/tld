package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/ingest"
	"github.com/mertcikla/tld/v2/internal/codeindex/remote"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// codeIndexRepositoryService exposes the repositories indexed by the in-process
// codeindex engine to the UI.
type codeIndexRepositoryService struct {
	codeindexv1connect.UnimplementedRepositoryServiceHandler
	store   *cstore.Store
	ws      *store.SQLiteStore
	dataDir string
	config  *workspace.Config
}

func (s *codeIndexRepositoryService) ListRepositories(ctx context.Context, _ *connect.Request[codeindexv1.ListRepositoriesRequest]) (*connect.Response[codeindexv1.ListRepositoriesResponse], error) {
	repositories, err := s.store.ListRepositories(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, repository := range repositories {
		if repository.GetRemoteUrl() == "" {
			if remote := repolink.GitRemoteURL(ctx, repository.GetRoot()); remote != "" {
				repository.RemoteUrl = remote
				_ = s.store.SetRepositoryOrigin(ctx, repository.GetId(), remote, repository.GetManaged())
			}
		}
		repository.Name = repositoryDisplayName(repository)
	}
	return connect.NewResponse(&codeindexv1.ListRepositoriesResponse{Repositories: repositories}), nil
}

// repositoryDisplayName derives a short repository name from its remote path or
// local root.
func repositoryDisplayName(repository *codeindexv1.RepositorySummary) string {
	if remote := repository.GetRemoteUrl(); remote != "" {
		if parsed, err := url.Parse(remote); err == nil {
			if name := path.Base(strings.TrimSuffix(parsed.Path, "/")); name != "" && name != "." && name != "/" {
				return name
			}
		}
	}
	return filepath.Base(repository.GetRoot())
}

// AddRepository indexes a local directory or tld-managed clone in-process and
// registers it as a repository. Progress is streamed while the snapshot is
// built, then the registered repository is sent once indexing completes.
func (s *codeIndexRepositoryService) AddRepository(ctx context.Context, req *connect.Request[codeindexv1.AddRepositoryRequest], stream *connect.ServerStream[codeindexv1.AddRepositoryEvent]) error {
	path := strings.TrimSpace(req.Msg.GetPath())
	remoteURL := strings.TrimSpace(req.Msg.GetRemoteUrl())
	if path == "" && remoteURL == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path or remote_url is required"))
	}
	if path != "" && remoteURL != "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path and remote_url are mutually exclusive"))
	}

	var root string
	var spec remote.Spec
	if remoteURL != "" {
		parsed, err := remote.Parse(remoteURL)
		if err != nil {
			return connect.NewError(connect.CodeInvalidArgument, err)
		}
		spec = parsed
		if s.dataDir == "" {
			return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("remote repositories require a local data directory"))
		}
		root = remote.ManagedDir(s.dataDir, spec)
		_ = stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Progress{Progress: &codeindexv1.IndexProgress{
			Stage:  "clone",
			Detail: spec.WebURL,
		}}})
		if err := remote.Clone(ctx, spec, root); err != nil {
			return connect.NewError(connect.CodeInternal, err)
		}
	} else {
		resolved, err := resolveRepositoryRoot(path)
		if err != nil {
			return connect.NewError(connect.CodeInvalidArgument, err)
		}
		root = resolved
	}

	repositoryID := cgraph.RepositoryID(root)
	ctx, release, err := s.store.AcquireLease(ctx, repositoryID)
	if err != nil {
		return impactError(err)
	}
	defer release()

	engine := ingest.Engine{
		Store:        s.store,
		Config:       configbridge.FromGlobal(s.config),
		Root:         root,
		RepositoryID: repositoryID,
		Progress: func(p indexer.Progress) {
			_ = stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Progress{Progress: &codeindexv1.IndexProgress{
				Stage:   p.Stage,
				Current: uint32(p.Current),
				Total:   uint32(p.Total),
				Detail:  p.Detail,
			}}})
		},
	}
	if _, err := engine.Prepare(ctx, &codeindexv1.ComparisonTarget{WorkingTree: true}); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	if remoteURL != "" {
		if err := s.store.SetRepositoryOrigin(ctx, repositoryID, spec.WebURL, true); err != nil {
			return connect.NewError(connect.CodeInternal, err)
		}
	}
	repository, err := s.store.Repository(ctx, repositoryID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	return stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Repository{Repository: repository}})
}

// resolveRepositoryRoot canonicalizes a user-supplied path the same way the CLI
// does: expand ~, make it absolute, require an existing directory, and resolve
// symlinks so the repository id is stable.
func resolveRepositoryRoot(path string) (string, error) {
	expanded := path
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		expanded = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	root, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func (s *codeIndexRepositoryService) DeleteRepository(ctx context.Context, req *connect.Request[codeindexv1.DeleteRepositoryRequest]) (*connect.Response[codeindexv1.DeleteRepositoryResponse], error) {
	repositoryID := strings.TrimSpace(req.Msg.GetId())
	if repositoryID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository id is required"))
	}
	repository, err := s.store.Repository(ctx, repositoryID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if req.Msg.GetDeleteMaterialized() {
		if err := s.deleteMaterializedResources(ctx, repositoryID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	if req.Msg.GetDeleteClone() {
		_, managed, err := s.store.RepositoryOrigin(ctx, repositoryID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if managed {
			if s.dataDir == "" || !remote.IsManagedPath(s.dataDir, repository.Root) {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("repository checkout is not tld-managed"))
			}
			if err := os.RemoveAll(repository.Root); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete clone: %w", err))
			}
		}
	}
	if err := s.store.DeleteRepository(ctx, repositoryID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&codeindexv1.DeleteRepositoryResponse{}), nil
}

// deleteMaterializedResources removes the workspace views, elements, and
// connectors created by the mapper for a repository. Mappings are left in place
// so DeleteRepository can clear them with the rest of the repository's data.
func (s *codeIndexRepositoryService) deleteMaterializedResources(ctx context.Context, repositoryID string) error {
	mappings, err := s.store.MappingsByRepository(ctx, repositoryID)
	if err != nil {
		return err
	}
	for _, mapping := range mappings {
		switch mapping.Kind {
		case cstore.MappingView:
			if err := s.ws.DeleteView(ctx, mapping.ResourceID); err != nil {
				return err
			}
		case cstore.MappingElement:
			if err := s.ws.DeleteElement(ctx, mapping.ResourceID); err != nil {
				return err
			}
		case cstore.MappingConnector:
			if err := s.ws.DeleteConnector(ctx, mapping.ResourceID); err != nil {
				return err
			}
		}
	}
	return nil
}

// codeIndexFactService serves an indexed repository's snapshots and immutable
// code graph. Indexing runs in-process; this surface is read-only.
type codeIndexFactService struct {
	codeindexv1connect.UnimplementedCodeFactServiceHandler
	store *cstore.Store
}

func (s *codeIndexFactService) GetFact(ctx context.Context, req *connect.Request[codeindexv1.CodeFactID]) (*connect.Response[codeindexv1.CodeFact], error) {
	fact, err := s.store.Fact(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(fact), nil
}

func (s *codeIndexFactService) ListFacts(ctx context.Context, req *connect.Request[codeindexv1.CodeFactFilter]) (*connect.Response[codeindexv1.CodeFactPage], error) {
	filter := req.Msg
	if filter.GetLogicalKey() != "" {
		facts, err := s.store.FactVersions(ctx, filter.GetLogicalKey())
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return connect.NewResponse(&codeindexv1.CodeFactPage{Facts: facts}), nil
	}
	if filter.GetSnapshotId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot_id is required"))
	}
	limit := int(filter.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	facts, err := s.store.Facts(ctx, filter.GetSnapshotId(), filter.GetKind(), filter.GetPathPrefix(), filter.GetPageToken(), limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	page := &codeindexv1.CodeFactPage{Facts: facts}
	if len(facts) == limit {
		page.NextPageToken = facts[len(facts)-1].GetId()
	}
	return connect.NewResponse(page), nil
}

func (s *codeIndexFactService) GetEdgeFact(ctx context.Context, req *connect.Request[codeindexv1.EdgeFactID]) (*connect.Response[codeindexv1.EdgeFact], error) {
	edge, err := s.store.EdgeFact(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(edge), nil
}

func (s *codeIndexFactService) ListEdgeFacts(ctx context.Context, req *connect.Request[codeindexv1.EdgeFactFilter]) (*connect.Response[codeindexv1.EdgeFactPage], error) {
	filter := req.Msg
	if filter.GetLogicalKey() != "" {
		edges, err := s.store.EdgeVersions(ctx, filter.GetLogicalKey())
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return connect.NewResponse(&codeindexv1.EdgeFactPage{EdgeFacts: edges}), nil
	}
	if filter.GetSnapshotId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot_id is required"))
	}
	limit := int(filter.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	edges, err := s.store.EdgeFacts(ctx, filter.GetSnapshotId(), filter.GetKind(), "", filter.GetPageToken(), limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	page := &codeindexv1.EdgeFactPage{EdgeFacts: edges}
	if len(edges) == limit {
		page.NextPageToken = edges[len(edges)-1].GetId()
	}
	return connect.NewResponse(page), nil
}

func (s *codeIndexFactService) GetSource(ctx context.Context, req *connect.Request[codeindexv1.SourceRequest]) (*connect.Response[codeindexv1.SourceResponse], error) {
	anchor := req.Msg.GetAnchor()
	if anchor == nil || anchor.GetSourceHash() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("anchor.source_hash is required"))
	}
	content, err := s.store.Source(ctx, anchor.GetSourceHash())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&codeindexv1.SourceResponse{Content: content, Anchor: anchor}), nil
}

func (s *codeIndexFactService) ListSnapshots(ctx context.Context, req *connect.Request[codeindexv1.RepositoryID]) (*connect.Response[codeindexv1.ListSnapshotsResponse], error) {
	snapshots, err := s.store.Snapshots(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Working-tree snapshots capture the transient checkout state and are not
	// saved points users can return to, so they are excluded from the saved
	// snapshot list. They remain addressable by id through the working_tree
	// comparison target and the live change overlay.
	saved := make([]*codeindexv1.Snapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		if snap.GetProvenance() == "working_tree" {
			continue
		}
		saved = append(saved, snap)
	}
	return connect.NewResponse(&codeindexv1.ListSnapshotsResponse{Snapshots: saved}), nil
}

func (s *codeIndexFactService) GetRepository(ctx context.Context, req *connect.Request[codeindexv1.RepositoryID]) (*connect.Response[codeindexv1.Repository], error) {
	repo, err := s.store.Repository(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(repo), nil
}

func (s *codeIndexFactService) DiffSnapshots(ctx context.Context, req *connect.Request[codeindexv1.SnapshotDiffRequest]) (*connect.Response[codeindexv1.SnapshotDiff], error) {
	msg := req.Msg
	if msg.GetFromSnapshotId() == "" || msg.GetToSnapshotId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("from_snapshot_id and to_snapshot_id are required"))
	}
	diff, err := s.store.Diff(ctx, msg.GetFromSnapshotId(), msg.GetToSnapshotId(), msg.GetSourcesOnly())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(diff), nil
}

func (s *codeIndexFactService) DeleteSnapshot(ctx context.Context, req *connect.Request[codeindexv1.DeleteSnapshotRequest]) (*connect.Response[codeindexv1.DeleteSnapshotResponse], error) {
	snapshotID := strings.TrimSpace(req.Msg.GetSnapshotId())
	if snapshotID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot_id is required"))
	}
	if err := s.store.DeleteSnapshot(ctx, snapshotID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("snapshot %s not found", snapshotID))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&codeindexv1.DeleteSnapshotResponse{}), nil
}

func (s *codeIndexFactService) AggregateEdges(ctx context.Context, req *connect.Request[codeindexv1.EdgeFactFilter]) (*connect.Response[codeindexv1.EdgeAggregatePage], error) {
	filter := req.Msg
	if filter.GetSnapshotId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot_id is required"))
	}
	limit := int(filter.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	edges, err := s.store.EdgeFacts(ctx, filter.GetSnapshotId(), filter.GetKind(), filter.GetLogicalKey(), filter.GetPageToken(), limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	byKey := make(map[string]*codeindexv1.EdgeAggregate, len(edges))
	order := make([]string, 0, len(edges))
	for _, edge := range edges {
		key := edge.GetLogicalKey()
		if key == "" {
			key = edge.GetId()
		}
		agg := byKey[key]
		if agg == nil {
			agg = &codeindexv1.EdgeAggregate{
				LogicalKey:      key,
				Kind:            edge.GetKind(),
				FromFactId:      edge.GetFromFactId(),
				ToFactId:        edge.GetToFactId(),
				TargetSymbolKey: edge.GetTargetSymbolKey(),
			}
			byKey[key] = agg
			order = append(order, key)
		}
		agg.Weight += edge.GetWeight()
		agg.ObservationCount++
		if len(agg.Observations) < maxEdgeObservations {
			agg.Observations = append(agg.Observations, edge)
		} else {
			agg.ObservationsTruncated = true
		}
	}
	aggregates := make([]*codeindexv1.EdgeAggregate, 0, len(order))
	for _, key := range order {
		aggregates = append(aggregates, byKey[key])
	}
	return connect.NewResponse(&codeindexv1.EdgeAggregatePage{Edges: aggregates, NextPageToken: nextEdgeCursor(edges, limit)}), nil
}

const (
	defaultPageSize     = 200
	maxEdgeObservations = 20
)

// nextEdgeCursor returns the last edge id when a page is full so callers can
// continue listing; aggregate grouping across a page boundary is approximate.
func nextEdgeCursor(edges []*codeindexv1.EdgeFact, limit int) string {
	if len(edges) == limit && len(edges) > 0 {
		return edges[len(edges)-1].GetId()
	}
	return ""
}

func registerCodeIndexHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore, dataDir string, configs ...*workspace.Config) {
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	repoSvc := &codeIndexRepositoryService{store: idx, ws: sqliteStore, dataDir: dataDir}
	if len(configs) > 0 {
		repoSvc.config = configs[0]
	}
	repoPath, repoHandler := codeindexv1connect.NewRepositoryServiceHandler(repoSvc)
	mux.Handle("/api"+repoPath, http.StripPrefix("/api", repoHandler))

	factSvc := &codeIndexFactService{store: idx}
	factPath, factHandler := codeindexv1connect.NewCodeFactServiceHandler(factSvc)
	mux.Handle("/api"+factPath, http.StripPrefix("/api", factHandler))
}
