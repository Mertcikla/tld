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

	"github.com/mertcikla/tld/internal/analyzer"
	analyzerlsp "github.com/mertcikla/tld/internal/analyzer/lsp"
	tldgit "github.com/mertcikla/tld/internal/git"
	"github.com/mertcikla/tld/internal/ignore"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type Scanner struct {
	Store    *Store
	Analyzer analyzer.Service
	Rules    *ignore.Rules
	Progress ProgressSink
	Settings Settings
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
		Store:    store,
		Analyzer: analyzer.NewService(),
		Rules:    &ignore.Rules{},
	}
}

func (s *Scanner) Scan(ctx context.Context, path string) (ScanResult, error) {
	return s.ScanWithOptions(ctx, path, ScanOptions{})
}

type ScanOptions struct {
	Force bool
}

func (s *Scanner) ScanWithOptions(ctx context.Context, path string, opts ScanOptions) (ScanResult, error) {
	if s == nil || s.Store == nil {
		return ScanResult{}, fmt.Errorf("watch scanner requires a store")
	}
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

	mode := "incremental"
	if opts.Force {
		mode = "full"
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
	files, err := s.collectSourceFiles(repoRoot, workers, settings.Languages)
	if err != nil {
		scanErr = err
		return result, err
	}
	result.FilesSeen = len(files)
	progress := &synchronizedProgress{sink: s.Progress}
	progressStart(progress, "Scanning source files", len(files))
	defer progressFinish(progress)
	seen := make(map[string]struct{}, len(files))
	var parsedFiles []parsedFile
	var parsedFileIDs []int64

	fileResults, err := s.scanFiles(ctx, repo.ID, repoRoot, files, workers, progress, opts.Force)
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
	}

	if err := s.Store.DeleteMissingFiles(ctx, repo.ID, seen); err != nil {
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

	refs, warning, err := s.resolveReferences(ctx, repoRoot, repo.ID, parsedFiles)
	if err != nil {
		scanErr = err
		return result, err
	}
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
}

func (s *Scanner) scanFiles(ctx context.Context, repositoryID int64, repoRoot string, files []string, workers int, progress ProgressSink, force bool) ([]scanFileResult, error) {
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			workerAnalyzer := analyzer.NewService()
			for absFile := range jobs {
				fileResult, err := s.scanFile(ctx, workerAnalyzer, repositoryID, repoRoot, absFile, progress, force)
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					continue
				}
				results <- fileResult
			}
		}()
	}
	for _, file := range files {
		select {
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

func (s *Scanner) scanFile(ctx context.Context, workerAnalyzer analyzer.Service, repositoryID int64, repoRoot, absFile string, progress ProgressSink, force bool) (scanFileResult, error) {
	rel, err := filepath.Rel(repoRoot, absFile)
	if err != nil {
		return scanFileResult{}, err
	}
	rel = filepath.ToSlash(rel)
	defer progressAdvance(progress, rel)
	result := scanFileResult{RelPath: rel}
	language, ok := analyzer.DetectLanguage(absFile)
	if !ok {
		result.Skipped = true
		return result, nil
	}
	languageName := string(language)
	info, err := os.Stat(absFile)
	if err != nil {
		file, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, "", "", 0, 0, "error", err)
		if upsertErr != nil {
			return result, upsertErr
		}
		result.File = file
		return result, nil
	}
	if cached, ok, err := s.Store.CachedFileByPath(ctx, repositoryID, rel); err != nil {
		return result, err
	} else if !force && ok && cached.SizeBytes == info.Size() && cached.MtimeUnix == info.ModTime().Unix() && cached.WorktreeHash != "" && cached.ScanStatus != "error" {
		if _, _, err := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, nullStringValue(cached.GitBlobHash), cached.WorktreeHash, info.Size(), info.ModTime().Unix(), "parsed", nil); err != nil {
			return result, err
		}
		result.Skipped = true
		return result, nil
	}
	data, err := os.ReadFile(absFile)
	if err != nil {
		_, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, "", "", info.Size(), info.ModTime().Unix(), "error", err)
		return result, upsertErr
	}
	worktreeHash := hashBytes(data)
	blobHash := detectString(func() (string, error) { return tldgit.FileBlobHash(repoRoot, rel) })
	file, skipped, err := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, blobHash, worktreeHash, info.Size(), info.ModTime().Unix(), "parsed", nil)
	if err != nil {
		return result, err
	}
	result.File = file
	if !force && skipped {
		result.Skipped = true
		return result, nil
	}
	extracted, err := workerAnalyzer.ExtractPath(ctx, absFile, s.Rules, nil)
	if err != nil {
		_, _, upsertErr := s.Store.UpsertFile(ctx, repositoryID, rel, languageName, blobHash, worktreeHash, info.Size(), info.ModTime().Unix(), "error", err)
		return result, upsertErr
	}
	symbols := watchSymbolsFromAnalyzer(repositoryID, file.ID, rel, languageName, data, extracted.Symbols)
	if err := s.Store.ReplaceFileSymbols(ctx, repositoryID, file.ID, symbols); err != nil {
		return result, err
	}
	result.Parsed = true
	result.SymbolsSeen = len(symbols)
	result.Refs = extracted.Refs
	return result, nil
}

func (s *Scanner) collectSourceFiles(root string, workers int, languages []string) ([]string, error) {
	var files []string
	rules := s.Rules
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
	jobs := make(chan string)
	results := make(chan []string, len(entries))
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entryPath := range jobs {
				found, err := s.collectSourceFilesUnder(root, entryPath, rules, languages)
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					continue
				}
				results <- found
			}
		}()
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
		language, ok := analyzer.DetectLanguage(path)
		if !ok || !languageAllowed(string(language), allowed) {
			return nil
		}
		if rules.ShouldIgnorePath(rel) {
			return nil
		}
		if language == analyzer.LanguageGo {
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
	for _, sym := range symbols {
		qualified := sym.Name
		if sym.Parent != "" {
			qualified = sym.Parent + "." + sym.Name
		}
		endLine := sym.EndLine
		if endLine <= 0 {
			endLine = sym.Line
		}
		raw, _ := json.Marshal(sym)
		endPtr := endLine
		out = append(out, Symbol{
			RepositoryID:  repositoryID,
			FileID:        fileID,
			FilePath:      relPath,
			StableKey:     fmt.Sprintf("%s:%s:%s:%s", language, relPath, sym.Kind, qualified),
			Name:          sym.Name,
			QualifiedName: qualified,
			Kind:          sym.Kind,
			StartLine:     sym.Line,
			EndLine:       &endPtr,
			SignatureHash: hashString(fmt.Sprintf("%s:%s:%d", sym.Kind, qualified, sym.Line)),
			ContentHash:   hashString(lineRange(lines, sym.Line, endLine)),
			RawJSON:       string(raw),
		})
	}
	return out
}

func (s *Scanner) resolveReferences(ctx context.Context, repoRoot string, repositoryID int64, files []parsedFile) ([]Reference, string, error) {
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

	resolver, warning := newGoDefinitionResolver(ctx, repoRoot)
	if resolver != nil {
		defer func() { _ = resolver.Close() }()
	}

	var refs []Reference
	for _, file := range files {
		for _, parsedRef := range file.Refs {
			if parsedRef.Kind != "" && parsedRef.Kind != "call" {
				continue
			}
			target, ok := resolveTargetSymbol(ctx, resolver, repoRoot, file.File.Language, parsedRef, byName, symbols)
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
	return refs, warning, nil
}

func resolveTargetSymbol(ctx context.Context, resolver *goDefinitionResolver, repoRoot, sourceLanguage string, ref analyzer.Ref, byName map[string][]Symbol, symbols []Symbol) (Symbol, bool) {
	if resolver != nil && sourceLanguage == string(analyzer.LanguageGo) {
		locations, err := resolver.Resolve(ctx, ref)
		if err == nil {
			for _, location := range locations {
				if sym, ok := symbolAtLocation(repoRoot, symbols, location); ok {
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

type goDefinitionResolver struct {
	root     string
	session  *analyzerlsp.Session
	opened   map[string]struct{}
	contents map[string]string
}

func newGoDefinitionResolver(ctx context.Context, root string) (*goDefinitionResolver, string) {
	session, err := analyzerlsp.StartSession(ctx, analyzerlsp.SessionConfig{
		Language: analyzer.LanguageGo,
		RootDir:  root,
	})
	if err != nil {
		return nil, "gopls unavailable; references were resolved with parser-only local symbols"
	}
	if !session.SupportsDefinition() {
		_ = session.Close()
		return nil, "gopls does not support definition lookup; references were resolved with parser-only local symbols"
	}
	return &goDefinitionResolver{
		root:     root,
		session:  session,
		opened:   make(map[string]struct{}),
		contents: make(map[string]string),
	}, ""
}

func (r *goDefinitionResolver) Resolve(ctx context.Context, ref analyzer.Ref) ([]definitionLocation, error) {
	if r == nil || r.session == nil || ref.FilePath == "" || ref.Line <= 0 {
		return nil, nil
	}
	if err := r.openDocument(ctx, ref.FilePath); err != nil {
		return nil, err
	}
	column := 0
	if ref.Column > 0 {
		column = ref.Column - 1
	}
	locations, err := r.session.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(ref.FilePath)},
			Position: protocol.Position{
				Line:      uint32(ref.Line - 1),
				Character: uint32(column),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	out := make([]definitionLocation, 0, len(locations))
	for _, location := range locations {
		filePath := filepath.Clean(location.URI.Filename())
		if filePath == "" {
			continue
		}
		out = append(out, definitionLocation{
			FilePath: filePath,
			Line:     int(location.Range.Start.Line) + 1,
		})
	}
	return out, nil
}

func (r *goDefinitionResolver) openDocument(ctx context.Context, filePath string) error {
	cleanPath := filepath.Clean(filePath)
	if _, ok := r.opened[cleanPath]; ok {
		return nil
	}
	content, ok := r.contents[cleanPath]
	if !ok {
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", cleanPath, err)
		}
		content = string(data)
		r.contents[cleanPath] = content
	}
	if err := r.session.OpenDocument(ctx, cleanPath, content); err != nil {
		return fmt.Errorf("open %s in language server: %w", cleanPath, err)
	}
	r.opened[cleanPath] = struct{}{}
	return nil
}

func (r *goDefinitionResolver) Close() error {
	if r == nil || r.session == nil {
		return nil
	}
	return r.session.Close()
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
		case ".git", ".cache", ".next", ".turbo":
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
