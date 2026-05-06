package enrich

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/internal/analyzer"
)

type enricherFunc struct {
	meta  Metadata
	match func(FileInput) bool
	run   func(context.Context, FileInput, FactEmitter) error
}

func NewEnricher(meta Metadata, match func(FileInput) bool, run func(context.Context, FileInput, FactEmitter) error) Enricher {
	return enricherFunc{meta: meta, match: match, run: run}
}

func (e enricherFunc) Metadata() Metadata { return e.meta }
func (e enricherFunc) MatchFile(input FileInput) bool {
	if e.match == nil {
		return true
	}
	return e.match(input)
}
func (e enricherFunc) EnrichFile(ctx context.Context, input FileInput, emit FactEmitter) error {
	if e.run == nil {
		return nil
	}
	return e.run(ctx, input, emit)
}

type RoutePattern struct {
	Re          *regexp.Regexp
	FactType    string
	Method      string
	Framework   string
	MethodGroup int
	PathGroup   int
	Tags        []string
	Custom      func([]string) (string, map[string]string, []string)
}

func RouteRegexEnricher(id, name, languages string, triggers []ActivationSignal, patterns []*RoutePattern) Enricher {
	return enricherFunc{
		meta: Metadata{ID: id, Name: name, Mode: ActivationImportOrDependency, Triggers: triggers},
		match: func(input FileInput) bool {
			allowed := strings.Split(languages, ",")
			return matchLanguages(allowed...)(input)
		},
		run: func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return emitMatches(input, emit, patterns)
		},
	}
}

func EmitMatches(input FileInput, emit FactEmitter, patterns []*RoutePattern) error {
	source := string(input.Source)
	for _, pattern := range patterns {
		matches := pattern.Re.FindAllStringSubmatchIndex(source, -1)
		for _, indexes := range matches {
			match := submatches(source, indexes)
			line := lineForOffset(source, indexes[0])
			factType := pattern.FactType
			if factType == "" {
				factType = "http.route"
			}
			name, attrs, tags := routeFactValues(pattern, match)
			if name == "" {
				continue
			}
			subject := subjectForLine(input, line)
			key := fmt.Sprintf("%s:%s:%s:%s:%d", factType, pattern.Framework, input.RelPath, name, line)
			if err := emit.EmitFact(Fact{
				Type:         factType,
				StableKey:    key,
				Subject:      subject,
				Object:       SubjectRef{Kind: factType, StableKey: factType + ":" + pattern.Framework + ":" + name, FilePath: input.RelPath, Name: name},
				Relationship: "declares",
				Source:       SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
				Confidence:   0.90,
				Name:         name,
				Tags:         tags,
				Attributes:   attrs,
				VisibilityHints: map[string]float64{
					"high_signal": 1,
				},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func emitMatches(input FileInput, emit FactEmitter, patterns []*RoutePattern) error {
	return EmitMatches(input, emit, patterns)
}

func routeFactValues(pattern *RoutePattern, match []string) (string, map[string]string, []string) {
	if pattern.Custom != nil {
		return pattern.Custom(match)
	}
	method := strings.ToUpper(pattern.Method)
	routePath := ""
	if pattern.PathGroup > 0 && pattern.PathGroup < len(match) {
		routePath = match[pattern.PathGroup]
	} else if len(match) > 1 {
		routePath = match[1]
	}
	if pattern.MethodGroup > 0 && pattern.MethodGroup < len(match) {
		method = strings.ToUpper(match[pattern.MethodGroup])
	}
	attrs := map[string]string{"framework": pattern.Framework, "path": routePath}
	name := routePath
	if method != "" {
		attrs["method"] = method
		name = method + " " + routePath
	}
	tags := append([]string{}, pattern.Tags...)
	if len(tags) == 0 {
		tags = []string{"http:route"}
	}
	if pattern.Framework != "" {
		tags = append(tags, "framework:"+pattern.Framework)
	}
	return name, attrs, tags
}

func matchLanguages(languages ...string) func(FileInput) bool {
	return MatchLanguages(languages...)
}

func MatchLanguages(languages ...string) func(FileInput) bool {
	allowed := map[string]struct{}{}
	for _, language := range languages {
		language = strings.TrimSpace(strings.ToLower(language))
		if language != "" {
			allowed[language] = struct{}{}
		}
	}
	return func(input FileInput) bool {
		_, ok := allowed[strings.ToLower(input.Language)]
		return ok
	}
}

func subjectForLine(input FileInput, line int) SubjectRef {
	return SubjectForLine(input, line)
}

func SubjectForLine(input FileInput, line int) SubjectRef {
	if input.Parsed != nil {
		for _, sym := range input.Parsed.Symbols {
			end := sym.EndLine
			if end <= 0 {
				end = sym.Line
			}
			if sym.Line <= line && end >= line {
				return SubjectRef{
					Kind:      "symbol",
					StableKey: symbolStableKey(input.Language, input.RelPath, sym),
					FilePath:  input.RelPath,
					Name:      symbolQualifiedName(sym),
				}
			}
		}
	}
	return fileSubject(input.RelPath)
}

func fileSubject(relPath string) SubjectRef {
	return FileSubject(relPath)
}

func FileSubject(relPath string) SubjectRef {
	return SubjectRef{Kind: "file", StableKey: "file:" + relPath, FilePath: relPath, Name: path.Base(relPath)}
}

func symbolStableKey(language, relPath string, sym analyzer.Symbol) string {
	return fmt.Sprintf("%s:%s:%s:%s", language, relPath, sym.Kind, symbolQualifiedName(sym))
}

func symbolQualifiedName(sym analyzer.Symbol) string {
	if sym.Parent == "" {
		return sym.Name
	}
	return sym.Parent + "." + sym.Name
}

func submatches(source string, indexes []int) []string {
	return Submatches(source, indexes)
}

func Submatches(source string, indexes []int) []string {
	out := make([]string, 0, len(indexes)/2)
	for i := 0; i < len(indexes); i += 2 {
		if indexes[i] < 0 || indexes[i+1] < 0 {
			out = append(out, "")
			continue
		}
		out = append(out, source[indexes[i]:indexes[i+1]])
	}
	return out
}

func lineForOffset(source string, offset int) int {
	return LineForOffset(source, offset)
}

func LineForOffset(source string, offset int) int {
	if offset < 0 {
		return 1
	}
	return strings.Count(source[:offset], "\n") + 1
}
