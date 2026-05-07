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

func Fastify() Enricher {
	return enrich.RouteRegexEnricher("ts.fastify", "Fastify routes", "typescript,javascript", []ActivationSignal{
		{Kind: SignalImport, Value: "fastify"},
		{Kind: SignalDependency, Value: "fastify"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(get|post|put|delete|patch)\(\s*["'\x60]([^"'\x60]+)["'\x60]`), Framework: "fastify", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.route\(\s*\{[^}]*method:\s*["'\x60]([A-Z]+)["'\x60][^}]*url:\s*["'\x60]([^"'\x60]+)["'\x60]`), Framework: "fastify", MethodGroup: 1, PathGroup: 2},
	})
}

func NestJS() Enricher {
	return enrich.RouteRegexEnricher("ts.nestjs", "NestJS routes", "typescript,javascript", []ActivationSignal{
		{Kind: SignalImport, Value: "@nestjs/common"},
		{Kind: SignalDependency, Value: "@nestjs/common"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`@(Get|Post|Put|Delete|Patch)\(\s*["'\x60]?([^"'\x60)]*)["'\x60]?\s*\)`), Framework: "nestjs", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`@Controller\(\s*["'\x60]([^"'\x60]+)["'\x60]`), Framework: "nestjs", PathGroup: 1},
	})
}

func Hono() Enricher {
	return enrich.RouteRegexEnricher("ts.hono", "Hono routes", "typescript,javascript", []ActivationSignal{
		{Kind: SignalImport, Value: "hono"},
		{Kind: SignalDependency, Value: "hono"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(get|post|put|delete|patch)\(\s*["'\x60]([^"'\x60]+)["'\x60]`), Framework: "hono", MethodGroup: 1, PathGroup: 2},
	})
}
