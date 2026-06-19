package watch

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/ignore"
)

type OneShotOptions struct {
	Path             string
	Files            []string
	FocusFiles       []string
	Rescan           bool
	Embedding        EmbeddingConfig
	Settings         Settings
	Progress         ProgressSink
	DataDir          string
	Logger           EventLogger
	ConfirmAfterScan func(context.Context, ScanResult) error
	Rules            *ignore.Rules
}

type OneShotResult struct {
	Repository     Repository           `json:"repository"`
	Scan           ScanResult           `json:"scan"`
	Representation RepresentResult      `json:"representation"`
	GitStatus      GitStatus            `json:"git_status"`
	Diffs          []RepresentationDiff `json:"diffs,omitempty"`
}

type PipelineExecutor interface {
	Execute(context.Context, OneShotOptions) (OneShotResult, error)
}

type WatchPipeline struct {
	Store       *Store
	Scanner     *Scanner
	Representer *Representer
}

func NewWatchPipeline(store *Store) *WatchPipeline {
	return &WatchPipeline{
		Store:       store,
		Scanner:     NewScanner(store),
		Representer: NewRepresenter(store),
	}
}

func (r *Runner) RunOnce(ctx context.Context, opts OneShotOptions) (OneShotResult, error) {
	if r == nil {
		return OneShotResult{}, fmt.Errorf("watch runner requires a store")
	}
	return r.pipelineExecutor().Execute(ctx, opts)
}

func (r *Runner) pipelineExecutor() PipelineExecutor {
	if r.Pipeline != nil {
		return r.Pipeline
	}
	if r.Scanner == nil {
		r.Scanner = NewScanner(r.Store)
	}
	if r.Representer == nil {
		r.Representer = NewRepresenter(r.Store)
	}
	return &WatchPipeline{
		Store:       r.Store,
		Scanner:     r.Scanner,
		Representer: r.Representer,
	}
}

func (p *WatchPipeline) Execute(ctx context.Context, opts OneShotOptions) (OneShotResult, error) {
	if p == nil || p.Store == nil {
		return OneShotResult{}, fmt.Errorf("watch runner requires a store")
	}
	if p.Scanner == nil {
		p.Scanner = NewScanner(p.Store)
	}
	if p.Representer == nil {
		p.Representer = NewRepresenter(p.Store)
	}
	if opts.Path == "" {
		opts.Path = "."
	}
	settings := NormalizeSettings(opts.Settings)
	p.Scanner.Settings = settings
	p.Scanner.Progress = opts.Progress
	p.Scanner.Logger = opts.Logger
	if opts.Rules != nil {
		p.Scanner.Rules = opts.Rules
	}

	prepareStarted := time.Now()
	logInfo(ctx, opts.Logger, "watch.prepare.started", "path", opts.Path)
	progressStart(opts.Progress, "Preparing repository", 3)
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		progressFinish(opts.Progress)
		logError(ctx, opts.Logger, "watch.prepare.failed", err, "elapsed", logElapsed(prepareStarted))
		return OneShotResult{}, err
	}
	progressAdvance(opts.Progress, "Resolved repository path")
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		progressFinish(opts.Progress)
		logError(ctx, opts.Logger, "watch.prepare.failed", err, "elapsed", logElapsed(prepareStarted), "abs_path", absPath)
		return OneShotResult{}, fmt.Errorf("%s is not inside a git repository: %w", opts.Path, err)
	}
	progressAdvance(opts.Progress, "Detected git repository")
	gitStatus, _ := gitStatusSnapshot(repoRoot)
	progressAdvance(opts.Progress, "Captured git status")
	progressFinish(opts.Progress)
	logInfo(ctx, opts.Logger, "watch.prepare.completed", "elapsed", logElapsed(prepareStarted), "abs_path", absPath, "repo_root", repoRoot, "branch", gitStatus.Branch, "head", gitStatus.HeadCommit)

	scanStarted := time.Now()
	logInfo(ctx, opts.Logger, "watch.scan.started", "repo_root", repoRoot, "rescan", opts.Rescan)

	var scan ScanResult
	if len(opts.Files) > 0 {
		repoInput := RepositoryInput{
			RemoteURL:    detectString(func() (string, error) { return tldgit.DetectRemoteURL(repoRoot) }),
			RepoRoot:     repoRoot,
			DisplayName:  filepath.Base(repoRoot),
			Branch:       detectString(func() (string, error) { return tldgit.DetectBranch(repoRoot) }),
			HeadCommit:   detectString(func() (string, error) { return tldgit.DetectHeadCommit(repoRoot) }),
			SettingsHash: stableHash(settings),
		}
		repo, err := p.Store.EnsureRepository(ctx, repoInput)
		if err != nil {
			return OneShotResult{}, err
		}
		scan, err = p.Scanner.ScanFilesWithOptions(ctx, repo, opts.Files, ScanOptions{Force: opts.Rescan, DataDir: opts.DataDir, FocusFiles: opts.FocusFiles})
		if err != nil {
			logError(ctx, opts.Logger, "watch.scan.failed", err, "elapsed", logElapsed(scanStarted), "repo_root", repoRoot)
			return OneShotResult{}, err
		}
	} else {
		scan, err = p.Scanner.ScanWithOptions(ctx, repoRoot, ScanOptions{Force: opts.Rescan, DataDir: opts.DataDir, FocusFiles: opts.FocusFiles})
		if err != nil {
			logError(ctx, opts.Logger, "watch.scan.failed", err, "elapsed", logElapsed(scanStarted), "repo_root", repoRoot)
			return OneShotResult{}, err
		}
	}
	logInfo(ctx, opts.Logger, "watch.scan.completed", "elapsed", logElapsed(scanStarted), "repository_id", scan.RepositoryID, "scan_run_id", scan.ScanRunID, "files_seen", scan.FilesSeen, "files_parsed", scan.FilesParsed, "files_skipped", scan.FilesSkipped, "symbols_seen", scan.SymbolsSeen, "references_seen", scan.ReferencesSeen, "warnings", len(scan.Warnings), "mode", scan.Mode, "strategy", scan.Strategy)
	if opts.ConfirmAfterScan != nil {
		if err := opts.ConfirmAfterScan(ctx, scan); err != nil {
			logError(ctx, opts.Logger, "watch.scan.confirmation_failed", err, "repository_id", scan.RepositoryID)
			return OneShotResult{}, err
		}
	}
	repo, err := p.Store.Repository(ctx, scan.RepositoryID)
	if err != nil {
		logError(ctx, opts.Logger, "watch.repository.load.failed", err, "repository_id", scan.RepositoryID)
		return OneShotResult{}, err
	}
	representStarted := time.Now()
	logInfo(ctx, opts.Logger, "watch.representation.started", "repository_id", repo.ID)
	rep, err := p.Representer.Represent(ctx, repo.ID, RepresentRequest{
		Embedding:          opts.Embedding,
		Thresholds:         settings.Thresholds,
		Visibility:         settings.Visibility,
		Dependencies:       settings.Dependencies,
		AssumeNoRawChanges: !opts.Rescan && scan.FilesSeen > 0 && scan.FilesParsed == 0,
		BlastRadiusFiles:   scan.BlastRadiusFiles,
		Progress:           opts.Progress,
		Logger:             opts.Logger,
	})
	if err != nil {
		logError(ctx, opts.Logger, "watch.representation.failed", err, "elapsed", logElapsed(representStarted), "repository_id", repo.ID)
		return OneShotResult{}, err
	}
	logInfo(ctx, opts.Logger, "watch.representation.completed", "elapsed", logElapsed(representStarted), "repository_id", repo.ID, "representation_run_id", rep.RepresentationRun, "filter_run_id", rep.FilterRunID, "elements_created", rep.ElementsCreated, "elements_updated", rep.ElementsUpdated, "connectors_created", rep.ConnectorsCreated, "connectors_updated", rep.ConnectorsUpdated, "views_created", rep.ViewsCreated, "embedding_cache_hits", rep.EmbeddingCacheHits, "embeddings_created", rep.EmbeddingsCreated)
	if latest, found, err := p.Store.LatestWatchVersion(ctx, repo.ID); err != nil {
		logError(ctx, opts.Logger, "watch.diffs.reuse_check.failed", err, "repository_id", repo.ID, "representation_hash", rep.RepresentationHash)
		return OneShotResult{}, err
	} else if found && latest.RepresentationHash == rep.RepresentationHash {
		logInfo(ctx, opts.Logger, "watch.diffs.reused", "repository_id", repo.ID, "version_id", latest.ID, "representation_hash", rep.RepresentationHash)
		return OneShotResult{Repository: repo, Scan: scan, Representation: rep, GitStatus: gitStatus, Diffs: []RepresentationDiff{}}, nil
	}
	diffStarted := time.Now()
	logInfo(ctx, opts.Logger, "watch.diffs.started", "repository_id", repo.ID, "representation_hash", rep.RepresentationHash)
	progressStart(opts.Progress, "Computing representation diffs", 1)
	diffs, err := p.Store.BuildWatchDiffs(ctx, repo.ID, rep.RepresentationHash)
	if err != nil {
		progressFinish(opts.Progress)
		logError(ctx, opts.Logger, "watch.diffs.failed", err, "elapsed", logElapsed(diffStarted), "repository_id", repo.ID)
		return OneShotResult{}, err
	}
	progressAdvance(opts.Progress, "Representation diffs computed")
	progressFinish(opts.Progress)
	logInfo(ctx, opts.Logger, "watch.diffs.completed", "elapsed", logElapsed(diffStarted), "repository_id", repo.ID, "diffs", len(diffs))
	return OneShotResult{Repository: repo, Scan: scan, Representation: rep, GitStatus: gitStatus, Diffs: diffs}, nil
}
