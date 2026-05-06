package python

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

func PythonLocust() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID: "python.locust", Name: "Locust HTTP traffic", Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{{Kind: SignalImport, Value: "locust"}, {Kind: SignalDependency, Value: "locust"}},
		},
		matchLanguages("python"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return enrich.EmitMatches(input, emit, []*RoutePattern{{
				Re:          regexp.MustCompile(`\bclient\.(get|post|put|delete|patch)\(\s*["']([^"']+)["']`),
				FactType:    "http.client",
				Framework:   "locust",
				MethodGroup: 1,
				PathGroup:   2,
				Tags:        []string{"http:client", "framework:locust"},
			}})
		},
	)
}
