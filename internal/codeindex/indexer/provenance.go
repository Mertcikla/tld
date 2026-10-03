package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// CaptureInputs fingerprints exactly the source/configuration inputs used by indexing.
func CaptureInputs(ctx context.Context, root string, cfg config.Config, excludes []string) (fingerprint, revision, branch, provenance string, err error) {
	return captureInputs(ctx, root, cfg, &pb.IndexRequest{Exclude: excludes})
}

// ConfigurationHash includes per-request inputs affecting graph extraction.
func ConfigurationHash(cfg config.Config, req *pb.IndexRequest) string {
	raw, _ := json.Marshal(struct {
		Config                 config.Config
		Excludes, ProjectRoots []string
		SCIPArtifacts          map[string]string
	}{cfg, req.Exclude, req.ProjectRoots, req.ScipArtifacts})
	return graph.Hash(raw)
}

func captureInputs(ctx context.Context, root string, cfg config.Config, req *pb.IndexRequest) (fingerprint, revision, branch, provenance string, err error) {
	projects, sources, err := Discover(ctx, root, req.ProjectRoots, req.Exclude)
	if err != nil {
		return "", "", "", "", err
	}
	parts := []string{ConfigurationHash(cfg, req)}
	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		parts = append(parts, path, sources[path].Hash)
	}
	for _, project := range projects {
		content, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(project.ConfigPath)))
		if readErr != nil {
			return "", "", "", "", fmt.Errorf("read project configuration: %w", readErr)
		}
		parts = append(parts, project.ConfigPath, graph.Hash(content))
	}
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		raw, e := cmd.Output()
		return strings.TrimSpace(string(raw)), e
	}
	revision, _ = git("rev-parse", "HEAD")
	branch, _ = git("symbolic-ref", "--quiet", "--short", "HEAD")
	provenance = "working_tree"
	if status, e := git("status", "--porcelain", "--untracked-files=all"); e == nil && status == "" && revision != "" {
		provenance = "commit"
	}
	parts = append(parts, revision)
	return graph.ID(parts...), revision, branch, provenance, nil
}
