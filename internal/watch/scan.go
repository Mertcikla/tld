package watch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mertcikla/tld/internal/analyzer"
	analyzerlsp "github.com/mertcikla/tld/internal/analyzer/lsp"
	tldgit "github.com/mertcikla/tld/internal/git"
	"github.com/mertcikla/tld/internal/ignore"
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/defaults"
)

const (
	enrichmentVersion         = "watch-enrich-v2"
	enrichmentVersionEnricher = "watch.enrichment"
	enrichmentVersionType     = "watch.enrichment.version"
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

	resolver         *analyzerlsp.MultiLanguageResolver
	resolverMu       sync.Mutex
	resolverRepoRoot string
}

type synchronizedProgress struct {
	sink ProgressSink
	mu   sync.Mutex
}

func (p *synchronizedProgress) Start(label string, total int) {
	if p.sink == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
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
	p.sink.Finish()
}

func NewScanner(store *Store) *Scanner {
	return &Scanner{
		Store:     store,
		Analyzer:  analyzer.NewService(),
		Enrichers: defaults.NewRegistry(),
		Rules:     &ignore.Rules{},
	}
}

func (s *Scanner) Scan(ctx context.Context, path string) (ScanResult, error) {
	return s.ScanWithOptions(ctx, path, ScanOptions{})
}

type ScanOptions struct {
	Force   bool
	DataDir string
}

func (s *Scanner) ScanFilesWithOptions(ctx context.Context, repo Repository, relFiles []string, opts ScanOptions) (ScanResult, error) {
	if s == nil || s.Store == nil {
		return ScanResult{}, fmt.Errorf("watch scanner requires a store")
	}
	if s.Analyzer == nil {
		s.Analyzer = analyzer.NewService()
	}
	if s.Enrichers == nil {
		s.Enrichers = defaults.NewRegistry()
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
	result := ScanResult{RepositoryID: repo.ID, FilesSeen: len(files)}
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
		_ = s.Store.FinishScanRun(context.Background(), runID, status, result, scanErr)
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
	progressStart(progress, "Scanning context files", len(files))
	defer progressFinish(progress)
	fileResults, err := s.scanFiles(ctx, repo.ID, repoRoot, files, workers, progress, opts.Force, effectiveRules, repoSignals, cache)
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
	progressFinish(progress)
	resolveStarted := time.Now()
	logInfo(ctx, s.Logger, "watch.scan.references.started", "repository_id", repo.ID, "files", len(parsedFiles))
	progressStart(progress, "Resolving code references", len(parsedFiles))
	refs, warning, err := s.resolveReferences(ctx, repoRoot, repo.ID, parsedFiles, progress)
	progressFinish(progress)
	if err != nil {
		scanErr = err
		logError(ctx, s.Logger, "watch.scan.references.failed", err, "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID)
		return result, err
	}
	logInfo(ctx, s.Logger, "watch.scan.references.completed", "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID, "references", len(refs), "warning", warning)
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
	if s.Analyzer == nil {
		s.Analyzer = analyzer.NewService()
	}
	if s.Enrichers == nil {
		s.Enrichers = defaults.NewRegistry()
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
	result := ScanResult{RepositoryID: repo.ID}
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
		_ = s.Store.FinishScanRun(context.Background(), runID, status, result, scanErr)
	}()

	workers := runtime.NumCPU()
	progress := &synchronizedProgress{sink: s.Progress}
	var files []string
	if plan.Limited {
		files = append(files, plan.Files...)
		changed, err := tldgit.WorktreeChangesAgainstHead(repoRoot)
		if err != nil {
			result.Warnings = append(result.Warnings, "limited view: git change detection failed: "+err.Error())
		} else {
			files = append(files, changedScanFiles(repoRoot, changed, settings, effectiveRules)...)
		}
		files = uniqueAbsFiles(files)
		logInfo(ctx, s.Logger, "watch.scan.source_discovery.completed", "repository_id", repo.ID, "files", len(files), "tracked_files", result.TrackedFiles, "selected_files", result.SelectedFiles, "mode", result.Mode, "strategy", result.Strategy)
	} else {
		discoveryStarted := time.Now()
		logInfo(ctx, s.Logger, "watch.scan.source_discovery.started", "repository_id", repo.ID, "repo_root", repoRoot, "languages", strings.Join(settings.Languages, ","))
		files, err = s.collectSourceFiles(repoRoot, workers, settings.Languages, effectiveRules, progress)
		progressFinish(progress)
		if err != nil {
			scanErr = err
			logError(ctx, s.Logger, "watch.scan.source_discovery.failed", err, "elapsed", logElapsed(discoveryStarted), "repository_id", repo.ID)
			return result, err
		}
		logInfo(ctx, s.Logger, "watch.scan.source_discovery.completed", "elapsed", logElapsed(discoveryStarted), "repository_id", repo.ID, "files", len(files), "mode", result.Mode, "strategy", result.Strategy)
	}
	if err := ctx.Err(); err != nil {
		scanErr = err
		return result, err
	}
	repoSignals := enrich.DiscoverRepositorySignalsFromFiles(repoRoot, files)
	result.FilesSeen = len(files)
	if plan.Limited {
		result.SelectedFiles = len(files)
		result.SkippedTrackedFiles = max(result.TrackedFiles-len(files), 0)
	}
	cache, err := s.loadScanCache(ctx, repo.ID, opts.Force, progress)
	if err != nil {
		scanErr = err
		return result, err
	}
	progressStart(progress, "Scanning source files", len(files))
	defer progressFinish(progress)
	seen := make(map[string]struct{}, len(files))
	var parsedFiles []parsedFile
	var parsedFileIDs []int64

	fileResults, err := s.scanFiles(ctx, repo.ID, repoRoot, files, workers, progress, opts.Force, effectiveRules, repoSignals, cache)
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
	if !plan.Limited {
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

	progressFinish(progress)
	resolveStarted := time.Now()
	logInfo(ctx, s.Logger, "watch.scan.references.started", "repository_id", repo.ID, "files", len(parsedFiles))
	progressStart(progress, "Resolving code references", len(parsedFiles))
	refs, warning, err := s.resolveReferences(ctx, repoRoot, repo.ID, parsedFiles, progress)
	progressFinish(progress)
	if err != nil {
		scanErr = err
		logError(ctx, s.Logger, "watch.scan.references.failed", err, "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID)
		return result, err
	}
	logInfo(ctx, s.Logger, "watch.scan.references.completed", "elapsed", logElapsed(resolveStarted), "repository_id", repo.ID, "references", len(refs), "warning", warning)
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

func uniqueAbsFiles(files []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = filepath.Clean(file)
		if file == "" {
			continue
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		out = append(out, file)
	}
	sort.Strings(out)
	return out
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
		return nil
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

func (s *Scanner) scanFiles(ctx context.Context, repositoryID int64, repoRoot string, files []string, workers int, progress ProgressSink, force bool, rules *ignore.Rules, repoSignals []enrich.ActivationSignal, cache scanCache) ([]scanFileResult, error) {
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
					fileResult, err := s.scanFile(ctx, workerAnalyzer, repositoryID, repoRoot, absFile, progress, force, rules, repoSignals, cache)
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

func (s *Scanner) scanFile(ctx context.Context, workerAnalyzer analyzer.Service, repositoryID int64, repoRoot, absFile string, progress ProgressSink, force bool, rules *ignore.Rules, repoSignals []enrich.ActivationSignal, cache scanCache) (scanFileResult, error) {
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
		s.logScanFile(ctx, repositoryID, result, "", "unsupported", nil)
		return result, nil
	}
	info, err := os.Stat(absFile)
	if err != nil {
		file, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, "", "", 0, 0, "error", err)
		if upsertErr != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", upsertErr)
			return result, upsertErr
		}
		result.File = file
		s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
		return result, nil
	}
	if cached, ok := cache.cachedFile(rel); !force && ok && cached.SizeBytes == info.Size() && cached.MtimeUnix == info.ModTime().UnixNano() && cached.WorktreeHash != "" && cached.ScanStatus != "error" {
		result.File = cached
		decision := "skipped"
		if !cache.hasCurrentEnrichment(rel) {
			if err := s.backfillFactsForCachedFile(ctx, workerAnalyzer, repositoryID, repoRoot, rel, absFile, languageName, parseable, rules, repoSignals, cached, nil, &result); err != nil {
				s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
				return result, err
			}
			decision = "skipped_backfilled"
		}
		result.Skipped = true
		s.logScanFile(ctx, repositoryID, result, languageName, decision, nil)
		return result, nil
	}
	data, err := os.ReadFile(absFile)
	if err != nil {
		_, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, "", "", info.Size(), info.ModTime().UnixNano(), "error", err)
		if upsertErr != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", upsertErr)
		} else {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
		}
		return result, upsertErr
	}
	worktreeHash := hashBytes(data)
	blobHash := detectString(func() (string, error) { return tldgit.FileBlobHash(repoRoot, rel) })
	file, skipped, err := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, blobHash, worktreeHash, info.Size(), info.ModTime().UnixNano(), "parsed", nil)
	if err != nil {
		s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
		return result, err
	}
	result.File = file
	if !force && skipped {
		if err := s.backfillFactsForCachedFile(ctx, workerAnalyzer, repositoryID, repoRoot, rel, absFile, languageName, parseable, rules, repoSignals, file, data, &result); err != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
			return result, err
		}
		result.Skipped = true
		s.logScanFile(ctx, repositoryID, result, languageName, "skipped_backfilled", nil)
		return result, nil
	}
	if !parseable {
		if err := s.enrichFile(ctx, repositoryID, file.ID, repoRoot, rel, absFile, languageName, data, nil, repoSignals, &result); err != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
			return result, err
		}
		s.logScanFile(ctx, repositoryID, result, languageName, "metadata", nil)
		return result, nil
	}
	extracted, err := workerAnalyzer.ExtractPath(ctx, absFile, rules, nil)
	if err != nil {
		_, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, blobHash, worktreeHash, info.Size(), info.ModTime().UnixNano(), "error", err)
		if upsertErr != nil {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", upsertErr)
		} else {
			s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
		}
		return result, upsertErr
	}
	symbols := watchSymbolsFromAnalyzer(repositoryID, file.ID, rel, languageName, data, extracted.Symbols)
	if err := s.Store.ReplaceFileSymbols(ctx, repositoryID, file.ID, symbols); err != nil {
		s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
		return result, err
	}
	if err := s.enrichFile(ctx, repositoryID, file.ID, repoRoot, rel, absFile, languageName, data, extracted, repoSignals, &result); err != nil {
		s.logScanFile(ctx, repositoryID, result, languageName, "error", err)
		return result, err
	}
	result.Parsed = true
	result.SymbolsSeen = len(symbols)
	result.Refs = extracted.Refs
	s.logScanFile(ctx, repositoryID, result, languageName, "parsed", nil)
	return result, nil
}

func (s *Scanner) logScanFile(ctx context.Context, repositoryID int64, result scanFileResult, language, decision string, err error) {
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
		extracted, err = workerAnalyzer.ExtractPath(ctx, absFile, rules, nil)
		if err != nil {
			return err
		}
	}
	return s.enrichFile(ctx, repositoryID, file.ID, repoRoot, rel, absFile, language, data, extracted, repoSignals, result)
}

func (s *Scanner) enrichFile(ctx context.Context, repositoryID, fileID int64, repoRoot, rel, absFile, language string, data []byte, extracted *analyzer.Result, repoSignals []enrich.ActivationSignal, result *scanFileResult) error {
	if s.Enrichers == nil {
		s.Enrichers = defaults.NewRegistry()
	}
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

func watchedFileLanguage(path string) (language string, parseable bool, ok bool) {
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
			StartLine:     sym.Line,
			EndLine:       &endPtr,
			SignatureHash: hashString(fmt.Sprintf("%s:%s:%d", sym.Kind, qualified, sym.Line)),
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

func (s *Scanner) getOrCreateResolver(repoRoot string) *analyzerlsp.MultiLanguageResolver {
	s.resolverMu.Lock()
	defer s.resolverMu.Unlock()
	if s.resolver != nil && s.resolverRepoRoot != repoRoot {
		_ = s.resolver.Close()
		s.resolver = nil
	}
	if s.resolver == nil {
		s.resolver = analyzerlsp.NewMultiLanguageResolver(repoRoot)
		s.resolverRepoRoot = repoRoot
	}
	return s.resolver
}

func (s *Scanner) resolveReferences(ctx context.Context, repoRoot string, repositoryID int64, files []parsedFile, progress ProgressSink) ([]Reference, string, error) {
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

	resolver := s.getOrCreateResolver(repoRoot)

	var refs []Reference
	for _, file := range files {
		progressAdvance(progress, file.File.Path)
		for _, parsedRef := range file.Refs {
			if parsedRef.Kind != "" && parsedRef.Kind != "call" {
				continue
			}
			target, ok := resolveTargetSymbol(ctx, resolver, repoRoot, parsedRef, byName, symbols)
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

func resolveTargetSymbol(ctx context.Context, resolver *analyzerlsp.MultiLanguageResolver, repoRoot string, ref analyzer.Ref, byName map[string][]Symbol, symbols []Symbol) (Symbol, bool) {
	if resolver != nil {
		locations, err := resolver.ResolveDefinitions(ctx, ref)
		if err == nil {
			for _, location := range locations {
				if sym, ok := symbolAtLocation(repoRoot, symbols, definitionLocation{FilePath: location.FilePath, Line: location.Line}); ok {
					return sym, true
				}
			}
		}
	}
	targets := byName[ref.Name]
	if len(targets) != 1 {
		return Symbol{}, false
	}
	return targets[0], true
}

type definitionLocation struct {
	FilePath string
	Line     int
}

func symbolAtLocation(repoRoot string, symbols []Symbol, location definitionLocation) (Symbol, bool) {
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
		end := sym.StartLine
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

func enclosingSymbol(symbols []Symbol, line int) (Symbol, bool) {
	for _, sym := range symbols {
		end := sym.StartLine
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
