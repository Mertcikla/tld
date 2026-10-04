package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

type Pipeline struct {
	Config config.Config
	// RepositoryID preserves canonical identity when indexing a temporary checkout.
	RepositoryID string
}

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
// incremental build. Unchanged syntax extraction and compatible SCIP artifacts
// are reused; symbol bindings are recomputed from current project inputs.
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
	var baseSources map[string]*graph.Source
	if base != nil && base.Snapshot.ConfigHash == ConfigurationHash(p.Config, req) {
		baseSources = base.Graph.Sources
	}
	projects, sources, err := discover(ctx, root, req.ProjectRoots, req.Exclude, baseSources, false)
	if err != nil {
		return nil, nil, false, err
	}
	before, revision, branch, provenance, err := fingerprintInputs(ctx, root, p.Config, req, projects, sources)
	if err != nil {
		return nil, nil, false, err
	}
	repo := p.RepositoryID
	if repo == "" {
		repo = graph.RepositoryID(root)
	}
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
	if base != nil && ToolchainCompatible(ctx, p.Config, root, base.Snapshot, req.ScipArtifacts) && base.Snapshot.ContentFingerprint == before && len(kept) == len(sources) && !anyBaseSourceRemoved(sources, base.Sources) {
		// Nothing changed; reuse the published snapshot verbatim.
		return base.Snapshot, base.Graph, true, nil
	}

	snap := &pb.Snapshot{Id: snapshot, RepositoryId: repo, CreatedUnix: time.Now().Unix(), Projects: projects, IngestionStatus: "staging"}
	snap.ConfigHash = ConfigurationHash(p.Config, req)
	snap.ContentFingerprint = before
	snap.Provenance = provenance
	snap.GitRevision = revision
	snap.GitBranch = branch
	snap.CommitMessage = commitSubject(ctx, root)
	snap.ToolVersions = map[string]string{
		"gotreesitter": "0.15.2",
	}
	for _, s := range sources {
		snap.Sources = append(snap.Sources, &pb.SourceFile{Path: s.Path, Hash: s.Hash, Size: uint64(len(s.Text))})
	}
	sort.Slice(snap.Sources, func(i, j int) bool { return snap.Sources[i].Path < snap.Sources[j].Path })
	var syntaxSources []*graph.Source
	for _, f := range snap.Sources {
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
		sites, err := syntaxFacts(ctx, g, src)
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
	// Reindex only projects whose own inputs changed. Unchanged projects reuse
	// their cached SCIP artifacts, and cross-project bindings are recomputed by
	// table.apply against the current symbol table.
	projectFingerprints, err := symbolInputs(root, projects, sources, snap.ConfigHash)
	if err != nil {
		return nil, nil, false, err
	}
	totalProjects := len(projects)
	emitProgress(progress, Progress{Stage: "scip", Total: int64(totalProjects)})
	for i, pr := range projects {
		family := languageFamily(pr.Language)
		scipBacked := !isSyntaxFamily(family)
		tool := "scip"
		if spec, specErr := indexerForFamily(family, p.Config); specErr == nil {
			tool = spec.name
		}
		emitProgress(progress, Progress{Stage: "scip", Current: int64(i), Total: int64(totalProjects), Detail: fmt.Sprintf("%s · %s", tool, pr.Root)})
		projectDir := filepath.Join(root, filepath.FromSlash(pr.Root))
		projectKey := family + "|" + pr.Root
		fingerprint := projectFingerprints[projectKey]
		if base != nil && req.ScipArtifacts[pr.Root] == "" {
			if cached, ok := base.Graph.ProjectArtifacts[projectKey]; ok && cached.Fingerprint == fingerprint {
				spec, e := indexerForFamily(family, p.Config)
				if e != nil {
					return nil, nil, false, e
				}
				version := toolVersion(ctx, spec.executable(p.Config, indexerContext{projectDir: projectDir, root: root}), spec.versionArgs)
				if version == base.Snapshot.ToolVersions[spec.name] {
					snap.ToolVersions[spec.name] = version
					hashes := projectHashes(pr, sources)
					emitProgress(progress, Progress{Stage: "scip", Current: int64(i), Total: int64(totalProjects), Detail: fmt.Sprintf("reusing cached · %s · %s", spec.name, pr.Root)})
					if err := importSCIPReader(ctx, g, pr, bytes.NewReader(cached.Data), hashes, false, scipBacked, table); err != nil {
						return nil, nil, false, err
					}
					g.ProjectArtifacts[projectKey] = cached
					continue
				}
			}
		}
		artifact := req.ScipArtifacts[pr.Root]
		strict := artifact != ""
		var hashes map[string]string
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
			runTool := func(args []string) ([]byte, error) {
				toolCtx, cancel := context.WithTimeout(ctx, p.Config.ToolTimeout())
				defer cancel()
				cmd := exec.CommandContext(toolCtx, tool, args...)
				cmd.Dir = projectDir
				return cmd.CombinedOutput()
			}
			emitProgress(progress, Progress{Stage: "scip", Current: int64(i), Total: int64(totalProjects), Detail: fmt.Sprintf("running %s · %s", spec.name, pr.Root)})
			out, e := runTool(argv)
			if e != nil && spec.fallbackArgs != nil {
				// Extra projects are an enrichment; when one of them breaks the
				// invocation, retry with the primary project only.
				if fallback, fallbackExplicit, fallbackErr := spec.fallbackArgs(p.Config, c); fallbackErr == nil && len(fallback) > 0 && !slices.Equal(fallback, argv) {
					_ = os.Remove(c.artifact)
					if retryOut, retryErr := runTool(fallback); retryErr == nil {
						explicit, out, e = fallbackExplicit, retryOut, nil
					}
				}
			}
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
		hashes = projectHashes(pr, sources)
		if strict {
			hashes, err = verifySCIPManifest(artifact, pr.Root, sources)
			if err != nil {
				return nil, nil, false, err
			}
		}
		if err := importSCIP(ctx, g, pr, artifact, hashes, strict, scipBacked, table); err != nil {
			table.skip = nil
			return nil, nil, false, fmt.Errorf("import %s: %w", artifact, err)
		}
		table.skip = nil
		if !strict {
			data, err := os.ReadFile(artifact)
			if err != nil {
				return nil, nil, false, err
			}
			g.ProjectArtifacts[projectKey] = graph.ProjectArtifact{Fingerprint: fingerprint, Data: data}
		}
		if !strict && !preExisting && artifact != "" && strings.HasPrefix(artifact, projectDir+string(filepath.Separator)) {
			_ = os.Remove(artifact)
		}
	}
	emitProgress(progress, Progress{Stage: "scip", Current: int64(totalProjects), Total: int64(totalProjects)})
	emitProgress(progress, Progress{Stage: "relationships"})
	table.apply(g)
	deriveCalls(g, calls, table)
	emitProgress(progress, Progress{Stage: "infra"})
	if e := addInfraFacts(ctx, g, root); e != nil {
		return nil, nil, false, e
	}
	addFileFacts(g)
	emitProgress(progress, Progress{Stage: "verify"})
	afterProjects, afterSources, err := discover(ctx, root, req.ProjectRoots, req.Exclude, sources, false)
	if err != nil {
		return nil, nil, false, err
	}
	after, _, _, _, err := fingerprintInputs(ctx, root, p.Config, req, afterProjects, afterSources)
	if err != nil {
		return nil, nil, false, err
	}
	if after != before {
		return nil, nil, false, fmt.Errorf("repository inputs changed during indexing; retry")
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
