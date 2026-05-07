package pattern

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type Spec struct {
	ID           string
	Name         string
	Category     string
	Languages    []string
	Mode         enrich.ActivationMode
	Triggers     []enrich.ActivationSignal
	FactType     string
	Relationship string
	ObjectKind   string
	SourceTokens []string
	PathTokens   []string
	Tags         []string
	Attributes   map[string]string
}

var commentsRE = regexp.MustCompile(`(?m)//.*$|#.*$|/\*[\s\S]*?\*/|<!--[\s\S]*?-->`)

func New(spec Spec) enrich.Enricher {
	return enrich.NewEnricher(
		enrich.Metadata{
			ID:       spec.ID,
			Name:     spec.Name,
			Mode:     spec.Mode,
			Triggers: spec.Triggers,
		},
		func(input enrich.FileInput) bool {
			if ignoredPath(input.RelPath) {
				return false
			}
			if len(spec.Languages) == 0 {
				return true
			}
			for _, language := range spec.Languages {
				if strings.EqualFold(strings.TrimSpace(language), strings.TrimSpace(input.Language)) {
					return true
				}
			}
			return pathMatches(input.RelPath, spec.PathTokens)
		},
		func(ctx context.Context, input enrich.FileInput, emit enrich.FactEmitter) error {
			line := matchLine(input, spec)
			if line == 0 {
				return nil
			}
			attrs := map[string]string{
				"category":   spec.Category,
				"technology": spec.Name,
				"framework":  spec.ID,
			}
			maps.Copy(attrs, spec.Attributes)
			tags := []string{"arch:glue", "category:" + tagValue(spec.Category), "technology:" + tagValue(spec.Name)}
			tags = append(tags, spec.Tags...)
			objectKind := spec.ObjectKind
			if objectKind == "" {
				objectKind = spec.FactType
			}
			return emit.EmitFact(enrich.Fact{
				Type:         spec.FactType,
				StableKey:    fmt.Sprintf("%s:%s:%s:%d", spec.FactType, spec.ID, input.RelPath, line),
				Subject:      enrich.SubjectForLine(input, line),
				Object:       enrich.SubjectRef{Kind: objectKind, StableKey: spec.FactType + ":" + spec.ID, FilePath: input.RelPath, Name: spec.Name},
				Relationship: spec.Relationship,
				Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
				Confidence:   0.78,
				Name:         spec.Name,
				Tags:         tags,
				Attributes:   attrs,
				VisibilityHints: map[string]float64{
					"high_signal": 0.6,
				},
			})
		},
	)
}

func FromSpecs(specs []Spec) []enrich.Enricher {
	out := make([]enrich.Enricher, 0, len(specs))
	for _, spec := range specs {
		out = append(out, New(spec))
	}
	return out
}

func matchLine(input enrich.FileInput, spec Spec) int {
	if len(spec.SourceTokens) > 0 {
		return matchSourceTokens(input, spec.SourceTokens)
	}
	if pathMatches(input.RelPath, spec.PathTokens) {
		return 1
	}
	return 0
}

func matchSourceTokens(input enrich.FileInput, tokens []string) int {
	source := commentsRE.ReplaceAllString(string(input.Source), "")
	lower := strings.ToLower(source)
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if idx := strings.Index(lower, token); idx >= 0 {
			return enrich.LineForOffset(source, idx)
		}
	}
	return 0
}

func pathMatches(relPath string, tokens []string) bool {
	rel := strings.ToLower(filepath.ToSlash(relPath))
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" && strings.Contains(rel, token) {
			return true
		}
	}
	return false
}

func ignoredPath(relPath string) bool {
	parts := strings.SplitSeq(filepath.ToSlash(relPath), "/")
	for part := range parts {
		switch strings.ToLower(part) {
		case ".git", "node_modules", "vendor", "dist", "build", "coverage", "generated", "gen":
			return true
		}
	}
	return false
}

func tagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" / ", "-", "/", "-", " ", "-", "&", "and", ".", "", "+", "plus").Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}
