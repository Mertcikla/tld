package watch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
}

func NewScanner(store *Store) *Scanner {
	return &Scanner{
		Store:    store,
		Analyzer: analyzer.NewService(),
		Rules:    &ignore.Rules{},
	}
}

func (s *Scanner) Scan(ctx context.Context, path string) (ScanResult, error) {
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

	repoInput := RepositoryInput{
		RemoteURL:    detectString(func() (string, error) { return tldgit.DetectRemoteURL(repoRoot) }),
		RepoRoot:     repoRoot,
		DisplayName:  filepath.Base(repoRoot),
		Branch:       detectString(func() (string, error) { return tldgit.DetectBranch(repoRoot) }),
		HeadCommit:   detectString(func() (string, error) { return tldgit.DetectHeadCommit(repoRoot) }),
		SettingsHash: SettingsHash,
	}
	repo, err := s.Store.EnsureRepository(ctx, repoInput)
	if err != nil {
		return ScanResult{}, err
	}
	result := ScanResult{RepositoryID: repo.ID}

	runID, err := s.Store.BeginScanRun(ctx, repo.ID, "full")
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

	files, err := s.collectGoFiles(repoRoot)
	if err != nil {
		scanErr = err
		return result, err
	}
	result.FilesSeen = len(files)
	seen := make(map[string]struct{}, len(files))
	var parsedFiles []parsedFile
	var parsedFileIDs []int64

	for _, absFile := range files {
		rel, err := filepath.Rel(repoRoot, absFile)
		if err != nil {
			scanErr = err
			return result, err
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = struct{}{}
		info, err := os.Stat(absFile)
		if err != nil {
			file, _, upsertErr := s.Store.UpsertFile(ctx, repo.ID, rel, "go", "", "", 0, 0, "error", err)
			if upsertErr != nil {
				scanErr = upsertErr
				return result, upsertErr
			}
			_ = file
			continue
		}
		data, err := os.ReadFile(absFile)
		if err != nil {
			_, _, upsertErr := s.Store.UpsertFile(ctx, repo.ID, rel, "go", "", "", info.Size(), info.ModTime().Unix(), "error", err)
			if upsertErr != nil {
				scanErr = upsertErr
				return result, upsertErr
			}
			continue
		}
		worktreeHash := hashBytes(data)
		blobHash := detectString(func() (string, error) { return tldgit.FileBlobHash(repoRoot, rel) })
		file, skipped, err := s.Store.UpsertFile(ctx, repo.ID, rel, "go", blobHash, worktreeHash, info.Size(), info.ModTime().Unix(), "parsed", nil)
		if err != nil {
			scanErr = err
			return result, err
		}
		if skipped {
			result.FilesSkipped++
			continue
		}
		extracted, err := s.Analyzer.ExtractPath(ctx, absFile, s.Rules, nil)
		if err != nil {
			_, _, upsertErr := s.Store.UpsertFile(ctx, repo.ID, rel, "go", blobHash, worktreeHash, info.Size(), info.ModTime().Unix(), "error", err)
			if upsertErr != nil {
				scanErr = upsertErr
				return result, upsertErr
			}
			continue
		}
		symbols := watchSymbolsFromAnalyzer(repo.ID, file.ID, rel, data, extracted.Symbols)
		if err := s.Store.ReplaceFileSymbols(ctx, repo.ID, file.ID, symbols); err != nil {
			scanErr = err
			return result, err
		}
		result.FilesParsed++
		result.SymbolsSeen += len(symbols)
		parsedFiles = append(parsedFiles, parsedFile{File: file, Refs: extracted.Refs})
		parsedFileIDs = append(parsedFileIDs, file.ID)
	}

	if err := s.Store.DeleteMissingFiles(ctx, repo.ID, seen); err != nil {
		scanErr = err
		return result, err
	}

	refs, warning, err := s.resolveReferences(ctx, repoRoot, repo.ID, parsedFiles)
	if err != nil {
		scanErr = err
		return result, err
	}
	result.Warning = warning
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

func (s *Scanner) collectGoFiles(root string) ([]string, error) {
	var files []string
	rules := s.Rules
	if rules == nil {
		rules = &ignore.Rules{}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if rules.ShouldIgnorePath(rel) {
			return nil
		}
		generated, err := isGeneratedGoFile(path)
		if err != nil {
			return nil
		}
		if generated {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)
	return files, err
}

func watchSymbolsFromAnalyzer(repositoryID, fileID int64, relPath string, source []byte, symbols []analyzer.Symbol) []Symbol {
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
			StableKey:     fmt.Sprintf("go:%s:%s:%s", relPath, sym.Kind, qualified),
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
	return refs, warning, nil
}

func resolveTargetSymbol(ctx context.Context, resolver *goDefinitionResolver, repoRoot string, ref analyzer.Ref, byName map[string][]Symbol, symbols []Symbol) (Symbol, bool) {
	if resolver != nil {
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
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := strings.SplitN(string(data), "\n", 21)
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
