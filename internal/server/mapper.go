package server

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mapper"
	"github.com/mertcikla/tld/v2/internal/store"
)

// mapperService runs the deterministic embedding clustering/binning pipeline and
// materializes its folder/bin/cluster hierarchy into the workspace.
type mapperService struct {
	codeindexv1connect.UnimplementedMapperServiceHandler
	ws  *store.SQLiteStore
	idx *cstore.Store

	mu      sync.Mutex
	running map[string]struct{}
}

func registerMapperHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore) {
	svc := &mapperService{
		ws:      sqliteStore,
		idx:     cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect()),
		running: map[string]struct{}{},
	}
	path, handler := codeindexv1connect.NewMapperServiceHandler(svc)
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
}

func (s *mapperService) MapRepository(ctx context.Context, req *connect.Request[codeindexv1.MapRepositoryRequest], stream *connect.ServerStream[codeindexv1.MapRepositoryEvent]) error {
	repositoryID := strings.TrimSpace(req.Msg.GetRepositoryId())
	if repositoryID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	if !s.begin(repositoryID) {
		return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a map is already running for this repository"))
	}
	defer s.end(repositoryID)

	send := func(progress *codeindexv1.MapProgress) {
		_ = stream.Send(&codeindexv1.MapRepositoryEvent{Event: &codeindexv1.MapRepositoryEvent_Progress{Progress: progress}})
	}

	snapshotID := strings.TrimSpace(req.Msg.GetSnapshotId())
	if snapshotID == "" {
		latest, err := s.idx.Latest(ctx, repositoryID)
		if err != nil {
			return connect.NewError(connect.CodeInternal, err)
		}
		if latest == "" {
			return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("repository has no published snapshot"))
		}
		snapshotID = latest
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

	options := mapper.DefaultOptions()
	pipeline, err := mapper.RunPipelineProgress(dataset.Vectors, &options, func(progress mapper.Progress) {
		send(&codeindexv1.MapProgress{Stage: "clustering", Current: uint32(progress.Current), Total: uint32(progress.Total), Detail: progress.Detail})
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	send(&codeindexv1.MapProgress{Stage: "binning"})
	binOptions := mapper.DefaultBinOptions()
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
	runID := cgraph.ID(repositoryID, snapshotID, "mapper")
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

	send(&codeindexv1.MapProgress{Stage: "materializing"})
	mapResult, err := materialize.ApplyMap(ctx, s.ws, s.idx, materialize.MapInput{
		RepositoryID:   repositoryID,
		RepositoryName: repositoryName,
		RepositoryRoot: repositoryRoot,
		SnapshotID:     snapshotID,
		RunID:          runID,
		Dataset:        dataset,
		Bins:           bins,
	}, materialize.MapOptions{
		Progress: func(current, total int, detail string) {
			send(&codeindexv1.MapProgress{Stage: "materializing", Current: uint32(current), Total: uint32(total), Detail: detail})
		},
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	return stream.Send(&codeindexv1.MapRepositoryEvent{Event: &codeindexv1.MapRepositoryEvent_Result{Result: &codeindexv1.MapResult{
		RunId:             runID,
		ViewId:            mapResult.ViewID,
		Facts:             uint32(len(dataset.Facts)),
		Clusters:          uint32(len(pipeline.Domains)),
		Bins:              uint32(len(bins.Sizes)),
		Unclustered:       uint32(len(pipeline.Leftovers)),
		WeightedTightness: pipeline.Metrics.WeightedTightness,
	}}})
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
