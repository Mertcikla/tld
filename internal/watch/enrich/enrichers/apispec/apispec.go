package apispec

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("apispec.openapi", "OpenAPI / Swagger", "yaml", []string{"openapi.yaml", "swagger.yaml"}, []string{"openapi: 3", "openapi: \"3\""}, "api.spec", "documents"),
		spec("apispec.asyncapi", "AsyncAPI", "yaml", []string{"asyncapi.yaml"}, []string{"asyncapi: 2", "asyncapi: \"2\""}, "api.spec", "documents"),
		spec("apispec.graphql_schema", "GraphQL schema", "graphql", []string{".graphql", ".gql"}, []string{"type Query"}, "api.schema", "declares"),
		spec("apispec.protobuf", "Protocol Buffers", "protobuf", []string{".proto"}, []string{"service "}, "rpc.service", "exposes"),
		spec("apispec.avro", "Avro", "json", []string{".avsc", ".avdl"}, []string{"\"type\":\"record\""}, "api.schema", "declares"),
		spec("apispec.json_schema", "JSON Schema", "json", []string{"schema.json"}, []string{"\"$schema\""}, "api.schema", "declares"),
	}
}

func spec(id, name, language string, pathTokens, sourceTokens []string, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "api-spec",
		Languages:    []string{language},
		Mode:         enrich.ActivationAlways,
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: sourceTokens,
		PathTokens:   pathTokens,
		Tags:         []string{"api-spec:" + id},
		Attributes:   map[string]string{"format": id},
	}
}
