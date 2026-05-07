package datastore

import (
	"context"
	"fmt"
	"regexp"
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
	tokenCleanupRE = regexp.MustCompile(`(?m)(^|[^:])//.*$|#.*$|/\*[\s\S]*?\*/|<!--[\s\S]*?-->`)
)

func DatastoreGlue() Enricher {
	return enrich.NewEnricher(
		Metadata{ID: "datastore.glue", Name: "Datastore glue", Mode: ActivationAlways},
		matchLanguages("go", "python", "javascript", "typescript", "c-sharp", "xml", "go-mod", "json", "python-requirements"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			source := string(input.Source)
			scannable := tokenCleanupRE.ReplaceAllString(source, "$1")
			lower := strings.ToLower(scannable)
			candidates := []struct {
				needle string
				name   string
				tech   string
			}{
				{"redis://", "redis", "Redis"},
				{"github.com/redis/go-redis", "redis", "Redis"},
				{"spanner.googleapis.com", "spanner", "Spanner"},
				{"alloydb.googleapis.com", "alloydb", "AlloyDB"},
				{"postgres://", "postgres", "PostgreSQL"},
				{"postgresql://", "postgres", "PostgreSQL"},
				{"github.com/lib/pq", "postgres", "PostgreSQL"},
				{"secretmanager.googleapis.com", "secretmanager", "Secret Manager"},
				{"go.opentelemetry.io/otel", "opentelemetry", "OpenTelemetry"},
			}
			for _, candidate := range candidates {
				if !strings.Contains(lower, candidate.needle) {
					continue
				}
				idx := strings.Index(lower, candidate.needle)
				line := lineForOffset(scannable, idx)
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
