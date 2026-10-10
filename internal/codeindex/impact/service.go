package impact

import (
	"context"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/indexer"
	"github.com/mertcikla/codeindex/ingest"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// DefaultContextDepth is the number of dependency hops of unchanged context
// every comparison computes and displays.
const DefaultContextDepth = 3

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
	ContextDepth uint32
	Progress     indexer.ProgressFunc
	// PrepareCheckout runs only when a revision needs indexing.
	PrepareCheckout func(context.Context, string) error
}

// Compare prepares both targets (indexing on demand), builds the impact diagram
// for their snapshot pair, persists it, and returns it.
func (s Service) Compare(ctx context.Context, req CompareRequest) (*pb.ImpactDiagram, error) {
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
	key := graph.ID(before.Id, after.Id)
	return Save(ctx, s.Workspace, s.Index, repo.Id, key, before.Id, after.Id, req.ContextDepth)
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
