package analyze

import (
	"context"
	"path/filepath"

	"github.com/mertcikla/tld/internal/analyzer"
	analyzerlsp "github.com/mertcikla/tld/internal/analyzer/lsp"
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
	inner *analyzerlsp.MultiLanguageResolver
}

func newAnalyzeLSPResolver(rootDir string) *analyzeLSPResolver {
	return &analyzeLSPResolver{
		inner: analyzerlsp.NewMultiLanguageResolver(rootDir),
	}
}

func (r *analyzeLSPResolver) ResolveDefinitions(ctx context.Context, ref analyzer.Ref) ([]analyzeDefinitionLocation, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	locations, err := r.inner.ResolveDefinitions(ctx, ref)
	if err != nil {
		return nil, err
	}
	resolved := make([]analyzeDefinitionLocation, 0, len(locations))
	for _, location := range locations {
		resolved = append(resolved, analyzeDefinitionLocation{
			FilePath: location.FilePath,
			Line:     location.Line,
		})
	}
	return resolved, nil
}

func (r *analyzeLSPResolver) Close() error {
	if r == nil || r.inner == nil {
		return nil
	}
	return r.inner.Close()
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
