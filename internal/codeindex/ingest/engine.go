// Package ingest shares snapshot preparation between the CLI watcher and APIs.
package ingest

import (
	"context"
	"fmt"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/gitstate"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

type Engine struct {
	Store              *cstore.Store
	Config             config.Config
	Root, RepositoryID string
	Exclude            []string
	Progress           indexer.ProgressFunc
}

func (e Engine) Base(ctx context.Context, id string) (*indexer.IncrementalBase, error) {
	if id == "" {
		return nil, nil
	}
	snap, err := e.Store.Snapshot(ctx, id)
	if err != nil {
		return nil, err
	}
	g, err := e.Store.LoadGraph(ctx, id)
	if err != nil {
		return nil, err
	}
	sources := map[string]string{}
	for _, src := range snap.Sources {
		sources[src.Path] = src.Hash
	}
	return &indexer.IncrementalBase{Snapshot: snap, Graph: g, Sources: sources}, nil
}

func (e Engine) Prepare(ctx context.Context, target *pb.ComparisonTarget) (*pb.Snapshot, error) {
	if target == nil {
		return nil, fmt.Errorf("comparison target is required")
	}
	count := 0
	if target.SnapshotId != "" {
		count++
	}
	if target.GitRevision != "" {
		count++
	}
	if target.WorkingTree {
		count++
	}
	if count != 1 {
		return nil, fmt.Errorf("select exactly one snapshot, commit, or working tree")
	}
	if target.SnapshotId != "" {
		snap, err := e.Store.Snapshot(ctx, target.SnapshotId)
		if err != nil {
			return nil, err
		}
		if snap.RepositoryId != e.RepositoryID {
			return nil, fmt.Errorf("snapshot belongs to another repository")
		}
		if snap.IngestionStatus != "complete" && snap.IngestionStatus != "" {
			return nil, fmt.Errorf("snapshot indexing is incomplete")
		}
		return snap, nil
	}
	revision, branch := target.GitRevision, target.GitBranch
	snapshots, err := e.Store.Snapshots(ctx, e.RepositoryID)
	if err != nil {
		return nil, err
	}
	req := &pb.IndexRequest{Exclude: e.Exclude}
	configHash := indexer.ConfigurationHash(e.Config, req)
	find := func(sha string) *pb.Snapshot {
		for i := len(snapshots) - 1; i >= 0; i-- {
			snap := snapshots[i]
			if snap.Provenance == "commit" && snap.GitRevision == sha && snap.ConfigHash == configHash && snap.IngestionStatus == "complete" && indexer.ToolchainCompatible(ctx, e.Config, e.Root, snap, nil) {
				return snap
			}
		}
		return nil
	}
	if !target.WorkingTree && branch == "" {
		if snap := find(revision); snap != nil {
			return snap, nil
		}
	}
	if !target.WorkingTree {
		revision, err = gitstate.Resolve(ctx, e.Root, revision)
		if err != nil {
			return nil, err
		}
		if branch == "" {
			ref, _ := gitstate.Run(ctx, e.Root, "rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", target.GitRevision)
			ref = strings.TrimSpace(ref)
			if strings.HasPrefix(ref, "refs/heads/") {
				branch = strings.TrimPrefix(ref, "refs/heads/")
			} else if strings.HasPrefix(ref, "refs/remotes/") {
				branch = strings.TrimPrefix(ref, "refs/remotes/")
			}
		}
		if snap := find(revision); snap != nil {
			return snap, nil
		}
	}
	var base *indexer.IncrementalBase
	for i := len(snapshots) - 1; i >= 0; i-- {
		if snapshots[i].ConfigHash == configHash && snapshots[i].IngestionStatus == "complete" {
			base, err = e.Base(ctx, snapshots[i].Id)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	var snapshot *pb.Snapshot
	build := func(directory string) error {
		pipeline := indexer.Pipeline{Config: e.Config, RepositoryID: e.RepositoryID}
		input := &pb.IndexRequest{Directory: directory, Exclude: e.Exclude, Incremental: base != nil}
		snap, g, reused, err := pipeline.BuildIncremental(ctx, input, e.Progress, base)
		if err != nil {
			return err
		}
		// A provenance transition must retain a distinct immutable commit record.
		if reused && !target.WorkingTree && snap.Provenance != "commit" {
			snap, g, err = pipeline.Build(ctx, input, e.Progress)
			if err != nil {
				return err
			}
			reused = false
		}
		if !target.WorkingTree {
			snap.Provenance = "commit"
			snap.GitRevision = revision
			snap.GitBranch = branch
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if !reused {
			if target.WorkingTree {
				err = e.Store.Publish(ctx, e.Root, snap, g)
			} else {
				err = e.Store.PublishHistorical(ctx, e.Root, snap, g)
			}
			if err != nil {
				return err
			}
		}
		snapshot = snap
		return nil
	}
	if target.WorkingTree {
		err = build(e.Root)
	} else {
		err = gitstate.WithCommit(ctx, e.Root, revision, build)
	}
	return snapshot, err
}
