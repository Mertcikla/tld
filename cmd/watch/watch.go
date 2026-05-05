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
	var maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int
	var noServe, openBrowser, rescan, verbose, dryRun, failOnDrift bool
	c := &cobra.Command{
		Use:   "watch [path]",
		Short: "Scan and materialize source repositories into the local workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			cfg, _ := workspace.LoadGlobalConfig()
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}
			embeddingCfg := resolveEmbeddingConfig(cfg, embeddingProvider, embeddingEndpoint, embeddingModel, embeddingDimension)
			watchSettings := resolveWatchSettings(cfg, languageFlags, watcherMode, pollInterval, debounce, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup)
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
			watchProgress := newWatchActivityProgress(cmd.ErrOrStderr(), watchClientCounter(url))
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
			cfg, _ := workspace.LoadGlobalConfig()
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
			cfg, _ := workspace.LoadGlobalConfig()
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
	cfg, _ := workspace.LoadGlobalConfig()
	dataDir, err := workspace.ResolveDataDir(cfg, opts.DataDirFlag)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	embeddingCfg := resolveEmbeddingConfig(cfg, opts.EmbeddingProvider, opts.EmbeddingEndpoint, opts.EmbeddingModel, opts.EmbeddingDimension)
	watchSettings := resolveWatchSettings(cfg, opts.LanguageFlags, "", "", "", opts.MaxElements, opts.MaxConnectors, opts.MaxIncoming, opts.MaxOutgoing, opts.MaxExpandedGroup)
	sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		return err
	}
	defer func() { _ = sqliteStore.DB().Close() }()
	watchStore := watch.NewStore(sqliteStore.DB())
	once, err := watch.NewRunner(watchStore).RunOnce(cmd.Context(), watch.OneShotOptions{Path: path, Rescan: opts.Rescan, Embedding: embeddingCfg, Settings: watchSettings})
	if err != nil {
		return err
	}
	latest, found, err := watchStore.LatestWatchVersion(cmd.Context(), once.Scan.RepositoryID)
	if err != nil {
		return err
	}
	changed := found && latest.RepresentationHash != once.Representation.RepresentationHash || hasWatchDriftDiffs(once.Diffs)
	var payload any = watchDiffPayload{Changed: changed, Scan: once.Scan, Representation: once.Representation, Diffs: once.Diffs}
	if opts.GroupDiffs {
		payload = watchGroupedDiffPayload{Changed: changed, Scan: once.Scan, Representation: once.Representation, Diffs: groupWatchDiffs(once.Diffs)}
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(payload); err != nil {
		return err
	}
	if opts.FailOnDrift && changed {
		return fmt.Errorf("watch representation drift detected")
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
