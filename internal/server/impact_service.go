package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

// impactService is a local-only service. It manages repositories as persisted
// records linked to a diagram element, and reconciles git changes against the
// workspace architecture stored in the local database. It never mutates the
// architecture beyond creating/removing the repository element the user asks
// for.
type impactService struct {
	store        *store.APIAdapter
	watchStore   *watch.Store
	workspaceDir string
	dataDir      string
}

// workspaceCheckout reports the local checkout of the workspace itself when it
// belongs to the given repository, so a workspace-as-code project can be
// analyzed without manually linking a path.
func (s *impactService) workspaceCheckout(remote string) (string, string, string, bool) {
	if strings.TrimSpace(s.workspaceDir) == "" || strings.TrimSpace(remote) == "" {
		return "", "", "", false
	}
	root, err := tldgit.RepoRoot(s.workspaceDir)
	if err != nil {
		return "", "", "", false
	}
	remoteURL, _ := tldgit.DetectRemoteURL(root)
	if !sameRepo(remoteURL, remote) {
		return "", "", "", false
	}
	branch, _ := tldgit.DetectBranch(root)
	head, _ := tldgit.DetectHeadCommit(root)
	return root, branch, head, true
}

func (s *impactService) ListRepositories(ctx context.Context, _ *connect.Request[diagv1.ListRepositoriesRequest]) (*connect.Response[diagv1.ListRepositoriesResponse], error) {
	repositories := make([]*diagv1.Repository, 0)
	linked := map[int32]struct{}{}

	if s.watchStore != nil {
		records, err := s.watchStore.Repositories(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		for _, record := range records {
			summary := &diagv1.Repository{
				Name:            record.DisplayName,
				RemoteUrl:       record.RemoteURL.String,
				Branch:          record.Branch.String,
				LocalPath:       record.RepoRoot,
				HeadCommit:      record.HeadCommit.String,
				HasArchitecture: s.hasArchitecture(ctx, record.RemoteURL.String),
			}
			if record.RootElementID.Valid {
				id := int32(record.RootElementID.Int64)
				summary.ElementId = id
				summary.Ref = elementRef(id)
				linked[id] = struct{}{}
				if element, err := s.store.GetElement(ctx, id, api.WorkspaceIDFromCtx(ctx)); err == nil && element.GetName() != "" {
					summary.Name = element.GetName()
				}
			} else {
				// A checkout registered by the watcher may have no architecture
				// element yet. Expose a watch-scoped ref so the UI can still
				// inspect or unlink it instead of sending an empty reference.
				summary.Ref = watchRepositoryRef(record.ID)
			}
			repositories = append(repositories, summary)
		}
	}

	elements, _, err := s.store.ListElements(ctx, api.WorkspaceIDFromCtx(ctx), 0, 0, "")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, element := range elements {
		if !strings.EqualFold(element.GetKind(), "repository") {
			continue
		}
		if _, ok := linked[element.GetId()]; ok {
			continue
		}
		summary := &diagv1.Repository{
			Ref:             elementRef(element.GetId()),
			ElementId:       element.GetId(),
			Name:            element.GetName(),
			RemoteUrl:       element.GetRepo(),
			Branch:          element.GetBranch(),
			HasArchitecture: s.hasArchitecture(ctx, element.GetRepo()),
		}
		if path, branch, head, ok := s.workspaceCheckout(element.GetRepo()); ok {
			summary.LocalPath = path
			summary.HeadCommit = head
			if summary.Branch == "" {
				summary.Branch = branch
			}
		}
		repositories = append(repositories, summary)
	}
	return connect.NewResponse(&diagv1.ListRepositoriesResponse{Repositories: repositories}), nil
}

func (s *impactService) AddRepository(ctx context.Context, req *connect.Request[diagv1.AddRepositoryRequest]) (*connect.Response[diagv1.AddRepositoryResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	if path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}
	repoRoot, err := tldgit.RepoRoot(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%q is not inside a git repository: %w", path, err))
	}
	remote, _ := tldgit.DetectRemoteURL(repoRoot)
	branch, _ := tldgit.DetectBranch(repoRoot)
	head, _ := tldgit.DetectHeadCommit(repoRoot)
	if override := strings.TrimSpace(req.Msg.GetBranch()); override != "" {
		branch = override
	}
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		name = filepath.Base(repoRoot)
	}

	element, err := s.store.CreateElement(ctx, api.WorkspaceIDFromCtx(ctx), api.ElementInput{
		Name:   name,
		Kind:   strPtr("repository"),
		Repo:   strPtr(remote),
		Branch: strPtr(branch),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create repository element: %w", err))
	}
	if rootViewID, err := s.store.EnsureRootViewID(ctx); err == nil {
		_, _ = s.store.AddPlacement(ctx, rootViewID, element.GetId(), 0, 0)
	}

	summary := &diagv1.Repository{
		Ref:             elementRef(element.GetId()),
		ElementId:       element.GetId(),
		Name:            name,
		RemoteUrl:       remote,
		Branch:          branch,
		LocalPath:       repoRoot,
		HeadCommit:      head,
		HasArchitecture: s.hasArchitecture(ctx, remote),
	}
	if s.watchStore != nil {
		elementID := int64(element.GetId())
		record, err := s.watchStore.EnsureRepository(ctx, watch.RepositoryInput{
			RemoteURL:      remote,
			RepoRoot:       repoRoot,
			DisplayName:    name,
			Branch:         branch,
			HeadCommit:     head,
			IdentityStatus: "linked",
			RootElementID:  &elementID,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("register repository: %w", err))
		}
		summary.Name = record.DisplayName
	}
	return connect.NewResponse(&diagv1.AddRepositoryResponse{Repository: summary}), nil
}

func (s *impactService) UpdateRepository(ctx context.Context, req *connect.Request[diagv1.UpdateRepositoryRequest]) (*connect.Response[diagv1.UpdateRepositoryResponse], error) {
	ref := strings.TrimSpace(req.Msg.GetRef())
	branch := strings.TrimSpace(req.Msg.GetBranch())
	path := strings.TrimSpace(req.Msg.GetPath())
	if branch == "" && path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("branch or path is required"))
	}
	if watchID, ok := parseWatchRepositoryRef(ref); ok {
		if path != "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("checkout %s is not linked to an architecture element", ref))
		}
		if err := s.updateWatchRepositoryBranch(ctx, watchID, branch); err != nil {
			return nil, err
		}
		summary, err := s.watchRepositorySummary(ctx, watchID)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return connect.NewResponse(&diagv1.UpdateRepositoryResponse{Repository: summary}), nil
	}
	id, err := parseElementRef(ref)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if branch != "" {
		if err := s.store.UpdateElementBranch(ctx, id, branch); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update element branch: %w", err))
		}
		if record, ok := s.watchRepositoryForElement(ctx, id); ok && s.watchStore != nil {
			_, _ = s.watchStore.UpdateRepositoryBranch(ctx, record.ID, branch)
		}
	}
	if path != "" {
		if err := s.linkCheckout(ctx, id, path, branch); err != nil {
			return nil, err
		}
	}
	summary, err := s.repositorySummary(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&diagv1.UpdateRepositoryResponse{Repository: summary}), nil
}

func (s *impactService) updateWatchRepositoryBranch(ctx context.Context, id int64, branch string) error {
	if s.watchStore == nil {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("local repository registry unavailable"))
	}
	if _, err := s.watchStore.Repository(ctx, id); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("repository %s not found", watchRepositoryRef(id)))
	}
	if _, err := s.watchStore.UpdateRepositoryBranch(ctx, id, branch); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("update repository branch: %w", err))
	}
	return nil
}

// linkCheckout associates a local git checkout with an existing repository
// element, so an element created through git source linking can be analyzed
// without recreating it.
func (s *impactService) linkCheckout(ctx context.Context, id int32, path, branch string) error {
	if s.watchStore == nil {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("local repository registry unavailable"))
	}
	repoRoot, err := tldgit.RepoRoot(path)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%q is not inside a git repository: %w", path, err))
	}
	element, err := s.store.GetElement(ctx, id, api.WorkspaceIDFromCtx(ctx))
	if err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("repository %d not found", id))
	}
	remote, _ := tldgit.DetectRemoteURL(repoRoot)
	if remote == "" {
		remote = element.GetRepo()
	}
	head, _ := tldgit.DetectHeadCommit(repoRoot)
	effectiveBranch := branch
	if effectiveBranch == "" {
		effectiveBranch, _ = tldgit.DetectBranch(repoRoot)
	}
	if effectiveBranch == "" {
		effectiveBranch = element.GetBranch()
	}
	elementID := int64(id)
	if _, err := s.watchStore.EnsureRepository(ctx, watch.RepositoryInput{
		RemoteURL:      remote,
		RepoRoot:       repoRoot,
		DisplayName:    element.GetName(),
		Branch:         effectiveBranch,
		HeadCommit:     head,
		IdentityStatus: "linked",
		RootElementID:  &elementID,
	}); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("register repository: %w", err))
	}
	return nil
}

func (s *impactService) RemoveRepository(ctx context.Context, req *connect.Request[diagv1.RemoveRepositoryRequest]) (*connect.Response[diagv1.RemoveRepositoryResponse], error) {
	ref := strings.TrimSpace(req.Msg.GetRef())
	if watchID, ok := parseWatchRepositoryRef(ref); ok {
		if err := s.removeWatchRepository(ctx, watchID); err != nil {
			return nil, err
		}
		return connect.NewResponse(&diagv1.RemoveRepositoryResponse{}), nil
	}
	id, err := parseElementRef(ref)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if record, ok := s.watchRepositoryForElement(ctx, id); ok && s.watchStore != nil {
		_ = s.watchStore.DeleteRepository(ctx, record.ID)
	}
	if err := s.store.DeleteElement(ctx, id, api.WorkspaceIDFromCtx(ctx)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete repository element: %w", err))
	}
	return connect.NewResponse(&diagv1.RemoveRepositoryResponse{}), nil
}

// removeWatchRepository deletes a watcher-registered checkout that is not
// linked to an architecture element.
func (s *impactService) removeWatchRepository(ctx context.Context, id int64) error {
	if s.watchStore == nil {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("local repository registry unavailable"))
	}
	if _, err := s.watchStore.Repository(ctx, id); err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("repository %s not found", watchRepositoryRef(id)))
	}
	if err := s.watchStore.DeleteRepository(ctx, id); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("delete repository: %w", err))
	}
	return nil
}

func (s *impactService) GetRepositoryStatus(ctx context.Context, req *connect.Request[diagv1.GetRepositoryStatusRequest]) (*connect.Response[diagv1.GetRepositoryStatusResponse], error) {
	ref := strings.TrimSpace(req.Msg.GetRef())
	var summary *diagv1.Repository
	if watchID, ok := parseWatchRepositoryRef(ref); ok {
		recordSummary, err := s.watchRepositorySummary(ctx, watchID)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		summary = recordSummary
	} else {
		id, err := parseElementRef(ref)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		summary, err = s.repositorySummary(ctx, id)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
	}
	var commits []*diagv1.RepositoryCommit
	if summary.GetLocalPath() != "" {
		if recent, err := tldgit.RecentCommits(summary.GetLocalPath(), 40); err == nil {
			commits = protoCommits(recent)
		}
	}
	return connect.NewResponse(&diagv1.GetRepositoryStatusResponse{Repository: summary, Commits: commits}), nil
}

func (s *impactService) ListCommits(_ context.Context, req *connect.Request[diagv1.ListCommitsRequest]) (*connect.Response[diagv1.ListCommitsResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	if path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}
	repoRoot, err := tldgit.RepoRoot(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%q is not inside a git repository: %w", path, err))
	}
	commits, err := tldgit.RecentCommits(repoRoot, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&diagv1.ListCommitsResponse{Commits: protoCommits(commits)}), nil
}

func (s *impactService) ListCommitGraph(_ context.Context, req *connect.Request[diagv1.ListCommitGraphRequest]) (*connect.Response[diagv1.ListCommitGraphResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	if path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}
	repoRoot, err := tldgit.RepoRoot(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%q is not inside a git repository: %w", path, err))
	}
	commits, err := tldgit.HistoryGraph(repoRoot, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&diagv1.ListCommitGraphResponse{Commits: protoCommits(commits)}), nil
}

func (s *impactService) GetCommitDetails(_ context.Context, req *connect.Request[diagv1.GetCommitDetailsRequest]) (*connect.Response[diagv1.GetCommitDetailsResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	sha := strings.TrimSpace(req.Msg.GetSha())
	if path == "" || sha == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path and sha are required"))
	}
	repoRoot, err := tldgit.RepoRoot(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%q is not inside a git repository: %w", path, err))
	}
	header, files, added, removed, err := tldgit.ShowCommit(repoRoot, sha)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	protoFiles := make([]*diagv1.ImpactFile, 0, len(files))
	for _, file := range files {
		protoFiles = append(protoFiles, &diagv1.ImpactFile{
			Path:    file.Path,
			Change:  impactFileChangeType(string(file.Change)),
			Added:   int32(file.Added),
			Removed: int32(file.Removed),
		})
	}
	return connect.NewResponse(&diagv1.GetCommitDetailsResponse{
		Commit:  protoCommit(header),
		Files:   protoFiles,
		Added:   int32(added),
		Removed: int32(removed),
	}), nil
}

func (s *impactService) GetRangeStats(_ context.Context, req *connect.Request[diagv1.GetRangeStatsRequest]) (*connect.Response[diagv1.GetRangeStatsResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	base := strings.TrimSpace(req.Msg.GetBase())
	head := strings.TrimSpace(req.Msg.GetHead())
	if path == "" || base == "" || head == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path, base and head are required"))
	}
	repoRoot, err := tldgit.RepoRoot(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%q is not inside a git repository: %w", path, err))
	}
	count, err := tldgit.RangeCommitCount(repoRoot, base, head)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	changes, err := tldgit.FileChangesBetween(repoRoot, base, head)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	stats, err := tldgit.FileLineStatsBetween(repoRoot, base, head)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	added, removed := 0, 0
	for _, stat := range stats {
		added += stat.Added
		removed += stat.Removed
	}
	return connect.NewResponse(&diagv1.GetRangeStatsResponse{
		Commits:      int32(count),
		FilesChanged: int32(len(changes)),
		Added:        int32(added),
		Removed:      int32(removed),
	}), nil
}

func (s *impactService) AnalyzeImpact(ctx context.Context, req *connect.Request[diagv1.AnalyzeImpactRequest]) (*connect.Response[diagv1.AnalyzeImpactResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	base := strings.TrimSpace(req.Msg.GetBase())
	head := strings.TrimSpace(req.Msg.GetHead())
	if path == "" || base == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path and base are required"))
	}
	if head == "" {
		head = "HEAD"
	}
	repoRoot, err := tldgit.RepoRoot(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%q is not inside a git repository: %w", path, err))
	}
	remote, _ := tldgit.DetectRemoteURL(repoRoot)
	changed, err := tldgit.FileChangesBetween(repoRoot, base, head)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lineStats, _ := tldgit.FileLineStatsBetween(repoRoot, base, head)
	elements := s.elements(ctx)
	opts := watch.ImpactOptions{
		Base:                  base,
		Head:                  head,
		RepoRoot:              repoRoot,
		RemoteURL:             remote,
		Elements:              elements,
		Connectors:            s.connectors(ctx),
		ChangedFiles:          changed,
		LineStats:             lineStats,
		IncludeNameHeuristics: true,
	}
	if req.Msg.GetEvidence() {
		relationships, err := s.detectRelationships(ctx, repoRoot, remote, elements, changed)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		opts.Relationships = relationships
	}
	report := watch.AnalyzeImpact(opts)
	if req.Msg.GetSuggestBindings() {
		cfg, _ := workspace.LoadGlobalConfig()
		suggestions, err := watch.SuggestBindings(ctx, watch.SuggestionOptions{
			RepoRoot:  repoRoot,
			Elements:  elements,
			Unmapped:  report.Unmapped,
			Embedding: watch.ResolveEmbeddingConfig(cfg, "", "", "", 0, 0),
		})
		if err == nil && len(suggestions) > 0 {
			opts.Suggestions = suggestions
			report = watch.AnalyzeImpact(opts)
		}
	}
	run := s.impactRun(ctx, report, changed, remote)
	if s.watchStore != nil {
		if saved, saveErr := s.watchStore.SaveImpactRun(ctx, run); saveErr == nil {
			run = saved
		}
	}
	return connect.NewResponse(s.protoImpactRun(run)), nil
}

// GetLatestImpact returns the most recently persisted run for a checkout so the
// UI can restore the last analysis without recomputing it.
func (s *impactService) GetLatestImpact(ctx context.Context, req *connect.Request[diagv1.GetLatestImpactRequest]) (*connect.Response[diagv1.GetLatestImpactResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	if path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}
	if s.watchStore == nil {
		return connect.NewResponse(&diagv1.GetLatestImpactResponse{}), nil
	}
	repoRoot := path
	if root, err := tldgit.RepoRoot(path); err == nil {
		repoRoot = root
	}
	run, found, err := s.watchStore.LatestImpactRun(ctx, repoRoot)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !found {
		return connect.NewResponse(&diagv1.GetLatestImpactResponse{}), nil
	}
	return connect.NewResponse(&diagv1.GetLatestImpactResponse{
		Found:  true,
		Report: s.protoImpactRun(run),
	}), nil
}

// ListImpactRuns returns persisted impact runs for a checkout, newest first,
// so the UI can browse analysis history and reload a snapshot on demand.
func (s *impactService) ListImpactRuns(ctx context.Context, req *connect.Request[diagv1.ListImpactRunsRequest]) (*connect.Response[diagv1.ListImpactRunsResponse], error) {
	path := strings.TrimSpace(req.Msg.GetPath())
	if path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}
	resp := &diagv1.ListImpactRunsResponse{}
	if s.watchStore == nil {
		return connect.NewResponse(resp), nil
	}
	repoRoot := path
	if root, err := tldgit.RepoRoot(path); err == nil {
		repoRoot = root
	}
	runs, hasMore, err := s.watchStore.ListImpactRuns(ctx, repoRoot, int(req.Msg.GetLimit()), int(req.Msg.GetOffset()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, &diagv1.ImpactRunSnapshot{
			Id:        run.ID,
			Base:      run.Base,
			Head:      run.Head,
			CreatedAt: run.CreatedAt,
			Report:    s.protoImpactRun(run),
		})
	}
	resp.HasMore = hasMore
	return connect.NewResponse(resp), nil
}

// impactRun trims a deterministic report into the persisted snapshot shape,
// resolving element and connector ids so a loaded run renders without the live
// architecture.
func (s *impactService) impactRun(ctx context.Context, report watch.ImpactReport, changed map[string]tldgit.WorktreeChange, remote string) watch.ImpactRun {
	ids := s.elementIDs(ctx)
	connectors := s.connectorIDs(ctx)
	elements := s.elements(ctx)
	return watch.ImpactRun{
		RepoRef:      repositoryRefForRemote(elements, remote),
		RepoRoot:     report.RepoRoot,
		RemoteURL:    remote,
		Base:         report.Base,
		Head:         report.Head,
		ArchRevision: watch.ArchitectureRevision(elements, s.connectors(ctx)),
		ChangedFiles: report.ChangedFiles,
		Changed:      impactRunElements(report.Changed, ids, changed),
		Candidates:   impactRunElements(report.Candidates, ids, changed),
		Related:      impactRunElements(report.Related, ids, changed),
		Edges:        impactRunEdges(report.Edges, ids, connectors),
		Unmapped:     report.Unmapped,
		Coverage:     report.Coverage,
	}
}

func repositoryRefForRemote(elements map[string]*workspace.Element, remote string) string {
	if strings.TrimSpace(remote) == "" {
		return ""
	}
	for ref, element := range elements {
		if element == nil {
			continue
		}
		if strings.EqualFold(element.Kind, "repository") && sameRepo(element.Repo, remote) {
			return ref
		}
	}
	return ""
}

func impactRunElements(elements []watch.ImpactElement, ids map[string]int32, changed map[string]tldgit.WorktreeChange) []watch.ImpactRunElement {
	out := make([]watch.ImpactRunElement, 0, len(elements))
	for _, element := range elements {
		item := watch.ImpactRunElement{
			Ref:      element.Ref,
			Name:     element.Name,
			Kind:     element.Kind,
			Owner:    element.Owner,
			Change:   impactChangeTypeString(element, changed),
			Evidence: impactEvidenceStrings(element),
		}
		if id, ok := ids[element.Ref]; ok {
			item.ElementID = &id
		}
		out = append(out, item)
	}
	return out
}

func impactRunEdges(edges []watch.ImpactEdge, ids map[string]int32, connectors map[string]int32) []watch.ImpactRunEdge {
	out := make([]watch.ImpactRunEdge, 0, len(edges))
	for _, edge := range edges {
		item := watch.ImpactRunEdge{
			SourceRef: edge.SourceRef,
			TargetRef: edge.TargetRef,
			Label:     edge.Label,
			Observed:  edge.Observed,
		}
		sourceID, okSource := ids[edge.SourceRef]
		targetID, okTarget := ids[edge.TargetRef]
		if okSource && okTarget {
			if connectorID, ok := connectors[connectorKey(sourceID, targetID, edge.Label)]; ok {
				item.ConnectorID = &connectorID
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *impactService) detectRelationships(ctx context.Context, repoRoot, remote string, elements map[string]*workspace.Element, changed map[string]tldgit.WorktreeChange) ([]watch.RelationshipEvidence, error) {
	if s.watchStore == nil {
		return nil, nil
	}
	files := make([]string, 0, len(changed))
	for file := range changed {
		files = append(files, file)
	}
	cfg, _ := workspace.LoadGlobalConfig()
	branch, _ := tldgit.DetectBranch(repoRoot)
	head, _ := tldgit.DetectHeadCommit(repoRoot)
	return watch.DetectObservedRelationships(ctx, s.watchStore, watch.RelationshipOptions{
		RepoRoot:     repoRoot,
		RemoteURL:    remote,
		Branch:       branch,
		HeadCommit:   head,
		ChangedFiles: files,
		Elements:     elements,
		Settings:     watch.ResolveSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0),
		DataDir:      s.dataDir,
	})
}

// elements returns the workspace elements keyed by a stable ref derived from
// the element id, matching the refs used in the impact report.
func (s *impactService) elements(ctx context.Context) map[string]*workspace.Element {
	out := map[string]*workspace.Element{}
	elements, _, err := s.store.ListElements(ctx, api.WorkspaceIDFromCtx(ctx), 0, 0, "")
	if err != nil {
		return out
	}
	for _, element := range elements {
		out[elementRef(element.GetId())] = &workspace.Element{
			Name:     element.GetName(),
			Kind:     element.GetKind(),
			Repo:     element.GetRepo(),
			Branch:   element.GetBranch(),
			Language: element.GetLanguage(),
			FilePath: element.GetFilePath(),
		}
	}
	return out
}

func (s *impactService) connectors(ctx context.Context) map[string]*workspace.Connector {
	out := map[string]*workspace.Connector{}
	connectors, err := s.store.ListAllConnectors(ctx, api.WorkspaceIDFromCtx(ctx))
	if err != nil {
		return out
	}
	for _, connector := range connectors {
		spec := &workspace.Connector{
			View:   elementRef(connector.GetViewId()),
			Source: elementRef(connector.GetSourceElementId()),
			Target: elementRef(connector.GetTargetElementId()),
			Label:  connector.GetRelationship(),
		}
		out[workspace.ConnectorKey(spec)] = spec
	}
	return out
}

func (s *impactService) repositorySummary(ctx context.Context, id int32) (*diagv1.Repository, error) {
	element, err := s.store.GetElement(ctx, id, api.WorkspaceIDFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("repository %d not found", id)
	}
	summary := &diagv1.Repository{
		Ref:             elementRef(id),
		ElementId:       id,
		Name:            element.GetName(),
		RemoteUrl:       element.GetRepo(),
		Branch:          element.GetBranch(),
		HasArchitecture: s.hasArchitecture(ctx, element.GetRepo()),
	}
	if record, ok := s.watchRepositoryForElement(ctx, id); ok {
		summary.LocalPath = record.RepoRoot
		summary.HeadCommit = record.HeadCommit.String
		if summary.Branch == "" {
			summary.Branch = record.Branch.String
		}
	} else if path, branch, head, ok := s.workspaceCheckout(element.GetRepo()); ok {
		summary.LocalPath = path
		summary.HeadCommit = head
		if summary.Branch == "" {
			summary.Branch = branch
		}
	}
	return summary, nil
}

// watchRepositorySummary builds a repository summary from a watcher-registered
// checkout that may not be linked to an architecture element.
func (s *impactService) watchRepositorySummary(ctx context.Context, id int64) (*diagv1.Repository, error) {
	if s.watchStore == nil {
		return nil, fmt.Errorf("local repository registry unavailable")
	}
	record, err := s.watchStore.Repository(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("repository %s not found", watchRepositoryRef(id))
	}
	summary := &diagv1.Repository{
		Ref:             watchRepositoryRef(record.ID),
		Name:            record.DisplayName,
		RemoteUrl:       record.RemoteURL.String,
		Branch:          record.Branch.String,
		LocalPath:       record.RepoRoot,
		HeadCommit:      record.HeadCommit.String,
		HasArchitecture: s.hasArchitecture(ctx, record.RemoteURL.String),
	}
	if record.RootElementID.Valid {
		summary.ElementId = int32(record.RootElementID.Int64)
		if element, err := s.store.GetElement(ctx, summary.ElementId, api.WorkspaceIDFromCtx(ctx)); err == nil && element.GetName() != "" {
			summary.Name = element.GetName()
		}
	}
	return summary, nil
}

func (s *impactService) watchRepositoryForElement(ctx context.Context, id int32) (watch.Repository, bool) {
	if s.watchStore == nil {
		return watch.Repository{}, false
	}
	records, err := s.watchStore.Repositories(ctx)
	if err != nil {
		return watch.Repository{}, false
	}
	for _, record := range records {
		if record.RootElementID.Valid && record.RootElementID.Int64 == int64(id) {
			return record, true
		}
	}
	return watch.Repository{}, false
}

// hasArchitecture reports whether the workspace contains an architecture
// representation for the repository. When elements carry repository metadata,
// a match is required; otherwise (a single-repo or UI-authored workspace) any
// non-repository element counts as architecture.
func (s *impactService) hasArchitecture(ctx context.Context, remote string) bool {
	if strings.TrimSpace(remote) == "" {
		return false
	}
	hasAny, anyRepoInfo := false, false
	for _, element := range s.elements(ctx) {
		if strings.EqualFold(element.Kind, "repository") {
			continue
		}
		hasAny = true
		if strings.TrimSpace(element.Repo) != "" {
			anyRepoInfo = true
		}
		if sameRepo(element.Repo, remote) {
			return true
		}
	}
	if anyRepoInfo {
		return false
	}
	return hasAny
}

func (s *impactService) protoImpactRun(run watch.ImpactRun) *diagv1.AnalyzeImpactResponse {
	resp := &diagv1.AnalyzeImpactResponse{
		Base:         run.Base,
		Head:         run.Head,
		RepoRoot:     run.RepoRoot,
		Unmapped:     run.Unmapped,
		Changed:      protoRunElements(run.Changed),
		Candidates:   protoRunElements(run.Candidates),
		Related:      protoRunElements(run.Related),
		Coverage:     protoCoverage(run.Coverage),
		ChangedFiles: protoImpactFiles(run.ChangedFiles),
	}
	for _, edge := range run.Edges {
		resp.Edges = append(resp.Edges, &diagv1.ImpactEdge{
			SourceRef:   edge.SourceRef,
			TargetRef:   edge.TargetRef,
			Label:       edge.Label,
			Observed:    edge.Observed,
			ConnectorId: edge.ConnectorID,
		})
	}
	return resp
}

func protoImpactFiles(files []watch.ChangedFile) []*diagv1.ImpactFile {
	out := make([]*diagv1.ImpactFile, 0, len(files))
	for _, file := range files {
		out = append(out, &diagv1.ImpactFile{
			Path:    file.Path,
			Change:  impactFileChangeType(file.Change),
			Added:   int32(file.Added),
			Removed: int32(file.Removed),
		})
	}
	return out
}

func impactFileChangeType(change string) diagv1.ImpactChangeType {
	switch tldgit.WorktreeChange(change) {
	case tldgit.WorktreeAdded:
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_ADDED
	case tldgit.WorktreeDeleted:
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_DELETED
	case tldgit.WorktreeUpdated:
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_MODIFIED
	default:
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_UNSPECIFIED
	}
}

func (s *impactService) elementIDs(ctx context.Context) map[string]int32 {
	ids := map[string]int32{}
	elements, _, err := s.store.ListElements(ctx, api.WorkspaceIDFromCtx(ctx), 0, 0, "")
	if err != nil {
		return ids
	}
	for _, element := range elements {
		ids[elementRef(element.GetId())] = element.GetId()
	}
	return ids
}

func (s *impactService) connectorIDs(ctx context.Context) map[string]int32 {
	ids := map[string]int32{}
	connectors, err := s.store.ListAllConnectors(ctx, api.WorkspaceIDFromCtx(ctx))
	if err != nil {
		return ids
	}
	for _, connector := range connectors {
		ids[connectorKey(connector.GetSourceElementId(), connector.GetTargetElementId(), connector.GetRelationship())] = connector.GetId()
	}
	return ids
}

func protoRunElements(elements []watch.ImpactRunElement) []*diagv1.ImpactElement {
	out := make([]*diagv1.ImpactElement, 0, len(elements))
	for _, element := range elements {
		out = append(out, &diagv1.ImpactElement{
			Ref:       element.Ref,
			Name:      element.Name,
			Kind:      element.Kind,
			ElementId: element.ElementID,
			Change:    impactChangeTypeFromString(element.Change),
			Evidence:  element.Evidence,
		})
	}
	return out
}

func impactChangeTypeString(element watch.ImpactElement, changed map[string]tldgit.WorktreeChange) string {
	added, deleted, other := 0, 0, 0
	for _, evidence := range element.Evidence {
		if evidence.Kind == "contains" {
			continue
		}
		switch changed[evidence.Path] {
		case tldgit.WorktreeAdded:
			added++
		case tldgit.WorktreeDeleted:
			deleted++
		default:
			other++
		}
	}
	switch {
	case added > 0 && deleted == 0 && other == 0:
		return "added"
	case deleted > 0 && added == 0 && other == 0:
		return "deleted"
	default:
		return "modified"
	}
}

func impactChangeTypeFromString(value string) diagv1.ImpactChangeType {
	switch value {
	case "added":
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_ADDED
	case "deleted":
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_DELETED
	case "modified":
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_MODIFIED
	default:
		return diagv1.ImpactChangeType_IMPACT_CHANGE_TYPE_UNSPECIFIED
	}
}

func impactEvidenceStrings(element watch.ImpactElement) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, evidence := range element.Evidence {
		// "contains" evidence trails ancestor rollup, not owned files, and is
		// skipped so badges and markdown attribute files to their real owner.
		if evidence.Kind == "contains" {
			continue
		}
		// Prefer the changed file so persisted runs can attribute source detail
		// back to the element; fall back to the binding detail when no file
		// matched (for example observed relationships).
		value := strings.TrimSpace(evidence.Path)
		if value == "" {
			value = strings.TrimSpace(evidence.Detail)
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func protoCoverage(coverage watch.Coverage) *diagv1.ImpactCoverage {
	out := &diagv1.ImpactCoverage{
		Applicable:          coverage.Applicable,
		Complete:            coverage.Complete,
		Score:               coverage.Score,
		Percent:             int32(coverage.Percent),
		Confidence:          coverage.Confidence,
		SourceFiles:         int32(coverage.SourceFiles),
		BoundSourceFiles:    int32(coverage.BoundSourceFiles),
		WeakSourceFiles:     int32(coverage.WeakSourceFiles),
		UnmappedSourceFiles: int32(coverage.UnmappedSource),
		NonSourceFiles:      int32(coverage.NonSourceFiles),
		AnchoredElements:    int32(coverage.AnchoredElements),
		TotalElements:       int32(coverage.TotalElements),
	}
	for _, gap := range coverage.Gaps {
		out.Gaps = append(out.Gaps, &diagv1.ImpactCoverageGap{
			File:                    gap.File,
			Change:                  gap.Change,
			Reason:                  gap.Reason,
			SuggestedElementRef:     gap.SuggestedRef,
			SuggestedElementName:    gap.SuggestedName,
			SuggestedScore:          gap.SuggestedScore,
			SuggestedElementPattern: gap.CurrentPattern,
			SuggestedNewElement:     gap.NewElementName,
			SuggestedNewRef:         gap.NewElementRef,
		})
	}
	return out
}

func protoCommit(commit tldgit.Commit) *diagv1.RepositoryCommit {
	return &diagv1.RepositoryCommit{
		Sha:         commit.SHA,
		ShortSha:    commit.ShortSHA,
		Subject:     commit.Subject,
		Author:      commit.Author,
		Date:        commit.Date,
		Parents:     commit.Parents,
		Refs:        commit.Refs,
		AuthorEmail: commit.AuthorEmail,
		Body:        commit.Body,
	}
}

func protoCommits(commits []tldgit.Commit) []*diagv1.RepositoryCommit {
	out := make([]*diagv1.RepositoryCommit, 0, len(commits))
	for _, commit := range commits {
		out = append(out, protoCommit(commit))
	}
	return out
}

func connectorKey(sourceID, targetID int32, label string) string {
	return fmt.Sprintf("%d>%d|%s", sourceID, targetID, strings.TrimSpace(label))
}

func elementRef(id int32) string {
	return strconv.Itoa(int(id))
}

// watchRepositoryRefPrefix marks a ref that names a watcher-registered local
// checkout rather than an architecture element. Unlinked checkouts have no
// element id, so they need their own resolvable reference.
const watchRepositoryRefPrefix = "watch:"

func watchRepositoryRef(id int64) string {
	return watchRepositoryRefPrefix + strconv.FormatInt(id, 10)
}

func parseWatchRepositoryRef(ref string) (int64, bool) {
	value := strings.TrimSpace(ref)
	if !strings.HasPrefix(value, watchRepositoryRefPrefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, watchRepositoryRefPrefix), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseElementRef(ref string) (int32, error) {
	id, err := strconv.Atoi(strings.TrimSpace(ref))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid repository reference %q", ref)
	}
	return int32(id), nil
}

func strPtr(value string) *string {
	return &value
}

func sameRepo(a, b string) bool {
	normalizedA := normalizeRepo(a)
	normalizedB := normalizeRepo(b)
	if normalizedA == "" || normalizedB == "" {
		return false
	}
	return normalizedA == normalizedB ||
		strings.HasSuffix(normalizedA, normalizedB) ||
		strings.HasSuffix(normalizedB, normalizedA)
}

func normalizeRepo(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, ".git")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "git@")
	value = strings.ReplaceAll(value, ":", "/")
	return strings.Trim(value, "/")
}
