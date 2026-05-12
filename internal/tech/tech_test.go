package tech

import "testing"

func TestValidateAcceptsContainerAsDockerAlias(t *testing.T) {
	if missing := Validate("Container"); len(missing) != 0 {
		t.Fatalf("Validate(%q) missing = %v, want none", "Container", missing)
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
