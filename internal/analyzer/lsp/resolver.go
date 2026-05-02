package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mertcikla/tld/internal/analyzer"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type DefinitionLocation struct {
	FilePath string
	Line     int
}

type MultiLanguageResolver struct {
	RootDir  string
	sessions map[analyzer.Language]*Session
	disabled map[analyzer.Language]struct{}
	opened   map[string]struct{}
	contents map[string]string
}

func NewMultiLanguageResolver(rootDir string) *MultiLanguageResolver {
	return &MultiLanguageResolver{
		RootDir:  rootDir,
		sessions: make(map[analyzer.Language]*Session),
		disabled: make(map[analyzer.Language]struct{}),
		opened:   make(map[string]struct{}),
		contents: make(map[string]string),
	}
}

func (r *MultiLanguageResolver) ResolveDefinitions(ctx context.Context, ref analyzer.Ref) ([]DefinitionLocation, error) {
	if r == nil || r.RootDir == "" || ref.FilePath == "" || ref.Line <= 0 {
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
	resolved := make([]DefinitionLocation, 0, len(locations))
	for _, location := range locations {
		filePath := filepath.Clean(location.URI.Filename())
		if filePath == "" {
			continue
		}
		resolved = append(resolved, DefinitionLocation{
			FilePath: filePath,
			Line:     int(location.Range.Start.Line) + 1,
		})
	}
	return resolved, nil
}

func (r *MultiLanguageResolver) Close() error {
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

func (r *MultiLanguageResolver) sessionForLanguage(ctx context.Context, language analyzer.Language) (*Session, bool, error) {
	if r == nil {
		return nil, false, nil
	}
	if session, ok := r.sessions[language]; ok {
		return session, true, nil
	}
	if _, disabled := r.disabled[language]; disabled {
		return nil, false, nil
	}
	session, err := StartSession(ctx, SessionConfig{
		Language: language,
		RootDir:  r.RootDir,
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

func (r *MultiLanguageResolver) openDocument(ctx context.Context, session *Session, filePath string) error {
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
