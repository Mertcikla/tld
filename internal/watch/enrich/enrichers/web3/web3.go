package web3

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.ethers", "TypeScript ethers.js", "typescript", "ethers", "JsonRpcProvider", "web3.rpc_endpoint", "connects_to_chain"),
		spec("ts.web3js", "TypeScript web3.js", "typescript", "web3", "new Web3", "web3.rpc_endpoint", "connects_to_chain"),
		spec("python.web3py", "Python web3.py", "python", "web3", "Web3.HTTPProvider", "web3.rpc_endpoint", "connects_to_chain"),
		spec("solidity.foundry", "Foundry", "toml", "forge-std", "foundry.toml", "web3.chain_id", "connects_to_chain"),
		spec("ts.hardhat", "Hardhat", "typescript", "hardhat", "hardhat.config", "web3.chain_id", "connects_to_chain"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "web3",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		PathTokens:   []string{token},
		Tags:         []string{"web3:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
