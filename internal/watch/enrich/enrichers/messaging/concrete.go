package messaging

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
)

var messagingBindingPatterns = []struct {
	language     string
	factType     string
	relationship string
	re           *regexp.Regexp
}{
	{"typescript", "messaging.topic", "publishes", regexp.MustCompile(`\btopic\s*:\s*["']([^"']+)["']`)},
	{"typescript", "messaging.queue", "publishes", regexp.MustCompile(`\bnew\s+Queue\(\s*["']([^"']+)["']`)},
	{"go", "messaging.topic", "publishes", regexp.MustCompile(`\bTopic:\s*"([^"]+)"`)},
	{"go", "messaging.queue", "consumes", regexp.MustCompile(`\bQueue:\s*"([^"]+)"`)},
	{"python", "messaging.queue", "consumes", regexp.MustCompile(`\bqueue\s*=\s*["']([^"']+)["']`)},
	{"python", "messaging.topic", "publishes", regexp.MustCompile(`\btopic\s*=\s*["']([^"']+)["']`)},
	{"java", "messaging.topic", "publishes", regexp.MustCompile(`@KafkaListener\([^)]*topics\s*=\s*"([^"]+)"`)},
	{"rust", "messaging.topic", "subscribes_to", regexp.MustCompile(`\.subscribe\(\s*"([^"]+)"\s*\)`)},
	{"cpp", "messaging.topic", "subscribes_to", regexp.MustCompile(`\b(?:subscribe|Subscribe)\(\s*"([^"]+)"`)},
}

func ConcreteBindings() enrich.Enricher {
	return enrich.NewEnricher(
		enrich.Metadata{ID: "messaging.concrete_bindings", Name: "Concrete messaging bindings", Mode: enrich.ActivationAlways},
		func(input enrich.FileInput) bool {
			for _, pattern := range messagingBindingPatterns {
				if strings.EqualFold(input.Language, pattern.language) {
					return true
				}
			}
			return false
		},
		func(_ context.Context, input enrich.FileInput, emit enrich.FactEmitter) error {
			source := string(input.Source)
			for _, pattern := range messagingBindingPatterns {
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
						Relationship: pattern.relationship,
						Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
						Confidence:   0.9,
						Name:         name,
						Tags:         []string{"category:messaging", strings.ReplaceAll(pattern.factType, ".", ":")},
						Attributes:   map[string]string{"name": name, "language": input.Language},
						VisibilityHints: map[string]float64{
							"high_signal": 0.85,
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
