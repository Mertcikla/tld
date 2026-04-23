package analyzer

import (
	"path/filepath"
	"sort"
	"strings"
)

type Language string

const (
	LanguageC          Language = "c"
	LanguageCPP        Language = "cpp"
	LanguageGo         Language = "go"
	LanguageJava       Language = "java"
	LanguageJavaScript Language = "javascript"
	LanguagePython     Language = "python"
	LanguageRust       Language = "rust"
	LanguageTypeScript Language = "typescript"
)

type LanguageSpec struct {
	Language   Language
	Extensions []string
}

var languageSpecs = []LanguageSpec{
	{Language: LanguageGo, Extensions: []string{".go"}},
	{Language: LanguagePython, Extensions: []string{".py"}},
	{Language: LanguageRust, Extensions: []string{".rs"}},
	{Language: LanguageJava, Extensions: []string{".java"}},
	{Language: LanguageTypeScript, Extensions: []string{".ts", ".tsx", ".mts", ".cts"}},
	{Language: LanguageJavaScript, Extensions: []string{".js", ".jsx", ".mjs", ".cjs"}},
	{Language: LanguageCPP, Extensions: []string{".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx"}},
	{Language: LanguageC, Extensions: []string{".c", ".h"}},
}

var languageByExt = buildLanguageByExtension()

func SupportedLanguages() []LanguageSpec {
	cloned := make([]LanguageSpec, 0, len(languageSpecs))
	for _, spec := range languageSpecs {
		cloned = append(cloned, LanguageSpec{
			Language:   spec.Language,
			Extensions: append([]string{}, spec.Extensions...),
		})
	}
	return cloned
}

func DetectLanguage(path string) (Language, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	language, ok := languageByExt[ext]
	return language, ok
}

func LanguageSpecFor(language Language) (LanguageSpec, bool) {
	for _, spec := range languageSpecs {
		if spec.Language == language {
			return LanguageSpec{
				Language:   spec.Language,
				Extensions: append([]string{}, spec.Extensions...),
			}, true
		}
	}
	return LanguageSpec{}, false
}

func GroupFilesByLanguage(paths []string) map[Language][]string {
	grouped := make(map[Language][]string)
	for _, path := range paths {
		language, ok := DetectLanguage(path)
		if !ok {
			continue
		}
		grouped[language] = append(grouped[language], path)
	}
	for language := range grouped {
		sort.Strings(grouped[language])
	}
	return grouped
}

func buildLanguageByExtension() map[string]Language {
	byExt := make(map[string]Language)
	for _, spec := range languageSpecs {
		for _, ext := range spec.Extensions {
			byExt[ext] = spec.Language
		}
	}
	return byExt
}
