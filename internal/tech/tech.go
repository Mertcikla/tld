// Package tech provides technology icon and name validation logic.
package tech

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
	"unicode"
)

//go:embed icons.json
var iconsJSON []byte

// catalogItem represents an entry in the embedded icons.json.
type catalogItem struct {
	Name        string `json:"name"`
	NameShort   string `json:"nameShort"`
	DefaultSlug string `json:"defaultSlug"`
}

var (
	catalogCache     map[string]bool
	catalogSlugCache map[string]catalogItem
	catalogOnce      sync.Once
)

func initializeCatalog() {
	var items []catalogItem
	err := json.Unmarshal(iconsJSON, &items)
	if err != nil {
		catalogCache = make(map[string]bool)
		return
	}

	cache := make(map[string]bool, len(items)*3)
	slugCache := make(map[string]catalogItem, len(items)*3)

	add := func(key string, item catalogItem) {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			return
		}
		cache[key] = true
		if item.DefaultSlug != "" {
			if _, exists := slugCache[key]; exists {
				return
			}
			slugCache[key] = item
		}
	}

	for _, item := range items {
		add(item.Name, item)
		if item.NameShort != "" {
			add(item.NameShort, item)
		}
		add(item.DefaultSlug, item)
	}

	manualAliases := map[string]string{
		"go": "golang", "postgres": "postgresql", "node": "nodejs", "ts": "typescript", "js": "javascript",
		"tailwind": "tailwind-css", "tailwindcss": "tailwind-css", "next.js": "nextjs",
		"k8s": "kubernetes", "dockerfile": "docker", "python3": "python", "cpp": "cplusplus",
		"c#": "csharp", "dotnet": "dotnet", "aws": "aws", "gcp": "gcp", "azure": "azure",
		"container": "docker",
	}

	for alias, slug := range manualAliases {
		item, ok := slugCache[strings.ToLower(slug)]
		if !ok {
			item = catalogItem{Name: alias, NameShort: alias, DefaultSlug: slug}
		}
		add(alias, item)
	}

	catalogCache = cache
	catalogSlugCache = slugCache
}

// Validate returns true if the technology string or any of its parts (if separated)
// matches a known technology in the catalog.
// It follows the separator logic: , / ;
func Validate(techStr string) (missing []string) {
	if techStr == "" {
		return nil
	}

	catalogOnce.Do(initializeCatalog)

	parts := strings.FieldsFunc(techStr, func(r rune) bool {
		return r == ',' || r == '/' || r == ';'
	})

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		lower := strings.ToLower(p)
		if !catalogCache[lower] {
			missing = append(missing, p)
		}
	}

	return missing
}

// LookupCatalog returns the catalog slug and display name for an exact
// technology label, short name, slug, or known alias.
func LookupCatalog(label string) (slug, name string, ok bool) {
	catalogOnce.Do(initializeCatalog)

	normalized := strings.ToLower(strings.TrimSpace(label))
	item, ok := catalogSlugCache[normalized]
	if !ok || item.DefaultSlug == "" {
		return "", "", false
	}
	displayName := item.Name
	if strings.EqualFold(label, item.NameShort) {
		displayName = item.NameShort
	}
	if displayName == "" {
		displayName = strings.TrimSpace(label)
	}
	return item.DefaultSlug, displayName, true
}

// LookupCatalogFuzzy returns a known catalog technology for labels that are
// commonly decorated with instance names, roles, or separators.
func LookupCatalogFuzzy(label string) (slug, name string, ok bool) {
	if slug, name, ok := LookupCatalog(label); ok {
		return slug, name, true
	}

	catalogOnce.Do(initializeCatalog)
	for _, part := range splitTechnologyParts(label) {
		if slug, name, ok := LookupCatalog(part); ok {
			return slug, name, true
		}
		for _, token := range technologyTokens(part) {
			if len(token) < 3 || fuzzyTechnologyStopword(token) {
				continue
			}
			if item, ok := catalogSlugCache[token]; ok && item.DefaultSlug != "" {
				return item.DefaultSlug, catalogDisplayName(item, token), true
			}
		}
	}

	return "", "", false
}

func catalogDisplayName(item catalogItem, matched string) string {
	if strings.EqualFold(strings.TrimSpace(matched), item.NameShort) && item.NameShort != "" {
		return item.NameShort
	}
	if item.Name != "" {
		return item.Name
	}
	return strings.TrimSpace(matched)
}

func splitTechnologyParts(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '/' || r == ';' || r == '|'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func technologyTokens(value string) []string {
	var b strings.Builder
	var prev rune
	for _, r := range value {
		if unicode.IsUpper(r) && unicode.IsLower(prev) {
			b.WriteByte(' ')
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '#':
			b.WriteRune(r)
		case r == '+':
			b.WriteString("plus")
		default:
			b.WriteByte(' ')
		}
		prev = r
	}
	return strings.Fields(b.String())
}

func fuzzyTechnologyStopword(token string) bool {
	switch token {
	case "app", "api", "client", "server", "service", "worker", "job", "queue", "database", "db", "cache", "image", "images", "sdk":
		return true
	default:
		return false
	}
}
