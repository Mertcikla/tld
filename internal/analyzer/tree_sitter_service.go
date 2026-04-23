package analyzer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mertcikla/tld/internal/ignore"
	"github.com/mertcikla/tld/internal/symbol"
)

type TreeSitterService struct {
	fallback Service
	registry *parserRegistry
}

func NewService() *TreeSitterService {
	return &TreeSitterService{
		fallback: NewLegacyService(),
		registry: newDefaultParserRegistry(),
	}
}

func (s *TreeSitterService) ExtractPath(ctx context.Context, path string, rules *ignore.Rules, onEntry func(path string, isDir bool)) (*Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return s.extractDir(ctx, path, rules, onEntry)
	}
	if rules.ShouldIgnoreFile(path) {
		return &Result{}, nil
	}
	if onEntry != nil {
		onEntry(path, false)
	}
	return s.extractFile(ctx, path)
}

func (s *TreeSitterService) HasSymbol(ctx context.Context, filePath, symbolName string) (bool, error) {
	result, err := s.extractFile(ctx, filePath)
	if err != nil {
		return false, err
	}
	for _, sym := range result.Symbols {
		if sym.Name == symbolName {
			return true, nil
		}
	}
	return false, nil
}

func (s *TreeSitterService) extractDir(ctx context.Context, root string, rules *ignore.Rules, onEntry func(path string, isDir bool)) (*Result, error) {
	merged := &Result{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rules.ShouldIgnorePath(rel) || rules.ShouldIgnorePath(d.Name()) {
				return filepath.SkipDir
			}
			if onEntry != nil {
				onEntry(path, true)
			}
			return nil
		}
		if rules.ShouldIgnorePath(rel) || rules.ShouldIgnorePath(path) {
			return nil
		}
		if onEntry != nil {
			onEntry(path, false)
		}
		result, err := s.extractFile(ctx, path)
		if err != nil {
			return nil
		}
		mergeResult(merged, result)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return merged, nil
}

func (s *TreeSitterService) extractFile(ctx context.Context, path string) (*Result, error) {
	language, parser, ok := s.registry.parserForPath(path)
	if !ok {
		if s.fallback != nil {
			return s.fallback.ExtractPath(ctx, path, nil, nil)
		}
		if detectedLanguage, detected := DetectLanguage(path); detected {
			return nil, unsupportedLanguageError(path, detectedLanguage)
		}
		return nil, unsupportedLanguageError(path, language)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	result, err := parser.ParseFile(ctx, path, source)
	if err != nil {
		return nil, err
	}
	for i := range result.Symbols {
		result.Symbols[i].Technology = string(language)
	}
	return result, nil
}

func resultFromLegacy(result *symbol.Result) *Result {
	if result == nil {
		return &Result{}
	}
	converted := &Result{
		Symbols: make([]Symbol, 0, len(result.Symbols)),
		Refs:    make([]Ref, 0, len(result.Refs)),
	}
	for _, sym := range result.Symbols {
		tech := ""
		if lang, ok := DetectLanguage(sym.FilePath); ok {
			tech = string(lang)
		}
		converted.Symbols = append(converted.Symbols, Symbol{
			Name:       sym.Name,
			Kind:       sym.Kind,
			FilePath:   sym.FilePath,
			Line:       sym.Line,
			EndLine:    sym.EndLine,
			Parent:     sym.Parent,
			Technology: tech,
		})
	}
	for _, ref := range result.Refs {
		converted.Refs = append(converted.Refs, Ref{
			Name:     ref.Name,
			Kind:     "call",
			FilePath: ref.FilePath,
			Line:     ref.Line,
			Column:   0,
		})
	}
	return converted
}
