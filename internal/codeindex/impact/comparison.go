package impact

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

// captureComparison runs once, against pinned commit objects. Export and cache
// reads never consult the checkout to fill historical evidence.
func captureComparison(ctx context.Context, idx *cstore.Store, diagram *model.Diagram, captured ...*pb.Snapshot) error {
	diff := diagram.GetDiff()
	load := func(id string, position int) (*pb.Snapshot, error) {
		if len(captured) > position && captured[position] != nil && captured[position].Id == id {
			return captured[position], nil
		}
		return idx.Snapshot(ctx, id)
	}
	before, err := load(diff.GetFromSnapshotId(), 0)
	if err != nil {
		return err
	}
	after, err := load(diff.GetToSnapshotId(), 1)
	if err != nil {
		return err
	}
	repo, err := idx.Repository(ctx, diagram.RepositoryId)
	if err != nil {
		return err
	}
	comparison := &pb.ImpactComparison{RepositoryId: repo.Id, RepositoryName: repo.Name,
		Base: before.GitRevision, Head: after.GitRevision, SchemaVersion: PipelineVersion,
		GitStatus: "not_applicable"}
	diagram.Comparison = comparison
	availability := func(snapshot *pb.Snapshot, path string) string {
		for _, source := range snapshot.Sources {
			if source.Path == path {
				return "captured"
			}
		}
		for _, file := range snapshot.GetManifest().GetFiles() {
			if file.Path == path {
				return "manifest_only"
			}
		}
		if snapshot.GetManifest().GetComplete() {
			return "absent"
		}
		return "unknown"
	}
	for _, change := range diff.GetSources() {
		file := &pb.ImpactFile{Path: change.Path, Change: change.Change,
			LinesAdded: change.LinesAdded, LinesRemoved: change.LinesRemoved, LineCountsBasis: "captured_source",
			BaseAnalysis: availability(before, change.Path), HeadAnalysis: availability(after, change.Path), MetadataOnly: change.MetadataOnly, BaseHash: change.FromHash, HeadHash: change.ToHash}
		if change.FactsUnavailable {
			file.AnalysisUnavailableReason = "Source contents were not captured; extraction/dependency evidence is unavailable"
		}
		diagram.Files = append(diagram.Files, file)
	}
	if before.Provenance != "commit" || after.Provenance != "commit" || before.GitRevision == "" || after.GitRevision == "" {
		comparison.GitUnavailableReason = "Targets are captured snapshots, not two pinned commits"
		return nil
	}
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo.Root}, args...)...)
		return cmd.Output()
	}
	raw, err := run("-c", "diff.algorithm=myers", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--numstat", "-z", comparison.Base, comparison.Head, "--")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		comparison.GitStatus, comparison.GitUnavailableReason = "unavailable", "Pinned Git objects are unavailable; statistics use captured sources where possible"
		return nil
	}
	stats := map[string]*pb.ImpactFile{}
	binary := map[string]bool{}
	for _, record := range strings.Split(string(raw), "\x00") {
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\t", 3)
		if len(parts) != 3 {
			return fmt.Errorf("invalid Git numstat record")
		}
		stat := &pb.ImpactFile{LineCountsBasis: "net_tree"}
		if parts[0] == "-" {
			binary[parts[2]] = true
		} else {
			added, addErr := strconv.ParseUint(parts[0], 10, 32)
			removed, removeErr := strconv.ParseUint(parts[1], 10, 32)
			if addErr != nil || removeErr != nil {
				return fmt.Errorf("invalid Git line counts for %s", parts[2])
			}
			a, r := uint32(added), uint32(removed)
			stat.LinesAdded, stat.LinesRemoved = &a, &r
		}
		stats[parts[2]] = stat
	}
	for _, file := range diagram.Files {
		if stat := stats[file.Path]; stat != nil {
			file.LinesAdded, file.LinesRemoved, file.LineCountsBasis, file.Binary = stat.LinesAdded, stat.LinesRemoved, stat.LineCountsBasis, binary[file.Path]
		}
	}
	comparison.GitStatus = "available"
	raw, err = run("log", "--topo-order", "--format=%H%x00%s%x00%an%x00%at", comparison.Base+".."+comparison.Head, "--")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		comparison.GitStatus, comparison.GitUnavailableReason = "partial", "Git statistics captured, but commit history is unavailable"
		return nil
	}
	for _, record := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		parts := strings.Split(record, "\x00")
		if len(parts) != 4 {
			continue
		}
		stamp, parseErr := strconv.ParseInt(parts[3], 10, 64)
		if parseErr != nil {
			return parseErr
		}
		comparison.Commits = append(comparison.Commits, &pb.ImpactCommit{Sha: parts[0], Subject: parts[1], Author: parts[2], CreatedUnix: stamp})
	}
	return nil
}
