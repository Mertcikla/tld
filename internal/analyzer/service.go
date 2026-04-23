package analyzer

import (
	"context"
	"errors"
	"os"

	"github.com/mertcikla/diag/tld/internal/ignore"
	"github.com/mertcikla/diag/tld/internal/symbol"
)

type Service interface {
	ExtractPath(ctx context.Context, path string, rules *ignore.Rules, onEntry func(path string, isDir bool)) (*Result, error)
	HasSymbol(ctx context.Context, filePath, symbolName string) (bool, error)
}

type LegacyService struct{}

var defaultService Service = NewService()

func NewLegacyService() *LegacyService {
	return &LegacyService{}
}

func DefaultService() Service {
	return defaultService
}

func HasSymbol(ctx context.Context, filePath, symbolName string) (bool, error) {
	return defaultService.HasSymbol(ctx, filePath, symbolName)
}

func IsUnsupportedLanguage(err error) bool {
	var analyzerUnsupported ErrUnsupportedLanguage
	if errors.As(err, &analyzerUnsupported) {
		return true
	}
	var unsupported symbol.ErrUnsupportedLanguage
	return errors.As(err, &unsupported)
}

func (s *LegacyService) ExtractPath(ctx context.Context, path string, rules *ignore.Rules, onEntry func(path string, isDir bool)) (*Result, error) {
	result, err := legacyExtractPath(ctx, path, rules, onEntry)
	if err != nil {
		return nil, err
	}
	return resultFromLegacy(result), nil
}

func (s *LegacyService) HasSymbol(ctx context.Context, filePath, symbolName string) (bool, error) {
	return symbol.HasSymbol(ctx, filePath, symbolName)
}

func legacyExtractPath(ctx context.Context, path string, rules *ignore.Rules, onEntry func(path string, isDir bool)) (*symbol.Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return symbol.ExtractDirWithProgress(ctx, path, rules, onEntry)
	}
	if rules.ShouldIgnoreFile(path) {
		return &symbol.Result{}, nil
	}
	if onEntry != nil {
		onEntry(path, false)
	}
	return symbol.ExtractFile(ctx, path)
}
