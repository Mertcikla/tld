package typescript

import (
	"context"
	"regexp"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type ActivationSignal = enrich.ActivationSignal
type Enricher = enrich.Enricher
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput
type Metadata = enrich.Metadata
type RoutePattern = enrich.RoutePattern

const (
	ActivationImportOrDependency = enrich.ActivationImportOrDependency
	SignalDependency             = enrich.SignalDependency
	SignalImport                 = enrich.SignalImport
)

var matchLanguages = enrich.MatchLanguages

func Prisma() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID:   "ts.prisma",
			Name: "Prisma ORM queries",
			Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{
				{Kind: SignalImport, Value: "@prisma/client"},
				{Kind: SignalDependency, Value: "@prisma/client"},
				{Kind: SignalDependency, Value: "prisma"},
			},
		},
		matchLanguages("typescript", "javascript"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return enrich.EmitMatches(input, emit, []*RoutePattern{{
				Re:        regexp.MustCompile(`\bprisma\.([A-Za-z_][A-Za-z0-9_]*)\.(findMany|findUnique|findFirst|create|createMany|update|updateMany|delete|deleteMany|upsert|aggregate|count)\b`),
				FactType:  "orm.query",
				Framework: "prisma",
				Tags:      []string{"orm:prisma"},
				Custom: func(match []string) (name string, attrs map[string]string, tags []string) {
					return match[1] + "." + match[2], map[string]string{"orm": "prisma", "model": match[1], "operation": match[2]}, []string{"orm:prisma"}
				},
			}})
		},
	)
}
