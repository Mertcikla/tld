package impact

import (
	"context"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/ingest"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// DefaultContextDepth is the maximum number of dependency hops of unchanged
// context a comparison computes. It is the upper bound of the UI blast-radius
// slider; display scoping itself happens client-side.
const DefaultContextDepth = 3

// DefaultMaxNodes is the size budget used when a client does not pick one. A
// diagram wider than this is progressively narrowed by blast radius.
const DefaultMaxNodes = 400

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
	Base         *pb.ComparisonTarget
	Head         *pb.ComparisonTarget
	ContextDepth uint32
	Progress     indexer.ProgressFunc
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
		Store:        s.Index,
		Config:       s.Config,
		Root:         repo.Root,
		RepositoryID: repo.Id,
		Progress:     req.Progress,
	}
	before, err := engine.Prepare(ctx, req.Base)
	if err != nil {
		return nil, err
	}
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
