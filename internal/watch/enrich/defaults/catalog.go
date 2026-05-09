package defaults

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/ai"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/apispec"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/auth"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/cloud"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/compose"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/config"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/dataeng"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/datastore"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/deployment"
	frontendts "github.com/mertcikla/tld/internal/watch/enrich/enrichers/frontend/typescript"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/httpclient"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/iac"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/inventory"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/iot"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/ipc"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/jobs"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/messaging"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/observability"
	ormcpp "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/cpp"
	ormgo "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/golang"
	ormjava "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/java"
	ormpython "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/python"
	ormrust "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/rust"
	ormts "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/typescript"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/osintegration"
	cpproutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/cpp"
	goroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/golang"
	javaroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/java"
	pythonroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/python"
	rustroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/rust"
	tstypes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/typescript"
	rpcclients "github.com/mertcikla/tld/internal/watch/enrich/enrichers/rpc/clients"
	rpcgrpc "github.com/mertcikla/tld/internal/watch/enrich/enrichers/rpc/grpc"
	runtimeenrich "github.com/mertcikla/tld/internal/watch/enrich/enrichers/runtime"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/secrets"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/storage"
	pythontraffic "github.com/mertcikla/tld/internal/watch/enrich/enrichers/traffic/python"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/web3"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/workspace"
)

// NewRegistry returns the complete built-in enricher registry.
func NewRegistry() *enrich.Registry {
	return enrich.NewRegistry(DefaultEnrichers()...)
}

// DefaultEnrichers returns the complete built-in catalog.
//
// Keep this function as composition only. Add new enrichers to the narrowest
// domain/language package so the default registry does not become an unreadable
// list of framework constructors.
func DefaultEnrichers() []enrich.Enricher {
	return appendGroups(
		InventoryEnrichers(),
		ConfigEnrichers(),
		HTTPClientEnrichers(),
		RouteEnrichers(),
		FrontendEnrichers(),
		ORMEnrichers(),
		RPCEnrichers(),
		RuntimeEnrichers(),
		IaCEnrichers(),
		ComposeEnrichers(),
		CloudEnrichers(),
		MessagingEnrichers(),
		StorageEnrichers(),
		DatastoreEnrichers(),
		TrafficEnrichers(),
		ObservabilityEnrichers(),
		AuthEnrichers(),
		JobEnrichers(),
		APISpecEnrichers(),
		DeploymentEnrichers(),
		SecretEnrichers(),
		WorkspaceEnrichers(),
		AIEnrichers(),
		IoTEnrichers(),
		IPCEnrichers(),
		DataEnrichers(),
		Web3Enrichers(),
		OSIntegrationEnrichers(),
	)
}

func InventoryEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		inventory.DependencyInventory(),
	}
}

func ConfigEnrichers() []enrich.Enricher     { return config.All() }
func HTTPClientEnrichers() []enrich.Enricher { return httpclient.All() }

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

func ORMEnrichers() []enrich.Enricher {
	return appendGroups(
		ormts.All(),
		ormgo.All(),
		ormpython.All(),
		ormjava.All(),
		ormrust.All(),
		ormcpp.All(),
	)
}

func RPCEnrichers() []enrich.Enricher {
	return appendGroups(
		ContractEnrichers(),
		GRPCEnrichers(),
		RPCClientEnrichers(),
	)
}

func RPCClientEnrichers() []enrich.Enricher { return rpcclients.All() }

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

func IaCEnrichers() []enrich.Enricher       { return iac.All() }
func ComposeEnrichers() []enrich.Enricher   { return compose.All() }
func CloudEnrichers() []enrich.Enricher     { return cloud.All() }
func MessagingEnrichers() []enrich.Enricher { return messaging.All() }
func StorageEnrichers() []enrich.Enricher   { return storage.All() }

func DatastoreEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		datastore.DatastoreGlue(),
	}
}

func TrafficEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		pythontraffic.PythonLocust(),
	}
}

func ObservabilityEnrichers() []enrich.Enricher { return observability.All() }
func AuthEnrichers() []enrich.Enricher          { return auth.All() }
func JobEnrichers() []enrich.Enricher           { return jobs.All() }
func APISpecEnrichers() []enrich.Enricher       { return apispec.All() }
func DeploymentEnrichers() []enrich.Enricher    { return deployment.All() }
func SecretEnrichers() []enrich.Enricher        { return secrets.All() }
func WorkspaceEnrichers() []enrich.Enricher     { return workspace.All() }
func AIEnrichers() []enrich.Enricher            { return ai.All() }
func IoTEnrichers() []enrich.Enricher           { return iot.All() }
func IPCEnrichers() []enrich.Enricher           { return ipc.All() }
func DataEnrichers() []enrich.Enricher          { return dataeng.All() }
func Web3Enrichers() []enrich.Enricher          { return web3.All() }
func OSIntegrationEnrichers() []enrich.Enricher { return osintegration.All() }

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
