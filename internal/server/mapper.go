package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mapper"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// mapGroupingMode selects the grouping engine. The dependency-graph community
// pipeline is the default; set TLD_MAP_GROUPING=embedding for the legacy
// embedding clustering pipeline.
func mapGroupingMode() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("TLD_MAP_GROUPING")))
}

// mapperService runs the repository mapping pipeline and materializes its
// grouping hierarchy into the workspace. The default grouping is deterministic
// dependency-graph community detection; TLD_MAP_GROUPING=embedding selects the
// legacy embedding clustering/binning pipeline.
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
	if mapGroupingMode() != "embedding" {
		result, err := s.mapWithCommunities(ctx, req, repositoryID, snapshot, send)
		if err != nil {
			return err
		}
		return stream.Send(&codeindexv1.MapRepositoryEvent{Event: &codeindexv1.MapRepositoryEvent_Result{Result: result}})
	}
	snapshotID := snapshot.Id
	if err := s.ensureEmbeddings(ctx, snapshot, send); err != nil {
		return err
	}
	profile := strings.TrimSpace(req.Msg.GetProfile())
	if profile == "" {
		majority, err := s.idx.MajorityProfile(ctx, snapshotID)
		if err != nil {
			return connect.NewError(connect.CodeInternal, err)
		}
		if majority == "" {
			return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("snapshot has no embeddings; run `tld index --embed` first"))
		}
		profile = majority
	}

	options := mapper.DefaultOptions()
	binOptions := mapper.DefaultBinOptions()
	configBytes, _ := json.Marshal(mapOptionsParams(options, binOptions))
	configHash := cgraph.ID("mapper-v1", string(configBytes), strconv.FormatBool(req.Msg.IncludeImports))
	completed, err := s.idx.CompletedMaps(ctx, repositoryID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	for _, record := range completed {
		if record.Result.SnapshotId == snapshotID && record.Profile == profile && record.ConfigHash == configHash {
			if _, err := s.ws.ViewByID(ctx, record.Result.ViewId); err == nil {
				return stream.Send(&codeindexv1.MapRepositoryEvent{Event: &codeindexv1.MapRepositoryEvent_Result{Result: record.Result}})
			}
		}
	}
	send(&codeindexv1.MapProgress{Stage: "loading", Detail: "loading embeddings"})
	factVectors, err := s.idx.FactEmbeddings(ctx, snapshotID, profile, codeindexv1.FactKind_FACT_KIND_FILE)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	dataset, err := buildMapDataset(ctx, s.idx, repositoryID, snapshotID, profile, factVectors)
	if err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	send(&codeindexv1.MapProgress{Stage: "loading", Current: uint32(len(dataset.Facts)), Total: uint32(len(dataset.Facts)), Detail: "loaded"})

	pipeline, err := mapper.RunPipelineProgress(dataset.Vectors, &options, func(progress mapper.Progress) {
		send(&codeindexv1.MapProgress{Stage: "clustering", Current: uint32(progress.Current), Total: uint32(progress.Total), Detail: progress.Detail})
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	send(&codeindexv1.MapProgress{Stage: "binning"})
	bins, err := mapper.BuildBins(dataset, pipeline, &binOptions)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	repositoryRoot := ""
	if repo, err := s.idx.Repository(ctx, repositoryID); err == nil && repo != nil {
		repositoryRoot = repo.GetRoot()
	}
	repositoryName := repositoryID
	if base := filepath.Base(repositoryRoot); repositoryRoot != "" && base != "." && base != "/" {
		repositoryName = base
	}
	runID := cgraph.ID(repositoryID, snapshotID, "mapper", profile, configHash)
	domainNames, err := mapper.NameDomains(dataset, pipeline.Domains, mapper.DefaultNameOptions())
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	groups := make([]cstore.AnalysisGroup, 0, len(pipeline.Domains))
	for i, domain := range pipeline.Domains {
		label := domainNames[i]
		if label == "" && i < len(bins.Units) {
			label = bins.Units[i].Folder
		}
		members := make([]string, 0, len(domain.Members))
		for _, member := range domain.Members {
			if member >= 0 && member < len(dataset.Facts) {
				members = append(members, dataset.Facts[member].ID)
			}
		}
		groups = append(groups, cstore.AnalysisGroup{
			ID:      fmt.Sprintf("%s:%d", runID, i),
			Label:   label,
			Kind:    codeindexv1.GroupKind_GROUP_KIND_CLUSTER,
			Profile: profile,
			Members: members,
		})
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	send(&codeindexv1.MapProgress{Stage: "materializing"})
	fileEdges, err := s.idx.FileEdges(ctx, snapshotID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	var imports []materialize.MapImport
	if req.Msg.GetIncludeImports() {
		fileImports, err := s.idx.FileImports(ctx, snapshotID)
		if err != nil {
			return connect.NewError(connect.CodeInternal, err)
		}
		imports = mapImports(fileImports)
	}
	mapResult, err := materialize.ApplyMap(ctx, s.ws, s.idx, materialize.MapInput{
		RepositoryID:   repositoryID,
		RepositoryName: repositoryName,
		RepositoryRoot: repositoryRoot,
		SnapshotID:     snapshotID,
		RunID:          runID,
		Dataset:        dataset,
		Bins:           bins,
		Edges:          mapEdges(fileEdges),
		Imports:        imports,
	}, materialize.MapOptions{
		Progress: func(current, total int, detail string) {
			send(&codeindexv1.MapProgress{Stage: "materializing", Current: uint32(current), Total: uint32(total), Detail: detail})
		},
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	result := &codeindexv1.MapResult{
		RunId: runID, SnapshotId: snapshotID, ViewId: mapResult.ViewID,
		Facts: uint32(len(dataset.Facts)), Clusters: uint32(len(pipeline.Domains)),
		Bins: uint32(len(bins.Sizes)), Unclustered: uint32(len(pipeline.Leftovers)),
		WeightedTightness: pipeline.Metrics.WeightedTightness,
	}
	if err := s.idx.SaveAnalysis(ctx, cstore.AnalysisRun{
		ID:           runID,
		RepositoryID: repositoryID,
		SnapshotID:   snapshotID,
		Algorithm:    "mapper",
		Params:       mapOptionsParams(options, binOptions),
		Groups:       groups,
	}); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	if err := s.idx.SaveCompletedMap(ctx, repositoryID, &codeindexv1.CompletedMap{Result: result, Profile: profile, IncludeImports: req.Msg.IncludeImports, ConfigHash: configHash}); err != nil {
		return connect.NewError(connect.CodeInternal, err)
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

func mapEdges(edges []cstore.FileEdge) []materialize.MapEdge {
	out := make([]materialize.MapEdge, 0, len(edges))
	for _, edge := range edges {
		out = append(out, materialize.MapEdge{FromFactID: edge.FromFactID, ToFactID: edge.ToFactID, Weight: edge.Weight})
	}
	return out
}

func mapImports(imports []cstore.FileImport) []materialize.MapImport {
	out := make([]materialize.MapImport, 0, len(imports))
	for _, item := range imports {
		out = append(out, materialize.MapImport{FileFactID: item.FileFactID, Import: item.Import})
	}
	return out
}

// buildMapDataset mirrors the Rust loader: majority decoded dimension, stable
// ID ordering and display-name fallback.
func buildMapDataset(ctx context.Context, idx *cstore.Store, repositoryID, snapshotID, profile string, rows []cstore.FactVector) (*mapper.Dataset, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("snapshot/profile has no usable vectors for requested fact kind")
	}
	counts := map[int]int{}
	order := make([]int, 0)
	for _, row := range rows {
		dimension := len(row.Vector)
		if _, ok := counts[dimension]; !ok {
			order = append(order, dimension)
		}
		counts[dimension]++
	}
	majority := order[0]
	for _, dimension := range order {
		if counts[dimension] > counts[majority] {
			majority = dimension
		}
	}
	facts := make([]mapper.Fact, 0, len(rows))
	vectors := make([][]float64, 0, len(rows))
	for _, row := range rows {
		if len(row.Vector) != majority {
			continue
		}
		name := row.Fact.GetName()
		if name == "" {
			name = row.Fact.GetQualifiedName()
		}
		if name == "" {
			name = row.Fact.GetId()
		}
		vector := make([]float64, len(row.Vector))
		for i, value := range row.Vector {
			vector[i] = float64(value)
		}
		facts = append(facts, mapper.Fact{
			ID:          row.Fact.GetId(),
			Path:        row.Path,
			DisplayName: name,
			Language:    row.Fact.GetLanguage(),
		})
		vectors = append(vectors, vector)
	}
	root := ""
	if repo, err := idx.Repository(ctx, repositoryID); err == nil && repo != nil {
		root = repo.GetRoot()
	}
	dataset := &mapper.Dataset{Snapshot: snapshotID, Profile: profile, Root: &root, Facts: facts, Vectors: vectors}
	if err := dataset.Validate(); err != nil {
		return nil, err
	}
	return dataset, nil
}

func mapOptionsParams(options mapper.Options, bins mapper.BinOptions) map[string]string {
	params := map[string]string{
		"neighbors":                strconv.Itoa(options.Neighbors),
		"min_similarity":           strconv.FormatFloat(options.MinSimilarity, 'g', -1, 64),
		"symmetric_neighbors":      strconv.FormatBool(options.SymmetricNeighbors),
		"split_step":               strconv.FormatFloat(options.SplitStep, 'g', -1, 64),
		"member_floor":             strconv.FormatFloat(options.MemberFloor, 'g', -1, 64),
		"tightness_floor":          strconv.FormatFloat(options.TightnessFloor, 'g', -1, 64),
		"folder_pooling_threshold": strconv.Itoa(bins.FolderPoolingThreshold),
	}
	if options.SweepFloor != nil {
		params["sweep_floor"] = strconv.FormatFloat(*options.SweepFloor, 'g', -1, 64)
	}
	return params
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
	snapshotID := snapshot.Id
	configHash := cgraph.ID("group-v2", strconv.FormatBool(req.Msg.GetIncludeImports()))
	completed, err := s.idx.CompletedMaps(ctx, repositoryID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, record := range completed {
		if record.Result.SnapshotId == snapshotID && record.Profile == "" && record.ConfigHash == configHash {
			if _, err := s.ws.ViewByID(ctx, record.Result.ViewId); err == nil {
				return record.Result, nil
			}
		}
	}

	send(&codeindexv1.MapProgress{Stage: "loading", Detail: "loading file graph"})
	facts, err := s.idx.AllFacts(ctx, snapshotID, codeindexv1.FactKind_FACT_KIND_FILE)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(facts) == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("snapshot has no file facts to group"))
	}
	files := make([]community.File, 0, len(facts))
	datasetFacts := make([]mapper.Fact, 0, len(facts))
	indexOf := make(map[string]int, len(facts))
	for i, fact := range facts {
		path := ""
		if fact.GetAnchor() != nil {
			path = fact.GetAnchor().GetPath()
		}
		name := fact.GetName()
		if name == "" {
			name = fact.GetQualifiedName()
		}
		if name == "" {
			name = fact.GetId()
		}
		files = append(files, community.File{ID: fact.GetId(), Path: path, DisplayName: name, Language: fact.GetLanguage()})
		datasetFacts = append(datasetFacts, mapper.Fact{ID: fact.GetId(), Path: path, DisplayName: name, Language: fact.GetLanguage()})
		indexOf[fact.GetId()] = i
	}
	repositoryRoot := ""
	if repo, err := s.idx.Repository(ctx, repositoryID); err == nil && repo != nil {
		repositoryRoot = repo.GetRoot()
	}
	dataset := &mapper.Dataset{Snapshot: snapshotID, Profile: "", Root: &repositoryRoot, Facts: datasetFacts}
	send(&codeindexv1.MapProgress{Stage: "loading", Current: uint32(len(facts)), Total: uint32(len(facts)), Detail: "loaded"})

	fileEdges, err := s.idx.AggregatedFileEdges(ctx, snapshotID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	communityEdges := make([]community.Edge, 0, len(fileEdges))
	mapEdges := make([]materialize.MapEdge, 0, len(fileEdges))
	for _, edge := range fileEdges {
		from, fromOK := indexOf[edge.FromFactID]
		to, toOK := indexOf[edge.ToFactID]
		if fromOK && toOK {
			communityEdges = append(communityEdges, community.Edge{A: from, B: to, Weight: edge.Weight})
		}
		mapEdges = append(mapEdges, materialize.MapEdge{FromFactID: edge.FromFactID, ToFactID: edge.ToFactID, Weight: edge.Weight})
	}

	send(&codeindexv1.MapProgress{Stage: "grouping", Detail: "grouping dependencies"})
	options := community.DefaultOptions()
	options.NameFallback = communityNameFallback(dataset)
	grouping, err := community.Build(files, communityEdges, options)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	materialize.SortGroups(grouping.Groups)
	send(&codeindexv1.MapProgress{
		Stage:   "grouping",
		Current: uint32(grouping.Metrics.RootGroups),
		Total:   uint32(grouping.Metrics.RootGroups),
		Detail:  fmt.Sprintf("%d components · modularity %.2f · %d isolated files", grouping.Metrics.RootGroups, grouping.Metrics.Modularity, grouping.Metrics.IsolatedFiles),
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repositoryName := repositoryID
	if base := filepath.Base(repositoryRoot); repositoryRoot != "" && base != "." && base != "/" {
		repositoryName = base
	}
	var imports []materialize.MapImport
	if req.Msg.GetIncludeImports() {
		fileImports, err := s.idx.FileImports(ctx, snapshotID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		imports = mapImports(fileImports)
	}

	runID := cgraph.ID(repositoryID, snapshotID, "community", configHash)
	send(&codeindexv1.MapProgress{Stage: "materializing"})
	mapResult, err := materialize.ApplyGroupMap(ctx, s.ws, s.idx, materialize.GroupMapInput{
		RepositoryID:   repositoryID,
		RepositoryName: repositoryName,
		RepositoryRoot: repositoryRoot,
		SnapshotID:     snapshotID,
		Dataset:        dataset,
		Groups:         grouping.Groups,
		Edges:          mapEdges,
		Imports:        imports,
	}, materialize.MapOptions{
		MaxLeafConnectorsPerView: materialize.DefaultMaxLeafConnectorsPerView,
		Progress: func(current, total int, detail string) {
			send(&codeindexv1.MapProgress{Stage: "materializing", Current: uint32(current), Total: uint32(total), Detail: detail})
		},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	result := &codeindexv1.MapResult{
		RunId:             runID,
		SnapshotId:        snapshotID,
		ViewId:            mapResult.ViewID,
		Facts:             uint32(len(facts)),
		Clusters:          uint32(grouping.Metrics.RootGroups),
		Bins:              uint32(grouping.Metrics.Groups),
		Unclustered:       uint32(grouping.Metrics.IsolatedFiles),
		WeightedTightness: grouping.Metrics.Modularity,
	}
	params := make(map[string]string, len(grouping.Params)+6)
	for key, value := range grouping.Params {
		params[key] = value
	}
	params["root_groups"] = strconv.Itoa(grouping.Metrics.RootGroups)
	params["total_groups"] = strconv.Itoa(grouping.Metrics.Groups)
	params["modularity"] = strconv.FormatFloat(grouping.Metrics.Modularity, 'g', -1, 64)
	params["cross_weight"] = strconv.FormatFloat(grouping.Metrics.CrossWeight, 'g', -1, 64)
	params["isolated_files"] = strconv.Itoa(grouping.Metrics.IsolatedFiles)
	params["folder_coverage"] = strconv.FormatFloat(grouping.Metrics.FolderCoverage, 'g', -1, 64)
	if err := s.idx.SaveAnalysis(ctx, cstore.AnalysisRun{
		ID:           runID,
		RepositoryID: repositoryID,
		SnapshotID:   snapshotID,
		Algorithm:    "louvain",
		Params:       params,
		Groups:       analysisGroups(runID, grouping.Groups, files),
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.idx.SaveCompletedMap(ctx, repositoryID, &codeindexv1.CompletedMap{
		Result:         result,
		Profile:        "",
		IncludeImports: req.Msg.GetIncludeImports(),
		ConfigHash:     configHash,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return result, nil
}

// communityNameFallback adapts the mapper's lexical name index to the
// community package so groups without a discriminating folder still get a name.
// It requests two candidate tokens and returns the first that is not a file
// format, so a frontend component community is not named "tsx" just because its
// members are .tsx files.
func communityNameFallback(dataset *mapper.Dataset) func(members []int) string {
	options := mapper.DefaultNameOptions()
	options.Top = 2
	index, err := mapper.NewNameIndex(dataset, options)
	if err != nil || index == nil {
		return nil
	}
	return func(members []int) string {
		for _, field := range strings.Fields(index.Name(members)) {
			if _, ok := communityFormatTokens[field]; !ok {
				return field
			}
		}
		return ""
	}
}

var communityFormatTokens = map[string]struct{}{
	"tsx": {}, "jsx": {}, "css": {}, "scss": {}, "less": {}, "html": {}, "htm": {},
	"json": {}, "yaml": {}, "yml": {}, "toml": {}, "svg": {}, "png": {}, "jpg": {},
	"jpeg": {}, "gif": {}, "ico": {}, "lock": {}, "sql": {}, "proto": {}, "md": {},
}

// analysisGroups flattens a community hierarchy into persisted analysis groups,
// each carrying its full descendant membership and stable key-derived id.
func analysisGroups(runID string, groups []*community.Group, files []community.File) []cstore.AnalysisGroup {
	out := make([]cstore.AnalysisGroup, 0, len(groups))
	var walk func(group *community.Group)
	walk = func(group *community.Group) {
		members := make([]string, 0, group.Files)
		var collect func(item *community.Group)
		collect = func(item *community.Group) {
			for _, member := range item.Members {
				if member >= 0 && member < len(files) {
					members = append(members, files[member].ID)
				}
			}
			for _, child := range item.Children {
				collect(child)
			}
		}
		collect(group)
		sort.Strings(members)
		out = append(out, cstore.AnalysisGroup{
			ID:      runID + ":g:" + group.Key,
			Label:   group.Name,
			Kind:    codeindexv1.GroupKind_GROUP_KIND_COMMUNITY,
			Profile: "",
			Members: members,
		})
		for _, child := range group.Children {
			walk(child)
		}
	}
	for _, group := range groups {
		if group != nil {
			walk(group)
		}
	}
	return out
}
