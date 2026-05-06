package typescript

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

func Express() Enricher {
	return enrich.RouteRegexEnricher("ts.express", "Express routes", "typescript,javascript", []ActivationSignal{
		{Kind: SignalImport, Value: "express"},
		{Kind: SignalDependency, Value: "express"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b(?:app|router)\.(get|post|put|delete|patch)\(\s*["'\x60]([^"'\x60]+)["'\x60]`), Framework: "express", MethodGroup: 1, PathGroup: 2},
	})
}
