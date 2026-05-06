package analyzer

import (
	"context"
	"path/filepath"
)

type fileParser interface {
	ParseFile(ctx context.Context, path string, source []byte) (*Result, error)
}

type parserRegistry struct {
	parsers map[Language]fileParser
}

func newDefaultParserRegistry() *parserRegistry {
	return &parserRegistry{
		parsers: map[Language]fileParser{
			LanguageC:          &cppParser{},
			LanguageCPP:        &cppParser{},
			LanguageGo:         &goParser{},
			LanguageJava:       &javaParser{},
			LanguagePython:     &pythonParser{},
			LanguageRust:       &rustParser{},
			LanguageTypeScript: &tsParser{},
			LanguageJavaScript: &jsParser{},
		},
	}
}

func (r *parserRegistry) parserForPath(path string) (Language, fileParser, bool) {
	if r == nil {
		return "", nil, false
	}
	language, ok := DetectLanguage(path)
	if !ok {
		return "", nil, false
	}
	parser, ok := r.parsers[language]
	return language, parser, ok
}

func unsupportedLanguageError(path string, language Language) error {
	return ErrUnsupportedLanguage{
		Path:     path,
		Ext:      filepath.Ext(path),
		Language: language,
	}
}
