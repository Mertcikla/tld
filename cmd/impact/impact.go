package impact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	assets "github.com/mertcikla/tld/v2"
	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/store"
	watchpkg "github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// NewImpactCmd reports how a change set touches authored architecture. It is
// deterministic by default and never rewrites the architecture.
func NewImpactCmd(wdir *string) *cobra.Command {
	var base string
	var render string
	var diagram string
	var includeWorktree, evidence, rescan, suggestBindings, nameHeuristics bool
	var dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel string

	c := &cobra.Command{
		Use:   "impact [path]",
		Short: "Report how a code change set affects authored architecture",
		Long: `Reconciles a git change set against the authored workspace architecture.

By default it uses only deterministic git + path/glob bindings. Optional
Tree-sitter/LSP evidence (--evidence) detects observed implementation
relationships, and embedding suggestions (--suggest-bindings) can propose
owners for unmapped code. Nothing here mutates the architecture.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			absPath, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}
			if _, err := os.Stat(absPath); err != nil {
				return fmt.Errorf("path %q not found: %w", path, err)
			}
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			repoRoot, err := tldgit.RepoRoot(absPath)
			if err != nil {
				return fmt.Errorf("%s is not inside a git repository: %w", path, err)
			}
			remoteURL, _ := tldgit.DetectRemoteURL(repoRoot)
			head, _ := tldgit.DetectHeadCommit(repoRoot)

			changed, err := tldgit.FileChangesAgainstBase(repoRoot, base)
			if err != nil {
				return err
			}
			lineStats := map[string]tldgit.LineDiff{}
			if mergeBase, err := tldgit.MergeBase(repoRoot, base); err == nil {
				if stats, err := tldgit.FileLineStatsBetween(repoRoot, mergeBase, "HEAD"); err == nil {
					lineStats = stats
				}
			}
			if includeWorktree {
				worktree, err := tldgit.WorktreeChangesAgainstHead(repoRoot)
				if err != nil {
					return err
				}
				for file, change := range worktree {
					changed[file] = change
				}
			}

			opts := watchpkg.ImpactOptions{
				Base:                  base,
				Head:                  head,
				RepoRoot:              repoRoot,
				RemoteURL:             remoteURL,
				Elements:              ws.Elements,
				Connectors:            ws.Connectors,
				ChangedFiles:          changed,
				LineStats:             lineStats,
				IncludeNameHeuristics: nameHeuristics,
			}

			cfg, cfgErr := workspace.LoadGlobalConfig()
			if cfgErr != nil {
				if evidence || suggestBindings {
					return fmt.Errorf("load global config: %w", cfgErr)
				}
				cfg = nil
			}

			if evidence {
				relationships, err := detectRelationships(cmd, cfg, dataDirFlag, rescan, repoRoot, remoteURL, head, ws, changed)
				if err != nil {
					return err
				}
				opts.Relationships = relationships
			}

			report := watchpkg.AnalyzeImpact(opts)

			if suggestBindings {
				suggestions, err := suggestOwners(cmd, cfg, ws, repoRoot, report.Unmapped, embeddingProvider, embeddingEndpoint, embeddingModel)
				if err != nil {
					return err
				}
				opts.Suggestions = suggestions
				report = watchpkg.AnalyzeImpact(opts)
			}

			if formatFlag(cmd) == "json" {
				return watchpkg.RenderImpactJSON(cmd.OutOrStdout(), report)
			}
			style := watchpkg.NormalizeDiagramStyle(diagram)
			switch strings.ToLower(strings.TrimSpace(render)) {
			case "markdown", "md":
				return watchpkg.RenderImpactMarkdownStyle(cmd.OutOrStdout(), report, style)
			case "mermaid":
				return watchpkg.RenderImpactMermaidStyle(cmd.OutOrStdout(), report, style)
			default:
				return watchpkg.RenderImpactText(cmd.OutOrStdout(), report)
			}
		},
	}

	c.Flags().StringVar(&base, "base", "main", "git ref to diff against (uses the merge base)")
	c.Flags().StringVar(&render, "render", "text", "output renderer: text, markdown, or mermaid")
	c.Flags().StringVar(&diagram, "diagram", "full", "diagram style for markdown/mermaid: full, bounded, lanes, or groups")
	c.Flags().BoolVar(&includeWorktree, "include-worktree", false, "also include uncommitted worktree changes")
	c.Flags().BoolVar(&nameHeuristics, "name-heuristics", true, "derive weak candidate bindings from element names when no file path is set")
	c.Flags().BoolVar(&evidence, "evidence", false, "detect observed implementation relationships (Tree-sitter/LSP)")
	c.Flags().BoolVar(&rescan, "rescan", false, "force reparsing changed files when collecting evidence")
	c.Flags().BoolVar(&suggestBindings, "suggest-bindings", false, "suggest architecture owners for unmapped code using embeddings")
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for suggestions")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for suggestions")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for suggestions")
	return c
}

func detectRelationships(cmd *cobra.Command, cfg *workspace.Config, dataDirFlag string, rescan bool, repoRoot, remoteURL, head string, ws *workspace.Workspace, changed map[string]tldgit.WorktreeChange) ([]watchpkg.RelationshipEvidence, error) {
	dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	sqliteStore, err := store.OpenLocal(cmd.Context(), cfg, dataDir, assets.FS)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sqliteStore.Close() }()
	watchStore := watchpkg.NewStoreWithBun(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())

	files := make([]string, 0, len(changed))
	for file := range changed {
		files = append(files, file)
	}
	branch, _ := tldgit.DetectBranch(repoRoot)
	return watchpkg.DetectObservedRelationships(cmd.Context(), watchStore, watchpkg.RelationshipOptions{
		RepoRoot:     repoRoot,
		RemoteURL:    remoteURL,
		Branch:       branch,
		HeadCommit:   head,
		ChangedFiles: files,
		Elements:     ws.Elements,
		Settings:     watchpkg.ResolveSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0),
		DataDir:      dataDir,
		Force:        rescan,
	})
}

func suggestOwners(cmd *cobra.Command, cfg *workspace.Config, ws *workspace.Workspace, repoRoot string, unmapped []string, provider, endpoint, model string) ([]watchpkg.BindingSuggestion, error) {
	embedding := watchpkg.ResolveEmbeddingConfig(cfg, provider, endpoint, model, 0, 0)
	return watchpkg.SuggestBindings(cmd.Context(), watchpkg.SuggestionOptions{
		RepoRoot:  repoRoot,
		Elements:  ws.Elements,
		Unmapped:  unmapped,
		Embedding: embedding,
	})
}

func formatFlag(cmd *cobra.Command) string {
	flag := cmd.Root().Flag("format")
	if flag == nil || strings.TrimSpace(flag.Value.String()) == "" {
		return "text"
	}
	return strings.ToLower(strings.TrimSpace(flag.Value.String()))
}
