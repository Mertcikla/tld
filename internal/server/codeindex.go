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
	"sync"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/gitstate"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/identity"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/ingest"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/maprun"
	"github.com/mertcikla/tld/v2/internal/codeindex/remote"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/codeindex/tools"
	"github.com/mertcikla/tld/v2/internal/repolink"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/app"
	"google.golang.org/protobuf/types/known/emptypb"
)

// repositoryService exposes the repositories indexed by the in-process
// codeindex engine to the UI: registration, settings, Git history, pull
// requests, and the per-repository watcher.
type repositoryService struct {
	codeindexv1connect.UnimplementedRepositoryServiceHandler
	store      *cstore.Store
	ws         *store.SQLiteStore
	dataDir    string
	config     *workspace.Config
	watches    *watchManager
	selfHosted bool
}

func (s *repositoryService) ListRepositories(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[codeindexv1.ListRepositoriesResponse], error) {
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
func repositoryDisplayName(repository *codeindexv1.Repository) string {
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
// built, then the registered repository is sent once indexing completes. When
// materialize is set, the published snapshot is also projected into the
// workspace before the repository is sent.
func (s *repositoryService) AddRepository(ctx context.Context, req *connect.Request[codeindexv1.AddRepositoryRequest], stream *connect.ServerStream[codeindexv1.AddRepositoryEvent]) error {
	root, spec, err := s.resolveAddTarget(ctx, req.Msg.GetPath(), req.Msg.GetRemoteUrl(), func(spec remote.Spec) {
		_ = stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Progress{Progress: &codeindexv1.Progress{
			Stage:  "clone",
			Detail: spec.WebURL,
		}}})
	})
	if err != nil {
		return err
	}

	requirements, err := s.requiredIndexers(ctx, root)
	if err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	if missing := missingTools(requirements); len(missing) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("missing required indexers: %s; install them and try again", strings.Join(missing, ", ")))
	}

	resolved, err := identity.Apply(ctx, s.store, root, "", spec.WebURL, spec.WebURL != "")
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	repositoryID := resolved.ID
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
			_ = stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Progress{Progress: &codeindexv1.Progress{
				Stage:   p.Stage,
				Current: uint32(p.Current),
				Total:   uint32(p.Total),
				Detail:  p.Detail,
			}}})
		},
	}
	snapshot, err := engine.Prepare(ctx, &codeindexv1.Revision{WorkingTree: true})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	// The initial index is a saved point users can return to, so a dirty or
	// non-Git checkout that yields transient working_tree provenance is
	// promoted to a manual snapshot. Clean checkouts already carry commit
	// provenance and keep it.
	if snapshot.GetProvenance() == "working_tree" {
		if err := s.store.SetSnapshotProvenance(ctx, snapshot.GetId(), "manual"); err != nil {
			return connect.NewError(connect.CodeInternal, err)
		}
		snapshot.Provenance = "manual"
	}
	if req.Msg.GetMaterialize() {
		_ = stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Progress{Progress: &codeindexv1.Progress{Stage: "map"}}})
		if err := s.mapRepository(ctx, repositoryID, snapshot, func(stage string, current, total int, detail string) {
			_ = stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Progress{Progress: &codeindexv1.Progress{
				Stage:   stage,
				Current: uint32(current),
				Total:   uint32(total),
				Detail:  detail,
			}}})
		}); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("repository indexed, but mapping failed: %w", err))
		}
	}
	repository, err := s.store.Repository(ctx, repositoryID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	return stream.Send(&codeindexv1.AddRepositoryEvent{Event: &codeindexv1.AddRepositoryEvent_Repository{Repository: repository}})
}

// resolveAddTarget validates the mutually exclusive path/remote_url inputs and
// resolves them to a local root, cloning remote references into tld-managed
// storage. onClone, when set, runs just before the clone begins.
func (s *repositoryService) resolveAddTarget(ctx context.Context, path, remoteURL string, onClone func(remote.Spec)) (string, remote.Spec, error) {
	path = strings.TrimSpace(path)
	remoteURL = strings.TrimSpace(remoteURL)
	if path == "" && remoteURL == "" {
		return "", remote.Spec{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path or remote_url is required"))
	}
	if path != "" && remoteURL != "" {
		return "", remote.Spec{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path and remote_url are mutually exclusive"))
	}
	if remoteURL == "" {
		resolved, err := resolveRepositoryRoot(path)
		if err != nil {
			return "", remote.Spec{}, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return resolved, remote.Spec{}, nil
	}
	spec, err := remote.Parse(remoteURL)
	if err != nil {
		return "", remote.Spec{}, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if s.dataDir == "" {
		return "", remote.Spec{}, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("remote repositories require a local data directory"))
	}
	root := remote.ManagedDir(s.dataDir, spec, app.TenantOrgIDFromCtx(ctx))
	if onClone != nil {
		onClone(spec)
	}
	if err := remote.Clone(ctx, spec, root); err != nil {
		return "", remote.Spec{}, connect.NewError(connect.CodeInternal, err)
	}
	return root, spec, nil
}

// CheckRepositoryIndexers inspects a repository's project markers and reports
// the SCIP indexers indexing will require. Remote targets are cloned when
// necessary so their project markers can be read.
func (s *repositoryService) CheckRepositoryIndexers(ctx context.Context, req *connect.Request[codeindexv1.CheckRepositoryIndexersRequest]) (*connect.Response[codeindexv1.CheckRepositoryIndexersResponse], error) {
	root, _, err := s.resolveAddTarget(ctx, req.Msg.GetPath(), req.Msg.GetRemoteUrl(), nil)
	if err != nil {
		return nil, err
	}
	requirements, err := s.requiredIndexers(ctx, root)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewResponse(&codeindexv1.CheckRepositoryIndexersResponse{
		Indexers: requirements,
		Ready:    len(missingTools(requirements)) == 0,
	}), nil
}

// requiredIndexers maps a repository's discovered projects to the external
// indexers they need and probes each tool with the current configuration.
func (s *repositoryService) requiredIndexers(ctx context.Context, root string) ([]*codeindexv1.IndexerRequirement, error) {
	projects, err := indexer.DiscoverProjects(ctx, root, nil, nil)
	if err != nil {
		return nil, err
	}
	families := make([]string, 0, len(projects))
	languages := make(map[string][]string, len(projects))
	seen := make(map[string]bool, len(projects))
	for _, p := range projects {
		family := indexer.Family(p.GetLanguage())
		if !seen[family] {
			seen[family] = true
			families = append(families, family)
		}
		languages[family] = append(languages[family], p.GetLanguage())
	}
	statuses := tools.CheckTools(ctx, configbridge.FromGlobal(s.config), tools.ForFamilies(families))
	requirements := make([]*codeindexv1.IndexerRequirement, 0, len(statuses))
	for _, status := range statuses {
		requirements = append(requirements, &codeindexv1.IndexerRequirement{
			Family:       status.Family,
			Tool:         status.Name,
			Languages:    languages[status.Family],
			Installed:    status.Found,
			InstallHint:  status.InstallHint,
			Version:      status.Version,
			MinVersion:   status.Minimum,
			BelowMinimum: status.BelowMinimum,
			DownloadUrl:  status.DownloadURL,
			Path:         status.Path,
		})
	}
	return requirements, nil
}

// missingTools lists the tool names among requirements that are not installed.
func missingTools(requirements []*codeindexv1.IndexerRequirement) []string {
	var missing []string
	for _, requirement := range requirements {
		if !requirement.GetInstalled() {
			missing = append(missing, requirement.GetTool())
		}
	}
	return missing
}

// mapRepository runs the graph mapping pipeline for a published snapshot,
// materializing its dependency-graph community hierarchy into the workspace.
// A snapshot with no file facts has nothing to group and is left unmapped.
func (s *repositoryService) mapRepository(ctx context.Context, repositoryID string, snapshot *codeindexv1.Snapshot, onProgress func(stage string, current, total int, detail string)) error {
	_, _, err := maprun.Run(ctx, maprun.Deps{
		Workspace: s.ws,
		Codeindex: s.store,
		Options:   mapconfig.FromGlobal(s.config),
	}, maprun.Request{
		RepositoryID: repositoryID,
		SnapshotID:   snapshot.GetId(),
	}, func(stage string, current, total int, detail string) {
		if onProgress != nil {
			onProgress(stage, current, total, detail)
		}
	})
	if errors.Is(err, maprun.ErrNoFileFacts) {
		return nil
	}
	return err
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

func (s *repositoryService) DeleteRepository(ctx context.Context, req *connect.Request[codeindexv1.DeleteRepositoryRequest]) (*connect.Response[codeindexv1.DeleteRepositoryResponse], error) {
	repositoryID := strings.TrimSpace(req.Msg.GetId())
	if repositoryID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository id is required"))
	}
	repository, err := s.store.Repository(ctx, repositoryID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err := s.stopRepositoryWatches(ctx, repository); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	ctx, release, err := s.store.AcquireLease(ctx, repositoryID)
	if err != nil {
		return nil, impactError(err)
	}
	defer release()
	// Another deletion may have completed while we were stopping the watcher.
	repository, err = s.store.Repository(ctx, repositoryID)
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
			if s.dataDir == "" || !remote.IsManagedPath(s.dataDir, app.TenantOrgIDFromCtx(ctx), repository.Root) {
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

func (s *repositoryService) stopRepositoryWatches(ctx context.Context, repository *codeindexv1.Repository) error {
	states, err := s.store.ListWatchStates(ctx)
	if err != nil {
		return err
	}
	keys := map[string]bool{cgraph.RepositoryID(repository.Root): true, repository.Id: true}
	for _, state := range states {
		if keys[state.RepositoryID] {
			continue
		}
		if state.SnapshotID != "" {
			if snapshot, err := s.store.Snapshot(ctx, state.SnapshotID); err == nil && snapshot.RepositoryId == repository.Id {
				keys[state.RepositoryID] = true
				continue
			}
		}
		// A second checkout may not have published its first snapshot yet.
		if state.RepoRoot != "" {
			resolved, err := identity.Resolve(ctx, s.store, state.RepoRoot, "", "")
			if err != nil {
				return err
			}
			if resolved.ID == repository.Id {
				keys[state.RepositoryID] = true
			}
		}
	}
	manager := s.watches
	if manager == nil {
		manager = newWatchManager(s.dataDir, s.store)
	}
	for key := range keys {
		if err := s.store.RequestWatchStop(ctx, key); err != nil {
			return err
		}
		manager.stop(ctx, key)
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

// deleteMaterializedResources removes the workspace views, elements, and
// connectors created by the mapper for a repository. Mappings are left in place
// so DeleteRepository can clear them with the rest of the repository's data.
func (s *repositoryService) deleteMaterializedResources(ctx context.Context, repositoryID string) error {
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

// codeIndexService serves an indexed repository's snapshots and immutable code
// graph, and runs the deterministic mapper and impact pipelines. Indexing runs
// in-process; the graph surface is read-only.
type codeIndexService struct {
	codeindexv1connect.UnimplementedCodeIndexServiceHandler
	store  *cstore.Store
	ws     *store.SQLiteStore
	config *workspace.Config

	mu      sync.Mutex
	running map[string]string
}

func (s *codeIndexService) ListFacts(ctx context.Context, req *connect.Request[codeindexv1.CodeFactFilter]) (*connect.Response[codeindexv1.CodeFactPage], error) {
	filter := req.Msg
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

func (s *codeIndexService) ListSnapshots(ctx context.Context, req *connect.Request[codeindexv1.ID]) (*connect.Response[codeindexv1.ListSnapshotsResponse], error) {
	snapshots, err := s.store.Snapshots(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Working-tree snapshots capture the transient checkout state and are not
	// saved points users can return to, so they are excluded from the saved
	// snapshot list. They remain addressable by id through the working_tree
	// revision and the live change overlay.
	saved := make([]*codeindexv1.Snapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		if snap.GetProvenance() == "working_tree" {
			continue
		}
		saved = append(saved, snap)
	}
	return connect.NewResponse(&codeindexv1.ListSnapshotsResponse{Snapshots: saved}), nil
}

// CaptureSnapshot indexes a repository and publishes a new snapshot. Working
// tree captures include uncommitted changes and are recorded as durable manual
// saved points; commit captures index the checked-out commit.
func (s *codeIndexService) CaptureSnapshot(ctx context.Context, req *connect.Request[codeindexv1.CaptureSnapshotRequest], stream *connect.ServerStream[codeindexv1.CaptureSnapshotEvent]) error {
	repositoryID := strings.TrimSpace(req.Msg.GetRepositoryId())
	if repositoryID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	repository, err := s.store.Repository(ctx, repositoryID)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if !s.begin(repositoryID, "snapshot capture") {
		return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a map or snapshot capture is already running for this repository"))
	}
	defer s.end(repositoryID)
	ctx, release, err := s.store.AcquireLease(ctx, repositoryID)
	if err != nil {
		return impactError(err)
	}
	defer release()

	target := &codeindexv1.Revision{}
	provenance := ""
	if req.Msg.GetWorkingTree() {
		target.WorkingTree = true
		// Explicit captures are saved points, so they are published with a
		// durable provenance instead of the transient working_tree marker.
		provenance = "manual"
	} else {
		branch, revision, err := gitstate.CurrentCommit(ctx, repository.GetRoot())
		if err != nil {
			return connect.NewError(connect.CodeFailedPrecondition, err)
		}
		target.GitRevision = revision
		target.GitBranch = branch
	}
	engine := ingest.Engine{
		Store:        s.store,
		Config:       configbridge.FromGlobal(s.config),
		Root:         repository.GetRoot(),
		RepositoryID: repository.GetId(),
		Provenance:   provenance,
		Progress: func(p indexer.Progress) {
			_ = stream.Send(&codeindexv1.CaptureSnapshotEvent{Event: &codeindexv1.CaptureSnapshotEvent_Progress{Progress: &codeindexv1.Progress{
				Stage:   p.Stage,
				Current: uint32(p.Current),
				Total:   uint32(p.Total),
				Detail:  p.Detail,
			}}})
		},
	}
	snapshot, err := engine.Prepare(ctx, target)
	if err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return stream.Send(&codeindexv1.CaptureSnapshotEvent{Event: &codeindexv1.CaptureSnapshotEvent_Snapshot{Snapshot: snapshot}})
}

func (s *codeIndexService) DiffSnapshots(ctx context.Context, req *connect.Request[codeindexv1.SnapshotDiffRequest]) (*connect.Response[codeindexv1.SnapshotDiff], error) {
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

func (s *codeIndexService) DeleteSnapshot(ctx context.Context, req *connect.Request[codeindexv1.ID]) (*connect.Response[codeindexv1.DeleteSnapshotResponse], error) {
	snapshotID := strings.TrimSpace(req.Msg.GetId())
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

const defaultPageSize = 200

// registerCodeIndexHandlers wires the RepositoryService and CodeIndexService
// and returns the watcher manager so the server can stop watchers on shutdown.
func registerCodeIndexHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore, dataDir string, selfHosted bool, configs ...*workspace.Config) *watchManager {
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	_ = idx.BackfillRemoteKeys(context.Background())
	manager := newWatchManager(dataDir, idx)

	repoSvc := &repositoryService{store: idx, ws: sqliteStore, dataDir: dataDir, watches: manager, selfHosted: selfHosted}
	factSvc := &codeIndexService{store: idx, ws: sqliteStore, running: map[string]string{}}
	if len(configs) > 0 {
		repoSvc.config = configs[0]
		factSvc.config = configs[0]
	}
	repoPath, repoHandler := codeindexv1connect.NewRepositoryServiceHandler(repoSvc)
	mux.Handle("/api"+repoPath, http.StripPrefix("/api", repoHandler))

	codeindexPath, codeindexHandler := codeindexv1connect.NewCodeIndexServiceHandler(factSvc)
	mux.Handle("/api"+codeindexPath, http.StripPrefix("/api", codeindexHandler))

	return manager
}
