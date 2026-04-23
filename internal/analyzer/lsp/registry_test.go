package lsp

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/analyzer"
)

func TestResolveServerCommandWithLookPath(t *testing.T) {
	resolved, err := ResolveServerCommandWithLookPath(analyzer.LanguageGo, func(file string) (string, error) {
		if file == "gopls" {
			return filepath.Join("/tmp", "gopls"), nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("ResolveServerCommandWithLookPath: %v", err)
	}
	if resolved.Path != filepath.Join("/tmp", "gopls") {
		t.Fatalf("resolved path = %q", resolved.Path)
	}
	if len(resolved.Args) != 0 {
		t.Fatalf("expected no args, got %#v", resolved.Args)
	}
}

func TestResolveServerCommandWithLookPath_MissingServer(t *testing.T) {
	_, err := ResolveServerCommandWithLookPath(analyzer.LanguagePython, func(file string) (string, error) {
		return "", errors.New("missing")
	})
	var notFound ErrServerNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ErrServerNotFound, got %T: %v", err, err)
	}
	if got := err.Error(); !strings.Contains(got, "pyright-langserver --stdio") {
		t.Fatalf("missing candidate in error message: %q", got)
	}
}

func TestResolveServerCommandWithLookPath_UnconfiguredLanguage(t *testing.T) {
	_, err := ResolveServerCommandWithLookPath(analyzer.Language("ruby"), func(file string) (string, error) {
		return filepath.Join("/tmp", file), nil
	})
	var notConfigured ErrServerNotConfigured
	if !errors.As(err, &notConfigured) {
		t.Fatalf("expected ErrServerNotConfigured, got %T: %v", err, err)
	}
}
