package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/metrics"
	"github.com/mertcikla/tld/v2/internal/codeindex/tools"
)

type Pipeline struct {
	Config config.Config
	// RepositoryID preserves canonical identity when indexing a temporary checkout.
	RepositoryID string
	// Metrics, when non-nil, collects coarse per-stage timings and item counts
	// for the indexing run. It is optional and safe to leave nil.
	Metrics *metrics.Collector
}

// measure starts a coarse stage timer, folding the stage's item count in when
// the returned function is called. A nil Metrics collector makes it a no-op.
func (p Pipeline) measure(stage string) func(items int64) {
	return p.Metrics.Measure(stage)
}

// Progress reports indexing progress. Stage names the active phase; Current and
// Total are per-stage counters (zero when unknown); Detail is an optional item
// such as the file or project currently being processed. Target names the
// comparison side an update belongs to ("base" or "head") and is empty for a
// single-target index; it lets surfaces tell two otherwise identical stage runs
// apart.
type Progress struct {
	Stage   string
	Current int64
	Total   int64
	Detail  string
	Target  string
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
	// unpublished marks a base carried over from a failed attempt of the same
	// build. Its artifacts are reusable, but its snapshot was never published
	// and must not be returned as an unchanged reuse.
	unpublished bool
}

// maxDriftAttempts bounds how many times a build restarts after repository
// inputs change mid-index. Each retry carries the previous attempt's syntax
// caches and SCIP artifacts, so only what moved is reindexed.
const maxDriftAttempts = 3

// inputDriftError reports that repository inputs changed while indexing. It
// carries the partial build so the pipeline can retry incrementally instead of
// discarding all completed work.
type inputDriftError struct {
	summary string
	snap    *pb.Snapshot
	graph   *graph.Graph
}

func (e *inputDriftError) Error() string {
	return "repository inputs changed during indexing; retry: " + e.summary
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

// build runs buildOnce and, when repository inputs drift mid-index, restarts
// with the failed attempt's graph as the base. Edits made while one attempt ran
// are picked up by the next, which reuses every unaffected project artifact and
// syntax cache, so convergence is cheap even for large repositories.
func (p Pipeline) build(ctx context.Context, req *pb.IndexRequest, progress ProgressFunc, base *IncrementalBase) (*pb.Snapshot, *graph.Graph, bool, error) {
	var lastErr error
	for attempt := 0; attempt < maxDriftAttempts; attempt++ {
		snap, g, reused, err := p.buildOnce(ctx, req, progress, base)
		if err == nil {
			return snap, g, reused, nil
		}
		var drift *inputDriftError
		if !errors.As(err, &drift) {
			return nil, nil, false, err
		}
		lastErr = err
		if attempt == maxDriftAttempts-1 {
			break
		}
		emitProgress(progress, Progress{Stage: "verify", Detail: fmt.Sprintf("inputs changed; retrying with incremental reuse (attempt %d/%d)", attempt+2, maxDriftAttempts)})
		base = &IncrementalBase{Graph: drift.graph, Snapshot: drift.snap, Sources: sourceHashes(drift.graph.Sources), unpublished: true}
	}
	return nil, nil, false, lastErr
}

// sourceHashes projects a source set into the path-to-hash view used by
// incremental reuse checks.
func sourceHashes(sources map[string]*graph.Source) map[string]string {
	hashes := make(map[string]string, len(sources))
	for path, src := range sources {
		hashes[path] = src.Hash
	}
	return hashes
}

func (p Pipeline) buildOnce(ctx context.Context, req *pb.IndexRequest, progress ProgressFunc, base *IncrementalBase) (*pb.Snapshot, *graph.Graph, bool, error) {
	root, err := filepath.Abs(req.Directory)
	if err != nil {
		return nil, nil, false, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, false, err
	}
	emitProgress(progress, Progress{Stage: "discover"})
	doneDiscover := p.measure("discover")
	var baseSources map[string]*graph.Source
	if base != nil && base.Snapshot.ConfigHash == ConfigurationHash(p.Config, req) {
		baseSources = base.Graph.Sources
	}
	projects, sources, err := discover(ctx, root, req.ProjectRoots, req.Exclude, baseSources, false)
	if err != nil {
		return nil, nil, false, err
	}
	before, revision, branch, provenance, beforeInputs, err := fingerprintInputs(ctx, root, p.Config, req, projects, sources)
	if err != nil {
		return nil, nil, false, err
	}
	doneDiscover(int64(len(sources)))
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
	if base != nil && !base.unpublished && ToolchainCompatible(ctx, p.Config, root, base.Snapshot, req.ScipArtifacts) && base.Snapshot.ContentFingerprint == before && len(kept) == len(sources) && !anyBaseSourceRemoved(sources, base.Sources) {
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
		"gotreesitter": gotreesitterVersion(),
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
	doneTreeSitter := p.measure("tree-sitter")
	var calls []callSite
	for i, src := range syntaxSources {
		emitProgress(progress, Progress{Stage: "tree-sitter", Current: int64(i), Total: int64(len(syntaxSources)), Detail: src.Path})
		sites, err := syntaxFacts(ctx, g, src)
		if err != nil {
			return nil, nil, false, err
		}
		calls = append(calls, sites...)
	}
	doneTreeSitter(int64(len(syntaxSources)))
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
	doneScip := p.measure("scip")
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
		if err := p.indexProject(ctx, g, snap, req, pr, family, scipBacked, table, root, projectDir, projectKey, fingerprint, tmp, i, totalProjects, base, progress); err != nil {
			if ctx.Err() != nil {
				return nil, nil, false, ctx.Err()
			}
			// One project failing must not discard the rest of the repository:
			// record a warning and keep indexing siblings.
			snap.Warnings = append(snap.Warnings, fmt.Sprintf("%s (%s): %v", pr.Root, family, err))
			table.skip = nil
			continue
		}
	}
	doneScip(int64(totalProjects))
	emitProgress(progress, Progress{Stage: "scip", Current: int64(totalProjects), Total: int64(totalProjects)})
	emitProgress(progress, Progress{Stage: "relationships"})
	doneRelationships := p.measure("relationships")
	table.apply(g)
	deriveCalls(g, calls, table)
	doneRelationships(int64(len(g.EdgeFacts)))
	emitProgress(progress, Progress{Stage: "infra"})
	doneInfra := p.measure("infra")
	if e := addInfraFacts(ctx, g, root); e != nil {
		return nil, nil, false, e
	}
	addFileFacts(g)
	doneInfra(int64(len(g.Facts)))
	emitProgress(progress, Progress{Stage: "verify"})
	doneVerify := p.measure("verify")
	afterProjects, afterSources, err := discover(ctx, root, req.ProjectRoots, req.Exclude, sources, false)
	if err != nil {
		return nil, nil, false, err
	}
	after, _, _, _, afterInputs, err := fingerprintInputs(ctx, root, p.Config, req, afterProjects, afterSources)
	if err != nil {
		return nil, nil, false, err
	}
	doneVerify(int64(len(afterSources)))
	if after != before {
		drift := inputDrift(beforeInputs, afterInputs)
		emitProgress(progress, Progress{Stage: "verify", Detail: "inputs changed: " + drift})
		return snap, g, false, &inputDriftError{summary: drift, snap: snap, graph: g}
	}
	snap.IngestionStatus = "complete"
	return snap, g, false, nil
}

// indexProject indexes one discovered project into g. Errors are returned for
// the caller to record as a warning so a single failing project cannot abort the
// repository; projects are independent except for cross-project symbol bindings
// applied later by table.apply.
func (p Pipeline) indexProject(
	ctx context.Context,
	g *graph.Graph,
	snap *pb.Snapshot,
	req *pb.IndexRequest,
	pr *pb.Project,
	family string,
	scipBacked bool,
	table *symbols,
	root, projectDir, projectKey, fingerprint, tmp string,
	index, total int,
	base *IncrementalBase,
	progress ProgressFunc,
) error {
	if base != nil && req.ScipArtifacts[pr.Root] == "" {
		if cached, ok := base.Graph.ProjectArtifacts[projectKey]; ok && cached.Fingerprint == fingerprint {
			spec, err := indexerForFamily(family, p.Config)
			if err != nil {
				return err
			}
			exe, resolveErr := tools.ResolveName(spec.executable(p.Config, indexerContext{projectDir: projectDir, root: root}))
			if resolveErr != nil {
				return resolveErr
			}
			version := toolVersion(ctx, exe, spec.versionArgs)
			if version == base.Snapshot.ToolVersions[spec.name] {
				snap.ToolVersions[spec.name] = version
				hashes := projectHashes(pr, g.Sources)
				emitProgress(progress, Progress{Stage: "scip", Current: int64(index), Total: int64(total), Detail: fmt.Sprintf("reusing cached · %s · %s", spec.name, pr.Root)})
				if err := importSCIPReader(ctx, g, pr, bytes.NewReader(cached.Data), hashes, false, scipBacked, table); err != nil {
					return err
				}
				g.ProjectArtifacts[projectKey] = cached
				return nil
			}
		}
	}
	artifact := req.ScipArtifacts[pr.Root]
	strict := artifact != ""
	preExisting := false
	if artifact == "" {
		spec, err := indexerForFamily(family, p.Config)
		if err != nil {
			return err
		}
		c := indexerContext{root: root, projectDir: projectDir, artifact: filepath.Join(tmp, fmt.Sprintf("%d.scip", index)), name: filepath.Base(projectDir), configPath: pr.ConfigPath}
		if family == familyWeb {
			c.sourceFiles = webSourceFiles(root, pr.Root, g.Sources)
		}
		tool, err := tools.ResolveName(spec.executable(p.Config, c))
		if err != nil {
			return fmt.Errorf("%s unavailable: %w", spec.name, err)
		}
		snap.ToolVersions[spec.name] = toolVersion(ctx, tool, spec.versionArgs)
		argv, explicit, err := spec.args(p.Config, c)
		if err != nil {
			return err
		}
		if generated := inferredTsconfig(projectDir, argv); generated != "" {
			// scip-typescript writes an inferred tsconfig.json into the project;
			// remove it so indexing does not dirty the checkout or change the
			// post-index input fingerprint.
			defer func() { _ = os.Remove(generated) }()
		}
		implicitArtifact := filepath.Join(projectDir, "index.scip")
		if !explicit {
			// Detect a user-supplied artifact before the tool runs so a
			// generated one can be cleaned up without deleting theirs.
			if _, statErr := os.Stat(implicitArtifact); statErr == nil {
				preExisting = true
			}
		}
		runTool := func(args []string) ([]byte, error) {
			toolCtx, cancel := context.WithTimeout(ctx, p.Config.ToolTimeout())
			defer cancel()
			return tools.Run(toolCtx, tool, args, projectDir)
		}
		emitProgress(progress, Progress{Stage: "scip", Current: int64(index), Total: int64(total), Detail: fmt.Sprintf("running %s · %s", spec.name, pr.Root)})
		out, runErr := runTool(argv)
		if runErr != nil && !errors.Is(runErr, context.DeadlineExceeded) && !errors.Is(runErr, context.Canceled) && spec.fallbackArgs != nil {
			// Extra projects are an enrichment; when one of them breaks the
			// invocation, retry with the primary project only.
			if fallback, fallbackExplicit, fallbackErr := spec.fallbackArgs(p.Config, c); fallbackErr == nil && len(fallback) > 0 && !slices.Equal(fallback, argv) {
				_ = os.Remove(c.artifact)
				if retryOut, retryErr := runTool(fallback); retryErr == nil {
					explicit, out, runErr = fallbackExplicit, retryOut, nil
				}
			}
		}
		if runErr != nil {
			return fmt.Errorf("%s failed in %s: %w: %s", spec.name, pr.Root, runErr, strings.TrimSpace(string(out)))
		}
		if explicit {
			artifact = c.artifact
		} else {
			artifact = implicitArtifact
		}
	}
	hashes := projectHashes(pr, g.Sources)
	if strict {
		manifest, err := verifySCIPManifest(artifact, pr.Root, g.Sources)
		if err != nil {
			return err
		}
		hashes = manifest
	}
	if err := importSCIP(ctx, g, pr, artifact, hashes, strict, scipBacked, table); err != nil {
		table.skip = nil
		return fmt.Errorf("import %s: %w", artifact, err)
	}
	table.skip = nil
	if !strict {
		data, err := os.ReadFile(artifact)
		if err != nil {
			return err
		}
		g.ProjectArtifacts[projectKey] = graph.ProjectArtifact{Fingerprint: fingerprint, Data: data}
	}
	if !strict && !preExisting && artifact != "" && strings.HasPrefix(artifact, projectDir+string(filepath.Separator)) {
		_ = os.Remove(artifact)
	}
	return nil
}

// webSourceFiles returns the absolute paths of a web project's indexed sources,
// used to synthesize a JavaScript-aware tsconfig for empty project configs.
func webSourceFiles(root, projectRoot string, sources map[string]*graph.Source) []string {
	prefix := strings.Trim(strings.TrimSpace(projectRoot), "/")
	out := make([]string, 0, len(sources))
	for path := range sources {
		if prefix != "" && prefix != "." && !strings.HasPrefix(path, prefix+"/") {
			continue
		}
		out = append(out, filepath.Join(root, filepath.FromSlash(path)))
	}
	sort.Strings(out)
	return out
}

// gotreesitterVersion reports the linked gotreesitter module version so the
// snapshot records the parser that produced its syntax facts.
func gotreesitterVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/odvcencio/gotreesitter" {
				return strings.TrimPrefix(dep.Version, "v")
			}
		}
	}
	return "unknown"
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
	out, err := tools.Run(vctx, tool, args, "")
	if err != nil || strings.HasPrefix(string(out), "[earlier tool output truncated]") {
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
