package tech

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func isolateCatalog(t *testing.T) string {
	t.Helper()
	resetCatalogForTest()
	dir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", dir)
	t.Cleanup(resetCatalogForTest)
	return dir
}

func resetCatalogForTest() {
	catalogOnce = sync.Once{}
	catalogCache = nil
	catalogSlugCache = nil
	catalogItems = nil
	catalogRawItems = nil
}

func writeCustomCatalog(t *testing.T, configDir, body string) {
	t.Helper()
	dir := filepath.Join(configDir, "icons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir custom catalog dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icons.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write custom catalog: %v", err)
	}
}

func TestValidateAcceptsContainerAsDockerAlias(t *testing.T) {
	isolateCatalog(t)

	if missing := Validate("Container"); len(missing) != 0 {
		t.Fatalf("Validate(%q) missing = %v, want none", "Container", missing)
	}
}

func TestCatalogReturnsSortedCopy(t *testing.T) {
	isolateCatalog(t)

	items := Catalog()
	if len(items) == 0 {
		t.Fatal("Catalog returned no items")
	}
	items[0].Slug = "mutated"
	again := Catalog()
	if again[0].Slug == "mutated" {
		t.Fatal("Catalog returned mutable package state")
	}
	for i := 1; i < len(again); i++ {
		if strings.ToLower(again[i-1].Name) > strings.ToLower(again[i].Name) {
			t.Fatalf("Catalog is not sorted at %d: %q > %q", i, again[i-1].Name, again[i].Name)
		}
	}
}

func TestLookupCatalogMatchesEmbeddedIconLabels(t *testing.T) {
	isolateCatalog(t)

	slug, name, ok := LookupCatalog("flask")
	if !ok || slug != "flask" || name != "Flask" {
		t.Fatalf("LookupCatalog(%q) = slug:%q name:%q ok:%v, want flask/Flask/true", "flask", slug, name, ok)
	}
}

func TestLookupCatalogResolvesLegacySlugAliases(t *testing.T) {
	isolateCatalog(t)

	tests := []struct {
		label string
		slug  string
		name  string
	}{
		{label: "golang", slug: "go", name: "Go"},
		{label: "c-plusplus", slug: "cplusplus", name: "C++"},
		{label: "json-javascript-object-notation", slug: "json", name: "JSON"},
		{label: "tailwind-css", slug: "tailwindcss", name: "Tailwind CSS"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			slug, name, ok := LookupCatalog(tt.label)
			if !ok || slug != tt.slug || name != tt.name {
				t.Fatalf("LookupCatalog(%q) = slug:%q name:%q ok:%v, want %s/%s/true", tt.label, slug, name, ok, tt.slug, tt.name)
			}
		})
	}
}

func TestIconURLForSlugResolvesCanonicalAndLegacySlugs(t *testing.T) {
	isolateCatalog(t)

	tests := map[string]string{
		"go":                              "/icons/go.svg",
		"golang":                          "/icons/go.svg",
		"c-plusplus":                      "/icons/cplusplus.svg",
		"json-javascript-object-notation": "/icons/json.svg",
		"tailwind-css":                    "/icons/tailwindcss.svg",
	}
	for slug, want := range tests {
		got, ok := IconURLForSlug(slug)
		if !ok || got != want {
			t.Fatalf("IconURLForSlug(%q) = %q, %v; want %q, true", slug, got, ok, want)
		}
	}
}

func TestRemovedOldCatalogSlugHasNoIconURL(t *testing.T) {
	isolateCatalog(t)

	if got, ok := IconURLForSlug("aws-amazon-ec2-instances"); ok {
		t.Fatalf("IconURLForSlug removed slug = %q, true; want no match", got)
	}
}

func TestCustomCatalogAddsEntriesAndAliases(t *testing.T) {
	configDir := isolateCatalog(t)
	writeCustomCatalog(t, configDir, `[
  {
    "iconUrl": "/icons/my-custom-icon.svg",
    "name": "My Custom Icon",
    "nameShort": "Custom",
    "defaultSlug": "my-custom-icon",
    "aliases": ["custom-service"]
  }
]`)

	slug, name, ok := LookupCatalog("custom-service")
	if !ok || slug != "my-custom-icon" || name != "My Custom Icon" {
		t.Fatalf("LookupCatalog custom alias = slug:%q name:%q ok:%v, want my-custom-icon/My Custom Icon/true", slug, name, ok)
	}
	if missing := Validate("custom-service"); len(missing) != 0 {
		t.Fatalf("Validate custom alias missing = %v, want none", missing)
	}
	if got, ok := IconURLForSlug("my-custom-icon"); !ok || got != "/icons/my-custom-icon.svg" {
		t.Fatalf("IconURLForSlug custom = %q, %v; want custom icon URL", got, ok)
	}
}

func TestCustomCatalogOverridesBuiltInBySlug(t *testing.T) {
	configDir := isolateCatalog(t)
	writeCustomCatalog(t, configDir, `[
  {
    "iconUrl": "/icons/custom-go.svg",
    "name": "Custom Go",
    "nameShort": "Custom Go",
    "defaultSlug": "go",
    "aliases": ["custom-golang"]
  }
]`)

	slug, name, ok := LookupCatalog("go")
	if !ok || slug != "go" || name != "Custom Go" {
		t.Fatalf("LookupCatalog overridden go = slug:%q name:%q ok:%v, want go/Custom Go/true", slug, name, ok)
	}
	if got, ok := IconURLForSlug("golang"); !ok || got != "/icons/custom-go.svg" {
		t.Fatalf("IconURLForSlug legacy alias after override = %q, %v; want custom go URL", got, ok)
	}
	if slug, name, ok := LookupCatalog("custom-golang"); !ok || slug != "go" || name != "Custom Go" {
		t.Fatalf("LookupCatalog custom alias for override = slug:%q name:%q ok:%v, want go/Custom Go/true", slug, name, ok)
	}
}

func TestCustomCatalogInvalidJSONFallsBackToBuiltInCatalog(t *testing.T) {
	configDir := isolateCatalog(t)
	writeCustomCatalog(t, configDir, `{not-json`)

	slug, name, ok := LookupCatalog("go")
	if !ok || slug != "go" || name != "Go" {
		t.Fatalf("LookupCatalog with invalid custom catalog = slug:%q name:%q ok:%v, want go/Go/true", slug, name, ok)
	}
	if _, _, ok := LookupCatalog("custom-service"); ok {
		t.Fatal("invalid custom catalog should not add custom-service")
	}
}

func TestLookupCatalogFuzzyMatchesDecoratedTechnologyLabels(t *testing.T) {
	isolateCatalog(t)

	tests := []struct {
		label string
		slug  string
		name  string
	}{
		{label: "redis-cart", slug: "redis", name: "Redis"},
		{label: "postgres db", slug: "postgresql", name: "PostgreSQL"},
		{label: "payment grpc client", slug: "grpc", name: "gRPC"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			slug, name, ok := LookupCatalogFuzzy(tt.label)
			if !ok || slug != tt.slug || name != tt.name {
				t.Fatalf("LookupCatalogFuzzy(%q) = slug:%q name:%q ok:%v, want %s/%s/true", tt.label, slug, name, ok, tt.slug, tt.name)
			}
		})
	}
}

func TestLookupCatalogFuzzyRejectsUnknownLabels(t *testing.T) {
	isolateCatalog(t)

	if slug, name, ok := LookupCatalogFuzzy("Internal SDK"); ok {
		t.Fatalf("LookupCatalogFuzzy(%q) = slug:%q name:%q ok:%v, want no match", "Internal SDK", slug, name, ok)
	}
}

func TestCreateCustomTechnologyWritesSVGAndReloadsCatalog(t *testing.T) {
	configDir := isolateCatalog(t)
	_ = Catalog()

	item, err := CreateCustomTechnology(CustomTechnologyInput{
		Name:      "My Custom Icon",
		NameShort: "Custom",
		Aliases:   []string{"custom-service"},
		Icon:      []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M1 1h14v14H1z"/></svg>`),
		MediaType: "image/svg+xml",
	})
	if err != nil {
		t.Fatalf("CreateCustomTechnology: %v", err)
	}
	if item.DefaultSlug != "my-custom-icon" || item.IconURL != "/icons/my-custom-icon.svg" {
		t.Fatalf("custom item = %+v, want my-custom-icon svg", item)
	}

	iconBody, err := os.ReadFile(filepath.Join(configDir, "icons", "icons", "my-custom-icon.svg"))
	if err != nil {
		t.Fatalf("read custom icon: %v", err)
	}
	if !strings.Contains(string(iconBody), "<svg") {
		t.Fatalf("custom icon body not written: %s", string(iconBody))
	}
	catalogBody, err := os.ReadFile(filepath.Join(configDir, "icons", "icons.json"))
	if err != nil {
		t.Fatalf("read custom catalog: %v", err)
	}
	if !strings.Contains(string(catalogBody), `"defaultSlug": "my-custom-icon"`) {
		t.Fatalf("custom catalog missing item: %s", string(catalogBody))
	}

	slug, name, ok := LookupCatalog("custom-service")
	if !ok || slug != "my-custom-icon" || name != "My Custom Icon" {
		t.Fatalf("LookupCatalog custom alias = slug:%q name:%q ok:%v, want my-custom-icon/My Custom Icon/true", slug, name, ok)
	}
	if got, ok := IconURLForSlug("my-custom-icon"); !ok || got != "/icons/my-custom-icon.svg" {
		t.Fatalf("IconURLForSlug custom = %q, %v; want custom icon URL", got, ok)
	}
}

func TestCreateCustomTechnologyNormalizesPNG(t *testing.T) {
	configDir := isolateCatalog(t)

	item, err := CreateCustomTechnology(CustomTechnologyInput{
		Name:      "Wide PNG",
		Icon:      testPNG(t, 4, 2),
		MediaType: "image/png",
	})
	if err != nil {
		t.Fatalf("CreateCustomTechnology png: %v", err)
	}
	if item.IconURL != "/icons/wide-png.png" {
		t.Fatalf("png icon URL = %q, want /icons/wide-png.png", item.IconURL)
	}

	body, err := os.ReadFile(filepath.Join(configDir, "icons", "icons", "wide-png.png"))
	if err != nil {
		t.Fatalf("read normalized png: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("decode normalized png config: %v", err)
	}
	if cfg.Width != normalizedPNGIconSize || cfg.Height != normalizedPNGIconSize {
		t.Fatalf("normalized png dimensions = %dx%d, want %dx%d", cfg.Width, cfg.Height, normalizedPNGIconSize, normalizedPNGIconSize)
	}
	if got, ok := IconURLForSlug("wide-png"); !ok || got != "/icons/wide-png.png" {
		t.Fatalf("IconURLForSlug png = %q, %v; want png icon URL", got, ok)
	}
}

func TestCreateCustomTechnologyGeneratesUniqueSlug(t *testing.T) {
	isolateCatalog(t)

	first, err := CreateCustomTechnology(CustomTechnologyInput{
		Name:      "Go",
		Icon:      []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		MediaType: "image/svg+xml",
	})
	if err != nil {
		t.Fatalf("CreateCustomTechnology first: %v", err)
	}
	second, err := CreateCustomTechnology(CustomTechnologyInput{
		Name:      "Go",
		Icon:      []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		MediaType: "image/svg+xml",
	})
	if err != nil {
		t.Fatalf("CreateCustomTechnology second: %v", err)
	}
	if first.DefaultSlug != "go-2" || second.DefaultSlug != "go-3" {
		t.Fatalf("generated slugs = %q, %q; want go-2, go-3", first.DefaultSlug, second.DefaultSlug)
	}
}

func TestCreateCustomTechnologyRejectsInvalidInput(t *testing.T) {
	isolateCatalog(t)

	_, err := CreateCustomTechnology(CustomTechnologyInput{
		Name:      "Bad SVG",
		Icon:      []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script /></svg>`),
		MediaType: "image/svg+xml",
	})
	if !IsInvalidCustomTechnology(err) {
		t.Fatalf("script svg error = %v, want invalid custom technology", err)
	}

	_, err = CreateCustomTechnology(CustomTechnologyInput{
		Name:      "Bad File",
		Icon:      []byte("not an icon"),
		MediaType: "text/plain",
	})
	if !IsInvalidCustomTechnology(err) {
		t.Fatalf("plain text error = %v, want invalid custom technology", err)
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}
