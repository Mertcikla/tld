package defaults

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/ai"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/apispec"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/auth"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/dataeng"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/datastore"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/deployment"
	frontendts "github.com/mertcikla/tld/internal/watch/enrich/enrichers/frontend/typescript"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/inventory"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/iot"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/ipc"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/jobs"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/observability"
	ormts "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/typescript"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/osintegration"
	goroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/golang"
	pythonroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/python"
	tstypes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/typescript"
	rpcgrpc "github.com/mertcikla/tld/internal/watch/enrich/enrichers/rpc/grpc"
	runtimeenrich "github.com/mertcikla/tld/internal/watch/enrich/enrichers/runtime"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/secrets"
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
		RouteEnrichers(),
		FrontendEnrichers(),
		ORMEnrichers(),
		RPCEnrichers(),
		RuntimeEnrichers(),
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

func RouteEnrichers() []enrich.Enricher {
	return appendGroups(
		GoRouteEnrichers(),
		TypeScriptRouteEnrichers(),
		PythonRouteEnrichers(),
	)
}

func GoRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		goroutes.GoNetHTTP(),
		goroutes.GoChi(),
		goroutes.GoGin(),
		goroutes.GoGorillaMux(),
	}
}

func TypeScriptRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		tstypes.Express(),
	}
}

func PythonRouteEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		pythonroutes.PythonFlask(),
	}
}

func FrontendEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		frontendts.NextJS(),
		frontendts.ReactRouter(),
	}
}

func ORMEnrichers() []enrich.Enricher {
	return []enrich.Enricher{
		ormts.Prisma(),
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
