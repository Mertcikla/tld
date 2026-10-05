// Package symbolcheck reports whether a source file declares a named symbol.
// It replaces the legacy analyzer's per-language tree-sitter check using the
// same pure-Go gotreesitter engine as the codeindex indexer, so no cgo is
// required. Languages without a registered grammar are reported as unsupported,
// and callers skip them silently.
package symbolcheck

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// ErrUnsupportedLanguage is returned when no grammar handles a file's language.
var ErrUnsupportedLanguage = errors.New("unsupported language")

// IsUnsupported reports whether err indicates an unsupported language.
func IsUnsupported(err error) bool {
	return errors.Is(err, ErrUnsupportedLanguage)
}

// HasSymbol parses filePath and reports whether a declaration named symbol is
// present.
func HasSymbol(ctx context.Context, filePath, symbol string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	name := grammarForPath(filePath)
	if name == "" {
		return false, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, filepath.Ext(filePath))
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		return false, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, name)
	}
	lang := entry.Language()
	if lang == nil {
		return false, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, name)
	}
	source, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return false, err
	}
	if tree == nil {
		return false, nil
	}
	defer tree.Release()
	return findIdentifier(tree.RootNode(), lang, source, symbol), nil
}

func findIdentifier(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, symbol string) bool {
	if node == nil {
		return false
	}
	if strings.HasSuffix(node.Type(lang), "identifier") && node.Text(source) == symbol {
		return true
	}
	for i := 0; i < node.ChildCount(); i++ {
		if findIdentifier(node.Child(i), lang, source, symbol) {
			return true
		}
	}
	return false
}

func grammarForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".mts", ".cts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".py":
		return "python"
	default:
		return ""
	}
}
