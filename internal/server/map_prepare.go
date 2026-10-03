package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/embed"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
)

func (s *mapperService) prepareSnapshot(ctx context.Context, req *pb.MapRepositoryRequest, send func(*pb.MapProgress)) (*pb.Snapshot, error) {
	repo, err := s.idx.Repository(ctx, req.RepositoryId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if req.SnapshotId != "" || (req.GitRevision == "" && !req.WorkingTree) {
		id := req.SnapshotId
		if id == "" {
			id = repo.LatestSnapshotId
		}
		if id == "" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("repository has no snapshot; select a commit or Working tree"))
		}
		snapshot, err := s.idx.Snapshot(ctx, id)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if snapshot.RepositoryId != repo.Id {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot belongs to another repository"))
		}
		return snapshot, nil
	}
	cfg := configbridge.FromGlobal(s.config)
	configHash := indexer.ConfigurationHash(cfg, &pb.IndexRequest{})
	snapshots, err := s.idx.Snapshots(ctx, repo.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	find := func(revision, fingerprint, provenance string) *pb.Snapshot {
		for i := len(snapshots) - 1; i >= 0; i-- {
			snap := snapshots[i]
			if snap.ConfigHash == configHash && snap.Provenance == provenance && snap.IngestionStatus == "complete" &&
				((provenance == "commit" && snap.GitRevision == revision) || (provenance == "working_tree" && snap.ContentFingerprint == fingerprint)) {
				return snap
			}
		}
		return nil
	}
	// A full recorded SHA can be reused without its original checkout.
	if !req.WorkingTree {
		if snap := find(req.GitRevision, "", "commit"); snap != nil {
			return snap, nil
		}
	}
	send(&pb.MapProgress{Stage: "preparing", Detail: "resolving snapshot inputs"})
	directory := repo.Root
	revision := ""
	capturedBranch := req.GitBranch
	if req.WorkingTree {
		fingerprint, _, _, _, err := indexer.CaptureInputs(ctx, directory, cfg, nil)
		if err != nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("read working tree: %w", err))
		}
		if snap := find("", fingerprint, "working_tree"); snap != nil {
			return snap, nil
		}
	} else {
		revision, err = resolveRevision(ctx, repo.Root, req.GitRevision)
		if err != nil {
			return nil, err
		}
		if snap := find(revision, "", "commit"); snap != nil {
			return snap, nil
		}
		if capturedBranch == "" {
			ref, _ := repositoryGit(ctx, repo.Root, "rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", req.GitRevision)
			ref = strings.TrimSpace(ref)
			if strings.HasPrefix(ref, "refs/heads/") {
				capturedBranch = strings.TrimPrefix(ref, "refs/heads/")
			} else if strings.HasPrefix(ref, "refs/remotes/") {
				capturedBranch = strings.TrimPrefix(ref, "refs/remotes/")
			}
		}
		if req.GitBranch != "" {
			branchSHA, err := resolveRevision(ctx, repo.Root, req.GitBranch)
			if err != nil {
				return nil, err
			}
			if _, err := repositoryGit(ctx, repo.Root, "merge-base", "--is-ancestor", revision, branchSHA); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("selected commit is not on branch %q", req.GitBranch))
			}
		}
	}
	build := func(root string) (*pb.Snapshot, error) {
		pipeline := indexer.Pipeline{Config: cfg, RepositoryID: repo.Id}
		snapshot, g, err := pipeline.Build(ctx, &pb.IndexRequest{Directory: root}, func(p indexer.Progress) {
			send(&pb.MapProgress{Stage: "indexing", Current: uint32(p.Current), Total: uint32(p.Total), Detail: strings.TrimSpace(p.Stage + " " + p.Detail)})
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("index snapshot: %w", err))
		}
		if req.WorkingTree {
			snapshot.Provenance = "working_tree"
		} else {
			snapshot.Provenance = "commit"
			snapshot.GitRevision = revision
			snapshot.GitBranch = capturedBranch
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if req.WorkingTree {
			err = s.idx.Publish(ctx, repo.Root, snapshot, g)
		} else {
			err = s.idx.PublishHistorical(ctx, repo.Root, snapshot, g)
		}
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return snapshot, nil
	}
	if req.WorkingTree {
		return build(directory)
	}
	return inDetachedWorktree(ctx, repo.Root, revision, build)
}

func inDetachedWorktree(ctx context.Context, root, revision string, run func(string) (*pb.Snapshot, error)) (snapshot *pb.Snapshot, err error) {
	parent, err := os.MkdirTemp("", "tld-map-")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(parent)) }()
	checkout := filepath.Join(parent, "checkout")
	added := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		// Git can register a worktree before checkout is canceled or fails.
		if !added {
			registered, listErr := repositoryGit(cleanupCtx, root, "worktree", "list", "--porcelain")
			if listErr != nil {
				err = errors.Join(err, fmt.Errorf("inspect temporary worktree registration: %w", listErr))
				return
			}
			if !strings.Contains(registered, "worktree "+checkout+"\n") {
				return
			}
		}
		_, cleanupErr := repositoryGit(cleanupCtx, root, "worktree", "remove", "--force", checkout)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("clean temporary worktree: %w", cleanupErr))
		}
	}()
	if _, err = repositoryGit(ctx, root, "worktree", "add", "--detach", checkout, revision); err != nil {
		return nil, err
	}
	added = true
	return run(checkout)
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
