package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultWorkspaceSourceViewsDir = "views"

// LoadWorkspaceConfig reads only .tld.yaml from a workspace directory.
func LoadWorkspaceConfig(dir string) (*WorkspaceConfig, error) {
	data, err := os.ReadFile(WorkspaceConfigPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read .tld.yaml: %w", err)
	}
	cfg := &WorkspaceConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse .tld.yaml: %w", err)
	}
	return cfg, nil
}

// WorkspaceSourceViewsDir returns the configured views directory relative to the
// workspace directory unless an absolute path is configured.
func WorkspaceSourceViewsDir(cfg *WorkspaceConfig, repositoryName string) string {
	if cfg != nil && repositoryName != "" {
		if repo, ok := cfg.Repositories[repositoryName]; ok && repo.WorkspaceSource != nil {
			if value := strings.TrimSpace(repo.WorkspaceSource.ViewsDir); value != "" {
				return value
			}
		}
	}
	if cfg != nil && cfg.WorkspaceSource != nil {
		if value := strings.TrimSpace(cfg.WorkspaceSource.ViewsDir); value != "" {
			return value
		}
	}
	return DefaultWorkspaceSourceViewsDir
}

// ResolveWorkspaceSourceRoot resolves the on-disk Mermaid view root for a workspace.
func ResolveWorkspaceSourceRoot(workspaceDir, repositoryName string) (string, string, error) {
	if strings.TrimSpace(workspaceDir) == "" {
		return "", "", fmt.Errorf("workspace directory is not configured")
	}
	absWorkspaceDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace directory: %w", err)
	}
	cfg, err := LoadWorkspaceConfig(absWorkspaceDir)
	if err != nil {
		return "", "", err
	}
	viewsDir := WorkspaceSourceViewsDir(cfg, repositoryName)
	if filepath.IsAbs(viewsDir) {
		return filepath.Clean(viewsDir), viewsDir, nil
	}
	return filepath.Join(absWorkspaceDir, filepath.Clean(viewsDir)), viewsDir, nil
}
