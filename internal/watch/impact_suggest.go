package watch

import (
	"context"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

// SuggestionOptions configures embedding-based ownership suggestions for
// unmapped code. Suggestions are always weak/inferred and require human
// acceptance before becoming bindings.
type SuggestionOptions struct {
	RepoRoot       string
	Elements       map[string]*workspace.Element
	Unmapped       []string
	Embedding      EmbeddingConfig
	MaxSuggestions int
	MinScore       float64
}

// SuggestBindings ranks authored elements as likely owners of unmapped files.
func SuggestBindings(ctx context.Context, opts SuggestionOptions) ([]BindingSuggestion, error) {
	if len(opts.Unmapped) == 0 || len(opts.Elements) == 0 {
		return nil, nil
	}
	provider, err := NewEmbeddingProvider(opts.Embedding)
	if err != nil {
		return nil, err
	}
	if closer, ok := provider.(ClosableProvider); ok {
		defer func() { _ = closer.Close() }()
	}
	if provider.ModelID().Provider == "none" {
		return nil, nil
	}

	refs := make([]string, 0, len(opts.Elements))
	for ref := range opts.Elements {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	elementInputs := make([]EmbeddingInput, 0, len(refs))
	for _, ref := range refs {
		element := opts.Elements[ref]
		if element == nil {
			continue
		}
		elementInputs = append(elementInputs, EmbeddingInput{
			OwnerType: "populate_resource",
			OwnerKey:  ref,
			Text:      elementSuggestionText(element),
		})
	}
	elementVectors, err := provider.Embed(ctx, elementInputs)
	if err != nil {
		return nil, err
	}

	fileInputs := make([]EmbeddingInput, 0, len(opts.Unmapped))
	for _, file := range opts.Unmapped {
		fileInputs = append(fileInputs, EmbeddingInput{
			OwnerType: "query",
			OwnerKey:  file,
			Text:      file,
		})
	}
	fileVectors, err := provider.Embed(ctx, fileInputs)
	if err != nil {
		return nil, err
	}
	if len(elementVectors) != len(elementInputs) || len(fileVectors) != len(fileInputs) {
		return nil, nil
	}

	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = 0.30
	}
	maxSuggestions := opts.MaxSuggestions
	if maxSuggestions <= 0 {
		maxSuggestions = 3
	}

	var out []BindingSuggestion
	for fileIndex, file := range opts.Unmapped {
		type scored struct {
			ref   string
			score float64
		}
		var ranked []scored
		for elementIndex, input := range elementInputs {
			score := CosineSimilarity(fileVectors[fileIndex], elementVectors[elementIndex])
			if score < minScore {
				continue
			}
			ranked = append(ranked, scored{ref: input.OwnerKey, score: score})
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].score == ranked[j].score {
				return ranked[i].ref < ranked[j].ref
			}
			return ranked[i].score > ranked[j].score
		})
		if len(ranked) > maxSuggestions {
			ranked = ranked[:maxSuggestions]
		}
		for _, item := range ranked {
			name := ""
			if element := opts.Elements[item.ref]; element != nil {
				name = element.Name
			}
			out = append(out, BindingSuggestion{
				File:        file,
				ElementRef:  item.ref,
				ElementName: name,
				Score:       item.score,
			})
		}
	}
	return out, nil
}

func elementSuggestionText(element *workspace.Element) string {
	parts := []string{
		strings.TrimSpace(element.Name),
		strings.TrimSpace(element.Kind),
		strings.TrimSpace(element.Description),
		strings.TrimSpace(element.Technology),
		strings.TrimSpace(element.FilePath),
	}
	var nonEmpty []string
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, " | ")
}
