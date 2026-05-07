package datastore

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestDatastoreGlue(t *testing.T) {
	enrichertest.Run(t, []enrichertest.Case{
		{
			Name:     "datastore glue matches redis connection string",
			Enricher: DatastoreGlue(),
			Input: enrich.FileInput{
				RelPath:  "cache.go",
				Language: "go",
				Source:   []byte(`func connect() { _ = "redis://cache:6379" }`),
			},
			Want: enrichertest.Fact{Type: "datastore.dependency", Tag: "datastore:redis", Name: "redis"},
		},
	}...)
}

func TestDatastoreGlueNegatives(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"ignores redis in comments", `// TODO: consider using redis://cache:6379`},
		{"ignores bare redis mention", `var x = "redis"`},
		{"ignores postgres in comments", `/* postgres://localhost */`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := enrich.FileInput{
				RelPath:  "test.go",
				Language: "go",
				Source:   []byte(tc.source),
			}
			emitter := &factCollector{}
			err := DatastoreGlue().EnrichFile(context.Background(), input, emitter)
			if err != nil {
				t.Fatalf("enrich: %v", err)
			}
			if len(emitter.facts) > 0 {
				t.Fatalf("expected no facts for source %q, got %v", tc.source, emitter.facts)
			}
		})
	}
}

type factCollector struct {
	facts []enrich.Fact
}

func (c *factCollector) EmitFact(f enrich.Fact) error {
	c.facts = append(c.facts, f)
	return nil
}

func (c *factCollector) Warn(w enrich.Warning) {}
