package watch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "watch booting: data=%s embeddings=%s/%s\n", dataDir, embeddingCfg.Provider, embeddingCfg.Model)
			progress := newCLIProgress(cmd.ErrOrStderr())
			if embeddingCfg.Provider != "none" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "embedding healthcheck: %s %s\n", embeddingCfg.Endpoint, embeddingCfg.Model)
				checked, health, err := watch.CheckEmbeddingHealth(cmd.Context(), embeddingCfg)
				if err != nil {
					return fmt.Errorf("embedding healthcheck failed: %w", err)
				}
				embeddingCfg = checked
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "embedding healthcheck ok: dimension=%d similarity=%.3f\n", health.Dimension, health.Similarity)
			}
			addr := localserver.ResolveAddr(localserver.ServeOptions{Host: host, Port: port})
			url := "http://" + addr
			var srv *http.Server
			if !noServe {
				if !serverReady(url) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "server booting: %s\n", url)
					app, err := localserver.Bootstrap(dataDir, localserver.ServeOptions{Host: host, Port: port})
					if err != nil {
						return err
					}
					srv = &http.Server{Addr: app.Addr, Handler: app.Handler}
					go func() {
						if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "server error: %v\n", err)
						}
					}()
					url = "http://" + app.Addr
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "server ready: %s\n", url)
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
			go func() {
				for event := range events {
					if logWatchEvent(cmd, event) {
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
				_, runErr := watch.NewRunner(watchStore).Run(ctx, watch.RunnerOptions{Path: path, Rescan: rescan, Verbose: verbose, Embedding: embeddingCfg, Progress: progress, Events: events, Ready: ready})
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Watching:            %s\n", repo.RepoRoot)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Repository:          %s\n", repoIdentity(repo))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Branch:              %s\n", result.GitStatus.Branch)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "HEAD:                %s\n", result.GitStatus.HeadCommit)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Mode:                watch\n")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "tlDiagram available at: %s\n\n", url)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl-C to stop watching.")
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
	c.Flags().BoolVar(&rescan, "rescan", false, "force a rescan before watching")
	c.Flags().BoolVar(&verbose, "verbose", false, "print watch events")
	c.AddCommand(newScanCmd())
	c.AddCommand(newRepresentCmd())
	return c
}

func logWatchEvent(cmd *cobra.Command, event watch.Event) bool {
	out := cmd.OutOrStdout()
	switch event.Type {
	case "watch.started":
		_, _ = fmt.Fprintf(out, "%s started\n", term.Colorize(out, term.ColorGreen, "watch"))
		return true
	case "watch.stopped":
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
		_, _ = fmt.Fprintf(out, "%s changes processed: %d total, %d in the last minute\n",
			term.Colorize(out, term.ColorBlue, "watch"),
			counter.TotalChangesProcessed,
			counter.IntervalChangesProcessed,
		)
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
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func newScanCmd() *cobra.Command {
	var dataDirFlag string
	c := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan a Go repository into the local raw code graph",
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
			sqliteStore, err := store.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				return err
			}
			scanner := watch.NewScanner(watch.NewStore(sqliteStore.DB()))
			scanner.Progress = newCLIProgress(cmd.ErrOrStderr())
			result, err := scanner.Scan(cmd.Context(), path)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Repository: %d\n", result.RepositoryID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Scan run:   %d\n", result.ScanRunID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Files:      %d seen, %d parsed, %d skipped\n", result.FilesSeen, result.FilesParsed, result.FilesSkipped)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Symbols:    %d\n", result.SymbolsSeen)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "References: %d\n", result.ReferencesSeen)
			if result.Warning != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning:    %s\n", result.Warning)
			}
			return nil
		},
	}
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	return c
}

func newRepresentCmd() *cobra.Command {
	var dataDirFlag string
	var embeddingProvider, embeddingEndpoint, embeddingModel string
	var embeddingDimension int
	c := &cobra.Command{
		Use:   "represent [path]",
		Short: "Materialize a scanned Go repository into the local workspace",
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
			scanner.Progress = progress
			scanResult, err := scanner.Scan(cmd.Context(), path)
			if err != nil {
				return err
			}
			result, err := watch.NewRepresenter(watchStore).Represent(cmd.Context(), scanResult.RepositoryID, watch.RepresentRequest{Embedding: embeddingCfg, Progress: progress})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Repository:      %d\n", result.RepositoryID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Scan run:        %d\n", scanResult.ScanRunID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Filter run:      %d\n", result.FilterRunID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Represent run:   %d\n", result.RepresentationRun)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Elements:        %d created, %d updated\n", result.ElementsCreated, result.ElementsUpdated)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Connectors:      %d created, %d updated\n", result.ConnectorsCreated, result.ConnectorsUpdated)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Views:           %d created\n", result.ViewsCreated)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Raw graph hash:  %s\n", result.RawGraphHash)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Representation: %s\n", result.RepresentationHash)
			return nil
		},
	}
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory for the local app database")
	c.Flags().StringVar(&embeddingProvider, "embedding-provider", "", "embedding provider for representation")
	c.Flags().StringVar(&embeddingEndpoint, "embedding-endpoint", "", "embedding endpoint for representation")
	c.Flags().StringVar(&embeddingModel, "embedding-model", "", "embedding model for representation")
	c.Flags().IntVar(&embeddingDimension, "embedding-dimension", 0, "embedding vector dimension")
	return c
}

func resolveEmbeddingConfig(cfg *workspace.GlobalConfig, provider, endpoint, model string, dimension int) watch.EmbeddingConfig {
	embedding := watch.EmbeddingConfig{}
	if cfg != nil {
		embedding.Provider = cfg.Watch.Embedding.Provider
		embedding.Endpoint = cfg.Watch.Embedding.Endpoint
		embedding.Model = cfg.Watch.Embedding.Model
		embedding.Dimension = cfg.Watch.Embedding.Dimension
		embedding.HealthThreshold = cfg.Watch.Embedding.HealthThreshold
	}
	if value := os.Getenv("TLD_EMBEDDING_PROVIDER"); value != "" {
		embedding.Provider = value
	}
	if value := os.Getenv("TLD_EMBEDDING_ENDPOINT"); value != "" {
		embedding.Endpoint = value
	}
	if value := os.Getenv("TLD_EMBEDDING_MODEL"); value != "" {
		embedding.Model = value
	}
	if value := os.Getenv("TLD_EMBEDDING_DIMENSION"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			embedding.Dimension = parsed
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
	return watch.NormalizeEmbeddingConfig(embedding)
}

type cliProgress struct {
	out io.Writer
	bar *progressbar.ProgressBar
}

func newCLIProgress(out io.Writer) watch.ProgressSink {
	if !term.IsTerminal(out) {
		return nil
	}
	return &cliProgress{out: out}
}

func (p *cliProgress) Start(label string, total int) {
	if p == nil || total <= 0 {
		return
	}
	p.bar = progressbar.NewOptions(total,
		progressbar.OptionSetWriter(p.out),
		progressbar.OptionSetVisibility(true),
		progressbar.OptionSetDescription(label),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(12),
		progressbar.OptionFullWidth(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionThrottle(60*time.Millisecond),
	)
}

func (p *cliProgress) Advance(label string) {
	if p == nil || p.bar == nil {
		return
	}
	if label != "" {
		p.bar.Describe(label)
	}
	_ = p.bar.Add(1)
}

func (p *cliProgress) Finish() {
	if p == nil || p.bar == nil {
		return
	}
	_ = p.bar.Finish()
	p.bar = nil
}
