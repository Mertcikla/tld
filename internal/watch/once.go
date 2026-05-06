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

	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return OneShotResult{}, err
	}
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		return OneShotResult{}, fmt.Errorf("%s is not inside a git repository: %w", opts.Path, err)
	}
	gitStatus, _ := gitStatusSnapshot(repoRoot)
	scan, err := r.Scanner.ScanWithOptions(ctx, repoRoot, ScanOptions{Force: opts.Rescan})
	if err != nil {
		return OneShotResult{}, err
	}
	repo, err := r.Store.Repository(ctx, scan.RepositoryID)
	if err != nil {
		return OneShotResult{}, err
	}
	rep, err := r.Representer.Represent(ctx, repo.ID, RepresentRequest{Embedding: opts.Embedding, Thresholds: settings.Thresholds, Visibility: settings.Visibility, Progress: opts.Progress})
	if err != nil {
		return OneShotResult{}, err
	}
	diffs, err := r.Store.BuildWatchDiffs(ctx, repo.ID, rep.RepresentationHash)
	if err != nil {
		return OneShotResult{}, err
	}
	return OneShotResult{Repository: repo, Scan: scan, Representation: rep, GitStatus: gitStatus, Diffs: diffs}, nil
}
