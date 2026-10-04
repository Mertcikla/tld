package server

import (
	"context"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/gitstate"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/ingest"
)

func (s *mapperService) prepareSnapshot(ctx context.Context, req *pb.MapRepositoryRequest, send func(*pb.MapProgress)) (*pb.Snapshot, error) {
	repo, err := s.idx.Repository(ctx, req.RepositoryId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	target := &pb.ComparisonTarget{SnapshotId: req.SnapshotId, GitRevision: req.GitRevision, WorkingTree: req.WorkingTree, GitBranch: req.GitBranch}
	if target.SnapshotId == "" && target.GitRevision == "" && !target.WorkingTree {
		target.SnapshotId = repo.LatestSnapshotId
	}
	engine := ingest.Engine{Store: s.idx, Config: configbridge.FromGlobal(s.config), Root: repo.Root, RepositoryID: repo.Id, Progress: func(p indexer.Progress) {
		send(&pb.MapProgress{Stage: "indexing", Current: uint32(p.Current), Total: uint32(p.Total), Detail: strings.TrimSpace(p.Stage + " " + p.Detail)})
	}}
	snapshot, err := engine.Prepare(ctx, target)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return snapshot, nil
}

func inDetachedWorktree(ctx context.Context, root, revision string, run func(string) (*pb.Snapshot, error)) (snapshot *pb.Snapshot, err error) {
	err = gitstate.WithCommit(ctx, root, revision, func(checkout string) error {
		var e error
		snapshot, e = run(checkout)
		return e
	})
	return snapshot, err
}
