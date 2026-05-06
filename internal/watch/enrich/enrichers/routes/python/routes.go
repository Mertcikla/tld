package python

import (
	"regexp"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type ActivationSignal = enrich.ActivationSignal
type Enricher = enrich.Enricher
type RoutePattern = enrich.RoutePattern

const (
	SignalDependency = enrich.SignalDependency
	SignalImport     = enrich.SignalImport
)

func PythonFlask() Enricher {
	return enrich.RouteRegexEnricher("python.flask", "Python Flask routes", "python", []ActivationSignal{
		{Kind: SignalImport, Value: "flask"},
		{Kind: SignalDependency, Value: "flask"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`@(?:[A-Za-z_][A-Za-z0-9_]*\.)?route\(\s*["']([^"']+)["']`), FactType: "http.route", Framework: "flask", Tags: []string{"http:route", "framework:flask"}},
	})
}
