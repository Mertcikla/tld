package enrichertest

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type Case struct {
	Name     string
	Enricher enrich.Enricher
	Input    enrich.FileInput
	Signals  []enrich.ActivationSignal
	Want     Fact
}

type Fact struct {
	Type       string
	Tag        string
	Name       string
	Attribute  string
	AttrValue  string
	StablePart string
}

func Run(t *testing.T, cases ...Case) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Helper()
			input := tc.Input
			if input.RelPath == "" {
				input.RelPath = "snippet"
			}
			input.Signals = nil

			registry := enrich.NewRegistry(tc.Enricher)
			if tc.Enricher.Metadata().Mode == enrich.ActivationImportOrDependency && len(tc.Signals) > 0 {
				facts, _, err := registry.EnrichFile(context.Background(), input)
				if err != nil {
					t.Fatalf("enrich without signals: %v", err)
				}
				if len(facts) != 0 {
					t.Fatalf("expected enricher to stay inactive without activation signals, got %+v", facts)
				}
			}

			input.Signals = tc.Signals
			facts, _, err := registry.EnrichFile(context.Background(), input)
			if err != nil {
				t.Fatalf("enrich with signals: %v", err)
			}
			if !hasFact(facts, tc.Want) {
				t.Fatalf("missing expected fact %+v in %+v", tc.Want, facts)
			}
		})
	}
}

func hasFact(facts []enrich.Fact, want Fact) bool {
	for _, fact := range facts {
		if want.Type != "" && fact.Type != want.Type {
			continue
		}
		if want.Tag != "" && !contains(fact.Tags, want.Tag) {
			continue
		}
		if want.Name != "" && fact.Name != want.Name {
			continue
		}
		if want.Attribute != "" && fact.Attributes[want.Attribute] != want.AttrValue {
			continue
		}
		if want.StablePart != "" && !strings.Contains(fact.StableKey, want.StablePart) {
			continue
		}
		return true
	}
	return false
}

func contains(values []string, want string) bool {
	return slices.Contains(values, want)
}
