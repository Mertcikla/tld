package analyze

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	assets "github.com/mertcikla/tld"
	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/store"
	"github.com/mertcikla/tld/internal/term"
	watchpkg "github.com/mertcikla/tld/internal/watch"
	"github.com/mertcikla/tld/internal/watch/exportyaml"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

func NewAnalyzeCmd(wdir *string) *cobra.Command {
	var dryRun bool
	var dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel string
	var embeddingDimension int
	var languageFlags []string
	var rescan, failOnDrift bool
	var maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int

	c := &cobra.Command{
		Use:   "analyze <path>",
		Short: "Scan and materialize a source repository into workspace YAML",
		Long: `Scans the git repository containing the given path through the watch pipeline,
materializes the canonical SQLite code representation, and exports generated resources
to elements.yaml and connectors.yaml. Manual YAML resources are preserved.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scanPath := args[0]
			absPath, err := filepath.Abs(scanPath)
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}
			if _, err := os.Stat(absPath); err != nil {
				return fmt.Errorf("path %q not found: %w", scanPath, err)
			}
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}
			embeddingCfg := resolveAnalyzeEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
			settings := resolveAnalyzeWatchSettings(cfg, languageFlags, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup)
			if embeddingCfg.Provider != "none" {
				checked, _, err := watchpkg.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					return fmt.Errorf("embedding healthcheck failed: %w", err)
				}
				embeddingCfg = checked
			}
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return err
			}
			defer func() { _ = sqliteStore.DB().Close() }()
			watchStore := watchpkg.NewStore(sqliteStore.DB())
			progress := newAnalyzeWatchProgress(cmd.ErrOrStderr())
			linePrefix := ""
			if dryRun {
				linePrefix = "[dry-run] "
			}
			if formatFlag(cmd) != "json" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sAnalyzing %s (watch pipeline)...\n", linePrefix, scanPath)
			}
			once, err := watchpkg.NewRunner(watchStore).RunOnce(cmd.Context(), watchpkg.OneShotOptions{Path: absPath, Rescan: rescan, Embedding: embeddingCfg, Settings: settings, Progress: progress})
			if err != nil {
				return err
			}
			exported, exportResult, err := exportyaml.Export(cmd.Context(), sqliteStore, watchStore, ws, once.Scan.RepositoryID)
			if err != nil {
				return fmt.Errorf("export yaml: %w", err)
			}
			changed := hasAnalyzeDrift(once.Diffs)
			if formatFlag(cmd) == "json" {
				payload := map[string]any{"changed": changed, "scan": once.Scan, "representation": once.Representation, "export": exportResult, "diffs": once.Diffs}
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  OK  %d elements written to elements.yaml\n", linePrefix, exportResult.ElementsWritten)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  OK  %d connectors written to connectors.yaml\n", linePrefix, exportResult.ConnectorsWritten)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  OK  1 repository scanned\n", linePrefix)
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}
			if dryRun {
				if formatFlag(cmd) != "json" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%sNo files written. Remove --dry-run to apply.\n", linePrefix)
				}
			} else if err := workspace.Save(exported); err != nil {
				return fmt.Errorf("save workspace: %w", err)
			}
			if failOnDrift && changed {
				return fmt.Errorf("watch representation drift detected")
			}
			return nil
		},
	}

	c.Flags().BoolVar(&dryRun, "dry-run", false, "scan, materialize, and print drift without modifying workspace YAML")
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().BoolVar(&rescan, "rescan", false, "force reparsing files even if cached")
	c.Flags().BoolVar(&failOnDrift, "fail-on-drift", false, "exit nonzero when representation drift is detected")
	c.Flags().StringSliceVar(&languageFlags, "language", nil, "source language to scan (repeatable)")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for representation")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for representation")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for representation")
	c.Flags().IntVar(&embeddingDimension, "embedding-dimension", 0, "embedding vector dimension")
	c.Flags().IntVar(&maxElements, "max-elements-per-view", 0, "maximum generated elements per view")
	c.Flags().IntVar(&maxConnectors, "max-connectors-per-view", 0, "maximum generated connectors per view")
	c.Flags().IntVar(&maxIncoming, "max-incoming-per-element", 0, "maximum incoming references per element before collapsing")
	c.Flags().IntVar(&maxOutgoing, "max-outgoing-per-element", 0, "maximum outgoing references per element before collapsing")
	c.Flags().IntVar(&maxExpandedGroup, "max-expanded-connectors-per-group", 0, "maximum file-pair connectors to expand before collapsing to a folder connector")
	return c
}

func resolveAnalyzeEmbeddingConfig(cfg *workspace.Config, provider, endpoint, model string, dimension int) watchpkg.EmbeddingConfig {
	embedding := watchpkg.EmbeddingConfig{Provider: "none"}
	if cfg != nil {
		embedding = watchpkg.EmbeddingConfig{
			Provider:        cfg.Watch.Embedding.Provider,
			Endpoint:        cfg.Watch.Embedding.Endpoint,
			Model:           cfg.Watch.Embedding.Model,
			Dimension:       cfg.Watch.Embedding.Dimension,
			HealthThreshold: cfg.Watch.Embedding.HealthThreshold,
		}
	}
	if provider != "" {
		embedding.Provider = provider
	}
	if endpoint != "" {
		embedding.Endpoint = endpoint
	}
	if model != "" {
		embedding.Model = model
	}
	if dimension > 0 {
		embedding.Dimension = dimension
	}
	return watchpkg.NormalizeEmbeddingConfig(embedding)
}

func resolveAnalyzeWatchSettings(cfg *workspace.Config, languages []string, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int) watchpkg.Settings {
	settings := watchpkg.DefaultSettings()
	if cfg != nil {
		settings.Languages = cfg.Watch.Languages
		settings.Watcher = cfg.Watch.Watcher
		settings.PollInterval = parseAnalyzeDurationOrZero(cfg.Watch.PollInterval)
		settings.Debounce = parseAnalyzeDurationOrZero(cfg.Watch.Debounce)
		settings.Thresholds = watchpkg.Thresholds{
			MaxElementsPerView:            cfg.Watch.Thresholds.MaxElementsPerView,
			MaxConnectorsPerView:          cfg.Watch.Thresholds.MaxConnectorsPerView,
			MaxIncomingPerElement:         cfg.Watch.Thresholds.MaxIncomingPerElement,
			MaxOutgoingPerElement:         cfg.Watch.Thresholds.MaxOutgoingPerElement,
			MaxExpandedConnectorsPerGroup: cfg.Watch.Thresholds.MaxExpandedConnectorsPerGroup,
		}
		settings.Visibility = watchpkg.VisibilityConfig{
			CoreThresholdEnabled:   cfg.Watch.Visibility.CoreThresholdEnabled,
			CoreThreshold:          cfg.Watch.Visibility.CoreThreshold,
			TierMultiplier:         cfg.Watch.Visibility.TierMultiplier,
			MaxExpansionMultiplier: cfg.Watch.Visibility.MaxExpansionMultiplier,
			CoreThresholdSet:       true,
			WeightsSet:             true,
			Weights: watchpkg.VisibilityWeights{
				Changed:               cfg.Watch.Visibility.Weights.Changed,
				Selected:              cfg.Watch.Visibility.Weights.Selected,
				UserShow:              cfg.Watch.Visibility.Weights.UserShow,
				UserHide:              cfg.Watch.Visibility.Weights.UserHide,
				HighSignalFact:        cfg.Watch.Visibility.Weights.HighSignalFact,
				RelationshipProximity: cfg.Watch.Visibility.Weights.RelationshipProximity,
				DependencyFact:        cfg.Watch.Visibility.Weights.DependencyFact,
				UtilityNoise:          cfg.Watch.Visibility.Weights.UtilityNoise,
				HighDegreeNoise:       cfg.Watch.Visibility.Weights.HighDegreeNoise,
			},
		}
	}
	if len(languages) > 0 {
		settings.Languages = languages
	}
	if maxElements > 0 {
		settings.Thresholds.MaxElementsPerView = maxElements
	}
	if maxConnectors > 0 {
		settings.Thresholds.MaxConnectorsPerView = maxConnectors
	}
	if maxIncoming > 0 {
		settings.Thresholds.MaxIncomingPerElement = maxIncoming
	}
	if maxOutgoing > 0 {
		settings.Thresholds.MaxOutgoingPerElement = maxOutgoing
	}
	if maxExpandedGroup > 0 {
		settings.Thresholds.MaxExpandedConnectorsPerGroup = maxExpandedGroup
	}
	return watchpkg.NormalizeSettings(settings)
}

func parseAnalyzeDurationOrZero(value string) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func formatFlag(cmd *cobra.Command) string {
	flag := cmd.Root().Flag("format")
	if flag == nil || strings.TrimSpace(flag.Value.String()) == "" {
		return "text"
	}
	return strings.ToLower(strings.TrimSpace(flag.Value.String()))
}

func hasAnalyzeDrift(diffs []watchpkg.RepresentationDiff) bool {
	for _, diff := range diffs {
		if diff.ChangeType != "initialized" && diff.OwnerType != "repository" {
			return true
		}
	}
	return false
}

type analyzeWatchProgress struct {
	out io.Writer
	bar *progressbar.ProgressBar
}

func newAnalyzeWatchProgress(out io.Writer) *analyzeWatchProgress {
	return &analyzeWatchProgress{out: out}
}

func (p *analyzeWatchProgress) Start(label string, total int) {
	if p == nil || p.out == nil || total <= 0 {
		return
	}
	p.bar = newAnalyzeProgressBar(p.out, total)
	if p.bar != nil {
		p.bar.Describe(label)
	}
}

func (p *analyzeWatchProgress) Advance(label string) {
	if p == nil || p.bar == nil {
		return
	}
	if label != "" {
		p.bar.Describe(label)
	}
	_ = p.bar.Add(1)
}

func (p *analyzeWatchProgress) Finish() {
	if p == nil || p.bar == nil {
		return
	}
	_ = p.bar.Finish()
	p.bar = nil
}

func newAnalyzeProgressBar(out io.Writer, total int) *progressbar.ProgressBar {
	if total <= 0 || !term.IsTerminal(out) {
		return nil
	}
	return progressbar.NewOptions(total,
		progressbar.OptionSetWriter(out),
		progressbar.OptionSetVisibility(true),
		progressbar.OptionSetDescription("Scanning"),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetWidth(12),
		progressbar.OptionFullWidth(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionThrottle(60*time.Millisecond),
	)
}
