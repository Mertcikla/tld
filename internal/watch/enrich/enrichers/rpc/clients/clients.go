package clients

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.connectrpc", "TypeScript ConnectRPC", "typescript", "@connectrpc/connect", "createClient", "rpc.client", "calls"),
		spec("ts.openapi_client", "TypeScript generated OpenAPI client", "typescript", "openapi", "new DefaultApi", "integration.client", "calls"),
		spec("ts.graphql_client", "TypeScript GraphQL client", "typescript", "graphql-request", "GraphQLClient", "rpc.client", "calls"),
		spec("go.connectrpc", "Go connect-go", "go", "connectrpc.com/connect", "connect.New", "rpc.client", "calls"),
		spec("go.twirp", "Go Twirp", "go", "github.com/twitchtv/twirp", "twirp", "rpc.client", "calls"),
		spec("go.openapi_client", "Go generated OpenAPI client", "go", "openapi", "NewAPIClient", "integration.client", "calls"),
		spec("python.openapi_client", "Python generated OpenAPI client", "python", "openapi", "ApiClient", "integration.client", "calls"),
		spec("python.gql", "Python gql", "python", "gql", "Client(", "rpc.client", "calls"),
		spec("java.openfeign", "Java OpenFeign", "java", "spring-cloud-starter-openfeign", "@FeignClient", "rpc.client", "calls"),
		spec("java.retrofit_rpc", "Java Retrofit RPC", "java", "retrofit2", "Retrofit.Builder", "rpc.client", "calls"),
		spec("java.openapi_client", "Java generated OpenAPI client", "java", "openapi", "ApiClient", "integration.client", "calls"),
		spec("java.graphql_client", "Java GraphQL client", "java", "graphql-java", "GraphQL", "rpc.client", "calls"),
		spec("rust.tonic", "Rust tonic", "rust", "tonic", "tonic::transport", "rpc.client", "calls"),
		spec("rust.openapi_client", "Rust generated OpenAPI client", "rust", "openapi", "apis::", "integration.client", "calls"),
		spec("rust.graphql_client", "Rust graphql_client", "rust", "graphql_client", "GraphQLQuery", "rpc.client", "calls"),
		spec("cpp.grpc_cpp", "C++ grpc-cpp", "cpp", "grpc++", "grpc::CreateChannel", "rpc.client", "calls"),
		spec("cpp.openapi_client", "C++ generated OpenAPI client", "cpp", "openapi", "ApiClient", "integration.client", "calls"),
		spec("cpp.thrift", "C++ Thrift", "cpp", "thrift", "apache::thrift", "rpc.client", "calls"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "rpc",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"rpc:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
