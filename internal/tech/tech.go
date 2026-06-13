// Package tech provides technology icon and name validation logic.
package tech

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

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

// CustomTechnologyInput is the browser-provided payload for a custom icon
// catalog entry.
type CustomTechnologyInput struct {
	Name          string
	NameShort     string
	Aliases       []string
	Icon          []byte
	MediaType     string
	PreferredSlug string
}

// CatalogItem is the browser-facing shape used by the technology catalog.
type CatalogItem struct {
	IconURL     string   `json:"iconUrl"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider,omitempty"`
	DocsURL     string   `json:"docsUrl,omitempty"`
	Description string   `json:"description,omitempty"`
	WebsiteURL  string   `json:"websiteUrl,omitempty"`
	NameShort   string   `json:"nameShort"`
	DefaultSlug string   `json:"defaultSlug"`
	Aliases     []string `json:"aliases,omitempty"`
}

// InvalidCustomTechnologyError marks user-correctable custom technology input
// failures separately from filesystem or catalog IO failures.
type InvalidCustomTechnologyError struct {
	Err error
}

func (e InvalidCustomTechnologyError) Error() string {
	if e.Err == nil {
		return "invalid custom technology"
	}
	return e.Err.Error()
}

func (e InvalidCustomTechnologyError) Unwrap() error {
	return e.Err
}

func invalidCustomTechnology(err error) error {
	return InvalidCustomTechnologyError{Err: err}
}

// IsInvalidCustomTechnology reports whether err came from invalid upload input.
func IsInvalidCustomTechnology(err error) bool {
	var target InvalidCustomTechnologyError
	return errors.As(err, &target)
}

const (
	maxCustomIconBytes    = 2 * 1024 * 1024
	maxCustomPNGDimension = 4096
	normalizedPNGIconSize = 128
	customCatalogFilePerm = 0o644
	customCatalogDirPerm  = 0o755
)

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
	items, err := readCustomCatalogItems()
	if err != nil {
		return nil
	}
	return items
}

func readCustomCatalogItems() ([]catalogItem, error) {
	configDir, err := workspace.ConfigDir()
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(filepath.Join(configDir, "icons", "icons.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	items, err := decodeCatalogJSON(body, true)
	if err != nil {
		return nil, err
	}
	return items, nil
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
			item.Aliases = mergeAliases(out[idx].Aliases, item.Aliases)
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

func mergeAliases(left, right []string) []string {
	return normalizeAliases(append(append([]string{}, left...), right...))
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

func resetCatalog() {
	catalogOnce = sync.Once{}
	catalogCache = nil
	catalogSlugCache = nil
	catalogItems = nil
	catalogRawItems = nil
}

// ReloadCatalog invalidates the in-process technology catalog cache. It should
// be called after writing custom catalog files.
func ReloadCatalog() {
	resetCatalog()
}

func publicCatalogItem(item catalogItem) CatalogItem {
	return CatalogItem{
		IconURL:     item.IconURL,
		Name:        item.Name,
		Provider:    item.Provider,
		DocsURL:     item.DocsURL,
		Description: item.Description,
		WebsiteURL:  item.WebsiteURL,
		NameShort:   item.NameShort,
		DefaultSlug: item.DefaultSlug,
		Aliases:     append([]string{}, item.Aliases...),
	}
}

// CreateCustomTechnology validates and persists a user-provided technology
// icon, appends it to the global custom catalog, and reloads catalog state.
func CreateCustomTechnology(input CustomTechnologyInput) (CatalogItem, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CatalogItem{}, invalidCustomTechnology(errors.New("name must not be empty"))
	}
	if len(input.Icon) == 0 {
		return CatalogItem{}, invalidCustomTechnology(errors.New("icon must not be empty"))
	}
	if len(input.Icon) > maxCustomIconBytes {
		return CatalogItem{}, invalidCustomTechnology(fmt.Errorf("icon must be %d bytes or smaller", maxCustomIconBytes))
	}

	ext, iconBody, err := normalizeCustomIcon(input.Icon, input.MediaType)
	if err != nil {
		return CatalogItem{}, invalidCustomTechnology(err)
	}

	customItems, err := readCustomCatalogItems()
	if err != nil {
		return CatalogItem{}, fmt.Errorf("read custom catalog: %w", err)
	}
	builtinItems, err := builtinCatalogItems()
	if err != nil {
		return CatalogItem{}, fmt.Errorf("read built-in catalog: %w", err)
	}

	slugBase := sanitizeCatalogSlug(input.PreferredSlug)
	if slugBase == "" {
		slugBase = sanitizeCatalogSlug(name)
	}
	if slugBase == "" {
		return CatalogItem{}, invalidCustomTechnology(errors.New("name must contain letters or numbers"))
	}
	slug := uniqueCatalogSlug(slugBase, mergeCatalogs(builtinItems, customItems))

	nameShort := strings.TrimSpace(input.NameShort)
	if nameShort == "" {
		nameShort = name
	}

	item := catalogItem{
		IconURL:     fmt.Sprintf("/icons/%s.%s", slug, ext),
		Name:        name,
		NameShort:   nameShort,
		DefaultSlug: slug,
		Aliases:     normalizeAliases(input.Aliases),
		custom:      true,
	}

	configDir, err := workspace.ConfigDir()
	if err != nil {
		return CatalogItem{}, err
	}
	customRoot := filepath.Join(configDir, "icons")
	customIconsDir := filepath.Join(customRoot, "icons")
	if err := os.MkdirAll(customIconsDir, customCatalogDirPerm); err != nil {
		return CatalogItem{}, fmt.Errorf("create custom icons directory: %w", err)
	}

	iconFilename := slug + "." + ext
	if err := os.WriteFile(filepath.Join(customIconsDir, iconFilename), iconBody, customCatalogFilePerm); err != nil {
		return CatalogItem{}, fmt.Errorf("write custom icon: %w", err)
	}

	nextItems := append(append([]catalogItem{}, customItems...), item)
	if err := persistCustomCatalog(filepath.Join(customRoot, "icons.json"), nextItems); err != nil {
		return CatalogItem{}, err
	}

	ReloadCatalog()
	return publicCatalogItem(item), nil
}

func builtinCatalogItems() ([]catalogItem, error) {
	body, err := buildassets.IconCatalogJSON()
	if err != nil {
		return nil, err
	}
	return decodeCatalogJSON(body, false)
}

func persistCustomCatalog(target string, items []catalogItem) error {
	body, err := json.MarshalIndent(stripCatalogSource(items), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal custom catalog: %w", err)
	}
	body = append(body, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(target), ".icons-*.json")
	if err != nil {
		return fmt.Errorf("create custom catalog temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write custom catalog temp file: %w", err)
	}
	if err := tmp.Chmod(customCatalogFilePerm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod custom catalog temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close custom catalog temp file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace custom catalog: %w", err)
	}
	return nil
}

func uniqueCatalogSlug(base string, existing []catalogItem) string {
	used := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if slug := sanitizeCatalogSlug(item.DefaultSlug); slug != "" {
			used[slug] = struct{}{}
		}
	}
	if _, ok := used[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}

func sanitizeCatalogSlug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 80 {
		out = strings.Trim(out[:80], "-")
	}
	return out
}

func normalizeCustomIcon(body []byte, mediaType string) (ext string, normalized []byte, err error) {
	mediaType = strings.ToLower(strings.TrimSpace(strings.Split(mediaType, ";")[0]))
	detected := http.DetectContentType(body)

	switch {
	case mediaType == "image/svg+xml":
		if err := validateSVGIcon(body); err != nil {
			return "", nil, err
		}
		return "svg", body, nil
	case mediaType == "image/png" || detected == "image/png":
		normalized, err := normalizePNGIcon(body)
		if err != nil {
			return "", nil, err
		}
		return "png", normalized, nil
	default:
		return "", nil, fmt.Errorf("unsupported icon media type %q", mediaType)
	}
}

func validateSVGIcon(body []byte) error {
	if !utf8.Valid(body) {
		return errors.New("svg icon must be valid UTF-8")
	}

	lower := strings.ToLower(string(body))
	for _, marker := range []string{"<script", "javascript:", "data:text/html"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("svg icon contains disallowed content %q", marker)
		}
	}

	decoder := xml.NewDecoder(bytes.NewReader(body))
	seenRoot := false
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("parse svg icon: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		name := strings.ToLower(start.Name.Local)
		if !seenRoot {
			if name != "svg" {
				return errors.New("svg icon root must be <svg>")
			}
			seenRoot = true
		}

		switch name {
		case "script", "foreignobject", "iframe", "object", "embed":
			return fmt.Errorf("svg icon contains disallowed element <%s>", name)
		}

		for _, attr := range start.Attr {
			attrName := strings.ToLower(attr.Name.Local)
			attrValue := strings.ToLower(strings.TrimSpace(attr.Value))
			if strings.HasPrefix(attrName, "on") {
				return fmt.Errorf("svg icon contains disallowed event attribute %q", attr.Name.Local)
			}
			if (attrName == "href" || attrName == "src") && strings.HasPrefix(attrValue, "javascript:") {
				return fmt.Errorf("svg icon contains disallowed %q value", attr.Name.Local)
			}
			if attrName == "style" && strings.Contains(attrValue, "javascript:") {
				return errors.New("svg icon contains disallowed style value")
			}
		}
	}
	if !seenRoot {
		return errors.New("svg icon root must be <svg>")
	}
	return nil
}

func normalizePNGIcon(body []byte) ([]byte, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode png icon: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, errors.New("png icon dimensions must be positive")
	}
	if cfg.Width > maxCustomPNGDimension || cfg.Height > maxCustomPNGDimension {
		return nil, fmt.Errorf("png icon dimensions must be %dx%d or smaller", maxCustomPNGDimension, maxCustomPNGDimension)
	}

	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode png icon: %w", err)
	}
	dst := fitImageNearest(img, normalizedPNGIconSize)

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("encode normalized png icon: %w", err)
	}
	return out.Bytes(), nil
}

func fitImageNearest(src image.Image, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	scale := math.Min(float64(size)/float64(srcW), float64(size)/float64(srcH))
	drawW := max(1, int(math.Round(float64(srcW)*scale)))
	drawH := max(1, int(math.Round(float64(srcH)*scale)))
	offsetX := (size - drawW) / 2
	offsetY := (size - drawH) / 2

	for y := 0; y < drawH; y++ {
		srcY := bounds.Min.Y + min(srcH-1, int(float64(y)*float64(srcH)/float64(drawH)))
		for x := 0; x < drawW; x++ {
			srcX := bounds.Min.X + min(srcW-1, int(float64(x)*float64(srcW)/float64(drawW)))
			c := color.NRGBAModel.Convert(src.At(srcX, srcY)).(color.NRGBA)
			dst.SetNRGBA(offsetX+x, offsetY+y, c)
		}
	}
	return dst
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
