package datastore

import (
	"context"
	"fmt"
	"strings"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type Enricher = enrich.Enricher
type Fact = enrich.Fact
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput
type Metadata = enrich.Metadata
type SourceSpan = enrich.SourceSpan
type SubjectRef = enrich.SubjectRef

const ActivationAlways = enrich.ActivationAlways

var (
	fileSubject    = enrich.FileSubject
	lineForOffset  = enrich.LineForOffset
	matchLanguages = enrich.MatchLanguages
)

func DatastoreGlue() Enricher {
	return enrich.NewEnricher(
		Metadata{ID: "datastore.glue", Name: "Datastore glue", Mode: ActivationAlways},
		matchLanguages("go", "python", "javascript", "typescript", "c-sharp", "xml", "go-mod", "json", "python-requirements"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			lower := strings.ToLower(string(input.Source))
			candidates := []struct {
				needle string
				name   string
				tech   string
			}{
				{"redis", "redis", "Redis"},
				{"spanner", "spanner", "Spanner"},
				{"alloydb", "alloydb", "AlloyDB"},
				{"postgres", "postgres", "PostgreSQL"},
				{"secretmanager", "secretmanager", "Secret Manager"},
				{"opentelemetry", "opentelemetry", "OpenTelemetry"},
			}
			for _, candidate := range candidates {
				if !strings.Contains(lower, candidate.needle) {
					continue
				}
				line := lineForOffset(lower, strings.Index(lower, candidate.needle))
				if err := emit.EmitFact(Fact{
					Type:            "datastore.dependency",
					StableKey:       fmt.Sprintf("datastore.dependency:%s:%s", input.RelPath, candidate.name),
					Subject:         fileSubject(input.RelPath),
					Object:          SubjectRef{Kind: "datastore", StableKey: "datastore:" + candidate.name, Name: candidate.name},
					Relationship:    "uses",
					Source:          SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
					Confidence:      0.72,
					Name:            candidate.name,
					Tags:            []string{"arch:datastore", "datastore:" + candidate.name},
					Attributes:      map[string]string{"name": candidate.name, "technology": candidate.tech},
					VisibilityHints: map[string]float64{"high_signal": 0.5},
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)
}
