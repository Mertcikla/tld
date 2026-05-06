package pattern

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

func TestPatternEnricherIgnoresCommentedMatches(t *testing.T) {
	spec := Spec{
		ID:           "demo.pattern",
		Name:         "Demo Pattern",
		Category:     "demo",
		Languages:    []string{"go"},
		Mode:         enrich.ActivationAlways,
		FactType:     "demo.fact",
		Relationship: "uses",
		SourceTokens: []string{"real_call"},
	}
	facts, _, err := enrich.NewRegistry(New(spec)).EnrichFile(context.Background(), enrich.FileInput{
		RelPath:  "demo.go",
		Language: "go",
		Source:   []byte(`// real_call should not match`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Fatalf("expected no facts for commented match, got %+v", facts)
	}
}

func TestPatternEnricherIgnoresGeneratedAndVendorPaths(t *testing.T) {
	spec := Spec{
		ID:           "demo.pattern",
		Name:         "Demo Pattern",
		Category:     "demo",
		Languages:    []string{"go"},
		Mode:         enrich.ActivationAlways,
		FactType:     "demo.fact",
		Relationship: "uses",
		SourceTokens: []string{"real_call"},
	}
	for _, relPath := range []string{"vendor/pkg/demo.go", "generated/demo.go"} {
		facts, _, err := enrich.NewRegistry(New(spec)).EnrichFile(context.Background(), enrich.FileInput{
			RelPath:  relPath,
			Language: "go",
			Source:   []byte(`real_call()`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(facts) != 0 {
			t.Fatalf("expected no facts for ignored path %s, got %+v", relPath, facts)
		}
	}
}
