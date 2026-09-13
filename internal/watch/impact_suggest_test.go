package watch

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestSuggestBindingsRanksLikelyOwner(t *testing.T) {
	elements := map[string]*workspace.Element{
		"payments": {Name: "Payments Service", Kind: "service", Description: "handles refund payment worker"},
		"orders":   {Name: "Orders Service", Kind: "service", Description: "order checkout lifecycle"},
	}
	suggestions, err := SuggestBindings(context.Background(), SuggestionOptions{
		RepoRoot:  t.TempDir(),
		Elements:  elements,
		Unmapped:  []string{"internal/refund_worker.go"},
		Embedding: EmbeddingConfig{Provider: "local-lexical", Model: DefaultLexicalModel, Dimension: DefaultLexicalDimension},
		MinScore:  0.01,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) == 0 {
		t.Fatalf("expected at least one suggestion")
	}
	if suggestions[0].ElementRef != "payments" {
		t.Fatalf("top suggestion = %q, want payments (%+v)", suggestions[0].ElementRef, suggestions)
	}
	if suggestions[0].Score <= 0 {
		t.Fatalf("expected positive score, got %+v", suggestions[0])
	}
}

func TestSuggestBindingsNoopProviderReturnsNothing(t *testing.T) {
	suggestions, err := SuggestBindings(context.Background(), SuggestionOptions{
		RepoRoot:  t.TempDir(),
		Elements:  map[string]*workspace.Element{"a": {Name: "A"}},
		Unmapped:  []string{"foo.go"},
		Embedding: EmbeddingConfig{Provider: "none"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("noop provider should return no suggestions, got %+v", suggestions)
	}
}
