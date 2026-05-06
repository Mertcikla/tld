package defaults

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/datastore"
	frontendts "github.com/mertcikla/tld/internal/watch/enrich/enrichers/frontend/typescript"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/inventory"
	ormts "github.com/mertcikla/tld/internal/watch/enrich/enrichers/orm/typescript"
	goroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/golang"
	pythonroutes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/python"
	tstypes "github.com/mertcikla/tld/internal/watch/enrich/enrichers/routes/typescript"
	rpcgrpc "github.com/mertcikla/tld/internal/watch/enrich/enrichers/rpc/grpc"
	runtimeenrich "github.com/mertcikla/tld/internal/watch/enrich/enrichers/runtime"
	pythontraffic "github.com/mertcikla/tld/internal/watch/enrich/enrichers/traffic/python"
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
