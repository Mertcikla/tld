package analyzer

import (
	"fmt"
	"path/filepath"
)

type ErrUnsupportedLanguage struct {
	Path     string
	Ext      string
	Language Language
}

func (e ErrUnsupportedLanguage) Error() string {
	if e.Language != "" {
		return fmt.Sprintf("unsupported analyzer language %q for %q", e.Language, e.Path)
	}
	ext := e.Ext
	if ext == "" && e.Path != "" {
		ext = filepath.Ext(e.Path)
	}
	return fmt.Sprintf("unsupported analyzer language for extension %q", ext)
}
