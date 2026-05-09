// Package tech provides technology icon and name validation logic.
package tech

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
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
