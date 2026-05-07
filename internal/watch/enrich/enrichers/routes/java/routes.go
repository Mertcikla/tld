package java

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

func Spring() Enricher {
	return enrich.RouteRegexEnricher("java.spring_web", "Java Spring MVC/WebFlux routes", "java", []ActivationSignal{
		{Kind: SignalImport, Value: "org.springframework.web.bind.annotation"},
		{Kind: SignalDependency, Value: "spring-boot-starter-web"},
		{Kind: SignalDependency, Value: "spring-boot-starter-webflux"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`@(Get|Post|Put|Delete|Patch)Mapping\(\s*["']([^"']+)["']`), Framework: "spring", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`@RequestMapping\(\s*["']([^"']+)["']`), Framework: "spring", PathGroup: 1},
	})
}

func JAXRS() Enricher {
	return enrich.RouteRegexEnricher("java.jax_rs", "Java JAX-RS routes", "java", []ActivationSignal{
		{Kind: SignalImport, Value: "jakarta.ws.rs"},
		{Kind: SignalDependency, Value: "jakarta.ws.rs-api"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`@Path\(\s*["']([^"']+)["']`), Framework: "jax-rs", PathGroup: 1},
	})
}

func Micronaut() Enricher {
	return enrich.RouteRegexEnricher("java.micronaut", "Java Micronaut routes", "java", []ActivationSignal{
		{Kind: SignalImport, Value: "io.micronaut.http.annotation"},
		{Kind: SignalDependency, Value: "micronaut-http-server"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`@(Get|Post|Put|Delete|Patch)\(\s*["']([^"']+)["']`), Framework: "micronaut", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`@Controller\(\s*["']([^"']+)["']`), Framework: "micronaut", PathGroup: 1},
	})
}

func Quarkus() Enricher {
	return enrich.RouteRegexEnricher("java.quarkus", "Java Quarkus routes", "java", []ActivationSignal{
		{Kind: SignalImport, Value: "io.quarkus"},
		{Kind: SignalDependency, Value: "quarkus-resteasy-reactive"},
	}, []*RoutePattern{
		{Re: regexp.MustCompile(`@Path\(\s*["']([^"']+)["']`), Framework: "quarkus", PathGroup: 1},
	})
}
