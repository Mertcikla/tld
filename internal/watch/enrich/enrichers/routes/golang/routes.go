package golang

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

func GoGorillaMux() Enricher {
	return enrich.RouteRegexEnricher("go.gorilla_mux", "Go gorilla/mux routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "github.com/gorilla/mux"},
		{Kind: SignalDependency, Value: "github.com/gorilla/mux"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.HandleFunc\(\s*([^,\n]+),`), Framework: "gorilla-mux", PathGroup: 1},
	})
}

func GoNetHTTP() Enricher {
	return enrich.RouteRegexEnricher("go.nethttp", "Go net/http routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "net/http"},
		{Kind: SignalDependency, Value: "net/http"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\bhttp\.HandleFunc\(\s*"([^"]+)"`), Method: "", Framework: "nethttp"},
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.HandleFunc\(\s*"([^"]+)"`), Method: "", Framework: "nethttp"},
	})
}

func GoChi() Enricher {
	return enrich.RouteRegexEnricher("go.chi", "Go chi routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "github.com/go-chi/chi"},
		{Kind: SignalDependency, Value: "github.com/go-chi/chi"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(Get|Post|Put|Delete|Patch)\(\s*"([^"]+)"`), Framework: "chi", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.Route\(\s*"([^"]+)"`), Framework: "chi"},
	})
}

func GoGin() Enricher {
	return enrich.RouteRegexEnricher("go.gin", "Go gin routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "github.com/gin-gonic/gin"},
		{Kind: SignalDependency, Value: "github.com/gin-gonic/gin"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(GET|POST|PUT|DELETE|PATCH)\(\s*"([^"]+)"`), Framework: "gin", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.Group\(\s*"([^"]+)"`), Framework: "gin"},
	})
}

func GoEcho() Enricher {
	return enrich.RouteRegexEnricher("go.echo", "Go Echo routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "github.com/labstack/echo"},
		{Kind: SignalDependency, Value: "github.com/labstack/echo"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(GET|POST|PUT|DELETE|PATCH)\(\s*"([^"]+)"`), Framework: "echo", MethodGroup: 1, PathGroup: 2},
	})
}

func GoFiber() Enricher {
	return enrich.RouteRegexEnricher("go.fiber", "Go Fiber routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "github.com/gofiber/fiber"},
		{Kind: SignalDependency, Value: "github.com/gofiber/fiber"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(Get|Post|Put|Delete|Patch)\(\s*"([^"]+)"`), Framework: "fiber", MethodGroup: 1, PathGroup: 2},
	})
}
