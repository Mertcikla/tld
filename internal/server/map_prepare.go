package server

import (
	"context"
	"fmt"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/embed"
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

func (s *mapperService) ensureEmbeddings(ctx context.Context, snapshot *pb.Snapshot, send func(*pb.MapProgress)) error {
	profile, err := s.idx.MajorityProfile(ctx, snapshot.Id)
	if err != nil {
		return err
	}
	if profile != "" && snapshot.EmbeddingStatus == "complete" {
		return nil
	}
	// Existing indexed snapshots may have vectors produced outside the current embedding configuration.
	if profile != "" && snapshot.EmbeddingStatus == "" {
		return nil
	}
	client := embed.Client{Config: configbridge.FromGlobal(s.config), Store: s.idx, Progress: func(current, total int, detail string) {
		send(&pb.MapProgress{Stage: "embedding", Current: uint32(current), Total: uint32(total), Detail: detail})
	}}
	if !client.Enabled() {
		if profile != "" {
			return nil
		}
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("snapshot has no embeddings; configure index.embedding.endpoint and start the embedding service"))
	}
	if err := client.Embed(ctx, snapshot); err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("embedding failed; the indexed snapshot is saved and can be retried: %w", err))
	}
	return nil
}
