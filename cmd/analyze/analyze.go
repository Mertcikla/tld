package analyze

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd/version"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/ignore"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	watchpkg "github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/watch/exportyaml"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewAnalyzeCmd(wdir *string) *cobra.Command {
	var dryRun bool
	var dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel, embeddingRuntimePath string
	var embeddingDimension, embeddingMaxTokens int
	var languageFlags []string
	var rescan, failOnDrift, verbose bool
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
			scopes, err := cmdutil.ResolveAnalyzeRepoScopes(ws, absPath)
			if err != nil {
				return err
			}
			var rules *ignore.Rules
			if len(scopes) > 0 {
				rules = ws.IgnoreRulesForRepository(scopes[0].Name)
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
				"verbose", verbose,
				"languages", strings.Join(languageFlags, ","),
			)
			logger.InfoContext(cmd.Context(), "analyze.setup.completed", "elapsed", time.Since(commandStarted).Round(time.Millisecond).String(), "workspace", *wdir, "data_dir", dataDir)
			embeddingCfg := resolveAnalyzeEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension, embeddingMaxTokens, embeddingRuntimePath)
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
			var progress watchpkg.ProgressSink
			var progressHistory *bytes.Buffer
			if formatFlag(cmd) != "json" {
				progress, progressHistory = newAnalyzeWatchProgress(cmd.ErrOrStderr())
			}
			if embeddingCfg.Provider != "none" {
				if progress != nil {
					progress.Start("Checking embedding provider", 1)
				}
				healthStarted := time.Now()
				logger.InfoContext(cmd.Context(), "analyze.embedding_healthcheck.started", "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				checked, _, err := watchpkg.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					if progress != nil {
						progress.Finish()
					}
					return fail("analyze.embedding_healthcheck.failed", fmt.Errorf("embedding healthcheck failed: %w", err), "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				}
				embeddingCfg = checked
				logger.InfoContext(cmd.Context(), "analyze.embedding_healthcheck.completed", "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model, "dimension", embeddingCfg.Dimension)
				if progress != nil {
					progress.Advance("")
					progress.Finish()
				}
			}
			storeStarted := time.Now()
			logger.InfoContext(cmd.Context(), "analyze.store_open.started", "database", localserver.DatabasePath(dataDir))
			if progress != nil {
				progress.Start("Opening workspace database", 1)
			}
			sqliteStore, err := store.OpenLocal(cmd.Context(), cfg, dataDir, assets.FS)
			if err != nil {
				if progress != nil {
					progress.Finish()
				}
				return fail("analyze.store_open.failed", err, "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			}
			defer func() { _ = sqliteStore.Close() }()
			if progress != nil {
				progress.Advance("")
				progress.Finish()
			}
			logger.InfoContext(cmd.Context(), "analyze.store_open.completed", "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			watchStore := watchpkg.NewStoreWithBun(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
			once, err := watchpkg.NewRunner(watchStore).RunOnce(cmd.Context(), watchpkg.OneShotOptions{Path: absPath, Rescan: rescan, Embedding: embeddingCfg, Settings: settings, DataDir: dataDir, Progress: progress, Logger: logger, ConfirmAfterScan: confirmAnalyzeLSPProceed(cmd), Rules: rules})
			if err != nil {
				return fail("analyze.watch_pipeline.failed", err)
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
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); err != nil {
					return fail("analyze.output.failed", err)
				}
			}
			if dryRun {
				logger.InfoContext(cmd.Context(), "analyze.workspace_save.skipped", "reason", "dry_run", "changed", changed)
			} else {
				saveStarted := time.Now()
				logger.InfoContext(cmd.Context(), "analyze.workspace_save.started", "changed", changed)
				if err := workspace.Save(exported); err != nil {
					return fail("analyze.workspace_save.failed", fmt.Errorf("save workspace: %w", err), "elapsed", time.Since(saveStarted).Round(time.Millisecond).String())
				}
				logger.InfoContext(cmd.Context(), "analyze.workspace_save.completed", "elapsed", time.Since(saveStarted).Round(time.Millisecond).String(), "changed", changed)
			}
			if formatFlag(cmd) != "json" {
				renderAnalyzeComplete(cmd.OutOrStdout(), cmd.ErrOrStderr(), analyzeCompleteView{
					Version:           version.Version,
					Path:              scanPath,
					DataDir:           dataDir,
					LSP:               formatLSPStatus(watchpkg.InitialLSPStatus(settings)),
					EmbeddingProvider: embeddingCfg.Provider,
					EmbeddingModel:    embeddingCfg.Model,
					DryRun:            dryRun,
					Changed:           changed,
					Duration:          time.Since(commandStarted).Round(time.Second),
					Scan:              once.Scan,
					Export:            exportResult,
					ProgressHistory:   progressHistory,
					Verbose:           verbose,
				})
			}
			if failOnDrift && changed {
				return fail("analyze.drift.failed", fmt.Errorf("watch representation drift detected"))
			}
			return nil
		},
	}

	c.Flags().BoolVar(&dryRun, "dry-run", false, "scan, materialize, and print drift without modifying workspace YAML")
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().BoolVar(&rescan, "rescan", false, "force reparsing files even if cached")
	c.Flags().BoolVar(&failOnDrift, "fail-on-drift", false, "exit nonzero when representation drift is detected")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "print completed pipeline phase history")
	c.Flags().StringSliceVar(&languageFlags, "language", nil, "source language to scan (repeatable)")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for representation")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for representation")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for representation")
	c.Flags().IntVar(&embeddingDimension, "embedding-dimension", 0, "embedding vector dimension")
	c.Flags().IntVar(&embeddingMaxTokens, "embedding-max-tokens", 0, "maximum input token length for embedding model")
	c.Flags().StringVar(&embeddingRuntimePath, "embedding-runtime-path", "", "ONNX Runtime shared library path for local embedding providers")
	c.Flags().IntVar(&maxElements, "max-elements-per-view", 0, "maximum generated elements per view")
	c.Flags().IntVar(&maxConnectors, "max-connectors-per-view", 0, "maximum generated connectors per view")
	c.Flags().IntVar(&maxIncoming, "max-incoming-per-element", 0, "maximum incoming references per element before collapsing")
	c.Flags().IntVar(&maxOutgoing, "max-outgoing-per-element", 0, "maximum outgoing references per element before collapsing")
	c.Flags().IntVar(&maxExpandedGroup, "max-expanded-connectors-per-group", 0, "maximum file-pair connectors to expand before collapsing to a folder connector")
	return c
}

type analyzeCompleteView struct {
	Version           string
	Path              string
	DataDir           string
	LSP               string
	EmbeddingProvider string
	EmbeddingModel    string
	DryRun            bool
	Changed           bool
	Duration          time.Duration
	Scan              watchpkg.ScanResult
	Export            exportyaml.Result
	ProgressHistory   *bytes.Buffer
	Verbose           bool
}

type analyzeRow struct {
	Label string
	Value string
}

func renderAnalyzeComplete(out, progressOut io.Writer, view analyzeCompleteView) {
	if term.IsTerminal(progressOut) {
		_, _ = fmt.Fprint(progressOut, "\r\033[K")
	}
	_, _ = fmt.Fprintf(out, "%s %s %s\n", term.Colorize(out, term.ColorCyan+term.ColorBold, "tld"), "analyze", view.Version)
	renderAnalyzeSection(out, "Workspace", []analyzeRow{
		{Label: "Path", Value: view.Path},
		{Label: "Data directory", Value: term.Path(out, view.DataDir)},
	})
	runtimeRows := []analyzeRow{{Label: "LSP", Value: view.LSP}}
	if view.EmbeddingProvider != "" && view.EmbeddingProvider != "none" {
		runtimeRows = append(runtimeRows,
			analyzeRow{Label: "Embedding", Value: strings.TrimSpace(view.EmbeddingProvider + " " + view.EmbeddingModel)},
		)
	}
	if view.DryRun {
		runtimeRows = append(runtimeRows, analyzeRow{Label: "Mode", Value: "dry-run"})
	}
	renderAnalyzeSection(out, "Runtime", runtimeRows)
	if view.Verbose {
		renderAnalyzeProgressHistory(out, view.ProgressHistory)
	}
	printAnalyzeScanModeWarning(out, view.Scan)
	status := "unchanged"
	if view.Changed {
		status = "changed"
	}
	renderAnalyzeSection(out, "Results", []analyzeRow{
		{Label: "Elements", Value: fmt.Sprintf("%d written to elements.yaml", view.Export.ElementsWritten)},
		{Label: "Connectors", Value: fmt.Sprintf("%d written to connectors.yaml", view.Export.ConnectorsWritten)},
		{Label: "Views", Value: fmt.Sprintf("%d written to views.yaml", view.Export.ViewsWritten)},
		{Label: "Repositories", Value: "1 scanned"},
		{Label: "Workspace", Value: status},
		{Label: "Duration", Value: formatAnalyzeDuration(view.Duration)},
	})
	if view.DryRun {
		term.Hint(out, "No files written. Remove --dry-run to apply.")
	}
}

func renderAnalyzeSection(out io.Writer, title string, rows []analyzeRow) {
	if len(rows) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", term.Colorize(out, term.ColorBold, title))
	renderAnalyzeRows(out, rows)
}

func renderAnalyzeRows(out io.Writer, rows []analyzeRow) {
	width := 0
	for _, row := range rows {
		if row.Value == "" {
			continue
		}
		if len(row.Label) > width {
			width = len(row.Label)
		}
	}
	for _, row := range rows {
		if row.Value == "" {
			continue
		}
		_, _ = fmt.Fprintf(out, "  %-*s  %s\n", width, row.Label, row.Value)
	}
}

func renderAnalyzeProgressHistory(out io.Writer, history *bytes.Buffer) {
	if history == nil || history.Len() == 0 {
		return
	}
	lines := strings.Split(strings.TrimSpace(history.String()), "\n")
	if len(lines) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", term.Colorize(out, term.ColorBold, "Pipeline"))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "Finished ")
		_, _ = fmt.Fprintf(out, "  %s\n", line)
	}
}

func formatAnalyzeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm%02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
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

func resolveAnalyzeEmbeddingConfig(cfg *workspace.Config, provider, endpoint, model string, dimension int, maxTokens int, runtimePath ...string) watchpkg.EmbeddingConfig {
	embedding := watchpkg.EmbeddingConfig{Provider: "none"}
	if cfg != nil {
		embedding = watchpkg.EmbeddingConfig{
			Provider:        cfg.Watch.Embedding.Provider,
			Endpoint:        cfg.Watch.Embedding.Endpoint.String(),
			Model:           cfg.Watch.Embedding.Model,
			Dimension:       cfg.Watch.Embedding.Dimension,
			RuntimePath:     cfg.Watch.Embedding.RuntimePath,
			HealthThreshold: cfg.Watch.Embedding.HealthThreshold,
			MaxTokens:       cfg.Watch.Embedding.MaxTokens,
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
	if maxTokens > 0 {
		embedding.MaxTokens = maxTokens
	}
	if len(runtimePath) > 0 && runtimePath[0] != "" {
		embedding.RuntimePath = runtimePath[0]
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
		settings.Dependencies = watchpkg.DependencyConfig{
			Enabled: cfg.Watch.Dependencies.Enabled,
		}
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
			Commands:         cfg.Watch.LSP.Commands,
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

func printAnalyzeScanModeWarning(out io.Writer, scan watchpkg.ScanResult) {
	if scan.Mode != watchpkg.ScanStrategyLimited {
		return
	}
	reason := strings.TrimSpace(scan.StrategyReason)
	if reason == "" {
		reason = "repository exceeded the configured scale threshold"
	}
	term.Warnf(out, "Limited scan mode active: %s.", reason)
	if scan.TrackedFiles > 0 {
		term.Hint(out, "Scanned recent files plus bounded reference/caller context; source symbols and connectors may still be omitted.")
		term.Hint(out, fmt.Sprintf("Selected %d file(s) out of %d tracked file(s); skipped %d tracked file(s).", scan.FilesSeen, scan.TrackedFiles, scan.SkippedTrackedFiles))
	} else {
		term.Hint(out, "Scanned recent files plus bounded reference/caller context; source symbols and connectors may still be omitted.")
	}
	if scan.RecentFiles > 0 || scan.AnchorFiles > 0 || scan.NeighborFiles > 0 || scan.CallerFiles > 0 {
		term.Hint(out, fmt.Sprintf("Limited expansion: recent=%d anchors=%d neighbors=%d callers=%d depth=%d shared_ancestor=%t cap_reached=%t.",
			scan.RecentFiles,
			scan.AnchorFiles,
			scan.NeighborFiles,
			scan.CallerFiles,
			scan.CallerDepthReached,
			scan.SharedAncestorFound,
			scan.LimitedCapReached,
		))
	}
	if scan.LimitedFallback != "" {
		term.Hint(out, "Limited expansion fallback: "+scan.LimitedFallback+".")
	}
	term.Hint(out, "Use `tld config set watch.scale.strategy full` or raise `watch.scale.max_tracked_files` for a full scan.")
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
	hasUnavailable := false
	hasFailed := false
	for _, server := range watchpkg.LSPDegradedServers(status) {
		language := server.Language
		if len(language) > 0 {
			language = strings.ToUpper(language[0:1]) + language[1:]
		}

		statusDesc := ""
		switch server.State {
		case "unavailable":
			statusDesc = "not found in PATH"
			hasUnavailable = true
		case "failed":
			hasFailed = true
			if server.RestartCount > 0 {
				statusDesc = "crashed"
			} else {
				statusDesc = "initialization failed"
			}
		case "memory_limited":
			statusDesc = "memory limit exceeded"
			hasFailed = true
		default:
			statusDesc = server.State
		}

		detail := fmt.Sprintf("%s (%s): %s", language, server.Command, statusDesc)
		if server.Command == "" {
			detail = fmt.Sprintf("%s: %s", language, statusDesc)
		}
		term.Hint(cmd.OutOrStdout(), detail)
		if server.LastError != "" {
			term.Hint(cmd.OutOrStdout(), "    Error: "+server.LastError)
		}
		if server.Language != "" {
			term.Hint(cmd.OutOrStdout(), fmt.Sprintf("    Override: tld config set watch.lsp.commands.%s %q", server.Language, suggestedLSPOverrideCommand(server.Command, server.Language)))
		}
	}

	if hasUnavailable {
		term.Hint(cmd.OutOrStdout(), "Remediation: install the missing language server(s) or ensure they are on your PATH.")
	}
	if hasFailed {
		term.Hint(cmd.OutOrStdout(), "Remediation: check your project configuration or try increasing memory limits.")
	}
	term.Hint(cmd.OutOrStdout(), "Alternatively, disable LSP with `tld config set watch.lsp.enabled false`.")
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

func suggestedLSPOverrideCommand(command, language string) string {
	name := language + "-language-server"
	fields := strings.Fields(command)
	if len(fields) > 0 {
		name = fields[0]
		if idx := strings.LastIndexAny(name, `/\`); idx >= 0 && idx+1 < len(name) {
			name = name[idx+1:]
		}
	}
	return "/path/to/" + strings.Trim(name, `"'`)
}

func isInteractiveInput(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func newAnalyzeWatchProgress(out io.Writer) (watchpkg.ProgressSink, *bytes.Buffer) {
	if !term.IsTerminal(out) {
		return nil, nil
	}
	var history bytes.Buffer
	return term.NewProgressLine(out, term.ProgressLineOptions{FinishedWriter: &history}), &history
}
