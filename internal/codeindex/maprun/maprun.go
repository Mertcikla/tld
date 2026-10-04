// Package maprun runs the graph mapping pipeline: it loads a snapshot's file
// facts and aggregated edges, groups them into a Louvain community hierarchy,
// and materializes the result into workspace views. The server RPC and the CLI
// share this runner so mapping behaves identically everywhere.
package maprun

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// ErrNoFileFacts reports a snapshot without file facts to group.
var ErrNoFileFacts = errors.New("snapshot has no file facts to group")

// Deps are the stores a map run reads from and writes to.
type Deps struct {
	Workspace core.Store
	Codeindex *cstore.Store
	Options   mapconfig.Options
}

// Request selects which snapshot to map.
type Request struct {
	RepositoryID   string
	SnapshotID     string
	IncludeImports bool
}

// ProgressFunc receives coarse pipeline updates. It may be nil.
type ProgressFunc func(stage string, current, total int, detail string)

// Run maps the requested snapshot. cached reports that an identical completed
// map was reused and no work was performed.
func Run(ctx context.Context, deps Deps, req Request, progress ProgressFunc) (*codeindexv1.MapResult, bool, error) {
	if deps.Workspace == nil || deps.Codeindex == nil {
		return nil, false, fmt.Errorf("map run requires workspace and codeindex stores")
	}
	report := func(stage string, current, total int, detail string) {
		if progress != nil {
			progress(stage, current, total, detail)
		}
	}
	repositoryID, snapshotID := req.RepositoryID, req.SnapshotID
	overrides, err := deps.Codeindex.RepositoryMapOverrides(ctx, repositoryID)
	if err != nil {
		return nil, false, err
	}
	deps.Options, err = deps.Options.WithOverrides(overrides)
	if err != nil {
		return nil, false, err
	}
	configHash := deps.Options.ConfigHash(req.IncludeImports)

	completed, err := deps.Codeindex.CompletedMaps(ctx, repositoryID)
	if err != nil {
		return nil, false, err
	}
	for _, record := range completed {
		if record.Result.SnapshotId == snapshotID && record.ConfigHash == configHash {
			if _, err := deps.Workspace.ViewByID(ctx, record.Result.ViewId); err == nil {
				return record.Result, true, nil
			}
		}
	}

	report("loading", 0, 0, "loading file graph")
	facts, err := deps.Codeindex.AllFacts(ctx, snapshotID, codeindexv1.FactKind_FACT_KIND_FILE)
	if err != nil {
		return nil, false, err
	}
	if len(facts) == 0 {
		return nil, false, ErrNoFileFacts
	}
	files := make([]community.File, 0, len(facts))
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
		indexOf[fact.GetId()] = i
	}
	repositoryRoot := ""
	if repo, err := deps.Codeindex.Repository(ctx, repositoryID); err == nil && repo != nil {
		repositoryRoot = repo.GetRoot()
	}
	report("loading", len(facts), len(facts), "loaded")

	fileEdges, err := deps.Codeindex.AggregatedFileEdges(ctx, snapshotID)
	if err != nil {
		return nil, false, err
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

	report("grouping", 0, 0, "grouping dependencies")
	grouping, err := community.Build(files, communityEdges, deps.Options.Grouping)
	if err != nil {
		return nil, false, err
	}
	materialize.SortGroups(grouping.Groups)
	report("grouping", grouping.Metrics.RootGroups, grouping.Metrics.RootGroups, fmt.Sprintf(
		"%d components · modularity %.2f · %d isolated files",
		grouping.Metrics.RootGroups, grouping.Metrics.Modularity, grouping.Metrics.IsolatedFiles,
	))
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	repositoryName := repositoryID
	if base := filepath.Base(repositoryRoot); repositoryRoot != "" && base != "." && base != "/" {
		repositoryName = base
	}
	var imports []materialize.MapImport
	if req.IncludeImports {
		fileImports, err := deps.Codeindex.FileImports(ctx, snapshotID)
		if err != nil {
			return nil, false, err
		}
		imports = mapImports(fileImports)
	}

	runID := cgraph.ID(repositoryID, snapshotID, "community", configHash)
	report("materializing", 0, 0, "")
	mapResult, err := materialize.ApplyGroupMap(ctx, deps.Workspace, deps.Codeindex, materialize.GroupMapInput{
		RepositoryID:   repositoryID,
		RepositoryName: repositoryName,
		RepositoryRoot: repositoryRoot,
		SnapshotID:     snapshotID,
		Files:          files,
		Groups:         grouping.Groups,
		Edges:          mapEdges,
		Imports:        imports,
	}, deps.Options.MaterializeOptions(func(current, total int, detail string) {
		report("materializing", current, total, detail)
	}))
	if err != nil {
		return nil, false, err
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
	params := make(map[string]string, len(grouping.Params)+8)
	for key, value := range grouping.Params {
		params[key] = value
	}
	params["root_groups"] = strconv.Itoa(grouping.Metrics.RootGroups)
	params["total_groups"] = strconv.Itoa(grouping.Metrics.Groups)
	params["modularity"] = strconv.FormatFloat(grouping.Metrics.Modularity, 'g', -1, 64)
	params["cross_weight"] = strconv.FormatFloat(grouping.Metrics.CrossWeight, 'g', -1, 64)
	params["isolated_files"] = strconv.Itoa(grouping.Metrics.IsolatedFiles)
	params["folder_coverage"] = strconv.FormatFloat(grouping.Metrics.FolderCoverage, 'g', -1, 64)
	params["max_connectors_per_view"] = strconv.Itoa(deps.Options.MaxConnectorsPerView)
	params["max_leaf_connectors_per_view"] = strconv.Itoa(deps.Options.MaxLeafConnectorsPerView)
	if err := deps.Codeindex.SaveAnalysis(ctx, cstore.AnalysisRun{
		ID:           runID,
		RepositoryID: repositoryID,
		SnapshotID:   snapshotID,
		Algorithm:    "louvain",
		Params:       params,
		Groups:       analysisGroups(runID, grouping.Groups, files),
	}); err != nil {
		return nil, false, err
	}
	if err := deps.Codeindex.SaveCompletedMap(ctx, repositoryID, &codeindexv1.CompletedMap{
		Result:         result,
		IncludeImports: req.IncludeImports,
		ConfigHash:     configHash,
	}); err != nil {
		return nil, false, err
	}
	return result, false, nil
}

func mapImports(imports []cstore.FileImport) []materialize.MapImport {
	out := make([]materialize.MapImport, 0, len(imports))
	for _, item := range imports {
		out = append(out, materialize.MapImport{FileFactID: item.FileFactID, Import: item.Import})
	}
	return out
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
