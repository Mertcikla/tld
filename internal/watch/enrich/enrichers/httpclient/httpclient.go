package httpclient

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		native("ts.fetch", "TypeScript fetch", "typescript", "fetch(", "fetch"),
		lib("ts.axios", "TypeScript Axios", "typescript", "axios", "axios."),
		lib("ts.got", "TypeScript Got", "typescript", "got", "got("),
		lib("ts.ky", "TypeScript Ky", "typescript", "ky", "ky."),
		native("go.net_http_client", "Go net/http client", "go", "http.NewRequest", "net/http"),
		lib("go.resty", "Go Resty", "go", "github.com/go-resty/resty", "resty.New"),
		lib("go.retryablehttp", "Go retryablehttp", "go", "github.com/hashicorp/go-retryablehttp", "retryablehttp.NewClient"),
		lib("python.requests", "Python requests", "python", "requests", "requests."),
		lib("python.httpx", "Python httpx", "python", "httpx", "httpx."),
		lib("python.aiohttp", "Python aiohttp", "python", "aiohttp", "ClientSession"),
		native("java.http_client", "Java HttpClient", "java", "HttpClient.newHttpClient", "java.net.http"),
		lib("java.okhttp", "Java OkHttp", "java", "okhttp3", "OkHttpClient"),
		lib("java.retrofit", "Java Retrofit", "java", "retrofit2", "Retrofit.Builder"),
		lib("java.spring_webclient", "Spring WebClient", "java", "org.springframework.web.reactive.function.client.WebClient", "WebClient.builder"),
		lib("java.rest_template", "Spring RestTemplate", "java", "org.springframework.web.client.RestTemplate", "RestTemplate"),
		lib("rust.reqwest", "Rust reqwest", "rust", "reqwest", "reqwest::"),
		lib("rust.hyper", "Rust hyper", "rust", "hyper", "hyper::Client"),
		lib("rust.ureq", "Rust ureq", "rust", "ureq", "ureq::"),
		lib("cpp.libcurl", "C++ libcurl", "cpp", "libcurl", "curl_easy_perform"),
		lib("cpp.cpprestsdk_client", "C++ cpprestsdk client", "cpp", "cpprestsdk", "http_client"),
		lib("cpp.boost_beast", "C++ Boost.Beast", "cpp", "boost-beast", "boost::beast"),
		lib("cpp.cpr", "C++ cpr", "cpp", "cpr", "cpr::"),
	}
}

func native(id, name, language, token, dependency string) pattern.Spec {
	spec := base(id, name, language, dependency, token)
	spec.Mode = enrich.ActivationAlways
	spec.Triggers = nil
	return spec
}

func lib(id, name, language, dependency, token string) pattern.Spec {
	return base(id, name, language, dependency, token)
}

func base(id, name, language, dependency, token string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "http-client",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "http.client_call",
		Relationship: "calls",
		SourceTokens: []string{token},
		Tags:         []string{"http-client:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
