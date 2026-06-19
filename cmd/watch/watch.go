package watch

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/cmd/version"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	xterm "golang.org/x/term"
)

func NewWatchCmd() *cobra.Command {
	var host, port, dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel, embeddingRuntimePath string
	var embeddingDimension, embeddingMaxTokens int
	var languageFlags []string
	var watcherMode, pollInterval, debounce string
	var maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int
	var noServe, openBrowser, rescan, verbose, dryRun, failOnDrift bool
	c := &cobra.Command{
		Use:   "watch [path]",
		Short: "Scan and materialize source repositories into the local workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			commandStarted := time.Now()
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			if dryRun {
				return runWatchDiff(cmd, path, watchDiffOptions{
					DataDirFlag:          dataDirFlag,
					EmbeddingProvider:    embeddingProvider,
					EmbeddingEndpoint:    embeddingEndpoint,
					EmbeddingModel:       embeddingModel,
					EmbeddingDimension:   embeddingDimension,
					EmbeddingMaxTokens:   embeddingMaxTokens,
					EmbeddingRuntimePath: embeddingRuntimePath,
					LanguageFlags:        languageFlags,
					MaxElements:          maxElements,
					MaxConnectors:        maxConnectors,
					MaxIncoming:          maxIncoming,
					MaxOutgoing:          maxOutgoing,
					MaxExpandedGroup:     maxExpandedGroup,
					Rescan:               rescan,
					FailOnDrift:          failOnDrift,
					GroupDiffs:           true,
				})
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
			logFile, logger, err := openWatchLog(dataDir)
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
					logger.ErrorContext(cmd.Context(), "watch.command.failed", "elapsed", time.Since(commandStarted).Round(time.Millisecond).String(), "error", finalErr)
					return
				}
				logger.InfoContext(cmd.Context(), "watch.command.completed", "elapsed", time.Since(commandStarted).Round(time.Millisecond).String())
			}()
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension, embeddingMaxTokens, embeddingRuntimePath)
			watchSettings := resolveWatchSettings(cfg, languageFlags, watcherMode, pollInterval, debounce, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup)
			logger.InfoContext(cmd.Context(), "watch.command.started",
				"path", path,
				"data_dir", dataDir,
				"host", host,
				"port", port,
				"no_serve", noServe,
				"open_browser", openBrowser,
				"rescan", rescan,
				"verbose", verbose,
				"embedding_provider", embeddingCfg.Provider,
				"embedding_model", embeddingCfg.Model,
				"watcher", watchSettings.Watcher,
				"poll_interval", watchSettings.PollInterval.String(),
				"debounce", watchSettings.Debounce.String(),
				"languages", strings.Join(watchSettings.Languages, ","),
				"lsp_enabled", watchSettings.LSP.Enabled,
				"lsp_health_interval", watchSettings.LSP.HealthInterval.String(),
				"lsp_memory_limit_bytes", watchSettings.LSP.MemoryLimitBytes,
			)
			hasEmbedding := embeddingCfg.Provider != "" && embeddingCfg.Provider != "none" && embeddingCfg.Provider != "local-lexical"
			progress, progressHistory := newBufferedCLIProgress(cmd.ErrOrStderr())
			embeddingHealth := ""
			if hasEmbedding {
				if progress != nil {
					progress.Start("Checking embedding provider", 1)
				}
				healthStarted := time.Now()
				logger.InfoContext(cmd.Context(), "watch.embedding_healthcheck.started", "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				checked, health, err := watch.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					if progress != nil {
						if progress != nil {
							progress.Finish()
						}
					}
					return fail("watch.embedding_healthcheck.failed", fmt.Errorf("embedding healthcheck failed: %w", err), "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				}
				embeddingCfg = checked
				logger.InfoContext(cmd.Context(), "watch.embedding_healthcheck.completed", "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model, "dimension", health.Dimension, "similarity", health.Similarity)
				if progress != nil {
					progress.Advance("")
					progress.Finish()
				}
				embeddingHealth = fmt.Sprintf("dimension=%d similarity=%.3f", health.Dimension, health.Similarity)
			}
			serveCfg := workspace.ResolveServeOptions(cfg, host, port)
			serveOpts := localserver.ServeOptions{
				Host:           serveCfg.Host,
				Port:           serveCfg.Port,
				PublicURL:      serveCfg.PublicURL,
				AllowedOrigins: serveCfg.AllowedOrigins,
				Config:         cfg,
			}
			addr := localserver.ResolveAddr(serveOpts)
			serverURL := "http://" + addr
			browserURL := localserver.DisplayURL(serveOpts, addr)
			if err := localserver.RegisterProcess(localserver.ProcessRecord{
				Kind:    localserver.ProcessKindWatch,
				PID:     os.Getpid(),
				DataDir: dataDir,
				Addr:    addr,
			}); err != nil {
				return fail("watch.process_register.failed", err)
			}
			defer func() { _ = localserver.RemoveProcess(os.Getpid()) }()
			var srv *http.Server
			if !noServe {
				serveStarted := time.Now()
				logger.InfoContext(cmd.Context(), "watch.server.ensure.started", "url", serverURL)
				if !serverReady(serverURL) {
					if progress != nil {
						progress.Start("Starting watch server", 1)
					}
					logger.InfoContext(cmd.Context(), "watch.server.bootstrap.started", "data_dir", dataDir, "addr", addr)
					app, err := localserver.Bootstrap(dataDir, serveOpts)
					if err != nil {
						if progress != nil {
							progress.Finish()
						}
						return fail("watch.server.bootstrap.failed", err, "elapsed", time.Since(serveStarted).Round(time.Millisecond).String())
					}
					srv = &http.Server{Addr: app.Addr, Handler: app.Handler}
					go func() {
						if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							logger.ErrorContext(context.Background(), "watch.server.listen.failed", "error", err, "addr", app.Addr)
							term.Failf(cmd.ErrOrStderr(), "server error: %v", err)
						}
					}()
					serverURL = "http://" + app.Addr
					browserURL = localserver.DisplayURL(serveOpts, app.Addr)
					if progress != nil {
						progress.Advance("")
						progress.Finish()
					}
					logger.InfoContext(cmd.Context(), "watch.server.bootstrap.completed", "elapsed", time.Since(serveStarted).Round(time.Millisecond).String(), "url", serverURL)
				} else {
					logger.InfoContext(cmd.Context(), "watch.server.reused", "elapsed", time.Since(serveStarted).Round(time.Millisecond).String(), "url", serverURL)
				}
				if openBrowser {
					logger.InfoContext(cmd.Context(), "watch.browser.open.started", "url", browserURL)
					_ = cmdutil.OpenBrowser(browserURL)
				}
			} else {
				logger.InfoContext(cmd.Context(), "watch.server.skipped", "reason", "no_serve")
			}
			defer func() {
				if srv != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					logger.InfoContext(context.Background(), "watch.server.shutdown.started", "addr", srv.Addr)
					_ = srv.Shutdown(ctx)
					logger.InfoContext(context.Background(), "watch.server.shutdown.completed", "addr", srv.Addr)
				}
			}()

			storeStarted := time.Now()
			logger.InfoContext(cmd.Context(), "watch.store_open.started", "database", localserver.DatabasePath(dataDir))
			if progress != nil {
				progress.Start("Opening workspace database", 1)
			}
			sqliteStore, err := store.OpenLocal(cmd.Context(), cfg, dataDir, assets.FS)
			if err != nil {
				if progress != nil {
					progress.Finish()
				}
				return fail("watch.store_open.failed", err, "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			}
			defer func() { _ = sqliteStore.Close() }()
			if progress != nil {
				progress.Advance("")
				progress.Finish()
			}
			logger.InfoContext(cmd.Context(), "watch.store_open.completed", "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			watchStore := watch.NewStoreWithBun(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			events := watch.NewEventQueue()
			ready := make(chan watch.RunnerResult, 1)
			watchProgress := newWatchActivityProgress(cmd.ErrOrStderr(), watchClientCounter(serverURL))
			defer func() {
				if watchProgress != nil {
					watchProgress.Stop()
				}
			}()
			go func() {
				for event := range events.Out() {
					logWatchRuntimeEvent(cmd.Context(), logger, event)
					watch.BroadcastWatchEvent(event)
					if logWatchEvent(cmd, event, watchProgress) {
						continue
					}
					if verbose || event.Type == "watch.error" || event.Type == "version.created" {
						if event.Message != "" {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", event.Type, event.Message)
						} else {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", event.Type)
						}
					}
				}
			}()
			errCh := make(chan error, 1)
			go func() {
				_, runErr := watch.NewRunner(watchStore).Run(ctx, watch.RunnerOptions{Path: path, Rescan: rescan, Verbose: verbose, Embedding: embeddingCfg, Settings: watchSettings, DataDir: dataDir, Progress: progress, Logger: logger, Events: events, Ready: ready, ConfirmAfterScan: confirmWatchLSPProceed(cmd)})
				errCh <- runErr
				events.Close()
			}()
			var result watch.RunnerResult
			select {
			case result = <-ready:
			case err := <-errCh:
				if err != nil {
					return fail("watch.runner.failed", err)
				}
				return nil
			}
			repo := result.Repository
			_ = localserver.RegisterProcess(localserver.ProcessRecord{
				Kind:         localserver.ProcessKindWatch,
				PID:          os.Getpid(),
				DataDir:      dataDir,
				Addr:         addr,
				RepoRoot:     repo.RepoRoot,
				RepositoryID: repo.ID,
			})
			renderWatchReady(cmd.OutOrStdout(), cmd.ErrOrStderr(), watchReadyView{
				Version:           version.Version,
				DataDir:           dataDir,
				RepoRoot:          repo.RepoRoot,
				Repository:        repoIdentity(repo),
				Branch:            result.GitStatus.Branch,
				Head:              result.GitStatus.HeadCommit,
				ActiveLSPs:        formatWatchLSPServers(result.InitialScan.LSP),
				URL:               browserURL,
				EmbeddingProvider: embeddingCfg.Provider,
				EmbeddingModel:    embeddingCfg.Model,
				EmbeddingHealth:   embeddingHealth,
				StartupHistory:    progressHistory,
				Verbose:           verbose,
			})
			if watchProgress != nil {
				watchProgress.Start("Workspace status: current")
			}
			logger.InfoContext(cmd.Context(), "watch.command.ready", "repository_id", repo.ID, "repo_root", repo.RepoRoot, "url", browserURL)
			if err := <-errCh; err != nil {
				return fail("watch.runner.failed", err)
			}
			return nil
		},
	}
	c.Flags().StringVar(&host, "host", "", "host for the local app server")
	c.Flags().StringVar(&port, "port", "", "port for the local app server")
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().BoolVar(&noServe, "no-serve", false, "do not start the local app server")
	c.Flags().BoolVar(&openBrowser, "open", false, "open the webapp in a browser")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "scan, materialize, print frontend-equivalent watch diffs as JSON, and exit")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for representation")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for representation")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for representation")
	c.Flags().IntVar(&embeddingDimension, "embedding-dimension", 0, "embedding vector dimension")
	c.Flags().IntVar(&embeddingMaxTokens, "embedding-max-tokens", 0, "maximum input token length for embedding model")
	c.Flags().StringSliceVar(&languageFlags, "language", nil, "source language to watch (repeatable)")
	c.Flags().StringVar(&watcherMode, "watcher", "", "watcher backend: auto, fsnotify, or poll")
	c.Flags().StringVar(&pollInterval, "poll-interval", "", "poll interval (for example 1s)")
	c.Flags().StringVar(&debounce, "debounce", "", "change debounce duration (for example 500ms)")
	c.Flags().IntVar(&maxElements, "max-elements-per-view", 0, "maximum generated elements per view")
	c.Flags().IntVar(&maxConnectors, "max-connectors-per-view", 0, "maximum generated connectors per view")
	c.Flags().IntVar(&maxIncoming, "max-incoming-per-element", 0, "maximum incoming references per element before collapsing")
	c.Flags().IntVar(&maxOutgoing, "max-outgoing-per-element", 0, "maximum outgoing references per element before collapsing")
	c.Flags().IntVar(&maxExpandedGroup, "max-expanded-connectors-per-group", 0, "maximum file-pair connectors to expand before collapsing to a folder connector")
	c.Flags().BoolVar(&rescan, "rescan", false, "force a rescan before watching")
	c.Flags().BoolVar(&failOnDrift, "fail-on-drift", false, "with --dry-run, exit nonzero when representation drift is detected")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "print watch events and startup phase history")
	c.AddCommand(newScanCmd())
	c.AddCommand(newRepresentCmd())
	c.AddCommand(newDiffCmd())
	return c
}

func logWatchEvent(cmd *cobra.Command, event watch.Event, activity *watchActivityProgress) bool {
	out := cmd.OutOrStdout()
	switch event.Type {
	case "watch.started":
		return true
	case "watch.stopped":
		if activity != nil {
			if elapsed, ok := activity.Stop(); ok {
				_, _ = fmt.Fprintf(out, "watch stopped after %s\n", formatWatchActivityDuration(elapsed))
				return true
			}
		}
		_, _ = fmt.Fprintf(out, "watch stopped\n")
		return true
	case "scan.started":
		return true
	case "scan.completed":
		return true
	case "representation.started":
		return true
	case "representation.updated":
		return true
	case "source.changed":
		result, ok := event.Data.(watch.SourceFileChangeResult)
		if !ok {
			return false
		}
		if activity != nil {
			activity.Advance(workspaceStatusSummary(result, event.ChangedFiles))
		}
		return true
	case "watch.changeCounter":
		return true
	case "lsp.status":
		return true
	case "watch.error":
		message := event.Message
		if message == "" {
			message = "unknown error"
		}
		_, _ = fmt.Fprintf(out, "%s %s\n", term.Colorize(out, term.ColorRed, "watch.error:"), message)
		return true
	case "version.created":
		_, _ = fmt.Fprintf(out, "version created\n")
		return true
	default:
		return false
	}
}

func formatWatchLSPStatus(status watch.LSPStatus) string {
	if !status.Enabled {
		return "disabled"
	}
	return fmt.Sprintf("requested=%d available=%d active=%d degraded=%d restarted=%d",
		status.Summary.Requested,
		status.Summary.Available,
		status.Summary.Active,
		status.Summary.Unavailable+status.Summary.Failed,
		status.Summary.Restarted,
	)
}

func formatWatchLSPServers(status watch.LSPStatus) string {
	if !status.Enabled {
		return "disabled"
	}
	var active []string
	for _, server := range status.Servers {
		if server.State == "active" {
			active = append(active, server.Language)
		}
	}
	if len(active) == 0 {
		return "none"
	}
	sort.Strings(active)
	return strings.Join(active, ",")
}

func confirmWatchLSPProceed(cmd *cobra.Command) func(context.Context, watch.ScanResult) error {
	return func(_ context.Context, scan watch.ScanResult) error {
		return confirmLSPProceed(cmd, scan.LSP)
	}
}

func confirmLSPProceed(cmd *cobra.Command, status watch.LSPStatus) error {
	if !watch.LSPNeedsConfirmation(status) {
		return nil
	}
	term.Warn(cmd.OutOrStdout(), "Some requested language servers are unavailable or unhealthy. Reference resolution quality will be lower.")
	term.Hint(cmd.OutOrStdout(), "tld will fall back to conservative name matching, which may drop ambiguous connectors.")
	hasUnavailable := false
	hasFailed := false
	for _, server := range watch.LSPDegradedServers(status) {
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

func workspaceStatusSummary(result watch.SourceFileChangeResult, changedFiles int) string {
	state := "current"
	if !result.RepresentationChanged {
		state = "current; no representation update"
	}
	fileLabel := ""
	if changedFiles > 1 {
		fileLabel = fmt.Sprintf("; %d files changed", changedFiles)
	}
	return fmt.Sprintf("Workspace status: %s%s (%s)", state, fileLabel, representationChangeSummary(result.Representation, result.GitTags))
}

func representationChangeSummary(rep watch.RepresentResult, tags watch.GitTagUpdateResult) string {
	out := new(strings.Builder)
	plus := "\033[32m+\033[0m"
	tilde := "\033[33m~\033[0m"
	minus := "\033[31m-\033[0m"
	fmt.Fprintf(out, "elements %s%d %s%d, connectors %s%d %s%d, views %s%d, tags %s%d%s%d",
		plus, rep.ElementsCreated,
		tilde, rep.ElementsUpdated,
		plus, rep.ConnectorsCreated,
		tilde, rep.ConnectorsUpdated,
		plus, rep.ViewsCreated,
		plus, tags.TagsAdded,
		minus, tags.TagsRemoved)
	if rep.DeletesPreserved > 0 {
		fmt.Fprintf(out, ", cleaned %s%d", minus, rep.DeletesPreserved)
	}
	return out.String()
}

func openWatchLog(dataDir string) (*os.File, *slog.Logger, error) {
	path := localserver.LogPath(dataDir)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open watch log file: %w", err)
	}
	return file, slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})), nil
}

func logWatchRuntimeEvent(ctx context.Context, logger *slog.Logger, event watch.Event) {
	if logger == nil {
		return
	}
	fields := []any{
		"type", event.Type,
		"repository_id", event.RepositoryID,
		"phase", event.Phase,
		"watcher_mode", event.WatcherMode,
		"changed_files", event.ChangedFiles,
		"warnings", len(event.Warnings),
	}
	if event.Message != "" {
		fields = append(fields, "message", event.Message)
	}
	if event.Type == "watch.heartbeat" || event.Type == "watch.changeCounter" || event.Type == "watch.change.skipped" {
		logger.DebugContext(ctx, "watch.event", fields...)
	} else {
		logger.InfoContext(ctx, "watch.event", fields...)
	}
}

func repoIdentity(repo watch.Repository) string {
	if repo.RemoteURL.Valid && repo.RemoteURL.String != "" {
		return repo.RemoteURL.String
	}
	return repo.RepoRoot
}

func serverReady(url string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(url + "/api/ready")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func watchClientCounter(url string) func() int {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var mu sync.Mutex
	var cached int
	var checkedAt time.Time
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(checkedAt) < time.Second {
			return cached
		}
		checkedAt = time.Now()
		resp, err := client.Get(url + "/api/watch/status")
		if err != nil {
			cached = watch.WatchWebSocketClientCount()
			return cached
		}
		defer func() { _ = resp.Body.Close() }()
		var status struct {
			ConnectedClients int `json:"connected_clients"`
		}
		if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&status) != nil {
			cached = watch.WatchWebSocketClientCount()
			return cached
		}
		cached = status.ConnectedClients
		return cached
	}
}

func newScanCmd() *cobra.Command {
	var dataDirFlag string
	var languageFlags []string
	var jsonOut, rescan bool
	c := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan a repository into the local raw code graph",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
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
			watchSettings := resolveWatchSettings(cfg, languageFlags, "", "", "", 0, 0, 0, 0, 0)
			sqliteStore, err := store.OpenLocal(cmd.Context(), cfg, dataDir, assets.FS)
			if err != nil {
				return err
			}
			defer func() { _ = sqliteStore.Close() }()
			scanner := watch.NewScanner(watch.NewStoreWithBun(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect()))
			scanner.Settings = watchSettings
			if !jsonOut {
				scanner.Progress = newCLIProgress(cmd.ErrOrStderr())
			}
			result, err := scanner.ScanWithOptions(cmd.Context(), path, watch.ScanOptions{Force: rescan, DataDir: dataDir})
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			term.Label(cmd.OutOrStdout(), 15, "Repository", fmt.Sprintf("%d", result.RepositoryID))
			term.Label(cmd.OutOrStdout(), 15, "Scan run", fmt.Sprintf("%d", result.ScanRunID))
			term.Label(cmd.OutOrStdout(), 15, "Files", fmt.Sprintf("%d seen, %d parsed, %d skipped", result.FilesSeen, result.FilesParsed, result.FilesSkipped))
			term.Label(cmd.OutOrStdout(), 15, "Symbols", fmt.Sprintf("%d", result.SymbolsSeen))
			term.Label(cmd.OutOrStdout(), 15, "References", fmt.Sprintf("%d", result.ReferencesSeen))
			term.Label(cmd.OutOrStdout(), 15, "LSP", formatWatchLSPStatus(result.LSP))
			if result.Warning != "" {
				term.Warn(cmd.OutOrStdout(), result.Warning)
			}
			return nil
		},
	}
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().StringSliceVar(&languageFlags, "language", nil, "source language to scan (repeatable)")
	c.Flags().BoolVar(&rescan, "rescan", false, "force reparsing files even if cached")
	c.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return c
}

func newRepresentCmd() *cobra.Command {
	var dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel, embeddingRuntimePath string
	var embeddingDimension, embeddingMaxTokens int
	var languageFlags []string
	var jsonOut, rescan bool
	var maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int
	c := &cobra.Command{
		Use:   "represent [path]",
		Short: "Materialize a scanned repository into the local workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
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
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension, embeddingMaxTokens, embeddingRuntimePath)
			watchSettings := resolveWatchSettings(cfg, languageFlags, "", "", "", maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup)
			var progress watch.ProgressSink
			if !jsonOut {
				progress = newCLIProgress(cmd.ErrOrStderr())
			}
			if embeddingCfg.Provider != "none" {
				checked, health, err := watch.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					return fmt.Errorf("embedding healthcheck failed: %w", err)
				}
				embeddingCfg = checked
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Embedding:       %s/%s dimension=%d similarity=%.3f\n", embeddingCfg.Provider, embeddingCfg.Model, health.Dimension, health.Similarity)
			}
			sqliteStore, err := store.OpenLocal(cmd.Context(), cfg, dataDir, assets.FS)
			if err != nil {
				return err
			}
			defer func() { _ = sqliteStore.Close() }()
			watchStore := watch.NewStoreWithBun(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
			scanner := watch.NewScanner(watchStore)
			scanner.Settings = watchSettings
			scanner.Progress = progress
			scanResult, err := scanner.ScanWithOptions(cmd.Context(), path, watch.ScanOptions{Force: rescan, DataDir: dataDir})
			if err != nil {
				return err
			}
			result, err := watch.NewRepresenter(watchStore).Represent(cmd.Context(), scanResult.RepositoryID, watch.RepresentRequest{
				Embedding:    embeddingCfg,
				Thresholds:   watchSettings.Thresholds,
				Visibility:   watchSettings.Visibility,
				Dependencies: watchSettings.Dependencies,
				Progress:     progress,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Scan           watch.ScanResult      `json:"scan"`
					Representation watch.RepresentResult `json:"representation"`
				}{Scan: scanResult, Representation: result})
			}
			term.Label(cmd.OutOrStdout(), 18, "Repository", fmt.Sprintf("%d", result.RepositoryID))
			term.Label(cmd.OutOrStdout(), 18, "Scan run", fmt.Sprintf("%d", scanResult.ScanRunID))
			term.Label(cmd.OutOrStdout(), 18, "Filter run", fmt.Sprintf("%d", result.FilterRunID))
			term.Label(cmd.OutOrStdout(), 18, "Represent run", fmt.Sprintf("%d", result.RepresentationRun))
			term.Label(cmd.OutOrStdout(), 18, "Elements", fmt.Sprintf("%d created, %d updated", result.ElementsCreated, result.ElementsUpdated))
			term.Label(cmd.OutOrStdout(), 18, "Connectors", fmt.Sprintf("%d created, %d updated", result.ConnectorsCreated, result.ConnectorsUpdated))
			term.Label(cmd.OutOrStdout(), 18, "Views", fmt.Sprintf("%d created", result.ViewsCreated))
			term.Label(cmd.OutOrStdout(), 18, "LSP", formatWatchLSPStatus(scanResult.LSP))
			term.Label(cmd.OutOrStdout(), 18, "Raw graph hash", result.RawGraphHash)
			term.Label(cmd.OutOrStdout(), 18, "Representation", result.RepresentationHash)
			return nil
		},
	}
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for representation")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for representation")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for representation")
	c.Flags().IntVar(&embeddingDimension, "embedding-dimension", 0, "embedding vector dimension")
	c.Flags().IntVar(&embeddingMaxTokens, "embedding-max-tokens", 0, "maximum input token length for embedding model")
	c.Flags().StringVar(&embeddingRuntimePath, "embedding-runtime-path", "", "ONNX Runtime shared library path for local embedding providers")
	c.Flags().StringSliceVar(&languageFlags, "language", nil, "source language to scan (repeatable)")
	c.Flags().IntVar(&maxElements, "max-elements-per-view", 0, "maximum generated elements per view")
	c.Flags().IntVar(&maxConnectors, "max-connectors-per-view", 0, "maximum generated connectors per view")
	c.Flags().IntVar(&maxIncoming, "max-incoming-per-element", 0, "maximum incoming references per element before collapsing")
	c.Flags().IntVar(&maxOutgoing, "max-outgoing-per-element", 0, "maximum outgoing references per element before collapsing")
	c.Flags().IntVar(&maxExpandedGroup, "max-expanded-connectors-per-group", 0, "maximum file-pair connectors to expand before collapsing to a folder connector")
	c.Flags().BoolVar(&rescan, "rescan", false, "force reparsing files even if cached")
	c.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return c
}

func newDiffCmd() *cobra.Command {
	var dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel, embeddingRuntimePath string
	var embeddingDimension, embeddingMaxTokens int
	var languageFlags []string
	var failOnDrift bool
	var maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int
	c := &cobra.Command{
		Use:   "diff [path]",
		Short: "Scan and report watch representation drift as JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runWatchDiff(cmd, path, watchDiffOptions{
				DataDirFlag:          dataDirFlag,
				EmbeddingProvider:    embeddingProvider,
				EmbeddingEndpoint:    embeddingEndpoint,
				EmbeddingModel:       embeddingModel,
				EmbeddingDimension:   embeddingDimension,
				EmbeddingMaxTokens:   embeddingMaxTokens,
				EmbeddingRuntimePath: embeddingRuntimePath,
				LanguageFlags:        languageFlags,
				MaxElements:          maxElements,
				MaxConnectors:        maxConnectors,
				MaxIncoming:          maxIncoming,
				MaxOutgoing:          maxOutgoing,
				MaxExpandedGroup:     maxExpandedGroup,
				FailOnDrift:          failOnDrift,
			})
		},
	}
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for representation")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for representation")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for representation")
	c.Flags().IntVar(&embeddingDimension, "embedding-dimension", 0, "embedding vector dimension")
	c.Flags().IntVar(&embeddingMaxTokens, "embedding-max-tokens", 0, "maximum input token length for embedding model")
	c.Flags().StringVar(&embeddingRuntimePath, "embedding-runtime-path", "", "ONNX Runtime shared library path for local embedding providers")
	c.Flags().StringSliceVar(&languageFlags, "language", nil, "source language to scan (repeatable)")
	c.Flags().IntVar(&maxElements, "max-elements-per-view", 0, "maximum generated elements per view")
	c.Flags().IntVar(&maxConnectors, "max-connectors-per-view", 0, "maximum generated connectors per view")
	c.Flags().IntVar(&maxIncoming, "max-incoming-per-element", 0, "maximum incoming references per element before collapsing")
	c.Flags().IntVar(&maxOutgoing, "max-outgoing-per-element", 0, "maximum outgoing references per element before collapsing")
	c.Flags().IntVar(&maxExpandedGroup, "max-expanded-connectors-per-group", 0, "maximum file-pair connectors to expand before collapsing to a folder connector")
	c.Flags().BoolVar(&failOnDrift, "fail-on-drift", false, "exit nonzero when representation drift is detected")
	return c
}

type watchDiffOptions struct {
	DataDirFlag          string
	EmbeddingProvider    string
	EmbeddingEndpoint    string
	EmbeddingModel       string
	EmbeddingDimension   int
	EmbeddingMaxTokens   int
	EmbeddingRuntimePath string
	LanguageFlags        []string
	MaxElements          int
	MaxConnectors        int
	MaxIncoming          int
	MaxOutgoing          int
	MaxExpandedGroup     int
	Rescan               bool
	FailOnDrift          bool
	GroupDiffs           bool
}

type watchDiffPayload struct {
	Changed        bool                       `json:"changed"`
	Scan           watch.ScanResult           `json:"scan"`
	Representation watch.RepresentResult      `json:"representation"`
	Diffs          []watch.RepresentationDiff `json:"diffs"`
}

type watchGroupedDiffPayload struct {
	Changed        bool                                             `json:"changed"`
	Scan           watch.ScanResult                                 `json:"scan"`
	Representation watch.RepresentResult                            `json:"representation"`
	Diffs          map[string]map[string][]watch.RepresentationDiff `json:"diffs"`
}

func runWatchDiff(cmd *cobra.Command, path string, opts watchDiffOptions) error {
	started := time.Now()
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return err
	}
	dataDir, err := workspace.ResolveDataDir(cfg, opts.DataDirFlag)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	logFile, logger, err := openWatchLog(dataDir)
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
			logger.ErrorContext(cmd.Context(), "watch.diff.failed", "elapsed", time.Since(started).Round(time.Millisecond).String(), "error", finalErr)
			return
		}
		logger.InfoContext(cmd.Context(), "watch.diff.completed", "elapsed", time.Since(started).Round(time.Millisecond).String())
	}()
	embeddingCfg := resolveEmbeddingConfig(cfg, opts.EmbeddingProvider, opts.EmbeddingEndpoint, opts.EmbeddingModel, opts.EmbeddingDimension, opts.EmbeddingMaxTokens, opts.EmbeddingRuntimePath)
	watchSettings := resolveWatchSettings(cfg, opts.LanguageFlags, "", "", "", opts.MaxElements, opts.MaxConnectors, opts.MaxIncoming, opts.MaxOutgoing, opts.MaxExpandedGroup)
	logger.InfoContext(cmd.Context(), "watch.diff.started", "path", path, "data_dir", dataDir, "rescan", opts.Rescan, "fail_on_drift", opts.FailOnDrift, "group_diffs", opts.GroupDiffs, "embedding_provider", embeddingCfg.Provider, "embedding_model", embeddingCfg.Model, "languages", strings.Join(watchSettings.Languages, ","))
	sqliteStore, err := store.OpenLocal(cmd.Context(), cfg, dataDir, assets.FS)
	if err != nil {
		return fail("watch.diff.store_open.failed", err)
	}
	defer func() { _ = sqliteStore.Close() }()
	watchStore := watch.NewStoreWithBun(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	once, err := watch.NewRunner(watchStore).RunOnce(cmd.Context(), watch.OneShotOptions{Path: path, Rescan: opts.Rescan, Embedding: embeddingCfg, Settings: watchSettings, DataDir: dataDir, Logger: logger})
	if err != nil {
		return fail("watch.diff.pipeline.failed", err)
	}
	latest, found, err := watchStore.LatestWatchVersion(cmd.Context(), once.Scan.RepositoryID)
	if err != nil {
		return fail("watch.diff.latest_version.failed", err, "repository_id", once.Scan.RepositoryID)
	}
	changed := found && latest.RepresentationHash != once.Representation.RepresentationHash || hasWatchDriftDiffs(once.Diffs)
	logger.InfoContext(cmd.Context(), "watch.diff.payload", "repository_id", once.Scan.RepositoryID, "changed", changed, "diffs", len(once.Diffs), "latest_found", found)
	var payload any = watchDiffPayload{Changed: changed, Scan: once.Scan, Representation: once.Representation, Diffs: once.Diffs}
	if opts.GroupDiffs {
		payload = watchGroupedDiffPayload{Changed: changed, Scan: once.Scan, Representation: once.Representation, Diffs: groupWatchDiffs(once.Diffs)}
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); err != nil {
		return fail("watch.diff.output.failed", err)
	}
	if opts.FailOnDrift && changed {
		return fail("watch.diff.drift.failed", fmt.Errorf("watch representation drift detected"))
	}
	return nil
}

func groupWatchDiffs(diffs []watch.RepresentationDiff) map[string]map[string][]watch.RepresentationDiff {
	grouped := map[string]map[string][]watch.RepresentationDiff{}
	for _, diff := range diffs {
		changeType := strings.TrimSpace(diff.ChangeType)
		if changeType == "" {
			changeType = "updated"
		}
		resourceType := diffResourceType(diff)
		if _, ok := grouped[changeType]; !ok {
			grouped[changeType] = map[string][]watch.RepresentationDiff{}
		}
		grouped[changeType][resourceType] = append(grouped[changeType][resourceType], diff)
	}
	return grouped
}

func diffResourceType(diff watch.RepresentationDiff) string {
	if diff.ResourceType != nil && strings.TrimSpace(*diff.ResourceType) != "" {
		return strings.TrimSpace(*diff.ResourceType)
	}
	if strings.TrimSpace(diff.OwnerType) != "" {
		return strings.TrimSpace(diff.OwnerType)
	}
	return "unknown"
}

func hasWatchDriftDiffs(diffs []watch.RepresentationDiff) bool {
	for _, diff := range diffs {
		if diff.ChangeType != "initialized" && diff.OwnerType != "repository" {
			return true
		}
	}
	return false
}

func resolveEmbeddingConfig(cfg *workspace.Config, provider, endpoint, model string, dimension int, maxTokens int, runtimePath ...string) watch.EmbeddingConfig {
	return watch.ResolveEmbeddingConfig(cfg, provider, endpoint, model, dimension, maxTokens, runtimePath...)
}

func resolveWatchSettings(cfg *workspace.Config, languages []string, watcherMode, pollInterval, debounce string, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int) watch.Settings {
	return watch.ResolveSettings(cfg, languages, watcherMode, pollInterval, debounce, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup)
}

type watchReadyView struct {
	Version           string
	DataDir           string
	RepoRoot          string
	Repository        string
	Branch            string
	Head              string
	ActiveLSPs        string
	URL               string
	EmbeddingProvider string
	EmbeddingModel    string
	EmbeddingHealth   string
	StartupHistory    *bytes.Buffer
	Verbose           bool
}

type watchReadyRow struct {
	Label string
	Value string
}

func renderWatchReady(out, progressOut io.Writer, view watchReadyView) {
	if term.IsTerminal(progressOut) {
		_, _ = fmt.Fprint(progressOut, "\r\033[K")
	}
	_, _ = fmt.Fprintf(out, "%s %s\n", term.Colorize(out, term.ColorCyan+term.ColorBold, "tld"), view.Version)
	renderWatchSection(out, "Workspace", []watchReadyRow{
		{Label: "Data directory", Value: term.Path(out, view.DataDir)},
		{Label: "Watching", Value: view.RepoRoot},
		{Label: "Repository", Value: view.Repository},
	})
	renderWatchSection(out, "Runtime", watchRuntimeRows(out, view))
	if view.Verbose {
		renderWatchStartup(out, view.StartupHistory)
	}
	term.Hint(out, "Press Ctrl-C to stop watching.")
}

func watchRuntimeRows(out io.Writer, view watchReadyView) []watchReadyRow {
	rows := []watchReadyRow{
		{Label: "Branch", Value: view.Branch},
		{Label: "HEAD", Value: view.Head},
		{Label: "Active LSPs", Value: view.ActiveLSPs},
		{Label: "tlDiagram", Value: term.URL(out, view.URL)},
	}
	if view.EmbeddingProvider != "" && view.EmbeddingProvider != "none" && view.EmbeddingProvider != "local-lexical" {
		rows = append(rows,
			watchReadyRow{Label: "Embedding", Value: strings.TrimSpace(view.EmbeddingProvider + " " + view.EmbeddingModel)},
			watchReadyRow{Label: "Embedding health", Value: view.EmbeddingHealth},
		)
	}
	return rows
}

func renderWatchSection(out io.Writer, title string, rows []watchReadyRow) {
	if len(rows) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", term.Colorize(out, term.ColorBold, title))
	renderWatchRows(out, rows)
}

func renderWatchRows(out io.Writer, rows []watchReadyRow) {
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

func renderWatchStartup(out io.Writer, history *bytes.Buffer) {
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
	_, _ = fmt.Fprintln(out)
}

type watchActivityProgress struct {
	mu            sync.Mutex
	out           io.Writer
	label         string
	clientCount   func() int
	now           func() time.Time
	width         int
	interval      time.Duration
	disableTicker bool
	started       time.Time
	rendered      bool
	stopped       bool
	frame         int
	ticker        *time.Ticker
	done          chan struct{}
}

type watchActivityOptions struct {
	ForceTerminal bool
	DisableTicker bool
	Interval      time.Duration
	Width         int
	Now           func() time.Time
}

func newCLIProgress(out io.Writer) watch.ProgressSink {
	return term.NewProgressLine(out, term.ProgressLineOptions{})
}

func newBufferedCLIProgress(out io.Writer) (watch.ProgressSink, *bytes.Buffer) {
	if !term.IsTerminal(out) {
		return nil, nil
	}
	var history bytes.Buffer
	return term.NewProgressLine(out, term.ProgressLineOptions{FinishedWriter: &history}), &history
}

func newWatchActivityProgress(out io.Writer, clientCount func() int) *watchActivityProgress {
	return newWatchActivityProgressWithOptions(out, clientCount, watchActivityOptions{})
}

func newWatchActivityProgressWithOptions(out io.Writer, clientCount func() int, opts watchActivityOptions) *watchActivityProgress {
	if out == nil || (!opts.ForceTerminal && !term.IsTerminal(out)) {
		return nil
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 900 * time.Millisecond
	}
	return &watchActivityProgress{
		out:           out,
		clientCount:   clientCount,
		now:           now,
		width:         watchActivityWidth(out, opts.Width),
		interval:      interval,
		disableTicker: opts.DisableTicker,
	}
}

func (p *watchActivityProgress) Start(label string) {
	if p == nil {
		return
	}
	if label == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started.IsZero() {
		p.started = p.now()
		p.stopped = false
		if !p.disableTicker {
			p.done = make(chan struct{})
			p.ticker = time.NewTicker(p.interval)
			go p.run(p.ticker, p.done)
		}
	}
	p.label = label
	p.renderLocked()
}

func (p *watchActivityProgress) run(ticker *time.Ticker, done <-chan struct{}) {
	for {
		select {
		case <-ticker.C:
			p.tick()
		case <-done:
			return
		}
	}
}

func (p *watchActivityProgress) tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.started.IsZero() {
		return
	}
	p.frame++
	p.renderLocked()
}

func (p *watchActivityProgress) statusLabel() string {
	clientLabel := ""
	if p.clientCount != nil {
		clients := p.clientCount()
		plural := "s"
		if clients == 1 {
			plural = ""
		}
		clientLabel = fmt.Sprintf(" | %d client%s connected", clients, plural)
	}
	return p.label + clientLabel
}

func (p *watchActivityProgress) Advance(label string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if label != "" {
		p.label = label
	}
	p.renderLocked()
}

func (p *watchActivityProgress) Stop() (time.Duration, bool) {
	if p == nil {
		return 0, false
	}
	p.mu.Lock()
	if p.stopped || p.started.IsZero() {
		p.mu.Unlock()
		return 0, false
	}
	now := p.now()
	elapsed := now.Sub(p.started).Round(time.Second)
	if elapsed < 0 {
		elapsed = 0
	}
	p.stopped = true
	rendered := p.rendered
	ticker := p.ticker
	done := p.done
	p.ticker = nil
	p.done = nil
	p.mu.Unlock()

	if ticker != nil {
		ticker.Stop()
	}
	if done != nil {
		close(done)
	}
	if rendered {
		_, _ = fmt.Fprint(p.out, "\r\033[K")
	}
	return elapsed, true
}

func (p *watchActivityProgress) renderLocked() {
	if p.out == nil || p.stopped || p.started.IsZero() {
		return
	}
	line := fmt.Sprintf("%s %s", watchSpinnerFrames[p.frame%len(watchSpinnerFrames)], stripANSI(p.statusLabel()))
	_, _ = fmt.Fprintf(p.out, "\r\033[K%s", truncateWatchActivityLine(line, p.width))
	p.rendered = true
}

var watchSpinnerFrames = []string{"-", "\\", "|", "/"}

func watchActivityWidth(out io.Writer, configured int) int {
	if configured > 0 {
		return configured
	}
	if f, ok := out.(*os.File); ok {
		width, _, err := xterm.GetSize(int(f.Fd()))
		if err == nil && width > 0 {
			return width
		}
	}
	if value := strings.TrimSpace(os.Getenv("COLUMNS")); value != "" {
		if width, err := strconv.Atoi(value); err == nil && width > 0 {
			return width
		}
	}
	return 80
}

func truncateWatchActivityLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func stripANSI(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == 0x1b && i+1 < len(value) && value[i+1] == '[' {
			i += 2
			for i < len(value) && (value[i] < '@' || value[i] > '~') {
				i++
			}
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

func formatWatchActivityDuration(d time.Duration) string {
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm%02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
