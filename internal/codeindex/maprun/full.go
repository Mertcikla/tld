package maprun

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
)

func runFull(ctx context.Context, deps Deps, req Request, snapshot *pb.Snapshot, progress ProgressFunc) (*pb.MapResult, bool, error) {
	const configHash = "full-files-v6"
	active, err := deps.Codeindex.ActiveMap(ctx, req.RepositoryID)
	if err != nil {
		return nil, false, err
	}
	if active != nil && active.ConfigHash == configHash && active.Result.SnapshotId == req.SnapshotID {
		if _, err := deps.Workspace.ViewByID(ctx, active.Result.ViewId); err == nil {
			return active.Result, true, nil
		}
	}
	facts, err := deps.Codeindex.AllFacts(ctx, req.SnapshotID, pb.FactKind_FACT_KIND_FILE)
	if err != nil {
		return nil, false, err
	}
	if len(facts) == 0 {
		return nil, false, ErrNoFileFacts
	}
	files, _, _ := buildFileInputs(facts, nil, nil)
	byPath := map[string]string{}
	for i, f := range facts {
		byPath[f.GetAnchor().GetPath()] = files[i].ID
	}
	// Keep declaration anchors while rolling relationships up to files. The
	// materializer unions calls/references and all participating symbols.
	relationships, err := deps.Codeindex.FileSymbolEdges(ctx, req.SnapshotID)
	if err != nil {
		return nil, false, err
	}
	edges := make([]materialize.MapEdge, 0, len(relationships))
	for _, edge := range relationships {
		from, to := byPath[edge.FromPath], byPath[edge.ToPath]
		if from == "" || to == "" {
			continue
		}
		label := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(edge.Kind.String(), "EDGE_KIND_"), "_", " "))
		edges = append(edges, materialize.MapEdge{FromFactID: from, ToFactID: to, Kind: label, SourceLines: []uint32{edge.SourceLine}, TargetLines: []uint32{edge.TargetLine}})
	}
	repo, err := deps.Codeindex.Repository(ctx, req.RepositoryID)
	if err != nil {
		return nil, false, err
	}
	remote, _, err := deps.Codeindex.RepositoryOrigin(ctx, req.RepositoryID)
	if err != nil {
		return nil, false, err
	}
	name := repositoryRemoteName(remote)
	if name == "" {
		name = filepath.Base(repo.GetRoot())
	}
	namespace, err := deps.Codeindex.MappingNamespace(ctx, req.RepositoryID)
	if err != nil {
		return nil, false, err
	}
	id, err := deps.Codeindex.AllocateIDs(ctx, 1)
	if err != nil {
		return nil, false, err
	}
	if err := deps.Codeindex.InvalidateActiveMap(ctx, req.RepositoryID); err != nil {
		return nil, false, err
	}
	result, err := materialize.ApplyGroupMap(ctx, deps.Workspace, deps.Codeindex, materialize.GroupMapInput{
		RepositoryID: req.RepositoryID, MappingNamespace: namespace, RepositoryName: name, RepositoryRoot: repo.GetRoot(), RepositoryRemoteURL: remote, RepositoryBranch: snapshot.GitBranch, SnapshotID: req.SnapshotID, Files: files, Full: true, Edges: edges,
	}, materialize.MapOptions{Progress: func(current, total int, detail string) {
		if progress != nil {
			progress("materializing", current, total, detail)
		}
	}})
	if err != nil {
		return nil, false, err
	}
	out := &pb.MapResult{RunId: strconv.FormatInt(id, 10), SnapshotId: req.SnapshotID, ViewId: result.ViewID, Facts: uint32(len(facts))}
	if err := deps.Codeindex.SaveCompletedMap(ctx, req.RepositoryID, &pb.CompletedMap{Result: out, ConfigHash: configHash}); err != nil {
		return nil, false, err
	}
	return out, false, nil
}
