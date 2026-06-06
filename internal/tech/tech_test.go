package tech

import (
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
