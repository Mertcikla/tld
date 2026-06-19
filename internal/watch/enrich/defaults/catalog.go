package defaults

import (
	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/compose"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/datastore"
	frontendts "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/frontend/typescript"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/inventory"
	cpproutes "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/routes/cpp"
	goroutes "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/routes/golang"
	javaroutes "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/routes/java"
	pythonroutes "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/routes/python"
	rustroutes "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/routes/rust"
	tstypes "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/routes/typescript"
	rpcgrpc "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/rpc/grpc"
	runtimeenrich "github.com/mertcikla/tld/v2/internal/watch/enrich/enrichers/runtime"
)

// NewRegistry returns the complete built-in enricher registry.
func NewRegistry() *enrich.Registry {
	return enrich.NewRegistry(DefaultEnrichers()...)
}

// NewRegistryWithDependencyInventory returns the built-in registry with optional
// dependency/import inventory facts.
func NewRegistryWithDependencyInventory(enabled bool) *enrich.Registry {
	return enrich.NewRegistry(DefaultEnrichersWithDependencyInventory(enabled)...)
}

// DefaultEnrichers returns the complete built-in catalog.
//
// Keep this function as composition only. Add new enrichers to the narrowest
// domain/language package so the default registry does not become an unreadable
// list of framework constructors.
func DefaultEnrichers() []enrich.Enricher {
	return DefaultEnrichersWithDependencyInventory(true)
}

func DefaultEnrichersWithDependencyInventory(dependencyInventory bool) []enrich.Enricher {
	inventoryEnrichers := []enrich.Enricher{}
	if dependencyInventory {
		inventoryEnrichers = InventoryEnrichers()
	}
	return appendGroups(
		inventoryEnrichers,
		RouteEnrichers(),
		FrontendEnrichers(),
		RPCEnrichers(),
		RuntimeEnrichers(),
		ComposeEnrichers(),
		DatastoreEnrichers(),
	)
}

func InventoryEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		inventory.DependencyInventory(),
	}
}

func RouteEnrichers() []enrich.Enricher {
	return appendGroups(
		GoRouteEnrichers(),
		TypeScriptRouteEnrichers(),
		PythonRouteEnrichers(),
		JavaRouteEnrichers(),
		RustRouteEnrichers(),
		CPPRouteEnrichers(),
	)
}

func GoRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		goroutes.GoNetHTTP(),
		goroutes.GoChi(),
		goroutes.GoGin(),
		goroutes.GoGorillaMux(),
		goroutes.GoEcho(),
		goroutes.GoFiber(),
	}
}

func TypeScriptRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		tstypes.Express(),
		tstypes.Fastify(),
		tstypes.NestJS(),
		tstypes.Hono(),
	}
}

func PythonRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		pythonroutes.PythonFlask(),
		pythonroutes.PythonFastAPI(),
		pythonroutes.PythonDjango(),
		pythonroutes.PythonStarlette(),
	}
}

func JavaRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		javaroutes.Spring(),
		javaroutes.JAXRS(),
		javaroutes.Micronaut(),
		javaroutes.Quarkus(),
	}
}

func RustRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		rustroutes.Axum(),
		rustroutes.ActixWeb(),
		rustroutes.Rocket(),
		rustroutes.Warp(),
	}
}

func CPPRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		cpproutes.Drogon(),
		cpproutes.Oatpp(),
		cpproutes.Pistache(),
		cpproutes.Crow(),
		cpproutes.CppRestSDK(),
	}
}

func FrontendEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		frontendts.NextJS(),
		frontendts.ReactRouter(),
	}
}

func RPCEnrichers() []enrich.Enricher {
	return appendGroups(
		ContractEnrichers(),
		GRPCEnrichers(),
	)
}

func ContractEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		rpcgrpc.ProtobufContracts(),
	}
}

func GRPCEnrichers() []enrich.Enricher {
	return appendGroups(
		GoGRPCEnrichers(),
		PythonGRPCEnrichers(),
		NodeGRPCEnrichers(),
		JavaGRPCEnrichers(),
		DotNetGRPCEnrichers(),
	)
}

func GoGRPCEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		rpcgrpc.GoGRPC(),
	}
}

func PythonGRPCEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		rpcgrpc.PythonGRPC(),
	}
}

func NodeGRPCEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		rpcgrpc.NodeGRPC(),
	}
}

func JavaGRPCEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		rpcgrpc.JavaGRPC(),
	}
}

func DotNetGRPCEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		rpcgrpc.DotNetGRPC(),
	}
}

func RuntimeEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		runtimeenrich.RuntimeManifests(),
	}
}

func ComposeEnrichers() []enrich.Enricher { return compose.All() }

func DatastoreEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		datastore.DatastoreGlue(),
	}
}

func appendGroups(groups ...[]enrich.Enricher) []enrich.Enricher {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	out := make([]enrich.Enricher, 0, total)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}
