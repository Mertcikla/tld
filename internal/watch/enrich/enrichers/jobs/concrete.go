package jobs

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
)

var jobBindingPatterns = []struct {
	language string
	factType string
	re       *regexp.Regexp
}{
	{"typescript", "job.schedule", regexp.MustCompile(`cron\.schedule\(\s*["']([^"']+)["']`)},
	{"go", "job.schedule", regexp.MustCompile(`\.AddFunc\(\s*"([^"]+)"\s*,`)},
	{"python", "job.schedule", regexp.MustCompile(`add_job\([^)]*trigger\s*=\s*["']cron["'][^)]*(?:id\s*=\s*["']([^"']+)["']|minute\s*=\s*["']?([^"',)]+))`)},
	{"java", "job.schedule", regexp.MustCompile(`@Scheduled\(\s*cron\s*=\s*"([^"]+)"`)},
	{"rust", "job.schedule", regexp.MustCompile(`Job::new_async\(\s*"([^"]+)"`)},
	{"cpp", "job.schedule", regexp.MustCompile(`schedule_every\(\s*"([^"]+)"`)},
}

func ConcreteBindings() enrich.Enricher {
	return enrich.NewEnricher(
		enrich.Metadata{ID: "jobs.concrete_bindings", Name: "Concrete job bindings", Mode: enrich.ActivationAlways},
		func(input enrich.FileInput) bool {
			for _, pattern := range jobBindingPatterns {
				if strings.EqualFold(input.Language, pattern.language) {
					return true
				}
			}
			return false
		},
		func(_ context.Context, input enrich.FileInput, emit enrich.FactEmitter) error {
			source := string(input.Source)
			for _, pattern := range jobBindingPatterns {
				if !strings.EqualFold(input.Language, pattern.language) {
					continue
				}
				for _, match := range pattern.re.FindAllStringSubmatchIndex(source, -1) {
					name := firstCapture(source, match)
					if name == "" {
						continue
					}
					line := enrich.LineForOffset(source, match[0])
					if err := emit.EmitFact(enrich.Fact{
						Type:         pattern.factType,
						StableKey:    fmt.Sprintf("%s:%s:%s:%d", pattern.factType, input.RelPath, name, line),
						Subject:      enrich.SubjectForLine(input, line),
						Object:       enrich.SubjectRef{Kind: pattern.factType, StableKey: pattern.factType + ":" + name, FilePath: input.RelPath, Name: name},
						Relationship: "runs_on_schedule",
						Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
						Confidence:   0.86,
						Name:         name,
						Tags:         []string{"category:jobs", "job:schedule"},
						Attributes:   map[string]string{"schedule": name, "language": input.Language},
						VisibilityHints: map[string]float64{
							"high_signal": 0.75,
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

func firstCapture(source string, match []int) string {
	for i := 2; i+1 < len(match); i += 2 {
		if match[i] >= 0 && match[i+1] >= match[i] {
			return source[match[i]:match[i+1]]
		}
	}
	return ""
}
