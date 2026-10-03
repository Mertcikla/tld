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

// symbolInputs fingerprints each project independently. A change under one
// project's root no longer invalidates every project in the same language
// family, so only projects containing edited files rerun their indexer.
func symbolInputs(root string, projects []*pb.Project, sources map[string]*graph.Source, configHash string) (map[string]string, error) {
	result := map[string]string{}
	for _, pr := range projects {
		family := languageFamily(pr.Language)
		key := family + "|" + pr.Root
		prefix := strings.Trim(pr.Root, "/")
		if prefix != "" && prefix != "." {
			prefix += "/"
		}
		var parts []string
		for path, src := range sources {
			underRoot := pr.Root == "." || pr.Root == "" || strings.HasPrefix(path, prefix)
			if !underRoot {
				continue
			}
			// A repository-root project owns only sources of its own language
			// family; other languages are covered by their own projects.
			if src.Language != "" && languageFamily(src.Language) != family {
				continue
			}
			parts = append(parts, path, src.Hash)
		}
		sort.Strings(parts)
		manifests := pr.ConfigPath
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pr.ConfigPath)))
		if err != nil {
			return nil, err
		}
		parts = append(parts, manifests, graph.Hash(raw))
		result[key] = graph.ID(append([]string{configHash, key}, parts...)...)
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
