package analyze

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd/version"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/semantic"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	watchpkg "github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/watch/exportyaml"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

func NewAnalyzeCmd(wdir *string) *cobra.Command {
	var dryRun bool
	var dataDirFlag string
	var semanticOutput string
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
			commandStarted := time.Now()
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
			logFile, logger, err := openAnalyzeLog(dataDir)
			if err != nil {
				return err
			}
			defer func() { _ = logFile.Close() }()
			var finalErr error
			fail := func(event string, err error, args ...any) error {
				finalErr = err
				fields := append([]any{"error", err}, args...)
				logger.ErrorContext(cmd.Context(), event, fields...)
				return err
			}
			defer func() {
				if finalErr != nil {
					logger.ErrorContext(cmd.Context(), "analyze.failed", "elapsed", time.Since(commandStarted).Round(time.Millisecond).String(), "error", finalErr)
					return
				}
				logger.InfoContext(cmd.Context(), "analyze.completed", "elapsed", time.Since(commandStarted).Round(time.Millisecond).String())
			}()
			logger.InfoContext(cmd.Context(), "analyze.started",
				"path", scanPath,
				"abs_path", absPath,
				"data_dir", dataDir,
				"format", formatFlag(cmd),
				"dry_run", dryRun,
				"rescan", rescan,
				"fail_on_drift", failOnDrift,
				"languages", strings.Join(languageFlags, ","),
				"semantic_output", semanticOutput,
			)
			logger.InfoContext(cmd.Context(), "analyze.setup.completed", "elapsed", time.Since(commandStarted).Round(time.Millisecond).String(), "workspace", *wdir, "data_dir", dataDir)
			embeddingCfg := resolveAnalyzeEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
			settings := resolveAnalyzeWatchSettings(cfg, languageFlags, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup)
			logger.InfoContext(cmd.Context(), "analyze.settings.resolved",
				"embedding_provider", embeddingCfg.Provider,
				"embedding_model", embeddingCfg.Model,
				"embedding_dimension", embeddingCfg.Dimension,
				"watcher", settings.Watcher,
				"languages", strings.Join(settings.Languages, ","),
				"lsp_enabled", settings.LSP.Enabled,
				"lsp_health_interval", settings.LSP.HealthInterval.String(),
				"lsp_memory_limit_bytes", settings.LSP.MemoryLimitBytes,
				"max_elements_per_view", settings.Thresholds.MaxElementsPerView,
				"max_connectors_per_view", settings.Thresholds.MaxConnectorsPerView,
			)
			if embeddingCfg.Provider != "none" {
				healthStarted := time.Now()
				logger.InfoContext(cmd.Context(), "analyze.embedding_healthcheck.started", "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				checked, _, err := watchpkg.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					return fail("analyze.embedding_healthcheck.failed", fmt.Errorf("embedding healthcheck failed: %w", err), "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				}
				embeddingCfg = checked
				logger.InfoContext(cmd.Context(), "analyze.embedding_healthcheck.completed", "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model, "dimension", embeddingCfg.Dimension)
			}
			storeStarted := time.Now()
			logger.InfoContext(cmd.Context(), "analyze.store_open.started", "database", localserver.DatabasePath(dataDir))
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return fail("analyze.store_open.failed", err, "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			}
			defer func() { _ = sqliteStore.DB().Close() }()
			logger.InfoContext(cmd.Context(), "analyze.store_open.completed", "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			watchStore := watchpkg.NewStore(sqliteStore.DB())
			progress := newAnalyzeWatchProgress(cmd.ErrOrStderr())
			if formatFlag(cmd) != "json" {
				term.PrintLogo(cmd.OutOrStdout(), version.Version)
				term.Label(cmd.OutOrStdout(), 20, "Path", scanPath)
				term.Label(cmd.OutOrStdout(), 20, "Data directory", dataDir)
				term.Label(cmd.OutOrStdout(), 20, "LSP", formatLSPStatus(watchpkg.InitialLSPStatus(settings)))
				if embeddingCfg.Provider != "none" {
					term.Label(cmd.OutOrStdout(), 20, "Embedding provider", embeddingCfg.Provider)
					term.Label(cmd.OutOrStdout(), 20, "Embedding model", embeddingCfg.Model)
				}
				if dryRun {
					term.Label(cmd.OutOrStdout(), 20, "Mode", "dry-run")
				}
			}
			once, err := watchpkg.NewRunner(watchStore).RunOnce(cmd.Context(), watchpkg.OneShotOptions{Path: absPath, Rescan: rescan, Embedding: embeddingCfg, Settings: settings, DataDir: dataDir, Progress: progress, Logger: logger, ConfirmAfterScan: confirmAnalyzeLSPProceed(cmd)})
			if err != nil {
				return fail("analyze.watch_pipeline.failed", err)
			}
			var semanticResult *semantic.ExportResult
			if strings.TrimSpace(semanticOutput) != "" {
				semanticStarted := time.Now()
				logger.InfoContext(cmd.Context(), "analyze.semantic_export.started", "repository_id", once.Scan.RepositoryID, "output", semanticOutput)
				result, err := semantic.ExportJSONL(cmd.Context(), watchStore, once.Scan.RepositoryID, semanticOutput)
				if err != nil {
					return fail("analyze.semantic_export.failed", fmt.Errorf("export semantic jsonl: %w", err), "elapsed", time.Since(semanticStarted).Round(time.Millisecond).String(), "repository_id", once.Scan.RepositoryID)
				}
				semanticResult = &result
				logger.InfoContext(cmd.Context(), "analyze.semantic_export.completed", "elapsed", time.Since(semanticStarted).Round(time.Millisecond).String(), "repository_id", once.Scan.RepositoryID, "records_written", result.RecordsWritten, "output", semanticOutput)
			}
			exportStarted := time.Now()
			logger.InfoContext(cmd.Context(), "analyze.export.started", "repository_id", once.Scan.RepositoryID)
			exported, exportResult, err := exportyaml.ExportWithProgress(cmd.Context(), sqliteStore, watchStore, ws, once.Scan.RepositoryID, progress)
			if err != nil {
				return fail("analyze.export.failed", fmt.Errorf("export yaml: %w", err), "elapsed", time.Since(exportStarted).Round(time.Millisecond).String(), "repository_id", once.Scan.RepositoryID)
			}
			logger.InfoContext(cmd.Context(), "analyze.export.completed", "elapsed", time.Since(exportStarted).Round(time.Millisecond).String(), "repository_id", once.Scan.RepositoryID, "elements_written", exportResult.ElementsWritten, "connectors_written", exportResult.ConnectorsWritten, "views_written", exportResult.ViewsWritten)
			changed := hasAnalyzeDrift(once.Diffs)
			if formatFlag(cmd) == "json" {
				payload := map[string]any{"changed": changed, "scan": once.Scan, "lsp": once.Scan.LSP, "representation": once.Representation, "export": exportResult, "diffs": once.Diffs}
				if semanticResult != nil {
					payload["semantic_export"] = map[string]any{"output": semanticOutput, "records_written": semanticResult.RecordsWritten}
				}
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); err != nil {
					return fail("analyze.output.failed", err)
				}
			} else {
				term.Separator(cmd.OutOrStdout())
				term.Successf(cmd.OutOrStdout(), "%d elements written to elements.yaml", exportResult.ElementsWritten)
				term.Successf(cmd.OutOrStdout(), "%d connectors written to connectors.yaml", exportResult.ConnectorsWritten)
				term.Successf(cmd.OutOrStdout(), "1 repository scanned")
				if semanticResult != nil {
					term.Successf(cmd.OutOrStdout(), "%d semantic records written to %s", semanticResult.RecordsWritten, semanticOutput)
				}
				term.Separator(cmd.OutOrStdout())
			}
			if dryRun {
				logger.InfoContext(cmd.Context(), "analyze.workspace_save.skipped", "reason", "dry_run", "changed", changed)
				if formatFlag(cmd) != "json" {
					term.Hint(cmd.OutOrStdout(), "No files written. Remove --dry-run to apply.")
				}
			} else {
				saveStarted := time.Now()
				logger.InfoContext(cmd.Context(), "analyze.workspace_save.started", "changed", changed)
				if err := workspace.Save(exported); err != nil {
					return fail("analyze.workspace_save.failed", fmt.Errorf("save workspace: %w", err), "elapsed", time.Since(saveStarted).Round(time.Millisecond).String())
				}
				logger.InfoContext(cmd.Context(), "analyze.workspace_save.completed", "elapsed", time.Since(saveStarted).Round(time.Millisecond).String(), "changed", changed)
			}
			if failOnDrift && changed {
				return fail("analyze.drift.failed", fmt.Errorf("watch representation drift detected"))
			}
			return nil
		},
	}

	c.Flags().BoolVar(&dryRun, "dry-run", false, "scan, materialize, and print drift without modifying workspace YAML")
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().StringVar(&semanticOutput, "semantic-output", "", "write protobuf JSONL SymbolContext records to this path")
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

func openAnalyzeLog(dataDir string) (*os.File, *slog.Logger, error) {
	path := localserver.LogPath(dataDir)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open analyze log file: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return file, logger, nil
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
		settings.Scale = watchpkg.ScaleConfig{
			Strategy:        cfg.Watch.Scale.Strategy,
			MaxTrackedFiles: cfg.Watch.Scale.MaxTrackedFiles,
			MaxLimitedFiles: cfg.Watch.Scale.MaxLimitedFiles,
		}
		settings.LSP = watchpkg.LSPConfig{
			Enabled:          cfg.Watch.LSP.Enabled,
			HealthInterval:   parseAnalyzeDurationOrZero(cfg.Watch.LSP.HealthInterval),
			MemoryLimitBytes: cfg.Watch.LSP.MemoryLimitBytes,
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

func formatLSPStatus(status watchpkg.LSPStatus) string {
	if !status.Enabled {
		return "disabled"
	}
	return fmt.Sprintf("requested=%d available=%d active=%d degraded=%d",
		status.Summary.Requested,
		status.Summary.Available,
		status.Summary.Active,
		status.Summary.Unavailable+status.Summary.Failed,
	)
}

func confirmAnalyzeLSPProceed(cmd *cobra.Command) func(context.Context, watchpkg.ScanResult) error {
	if formatFlag(cmd) == "json" {
		return nil
	}
	return func(_ context.Context, scan watchpkg.ScanResult) error {
		return confirmLSPProceed(cmd, scan.LSP)
	}
}

func confirmLSPProceed(cmd *cobra.Command, status watchpkg.LSPStatus) error {
	if !watchpkg.LSPNeedsConfirmation(status) {
		return nil
	}
	term.Warn(cmd.OutOrStdout(), "Some requested language servers are unavailable or unhealthy. Reference resolution quality will be lower.")
	term.Hint(cmd.OutOrStdout(), "tld will fall back to conservative name matching, which may drop ambiguous connectors.")
	for _, server := range watchpkg.LSPDegradedServers(status) {
		detail := server.Command
		if detail == "" {
			detail = server.Language
		}
		if server.LastError != "" {
			detail += ": " + server.LastError
		}
		term.Hint(cmd.OutOrStdout(), detail)
	}
	term.Hint(cmd.OutOrStdout(), "Remediation: install the missing language server, ensure it is on PATH, or disable LSP with `tld config set watch.lsp.enabled false`.")
	term.Hint(cmd.OutOrStdout(), "Examples: gopls, pyright-langserver, rust-analyzer, jdtls, typescript-language-server, clangd.")
	if !isInteractiveInput(cmd.InOrStdin()) {
		term.Hint(cmd.OutOrStdout(), "Non-interactive input detected; continuing without confirmation.")
		return nil
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "  Continue with lower-quality reference resolution? [yes/no]: ")
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		return errors.New("aborted")
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "yes" && answer != "y" {
		return errors.New("aborted: LSP confirmation declined")
	}
	return nil
}

func isInteractiveInput(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

type analyzeWatchProgress struct {
	out io.Writer
	bar *progressbar.ProgressBar
	mu  sync.Mutex
}

func newAnalyzeWatchProgress(out io.Writer) *analyzeWatchProgress {
	return &analyzeWatchProgress{out: out}
}

func (p *analyzeWatchProgress) Start(label string, total int) {
	if p == nil || p.out == nil || total <= 0 || !term.IsTerminal(p.out) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bar != nil {
		_ = p.bar.Finish()
		p.bar = nil
	}
	p.bar = newAnalyzeProgressBar(p.out, total)
	if p.bar != nil {
		p.bar.Describe(label)
	}
}

func (p *analyzeWatchProgress) Advance(label string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bar == nil {
		return
	}
	if label != "" {
		p.bar.Describe(label)
	}
	_ = p.bar.Add(1)
}

func (p *analyzeWatchProgress) Finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bar == nil {
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
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionThrottle(60*time.Millisecond),
	)
}
