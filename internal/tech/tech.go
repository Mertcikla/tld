// Package tech provides technology icon and name validation logic.
package tech

import (
	_ "embed"
	"encoding/json"
	"sort"
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

// CatalogEntry is a read-only public view of one embedded catalog item.
type CatalogEntry struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	NameShort string `json:"name_short,omitempty"`
}

var (
	catalogCache     map[string]bool
	catalogSlugCache map[string]catalogItem
	catalogItems     []CatalogEntry
	catalogOnce      sync.Once
)

func initializeCatalog() {
	var items []catalogItem
	err := json.Unmarshal(iconsJSON, &items)
	if err != nil {
		catalogCache = make(map[string]bool)
		catalogItems = nil
		return
	}

	cache := make(map[string]bool, len(items)*3)
	slugCache := make(map[string]catalogItem, len(items)*3)
	entries := make([]CatalogEntry, 0, len(items))

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
		if item.DefaultSlug != "" {
			entries = append(entries, CatalogEntry{
				Slug:      item.DefaultSlug,
				Name:      item.Name,
				NameShort: item.NameShort,
			})
		}
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
	sort.SliceStable(entries, func(i, j int) bool {
		left := strings.ToLower(entries[i].Name)
		right := strings.ToLower(entries[j].Name)
		if left == right {
			return entries[i].Slug < entries[j].Slug
		}
		return left < right
	})
	catalogItems = entries
}

// Catalog returns the embedded technology catalog sorted by display name.
func Catalog() []CatalogEntry {
	catalogOnce.Do(initializeCatalog)
	out := make([]CatalogEntry, len(catalogItems))
	copy(out, catalogItems)
	return out
}

// scored holds a candidate match with its edit distance.
type scored struct {
	name     string
	distance int
}

// SuggestSimilar finds the closest catalog matches for an unrecognized
// technology label using edit distance. Returns up to maxResults suggestions.
func SuggestSimilar(label string, maxResults int) []string {
	catalogOnce.Do(initializeCatalog)

	normalized := strings.ToLower(strings.TrimSpace(label))
	if normalized == "" {
		return nil
	}

	var candidates []scored
	seen := make(map[string]bool)

	for key := range catalogCache {
		if seen[key] {
			continue
		}
		seen[key] = true
		dist := levenshtein(normalized, key, 3)
		if dist < 0 {
			continue
		}
		candidates = append(candidates, scored{name: key, distance: dist})
	}

	sortByDistance(candidates)

	limit := len(candidates)
	if limit > maxResults {
		limit = maxResults
	}
	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, candidates[i].name)
	}
	return result
}

func sortByDistance(items []scored) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].distance < items[i].distance ||
				(items[j].distance == items[i].distance && len(items[j].name) < len(items[i].name)) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func levenshtein(a, b string, maxDist int) int {
	la, lb := len(a), len(b)
	if la == 0 {
		if lb <= maxDist {
			return lb
		}
		return -1
	}
	if lb == 0 {
		if la <= maxDist {
			return la
		}
		return -1
	}

	if la < lb {
		a, b = b, a
		la, lb = lb, la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		minInRow := i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			subst := prev[j-1] + cost
			ins := curr[j-1] + 1
			del := prev[j] + 1
			curr[j] = ins
			if del < curr[j] {
				curr[j] = del
			}
			if subst < curr[j] {
				curr[j] = subst
			}
			if curr[j] < minInRow {
				minInRow = curr[j]
			}
		}
		if minInRow > maxDist {
			return -1
		}
		prev, curr = curr, prev
	}

	if prev[lb] <= maxDist {
		return prev[lb]
	}
	return -1
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
