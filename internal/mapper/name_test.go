package mapper

import (
	"fmt"
	"testing"
)

func namedDataset(root string, display, paths []string) *Dataset {
	facts := make([]Fact, len(display))
	vectors := make([][]float64, len(display))
	for i := range display {
		facts[i] = Fact{ID: fmt.Sprintf("id-%d", i), Path: paths[i], DisplayName: display[i], Language: "python"}
		vectors[i] = []float64{1, 0, 0, 0}
	}
	rootValue := root
	return &Dataset{Snapshot: "s", Profile: "p", Root: &rootValue, Facts: facts, Vectors: vectors}
}

func TestNameDomainsPrefersExclusiveTokensAndFallsBackToFolder(t *testing.T) {
	data := namedDataset(
		"repo",
		[]string{"TokenAuth", "TokenAuth", "parseXml", "parseXml"},
		[]string{"src/api/auth.py", "src/api/auth.py", "src/api/xml.py", "src/api/xml.py"},
	)
	names, err := NameDomains(data, []Domain{{Members: []int{0, 1}, Tightness: 0.9}}, DefaultNameOptions())
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "auth" {
		t.Fatalf("name = %q, want auth", names[0])
	}
	both, err := NameDomains(data, []Domain{
		{Members: []int{0, 1}, Tightness: 0.9},
		{Members: []int{2, 3}, Tightness: 0.8},
	}, DefaultNameOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 2 {
		t.Fatalf("names = %v", both)
	}
}

func TestNameDomainsSuppressesRepositoryNameAndCommonTokens(t *testing.T) {
	data := namedDataset(
		"/code/flask",
		[]string{"flask", "flask", "AlphaWidget", "BetaWidget"},
		[]string{"src/flask/a.py", "src/flask/b.py", "src/svc/a.py", "src/svc/b.py"},
	)
	names, err := NameDomains(data, []Domain{{Members: []int{0, 1}, Tightness: 0.9}}, DefaultNameOptions())
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "src/flask" {
		t.Fatalf("name = %q, want src/flask", names[0])
	}
	widget, err := NameDomains(data, []Domain{{Members: []int{2, 3}, Tightness: 0.9}}, DefaultNameOptions())
	if err != nil {
		t.Fatal(err)
	}
	if widget[0] != "widget" {
		t.Fatalf("name = %q, want widget", widget[0])
	}
}

func TestTokenizeCamelAcronymAndDottedNames(t *testing.T) {
	cases := map[string][]string{
		"TokenAuth":      {"token", "auth"},
		"parseXml":       {"parse", "xml"},
		"QueryParser2":   {"query", "parser"},
		"HTMLParser":     {"html", "parser"},
		"pyproject.toml": {"pyproject", "toml"},
		"getUserByID":    {"get", "user"},
	}
	for input, want := range cases {
		got := tokenize(input)
		if len(got) != len(want) {
			t.Fatalf("tokenize(%q) = %v, want %v", input, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("tokenize(%q) = %v, want %v", input, got, want)
			}
		}
	}
}

func TestNameDomainsValidatesArguments(t *testing.T) {
	data := namedDataset("repo", []string{"Alpha", "Beta"}, []string{"a/x.py", "a/y.py"})
	invalid := []NameOptions{
		{Top: 0, MinCount: 2, MaxDocumentFrequency: 0.35, MinExclusivity: 0.5},
		{Top: 1, MinCount: 0, MaxDocumentFrequency: 0.35, MinExclusivity: 0.5},
		{Top: 1, MinCount: 2, MaxDocumentFrequency: 0, MinExclusivity: 0.5},
		{Top: 1, MinCount: 2, MaxDocumentFrequency: 1.5, MinExclusivity: 0.5},
		{Top: 1, MinCount: 2, MaxDocumentFrequency: 0.35, MinExclusivity: 0},
	}
	for _, opts := range invalid {
		if _, err := NameDomains(data, []Domain{{Members: []int{0}, Tightness: 1.0}}, opts); err == nil {
			t.Fatalf("expected validation error for %+v", opts)
		}
	}
}

func TestNameDomainsJoinsTopTokens(t *testing.T) {
	data := namedDataset(
		"repo",
		[]string{"AuthToken", "AuthToken", "AuthToken"},
		[]string{"src/auth/token.go", "src/auth/token.go", "src/auth/token.go"},
	)
	options := DefaultNameOptions()
	options.Top = 2
	names, err := NameDomains(data, []Domain{{Members: []int{0, 1, 2}, Tightness: 1.0}}, options)
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "auth token" && names[0] != "token auth" {
		t.Fatalf("name = %q", names[0])
	}
}
