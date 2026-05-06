package workspace

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("workspace.nx", "Nx", "json", []string{"nx.json"}, []string{"\"projects\""}),
		spec("workspace.turborepo", "Turborepo", "json", []string{"turbo.json"}, []string{"\"pipeline\""}),
		spec("workspace.pnpm", "pnpm workspaces", "yaml", []string{"pnpm-workspace.yaml"}, []string{"packages:"}),
		spec("workspace.yarn", "Yarn workspaces", "json", nil, []string{"\"workspaces\""}),
		spec("workspace.bazel", "Bazel", "bazel", []string{"WORKSPACE", "BUILD.bazel"}, []string{"bazel_dep("}),
		spec("workspace.gradle", "Gradle multi-project", "gradle", []string{"settings.gradle"}, []string{"include("}),
		spec("workspace.maven", "Maven modules", "xml", []string{"pom.xml"}, []string{"<modules>"}),
		spec("workspace.cargo", "Cargo workspace", "toml", []string{"Cargo.toml"}, []string{"[workspace]"}),
		spec("workspace.go", "Go workspaces", "go-work", []string{"go.work"}, []string{"use ("}),
	}
}

func spec(id, name, language string, pathTokens, sourceTokens []string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "workspace",
		Languages:    []string{language},
		Mode:         enrich.ActivationAlways,
		FactType:     "workspace.package",
		Relationship: "contains",
		SourceTokens: sourceTokens,
		PathTokens:   pathTokens,
		Tags:         []string{"workspace:" + id},
		Attributes:   map[string]string{"tool": id},
	}
}
