package storage

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
)

var storageBindingPatterns = []struct {
	language string
	factType string
	re       *regexp.Regexp
}{
	{"typescript", "storage.collection", regexp.MustCompile(`\.collection\(\s*["']([^"']+)["']`)},
	{"typescript", "storage.bucket", regexp.MustCompile(`\.bucket\(\s*["']([^"']+)["']`)},
	{"go", "storage.collection", regexp.MustCompile(`\.Collection\(\s*"([^"]+)"\s*\)`)},
	{"go", "storage.bucket", regexp.MustCompile(`Bucket:\s*aws\.String\(\s*"([^"]+)"\s*\)`)},
	{"python", "storage.collection", regexp.MustCompile(`\[\s*["']([^"']+)["']\s*\]\.(?:find|insert|update|delete)`)},
	{"python", "storage.bucket", regexp.MustCompile(`Bucket\s*=\s*["']([^"']+)["']`)},
	{"java", "storage.collection", regexp.MustCompile(`getCollection\(\s*"([^"]+)"\s*\)`)},
	{"rust", "storage.collection", regexp.MustCompile(`\.collection::<[^>]+>\(\s*"([^"]+)"\s*\)`)},
	{"cpp", "storage.collection", regexp.MustCompile(`collection\(\s*"([^"]+)"\s*\)`)},
}

func ConcreteBindings() enrich.Enricher {
	return enrich.NewEnricher(
		enrich.Metadata{ID: "storage.concrete_bindings", Name: "Concrete storage bindings", Mode: enrich.ActivationAlways},
		func(input enrich.FileInput) bool {
			for _, pattern := range storageBindingPatterns {
				if strings.EqualFold(input.Language, pattern.language) {
					return true
				}
			}
			return false
		},
		func(_ context.Context, input enrich.FileInput, emit enrich.FactEmitter) error {
			source := string(input.Source)
			for _, pattern := range storageBindingPatterns {
				if !strings.EqualFold(input.Language, pattern.language) {
					continue
				}
				for _, match := range pattern.re.FindAllStringSubmatchIndex(source, -1) {
					if len(match) < 4 {
						continue
					}
					name := source[match[2]:match[3]]
					line := enrich.LineForOffset(source, match[0])
					if err := emit.EmitFact(enrich.Fact{
						Type:         pattern.factType,
						StableKey:    fmt.Sprintf("%s:%s:%s:%d", pattern.factType, input.RelPath, name, line),
						Subject:      enrich.SubjectForLine(input, line),
						Object:       enrich.SubjectRef{Kind: pattern.factType, StableKey: pattern.factType + ":" + name, FilePath: input.RelPath, Name: name},
						Relationship: "binds_storage",
						Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
						Confidence:   0.88,
						Name:         name,
						Tags:         []string{"category:storage", strings.ReplaceAll(pattern.factType, ".", ":")},
						Attributes:   map[string]string{"name": name, "language": input.Language},
						VisibilityHints: map[string]float64{
							"high_signal": 0.8,
						},
					}); err != nil {
						return err
					}
				}
			}
			return nil
		},
	)
}
