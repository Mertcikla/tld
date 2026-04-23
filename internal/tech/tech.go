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
	catalogCache map[string]bool
	catalogOnce  sync.Once
)

func initializeCatalog() {
	var items []catalogItem
	err := json.Unmarshal(iconsJSON, &items)
	if err != nil {
		catalogCache = make(map[string]bool)
		return
	}

	cache := make(map[string]bool, len(items)*3)

	add := func(key string) {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			return
		}
		cache[key] = true
	}

	for _, item := range items {
		add(item.Name)
		if item.NameShort != "" {
			add(item.NameShort)
		}
		add(item.DefaultSlug)
	}

	manualAliases := []string{
		"go", "postgres", "node", "ts", "js", "tailwind", "tailwindcss",
		"next.js", "k8s", "dockerfile", "python3", "cpp", "c#", "dotnet",
		"aws", "gcp", "azure",
	}

	for _, alias := range manualAliases {
		add(alias)
	}

	catalogCache = cache
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
