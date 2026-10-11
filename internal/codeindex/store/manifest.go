package store

import (
	"context"
	"encoding/json"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/gitstate"
)

// EnsureManifest can enrich old commit snapshots from immutable Git trees.
// An old working-tree snapshot cannot be reconstructed from today's checkout.
func (s *Store) EnsureManifest(ctx context.Context, snap *pb.Snapshot) error {
	if snap.GetManifest().GetComplete() || snap.GetProvenance() != "commit" || snap.GetGitRevision() == "" {
		return nil
	}
	repo, err := s.Repository(ctx, snap.RepositoryId)
	if err != nil {
		return err
	}
	var manifest *pb.SnapshotManifest
	err = gitstate.WithCommit(ctx, repo.Root, snap.GitRevision, func(root string) error {
		var captureErr error
		manifest, captureErr = gitstate.Manifest(ctx, root)
		return captureErr
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil // Legacy/offline snapshots retain an explicit incomplete status.
	}
	if !manifest.GetComplete() {
		return nil
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	where, args := scope(ctx).clause("org_id")
	if _, err := s.bun.NewRaw(`UPDATE codeindex_snapshots SET manifest_json = ? WHERE id = ?`+where, append([]any{string(raw), snap.Id}, args...)...).Exec(ctx); err != nil {
		return err
	}
	snap.Manifest = manifest
	return nil
}

func diffManifest(from, to *pb.Snapshot, fromSources, toSources map[string]string) []*pb.SourceChange {
	old, next := map[string]*pb.SourceFile{}, map[string]*pb.SourceFile{}
	paths := map[string]bool{}
	for _, file := range from.Manifest.Files {
		old[file.Path] = file
		paths[file.Path] = true
	}
	for _, file := range to.Manifest.Files {
		next[file.Path] = file
		paths[file.Path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	out := make([]*pb.SourceChange, 0)
	for _, path := range ordered {
		before, after := old[path], next[path]
		change := &pb.SourceChange{Path: path}
		switch {
		case before == nil:
			change.Change = pb.ChangeKind_CHANGE_KIND_ADDED
		case after == nil:
			change.Change = pb.ChangeKind_CHANGE_KIND_REMOVED
		case before.Hash != after.Hash || before.Mode != after.Mode:
			change.Change = pb.ChangeKind_CHANGE_KIND_MODIFIED
			change.MetadataOnly = before.Hash == after.Hash
		default:
			continue
		}
		change.FromHash, change.ToHash = fromSources[path], toSources[path]
		change.FactsUnavailable = (before != nil && change.FromHash == "") || (after != nil && change.ToHash == "")
		out = append(out, change)
	}
	return out
}
