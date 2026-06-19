package watch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mertcikla/tld/v2/internal/analyzer"
	analyzerlsp "github.com/mertcikla/tld/v2/internal/analyzer/lsp"
	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/ignore"
	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/defaults"
)

const (
	enrichmentVersion         = "watch-enrich-v2"
	enrichmentVersionEnricher = "watch.enrichment"
	enrichmentVersionType     = "watch.enrichment.version"

	maxSourceFileBytes = 100 * 1024 * 1024 // 100 MB
)

type Scanner struct {
	Store          *Store
	Analyzer       analyzer.Service
	Enrichers      *enrich.Registry
	Rules          *ignore.Rules
	EffectiveRules *ignore.Rules
	Progress       ProgressSink
	Logger         EventLogger
	Settings       Settings

	resolver         definitionResolver
	resolverFactory  func(rootDir string) definitionResolver
	resolverMu       sync.Mutex
	resolverRepoRoot string

	expansionCache map[string]*analyzer.Result
	locationsCache map[string][]analyzerlsp.DefinitionLocation
	cacheMu        sync.Mutex
}

type definitionResolver interface {
	ResolveDefinitions(context.Context, analyzer.Ref) ([]analyzerlsp.DefinitionLocation, error)
	References(context.Context, string, int, int) ([]analyzerlsp.DefinitionLocation, error)
	Implementations(context.Context, string, int, int) ([]analyzerlsp.DefinitionLocation, error)
	IncomingCalls(context.Context, string, int, int) ([]analyzerlsp.CallLocation, error)
	OutgoingCalls(context.Context, string, int, int) ([]analyzerlsp.CallLocation, error)
	Close() error
}

type synchronizedProgress struct {
	sink     ProgressSink
	mu       sync.Mutex
	finished bool
}

func (p *synchronizedProgress) Start(label string, total int) {
	if p.sink == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finished = false
	p.sink.Start(label, total)
}

func (p *synchronizedProgress) Advance(label string) {
	if p.sink == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sink.Advance(label)
}

func (p *synchronizedProgress) Finish() {
	if p.sink == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	p.finished = true
	p.sink.Finish()
}

func NewScanner(store *Store) *Scanner {
	return &Scanner{
		Store:    store,
		Analyzer: analyzer.NewService(),
		Rules:    &ignore.Rules{},
	}
}

func (s *Scanner) Scan(ctx context.Context, path string) (ScanResult, error) {
	return s.ScanWithOptions(ctx, path, ScanOptions{})
}

type ScanOptions struct {
	Force      bool
	DataDir    string
	FocusFiles []string
}

func (s *Scanner) ScanFilesWithOptions(ctx context.Context, repo Repository, relFiles []string, opts ScanOptions) (ScanResult, error) {
	if s == nil || s.Store == nil {
		return ScanResult{}, fmt.Errorf("watch scanner requires a store")
	}
	s.cacheMu.Lock()
	s.expansionCache = make(map[string]*analyzer.Result)
	s.locationsCache = make(map[string][]analyzerlsp.DefinitionLocation)
	s.cacheMu.Unlock()
	if s.Analyzer == nil {
		s.Analyzer = analyzer.NewService()
	}
	repoRoot := filepath.Clean(repo.RepoRoot)
	gitignoreRules, err := ignore.LoadGitIgnore(repoRoot)
	if err != nil {
		return ScanResult{}, fmt.Errorf("load .gitignore rules: %w", err)
	}
	effectiveRules := ignore.Merge(s.Rules, gitignoreRules)
	if effectiveRules == nil {
		effectiveRules = &ignore.Rules{}
	}
	s.EffectiveRules = effectiveRules
	settings := NormalizeSettings(s.Settings)
	s.ensureDefaultEnrichers(settings)
	allowed := map[string]struct{}{}
	for _, language := range settings.Languages {
		allowed[language] = struct{}{}
	}

	files := make([]string, 0, len(relFiles))
	seenRel := map[string]struct{}{}
	for _, rel := range relFiles {
		rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
		if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") {
			continue
		}
		absFile := filepath.Join(repoRoot, filepath.FromSlash(rel))
		language, parseable, ok := watchedFileLanguage(absFile)
		if !ok || (parseable && !languageAllowed(language, allowed)) || effectiveRules.ShouldIgnorePath(rel) {
			continue
		}
		if _, ok := seenRel[rel]; ok {
			continue
		}
		seenRel[rel] = struct{}{}
		files = append(files, absFile)
	}
	sort.Strings(files)
	repoSignals := enrich.DiscoverRepositorySignalsFromFiles(repoRoot, files)
	result := ScanResult{RepositoryID: repo.ID, FilesSeen: len(files), LSP: InitialLSPStatus(settings)}
	logInfo(ctx, s.Logger, "watch.scan.source_discovery.completed", "repository_id", repo.ID, "files", len(files), "mode", "focused")
	mode := "focused"
	if opts.Force {
		mode = "focused-force"
	}
	runID, err := s.Store.BeginScanRun(ctx, repo.ID, mode)
	if err != nil {
		return ScanResult{}, err
	}
	result.ScanRunID = runID
	status := "completed"
	var scanErr error
	defer func() {
		if scanErr != nil {
			status = "failed"
		}
		_ = s.Store.FinishScanRun(context.WithoutCancel(ctx), runID, status, result, scanErr)
	}()
	if len(files) == 0 {
		return result, nil
	}
	workers := runtime.NumCPU()
	progress := &synchronizedProgress{sink: s.Progress}
	cache, err := s.loadScanCache(ctx, repo.ID, opts.Force, progress)
	if err != nil {
		scanErr = err
		return result, err
	}
	cache.blobHashesByPath = s.loadBlobHashes(ctx, repoRoot)
	identityStarted := time.Now()
	missingIdentities, err := s.Store.replacementMissingIdentityCandidates(ctx, repo.ID)
	if err != nil {
		scanErr = err
		return result, err
	}
	logInfo(ctx, s.Logger, "watch.scan.identity_candidates.completed", "elapsed", logElapsed(identityStarted), "repository_id", repo.ID, "missing_symbol_identities", len(missingIdentities), "mode", "focused")
	progressStart(progress, "Scanning context files", len(files))
	defer progressFinish(progress)
	fileResults, err := s.scanFiles(ctx, repo.ID, repoRoot, files, workers, progress, opts.Force, effectiveRules, repoSignals, cache, missingIdentities)
	if err != nil {
		scanErr = err
		return result, err
	}
	var parsedFiles []parsedFile
	var parsedFileIDs []int64
	for _, fileResult := range fileResults {
		if fileResult.Skipped {
			result.FilesSkipped++
		}
		if fileResult.Parsed {
			result.FilesParsed++
			result.SymbolsSeen += fileResult.SymbolsSeen
			parsedFiles = append(parsedFiles, parsedFile{File: fileResult.File, Refs: fileResult.Refs})
			parsedFileIDs = append(parsedFileIDs, fileResult.File.ID)
		}
		result.Warnings = append(result.Warnings, fileResult.Warnings...)
	}
	if len(parsedFileIDs) == 0 {
		return result, nil
	}
	resolveStarted := time.Now()
	logInfo(ctx, s.Logger, "watch.scan.references.started", "repository_id", repo.ID, "files", len(parsedFiles))
	progressStart(progress, "Resolving code references", len(parsedFiles))
	refs, warning, err := s.resolveReferences(ctx, repoRoot, repo.ID, parsedFiles, settings, progress)
	progressFinish(progress)
	if err != nil {
		scanErr = err
		logError(ctx, s.Logger, "watch.scan.references.failed", err, "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID)
		return result, err
	}
	result.LSP = s.currentLSPStatus(settings)
	result.Warnings = append(result.Warnings, lspWarnings(result.LSP)...)
	logInfo(ctx, s.Logger, "watch.scan.references.completed", "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID, "references", len(refs), "warning", warning, "lsp_active", result.LSP.Summary.Active, "lsp_failed", result.LSP.Summary.Failed, "lsp_unavailable", result.LSP.Summary.Unavailable)
	result.Warning = warning
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	if err := s.Store.ReplaceReferencesForFiles(ctx, repo.ID, parsedFileIDs, refs); err != nil {
		scanErr = err
		return result, err
	}
	result.ReferencesSeen = len(refs)
	return result, nil
}

func (s *Scanner) ScanWithOptions(ctx context.Context, path string, opts ScanOptions) (ScanResult, error) {
	if s == nil || s.Store == nil {
		return ScanResult{}, fmt.Errorf("watch scanner requires a store")
	}
	s.cacheMu.Lock()
	s.expansionCache = make(map[string]*analyzer.Result)
	s.locationsCache = make(map[string][]analyzerlsp.DefinitionLocation)
	s.cacheMu.Unlock()
	if s.Analyzer == nil {
		s.Analyzer = analyzer.NewService()
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ScanResult{}, err
	}
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		return ScanResult{}, fmt.Errorf("%s is not inside a git repository: %w", path, err)
	}
	repoRoot = filepath.Clean(repoRoot)
	gitignoreRules, err := ignore.LoadGitIgnore(repoRoot)
	if err != nil {
		return ScanResult{}, fmt.Errorf("load .gitignore rules: %w", err)
	}
	effectiveRules := ignore.Merge(s.Rules, gitignoreRules)
	if effectiveRules == nil {
		effectiveRules = &ignore.Rules{}
	}
	s.EffectiveRules = effectiveRules
	settings := NormalizeSettings(s.Settings)
	s.ensureDefaultEnrichers(settings)

	repoInput := RepositoryInput{
		RemoteURL:    detectString(func() (string, error) { return tldgit.DetectRemoteURL(repoRoot) }),
		RepoRoot:     repoRoot,
		DisplayName:  filepath.Base(repoRoot),
		Branch:       detectString(func() (string, error) { return tldgit.DetectBranch(repoRoot) }),
		HeadCommit:   detectString(func() (string, error) { return tldgit.DetectHeadCommit(repoRoot) }),
		SettingsHash: stableHash(settings),
	}
	repo, err := s.Store.EnsureRepository(ctx, repoInput)
	if err != nil {
		return ScanResult{}, err
	}
	result := ScanResult{RepositoryID: repo.ID, LSP: InitialLSPStatus(settings)}
	plan, err := planScan(repoRoot, settings, effectiveRules)
	if err != nil {
		return ScanResult{}, err
	}
	result.Mode = plan.Mode
	result.Strategy = plan.Strategy
	result.StrategyReason = plan.Reason
	result.TrackedFiles = plan.TrackedFiles
	result.SelectedFiles = plan.SelectedFiles
	result.SkippedTrackedFiles = plan.SkippedTrackedFiles
	result.Warnings = append(result.Warnings, plan.Warnings...)
	if plan.Limited {
		if warning := s.prepareLimitedBaselineWorktree(repoRoot, repo.ID, opts.DataDir, repo.HeadCommit.String, &result); warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
	}

	mode := "incremental"
	if plan.Limited {
		mode = "limited"
	}
	if opts.Force {
		mode = "full"
		if plan.Limited {
			mode = "limited-force"
		}
	}
	runID, err := s.Store.BeginScanRun(ctx, repo.ID, mode)
	if err != nil {
		return ScanResult{}, err
	}
	result.ScanRunID = runID
	status := "completed"
	var scanErr error
	defer func() {
		if scanErr != nil {
			status = "failed"
		}
		_ = s.Store.FinishScanRun(context.WithoutCancel(ctx), runID, status, result, scanErr)
	}()

	workers := runtime.NumCPU()
	progress := &synchronizedProgress{sink: s.Progress}
	var files []string
	if plan.Limited {
		focusFiles, focusWarnings := s.limitedFocusFiles(ctx, repo.ID, repoRoot, opts.FocusFiles, settings, effectiveRules)
		result.Warnings = append(result.Warnings, focusWarnings...)
		seeds := append([]string{}, plan.RecentFiles...)
		expanded, err := s.expandLimitedFiles(ctx, repo.ID, repoRoot, focusFiles, seeds, plan.AnchorFiles, settings, effectiveRules)
		if err != nil {
			result.Warnings = append(result.Warnings, "limited view: graph expansion failed: "+err.Error())
			files = uniqueLimitedAbsFiles(append(append(append([]string{}, focusFiles...), seeds...), plan.AnchorFiles...), settings.Scale.MaxLimitedFiles)
			result.LimitedFallback = "graph expansion failed"
		} else {
			files = expanded.Files
			applyLimitedExpansionResult(repoRoot, &result, expanded)
		}
		logInfo(ctx, s.Logger, "watch.scan.source_discovery.completed", "repository_id", repo.ID, "files", len(files), "tracked_files", result.TrackedFiles, "selected_files", result.SelectedFiles, "mode", result.Mode, "strategy", result.Strategy)
	} else {
		discoveryStarted := time.Now()
		logInfo(ctx, s.Logger, "watch.scan.source_discovery.started", "repository_id", repo.ID, "repo_root", repoRoot, "languages", strings.Join(settings.Languages, ","))
		files, err = s.collectSourceFiles(repoRoot, workers, settings.Languages, effectiveRules, progress)
		progressFinish(progress)
		if err != nil {
			if settings.Scale.Strategy != ScanStrategyAuto {
				scanErr = err
				logError(ctx, s.Logger, "watch.scan.source_discovery.failed", err, "elapsed", logElapsed(discoveryStarted), "repository_id", repo.ID)
				return result, err
			}
			fallbackReason := "full source discovery failed: " + err.Error()
			result.Mode = ScanStrategyLimited
			result.LimitedFallback = fallbackReason
			result.Warnings = append(result.Warnings, "limited view: "+fallbackReason)
			if warning := s.prepareLimitedBaselineWorktree(repoRoot, repo.ID, opts.DataDir, repo.HeadCommit.String, &result); warning != "" {
				result.Warnings = append(result.Warnings, warning)
			}
			tracked, trackErr := tldgit.ListTrackedFiles(repoRoot, 0)
			if trackErr != nil {
				scanErr = err
				result.Warnings = append(result.Warnings, "limited view: tracked-file listing failed during fallback: "+trackErr.Error())
				logError(ctx, s.Logger, "watch.scan.source_discovery.failed", err, "elapsed", logElapsed(discoveryStarted), "repository_id", repo.ID)
				return result, err
			}
			result.TrackedFiles = tracked.Total
			focusFiles, focusWarnings := s.limitedFocusFiles(ctx, repo.ID, repoRoot, opts.FocusFiles, settings, effectiveRules)
			result.Warnings = append(result.Warnings, focusWarnings...)
			seeds := selectRecentFiles(repoRoot, settings, effectiveRules)
			if len(seeds) == 0 {
				result.Warnings = append(result.Warnings, "limited view: local git history did not return recent files; falling back to tracked file order")
				seeds = selectTrackedFallbackFiles(repoRoot, tracked.Files, settings, effectiveRules)
			}
			expanded, expandErr := s.expandLimitedFiles(ctx, repo.ID, repoRoot, focusFiles, seeds, selectAnchorFiles(repoRoot, tracked.Files, settings, effectiveRules), settings, effectiveRules)
			if expandErr != nil {
				scanErr = err
				result.Warnings = append(result.Warnings, "limited view: graph expansion failed during fallback: "+expandErr.Error())
				logError(ctx, s.Logger, "watch.scan.source_discovery.failed", err, "elapsed", logElapsed(discoveryStarted), "repository_id", repo.ID)
				return result, err
			}
			files = expanded.Files
			applyLimitedExpansionResult(repoRoot, &result, expanded)
			logInfo(ctx, s.Logger, "watch.scan.source_discovery.fallback_limited", "elapsed", logElapsed(discoveryStarted), "repository_id", repo.ID, "files", len(files), "reason", fallbackReason)
		} else {
			logInfo(ctx, s.Logger, "watch.scan.source_discovery.completed", "elapsed", logElapsed(discoveryStarted), "repository_id", repo.ID, "files", len(files), "mode", result.Mode, "strategy", result.Strategy)
		}
	}
	if err := ctx.Err(); err != nil {
		scanErr = err
		return result, err
	}
	repoSignals := enrich.DiscoverRepositorySignalsFromFiles(repoRoot, files)
	result.FilesSeen = len(files)
	if result.Mode == ScanStrategyLimited {
		result.SelectedFiles = len(files)
		result.SkippedTrackedFiles = max(result.TrackedFiles-len(files), 0)
	}
	cache, err := s.loadScanCache(ctx, repo.ID, opts.Force, progress)
	if err != nil {
		scanErr = err
		return result, err
	}
	cache.blobHashesByPath = s.loadBlobHashes(ctx, repoRoot)
	identityStarted := time.Now()
	missingIdentities, err := s.Store.replacementMissingIdentityCandidates(ctx, repo.ID)
	if err != nil {
		scanErr = err
		return result, err
	}
	logInfo(ctx, s.Logger, "watch.scan.identity_candidates.completed", "elapsed", logElapsed(identityStarted), "repository_id", repo.ID, "missing_symbol_identities", len(missingIdentities), "mode", result.Mode, "strategy", result.Strategy)
	progressStart(progress, "Scanning source files", len(files))
	defer progressFinish(progress)
	seen := make(map[string]struct{}, len(files))
	var parsedFiles []parsedFile
	var parsedFileIDs []int64

	fileResults, err := s.scanFiles(ctx, repo.ID, repoRoot, files, workers, progress, opts.Force, effectiveRules, repoSignals, cache, missingIdentities)
	if err != nil {
		scanErr = err
		return result, err
	}
	for _, fileResult := range fileResults {
		seen[fileResult.RelPath] = struct{}{}
		if fileResult.Skipped {
			result.FilesSkipped++
		}
		if fileResult.Parsed {
			result.FilesParsed++
			result.SymbolsSeen += fileResult.SymbolsSeen
			parsedFiles = append(parsedFiles, parsedFile{File: fileResult.File, Refs: fileResult.Refs})
			parsedFileIDs = append(parsedFileIDs, fileResult.File.ID)
		}
		result.Warnings = append(result.Warnings, fileResult.Warnings...)
	}

	if err := ctx.Err(); err != nil {
		scanErr = err
		return result, err
	}
	if result.Mode != ScanStrategyLimited {
		if err := s.Store.DeleteMissingFiles(ctx, repo.ID, seen); err != nil {
			scanErr = err
			return result, err
		}
	} else if err := s.deleteLimitedRemovedFiles(ctx, repoRoot, repo.ID); err != nil {
		scanErr = err
		return result, err
	}
	if len(parsedFileIDs) == 0 {
		if summary, err := s.Store.Summary(ctx, repo.ID); err == nil {
			result.SymbolsSeen = summary.Symbols
			result.ReferencesSeen = summary.References
		}
		return result, nil
	}

	resolveStarted := time.Now()
	logInfo(ctx, s.Logger, "watch.scan.references.started", "repository_id", repo.ID, "files", len(parsedFiles))
	progressStart(progress, "Resolving code references", len(parsedFiles))
	refs, warning, err := s.resolveReferences(ctx, repoRoot, repo.ID, parsedFiles, settings, progress)
	progressFinish(progress)
	if err != nil {
		scanErr = err
		logError(ctx, s.Logger, "watch.scan.references.failed", err, "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID)
		return result, err
	}
	result.LSP = s.currentLSPStatus(settings)
	result.Warnings = append(result.Warnings, lspWarnings(result.LSP)...)
	logInfo(ctx, s.Logger, "watch.scan.references.completed", "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID, "references", len(refs), "warning", warning, "lsp_active", result.LSP.Summary.Active, "lsp_failed", result.LSP.Summary.Failed, "lsp_unavailable", result.LSP.Summary.Unavailable)
	result.Warning = warning
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	if err := ctx.Err(); err != nil {
		scanErr = err
		return result, err
	}
	if err := s.Store.ReplaceReferencesForFiles(ctx, repo.ID, parsedFileIDs, refs); err != nil {
		scanErr = err
		return result, err
	}
	result.ReferencesSeen = len(refs)
	return result, nil
}

func changedScanFiles(repoRoot string, changes map[string]tldgit.WorktreeChange, settings Settings, rules *ignore.Rules) []string {
	settings = NormalizeSettings(settings)
	allowed := map[string]struct{}{}
	for _, language := range settings.Languages {
		allowed[language] = struct{}{}
	}
	var files []string
	for rel, change := range changes {
		if change == tldgit.WorktreeDeleted {
			continue
		}
		rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
			continue
		}
		if rules != nil && rules.ShouldIgnorePath(rel) {
			continue
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		language, parseable, ok := watchedFileLanguage(abs)
		if !ok || (parseable && !languageAllowed(language, allowed)) {
			continue
		}
		files = append(files, abs)
	}
	sort.Strings(files)
	return files
}

func (s *Scanner) limitedFocusFiles(ctx context.Context, repositoryID int64, repoRoot string, explicit []string, settings Settings, rules *ignore.Rules) ([]string, []string) {
	var files []string
	var warnings []string
	add := func(candidates []string) {
		for _, candidate := range candidates {
			abs, _, ok := limitedCandidateFile(repoRoot, candidate, settings, rules)
			if ok {
				files = append(files, abs)
			}
		}
	}
	add(explicit)
	committed, err := gitChangesSinceLatestWatchVersion(ctx, s.Store, repositoryID, repoRoot)
	if err != nil {
		warnings = append(warnings, "limited view: committed change detection failed: "+err.Error())
	} else {
		add(changedScanFiles(repoRoot, committed, settings, rules))
	}
	changed, err := tldgit.WorktreeChangesAgainstHead(repoRoot)
	if err != nil {
		warnings = append(warnings, "limited view: git change detection failed: "+err.Error())
	} else {
		add(changedScanFiles(repoRoot, changed, settings, rules))
	}
	return uniqueLimitedAbsFiles(files, 0), warnings
}

func applyLimitedExpansionResult(repoRoot string, result *ScanResult, expanded limitedExpansion) {
	if result == nil {
		return
	}
	result.RecentFiles = expanded.RecentFiles
	result.AnchorFiles = expanded.AnchorFiles
	result.NeighborFiles = expanded.NeighborFiles
	result.CallerFiles = expanded.CallerFiles
	result.CallerDepthReached = expanded.CallerDepthReached
	result.SharedAncestorFound = expanded.SharedAncestorFound
	result.BlastRadiusFiles = expanded.blastRadiusFiles(repoRoot)
	result.BlastRadiusRoots = expanded.blastRadiusRoots(repoRoot)
	result.LimitedCapReached = expanded.CapReached
	result.LimitedFallback = expanded.Fallback
	if expanded.CapReached {
		result.Warnings = append(result.Warnings, "limited view: max_limited_files cap reached; graph expansion may be incomplete")
	}
	if expanded.Fallback != "" {
		result.Warnings = append(result.Warnings, "limited view: "+expanded.Fallback)
	}
}

type limitedExpansion struct {
	Files               []string
	RecentFiles         int
	AnchorFiles         int
	NeighborFiles       int
	CallerFiles         int
	CallerDepthReached  int
	SharedAncestorFound bool
	CapReached          bool
	Fallback            string

	capacity   int
	seen       map[string]string
	roots      map[string]map[string]struct{}
	blastRoots map[string]map[string]struct{}
}

type limitedParsedFile struct {
	AbsFile    string
	RelPath    string
	Roots      []string
	BlastRoots []string
	Result     analyzer.Result
}

func newLimitedExpansion(capacity int) limitedExpansion {
	return limitedExpansion{
		capacity:   capacity,
		seen:       map[string]string{},
		roots:      map[string]map[string]struct{}{},
		blastRoots: map[string]map[string]struct{}{},
	}
}

func (e *limitedExpansion) add(absFile, reason string, roots ...string) bool {
	absFile = filepath.Clean(absFile)
	if absFile == "" {
		return false
	}
	if _, ok := e.seen[absFile]; ok {
		e.addRoots(absFile, roots...)
		return false
	}
	if e.capacity > 0 && len(e.Files) >= e.capacity {
		e.CapReached = true
		return false
	}
	e.seen[absFile] = reason
	e.Files = append(e.Files, absFile)
	e.addRoots(absFile, roots...)
	switch reason {
	case "recent":
		e.RecentFiles++
	case "anchor":
		e.AnchorFiles++
	case "neighbor":
		e.NeighborFiles++
	case "blast":
		e.RecentFiles++
	case "caller":
		e.CallerFiles++
	}
	return true
}

func (e *limitedExpansion) addBlast(absFile, root string) bool {
	absFile = filepath.Clean(absFile)
	_, exists := e.seen[absFile]
	added := e.add(absFile, "blast", root)
	if added || exists {
		e.addBlastRoots(absFile, root)
	}
	return added
}

func (e *limitedExpansion) addRelated(absFile, reason string, roots, blastRoots []string) bool {
	absFile = filepath.Clean(absFile)
	_, exists := e.seen[absFile]
	added := e.add(absFile, reason, roots...)
	if added || exists {
		e.addBlastRoots(absFile, blastRoots...)
	}
	return added
}

func (e *limitedExpansion) addRoots(absFile string, roots ...string) {
	if len(roots) == 0 {
		return
	}
	if e.roots[absFile] == nil {
		e.roots[absFile] = map[string]struct{}{}
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root != "" {
			e.roots[absFile][root] = struct{}{}
		}
	}
}

func (e *limitedExpansion) addBlastRoots(absFile string, roots ...string) {
	if len(roots) == 0 {
		return
	}
	absFile = filepath.Clean(absFile)
	if e.blastRoots[absFile] == nil {
		e.blastRoots[absFile] = map[string]struct{}{}
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root != "" {
			e.blastRoots[absFile][root] = struct{}{}
		}
	}
}

func (e limitedExpansion) fileRoots(absFile string) []string {
	rootSet := e.roots[filepath.Clean(absFile)]
	if len(rootSet) == 0 {
		return nil
	}
	roots := make([]string, 0, len(rootSet))
	for root := range rootSet {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func (e limitedExpansion) fileBlastRoots(absFile string) []string {
	rootSet := e.blastRoots[filepath.Clean(absFile)]
	if len(rootSet) == 0 {
		return nil
	}
	roots := make([]string, 0, len(rootSet))
	for root := range rootSet {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func (e limitedExpansion) blastRadiusFiles(repoRoot string) []string {
	files := make([]string, 0, len(e.blastRoots))
	for absFile, roots := range e.blastRoots {
		if len(roots) == 0 {
			continue
		}
		rel, err := filepath.Rel(repoRoot, absFile)
		if err != nil {
			continue
		}
		files = append(files, filepath.ToSlash(rel))
	}
	sort.Strings(files)
	return files
}

func (e limitedExpansion) blastRadiusRoots(repoRoot string) map[string][]string {
	out := map[string][]string{}
	for absFile := range e.blastRoots {
		roots := e.fileBlastRoots(absFile)
		if len(roots) == 0 {
			continue
		}
		rel, err := filepath.Rel(repoRoot, absFile)
		if err != nil {
			continue
		}
		out[filepath.ToSlash(rel)] = roots
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Scanner) expandLimitedFiles(ctx context.Context, repositoryID int64, repoRoot string, focus, seeds, anchors []string, settings Settings, rules *ignore.Rules) (limitedExpansion, error) {
	settings = NormalizeSettings(settings)
	expanded := newLimitedExpansion(settings.Scale.MaxLimitedFiles)
	var frontier []string
	for _, file := range focus {
		absFile, rel, ok := limitedCandidateFile(repoRoot, file, settings, rules)
		if !ok {
			continue
		}
		if expanded.addBlast(absFile, rel) {
			frontier = append(frontier, absFile)
		}
	}
	focused := len(frontier) > 0
	resolver := s.getOrCreateResolver(repoRoot, settings)
	if focused {
		s.expandBlastRadius(ctx, repositoryID, repoRoot, settings, rules, resolver, frontier, &expanded)
	}
	useRecentRoots := !focused
	for _, file := range seeds {
		absFile, rel, ok := limitedCandidateFile(repoRoot, file, settings, rules)
		if !ok {
			continue
		}
		if useRecentRoots {
			if expanded.add(absFile, "recent", rel) {
				frontier = append(frontier, absFile)
			}
		} else {
			expanded.add(absFile, "recent")
		}
	}
	for _, file := range anchors {
		absFile, _, ok := limitedCandidateFile(repoRoot, file, settings, rules)
		if !ok {
			continue
		}
		expanded.add(absFile, "anchor")
	}
	if len(expanded.Files) == 0 {
		expanded.Fallback = "no parseable recent or anchor files selected"
		return expanded, nil
	}

	parsed, parseWarnings := s.parseLimitedExpansionFiles(ctx, repoRoot, expanded.Files, rules, expanded)
	if len(parseWarnings) > 0 {
		expanded.Fallback = strings.Join(parseWarnings, "; ")
	}
	referenceParsed := parsed
	if focused {
		referenceParsed = make([]limitedParsedFile, 0, len(parsed))
		for _, file := range parsed {
			if len(file.BlastRoots) == 0 {
				referenceParsed = append(referenceParsed, file)
			}
		}
	}
	s.expandReferenceNeighbors(ctx, repositoryID, repoRoot, settings, rules, resolver, referenceParsed, &expanded)
	if !focused {
		s.expandBlastRadius(ctx, repositoryID, repoRoot, settings, rules, resolver, frontier, &expanded)
	}
	return expanded, nil
}

func limitedCandidateFile(repoRoot, file string, settings Settings, rules *ignore.Rules) (string, string, bool) {
	settings = NormalizeSettings(settings)
	allowed := map[string]struct{}{}
	for _, language := range settings.Languages {
		allowed[language] = struct{}{}
	}
	absFile := filepath.Clean(file)
	if !filepath.IsAbs(absFile) {
		absFile = filepath.Join(repoRoot, filepath.FromSlash(file))
	}
	rel, err := filepath.Rel(repoRoot, absFile)
	if err != nil {
		return "", "", false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return "", "", false
	}
	if rules != nil && rules.ShouldIgnorePath(rel) {
		return "", "", false
	}
	if info, err := os.Stat(absFile); err != nil || info.IsDir() {
		return "", "", false
	}
	language, parseable, ok := watchedFileLanguage(absFile)
	if !ok || (parseable && !languageAllowed(language, allowed)) {
		return "", "", false
	}
	return absFile, rel, true
}

func (s *Scanner) parseLimitedExpansionFiles(ctx context.Context, repoRoot string, files []string, rules *ignore.Rules, expanded limitedExpansion) ([]limitedParsedFile, []string) {
	var parsed []limitedParsedFile
	var warnings []string
	analyzerSvc := s.Analyzer
	if analyzerSvc == nil {
		analyzerSvc = analyzer.NewService()
	}
	for _, absFile := range files {
		rel, err := filepath.Rel(repoRoot, absFile)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		_, parseable, ok := watchedFileLanguage(absFile)
		if !ok || !parseable {
			continue
		}
		var result *analyzer.Result
		s.cacheMu.Lock()
		var cached *analyzer.Result
		var exists bool
		if s.expansionCache != nil {
			cached, exists = s.expansionCache[absFile]
		}
		if exists {
			result = cached
			s.cacheMu.Unlock()
		} else {
			s.cacheMu.Unlock()
			var err error
			result, err = analyzerSvc.ExtractPath(ctx, absFile, rules, nil)
			if err == nil {
				s.cacheMu.Lock()
				if s.expansionCache == nil {
					s.expansionCache = make(map[string]*analyzer.Result)
				}
				s.expansionCache[absFile] = result
				s.cacheMu.Unlock()
			} else {
				warnings = append(warnings, fmt.Sprintf("could not parse %s for graph expansion: %v", rel, err))
				continue
			}
		}
		parsed = append(parsed, limitedParsedFile{
			AbsFile:    absFile,
			RelPath:    rel,
			Roots:      expanded.fileRoots(absFile),
			BlastRoots: expanded.fileBlastRoots(absFile),
			Result:     *result,
		})
	}
	return parsed, warnings
}

func (s *Scanner) expandReferenceNeighbors(ctx context.Context, repositoryID int64, repoRoot string, settings Settings, rules *ignore.Rules, resolver definitionResolver, parsed []limitedParsedFile, expanded *limitedExpansion) {
	for _, file := range parsed {
		for _, ref := range file.Result.Refs {
			if ref.Kind != "" && ref.Kind != "call" {
				continue
			}
			if resolver == nil {
				continue
			}
			key := ref.FilePath + ":" + fmt.Sprint(ref.Line) + ":" + fmt.Sprint(ref.Column)
			s.cacheMu.Lock()
			var locations []analyzerlsp.DefinitionLocation
			var exists bool
			if s.locationsCache != nil {
				locations, exists = s.locationsCache[key]
			}
			s.cacheMu.Unlock()

			if !exists {
				var err error
				locations, err = resolver.ResolveDefinitions(ctx, ref)
				if err == nil {
					s.cacheMu.Lock()
					if s.locationsCache == nil {
						s.locationsCache = make(map[string][]analyzerlsp.DefinitionLocation)
					}
					s.locationsCache[key] = locations
					s.cacheMu.Unlock()
				} else {
					continue
				}
			}
			for _, location := range locations {
				absFile, _, ok := limitedCandidateFile(repoRoot, location.FilePath, settings, rules)
				if !ok {
					continue
				}
				expanded.add(absFile, "neighbor", file.Roots...)
			}
		}
	}
	s.expandStoredReferenceNeighbors(ctx, repositoryID, repoRoot, settings, rules, parsed, expanded)
}

func (s *Scanner) expandBlastRadius(ctx context.Context, repositoryID int64, repoRoot string, settings Settings, rules *ignore.Rules, resolver definitionResolver, frontier []string, expanded *limitedExpansion) {
	if expanded == nil || settings.Scale.MaxBlastRadiusHops <= 0 || len(frontier) == 0 {
		return
	}
	frontier = uniqueLimitedAbsFiles(frontier, 0)
	processed := map[string]struct{}{}
	for depth := 1; depth <= settings.Scale.MaxBlastRadiusHops && len(frontier) > 0; depth++ {
		if expanded.CapReached {
			return
		}
		var current []string
		for _, absFile := range frontier {
			absFile = filepath.Clean(absFile)
			if _, ok := processed[absFile]; ok {
				continue
			}
			processed[absFile] = struct{}{}
			current = append(current, absFile)
		}
		if len(current) == 0 {
			return
		}
		parsed, parseWarnings := s.parseLimitedExpansionFiles(ctx, repoRoot, current, rules, *expanded)
		if len(parseWarnings) > 0 {
			if expanded.Fallback == "" {
				expanded.Fallback = strings.Join(parseWarnings, "; ")
			} else {
				expanded.Fallback += "; " + strings.Join(parseWarnings, "; ")
			}
		}
		if len(parsed) == 0 {
			return
		}
		expanded.CallerDepthReached = max(expanded.CallerDepthReached, depth)
		nextSet := map[string]struct{}{}
		addNeighbor := func(absFile, reason string, roots, blastRoots []string) {
			if len(roots) == 0 && len(blastRoots) == 0 {
				return
			}
			if reason == "caller" && limitedRootUnionCount(expanded.fileRoots(absFile), roots) >= 2 {
				expanded.SharedAncestorFound = true
			}
			hadBlastRoots := len(expanded.fileBlastRoots(absFile)) > 0
			added := expanded.addRelated(absFile, reason, roots, blastRoots)
			hasBlastRoots := len(expanded.fileBlastRoots(absFile)) > 0
			if added || (!hadBlastRoots && hasBlastRoots) {
				nextSet[filepath.Clean(absFile)] = struct{}{}
			}
		}
		s.expandStoredBlastRadiusNeighbors(ctx, repositoryID, repoRoot, settings, rules, parsed, addNeighbor)
		for _, file := range parsed {
			if len(file.Roots) == 0 && len(file.BlastRoots) == 0 {
				continue
			}
			for _, ref := range file.Result.Refs {
				if ref.Kind != "" && ref.Kind != "call" {
					continue
				}
				if resolver == nil {
					continue
				}
				locations, err := resolver.ResolveDefinitions(ctx, ref)
				if err == nil {
					for _, location := range locations {
						if absFile, _, ok := limitedCandidateFile(repoRoot, location.FilePath, settings, rules); ok {
							addNeighbor(absFile, "neighbor", file.Roots, file.BlastRoots)
						}
					}
				}
			}
			for _, symbol := range file.Result.Symbols {
				if resolver == nil {
					continue
				}
				for _, query := range []struct {
					kind      string
					locations []analyzerlsp.DefinitionLocation
				}{
					{kind: "caller", locations: definitionLocationsFromResolver(ctx, resolver.References, file.AbsFile, symbol.Line, 1)},
					{kind: "neighbor", locations: definitionLocationsFromResolver(ctx, resolver.Implementations, file.AbsFile, symbol.Line, 1)},
				} {
					for _, location := range query.locations {
						if absFile, _, ok := limitedCandidateFile(repoRoot, location.FilePath, settings, rules); ok {
							addNeighbor(absFile, query.kind, file.Roots, file.BlastRoots)
						}
					}
				}
				for _, query := range []struct {
					kind  string
					calls []analyzerlsp.CallLocation
				}{
					{kind: "caller", calls: callLocationsFromResolver(ctx, resolver.IncomingCalls, file.AbsFile, symbol.Line, 1)},
					{kind: "neighbor", calls: callLocationsFromResolver(ctx, resolver.OutgoingCalls, file.AbsFile, symbol.Line, 1)},
				} {
					for _, call := range query.calls {
						if absFile, _, ok := limitedCandidateFile(repoRoot, call.FilePath, settings, rules); ok {
							addNeighbor(absFile, query.kind, file.Roots, file.BlastRoots)
						}
					}
				}
			}
		}
		frontier = sortedLimitedAbsSet(nextSet)
	}
}

func limitedRootUnionCount(left, right []string) int {
	seen := map[string]struct{}{}
	for _, value := range left {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, value := range right {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	return len(seen)
}

func definitionLocationsFromResolver(ctx context.Context, fn func(context.Context, string, int, int) ([]analyzerlsp.DefinitionLocation, error), filePath string, line, column int) []analyzerlsp.DefinitionLocation {
	locations, err := fn(ctx, filePath, line, column)
	if err != nil {
		return nil
	}
	return locations
}

func callLocationsFromResolver(ctx context.Context, fn func(context.Context, string, int, int) ([]analyzerlsp.CallLocation, error), filePath string, line, column int) []analyzerlsp.CallLocation {
	calls, err := fn(ctx, filePath, line, column)
	if err != nil {
		return nil
	}
	return calls
}

func sortedLimitedAbsSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, filepath.Clean(value))
	}
	sort.Strings(out)
	return out
}

func (s *Scanner) expandStoredBlastRadiusNeighbors(ctx context.Context, repositoryID int64, repoRoot string, settings Settings, rules *ignore.Rules, parsed []limitedParsedFile, add func(string, string, []string, []string)) {
	if s == nil || s.Store == nil || len(parsed) == 0 || add == nil {
		return
	}
	parsedByRel := map[string]limitedParsedFile{}
	var filePaths []string
	for _, file := range parsed {
		parsedByRel[file.RelPath] = file
		filePaths = append(filePaths, file.RelPath)
	}
	symbols, err := s.Store.QuerySymbolsByFiles(ctx, repositoryID, filePaths)
	if err != nil || len(symbols) == 0 {
		return
	}
	symbolByID := map[int64]Symbol{}
	var ids []int64
	for _, symbol := range symbols {
		symbolByID[symbol.ID] = symbol
		ids = append(ids, symbol.ID)
	}
	addRefs := func(refs []Reference, sourceSide bool) {
		var relatedIDs []int64
		for _, ref := range refs {
			if sourceSide {
				relatedIDs = append(relatedIDs, ref.TargetSymbolID)
			} else {
				relatedIDs = append(relatedIDs, ref.SourceSymbolID)
			}
		}
		relatedSymbols, err := s.Store.QuerySymbolsByIDs(ctx, repositoryID, relatedIDs)
		if err != nil {
			return
		}
		relatedByID := map[int64]Symbol{}
		for _, sym := range relatedSymbols {
			relatedByID[sym.ID] = sym
		}
		for _, ref := range refs {
			var rootSymbol Symbol
			var related Symbol
			var ok bool
			reason := "neighbor"
			if sourceSide {
				rootSymbol, ok = symbolByID[ref.SourceSymbolID]
				related = relatedByID[ref.TargetSymbolID]
			} else {
				rootSymbol, ok = symbolByID[ref.TargetSymbolID]
				related = relatedByID[ref.SourceSymbolID]
				reason = "caller"
			}
			if !ok || strings.TrimSpace(related.FilePath) == "" {
				continue
			}
			rootFile, ok := parsedByRel[rootSymbol.FilePath]
			if !ok {
				continue
			}
			if absFile, _, ok := limitedCandidateFile(repoRoot, related.FilePath, settings, rules); ok {
				add(absFile, reason, rootFile.Roots, rootFile.BlastRoots)
			}
		}
	}
	if refs, err := s.Store.QueryReferencesBySourceIDs(ctx, repositoryID, ids); err == nil {
		addRefs(refs, true)
	}
	if refs, err := s.Store.QueryReferencesByTargetIDs(ctx, repositoryID, ids); err == nil {
		addRefs(refs, false)
	}
}

func (s *Scanner) expandStoredReferenceNeighbors(ctx context.Context, repositoryID int64, repoRoot string, settings Settings, rules *ignore.Rules, parsed []limitedParsedFile, expanded *limitedExpansion) {
	if s == nil || s.Store == nil || len(parsed) == 0 {
		return
	}
	var filePaths []string
	for _, file := range parsed {
		filePaths = append(filePaths, file.RelPath)
	}
	symbols, err := s.Store.QuerySymbolsByFiles(ctx, repositoryID, filePaths)
	if err != nil {
		return
	}
	var sourceIDs []int64
	sourceByID := map[int64]Symbol{}
	for _, symbol := range symbols {
		sourceIDs = append(sourceIDs, symbol.ID)
		sourceByID[symbol.ID] = symbol
	}
	refs, err := s.Store.QueryReferencesBySourceIDs(ctx, repositoryID, sourceIDs)
	if err != nil {
		return
	}
	var targetIDs []int64
	for _, ref := range refs {
		targetIDs = append(targetIDs, ref.TargetSymbolID)
	}
	targetSymbols, err := s.Store.QuerySymbolsByIDs(ctx, repositoryID, targetIDs)
	if err != nil {
		return
	}
	targetByID := map[int64]Symbol{}
	for _, sym := range targetSymbols {
		targetByID[sym.ID] = sym
	}
	fileRoots := map[string][]string{}
	for _, file := range parsed {
		fileRoots[file.RelPath] = file.Roots
	}
	for _, ref := range refs {
		target, ok := targetByID[ref.TargetSymbolID]
		if !ok {
			continue
		}
		srcSym, ok := sourceByID[ref.SourceSymbolID]
		if !ok {
			continue
		}
		roots := fileRoots[srcSym.FilePath]
		absFile, _, ok := limitedCandidateFile(repoRoot, target.FilePath, settings, rules)
		if ok {
			expanded.add(absFile, "neighbor", roots...)
		}
	}
}

func callerLocationKey(filePath string, line int, name string) string {
	return filepath.Clean(filePath) + ":" + fmt.Sprint(line) + ":" + name
}

func (s *Scanner) prepareLimitedBaselineWorktree(repoRoot string, repositoryID int64, dataDir, head string, result *ScanResult) string {
	if strings.TrimSpace(dataDir) == "" || strings.TrimSpace(head) == "" {
		return ""
	}
	target := filepath.Join(dataDir, "watch-worktrees", fmt.Sprint(repositoryID), head)
	if err := tldgit.EnsureDetachedWorktree(repoRoot, head, target); err != nil {
		return "limited view: baseline worktree unavailable, using tree-sitter-only baseline fallback: " + err.Error()
	}
	if result != nil {
		result.BaselineWorktree = target
	}
	return ""
}

func (s *Scanner) deleteLimitedRemovedFiles(ctx context.Context, repoRoot string, repositoryID int64) error {
	changes, err := tldgit.WorktreeChangesAgainstHead(repoRoot)
	if err != nil {
		logError(ctx, s.Logger, "watch.scan.delete_removed.failed", err, "repository_id", repositoryID)
		changes = map[string]tldgit.WorktreeChange{}
	}
	committed, err := gitChangesSinceLatestWatchVersion(ctx, s.Store, repositoryID, repoRoot)
	if err != nil {
		logError(ctx, s.Logger, "watch.scan.delete_committed_removed.failed", err, "repository_id", repositoryID)
	} else {
		maps.Copy(changes, committed)
	}
	var deleted []string
	for rel, change := range changes {
		if change == tldgit.WorktreeDeleted {
			deleted = append(deleted, rel)
		}
	}
	return s.Store.DeleteFilesByPath(ctx, repositoryID, deleted)
}

type parsedFile struct {
	File File
	Refs []analyzer.Ref
}

type scanFileResult struct {
	RelPath     string
	File        File
	Refs        []analyzer.Ref
	Parsed      bool
	Skipped     bool
	SymbolsSeen int
	Warnings    []string
}

type scanCache struct {
	filesByPath            map[string]File
	currentEnrichmentPaths map[string]struct{}
	blobHashesByPath       map[string]string
}

func (s *Scanner) loadScanCache(ctx context.Context, repositoryID int64, force bool, progress ProgressSink) (scanCache, error) {
	if force {
		return scanCache{}, nil
	}
	progressStart(progress, "Loading cached scan state", 2)
	defer progressFinish(progress)
	filesByPath, err := s.Store.CachedFilesByPath(ctx, repositoryID)
	if err != nil {
		return scanCache{}, err
	}
	progressAdvance(progress, "Cached file metadata loaded")
	currentEnrichmentPaths, err := s.Store.CurrentEnrichmentVersionPaths(ctx, repositoryID, enrichmentVersion)
	if err != nil {
		return scanCache{}, err
	}
	progressAdvance(progress, "Cached enrichment state loaded")
	return scanCache{filesByPath: filesByPath, currentEnrichmentPaths: currentEnrichmentPaths}, nil
}

func (c scanCache) cachedFile(rel string) (File, bool) {
	if c.filesByPath == nil {
		return File{}, false
	}
	file, ok := c.filesByPath[rel]
	return file, ok
}

func (c scanCache) hasCurrentEnrichment(rel string) bool {
	if c.currentEnrichmentPaths == nil {
		return false
	}
	_, ok := c.currentEnrichmentPaths[rel]
	return ok
}

func (c scanCache) blobHash(repoRoot, rel string) string {
	if c.blobHashesByPath != nil {
		return c.blobHashesByPath[rel]
	}
	return detectString(func() (string, error) { return tldgit.FileBlobHash(repoRoot, rel) })
}

func (s *Scanner) loadBlobHashes(ctx context.Context, repoRoot string) map[string]string {
	hashes, err := tldgit.FileBlobHashes(repoRoot)
	if err != nil {
		logError(ctx, s.Logger, "watch.scan.git_blob_hashes.failed", err, "repo_root", repoRoot)
		return nil
	}
	logInfo(ctx, s.Logger, "watch.scan.git_blob_hashes.completed", "repo_root", repoRoot, "files", len(hashes))
	return hashes
}

func (s *Scanner) scanFiles(ctx context.Context, repositoryID int64, repoRoot string, files []string, workers int, progress ProgressSink, force bool, rules *ignore.Rules, repoSignals []enrich.ActivationSignal, cache scanCache, missingIdentities []storedSymbolIdentity) ([]scanFileResult, error) {
	if workers <= 0 {
		workers = 1
	}
	if workers > len(files) && len(files) > 0 {
		workers = len(files)
	}
	jobs := make(chan string)
	results := make(chan scanFileResult, len(files))
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Go(func() {
			workerAnalyzer := analyzer.NewService()
			for {
				select {
				case <-ctx.Done():
					return
				case absFile, ok := <-jobs:
					if !ok {
						return
					}
					fileResult, err := s.scanFile(ctx, workerAnalyzer, repositoryID, repoRoot, absFile, progress, force, rules, repoSignals, cache, missingIdentities)
					if err != nil {
						select {
						case errs <- err:
						default:
						}
						continue
					}
					results <- fileResult
				}
			}
		})
	}
	for _, file := range files {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(results)
			return nil, ctx.Err()
		case jobs <- file:
		case err := <-errs:
			close(jobs)
			wg.Wait()
			close(results)
			return nil, err
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	select {
	case err := <-errs:
		return nil, err
	default:
	}
	out := make([]scanFileResult, 0, len(files))
	for result := range results {
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}

func (s *Scanner) scanFile(ctx context.Context, workerAnalyzer analyzer.Service, repositoryID int64, repoRoot, absFile string, progress ProgressSink, force bool, rules *ignore.Rules, repoSignals []enrich.ActivationSignal, cache scanCache, missingIdentities []storedSymbolIdentity) (scanFileResult, error) {
	started := time.Now()
	rel, err := filepath.Rel(repoRoot, absFile)
	if err != nil {
		return scanFileResult{}, err
	}
	rel = filepath.ToSlash(rel)
	defer progressAdvance(progress, rel)
	result := scanFileResult{RelPath: rel}
	languageName, parseable, ok := watchedFileLanguage(absFile)
	if !ok {
		result.Skipped = true
		s.logScanFile(ctx, repositoryID, result, "", "unsupported", started, nil)
		return result, nil
	}
	info, err := os.Stat(absFile)
	if err != nil {
		file, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, "", "", 0, 0, "error", err)
		if upsertErr != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", started, upsertErr)
			return result, upsertErr
		}
		result.File = file
		s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
		return result, nil
	}
	if info.Size() > maxSourceFileBytes {
		result.Skipped = true
		s.logScanFile(ctx, repositoryID, result, languageName, "oversized", started, nil)
		return result, nil
	}
	data, err := os.ReadFile(absFile)
	if err != nil {
		_, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, "", "", info.Size(), info.ModTime().UnixNano(), "error", err)
		if upsertErr != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", started, upsertErr)
		} else {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
		}
		return result, upsertErr
	}
	worktreeHash := hashBytes(data)
	if cached, ok := cache.cachedFile(rel); !force && ok && cached.WorktreeHash == worktreeHash && cached.ScanStatus != "error" {
		result.File = cached
		decision := "skipped"
		if !cache.hasCurrentEnrichment(rel) {
			if err := s.backfillFactsForCachedFile(ctx, workerAnalyzer, repositoryID, repoRoot, rel, absFile, languageName, parseable, rules, repoSignals, cached, data, &result); err != nil {
				s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
				return result, err
			}
			decision = "skipped_backfilled"
		}
		result.Skipped = true
		s.logScanFile(ctx, repositoryID, result, languageName, decision, started, nil)
		return result, nil
	}
	blobHash := cache.blobHash(repoRoot, rel)
	file, skipped, err := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, blobHash, worktreeHash, info.Size(), info.ModTime().UnixNano(), "parsed", nil)
	if err != nil {
		s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
		return result, err
	}
	result.File = file
	if !force && skipped {
		if err := s.backfillFactsForCachedFile(ctx, workerAnalyzer, repositoryID, repoRoot, rel, absFile, languageName, parseable, rules, repoSignals, file, data, &result); err != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
			return result, err
		}
		result.Skipped = true
		s.logScanFile(ctx, repositoryID, result, languageName, "skipped_backfilled", started, nil)
		return result, nil
	}
	if !parseable {
		if err := s.enrichFile(ctx, repositoryID, file.ID, repoRoot, rel, absFile, languageName, data, nil, repoSignals, &result); err != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
			return result, err
		}
		s.logScanFile(ctx, repositoryID, result, languageName, "metadata", started, nil)
		return result, nil
	}
	var extracted *analyzer.Result
	s.cacheMu.Lock()
	var cached *analyzer.Result
	var exists bool
	if s.expansionCache != nil {
		cached, exists = s.expansionCache[absFile]
	}
	if exists {
		extracted = cached
		s.cacheMu.Unlock()
	} else {
		s.cacheMu.Unlock()
		var err error
		extracted, err = workerAnalyzer.ExtractPath(ctx, absFile, rules, nil)
		if err == nil {
			s.cacheMu.Lock()
			if s.expansionCache == nil {
				s.expansionCache = make(map[string]*analyzer.Result)
			}
			s.expansionCache[absFile] = extracted
			s.cacheMu.Unlock()
		} else {
			_, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, blobHash, worktreeHash, info.Size(), info.ModTime().UnixNano(), "error", err)
			if upsertErr != nil {
				s.logScanFile(ctx, repositoryID, result, languageName, "error", started, upsertErr)
			} else {
				s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
			}
			return result, upsertErr
		}
	}
	symbols := watchSymbolsFromAnalyzer(repositoryID, file.ID, rel, languageName, data, extracted.Symbols)
	if err := s.Store.ReplaceFileSymbolsWithMissingCandidates(ctx, repositoryID, file.ID, symbols, missingIdentities); err != nil {
		s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
		return result, err
	}
	if err := s.enrichFile(ctx, repositoryID, file.ID, repoRoot, rel, absFile, languageName, data, extracted, repoSignals, &result); err != nil {
		s.logScanFile(ctx, repositoryID, result, languageName, "error", started, err)
		return result, err
	}
	result.Parsed = true
	result.SymbolsSeen = len(symbols)
	result.Refs = extracted.Refs
	s.logScanFile(ctx, repositoryID, result, languageName, "parsed", started, nil)
	return result, nil
}

func (s *Scanner) logScanFile(ctx context.Context, repositoryID int64, result scanFileResult, language, decision string, started time.Time, err error) {
	if s == nil || s.Logger == nil {
		return
	}
	fields := []any{
		"repository_id", repositoryID,
		"path", result.RelPath,
		"language", language,
		"decision", decision,
		"file_id", result.File.ID,
		"parsed", result.Parsed,
		"skipped", result.Skipped,
		"symbols", result.SymbolsSeen,
		"references", len(result.Refs),
		"warnings", len(result.Warnings),
		"elapsed", logElapsed(started),
	}
	if err != nil {
		logError(ctx, s.Logger, "watch.scan.file.failed", err, fields...)
		return
	}
	logInfo(ctx, s.Logger, "watch.scan.file", fields...)
}

func (s *Scanner) backfillFactsForCachedFile(ctx context.Context, workerAnalyzer analyzer.Service, repositoryID int64, repoRoot, rel, absFile, language string, parseable bool, rules *ignore.Rules, repoSignals []enrich.ActivationSignal, file File, data []byte, result *scanFileResult) error {
	version, err := s.Store.FactVersionForFile(ctx, repositoryID, file.ID, enrichmentVersionEnricher, enrichmentVersionStableKey(rel))
	if err != nil {
		return err
	}
	if version == enrichmentVersion {
		return nil
	}
	if data == nil {
		data, err = os.ReadFile(absFile)
		if err != nil {
			return err
		}
	}
	var extracted *analyzer.Result
	if parseable {
		s.cacheMu.Lock()
		var cached *analyzer.Result
		var exists bool
		if s.expansionCache != nil {
			cached, exists = s.expansionCache[absFile]
		}
		if exists {
			extracted = cached
			s.cacheMu.Unlock()
		} else {
			s.cacheMu.Unlock()
			extracted, err = workerAnalyzer.ExtractPath(ctx, absFile, rules, nil)
			if err != nil {
				return err
			}
			s.cacheMu.Lock()
			if s.expansionCache == nil {
				s.expansionCache = make(map[string]*analyzer.Result)
			}
			s.expansionCache[absFile] = extracted
			s.cacheMu.Unlock()
		}
	}
	return s.enrichFile(ctx, repositoryID, file.ID, repoRoot, rel, absFile, language, data, extracted, repoSignals, result)
}

func (s *Scanner) enrichFile(ctx context.Context, repositoryID, fileID int64, repoRoot, rel, absFile, language string, data []byte, extracted *analyzer.Result, repoSignals []enrich.ActivationSignal, result *scanFileResult) error {
	s.ensureDefaultEnrichers(NormalizeSettings(s.Settings))
	signals := append([]enrich.ActivationSignal{}, repoSignals...)
	if extracted != nil {
		signals = append(signals, enrich.ImportSignals(extracted.Refs)...)
	}
	facts, warnings, err := s.Enrichers.EnrichFile(ctx, enrich.FileInput{
		RepoRoot: repoRoot,
		AbsPath:  absFile,
		RelPath:  rel,
		Language: language,
		Source:   data,
		Parsed:   extracted,
		Signals:  signals,
	})
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		if warning.Message != "" {
			result.Warnings = append(result.Warnings, warning.Enricher+": "+warning.Message)
		}
	}
	watchFacts := watchFactsFromEnrich(repositoryID, fileID, rel, facts)
	watchFacts = append(watchFacts, enrichmentVersionFact(repositoryID, fileID, rel))
	return s.Store.ReplaceFactsForFile(ctx, repositoryID, fileID, watchFacts)
}

func (s *Scanner) ensureDefaultEnrichers(settings Settings) {
	if s.Enrichers != nil {
		return
	}
	s.Enrichers = defaults.NewRegistryWithDependencyInventory(settings.Dependencies.Enabled)
}

func watchedFileLanguage(path string) (language string, parseable bool, ok bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return "", false, false
	}
	if language, ok := analyzer.DetectLanguage(path); ok {
		return string(language), true, true
	}
	switch strings.ToLower(filepath.Base(path)) {
	case "go.mod":
		return "go-mod", false, true
	case "package.json", "package-lock.json":
		return "json", false, true
	case "requirements.txt", "requirements.in":
		return "python-requirements", false, true
	case "build.gradle", "settings.gradle":
		return "gradle", false, true
	case "cartservice.csproj":
		return "xml", false, true
	default:
		switch strings.ToLower(filepath.Ext(path)) {
		case ".cs":
			return "c-sharp", false, true
		case ".yaml", ".yml":
			return "yaml", false, true
		case ".proto":
			return "protobuf", false, true
		case ".tf":
			return "terraform", false, true
		case ".csproj":
			return "xml", false, true
		default:
			return "", false, false
		}
	}
}

func watchFactsFromEnrich(repositoryID, fileID int64, relPath string, facts []enrich.Fact) []Fact {
	out := make([]Fact, 0, len(facts))
	for _, fact := range facts {
		filePath := strings.TrimSpace(fact.Source.FilePath)
		if filePath == "" {
			filePath = relPath
		}
		subjectKind := strings.TrimSpace(fact.Subject.Kind)
		if subjectKind == "" {
			subjectKind = "file"
		}
		subjectKey := strings.TrimSpace(fact.Subject.StableKey)
		if subjectKey == "" {
			subjectKey = "file:" + relPath
		}
		endLine := fact.Source.EndLine
		var endPtr *int
		if endLine > 0 {
			endPtr = &endLine
		}
		attrs, _ := json.Marshal(fact.Attributes)
		hints, _ := json.Marshal(fact.VisibilityHints)
		raw, _ := json.Marshal(fact)
		watchFact := Fact{
			RepositoryID:        repositoryID,
			FileID:              fileID,
			FilePath:            filePath,
			StableKey:           fact.StableKey,
			Type:                fact.Type,
			Enricher:            fact.Enricher,
			SubjectKind:         subjectKind,
			SubjectStableKey:    subjectKey,
			ObjectKind:          strings.TrimSpace(fact.Object.Kind),
			ObjectStableKey:     strings.TrimSpace(fact.Object.StableKey),
			ObjectFilePath:      strings.TrimSpace(fact.Object.FilePath),
			ObjectName:          strings.TrimSpace(fact.Object.Name),
			Relationship:        strings.TrimSpace(fact.Relationship),
			StartLine:           fact.Source.StartLine,
			EndLine:             endPtr,
			Confidence:          fact.Confidence,
			Name:                fact.Name,
			Tags:                append([]string{}, fact.Tags...),
			AttributesJSON:      string(attrs),
			VisibilityHintsJSON: string(hints),
			RawJSON:             string(raw),
		}
		watchFact.FactHash = stableHash(struct {
			Type            string             `json:"type"`
			StableKey       string             `json:"stable_key"`
			Enricher        string             `json:"enricher"`
			Subject         string             `json:"subject"`
			ObjectKind      string             `json:"object_kind,omitempty"`
			ObjectStableKey string             `json:"object_stable_key,omitempty"`
			ObjectFilePath  string             `json:"object_file_path,omitempty"`
			ObjectName      string             `json:"object_name,omitempty"`
			Relationship    string             `json:"relationship,omitempty"`
			FilePath        string             `json:"file_path"`
			StartLine       int                `json:"start_line"`
			EndLine         *int               `json:"end_line,omitempty"`
			Confidence      float64            `json:"confidence"`
			Name            string             `json:"name"`
			Tags            []string           `json:"tags"`
			Attributes      map[string]string  `json:"attributes"`
			VisibilityHints map[string]float64 `json:"visibility_hints,omitempty"`
		}{
			Type:            watchFact.Type,
			StableKey:       watchFact.StableKey,
			Enricher:        watchFact.Enricher,
			Subject:         watchFact.SubjectKind + ":" + watchFact.SubjectStableKey,
			ObjectKind:      watchFact.ObjectKind,
			ObjectStableKey: watchFact.ObjectStableKey,
			ObjectFilePath:  watchFact.ObjectFilePath,
			ObjectName:      watchFact.ObjectName,
			Relationship:    watchFact.Relationship,
			FilePath:        watchFact.FilePath,
			StartLine:       watchFact.StartLine,
			EndLine:         watchFact.EndLine,
			Confidence:      watchFact.Confidence,
			Name:            watchFact.Name,
			Tags:            watchFact.Tags,
			Attributes:      fact.Attributes,
			VisibilityHints: fact.VisibilityHints,
		})
		out = append(out, watchFact)
	}
	return out
}

func enrichmentVersionStableKey(relPath string) string {
	return "watch.enrichment.version:" + relPath
}

func enrichmentVersionFact(repositoryID, fileID int64, relPath string) Fact {
	fact := Fact{
		RepositoryID:        repositoryID,
		FileID:              fileID,
		FilePath:            relPath,
		StableKey:           enrichmentVersionStableKey(relPath),
		Type:                enrichmentVersionType,
		Enricher:            enrichmentVersionEnricher,
		SubjectKind:         "file",
		SubjectStableKey:    "file:" + relPath,
		StartLine:           1,
		Confidence:          1,
		Name:                enrichmentVersion,
		AttributesJSON:      `{"version":"` + enrichmentVersion + `"}`,
		VisibilityHintsJSON: `{}`,
		RawJSON:             `{"version":"` + enrichmentVersion + `"}`,
	}
	fact.FactHash = stableHash(struct {
		Type      string `json:"type"`
		StableKey string `json:"stable_key"`
		Enricher  string `json:"enricher"`
		Version   string `json:"version"`
	}{
		Type:      fact.Type,
		StableKey: fact.StableKey,
		Enricher:  fact.Enricher,
		Version:   enrichmentVersion,
	})
	return fact
}

func (s *Scanner) collectSourceFiles(root string, workers int, languages []string, rules *ignore.Rules, progress ProgressSink) ([]string, error) {
	var files []string
	if rules == nil {
		rules = &ignore.Rules{}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > len(entries) && len(entries) > 0 {
		workers = len(entries)
	}
	progressStart(progress, "Discovering source files", len(entries))
	jobs := make(chan string)
	results := make(chan []string, len(entries))
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Go(func() {
			for entryPath := range jobs {
				found, err := s.collectSourceFilesUnder(root, entryPath, rules, languages)
				progressAdvance(progress, filepath.ToSlash(mustRel(root, entryPath)))
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					continue
				}
				results <- found
			}
		})
	}
	for _, entry := range entries {
		select {
		case jobs <- filepath.Join(root, entry.Name()):
		case err := <-errs:
			close(jobs)
			wg.Wait()
			close(results)
			return nil, err
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	select {
	case err := <-errs:
		return nil, err
	default:
	}
	for result := range results {
		files = append(files, result...)
	}
	sort.Strings(files)
	return files, nil
}

func (s *Scanner) collectSourceFilesUnder(root, start string, rules *ignore.Rules, languages []string) ([]string, error) {
	var files []string
	allowed := map[string]struct{}{}
	for _, language := range NormalizeSettings(Settings{Languages: languages}).Languages {
		allowed[language] = struct{}{}
	}
	err := filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rules.ShouldIgnorePath(rel) || isHiddenBuildOutput(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		language, parseable, ok := watchedFileLanguage(path)
		if !ok || (parseable && !languageAllowed(language, allowed)) {
			return nil
		}
		if rules.ShouldIgnorePath(rel) {
			return nil
		}
		if parseable && language == string(analyzer.LanguageGo) {
			generated, err := isGeneratedGoFile(path)
			if err != nil {
				return nil
			}
			if generated {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func watchSymbolsFromAnalyzer(repositoryID, fileID int64, relPath, language string, source []byte, symbols []analyzer.Symbol) []Symbol {
	out := make([]Symbol, 0, len(symbols))
	lines := strings.Split(string(source), "\n")
	baseKeyCounts := make(map[string]int, len(symbols))
	for _, sym := range symbols {
		baseKeyCounts[watchSymbolStableKey(language, relPath, sym)]++
	}
	baseKeySeen := make(map[string]int, len(baseKeyCounts))
	for _, sym := range symbols {
		qualified := watchSymbolQualifiedName(sym)
		stableKey := watchSymbolStableKey(language, relPath, sym)
		if baseKeyCounts[stableKey] > 1 {
			baseKeySeen[stableKey]++
			stableKey = fmt.Sprintf("%s:line:%d:ordinal:%d", stableKey, sym.Line, baseKeySeen[stableKey])
		}
		endLine := sym.EndLine
		if endLine <= 0 {
			endLine = sym.Line
		}
		raw, _ := json.Marshal(sym)
		endPtr := endLine
		body := lineRange(lines, sym.Line, endLine)
		out = append(out, Symbol{
			RepositoryID:  repositoryID,
			FileID:        fileID,
			FilePath:      relPath,
			StableKey:     stableKey,
			Name:          sym.Name,
			QualifiedName: qualified,
			Kind:          sym.Kind,
			NodeType:      sym.NodeType,
			StartLine:     sym.Line,
			EndLine:       &endPtr,
			SignatureHash: hashString(fmt.Sprintf("%s:%s", sym.Kind, qualified)),
			ContentHash:   hashString(normalizeSymbolContent(body, sym.Name, qualified)),
			RawJSON:       string(raw),
		})
	}
	return out
}

func watchSymbolQualifiedName(sym analyzer.Symbol) string {
	if sym.Parent == "" {
		return sym.Name
	}
	return sym.Parent + "." + sym.Name
}

func watchSymbolStableKey(language, relPath string, sym analyzer.Symbol) string {
	return fmt.Sprintf("%s:%s:%s:%s", language, relPath, sym.Kind, watchSymbolQualifiedName(sym))
}

func normalizeSymbolContent(body, name, qualified string) string {
	body = strings.TrimSpace(outdentCode(body))
	replacements := []string{name}
	if leaf := pathBaseQualifier(qualified); leaf != "" && leaf != name {
		replacements = append(replacements, leaf)
	}
	for _, replacement := range replacements {
		if replacement == "" {
			continue
		}
		body = strings.ReplaceAll(body, replacement, "__symbol__")
	}
	return body
}

func (s *Scanner) Close() error {
	s.resolverMu.Lock()
	defer s.resolverMu.Unlock()
	if s.resolver != nil {
		err := s.resolver.Close()
		s.resolver = nil
		s.resolverRepoRoot = ""
		return err
	}
	return nil
}

func (s *Scanner) getOrCreateResolver(repoRoot string, settings Settings) definitionResolver {
	s.resolverMu.Lock()
	defer s.resolverMu.Unlock()
	if s.resolver != nil && s.resolverRepoRoot != repoRoot {
		_ = s.resolver.Close()
		s.resolver = nil
	}
	if s.resolver == nil {
		factory := s.resolverFactory
		if factory == nil {
			factory = func(rootDir string) definitionResolver {
				return analyzerlsp.NewMultiLanguageResolverWithConfig(rootDir, analyzerlsp.ResolverConfig{
					Enabled:          settings.LSP.Enabled,
					HealthInterval:   settings.LSP.HealthInterval,
					MemoryLimitBytes: settings.LSP.MemoryLimitBytes,
					Commands:         analyzerlsp.NormalizeOverrideCommands(settings.LSP.Commands),
					Logger:           s.Logger,
				})
			}
		}
		s.resolver = factory(repoRoot)
		s.resolverRepoRoot = repoRoot
	}
	return s.resolver
}

func (s *Scanner) resolveReferences(ctx context.Context, repoRoot string, repositoryID int64, files []parsedFile, settings Settings, progress ProgressSink) ([]Reference, string, error) {
	symbols, err := s.Store.SymbolsForRepository(ctx, repositoryID)
	if err != nil {
		return nil, "", err
	}
	byName := make(map[string][]Symbol)
	byFile := make(map[int64][]Symbol)
	for _, sym := range symbols {
		byName[sym.Name] = append(byName[sym.Name], sym)
		byFile[sym.FileID] = append(byFile[sym.FileID], sym)
	}
	for fileID := range byFile {
		sort.Slice(byFile[fileID], func(i, j int) bool {
			return byFile[fileID][i].StartLine > byFile[fileID][j].StartLine
		})
	}

	resolver := s.getOrCreateResolver(repoRoot, settings)

	var refs []Reference
	for _, file := range files {
		progressAdvance(progress, file.File.Path)
		for _, parsedRef := range file.Refs {
			if parsedRef.Kind != "" && parsedRef.Kind != "call" {
				continue
			}
			target, ok := resolveTargetSymbol(ctx, s, resolver, repoRoot, parsedRef, byName, symbols)
			if !ok {
				continue
			}
			source, ok := enclosingSymbol(byFile[file.File.ID], parsedRef.Line)
			if !ok || source.ID == target.ID {
				continue
			}
			raw, _ := json.Marshal(parsedRef)
			kind := parsedRef.Kind
			if kind == "" {
				kind = "call"
			}
			refs = append(refs, Reference{
				RepositoryID:   repositoryID,
				SourceSymbolID: source.ID,
				TargetSymbolID: target.ID,
				SourceFileID:   file.File.ID,
				Kind:           kind,
				Line:           parsedRef.Line,
				Column:         parsedRef.Column,
				EvidenceHash:   hashString(fmt.Sprintf("%d:%d:%s:%s", parsedRef.Line, parsedRef.Column, kind, parsedRef.Name)),
				RawJSON:        string(raw),
			})
		}
	}
	return refs, "", nil
}

func resolveTargetSymbol(ctx context.Context, s *Scanner, resolver definitionResolver, repoRoot string, ref analyzer.Ref, byName map[string][]Symbol, symbols []Symbol) (Symbol, bool) {
	if resolver != nil {
		var locations []analyzerlsp.DefinitionLocation
		var exists bool
		if s != nil {
			key := ref.FilePath + ":" + fmt.Sprint(ref.Line) + ":" + fmt.Sprint(ref.Column)
			s.cacheMu.Lock()
			if s.locationsCache != nil {
				locations, exists = s.locationsCache[key]
			}
			s.cacheMu.Unlock()

			if !exists {
				var err error
				locations, err = resolver.ResolveDefinitions(ctx, ref)
				if err == nil {
					s.cacheMu.Lock()
					if s.locationsCache == nil {
						s.locationsCache = make(map[string][]analyzerlsp.DefinitionLocation)
					}
					s.locationsCache[key] = locations
					s.cacheMu.Unlock()
				}
			}
		} else {
			locations, _ = resolver.ResolveDefinitions(ctx, ref)
		}

		if len(locations) > 0 {
			for _, location := range locations {
				if sym, ok := symbolAtLocation(repoRoot, symbols, location); ok {
					return sym, true
				}
			}
		}
	}
	targets := byName[ref.Name]
	if ref.FilePath == "" || len(targets) != 1 {
		return Symbol{}, false
	}
	refRel := filepath.ToSlash(filepath.Clean(ref.FilePath))
	if filepath.IsAbs(ref.FilePath) {
		var err error
		refRel, err = filepath.Rel(repoRoot, ref.FilePath)
		if err != nil {
			return Symbol{}, false
		}
		refRel = filepath.ToSlash(refRel)
	}
	var sameFileTargets []Symbol
	for _, target := range targets {
		if target.FilePath == refRel {
			sameFileTargets = append(sameFileTargets, target)
		}
	}
	if len(sameFileTargets) == 1 {
		return sameFileTargets[0], true
	}
	if strings.EqualFold(filepath.Ext(refRel), ".go") {
		return targets[0], true
	}
	return Symbol{}, false
}

func symbolAtLocation(repoRoot string, symbols []Symbol, location analyzerlsp.DefinitionLocation) (Symbol, bool) {
	rel, err := filepath.Rel(repoRoot, location.FilePath)
	if err != nil {
		return Symbol{}, false
	}
	rel = filepath.ToSlash(rel)
	var best Symbol
	found := false
	for _, sym := range symbols {
		if sym.FilePath != rel {
			continue
		}
		end := math.MaxInt
		if sym.EndLine != nil {
			end = *sym.EndLine
		}
		if sym.StartLine <= location.Line && end >= location.Line {
			if !found || sym.StartLine > best.StartLine {
				best = sym
				found = true
			}
		}
	}
	return best, found
}

type lspStatusProvider interface {
	Snapshot() analyzerlsp.StatusSnapshot
}

func InitialLSPStatus(settings Settings) LSPStatus {
	settings = NormalizeSettings(settings)
	languages := configuredLSPLanguages(settings.Languages)
	snapshot := analyzerlsp.SnapshotLanguages(languages, analyzerlsp.ResolverConfig{
		Enabled:          settings.LSP.Enabled,
		HealthInterval:   settings.LSP.HealthInterval,
		MemoryLimitBytes: settings.LSP.MemoryLimitBytes,
		Commands:         analyzerlsp.NormalizeOverrideCommands(settings.LSP.Commands),
	})
	return convertLSPStatus(snapshot)
}

func (s *Scanner) currentLSPStatus(settings Settings) LSPStatus {
	if s == nil {
		return InitialLSPStatus(settings)
	}
	s.resolverMu.Lock()
	resolver := s.resolver
	s.resolverMu.Unlock()
	if provider, ok := resolver.(lspStatusProvider); ok {
		return convertLSPStatus(provider.Snapshot())
	}
	return InitialLSPStatus(settings)
}

func configuredLSPLanguages(values []string) []analyzer.Language {
	languages := make([]analyzer.Language, 0, len(values))
	for _, value := range values {
		language := analyzer.Language(strings.ToLower(strings.TrimSpace(value)))
		if len(analyzerlsp.DefaultCommands(language)) == 0 {
			continue
		}
		languages = append(languages, language)
	}
	return languages
}

func convertLSPStatus(snapshot analyzerlsp.StatusSnapshot) LSPStatus {
	status := LSPStatus{
		Enabled:               snapshot.Enabled,
		HealthIntervalSeconds: snapshot.HealthIntervalSeconds,
		MemoryLimitBytes:      snapshot.MemoryLimitBytes,
		MemoryMonitoring:      snapshot.MemoryMonitoring,
		Servers:               make([]LSPServerStatus, 0, len(snapshot.Servers)),
	}
	for _, server := range snapshot.Servers {
		converted := LSPServerStatus{
			Language:        server.Language,
			Command:         server.Command,
			CommandSource:   server.CommandSource,
			Path:            server.Path,
			State:           server.State,
			PID:             server.PID,
			ServerName:      server.ServerName,
			ServerVersion:   server.ServerVersion,
			Definition:      server.Definition,
			MemoryBytes:     server.MemoryBytes,
			RestartCount:    server.RestartCount,
			LastHealthcheck: server.LastHealthcheck,
			LastError:       server.LastError,
		}
		status.Servers = append(status.Servers, converted)
		status.Summary.Requested++
		if converted.Path != "" {
			status.Summary.Available++
		}
		switch converted.State {
		case analyzerlsp.StateActive:
			status.Summary.Active++
		case analyzerlsp.StateUnavailable:
			status.Summary.Unavailable++
		case analyzerlsp.StateFailed:
			status.Summary.Failed++
		}
		if converted.State == analyzerlsp.StateMemoryLimited || strings.Contains(strings.ToLower(converted.LastError), "exceeded limit") {
			status.Summary.MemoryLimited++
			if converted.State != analyzerlsp.StateActive {
				status.Summary.Failed++
			}
		}
		if converted.RestartCount > 0 {
			status.Summary.Restarted++
		}
	}
	return status
}

func lspWarnings(status LSPStatus) []string {
	if !status.Enabled || status.Summary.Requested == 0 {
		return nil
	}
	var warnings []string
	if status.Summary.Unavailable > 0 {
		warnings = append(warnings, fmt.Sprintf("LSP unavailable for %d requested language(s); falling back to conservative name matching", status.Summary.Unavailable))
	}
	if status.Summary.Failed > 0 {
		warnings = append(warnings, fmt.Sprintf("LSP failed for %d requested language(s); see log file for details", status.Summary.Failed))
	}
	if status.Summary.MemoryLimited > 0 {
		warnings = append(warnings, fmt.Sprintf("LSP memory limit exceeded for %d language server(s); restarted on demand", status.Summary.MemoryLimited))
	}
	return warnings
}

func LSPNeedsConfirmation(status LSPStatus) bool {
	if !status.Enabled || status.Summary.Requested == 0 {
		return false
	}
	return status.Summary.Unavailable+status.Summary.Failed+status.Summary.MemoryLimited > 0
}

func LSPDegradedServers(status LSPStatus) []LSPServerStatus {
	var servers []LSPServerStatus
	for _, server := range status.Servers {
		if server.State == analyzerlsp.StateUnavailable || server.State == analyzerlsp.StateFailed || server.State == analyzerlsp.StateMemoryLimited {
			servers = append(servers, server)
		}
	}
	return servers
}

func enclosingSymbol(symbols []Symbol, line int) (Symbol, bool) {
	for _, sym := range symbols {
		end := math.MaxInt
		if sym.EndLine != nil {
			end = *sym.EndLine
		}
		if sym.StartLine <= line && end >= line {
			return sym, true
		}
	}
	return Symbol{}, false
}

func detectString(fn func() (string, error)) string {
	value, err := fn()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashString(value string) string {
	return hashBytes([]byte(value))
}

func lineRange(lines []string, start, end int) string {
	if start <= 0 {
		start = 1
	}
	if end < start {
		end = start
	}
	if start > len(lines) {
		return ""
	}
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n")
}

func isGeneratedGoFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 8192)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	lines := strings.SplitN(string(buf[:n]), "\n", 21)
	for _, line := range lines {
		if strings.Contains(line, "Code generated") && strings.Contains(line, "DO NOT EDIT") {
			return true, nil
		}
	}
	return false, nil
}

func isHiddenBuildOutput(name string) bool {
	if name == "" || name == "." {
		return false
	}
	if strings.HasPrefix(name, ".") {
		switch name {
		case ".git", ".cache", ".next", ".tld", ".turbo":
			return true
		}
	}
	switch name {
	case "dist", "build", "out", "tmp":
		return true
	default:
		return false
	}
}
