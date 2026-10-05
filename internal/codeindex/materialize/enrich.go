package materialize

import (
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/tech"
)

// languageDisplay maps a canonical source language to the technology label
// shown on generated elements.
var languageDisplay = map[string]string{
	"go":          "Go",
	"typescript":  "TypeScript",
	"javascript":  "JavaScript",
	"python":      "Python",
	"csharp":      "C#",
	"visualbasic": "Visual Basic",
	"c":           "C",
	"cpp":         "C++",
	"dart":        "Dart",
	"java":        "Java",
	"scala":       "Scala",
	"kotlin":      "Kotlin",
	"php":         "PHP",
	"ruby":        "Ruby",
	"rust":        "Rust",
	"json":        "JSON",
}

// canonicalLanguage normalizes indexer language ids that share one technology
// (for example tsx and typescript) into a single canonical id.
func canonicalLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "tsx", "typescript":
		return "typescript"
	case "jsx", "javascript":
		return "javascript"
	case "c#", "csharp":
		return "csharp"
	case "vb", "vbnet", "visualbasic":
		return "visualbasic"
	case "c++", "cc", "cxx", "cpp":
		return "cpp"
	default:
		return strings.ToLower(strings.TrimSpace(language))
	}
}

// languageTag returns the plain element tag for a language, or "" when the
// language is unknown.
func languageTag(language string) string {
	canonical := canonicalLanguage(language)
	if _, ok := languageDisplay[canonical]; !ok {
		return ""
	}
	return canonical
}

// isTestPath reports whether a file path looks like a test. It is a cheap,
// deterministic heuristic: explicit test file suffixes, dotted test/spec
// segments, and conventional test directories.
func isTestPath(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	if normalized == "" {
		return false
	}
	base := normalized
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		base = normalized[idx+1:]
	}
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.HasSuffix(base, "_test.py"),
		strings.HasSuffix(base, "_test.rb"),
		strings.HasSuffix(base, "_test.rs"),
		strings.HasSuffix(base, "_test.dart"),
		strings.HasSuffix(base, "test.cs"),
		strings.HasSuffix(base, "tests.cs"):
		return true
	case strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		strings.Contains(base, "_spec."):
		return true
	}
	for _, segment := range []string{"testdata", "__tests__", "test", "tests", "spec", "specs", "e2e"} {
		if strings.Contains(normalized, "/"+segment+"/") {
			return true
		}
	}
	return false
}

// languageTechnology returns the display label and catalog links for a source
// language. Unknown languages yield no technology.
func languageTechnology(language string) (string, []core.TechnologyConnector) {
	canonical := canonicalLanguage(language)
	display := languageDisplay[canonical]
	if display == "" {
		return "", nil
	}
	links := convertTechnologyLinks(tech.TechnologyLinksForElement(display, canonical))
	if len(links) == 0 {
		links = convertTechnologyLinks(tech.TechnologyLinksForElement("", canonical))
	}
	if len(links) == 0 {
		return display, nil
	}
	label := display
	if links[0].Label != "" {
		label = links[0].Label
	}
	return label, links
}

// importTechnology derives catalog technology for an external import name. It
// relies on the catalog's fuzzy matching, so recognized packages and frameworks
// get an icon while unknown imports stay unchanged.
func importTechnology(importPath string) (string, []core.TechnologyConnector) {
	name := strings.TrimSpace(importPath)
	if name == "" {
		return "", nil
	}
	links := mergeTechnology(convertTechnologyLinks(tech.TechnologyLinksForElement(name, "")), nil, 3)
	if len(links) == 0 {
		return "", nil
	}
	label := links[0].Label
	if label == "" {
		label = name
	}
	return label, links
}

// convertTechnologyLinks adapts catalog links to the workspace element model.
func convertTechnologyLinks(links []*diagv1.TechnologyLink) []core.TechnologyConnector {
	if len(links) == 0 {
		return nil
	}
	out := make([]core.TechnologyConnector, 0, len(links))
	for _, link := range links {
		if link == nil {
			continue
		}
		slug := ""
		if link.Slug != nil {
			slug = *link.Slug
		}
		out = append(out, core.TechnologyConnector{
			Type:          link.Type,
			Slug:          slug,
			Label:         link.Label,
			IsPrimaryIcon: link.IsPrimaryIcon,
		})
	}
	return out
}

// mergeTechnology appends secondary links after a primary set, dropping
// duplicates and capping the result so element editors stay within their limit.
func mergeTechnology(primary, secondary []core.TechnologyConnector, max int) []core.TechnologyConnector {
	if max < 1 {
		max = 3
	}
	seen := map[string]bool{}
	out := make([]core.TechnologyConnector, 0, max)
	for _, group := range [][]core.TechnologyConnector{primary, secondary} {
		for _, link := range group {
			key := strings.ToLower(strings.TrimSpace(link.Slug))
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(link.Label))
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, link)
			if len(out) == max {
				return out
			}
		}
	}
	return out
}
