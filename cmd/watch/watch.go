package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

func NewWatchCmd() *cobra.Command {
	var host, port, dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel string
	var embeddingDimension int
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
					DataDirFlag:        dataDirFlag,
					EmbeddingProvider:  embeddingProvider,
					EmbeddingEndpoint:  embeddingEndpoint,
					EmbeddingModel:     embeddingModel,
					EmbeddingDimension: embeddingDimension,
					LanguageFlags:      languageFlags,
					MaxElements:        maxElements,
					MaxConnectors:      maxConnectors,
					MaxIncoming:        maxIncoming,
					MaxOutgoing:        maxOutgoing,
					MaxExpandedGroup:   maxExpandedGroup,
					Rescan:             rescan,
					FailOnDrift:        failOnDrift,
					GroupDiffs:         true,
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
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
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
			)
			term.PrintLogo(cmd.OutOrStdout(), version.Version)
			term.Label(cmd.OutOrStdout(), 20, "Mode", "watch")
			term.Label(cmd.OutOrStdout(), 20, "Data directory", term.Path(cmd.OutOrStdout(), dataDir))
			hasEmbedding := embeddingCfg.Provider != "" && embeddingCfg.Provider != "none" && embeddingCfg.Provider != "local-lexical"
			if hasEmbedding {
				term.Label(cmd.OutOrStdout(), 20, "Embedding provider", embeddingCfg.Provider)
				term.Label(cmd.OutOrStdout(), 20, "Embedding model", embeddingCfg.Model)
			}
			progress := newCLIProgress(cmd.ErrOrStderr())
			if hasEmbedding {
				healthStarted := time.Now()
				logger.InfoContext(cmd.Context(), "watch.embedding_healthcheck.started", "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				checked, health, err := watch.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					return fail("watch.embedding_healthcheck.failed", fmt.Errorf("embedding healthcheck failed: %w", err), "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model)
				}
				embeddingCfg = checked
				logger.InfoContext(cmd.Context(), "watch.embedding_healthcheck.completed", "elapsed", time.Since(healthStarted).Round(time.Millisecond).String(), "provider", embeddingCfg.Provider, "model", embeddingCfg.Model, "dimension", health.Dimension, "similarity", health.Similarity)
				term.Label(cmd.OutOrStdout(), 20, "Embedding health", fmt.Sprintf("dimension=%d similarity=%.3f", health.Dimension, health.Similarity))
			}
			serveCfg := workspace.ResolveServeOptions(cfg, host, port)
			serveOpts := localserver.ServeOptions{Host: serveCfg.Host, Port: serveCfg.Port}
			addr := localserver.ResolveAddr(serveOpts)
			url := "http://" + addr
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
				logger.InfoContext(cmd.Context(), "watch.server.ensure.started", "url", url)
				if !serverReady(url) {
					logger.InfoContext(cmd.Context(), "watch.server.bootstrap.started", "data_dir", dataDir, "addr", addr)
					app, err := localserver.Bootstrap(dataDir, serveOpts)
					if err != nil {
						return fail("watch.server.bootstrap.failed", err, "elapsed", time.Since(serveStarted).Round(time.Millisecond).String())
					}
					srv = &http.Server{Addr: app.Addr, Handler: app.Handler}
					go func() {
						if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							logger.ErrorContext(context.Background(), "watch.server.listen.failed", "error", err, "addr", app.Addr)
							term.Failf(cmd.ErrOrStderr(), "server error: %v", err)
						}
					}()
					url = "http://" + app.Addr
					logger.InfoContext(cmd.Context(), "watch.server.bootstrap.completed", "elapsed", time.Since(serveStarted).Round(time.Millisecond).String(), "url", url)
				} else {
					logger.InfoContext(cmd.Context(), "watch.server.reused", "elapsed", time.Since(serveStarted).Round(time.Millisecond).String(), "url", url)
				}
				if openBrowser {
					logger.InfoContext(cmd.Context(), "watch.browser.open.started", "url", url)
					_ = cmdutil.OpenBrowser(url)
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
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return fail("watch.store_open.failed", err, "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			}
			defer func() { _ = sqliteStore.DB().Close() }()
			logger.InfoContext(cmd.Context(), "watch.store_open.completed", "elapsed", time.Since(storeStarted).Round(time.Millisecond).String())
			watchStore := watch.NewStore(sqliteStore.DB())
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			events := make(chan watch.Event, 16)
			ready := make(chan watch.RunnerResult, 1)
			watchProgress := newWatchActivityProgress(cmd.ErrOrStderr(), watchClientCounter(url))
			defer func() {
				if watchProgress != nil {
					watchProgress.Stop()
				}
			}()
			go func() {
				for event := range events {
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
				_, runErr := watch.NewRunner(watchStore).Run(ctx, watch.RunnerOptions{Path: path, Rescan: rescan, Verbose: verbose, Embedding: embeddingCfg, Settings: watchSettings, DataDir: dataDir, Progress: progress, Logger: logger, Events: events, Ready: ready})
				errCh <- runErr
				close(events)
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\r\033[K\n")
			term.Separator(cmd.OutOrStdout())
			term.Label(cmd.OutOrStdout(), 20, "Watching", repo.RepoRoot)
			term.Label(cmd.OutOrStdout(), 20, "Repository", repoIdentity(repo))
			term.Label(cmd.OutOrStdout(), 20, "Branch", result.GitStatus.Branch)
			term.Label(cmd.OutOrStdout(), 20, "HEAD", result.GitStatus.HeadCommit)
			term.Label(cmd.OutOrStdout(), 20, "tlDiagram available at", term.URL(cmd.OutOrStdout(), url))
			term.Separator(cmd.OutOrStdout())
			term.Hint(cmd.OutOrStdout(), "Press Ctrl-C to stop watching.")
			logger.InfoContext(cmd.Context(), "watch.command.ready", "repository_id", repo.ID, "repo_root", repo.RepoRoot, "url", url)
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
	c.Flags().BoolVar(&verbose, "verbose", false, "print watch events")
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
			activity.Stop()
		}
		_, _ = fmt.Fprintf(out, "watch stopped\n")
		return true
	case "scan.started":
		_, _ = fmt.Fprintf(out, "\r\033[Kscanning source graph")
		return true
	case "scan.completed":
		if scan, ok := event.Data.(watch.ScanResult); ok {
			_, _ = fmt.Fprintf(out, "\r\033[Kscan complete: %d files, %d parsed, %d skipped", scan.FilesSeen, scan.FilesParsed, scan.FilesSkipped)
			return true
		}
		return false
	case "representation.started":
		_, _ = fmt.Fprintf(out, "\r\033[Kmaterializing representation")
		return true
	case "representation.updated":
		if rep, ok := event.Data.(watch.RepresentResult); ok {
			line := "Representation updated\n"
			line += "  Elements:"
			line += fmt.Sprintf(" %s", term.Colorize(out, term.ColorGreen, fmt.Sprintf("+%d \r\033[K", rep.ElementsCreated)))
			line += fmt.Sprintf(" %s", term.Colorize(out, term.ColorYellow, fmt.Sprintf("~%d \r\033[K", rep.ElementsUpdated)))

			line += "  Connectors:"
			line += fmt.Sprintf(" %s", term.Colorize(out, term.ColorGreen, fmt.Sprintf("+%d \r\033[K", rep.ConnectorsCreated)))
			line += fmt.Sprintf(" %s", term.Colorize(out, term.ColorYellow, fmt.Sprintf("~%d \r\033[K", rep.ConnectorsUpdated)))

			line += "  Embeddings:"
			line += fmt.Sprintf(" %s", term.Colorize(out, term.ColorGreen, fmt.Sprintf("+%d \r\033[K", rep.EmbeddingsCreated)))

			_, _ = fmt.Fprintf(out, "\r\033[K%s\n", line)
			return true
		}
		return false
	case "source.changed":
		result, ok := event.Data.(watch.SourceFileChangeResult)
		if !ok {
			return false
		}
		if activity != nil {
			activity.Advance("")
		}
		status := term.Colorize(out, term.ColorYellow, "no representation update")
		if result.RepresentationChanged {
			status = term.Colorize(out, term.ColorGreen, "representation updated")
		}
		_, _ = fmt.Fprintf(out, "%s %s %s: %s (%s)\n",
			term.Colorize(out, term.ColorBlue, "source"),
			term.Colorize(out, term.ColorYellow, result.Change.ChangeType),
			result.Change.Path,
			status,
			representationChangeSummary(result.Representation, result.GitTags),
		)
		return true
	case "watch.changeCounter":
		counter, ok := event.Data.(watch.ChangeCounter)
		if !ok {
			return false
		}
		if activity != nil {
			if counter.IntervalChangesProcessed > 0 {
				activity.Advance(fmt.Sprintf("watching: %d total, %d in last minute", counter.TotalChangesProcessed, counter.IntervalChangesProcessed))
			} else {
				activity.Advance(fmt.Sprintf("watching: %d total", counter.TotalChangesProcessed))
			}
			return true
		}
		if counter.IntervalChangesProcessed > 0 {
			_, _ = fmt.Fprintf(out, "changes processed: %d total, %d in the last minute\n",
				counter.TotalChangesProcessed,
				counter.IntervalChangesProcessed,
			)
		} else {
			_, _ = fmt.Fprintf(out, "changes processed: %d total\n",
				counter.TotalChangesProcessed,
			)
		}
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
	logger.InfoContext(ctx, "watch.event", fields...)
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
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return err
			}
			defer func() { _ = sqliteStore.DB().Close() }()
			scanner := watch.NewScanner(watch.NewStore(sqliteStore.DB()))
			scanner.Settings = watchSettings
			scanner.Progress = newCLIProgress(cmd.ErrOrStderr())
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
	var embeddingProvider, embeddingEndpoint, embeddingModel string
	var embeddingDimension int
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
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
			watchSettings := resolveWatchSettings(cfg, languageFlags, "", "", "", maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup)
			progress := newCLIProgress(cmd.ErrOrStderr())
			if embeddingCfg.Provider != "none" {
				checked, health, err := watch.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					return fmt.Errorf("embedding healthcheck failed: %w", err)
				}
				embeddingCfg = checked
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Embedding:       %s/%s dimension=%d similarity=%.3f\n", embeddingCfg.Provider, embeddingCfg.Model, health.Dimension, health.Similarity)
			}
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return err
			}
			defer func() { _ = sqliteStore.DB().Close() }()
			watchStore := watch.NewStore(sqliteStore.DB())
			scanner := watch.NewScanner(watchStore)
			scanner.Settings = watchSettings
			scanner.Progress = progress
			scanResult, err := scanner.ScanWithOptions(cmd.Context(), path, watch.ScanOptions{Force: rescan, DataDir: dataDir})
			if err != nil {
				return err
			}
			result, err := watch.NewRepresenter(watchStore).Represent(cmd.Context(), scanResult.RepositoryID, watch.RepresentRequest{Embedding: embeddingCfg, Thresholds: watchSettings.Thresholds, Visibility: watchSettings.Visibility, Progress: progress})
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
	var embeddingProvider, embeddingEndpoint, embeddingModel string
	var embeddingDimension int
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
				DataDirFlag:        dataDirFlag,
				EmbeddingProvider:  embeddingProvider,
				EmbeddingEndpoint:  embeddingEndpoint,
				EmbeddingModel:     embeddingModel,
				EmbeddingDimension: embeddingDimension,
				LanguageFlags:      languageFlags,
				MaxElements:        maxElements,
				MaxConnectors:      maxConnectors,
				MaxIncoming:        maxIncoming,
				MaxOutgoing:        maxOutgoing,
				MaxExpandedGroup:   maxExpandedGroup,
				FailOnDrift:        failOnDrift,
			})
		},
	}
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for representation")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for representation")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for representation")
	c.Flags().IntVar(&embeddingDimension, "embedding-dimension", 0, "embedding vector dimension")
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
	DataDirFlag        string
	EmbeddingProvider  string
	EmbeddingEndpoint  string
	EmbeddingModel     string
	EmbeddingDimension int
	LanguageFlags      []string
	MaxElements        int
	MaxConnectors      int
	MaxIncoming        int
	MaxOutgoing        int
	MaxExpandedGroup   int
	Rescan             bool
	FailOnDrift        bool
	GroupDiffs         bool
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
	embeddingCfg := resolveEmbeddingConfig(cfg, opts.EmbeddingProvider, opts.EmbeddingEndpoint, opts.EmbeddingModel, opts.EmbeddingDimension)
	watchSettings := resolveWatchSettings(cfg, opts.LanguageFlags, "", "", "", opts.MaxElements, opts.MaxConnectors, opts.MaxIncoming, opts.MaxOutgoing, opts.MaxExpandedGroup)
	logger.InfoContext(cmd.Context(), "watch.diff.started", "path", path, "data_dir", dataDir, "rescan", opts.Rescan, "fail_on_drift", opts.FailOnDrift, "group_diffs", opts.GroupDiffs, "embedding_provider", embeddingCfg.Provider, "embedding_model", embeddingCfg.Model, "languages", strings.Join(watchSettings.Languages, ","))
	sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		return fail("watch.diff.store_open.failed", err)
	}
	defer func() { _ = sqliteStore.DB().Close() }()
	watchStore := watch.NewStore(sqliteStore.DB())
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

func resolveEmbeddingConfig(cfg *workspace.Config, provider, endpoint, model string, dimension int) watch.EmbeddingConfig {
	embedding := watch.EmbeddingConfig{}
	if cfg != nil {
		embedding.Provider = cfg.Watch.Embedding.Provider
		embedding.Endpoint = cfg.Watch.Embedding.Endpoint
		embedding.Model = cfg.Watch.Embedding.Model
		embedding.Dimension = cfg.Watch.Embedding.Dimension
		embedding.HealthThreshold = cfg.Watch.Embedding.HealthThreshold
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
	return watch.NormalizeEmbeddingConfig(embedding)
}

func resolveWatchSettings(cfg *workspace.Config, languages []string, watcherMode, pollInterval, debounce string, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int) watch.Settings {
	settings := watch.DefaultSettings()
	if cfg != nil {
		settings.Languages = cfg.Watch.Languages
		settings.Watcher = cfg.Watch.Watcher
		settings.PollInterval = parseDurationOrZero(cfg.Watch.PollInterval)
		settings.Debounce = parseDurationOrZero(cfg.Watch.Debounce)
		settings.Thresholds = watch.Thresholds{
			MaxElementsPerView:            cfg.Watch.Thresholds.MaxElementsPerView,
			MaxConnectorsPerView:          cfg.Watch.Thresholds.MaxConnectorsPerView,
			MaxIncomingPerElement:         cfg.Watch.Thresholds.MaxIncomingPerElement,
			MaxOutgoingPerElement:         cfg.Watch.Thresholds.MaxOutgoingPerElement,
			MaxExpandedConnectorsPerGroup: cfg.Watch.Thresholds.MaxExpandedConnectorsPerGroup,
		}
		settings.Visibility = watch.VisibilityConfig{
			CoreThresholdEnabled:   cfg.Watch.Visibility.CoreThresholdEnabled,
			CoreThreshold:          cfg.Watch.Visibility.CoreThreshold,
			TierMultiplier:         cfg.Watch.Visibility.TierMultiplier,
			MaxExpansionMultiplier: cfg.Watch.Visibility.MaxExpansionMultiplier,
			CoreThresholdSet:       true,
			WeightsSet:             true,
			Weights: watch.VisibilityWeights{
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
		settings.Scale = watch.ScaleConfig{
			Strategy:        cfg.Watch.Scale.Strategy,
			MaxTrackedFiles: cfg.Watch.Scale.MaxTrackedFiles,
			MaxLimitedFiles: cfg.Watch.Scale.MaxLimitedFiles,
		}
	}
	if len(languages) > 0 {
		settings.Languages = languages
	}
	if watcherMode != "" {
		settings.Watcher = watcherMode
	}
	if pollInterval != "" {
		settings.PollInterval = parseDurationOrZero(pollInterval)
	}
	if debounce != "" {
		settings.Debounce = parseDurationOrZero(debounce)
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
	return watch.NormalizeSettings(settings)
}

func parseDurationOrZero(value string) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

type cliProgress struct {
	out io.Writer
	bar *progressbar.ProgressBar
	mu  sync.Mutex
}

type watchActivityProgress struct {
	out         io.Writer
	mu          sync.Mutex
	ticker      *time.Ticker
	stopCh      chan struct{}
	startTime   time.Time
	dots        int
	label       string
	clientCount func() int
}

func newCLIProgress(out io.Writer) watch.ProgressSink {
	if !term.IsTerminal(out) {
		return nil
	}
	return &cliProgress{out: out}
}

func newWatchActivityProgress(out io.Writer, clientCount func() int) *watchActivityProgress {
	if !term.IsTerminal(out) {
		return nil
	}
	return &watchActivityProgress{out: out, clientCount: clientCount}
}

func (p *watchActivityProgress) Start(label string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ticker != nil {
		if label != "" {
			p.label = label
			p.renderLocked(false)
		}
		return
	}
	p.label = label
	p.startTime = time.Now()
	p.ticker = time.NewTicker(1 * time.Second)
	p.stopCh = make(chan struct{})
	p.renderLocked(false)
	go func() {
		for {
			select {
			case <-p.ticker.C:
				p.mu.Lock()
				p.renderLocked(true)
				p.mu.Unlock()
			case <-p.stopCh:
				return
			}
		}
	}()
}

func (p *watchActivityProgress) renderLocked(incrementDots bool) {
	if incrementDots {
		p.dots = (p.dots + 1) % 4
	}
	dotsStr := strings.Repeat(".", p.dots) + strings.Repeat(" ", 3-p.dots)
	elapsed := time.Since(p.startTime).Round(time.Second)
	clientLabel := ""
	if p.clientCount != nil {
		clients := p.clientCount()
		plural := "s"
		if clients == 1 {
			plural = ""
		}
		clientLabel = fmt.Sprintf(" · %d client%s connected", clients, plural)
	}
	_, _ = fmt.Fprintf(p.out, "\r\033[K%s%s [%s]%s", term.Colorize(p.out, term.ColorCyan, p.label), dotsStr, elapsed, clientLabel)
}

func (p *watchActivityProgress) Advance(label string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ticker == nil {
		return
	}
	if label != "" {
		p.label = label
		p.renderLocked(false)
	}
}

func (p *watchActivityProgress) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ticker != nil {
		p.ticker.Stop()
		p.ticker = nil
	}
	if p.stopCh != nil {
		close(p.stopCh)
		p.stopCh = nil
	}
	_, _ = fmt.Fprintf(p.out, "\r\033[K")
}

func (p *cliProgress) Start(label string, total int) {
	if p == nil || total <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bar != nil {
		_ = p.bar.Finish()
	}
	p.bar = progressbar.NewOptions(total,
		progressbar.OptionSetWriter(p.out),
		progressbar.OptionSetVisibility(true),
		progressbar.OptionSetDescription(label),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(12),
		progressbar.OptionFullWidth(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionThrottle(60*time.Millisecond),
	)
}

func (p *cliProgress) Advance(label string) {
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

func (p *cliProgress) Finish() {
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
