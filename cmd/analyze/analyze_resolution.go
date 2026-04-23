package analyze

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mertcikla/tld/internal/analyzer"
	analyzerlsp "github.com/mertcikla/tld/internal/analyzer/lsp"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type analyzeDefinitionLocation struct {
	FilePath string
	Line     int
}

type analyzeDefinitionResolver interface {
	ResolveDefinitions(ctx context.Context, ref analyzer.Ref) ([]analyzeDefinitionLocation, error)
	Close() error
}

type analyzeLSPResolver struct {
	rootDir  string
	sessions map[analyzer.Language]*analyzerlsp.Session
	disabled map[analyzer.Language]struct{}
	opened   map[string]struct{}
	contents map[string]string
}

func newAnalyzeLSPResolver(rootDir string) *analyzeLSPResolver {
	return &analyzeLSPResolver{
		rootDir:  rootDir,
		sessions: make(map[analyzer.Language]*analyzerlsp.Session),
		disabled: make(map[analyzer.Language]struct{}),
		opened:   make(map[string]struct{}),
		contents: make(map[string]string),
	}
}

func (r *analyzeLSPResolver) ResolveDefinitions(ctx context.Context, ref analyzer.Ref) ([]analyzeDefinitionLocation, error) {
	if r == nil || r.rootDir == "" || ref.FilePath == "" || ref.Line <= 0 {
		return nil, nil
	}
	language, ok := analyzer.DetectLanguage(ref.FilePath)
	if !ok {
		return nil, nil
	}
	session, ok, err := r.sessionForLanguage(ctx, language)
	if err != nil || !ok {
		return nil, err
	}
	if err := r.openDocument(ctx, session, ref.FilePath); err != nil {
		return nil, err
	}
	column := 0
	if ref.Column > 0 {
		column = ref.Column - 1
	}
	locations, err := session.Definition(ctx, &protocol.DefinitionParams{
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
	resolved := make([]analyzeDefinitionLocation, 0, len(locations))
	for _, location := range locations {
		filePath := filepath.Clean(location.URI.Filename())
		if filePath == "" {
			continue
		}
		resolved = append(resolved, analyzeDefinitionLocation{
			FilePath: filePath,
			Line:     int(location.Range.Start.Line) + 1,
		})
	}
	return resolved, nil
}

func (r *analyzeLSPResolver) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	for _, session := range r.sessions {
		if session == nil {
			continue
		}
		if err := session.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *analyzeLSPResolver) sessionForLanguage(ctx context.Context, language analyzer.Language) (*analyzerlsp.Session, bool, error) {
	if r == nil {
		return nil, false, nil
	}
	if session, ok := r.sessions[language]; ok {
		return session, true, nil
	}
	if _, disabled := r.disabled[language]; disabled {
		return nil, false, nil
	}
	session, err := analyzerlsp.StartSession(ctx, analyzerlsp.SessionConfig{
		Language: language,
		RootDir:  r.rootDir,
	})
	if err != nil {
		r.disabled[language] = struct{}{}
		return nil, false, nil
	}
	if !session.SupportsDefinition() {
		r.disabled[language] = struct{}{}
		_ = session.Close()
		return nil, false, nil
	}
	r.sessions[language] = session
	return session, true, nil
}

func (r *analyzeLSPResolver) openDocument(ctx context.Context, session *analyzerlsp.Session, filePath string) error {
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
	if err := session.OpenDocument(ctx, cleanPath, content); err != nil {
		return fmt.Errorf("open %s in language server: %w", cleanPath, err)
	}
	r.opened[cleanPath] = struct{}{}
	return nil
}

func resolveAnalyzeTargetRef(ctx context.Context, resolver analyzeDefinitionResolver, ref analyzer.Ref, symbols []analyzer.Symbol, refBySymbol map[analyzeElementLookupKey]string, refsByName map[string][]string) string {
	if resolver != nil {
		locations, err := resolver.ResolveDefinitions(ctx, ref)
		if err == nil {
			for _, location := range locations {
				symbol, ok := symbolByFileAndLine(location.FilePath, location.Line, symbols)
				if !ok {
					continue
				}
				if targetRef, ok := refBySymbol[analyzeSymbolLookupKey(symbol)]; ok {
					return targetRef
				}
			}
		}
	}
	candidates := refsByName[ref.Name]
	if len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

func symbolByFileAndLine(filePath string, line int, symbols []analyzer.Symbol) (analyzer.Symbol, bool) {
	var bestSymbol analyzer.Symbol
	found := false
	cleanFilePath := filepath.Clean(filePath)
	for _, symbol := range symbols {
		if filepath.Clean(symbol.FilePath) != cleanFilePath {
			continue
		}
		if symbol.Line <= line && (symbol.EndLine == 0 || symbol.EndLine >= line) {
			if !found || symbol.Line > bestSymbol.Line {
				bestSymbol = symbol
				found = true
			}
		}
	}
	return bestSymbol, found
}
