package tech

import (
	"strings"
	"testing"
)

func TestValidateAcceptsContainerAsDockerAlias(t *testing.T) {
	if missing := Validate("Container"); len(missing) != 0 {
		t.Fatalf("Validate(%q) missing = %v, want none", "Container", missing)
	}
}

func TestCatalogReturnsSortedCopy(t *testing.T) {
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
	slug, name, ok := LookupCatalog("flask")
	if !ok || slug != "flask" || name != "Flask" {
		t.Fatalf("LookupCatalog(%q) = slug:%q name:%q ok:%v, want flask/Flask/true", "flask", slug, name, ok)
	}
}

func TestLookupCatalogFuzzyMatchesDecoratedTechnologyLabels(t *testing.T) {
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
	if slug, name, ok := LookupCatalogFuzzy("Internal SDK"); ok {
		t.Fatalf("LookupCatalogFuzzy(%q) = slug:%q name:%q ok:%v, want no match", "Internal SDK", slug, name, ok)
	}
}
