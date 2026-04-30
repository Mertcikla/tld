package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	assets "github.com/mertcikla/tld"
	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/store"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/watch"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

func NewWatchCmd() *cobra.Command {
	var host, port, dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel string
	var embeddingDimension int
	var languageFlags []string
	var watcherMode, pollInterval, debounce string
	var maxElements, maxConnectors, maxIncoming, maxOutgoing int
	var noServe, openBrowser, rescan, verbose bool
	c := &cobra.Command{
		Use:   "watch [path]",
		Short: "Scan and materialize source repositories into the local workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			cfg, _ := workspace.LoadGlobalConfig()
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
			watchSettings := resolveWatchSettings(cfg, languageFlags, watcherMode, pollInterval, debounce, maxElements, maxConnectors, maxIncoming, maxOutgoing)
			term.Infof(cmd.OutOrStdout(), "watch booting: data=%s embeddings=%s/%s", term.Path(cmd.OutOrStdout(), dataDir), embeddingCfg.Provider, embeddingCfg.Model)
			progress := newCLIProgress(cmd.ErrOrStderr())
			if embeddingCfg.Provider != "none" {
				term.Infof(cmd.OutOrStdout(), "embedding healthcheck: %s %s", embeddingCfg.Endpoint, embeddingCfg.Model)
				checked, health, err := watch.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					return fmt.Errorf("embedding healthcheck failed: %w", err)
				}
				embeddingCfg = checked
				term.Successf(cmd.OutOrStdout(), "embedding healthcheck ok: dimension=%d similarity=%.3f", health.Dimension, health.Similarity)
			}
			addr := localserver.ResolveAddr(localserver.ServeOptions{Host: host, Port: port})
			url := "http://" + addr
			var srv *http.Server
			if !noServe {
				if !serverReady(url) {
					term.Infof(cmd.OutOrStdout(), "server booting: %s", url)
					app, err := localserver.Bootstrap(dataDir, localserver.ServeOptions{Host: host, Port: port})
					if err != nil {
						return err
					}
					srv = &http.Server{Addr: app.Addr, Handler: app.Handler}
					go func() {
						if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							term.Failf(cmd.ErrOrStderr(), "server error: %v", err)
						}
					}()
					url = "http://" + app.Addr
				}
				term.Successf(cmd.OutOrStdout(), "server ready: %s", term.URL(cmd.OutOrStdout(), url))
				if openBrowser {
					_ = cmdutil.OpenBrowser(url)
				}
			}
			defer func() {
				if srv != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = srv.Shutdown(ctx)
				}
			}()

			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return err
			}
			watchStore := watch.NewStore(sqliteStore.DB())
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			events := make(chan watch.Event, 16)
			ready := make(chan watch.RunnerResult, 1)
			watchProgress := newWatchActivityProgress(cmd.ErrOrStderr())
			defer func() {
				if watchProgress != nil {
					watchProgress.Stop()
				}
			}()
			go func() {
				for event := range events {
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
				_, runErr := watch.NewRunner(watchStore).Run(ctx, watch.RunnerOptions{Path: path, Rescan: rescan, Verbose: verbose, Embedding: embeddingCfg, Settings: watchSettings, Progress: progress, Events: events, Ready: ready})
				errCh <- runErr
				close(events)
			}()
			var result watch.RunnerResult
			select {
			case result = <-ready:
			case err := <-errCh:
				if err != nil {
					return err
				}
				return nil
			}
			repo := result.Repository
			term.Separator(cmd.OutOrStdout())
			term.Label(cmd.OutOrStdout(), 20, "Watching", repo.RepoRoot)
			term.Label(cmd.OutOrStdout(), 20, "Repository", repoIdentity(repo))
			term.Label(cmd.OutOrStdout(), 20, "Branch", result.GitStatus.Branch)
			term.Label(cmd.OutOrStdout(), 20, "HEAD", result.GitStatus.HeadCommit)
			term.Label(cmd.OutOrStdout(), 20, "Mode", "watch")
			term.Label(cmd.OutOrStdout(), 20, "tlDiagram available at", term.URL(cmd.OutOrStdout(), url))
			term.Separator(cmd.OutOrStdout())
			term.Hint(cmd.OutOrStdout(), "Press Ctrl-C to stop watching.")
			if err := <-errCh; err != nil {
				return err
			}
			return nil
		},
	}
	c.Flags().StringVar(&host, "host", "", "host for the local app server")
	c.Flags().StringVar(&port, "port", "", "port for the local app server")
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().BoolVar(&noServe, "no-serve", false, "do not start the local app server")
	c.Flags().BoolVar(&openBrowser, "open", false, "open the webapp in a browser")
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
	c.Flags().BoolVar(&rescan, "rescan", false, "force a rescan before watching")
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
		if activity != nil {
			activity.Start("watching for changes")
		}
		_, _ = fmt.Fprintf(out, "%s started\n", term.Colorize(out, term.ColorGreen, "watch"))
		return true
	case "watch.stopped":
		if activity != nil {
			activity.Stop()
		}
		_, _ = fmt.Fprintf(out, "%s stopped\n", term.Colorize(out, term.ColorYellow, "watch"))
		return true
	case "scan.started":
		_, _ = fmt.Fprintf(out, "%s scanning source graph\n", term.Colorize(out, term.ColorBlue, "watch"))
		return true
	case "scan.completed":
		if scan, ok := event.Data.(watch.ScanResult); ok {
			_, _ = fmt.Fprintf(out, "%s scan complete: %d files, %d parsed, %d skipped\n", term.Colorize(out, term.ColorGreen, "watch"), scan.FilesSeen, scan.FilesParsed, scan.FilesSkipped)
			return true
		}
		return false
	case "representation.started":
		_, _ = fmt.Fprintf(out, "%s materializing representation\n", term.Colorize(out, term.ColorBlue, "watch"))
		return true
	case "representation.updated":
		if rep, ok := event.Data.(watch.RepresentResult); ok {
			_, _ = fmt.Fprintf(out, "%s representation updated: elements +%d/%d, connectors +%d/%d, embeddings +%d/%d cached\n",
				term.Colorize(out, term.ColorGreen, "watch"),
				rep.ElementsCreated, rep.ElementsUpdated,
				rep.ConnectorsCreated, rep.ConnectorsUpdated,
				rep.EmbeddingsCreated, rep.EmbeddingCacheHits)
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
			_, _ = fmt.Fprintf(out, "%s changes processed: %d total, %d in the last minute\n",
				term.Colorize(out, term.ColorBlue, "watch"),
				counter.TotalChangesProcessed,
				counter.IntervalChangesProcessed,
			)
		} else {
			_, _ = fmt.Fprintf(out, "%s changes processed: %d total\n",
				term.Colorize(out, term.ColorBlue, "watch"),
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
		_, _ = fmt.Fprintf(out, "%s version created\n", term.Colorize(out, term.ColorGreen, "watch"))
		return true
	default:
		return false
	}
}

func representationChangeSummary(rep watch.RepresentResult, tags watch.GitTagUpdateResult) string {
	return fmt.Sprintf("elements +%d/%d, connectors +%d/%d, views +%d, tags +%d/-%d",
		rep.ElementsCreated,
		rep.ElementsUpdated,
		rep.ConnectorsCreated,
		rep.ConnectorsUpdated,
		rep.ViewsCreated,
		tags.TagsAdded,
		tags.TagsRemoved,
	)
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
			cfg, _ := workspace.LoadGlobalConfig()
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}
			watchSettings := resolveWatchSettings(cfg, languageFlags, "", "", "", 0, 0, 0, 0)
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return err
			}
			scanner := watch.NewScanner(watch.NewStore(sqliteStore.DB()))
			scanner.Settings = watchSettings
			scanner.Progress = newCLIProgress(cmd.ErrOrStderr())
			result, err := scanner.ScanWithOptions(cmd.Context(), path, watch.ScanOptions{Force: rescan})
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
	var maxElements, maxConnectors, maxIncoming, maxOutgoing int
	c := &cobra.Command{
		Use:   "represent [path]",
		Short: "Materialize a scanned repository into the local workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			cfg, _ := workspace.LoadGlobalConfig()
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
			watchSettings := resolveWatchSettings(cfg, languageFlags, "", "", "", maxElements, maxConnectors, maxIncoming, maxOutgoing)
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
			watchStore := watch.NewStore(sqliteStore.DB())
			scanner := watch.NewScanner(watchStore)
			scanner.Settings = watchSettings
			scanner.Progress = progress
			scanResult, err := scanner.ScanWithOptions(cmd.Context(), path, watch.ScanOptions{Force: rescan})
			if err != nil {
				return err
			}
			result, err := watch.NewRepresenter(watchStore).Represent(cmd.Context(), scanResult.RepositoryID, watch.RepresentRequest{Embedding: embeddingCfg, Thresholds: watchSettings.Thresholds, Progress: progress})
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
	var maxElements, maxConnectors, maxIncoming, maxOutgoing int
	c := &cobra.Command{
		Use:   "diff [path]",
		Short: "Scan and report watch representation drift as JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			cfg, _ := workspace.LoadGlobalConfig()
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
			watchSettings := resolveWatchSettings(cfg, languageFlags, "", "", "", maxElements, maxConnectors, maxIncoming, maxOutgoing)
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return err
			}
			watchStore := watch.NewStore(sqliteStore.DB())
			scanner := watch.NewScanner(watchStore)
			scanner.Settings = watchSettings
			scanResult, err := scanner.ScanWithOptions(cmd.Context(), path, watch.ScanOptions{})
			if err != nil {
				return err
			}
			result, err := watch.NewRepresenter(watchStore).Represent(cmd.Context(), scanResult.RepositoryID, watch.RepresentRequest{Embedding: embeddingCfg, Thresholds: watchSettings.Thresholds})
			if err != nil {
				return err
			}
			diffs, err := watchStore.BuildWatchDiffs(cmd.Context(), scanResult.RepositoryID, result.RepresentationHash)
			if err != nil {
				return err
			}
			latest, found, err := watchStore.LatestWatchVersion(cmd.Context(), scanResult.RepositoryID)
			if err != nil {
				return err
			}
			changed := !found || latest.RepresentationHash != result.RepresentationHash || len(diffs) > 1
			payload := struct {
				Changed        bool                       `json:"changed"`
				Scan           watch.ScanResult           `json:"scan"`
				Representation watch.RepresentResult      `json:"representation"`
				Diffs          []watch.RepresentationDiff `json:"diffs"`
			}{Changed: changed, Scan: scanResult, Representation: result, Diffs: diffs}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); err != nil {
				return err
			}
			if failOnDrift && changed {
				return fmt.Errorf("watch representation drift detected")
			}
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
	c.Flags().BoolVar(&failOnDrift, "fail-on-drift", false, "exit nonzero when representation drift is detected")
	return c
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

func resolveWatchSettings(cfg *workspace.Config, languages []string, watcherMode, pollInterval, debounce string, maxElements, maxConnectors, maxIncoming, maxOutgoing int) watch.Settings {
	settings := watch.DefaultSettings()
	if cfg != nil {
		settings.Languages = cfg.Watch.Languages
		settings.Watcher = cfg.Watch.Watcher
		settings.PollInterval = parseDurationOrZero(cfg.Watch.PollInterval)
		settings.Debounce = parseDurationOrZero(cfg.Watch.Debounce)
		settings.Thresholds = watch.Thresholds{
			MaxElementsPerView:    cfg.Watch.Thresholds.MaxElementsPerView,
			MaxConnectorsPerView:  cfg.Watch.Thresholds.MaxConnectorsPerView,
			MaxIncomingPerElement: cfg.Watch.Thresholds.MaxIncomingPerElement,
			MaxOutgoingPerElement: cfg.Watch.Thresholds.MaxOutgoingPerElement,
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
	out    io.Writer
	bar    *progressbar.ProgressBar
	mu     sync.Mutex
	ticker *time.Ticker
	stopCh chan struct{}
}

func newCLIProgress(out io.Writer) watch.ProgressSink {
	if !term.IsTerminal(out) {
		return nil
	}
	return &cliProgress{out: out}
}

func newWatchActivityProgress(out io.Writer) *watchActivityProgress {
	if !term.IsTerminal(out) {
		return nil
	}
	return &watchActivityProgress{out: out}
}

func (p *watchActivityProgress) Start(label string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bar != nil {
		if label != "" {
			p.bar.Describe(label)
		}
		return
	}
	p.bar = progressbar.NewOptions(-1,
		progressbar.OptionSetWriter(p.out),
		progressbar.OptionSetVisibility(true),
		progressbar.OptionSetDescription(label),
		progressbar.OptionSetWidth(12),
		progressbar.OptionShowIts(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionThrottle(60*time.Millisecond),
		progressbar.OptionSetPredictTime(false),
	)
	p.ticker = time.NewTicker(200 * time.Millisecond)
	p.stopCh = make(chan struct{})
	go func() {
		for {
			select {
			case <-p.ticker.C:
				p.Advance("")
			case <-p.stopCh:
				return
			}
		}
	}()
}

func (p *watchActivityProgress) Advance(label string) {
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
	if p.bar != nil {
		_ = p.bar.Finish()
		p.bar = nil
	}
}

func (p *cliProgress) Start(label string, total int) {
	if p == nil || total <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
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
