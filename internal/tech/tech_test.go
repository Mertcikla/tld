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
