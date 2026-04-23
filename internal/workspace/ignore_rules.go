package workspace

import "github.com/mertcikla/tld/internal/ignore"

// IgnoreRulesForRepository returns the merged global and repository-specific
// exclusion rules for the named repository.
func (ws *Workspace) IgnoreRulesForRepository(repoName string) *ignore.Rules {
	if ws == nil || ws.WorkspaceConfig == nil {
		return nil
	}

	patterns := append([]string{}, ws.WorkspaceConfig.Exclude...)
	if repository, ok := ws.WorkspaceConfig.Repositories[repoName]; ok {
		patterns = append(patterns, repository.Exclude...)
	}

	return ignore.Merge(&ignore.Rules{Exclude: patterns})
}
