package indexer

import (
	"context"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func symbolInputs(root string, projects []*pb.Project, sources map[string]*graph.Source, configHash string) (map[string]string, error) {
	families := map[string][]string{}
	for _, pr := range projects {
		families[languageFamily(pr.Language)] = nil
	}
	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		src := sources[path]
		for family := range families {
			if src.Language == "" || languageFamily(src.Language) == family || projectLanguage(filepath.Base(path)) != "" {
				families[family] = append(families[family], path, src.Hash)
			}
		}
	}
	// Manifests such as pom.xml are not necessarily captured source files.
	for _, pr := range projects {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pr.ConfigPath)))
		if err != nil {
			return nil, err
		}
		for family := range families {
			families[family] = append(families[family], pr.ConfigPath, graph.Hash(raw))
		}
	}
	result := map[string]string{}
	for family, parts := range families {
		result[family] = graph.ID(append([]string{configHash, family}, parts...)...)
	}
	return result, nil
}
func projectHashes(pr *pb.Project, sources map[string]*graph.Source) map[string]string {
	hashes := map[string]string{}
	prefix := strings.Trim(pr.Root, "/") + "/"
	for path, src := range sources {
		if pr.Root == "." || pr.Root == "" {
			hashes[path] = src.Hash
		} else if strings.HasPrefix(path, prefix) {
			hashes[strings.TrimPrefix(path, prefix)] = src.Hash
		}
	}
	return hashes
}

// ToolchainCompatible permits offline reuse of recorded graphs, but invalidates
// caches when an installed extractor reports a different version.
func ToolchainCompatible(ctx context.Context, cfg config.Config, root string, snap *pb.Snapshot, explicit map[string]string) bool {
	if snap.ToolVersions["gotreesitter"] != "0.15.2" {
		return false
	}
	for _, project := range snap.Projects {
		if explicit[project.Root] != "" {
			continue
		}
		spec, err := indexerForFamily(languageFamily(project.Language), cfg)
		if err != nil {
			return false
		}
		tool := spec.executable(cfg, indexerContext{root: root, projectDir: filepath.Join(root, filepath.FromSlash(project.Root)), configPath: project.ConfigPath})
		if _, err = exec.LookPath(tool); err != nil {
			continue
		}
		current := toolVersion(ctx, tool, spec.versionArgs)
		if current != "unknown" && current != snap.ToolVersions[spec.name] {
			return false
		}
	}
	return true
}
