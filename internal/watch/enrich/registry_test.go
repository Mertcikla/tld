package enrich

import (
	"context"
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/analyzer"
)

func TestRegistryActivatesImportGatedEnrichers(t *testing.T) {
	source := []byte(`package main

import "github.com/go-chi/chi/v5"

func routes(r chi.Router) {
	r.Get("/users/{id}", getUser)
}

func getUser() {}
`)
	input := FileInput{
		RelPath:  "routes.go",
		Language: "go",
		Source:   source,
		Parsed: &analyzer.Result{Refs: []analyzer.Ref{{
			Kind:       "import",
			TargetPath: "github.com/go-chi/chi/v5",
			FilePath:   "routes.go",
			Line:       3,
		}}},
	}

	withoutSignals, _, err := NewRegistry(GoChi()).EnrichFile(context.Background(), input)
	if err != nil {
		t.Fatalf("enrich without signals: %v", err)
	}
	if len(withoutSignals) != 0 {
		t.Fatalf("expected inactive chi enricher without signals, got %+v", withoutSignals)
	}

	input.Signals = ImportSignals(input.Parsed.Refs)
	withSignals, _, err := NewRegistry(GoChi()).EnrichFile(context.Background(), input)
	if err != nil {
		t.Fatalf("enrich with signals: %v", err)
	}
	if len(withSignals) != 1 || withSignals[0].Type != "http.route" || !containsTag(withSignals[0].Tags, "framework:chi") {
		t.Fatalf("expected chi route fact, got %+v", withSignals)
	}
}

func TestRegistryRejectsInvalidFacts(t *testing.T) {
	bad := enricherFunc{
		meta: Metadata{ID: "bad", Mode: ActivationAlways},
		run: func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return emit.EmitFact(Fact{Type: "demo.fact"})
		},
	}
	_, _, err := NewRegistry(bad).EnrichFile(context.Background(), FileInput{RelPath: "demo.go"})
	if err == nil || !strings.Contains(err.Error(), "stable key") {
		t.Fatalf("expected stable key validation error, got %v", err)
	}
}

func TestDefaultRegistryEmitsDemoFacts(t *testing.T) {
	tests := []struct {
		name     string
		input    FileInput
		signals  []ActivationSignal
		wantType string
		wantTag  string
	}{
		{
			name: "express route",
			input: FileInput{
				RelPath:  "server.ts",
				Language: "typescript",
				Source:   []byte(`router.get("/api/users", listUsers)`),
			},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "express"}},
			wantType: "http.route",
			wantTag:  "framework:express",
		},
		{
			name: "next page route",
			input: FileInput{
				RelPath:  "src/app/users/[id]/page.tsx",
				Language: "typescript",
				Source:   []byte(`export default function Page() { return null }`),
			},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "next"}},
			wantType: "frontend.route",
			wantTag:  "framework:nextjs",
		},
		{
			name: "prisma query",
			input: FileInput{
				RelPath:  "db.ts",
				Language: "typescript",
				Source:   []byte(`await prisma.user.findMany()`),
			},
			signals:  []ActivationSignal{{Kind: SignalDependency, Value: "@prisma/client"}},
			wantType: "orm.query",
			wantTag:  "orm:prisma",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Signals = tt.signals
			facts, _, err := NewDefaultRegistry().EnrichFile(context.Background(), tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !hasFact(facts, tt.wantType, tt.wantTag) {
				t.Fatalf("missing %s/%s in %+v", tt.wantType, tt.wantTag, facts)
			}
		})
	}
}

func hasFact(facts []Fact, factType, tag string) bool {
	for _, fact := range facts {
		if fact.Type == factType && containsTag(fact.Tags, tag) {
			return true
		}
	}
	return false
}

func containsTag(tags []string, tag string) bool {
	for _, item := range tags {
		if item == tag {
			return true
		}
	}
	return false
}
