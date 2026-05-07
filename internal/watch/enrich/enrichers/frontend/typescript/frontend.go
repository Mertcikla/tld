package typescript

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type ActivationSignal = enrich.ActivationSignal
type Enricher = enrich.Enricher
type Fact = enrich.Fact
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput
type Metadata = enrich.Metadata
type RoutePattern = enrich.RoutePattern
type SourceSpan = enrich.SourceSpan
type SubjectRef = enrich.SubjectRef

const (
	ActivationImportOrDependency = enrich.ActivationImportOrDependency
	SignalDependency             = enrich.SignalDependency
	SignalImport                 = enrich.SignalImport
)

var fileSubject = enrich.FileSubject

func NextJS() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID:   "ts.nextjs",
			Name: "Next.js routes",
			Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{
				{Kind: SignalDependency, Value: "next"},
				{Kind: SignalImport, Value: "next"},
			},
		},
		func(input FileInput) bool {
			route := nextRoutePath(input.RelPath)
			return route != ""
		},
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			route := nextRoutePath(input.RelPath)
			if route == "" {
				return nil
			}
			return emit.EmitFact(Fact{
				Type:         "frontend.route",
				StableKey:    fmt.Sprintf("frontend.route:nextjs:%s:%s", input.RelPath, route),
				Subject:      fileSubject(input.RelPath),
				Object:       SubjectRef{Kind: "frontend.route", StableKey: "frontend.route:nextjs:" + route, FilePath: input.RelPath, Name: route},
				Relationship: "declares",
				Source:       SourceSpan{FilePath: input.RelPath, StartLine: 1, EndLine: 1},
				Confidence:   0.95,
				Name:         route,
				Tags:         []string{"frontend:route", "framework:nextjs"},
				Attributes:   map[string]string{"framework": "nextjs", "path": route},
				VisibilityHints: map[string]float64{
					"high_signal": 1,
				},
			})
		},
	)
}

func ReactRouter() Enricher {
	return enrich.RouteRegexEnricher("ts.react_router", "React Router routes", "typescript,javascript", []ActivationSignal{
		{Kind: SignalImport, Value: "react-router"},
		{Kind: SignalImport, Value: "react-router-dom"},
		{Kind: SignalDependency, Value: "react-router"},
		{Kind: SignalDependency, Value: "react-router-dom"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`<Route\b[^>]*\bpath\s*=\s*["'{\x60]([^"'}\x60]+)["'}\x60]`), FactType: "frontend.route", Framework: "react-router", Tags: []string{"frontend:route", "framework:react-router"}},
		{Re: regexp.MustCompile(`\bpath\s*:\s*["'\x60]([^"'\x60]+)["'\x60]`), FactType: "frontend.route", Framework: "react-router", Tags: []string{"frontend:route", "framework:react-router"}},
	})
}

func nextRoutePath(relPath string) string {
	rel := filepath.ToSlash(relPath)
	ext := path.Ext(rel)
	if ext == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(rel, ext)
	for _, prefix := range []string{"src/app/", "app/"} {
		if after, ok := strings.CutPrefix(trimmed, prefix); ok {
			route := after
			if !strings.HasSuffix(route, "/page") && !strings.HasSuffix(route, "/route") {
				return ""
			}
			route = strings.TrimSuffix(strings.TrimSuffix(route, "/page"), "/route")
			return normalizeNextRoute(route)
		}
	}
	for _, prefix := range []string{"src/pages/", "pages/"} {
		if after, ok := strings.CutPrefix(trimmed, prefix); ok {
			route := after
			return normalizeNextRoute(route)
		}
	}
	return ""
}

func normalizeNextRoute(route string) string {
	route = strings.Trim(route, "/")
	if route == "" || route == "index" {
		return "/"
	}
	parts := strings.Split(route, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "index" {
			continue
		}
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			part = ":" + strings.Trim(part, "[]")
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return "/" + strings.Join(out, "/")
}
