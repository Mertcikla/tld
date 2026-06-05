// Package tech provides technology icon and name validation logic.
package tech

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	buildassets "github.com/mertcikla/tld/v2/build-assets"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// catalogItem represents an entry in the frontend icons.json catalog.
type catalogItem struct {
	IconURL     string   `json:"iconUrl"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider,omitempty"`
	DocsURL     string   `json:"docsUrl,omitempty"`
	Description string   `json:"description,omitempty"`
	WebsiteURL  string   `json:"websiteUrl,omitempty"`
	NameShort   string   `json:"nameShort"`
	DefaultSlug string   `json:"defaultSlug"`
	Aliases     []string `json:"aliases,omitempty"`
	custom      bool
}

// CatalogEntry is a read-only public view of one catalog item.
type CatalogEntry struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	NameShort string `json:"name_short,omitempty"`
	IconURL   string `json:"icon_url,omitempty"`
}

var (
	catalogCache     map[string]bool
	catalogSlugCache map[string]catalogItem
	catalogItems     []CatalogEntry
	catalogRawItems  []catalogItem
	catalogOnce      sync.Once
)

var manualAliases = map[string]string{
	"go":                              "go",
	"golang":                          "go",
	"postgres":                        "postgresql",
	"node":                            "nodejs",
	"ts":                              "typescript",
	"js":                              "javascript",
	"tailwind":                        "tailwindcss",
	"tailwind-css":                    "tailwindcss",
	"tailwindcss":                     "tailwindcss",
	"next.js":                         "nextjs",
	"k8s":                             "kubernetes",
	"dockerfile":                      "docker",
	"python3":                         "python",
	"cpp":                             "cplusplus",
	"c-plusplus":                      "cplusplus",
	"c#":                              "csharp",
	"dotnet":                          "dot-net",
	".net":                            "dot-net",
	"net":                             "dot-net",
	"json-javascript-object-notation": "json",
	"gcp":                             "googlecloud",
	"google-cloud-platform":           "googlecloud",
	"container":                       "docker",
}

func initializeCatalog() {
	items := loadCatalogItems()

	cache := make(map[string]bool, len(items)*3)
	slugCache := make(map[string]catalogItem, len(items)*3)
	entries := make([]CatalogEntry, 0, len(items))
	aliases := make([]catalogAlias, 0)

	add := func(key string, item catalogItem, replace bool) {
		key = normalizeCatalogKey(key)
		if key == "" {
			return
		}
		cache[key] = true
		if item.DefaultSlug != "" {
			if _, exists := slugCache[key]; exists && !replace {
				return
			}
			slugCache[key] = item
		}
	}

	for _, item := range items {
		add(item.Name, item, false)
		if item.NameShort != "" {
			add(item.NameShort, item, false)
		}
		add(item.DefaultSlug, item, false)
		for _, alias := range item.Aliases {
			aliases = append(aliases, catalogAlias{key: alias, item: item})
		}
		if item.DefaultSlug != "" {
			entries = append(entries, CatalogEntry{
				Slug:      item.DefaultSlug,
				Name:      item.Name,
				NameShort: item.NameShort,
				IconURL:   item.IconURL,
			})
		}
	}

	for alias, slug := range manualAliases {
		item, ok := slugCache[normalizeCatalogKey(slug)]
		if !ok || item.DefaultSlug == "" {
			continue
		}
		add(alias, item, false)
	}

	for _, alias := range aliases {
		add(alias.key, alias.item, alias.item.custom)
	}

	catalogCache = cache
	catalogSlugCache = slugCache
	catalogRawItems = stripCatalogSource(items)
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

type catalogAlias struct {
	key  string
	item catalogItem
}

func loadCatalogItems() []catalogItem {
	builtinBody, err := buildassets.IconCatalogJSON()
	if err != nil {
		return loadCustomCatalogItems()
	}
	builtin, err := decodeCatalogJSON(builtinBody, false)
	if err != nil {
		return loadCustomCatalogItems()
	}
	return mergeCatalogs(builtin, loadCustomCatalogItems())
}

func loadCustomCatalogItems() []catalogItem {
	configDir, err := workspace.ConfigDir()
	if err != nil {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(configDir, "icons", "icons.json"))
	if err != nil {
		return nil
	}
	items, err := decodeCatalogJSON(body, true)
	if err != nil {
		return nil
	}
	return items
}

func decodeCatalogJSON(body []byte, custom bool) ([]catalogItem, error) {
	var items []catalogItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}

	out := make([]catalogItem, 0, len(items))
	for _, item := range items {
		normalized, ok := normalizeCatalogItem(item, custom)
		if ok {
			out = append(out, normalized)
		}
	}
	return out, nil
}

func mergeCatalogs(builtin, custom []catalogItem) []catalogItem {
	out := make([]catalogItem, 0, len(builtin)+len(custom))
	bySlug := make(map[string]int, len(builtin)+len(custom))

	add := func(item catalogItem) {
		slug := normalizeCatalogKey(item.DefaultSlug)
		if slug == "" {
			return
		}
		if idx, ok := bySlug[slug]; ok {
			out[idx] = item
			return
		}
		bySlug[slug] = len(out)
		out = append(out, item)
	}

	for _, item := range builtin {
		add(item)
	}
	for _, item := range custom {
		add(item)
	}
	return out
}

func normalizeCatalogItem(item catalogItem, custom bool) (catalogItem, bool) {
	item.DefaultSlug = normalizeCatalogKey(item.DefaultSlug)
	if item.DefaultSlug == "" {
		return catalogItem{}, false
	}
	item.IconURL = strings.TrimSpace(item.IconURL)
	if item.IconURL == "" {
		item.IconURL = "/icons/" + item.DefaultSlug + ".svg"
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		item.Name = item.DefaultSlug
	}
	item.NameShort = strings.TrimSpace(item.NameShort)
	item.Provider = strings.TrimSpace(item.Provider)
	item.DocsURL = strings.TrimSpace(item.DocsURL)
	item.Description = strings.TrimSpace(item.Description)
	item.WebsiteURL = strings.TrimSpace(item.WebsiteURL)
	item.Aliases = normalizeAliases(item.Aliases)
	item.custom = custom
	return item, true
}

func normalizeAliases(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		key := normalizeCatalogKey(trimmed)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeCatalogKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func stripCatalogSource(items []catalogItem) []catalogItem {
	out := make([]catalogItem, len(items))
	copy(out, items)
	for i := range out {
		out[i].custom = false
	}
	return out
}

// Catalog returns the technology catalog sorted by display name.
func Catalog() []CatalogEntry {
	catalogOnce.Do(initializeCatalog)
	out := make([]CatalogEntry, len(catalogItems))
	copy(out, catalogItems)
	return out
}

// CatalogJSON returns the merged frontend catalog JSON.
func CatalogJSON() ([]byte, error) {
	catalogOnce.Do(initializeCatalog)
	out := stripCatalogSource(catalogRawItems)
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

// IconURLForSlug returns the SVG icon URL for a catalog slug or a supported
// legacy slug alias.
func IconURLForSlug(slug string) (string, bool) {
	catalogOnce.Do(initializeCatalog)

	normalized := normalizeCatalogKey(slug)
	item, ok := catalogSlugCache[normalized]
	if !ok || item.DefaultSlug == "" {
		return "", false
	}
	if item.IconURL != "" {
		return item.IconURL, true
	}
	return "/icons/" + item.DefaultSlug + ".svg", true
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

	normalized := normalizeCatalogKey(label)
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

	limit := min(len(candidates), maxResults)
	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, candidates[i].name)
	}
	return result
}

func sortByDistance(items []scored) {
	for i := range items {
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
			curr[j] = min(del, ins)
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

		if !catalogCache[normalizeCatalogKey(p)] {
			missing = append(missing, p)
		}
	}

	return missing
}

// LookupCatalog returns the catalog slug and display name for an exact
// technology label, short name, slug, or known alias.
func LookupCatalog(label string) (slug, name string, ok bool) {
	catalogOnce.Do(initializeCatalog)

	normalized := normalizeCatalogKey(label)
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
