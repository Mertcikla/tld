package analyzer

import (
	"context"
	"errors"

	"github.com/mertcikla/tld/v2/internal/ignore"
)

type Service interface {
	ExtractPath(ctx context.Context, path string, rules *ignore.Rules, onEntry func(path string, isDir bool)) (*Result, error)
	HasSymbol(ctx context.Context, filePath, symbolName string) (bool, error)
}

var defaultService Service = NewService()

func DefaultService() Service {
	return defaultService
}

func HasSymbol(ctx context.Context, filePath, symbolName string) (bool, error) {
	return defaultService.HasSymbol(ctx, filePath, symbolName)
}

func IsUnsupportedLanguage(err error) bool {
	var analyzerUnsupported ErrUnsupportedLanguage
	return errors.As(err, &analyzerUnsupported)
}
