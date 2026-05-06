package watch

import (
	"context"
	"fmt"
	"path/filepath"

	tldgit "github.com/mertcikla/tld/internal/git"
)

type OneShotOptions struct {
	Path      string
	Rescan    bool
	Embedding EmbeddingConfig
	Settings  Settings
	Progress  ProgressSink
}

type OneShotResult struct {
	Repository     Repository           `json:"repository"`
	Scan           ScanResult           `json:"scan"`
	Representation RepresentResult      `json:"representation"`
	GitStatus      GitStatus            `json:"git_status"`
	Diffs          []RepresentationDiff `json:"diffs,omitempty"`
}

func (r *Runner) RunOnce(ctx context.Context, opts OneShotOptions) (OneShotResult, error) {
	if r == nil || r.Store == nil {
		return OneShotResult{}, fmt.Errorf("watch runner requires a store")
	}
	if r.Scanner == nil {
		r.Scanner = NewScanner(r.Store)
	}
	if r.Representer == nil {
		r.Representer = NewRepresenter(r.Store)
	}
	if opts.Path == "" {
		opts.Path = "."
	}
	settings := NormalizeSettings(opts.Settings)
	r.Scanner.Settings = settings
	r.Scanner.Progress = opts.Progress

	progressStart(opts.Progress, "Preparing repository", 3)
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		progressFinish(opts.Progress)
		return OneShotResult{}, err
	}
	progressAdvance(opts.Progress, "Resolved repository path")
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		progressFinish(opts.Progress)
		return OneShotResult{}, fmt.Errorf("%s is not inside a git repository: %w", opts.Path, err)
	}
	progressAdvance(opts.Progress, "Detected git repository")
	gitStatus, _ := gitStatusSnapshot(repoRoot)
	progressAdvance(opts.Progress, "Captured git status")
	progressFinish(opts.Progress)

	architecture := inferArchitectureWithProgress(repoRoot, opts.Progress)
	if len(architecture.Components) > 0 {
		progressStart(opts.Progress, "Registering architecture repository", 1)
		repoInput := RepositoryInput{
			RemoteURL:    detectString(func() (string, error) { return tldgit.DetectRemoteURL(repoRoot) }),
			RepoRoot:     repoRoot,
			DisplayName:  filepath.Base(repoRoot),
			Branch:       detectString(func() (string, error) { return tldgit.DetectBranch(repoRoot) }),
			HeadCommit:   detectString(func() (string, error) { return tldgit.DetectHeadCommit(repoRoot) }),
			SettingsHash: stableHash(settings),
		}
		repo, err := r.Store.EnsureRepository(ctx, repoInput)
		if err != nil {
			progressFinish(opts.Progress)
			return OneShotResult{}, err
		}
		progressAdvance(opts.Progress, "Repository registered")
		progressFinish(opts.Progress)
		scan := ScanResult{RepositoryID: repo.ID, Warning: "architecture artifacts detected; source symbol scan skipped"}
		rep, err := r.Representer.RepresentArchitecture(ctx, repo, architecture, settings.Thresholds, opts.Progress)
		if err != nil {
			return OneShotResult{}, err
		}
		progressStart(opts.Progress, "Computing architecture diffs", 1)
		diffs, err := r.Store.BuildWatchDiffs(ctx, repo.ID, rep.RepresentationHash)
		if err != nil {
			progressFinish(opts.Progress)
			return OneShotResult{}, err
		}
		progressAdvance(opts.Progress, "Architecture diffs computed")
		progressFinish(opts.Progress)
		return OneShotResult{Repository: repo, Scan: scan, Representation: rep, GitStatus: gitStatus, Diffs: diffs}, nil
	}
	scan, err := r.Scanner.ScanWithOptions(ctx, repoRoot, ScanOptions{Force: opts.Rescan})
	if err != nil {
		return OneShotResult{}, err
	}
	repo, err := r.Store.Repository(ctx, scan.RepositoryID)
	if err != nil {
		return OneShotResult{}, err
	}
	rep, err := r.Representer.Represent(ctx, repo.ID, RepresentRequest{Embedding: opts.Embedding, Thresholds: settings.Thresholds, Progress: opts.Progress})
	if err != nil {
		return OneShotResult{}, err
	}
	progressStart(opts.Progress, "Computing representation diffs", 1)
	diffs, err := r.Store.BuildWatchDiffs(ctx, repo.ID, rep.RepresentationHash)
	if err != nil {
		progressFinish(opts.Progress)
		return OneShotResult{}, err
	}
	progressAdvance(opts.Progress, "Representation diffs computed")
	progressFinish(opts.Progress)
	return OneShotResult{Repository: repo, Scan: scan, Representation: rep, GitStatus: gitStatus, Diffs: diffs}, nil
}
