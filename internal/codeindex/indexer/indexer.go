package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

type Pipeline struct{ Config config.Config }

// Progress reports indexing progress. Stage names the active phase; Current and
// Total are per-stage counters (zero when unknown); Detail is an optional item
// such as the file or project currently being processed.
type Progress struct {
	Stage   string
	Current int64
	Total   int64
	Detail  string
}

// ProgressFunc receives progress updates. It must be safe to call frequently.
type ProgressFunc func(Progress)

func emitProgress(progress ProgressFunc, p Progress) {
	if progress != nil {
		progress(p)
	}
}

// IncrementalBase is a previously published snapshot made available to an
// incremental build. Reuse is file-granular: facts, chunks, and edges for
// sources whose content hash is unchanged are carried forward, and only changed
// projects re-run their indexer.
type IncrementalBase struct {
	Graph    *graph.Graph
	Snapshot *pb.Snapshot
	Sources  map[string]string
}

func (p Pipeline) Build(ctx context.Context, req *pb.IndexRequest, progress ProgressFunc) (*pb.Snapshot, *graph.Graph, error) {
	snap, g, _, err := p.build(ctx, req, progress, nil)
	return snap, g, err
}

// BuildIncremental builds a snapshot reusing base where possible. reused is true
// when nothing changed and the base snapshot was returned unchanged, in which
// case the caller should not republish.
func (p Pipeline) BuildIncremental(ctx context.Context, req *pb.IndexRequest, progress ProgressFunc, base *IncrementalBase) (*pb.Snapshot, *graph.Graph, bool, error) {
	return p.build(ctx, req, progress, base)
}

func (p Pipeline) build(ctx context.Context, req *pb.IndexRequest, progress ProgressFunc, base *IncrementalBase) (*pb.Snapshot, *graph.Graph, bool, error) {
	root, err := filepath.Abs(req.Directory)
	if err != nil {
		return nil, nil, false, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, false, err
	}
	emitProgress(progress, Progress{Stage: "discover"})
	projects, sources, err := Discover(ctx, root, req.ProjectRoots, req.Exclude)
	if err != nil {
		return nil, nil, false, err
	}
	repo := graph.RepositoryID(root)
	snapshot := graph.ID(repo, fmt.Sprint(time.Now().UnixNano()))
	g := graph.NewGraph(repo, snapshot)
	g.Sources = sources

	// Determine which current sources are unchanged relative to the base.
	kept := map[string]bool{}
	if base != nil {
		for path, src := range sources {
			if hash, ok := base.Sources[path]; ok && hash == src.Hash {
				kept[path] = true
			}
		}
	}
	if base != nil && len(kept) == len(sources) && !anyBaseSourceRemoved(sources, base.Sources) {
		// Nothing changed; reuse the published snapshot verbatim.
		return base.Snapshot, base.Graph, true, nil
	}

	// Carry forward facts and chunks for unchanged sources.
	carried := map[string]string{}
	type parentLink struct{ factID, parentID string }
	parentLinks := make([]parentLink, 0)
	if base != nil {
		for _, f := range base.Graph.Facts {
			if f.Anchor == nil || !kept[f.Anchor.Path] {
				continue
			}
			nf := g.AdoptFact(f)
			if nf == nil {
				continue
			}
			carried[f.Id] = nf.Id
			if f.ParentFactId != "" {
				parentLinks = append(parentLinks, parentLink{nf.Id, f.ParentFactId})
			}
		}
		for _, link := range parentLinks {
			if parentID, ok := carried[link.parentID]; ok {
				g.Facts[link.factID].ParentFactId = parentID
			}
		}
		for _, c := range base.Graph.Chunks {
			if factID, ok := carried[c.FactId]; ok {
				g.AdoptChunk(c, factID)
			}
		}
	}

	snap := &pb.Snapshot{Id: snapshot, RepositoryId: repo, CreatedUnix: time.Now().Unix(), Projects: projects, IngestionStatus: "staging", EmbeddingStatus: "disabled"}
	configBytes, _ := json.Marshal(p.Config)
	snap.ConfigHash = graph.Hash(configBytes)
	snap.ToolVersions = map[string]string{
		"gotreesitter": "0.15.2",
	}
	if base != nil && base.Snapshot != nil {
		for name, version := range base.Snapshot.ToolVersions {
			snap.ToolVersions[name] = version
		}
	}
	for _, s := range sources {
		snap.Sources = append(snap.Sources, &pb.SourceFile{Path: s.Path, Hash: s.Hash, Size: uint64(len(s.Text))})
	}
	sort.Slice(snap.Sources, func(i, j int) bool { return snap.Sources[i].Path < snap.Sources[j].Path })
	if b, e := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output(); e == nil {
		snap.GitRevision = strings.TrimSpace(string(b))
	}
	if b, e := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output(); e == nil {
		if branch := strings.TrimSpace(string(b)); branch != "" && branch != "HEAD" {
			snap.GitBranch = branch
		}
	}
	var syntaxSources []*graph.Source
	for _, f := range snap.Sources {
		if kept[f.Path] {
			continue
		}
		src := sources[f.Path]
		if src == nil || !isSyntaxFamily(languageFamily(src.Language)) {
			continue
		}
		syntaxSources = append(syntaxSources, src)
	}
	emitProgress(progress, Progress{Stage: "tree-sitter", Total: int64(len(syntaxSources))})
	var calls []callSite
	for i, src := range syntaxSources {
		emitProgress(progress, Progress{Stage: "tree-sitter", Current: int64(i), Total: int64(len(syntaxSources)), Detail: src.Path})
		sites, err := treeFacts(ctx, g, src)
		if err != nil {
			return nil, nil, false, err
		}
		calls = append(calls, sites...)
	}
	emitProgress(progress, Progress{Stage: "tree-sitter", Current: int64(len(syntaxSources)), Total: int64(len(syntaxSources))})
	tmp, err := os.MkdirTemp("", "codeindex-scip-")
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	table := newSymbols()
	// Seed definitions from carried facts so references in changed files resolve
	// to facts retained from the base snapshot.
	for _, factID := range carried {
		fact := g.Facts[factID]
		if fact != nil && fact.SymbolKey != "" {
			table.definitions[fact.SymbolKey] = factID
			table.definitionSites[fact.SymbolKey] = fact.Anchor
		}
	}
	totalProjects := 0
	for _, pr := range projects {
		if base != nil && !projectHasChange(pr, sources, base.Sources, kept) {
			continue
		}
		totalProjects++
	}
	emitProgress(progress, Progress{Stage: "scip", Total: int64(totalProjects)})
	projectIndex := 0
	for i, pr := range projects {
		if base != nil && !projectHasChange(pr, sources, base.Sources, kept) {
			continue
		}
		emitProgress(progress, Progress{Stage: "scip", Current: int64(projectIndex), Total: int64(totalProjects), Detail: pr.Root})
		projectIndex++
		family := languageFamily(pr.Language)
		scipBacked := !isSyntaxFamily(family)
		projectDir := filepath.Join(root, filepath.FromSlash(pr.Root))
		artifact := req.ScipArtifacts[pr.Root]
		strict := artifact != ""
		hashes := map[string]string{}
		preExisting := false
		if artifact == "" {
			spec, err := indexerForFamily(family, p.Config)
			if err != nil {
				return nil, nil, false, err
			}
			c := indexerContext{root: root, projectDir: projectDir, artifact: filepath.Join(tmp, fmt.Sprintf("%d.scip", i)), name: filepath.Base(projectDir), configPath: pr.ConfigPath}
			tool := spec.executable(p.Config, c)
			if _, err := exec.LookPath(tool); err != nil {
				return nil, nil, false, fmt.Errorf("%s unavailable: %w", tool, err)
			}
			snap.ToolVersions[spec.name] = toolVersion(ctx, tool, spec.versionArgs)
			argv, explicit, err := spec.args(p.Config, c)
			if err != nil {
				return nil, nil, false, err
			}
			toolCtx, cancel := context.WithTimeout(ctx, p.Config.ToolTimeout())
			cmd := exec.CommandContext(toolCtx, tool, argv...)
			cmd.Dir = projectDir
			out, e := cmd.CombinedOutput()
			cancel()
			if e != nil {
				return nil, nil, false, fmt.Errorf("%s failed in %s: %w: %s", spec.name, pr.Root, e, strings.TrimSpace(string(out)))
			}
			if explicit {
				artifact = c.artifact
			} else {
				artifact = filepath.Join(projectDir, "index.scip")
				if _, statErr := os.Stat(artifact); statErr == nil {
					preExisting = true
				}
			}
		}
		prefix := strings.Trim(pr.Root, "/") + "/"
		for path, src := range sources {
			if pr.Root == "." {
				hashes[path] = src.Hash
			} else if strings.HasPrefix(path, prefix) {
				hashes[strings.TrimPrefix(path, prefix)] = src.Hash
			}
		}
		if strict {
			hashes, err = verifySCIPManifest(artifact, pr.Root, sources)
			if err != nil {
				return nil, nil, false, err
			}
		}
		if base != nil {
			table.skip = kept
		}
		if err := importSCIP(ctx, g, pr, artifact, hashes, strict, scipBacked, table); err != nil {
			table.skip = nil
			return nil, nil, false, fmt.Errorf("import %s: %w", artifact, err)
		}
		table.skip = nil
		if !strict && !preExisting && artifact != "" && strings.HasPrefix(artifact, projectDir+string(filepath.Separator)) {
			_ = os.Remove(artifact)
		}
	}
	emitProgress(progress, Progress{Stage: "relationships"})
	table.apply(g)
	deriveCalls(g, calls, table)
	emitProgress(progress, Progress{Stage: "infra"})
	if e := addInfraFacts(ctx, g, root, base, carried); e != nil {
		return nil, nil, false, e
	}
	if base != nil {
		// Carry forward edges whose source fact was retained, remapping both
		// endpoints to the new snapshot by stable logical identity.
		logicalNew := g.FactsByLogicalKey()
		for _, edge := range base.Graph.EdgeFacts {
			from, ok := carried[edge.FromFactId]
			if !ok {
				continue
			}
			to := ""
			targetKey := edge.TargetSymbolKey
			if edge.ToFactId != "" {
				if bf := base.Graph.Facts[edge.ToFactId]; bf != nil {
					if nf := logicalNew[bf.LogicalKey]; nf != nil {
						to = nf.Id
						targetKey = ""
					}
				}
			}
			g.AdoptEdgeFact(edge, from, to, targetKey)
		}
	}
	emitProgress(progress, Progress{Stage: "verify"})
	for _, f := range snap.Sources {
		b, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if e != nil || graph.Hash(b) != f.Hash {
			return nil, nil, false, fmt.Errorf("source changed during indexing: %s", f.Path)
		}
	}
	snap.IngestionStatus = "complete"
	return snap, g, false, nil
}

// anyBaseSourceRemoved reports whether the base snapshot indexed a source that
// the current working tree no longer contains.
func anyBaseSourceRemoved(sources map[string]*graph.Source, baseSources map[string]string) bool {
	for path := range baseSources {
		if _, ok := sources[path]; !ok {
			return true
		}
	}
	return false
}

// projectHasChange reports whether a project gained, lost, or modified any
// source, requiring its indexer to run.
func projectHasChange(project *pb.Project, sources map[string]*graph.Source, baseSources map[string]string, kept map[string]bool) bool {
	root := strings.Trim(project.Root, "/")
	for path := range sources {
		if underRoot(path, root) && !kept[path] {
			return true
		}
	}
	for path := range baseSources {
		if !underRoot(path, root) {
			continue
		}
		if _, ok := sources[path]; !ok {
			return true
		}
	}
	return false
}

// underRoot reports whether a repository-relative path lies inside a project
// root; "." covers the repository root.
func underRoot(path, root string) bool {
	if root == "" || root == "." {
		return true
	}
	return path == root || strings.HasPrefix(path, root+"/")
}

// toolVersion probes an indexer for its version, tolerating tools whose version
// flag is unsupported or whose output format differs.
func toolVersion(ctx context.Context, tool string, args []string) string {
	vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(vctx, tool, args...).CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0])
}

func verifySCIPManifest(artifact, projectRoot string, sources map[string]*graph.Source) (map[string]string, error) {
	b, e := os.ReadFile(artifact + ".manifest.json")
	if e != nil {
		if os.IsNotExist(e) {
			return nil, nil
		}
		return nil, fmt.Errorf("imported SCIP requires %s.manifest.json: %w", artifact, e)
	}
	var hashes map[string]string
	if e = json.Unmarshal(b, &hashes); e != nil {
		return nil, e
	}
	if len(hashes) == 0 {
		return nil, fmt.Errorf("empty SCIP manifest")
	}
	for rel, want := range hashes {
		path := filepath.ToSlash(filepath.Clean(filepath.Join(projectRoot, rel)))
		s := sources[path]
		if s == nil || s.Hash != want {
			return nil, fmt.Errorf("SCIP manifest mismatch for %s", path)
		}
	}
	return hashes, nil
}
