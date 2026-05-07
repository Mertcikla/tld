package rust

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

func Axum() Enricher {
	return enrich.RouteRegexEnricher("rust.axum", "Rust axum routes", "rust", []ActivationSignal{{Kind: SignalDependency, Value: "axum"}, {Kind: SignalImport, Value: "axum"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`\broute\(\s*"([^"]+)"\s*,\s*(get|post|put|delete|patch)\(`), Framework: "axum", PathGroup: 1, MethodGroup: 2},
	})
}

func ActixWeb() Enricher {
	return enrich.RouteRegexEnricher("rust.actix_web", "Rust actix-web routes", "rust", []ActivationSignal{{Kind: SignalDependency, Value: "actix-web"}, {Kind: SignalImport, Value: "actix_web"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`#\[(get|post|put|delete|patch)\("([^"]+)"\)\]`), Framework: "actix-web", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`\.route\(\s*"([^"]+)"\s*,\s*web::(get|post|put|delete|patch)\(`), Framework: "actix-web", PathGroup: 1, MethodGroup: 2},
	})
}

func Rocket() Enricher {
	return enrich.RouteRegexEnricher("rust.rocket", "Rust Rocket routes", "rust", []ActivationSignal{{Kind: SignalDependency, Value: "rocket"}, {Kind: SignalImport, Value: "rocket"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`#\[(get|post|put|delete|patch)\("([^"]+)"\)\]`), Framework: "rocket", MethodGroup: 1, PathGroup: 2},
	})
}

func Warp() Enricher {
	return enrich.RouteRegexEnricher("rust.warp", "Rust warp routes", "rust", []ActivationSignal{{Kind: SignalDependency, Value: "warp"}, {Kind: SignalImport, Value: "warp"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`warp::path!\(\s*"([^"]+)"`), Framework: "warp", PathGroup: 1},
	})
}
