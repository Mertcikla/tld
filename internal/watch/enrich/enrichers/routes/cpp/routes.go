package cpp

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

func Drogon() Enricher {
	return enrich.RouteRegexEnricher("cpp.drogon", "C++ Drogon routes", "cpp", []ActivationSignal{{Kind: SignalDependency, Value: "drogon"}, {Kind: SignalImport, Value: "drogon"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`METHOD_(GET|POST|PUT|DELETE|PATCH).*?ADD_METHOD_TO\([^,]+,\s*"([^"]+)"`), Framework: "drogon", MethodGroup: 1, PathGroup: 2},
		{Re: regexp.MustCompile(`PATH_ADD\(\s*"([^"]+)"`), Framework: "drogon", PathGroup: 1},
	})
}

func Oatpp() Enricher {
	return enrich.RouteRegexEnricher("cpp.oatpp", "C++ oatpp routes", "cpp", []ActivationSignal{{Kind: SignalDependency, Value: "oatpp"}, {Kind: SignalImport, Value: "oatpp"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`ENDPOINT\(\s*"([A-Z]+)"\s*,\s*"([^"]+)"`), Framework: "oatpp", MethodGroup: 1, PathGroup: 2},
	})
}

func Pistache() Enricher {
	return enrich.RouteRegexEnricher("cpp.pistache", "C++ Pistache routes", "cpp", []ActivationSignal{{Kind: SignalDependency, Value: "pistache"}, {Kind: SignalImport, Value: "pistache"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`Routes::(Get|Post|Put|Delete|Patch)\(\s*router\s*,\s*"([^"]+)"`), Framework: "pistache", MethodGroup: 1, PathGroup: 2},
	})
}

func Crow() Enricher {
	return enrich.RouteRegexEnricher("cpp.crow", "C++ Crow routes", "cpp", []ActivationSignal{{Kind: SignalDependency, Value: "crow"}, {Kind: SignalImport, Value: "crow"}}, []*RoutePattern{
		{Re: regexp.MustCompile(`CROW_ROUTE\([^,]+,\s*"([^"]+)"`), Framework: "crow", PathGroup: 1},
	})
}

func CppRestSDK() Enricher {
	return enrich.RouteRegexEnricher("cpp.cpprestsdk", "C++ cpprestsdk routes", "cpp", []ActivationSignal{{Kind: SignalDependency, Value: "cpprestsdk"}, {Kind: SignalImport, Value: "cpprest"}}, []*RoutePattern{
		{
			Re:        regexp.MustCompile(`support\(\s*methods::(GET|POST|PUT|DEL|PATCH)\s*,`),
			Framework: "cpprestsdk",
			Custom: func(match []string) (string, map[string]string, []string) {
				method := match[1]
				if method == "DEL" {
					method = "DELETE"
				}
				return method + " handler", map[string]string{"framework": "cpprestsdk", "method": method}, []string{"http:route", "framework:cpprestsdk"}
			},
		},
	})
}
