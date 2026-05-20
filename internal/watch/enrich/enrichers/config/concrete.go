package config

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
)

var configBindingPatterns = []struct {
	language string
	source   string
	re       *regexp.Regexp
}{
	{"typescript", "process.env", regexp.MustCompile(`\bprocess\.env\.([A-Z][A-Z0-9_]*)`)},
	{"typescript", "import.meta.env", regexp.MustCompile(`\bimport\.meta\.env\.([A-Z][A-Z0-9_]*)`)},
	{"typescript", "browser.storage", regexp.MustCompile(`\b(?:localStorage|sessionStorage)\.(?:getItem|setItem)\(\s*["']([^"']+)["']`)},
	{"go", "os.Getenv", regexp.MustCompile(`\bos\.Getenv\(\s*"([^"]+)"\s*\)`)},
	{"go", "viper", regexp.MustCompile(`\bviper\.Get(?:String|Bool|Int|Float64|Duration)?\(\s*"([^"]+)"\s*\)`)},
	{"python", "os.environ", regexp.MustCompile(`\bos\.(?:getenv|environ\.get)\(\s*["']([^"']+)["']`)},
	{"python", "os.environ", regexp.MustCompile(`\bos\.environ\[\s*["']([^"']+)["']\s*\]`)},
	{"java", "System.getenv", regexp.MustCompile(`\bSystem\.getenv\(\s*"([^"]+)"\s*\)`)},
	{"java", "Spring @Value", regexp.MustCompile(`@Value\(\s*"\$\{([^}:"]+)`)},
	{"rust", "std::env::var", regexp.MustCompile(`\bstd::env::var\(\s*"([^"]+)"\s*\)`)},
	{"cpp", "std::getenv", regexp.MustCompile(`\b(?:std::)?getenv\(\s*"([^"]+)"\s*\)`)},
}

func ConcreteBindings() enrich.Enricher {
	return enrich.NewEnricher(
		enrich.Metadata{ID: "config.concrete_bindings", Name: "Concrete config bindings", Mode: enrich.ActivationAlways},
		func(input enrich.FileInput) bool {
			for _, pattern := range configBindingPatterns {
				if strings.EqualFold(input.Language, pattern.language) {
					return true
				}
			}
			return false
		},
		func(_ context.Context, input enrich.FileInput, emit enrich.FactEmitter) error {
			source := string(input.Source)
			for _, pattern := range configBindingPatterns {
				if !strings.EqualFold(input.Language, pattern.language) {
					continue
				}
				for _, match := range pattern.re.FindAllStringSubmatchIndex(source, -1) {
					if len(match) < 4 {
						continue
					}
					key := source[match[2]:match[3]]
					line := enrich.LineForOffset(source, match[0])
					kind := "env"
					if strings.Contains(pattern.source, "storage") {
						kind = "browser_storage"
					}
					if err := emit.EmitFact(enrich.Fact{
						Type:         "config.env",
						StableKey:    fmt.Sprintf("config.binding:%s:%s:%d", input.RelPath, key, line),
						Subject:      enrich.SubjectForLine(input, line),
						Object:       enrich.SubjectRef{Kind: "config.key", StableKey: "config.key:" + key, FilePath: input.RelPath, Name: key},
						Relationship: "reads_config",
						Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
						Confidence:   0.92,
						Name:         key,
						Tags:         []string{"category:config", "config:key", "config:" + kind},
						Attributes:   map[string]string{"key": key, "source": pattern.source, "binding_kind": kind, "language": input.Language},
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
