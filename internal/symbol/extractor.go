package symbol

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mertcikla/tld/internal/ignore"
	"github.com/mertcikla/tld/internal/symbol/grammars"
)

// ExtractFile extracts symbols and refs from a single source file.
// Returns ErrUnsupportedLanguage if the file extension is not supported.
func ExtractFile(ctx context.Context, path string) (*Result, error) {
	wasm, err := grammarFor(path)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	result, err := runGrammar(ctx, wasm, src)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", path, err)
	}
	// Annotate with file path
	for i := range result.Symbols {
		result.Symbols[i].FilePath = path
	}
	for i := range result.Refs {
		result.Refs[i].FilePath = path
	}
	return result, nil
}

// ExtractSource extracts symbols and refs from in-memory source with the given
// file extension (e.g. ".go", ".ts", ".py").  Used for testing.
func ExtractSource(ctx context.Context, ext string, src []byte) (*Result, error) {
	wasm, err := grammarForExt(ext)
	if err != nil {
		return nil, err
	}
	return runGrammar(ctx, wasm, src)
}

// ExtractDir walks root (recursively) and extracts symbols from every supported
// source file, filtering via ignore rules.  All results are merged into one.
func ExtractDir(ctx context.Context, root string, rules *ignore.Rules) (*Result, error) {
	return ExtractDirWithProgress(ctx, root, rules, nil)
}

// ExtractDirWithProgress behaves like ExtractDir and invokes onEntry for every
// non-ignored directory and file visited while walking root.
func ExtractDirWithProgress(ctx context.Context, root string, rules *ignore.Rules, onEntry func(path string, isDir bool)) (*Result, error) {
	merged := &Result{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Relative path for ignore matching
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
		if rules.ShouldIgnorePath(path) {
			return nil
		}
		if onEntry != nil {
			onEntry(path, false)
		}
		if _, err := grammarFor(path); err != nil {
			return nil // unsupported skip silently
		}
		result, err := ExtractFile(ctx, path)
		if err != nil {
			return nil // skip files that fail to parse
		}
		merged.Symbols = append(merged.Symbols, result.Symbols...)
		merged.Refs = append(merged.Refs, result.Refs...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return merged, nil
}

// HasSymbol reports whether the named symbol exists in a source file.
func HasSymbol(ctx context.Context, filePath, symbolName string) (bool, error) {
	result, err := ExtractFile(ctx, filePath)
	if err != nil {
		return false, err
	}
	for _, s := range result.Symbols {
		if s.Name == symbolName {
			return true, nil
		}
	}
	return false, nil
}

// ErrUnsupportedLanguage is returned when no grammar module supports the file extension.
type ErrUnsupportedLanguage struct {
	Ext string
}

func (e ErrUnsupportedLanguage) Error() string {
	return fmt.Sprintf("unsupported language for extension %q", e.Ext)
}

// grammarFor returns the WASM bytes for the grammar matching path's extension.
func grammarFor(path string) ([]byte, error) {
	return grammarForExt(filepath.Ext(path))
}

func grammarForExt(ext string) ([]byte, error) {
	switch ext {
	case ".go":
		return grammars.Go, nil
	case ".ts", ".tsx":
		return grammars.TypeScript, nil
	case ".js", ".jsx", ".mjs", ".cjs":
		return grammars.JavaScript, nil
	case ".py":
		return grammars.Python, nil
	default:
		return nil, ErrUnsupportedLanguage{Ext: ext}
	}
}
