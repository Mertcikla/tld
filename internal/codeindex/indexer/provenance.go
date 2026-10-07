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

// Bump when relationship semantics change, independently of syntax caches.
const relationshipVersion = 1

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
		ExtractionVersion      int
		RelationshipVersion    int
	}{cfg, req.Exclude, req.ProjectRoots, req.ScipArtifacts, syntaxCacheVersion, relationshipVersion})
	return graph.Hash(raw)
}

func captureInputs(ctx context.Context, root string, cfg config.Config, req *pb.IndexRequest) (fingerprint, revision, branch, provenance string, err error) {
	projects, sources, err := Discover(ctx, root, req.ProjectRoots, req.Exclude)
	if err != nil {
		return "", "", "", "", err
	}
	fingerprint, revision, branch, provenance, _, err = fingerprintInputs(ctx, root, cfg, req, projects, sources)
	return fingerprint, revision, branch, provenance, err
}

// commitSubject returns the subject line of HEAD in root. It returns "" when
// root is not a Git repository or has no commits, so snapshot capture never
// fails and non-Git workspaces simply leave the field empty.
func commitSubject(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "log", "-1", "--format=%s")
	raw, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// fingerprintInputs hashes every input that determines a snapshot. It returns
// the aggregate fingerprint plus a per-input digest so callers can report
// exactly which inputs moved when a repository changes mid-index.
func fingerprintInputs(ctx context.Context, root string, cfg config.Config, req *pb.IndexRequest, projects []*pb.Project, sources map[string]*graph.Source) (fingerprint, revision, branch, provenance string, digest map[string]string, err error) {
	digest = map[string]string{}
	parts := []string{ConfigurationHash(cfg, req)}
	digest["config"] = parts[0]
	artifactRoots := make([]string, 0, len(req.ScipArtifacts))
	for key := range req.ScipArtifacts {
		artifactRoots = append(artifactRoots, key)
	}
	sort.Strings(artifactRoots)
	for _, key := range artifactRoots {
		raw, e := os.ReadFile(req.ScipArtifacts[key])
		if e != nil {
			return "", "", "", "", nil, e
		}
		hash := graph.Hash(raw)
		parts = append(parts, key, hash)
		digest["scip:"+key] = hash
	}
	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		parts = append(parts, path, sources[path].Hash)
		digest["source:"+path] = sources[path].Hash
	}
	for _, project := range projects {
		content, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(project.ConfigPath)))
		if readErr != nil {
			return "", "", "", "", nil, fmt.Errorf("read project configuration: %w", readErr)
		}
		hash := graph.Hash(content)
		parts = append(parts, project.ConfigPath, hash)
		digest["project:"+project.ConfigPath] = hash
		if languageFamily(project.Language) == familyWeb {
			projectDir := filepath.Join(root, filepath.FromSlash(project.Root))
			for _, name := range webProjectConfigs(projectDir) {
				extra, extraErr := os.ReadFile(filepath.Join(projectDir, name))
				if extraErr != nil {
					return "", "", "", "", nil, fmt.Errorf("read project configuration: %w", extraErr)
				}
				extraHash := graph.Hash(extra)
				rel := filepath.ToSlash(filepath.Join(project.Root, name))
				parts = append(parts, "tsconfig:"+rel, extraHash)
				digest["tsconfig:"+rel] = extraHash
			}
		}
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
	digest["revision"] = revision
	return graph.ID(parts...), revision, branch, provenance, digest, nil
}

// inputDrift renders the difference between two fingerprint digests as a
// bounded, sorted list of markers: "+" for an input that appeared, "-" for one
// that disappeared, and "~" for one whose content changed. It exists so the
// mid-index drift error names the files or configurations that moved.
func inputDrift(before, after map[string]string) string {
	markers := make([]string, 0, len(before)+len(after))
	for key, from := range before {
		to, ok := after[key]
		switch {
		case !ok:
			markers = append(markers, "-"+key)
		case from != to:
			markers = append(markers, fmt.Sprintf("~%s(%s->%s)", key, shortDigest(from), shortDigest(to)))
		}
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			markers = append(markers, "+"+key)
		}
	}
	if len(markers) == 0 {
		return "unknown (fingerprints differ without a per-input difference)"
	}
	sort.Strings(markers)
	const maxMarkers = 12
	if len(markers) > maxMarkers {
		return strings.Join(markers[:maxMarkers], ", ") + fmt.Sprintf(" and %d more", len(markers)-maxMarkers)
	}
	return strings.Join(markers, ", ")
}

// shortDigest abbreviates a digest value for drift messages.
func shortDigest(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
