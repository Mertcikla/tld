package impact

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	"github.com/mertcikla/codeindex/ingest"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// DefaultBlastRadius is the shared blast radius for CLI, RPC, and live impact.
// Zero includes direct changes only, matching an omitted proto blast_radius.
const DefaultBlastRadius = 0

// MaxBlastRadius bounds the dependency hops accepted by comparison callers.
const MaxBlastRadius = 2

// ValidateBlastRadius applies the same blast-radius limit on every surface.
func ValidateBlastRadius(radius uint32) error {
	if radius > MaxBlastRadius {
		return fmt.Errorf("blast radius must be between 0 and %d", MaxBlastRadius)
	}
	return nil
}

// DefaultMaxNodes is the size budget used when a client does not pick one. A
// diagram wider than this is progressively narrowed by blast radius.
const DefaultMaxNodes = 400

// Comparison targets reported on progress updates so a caller can attribute
// each stage run to the revision it came from.
const (
	TargetBase = "base"
	TargetHead = "head"
)

// Service builds repository comparisons and their transient scenes from a
// workspace and codeindex store. Server RPC handlers and the CLI share it so
// compare behavior cannot drift between surfaces.
type Service struct {
	Workspace core.Store
	Index     *cstore.Store
	Config    config.Config
}

// CompareRequest selects the two revisions and the amount of unchanged context
// to carry into the diagram.
type CompareRequest struct {
	RepositoryID string
	Base         *pb.Revision
	Head         *pb.Revision
	BlastRadius  uint32
	ViewFilter   string
	Progress     indexer.ProgressFunc
	// PrepareCheckout runs only when a revision needs indexing.
	PrepareCheckout func(context.Context, string) error
}

// Compare prepares both targets (indexing on demand), builds the impact diagram
// for their snapshot pair, persists it, and returns it.
func (s Service) Compare(ctx context.Context, req CompareRequest) (*model.Diagram, error) {
	if err := ValidateBlastRadius(req.BlastRadius); err != nil {
		return nil, err
	}
	repo, err := s.Index.Repository(ctx, req.RepositoryID)
	if err != nil {
		return nil, err
	}
	ctx, release, err := s.Index.AcquireLease(ctx, repo.Id)
	if err != nil {
		return nil, err
	}
	defer release()
	engine := ingest.Engine{
		Store:           s.Index,
		Config:          s.Config,
		Root:            repo.Root,
		RepositoryID:    repo.Id,
		PrepareCheckout: req.PrepareCheckout,
	}
	engine.Progress = forTarget(req.Progress, TargetBase)
	before, err := engine.Prepare(ctx, req.Base)
	if err != nil {
		return nil, err
	}
	engine.Progress = forTarget(req.Progress, TargetHead)
	after, err := engine.Prepare(ctx, req.Head)
	if err != nil {
		return nil, err
	}
	if req.Progress != nil {
		req.Progress(indexer.Progress{Stage: "overlay", Detail: "preparing change overlay"})
	}
	return s.CompareSnapshots(ctx, repo.Id, before.Id, after.Id, req.BlastRadius, req.ViewFilter)
}

// CompareSnapshots shares cached comparisons with callers that already prepared
// their snapshots, including the live watcher. The caller owns the index lease.
func (s Service) CompareSnapshots(ctx context.Context, repoID, beforeID, afterID string, radius uint32, viewFilters ...string) (*model.Diagram, error) {
	return s.compareSnapshots(ctx, repoID, beforeID, afterID, radius, nil, viewFilters...)
}

// CompareGraphs accepts the watch loop's retained immutable baseline and newly
// published graph. The persisted comparison and canonical scene are identical
// to CompareSnapshots; only the source-analysis reads are avoided.
func (s Service) CompareGraphs(ctx context.Context, before, after *pb.Snapshot, from, to *graph.Graph, radius uint32) (*model.Diagram, error) {
	diff, err := s.Index.ImpactDiffGraphs(ctx, before, after, from, to)
	if err != nil {
		return nil, err
	}
	oldEdges, oldKinds := cstore.GraphFilePairStats(from)
	newEdges, newKinds := cstore.GraphFilePairStats(to)
	inputs := &analysisInputs{diff: diff, before: before, after: after, oldEdges: oldEdges, oldKinds: oldKinds, newEdges: newEdges, newKinds: newKinds}
	return s.compareSnapshots(ctx, before.RepositoryId, before.Id, after.Id, radius, inputs)
}

func (s Service) compareSnapshots(ctx context.Context, repoID, beforeID, afterID string, radius uint32, inputs *analysisInputs, viewFilters ...string) (*model.Diagram, error) {
	if err := ValidateBlastRadius(radius); err != nil {
		return nil, err
	}
	workspace, err := s.Workspace.Explore(ctx)
	if err != nil {
		return nil, err
	}
	mappings, err := s.Index.MappingsByRepository(ctx, repoID)
	if err != nil {
		return nil, err
	}
	repositories, err := s.Index.ListRepositories(ctx)
	if err != nil {
		return nil, err
	}
	viewFilter := ""
	if len(viewFilters) > 0 {
		viewFilter = viewFilters[0]
	}
	raw, err := json.Marshal(struct {
		Workspace    core.ExploreData
		Mappings     []cstore.ResourceMapping
		ViewFilter   string
		Repositories []*pb.Repository
	}{workspace, mappings, viewFilter, repositories})
	if err != nil {
		return nil, err
	}
	fingerprint := graph.Hash(raw)
	key := ComparisonKey(beforeID, afterID, radius, fingerprint)
	// A completed comparison is immutable for its snapshot pair and radius,
	// so a repeat request is a single indexed lookup instead of a rebuild.
	if cached, err := s.Index.Impact(ctx, repoID, key); err == nil {
		if cached.Scene != nil && cached.Scene.SchemaVersion == PipelineVersion {
			return cached, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// Source analysis depends only on immutable snapshots and radius. Scene
	// assembly is cached separately against the captured workspace fingerprint.
	analysisKey := ComparisonKey(beforeID, afterID, radius, "source-analysis")
	analysis, err := s.Index.Impact(ctx, repoID, analysisKey)
	if errors.Is(err, sql.ErrNoRows) {
		analysis, err = buildAnalysis(ctx, s.Index, repoID, analysisKey, beforeID, afterID, radius, inputs)
		if err == nil {
			err = s.Index.SaveImpact(ctx, analysis)
		}
	}
	if err != nil {
		return nil, err
	}
	diagram := model.Clone(analysis)
	diagram.ComparisonKey, diagram.Version = key, uuid.NewString()
	diagram.WorkspaceFingerprint, diagram.ViewFilter = fingerprint, viewFilter
	repo, err := s.Index.Repository(ctx, repoID)
	if err != nil {
		return nil, err
	}
	capturedSnapshots := map[string]*pb.Snapshot{}
	if inputs != nil {
		capturedSnapshots[beforeID] = inputs.before
		capturedSnapshots[afterID] = inputs.after
	}
	diagram.Scene, err = assemble(ctx, s, diagram, workspace, mappings, repo.Root, repositories, capturedSnapshots)
	if err != nil {
		return nil, err
	}
	if err := s.Index.SaveImpact(ctx, diagram); err != nil {
		return nil, err
	}
	return diagram, nil
}

// ComparisonKey identifies a stored comparison payload. Snapshot IDs already
// pin the exact content, config, and toolchain of both sides, so a completed
// comparison is immutable and safe to serve again. The blast radius is part of
// the key because diagrams built at different radii carry different context.
func ComparisonKey(beforeID, afterID string, radius uint32, fingerprints ...string) string {
	parts := []string{beforeID, afterID, fmt.Sprintf("radius=%d", radius), "pipeline=" + PipelineVersion}
	parts = append(parts, fingerprints...)
	return graph.ID(parts...)
}

// forTarget tags every progress update with the comparison side being prepared
// so callers can label the base and head scans separately.
func forTarget(progress indexer.ProgressFunc, target string) indexer.ProgressFunc {
	if progress == nil {
		return nil
	}
	return func(update indexer.Progress) {
		update.Target = target
		progress(update)
	}
}
