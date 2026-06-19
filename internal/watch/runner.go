package watch

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/ignore"
)

type RunnerOptions struct {
	Path              string
	Rescan            bool
	Verbose           bool
	PollInterval      time.Duration
	Debounce          time.Duration
	HeartbeatInterval time.Duration
	SummaryInterval   time.Duration
	Embedding         EmbeddingConfig
	Settings          Settings
	DataDir           string
	Progress          ProgressSink
	Logger            EventLogger
	Events            *EventQueue
	Ready             chan<- RunnerResult
	ConfirmAfterScan  func(context.Context, ScanResult) error
	Rules             *ignore.Rules
}

type RunnerResult struct {
	Repository  Repository
	InitialScan ScanResult
	InitialRep  RepresentResult
	GitStatus   GitStatus
	Token       string
}

type Runner struct {
	Store       *Store
	Scanner     *Scanner
	Representer *Representer
	Pipeline    PipelineExecutor
}

func NewRunner(store *Store) *Runner {
	return &Runner{
		Store:       store,
		Scanner:     NewScanner(store),
		Representer: NewRepresenter(store),
	}
}

func (r *Runner) Run(ctx context.Context, opts RunnerOptions) (RunnerResult, error) {
	if r == nil || r.Store == nil {
		return RunnerResult{}, fmt.Errorf("watch runner requires a store")
	}
	if r.Scanner == nil {
		r.Scanner = NewScanner(r.Store)
	}
	r.Scanner.Progress = opts.Progress
	r.Scanner.Logger = opts.Logger
	if opts.Rules != nil {
		r.Scanner.Rules = opts.Rules
	}
	if r.Representer == nil {
		r.Representer = NewRepresenter(r.Store)
	}
	if opts.Path == "" {
		opts.Path = "."
	}
	settings := NormalizeSettings(opts.Settings)
	opts.Settings = settings
	if opts.PollInterval <= 0 {
		opts.PollInterval = settings.PollInterval
	}
	if opts.Debounce <= 0 {
		opts.Debounce = settings.Debounce
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = 2 * time.Second
	}
	if opts.SummaryInterval <= 0 {
		opts.SummaryInterval = time.Minute
	}
	started := time.Now()
	logInfo(ctx, opts.Logger, "watch.runner.started", "path", opts.Path, "rescan", opts.Rescan, "watcher", settings.Watcher, "poll_interval", opts.PollInterval.String(), "debounce", opts.Debounce.String(), "languages", strings.Join(settings.Languages, ","))
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		logError(ctx, opts.Logger, "watch.runner.failed", err, "elapsed", logElapsed(started))
		return RunnerResult{}, err
	}
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		logError(ctx, opts.Logger, "watch.runner.failed", err, "elapsed", logElapsed(started), "abs_path", absPath)
		return RunnerResult{}, fmt.Errorf("%s is not inside a git repository: %w", opts.Path, err)
	}
	logInfo(ctx, opts.Logger, "watch.runner.repository_resolved", "elapsed", logElapsed(started), "repo_root", repoRoot)

	events := QueueEventPublisher{Queue: opts.Events}
	gitStatus, _ := gitStatusSnapshot(repoRoot)
	events.Publish(Event{Type: "scan.started", At: nowString(), Phase: "scan", WatcherMode: settings.Watcher, Languages: settings.Languages})
	once, err := r.RunOnce(ctx, OneShotOptions{Path: repoRoot, Rescan: opts.Rescan, Embedding: opts.Embedding, Settings: settings, DataDir: opts.DataDir, Progress: opts.Progress, Logger: opts.Logger, ConfirmAfterScan: opts.ConfirmAfterScan, Rules: r.Scanner.Rules})
	if err != nil {
		logError(ctx, opts.Logger, "watch.runner.initial_pipeline.failed", err, "repo_root", repoRoot)
		return RunnerResult{}, err
	}
	progressStart(opts.Progress, "Starting file watcher", 4)
	logInfo(ctx, opts.Logger, "watch.runner.initial_pipeline.completed", "repository_id", once.Repository.ID, "scan_run_id", once.Scan.ScanRunID, "representation_run_id", once.Representation.RepresentationRun)
	scan := once.Scan
	logInfo(ctx, opts.Logger, "watch.lsp.status", "repository_id", scan.RepositoryID, "requested", scan.LSP.Summary.Requested, "available", scan.LSP.Summary.Available, "active", scan.LSP.Summary.Active, "failed", scan.LSP.Summary.Failed, "unavailable", scan.LSP.Summary.Unavailable, "restarted", scan.LSP.Summary.Restarted, "memory_limited", scan.LSP.Summary.MemoryLimited)
	events.Publish(Event{Type: "scan.completed", RepositoryID: scan.RepositoryID, At: nowString(), Data: scan, Phase: "scan", WatcherMode: settings.Watcher, Languages: settings.Languages, Warnings: scan.Warnings})
	events.Publish(Event{Type: "lsp.status", RepositoryID: scan.RepositoryID, At: nowString(), Data: scan.LSP, Phase: "scan", WatcherMode: settings.Watcher, Languages: settings.Languages, Warnings: scan.Warnings})
	events.Publish(Event{Type: "representation.started", RepositoryID: scan.RepositoryID, At: nowString(), Phase: "represent", WatcherMode: settings.Watcher, Languages: settings.Languages, Warnings: scan.Warnings})
	repo := once.Repository
	token := randomToken()
	lock, err := r.Store.AcquireLock(ctx, repo.ID, os.Getpid(), token, LockHeartbeatTimeout)
	if err != nil {
		logError(ctx, opts.Logger, "watch.lock.acquire.failed", err, "repository_id", repo.ID)
		return RunnerResult{}, err
	}
	_ = lock
	progressAdvance(opts.Progress, "Watch lock acquired")
	logInfo(ctx, opts.Logger, "watch.lock.enabled", "repository_id", repo.ID, "pid", os.Getpid())
	sourceWatcher := newSourceWatcher(ctx, repoRoot, settings, r.Scanner.EffectiveRules, opts.Logger)
	watcherMode := sourceWatcher.Mode
	warnings := append([]string{}, sourceWatcher.Warnings...)
	progressAdvance(opts.Progress, "File watcher initialized")
	logInfo(ctx, opts.Logger, "watch.source_watcher.started", "repository_id", repo.ID, "mode", watcherMode, "warnings", len(warnings))
	events.Publish(Event{Type: "watch.started", RepositoryID: repo.ID, At: nowString(), Data: repo.JSON(), Phase: "watch", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
	events.Publish(Event{Type: "lock.enabled", RepositoryID: repo.ID, At: nowString()})
	defer func() {
		if sourceWatcher.Close != nil {
			_ = sourceWatcher.Close()
		}
		_ = r.Scanner.Close()
		_ = r.Store.ReleaseLock(context.WithoutCancel(ctx), repo.ID, token)
		logInfo(context.WithoutCancel(ctx), opts.Logger, "watch.lock.disabled", "repository_id", repo.ID)
		logInfo(context.WithoutCancel(ctx), opts.Logger, "watch.runner.stopped", "repository_id", repo.ID, "elapsed", logElapsed(started))
		events.Publish(Event{Type: "lock.disabled", RepositoryID: repo.ID, At: nowString()})
		events.Publish(Event{Type: "watch.stopped", RepositoryID: repo.ID, At: nowString()})
	}()

	rep := once.Representation
	rep.Diffs = once.Diffs
	if rep.Diffs == nil {
		rep.Diffs = []RepresentationDiff{}
	}
	events.Publish(Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep, Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
	_, _ = r.Store.ApplyGitTags(ctx, repo.ID, gitStatus)
	if blastResult, err := r.Store.ApplyBlastRadiusTags(ctx, repo.ID, scan.BlastRadiusFiles, scan.BlastRadiusRoots, gitStatus); err != nil {
		logError(ctx, opts.Logger, "watch.blast_radius_tags.failed", err, "repository_id", repo.ID)
		events.Publish(Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
	} else {
		logInfo(ctx, opts.Logger, "watch.blast_radius_tags.applied", "repository_id", repo.ID, "tags_added", blastResult.TagsAdded, "tags_removed", blastResult.TagsRemoved, "files", len(scan.BlastRadiusFiles))
	}
	if gitStatus.HeadCommit != "" {
		if err := r.createVersionForHead(ctx, repo.ID, gitStatus, rep.RepresentationHash, false, opts.Logger); err != nil {
			logError(ctx, opts.Logger, "watch.version.create.failed", err, "repository_id", repo.ID, "head", gitStatus.HeadCommit)
			events.Publish(Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
		}
	}
	progressAdvance(opts.Progress, "Version recorded")

	progressAdvance(opts.Progress, "Watch loop started")
	progressFinish(opts.Progress)
	result := RunnerResult{Repository: repo, InitialScan: scan, InitialRep: rep, GitStatus: gitStatus, Token: token}
	if opts.Ready != nil {
		select {
		case opts.Ready <- result:
			logInfo(ctx, opts.Logger, "watch.runner.ready", "repository_id", repo.ID, "elapsed", logElapsed(started), "watcher_mode", watcherMode)
		default:
			logInfo(ctx, opts.Logger, "watch.runner.ready_skipped", "repository_id", repo.ID)
		}
	}
	changeDetector := NewChangeDetector(r.Store, repo.ID, repoRoot, settings, r.Scanner.Rules, watcherMode, once.Scan, gitStatus, opts.Logger)
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	heartbeatResults := r.startLockHeartbeat(heartbeatCtx, repo.ID, token, opts, watcherMode, warnings, events)
	poll := time.NewTicker(opts.PollInterval)
	summary := time.NewTicker(opts.SummaryInterval)
	defer poll.Stop()
	defer summary.Stop()
	totalChangesProcessed := 0
	intervalChangesProcessed := 0

	debounceTimer := time.NewTimer(0)
	if !debounceTimer.Stop() {
		select {
		case <-debounceTimer.C:
		default:
		}
	}
	defer debounceTimer.Stop()
	debouncing := false
	var pendingPaths map[string]struct{}
	sourceEvents := sourceWatcher.Events.Out()
	resetDebounceTimer := func() {
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(opts.Debounce)
	}

	for {
		process := false
		pollCheck := false

		select {
		case <-ctx.Done():
			logInfo(ctx, opts.Logger, "watch.runner.context_done", "repository_id", repo.ID, "error", ctx.Err())
			return result, nil
		case <-summary.C:
			logInfo(ctx, opts.Logger, "watch.summary", "repository_id", repo.ID, "total_changes_processed", totalChangesProcessed, "interval_changes_processed", intervalChangesProcessed)
			events.Publish(Event{
				Type:         "watch.changeCounter",
				RepositoryID: repo.ID,
				At:           nowString(),
				WatcherMode:  watcherMode,
				Languages:    settings.Languages,
				Data: ChangeCounter{
					TotalChangesProcessed:    totalChangesProcessed,
					IntervalChangesProcessed: intervalChangesProcessed,
				},
			})
			intervalChangesProcessed = 0
		case heartbeat, ok := <-heartbeatResults:
			if !ok {
				heartbeatResults = nil
				continue
			}
			if heartbeat.stop {
				return result, heartbeat.err
			}
		case ev, ok := <-sourceEvents:
			if !ok {
				sourceEvents = nil
				continue
			}
			if ev.Message != "" {
				logInfo(ctx, opts.Logger, "watch.source_event.received", "repository_id", repo.ID, "path", ev.Message)
				if pendingPaths == nil {
					pendingPaths = make(map[string]struct{})
				}
				pendingPaths[ev.Message] = struct{}{}
				debouncing = true
				resetDebounceTimer()
			}
		case <-debounceTimer.C:
			debouncing = false
			process = true
		case <-poll.C:
			pollCheck = true
		}

		if pollCheck {
			if debouncing {
				continue
			}
			status, err := r.Store.LockStatus(ctx, repo.ID, token)
			if errors.Is(err, sql.ErrNoRows) {
				logInfo(ctx, opts.Logger, "watch.poll.lock_missing", "repository_id", repo.ID)
				return result, nil
			}
			if err != nil {
				logError(ctx, opts.Logger, "watch.poll.lock_status_failed", err, "repository_id", repo.ID)
				return result, err
			}
			if status == "paused" {
				logInfo(ctx, opts.Logger, "watch.poll.skipped", "repository_id", repo.ID, "reason", "paused")
				continue
			}
			if status == "stopping" {
				logInfo(ctx, opts.Logger, "watch.poll.stopping", "repository_id", repo.ID)
				return result, nil
			}
			process = true
		}

		if process {
			change, changed, err := changeDetector.Detect(ctx, changeTrigger{Poll: pollCheck, PendingPaths: pendingPaths})
			if change.ConsumePendingPaths {
				pendingPaths = nil
			}
			if err != nil || !changed {
				continue
			}
			if len(change.DeletedFiles) > 0 {
				if err := r.Store.DeleteFilesByPath(ctx, repo.ID, change.DeletedFiles); err != nil {
					logError(ctx, opts.Logger, "watch.files.delete_failed", err, "repository_id", repo.ID, "deleted_files", len(change.DeletedFiles))
					events.Publish(Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
					continue
				}
			}

			pipelineStarted := time.Now()
			logInfo(ctx, opts.Logger, "watch.change.pipeline.started", "repository_id", repo.ID, "source_changed", change.SourceChanged, "changed_files", len(change.SourceChanges), "head", change.Git.HeadCommit)
			events.Publish(Event{Type: "scan.started", RepositoryID: repo.ID, At: nowString(), Phase: "scan", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(change.SourceChanges), Warnings: warnings})

			once, err := r.RunOnce(ctx, OneShotOptions{Path: repoRoot, Files: change.TargetedFiles, FocusFiles: sourceChangeFocusFiles(change.SourceChanges), Embedding: opts.Embedding, Settings: settings, DataDir: opts.DataDir, Progress: opts.Progress, Logger: opts.Logger, Rules: r.Scanner.Rules})
			if err != nil {
				logError(ctx, opts.Logger, "watch.change.pipeline.failed", err, "elapsed", logElapsed(pipelineStarted), "repository_id", repo.ID)
				events.Publish(Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				continue
			}
			logInfo(ctx, opts.Logger, "watch.change.pipeline.completed", "elapsed", logElapsed(pipelineStarted), "repository_id", repo.ID, "scan_run_id", once.Scan.ScanRunID, "representation_run_id", once.Representation.RepresentationRun)
			scan := once.Scan
			logInfo(ctx, opts.Logger, "watch.lsp.status", "repository_id", scan.RepositoryID, "requested", scan.LSP.Summary.Requested, "available", scan.LSP.Summary.Available, "active", scan.LSP.Summary.Active, "failed", scan.LSP.Summary.Failed, "unavailable", scan.LSP.Summary.Unavailable, "restarted", scan.LSP.Summary.Restarted, "memory_limited", scan.LSP.Summary.MemoryLimited)
			eventWarnings := append(append([]string{}, warnings...), scan.Warnings...)
			events.Publish(Event{Type: "scan.completed", RepositoryID: repo.ID, At: nowString(), Data: scan, Phase: "scan", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(change.SourceChanges), Warnings: eventWarnings})
			events.Publish(Event{Type: "lsp.status", RepositoryID: repo.ID, At: nowString(), Data: scan.LSP, Phase: "scan", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(change.SourceChanges), Warnings: eventWarnings})
			events.Publish(Event{Type: "representation.started", RepositoryID: repo.ID, At: nowString(), Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(change.SourceChanges), Warnings: eventWarnings})
			rep := once.Representation
			rep.Diffs = once.Diffs
			if rep.Diffs == nil {
				rep.Diffs = []RepresentationDiff{}
			}
			events.Publish(Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep, Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(change.SourceChanges), Warnings: eventWarnings})
			tagResult, _ := r.Store.ApplyGitTags(ctx, repo.ID, change.Git)
			logInfo(ctx, opts.Logger, "watch.git_tags.applied", "repository_id", repo.ID, "tags_added", tagResult.TagsAdded, "tags_removed", tagResult.TagsRemoved)
			if blastResult, err := r.Store.ApplyBlastRadiusTags(ctx, repo.ID, scan.BlastRadiusFiles, scan.BlastRadiusRoots, change.Git); err != nil {
				logError(ctx, opts.Logger, "watch.blast_radius_tags.failed", err, "repository_id", repo.ID)
				events.Publish(Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
			} else {
				logInfo(ctx, opts.Logger, "watch.blast_radius_tags.applied", "repository_id", repo.ID, "tags_added", blastResult.TagsAdded, "tags_removed", blastResult.TagsRemoved, "files", len(scan.BlastRadiusFiles))
			}
			diffs := rep.Diffs
			var diffErr error
			if diffs == nil {
				diffs, diffErr = r.Store.BuildWatchDiffs(ctx, repo.ID, rep.RepresentationHash)
			}
			if diffErr != nil {
				logError(ctx, opts.Logger, "watch.diffs.failed", diffErr, "repository_id", repo.ID, "representation_hash", rep.RepresentationHash)
				events.Publish(Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: diffErr.Error()})
			}
			for _, sourceChange := range change.SourceChanges {
				logInfo(ctx, opts.Logger, "watch.source.changed", "repository_id", repo.ID, "path", sourceChange.Path, "change_type", sourceChange.ChangeType, "language", sourceChange.Language, "representation_changed", sourceChangeRepresentationChanged(sourceChange, diffs))
				events.Publish(Event{
					Type:         "source.changed",
					RepositoryID: repo.ID,
					At:           nowString(),
					Phase:        "watch",
					WatcherMode:  watcherMode,
					Languages:    settings.Languages,
					ChangedFiles: len(change.SourceChanges),
					Warnings:     eventWarnings,
					Data: SourceFileChangeResult{
						Change:                sourceChange,
						RepresentationChanged: sourceChangeRepresentationChanged(sourceChange, diffs),
						Representation:        rep,
						GitTags:               tagResult,
					},
				})
			}
			processed := len(change.SourceChanges)
			if processed == 0 {
				processed = 1
			}
			totalChangesProcessed += processed
			intervalChangesProcessed += processed
			result.InitialRep = rep
			events.Publish(Event{Type: "git.statusChanged", RepositoryID: repo.ID, At: nowString(), Data: change.Git})
			if change.Git.HeadCommit != "" && change.Git.HeadCommit != changeDetector.LastHead() {
				if err := r.createVersionForHead(ctx, repo.ID, change.Git, rep.RepresentationHash, !change.SourceChanged, opts.Logger); err != nil {
					logError(ctx, opts.Logger, "watch.version.create.failed", err, "repository_id", repo.ID, "head", change.Git.HeadCommit)
					events.Publish(Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				} else {
					logInfo(ctx, opts.Logger, "watch.version.created", "repository_id", repo.ID, "head", change.Git.HeadCommit, "source_changed", change.SourceChanged)
					events.Publish(Event{Type: "version.created", RepositoryID: repo.ID, At: nowString(), Data: map[string]string{"commit_hash": change.Git.HeadCommit}})
					changeDetector.MarkHeadRecorded(change.Git.HeadCommit)
				}
			}
			changeDetector.Commit(once.Scan, change)
		}
	}
}

type heartbeatResult struct {
	stop bool
	err  error
}

func (r *Runner) startLockHeartbeat(ctx context.Context, repositoryID int64, token string, opts RunnerOptions, watcherMode string, warnings []string, events WatchEventPublisher) <-chan heartbeatResult {
	results := make(chan heartbeatResult, 1)
	ticker := time.NewTicker(opts.HeartbeatInterval)
	go func() {
		defer ticker.Stop()
		defer close(results)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := r.Store.HeartbeatLock(ctx, repositoryID, token); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						logInfo(ctx, opts.Logger, "watch.heartbeat.lock_missing", "repository_id", repositoryID)
						sendHeartbeatResult(results, heartbeatResult{stop: true})
						return
					}
					logError(ctx, opts.Logger, "watch.heartbeat.failed", err, "repository_id", repositoryID)
					sendHeartbeatResult(results, heartbeatResult{stop: true, err: err})
					return
				}
				status, err := r.Store.LockStatus(ctx, repositoryID, token)
				if errors.Is(err, sql.ErrNoRows) {
					logInfo(ctx, opts.Logger, "watch.status.lock_missing", "repository_id", repositoryID)
					sendHeartbeatResult(results, heartbeatResult{stop: true})
					return
				}
				if err != nil {
					logError(ctx, opts.Logger, "watch.status.lock_status_failed", err, "repository_id", repositoryID)
					sendHeartbeatResult(results, heartbeatResult{stop: true, err: err})
					return
				}
				if status == "stopping" {
					logInfo(ctx, opts.Logger, "watch.status.stopping", "repository_id", repositoryID)
					sendHeartbeatResult(results, heartbeatResult{stop: true})
					return
				}
				if status == "paused" {
					logInfo(ctx, opts.Logger, "watch.status.paused", "repository_id", repositoryID)
					events.Publish(Event{Type: "watch.paused", RepositoryID: repositoryID, At: nowString()})
				}
				events.Publish(Event{Type: "watch.heartbeat", RepositoryID: repositoryID, At: nowString(), Phase: "watch", WatcherMode: watcherMode, Languages: opts.Settings.Languages, Warnings: warnings})
			}
		}
	}()
	return results
}

func sendHeartbeatResult(results chan<- heartbeatResult, result heartbeatResult) {
	select {
	case results <- result:
	default:
	}
}

func sourceChangeRepresentationChanged(change SourceFileChange, diffs []RepresentationDiff) bool {
	path := strings.TrimSpace(filepathToSlash(change.Path))
	if path == "" {
		return false
	}
	for _, diff := range diffs {
		if diff.OwnerType == "repository" {
			continue
		}
		for _, candidate := range representationDiffSourcePaths(diff) {
			if candidate == path || strings.HasPrefix(candidate, path+"/") || strings.HasPrefix(path, candidate+"/") {
				return true
			}
		}
	}
	return false
}

func sourceChangeFocusFiles(changes []SourceFileChange) []string {
	seen := map[string]struct{}{}
	var files []string
	for _, change := range changes {
		if change.ChangeType == string(tldgit.WorktreeDeleted) || change.ChangeType == "deleted" {
			continue
		}
		path := strings.TrimSpace(filepathToSlash(change.Path))
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func representationDiffSourcePaths(diff RepresentationDiff) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(filepathToSlash(value))
		value = strings.TrimPrefix(value, "file:")
		value = strings.TrimPrefix(value, "folder:")
		if value == "" || value == "." {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	switch diff.OwnerType {
	case "file", "folder":
		add(diff.OwnerKey)
	case "symbol":
		if path, ok := filePathFromStableKey(diff.OwnerKey); ok {
			add(path)
		}
	default:
		if strings.HasPrefix(diff.OwnerKey, "file:") || strings.HasPrefix(diff.OwnerKey, "folder:") {
			add(diff.OwnerKey)
		}
	}
	return out
}

func gitStatusSnapshot(repoRoot string) (GitStatus, error) {
	status, err := tldgit.StatusSnapshot(repoRoot)
	return GitStatus{
		Branch:      status.Branch,
		HeadCommit:  status.HeadCommit,
		HeadMessage: status.HeadMessage,
		RemoteURL:   status.RemoteURL,
		Staged:      status.Staged,
		Unstaged:    status.Unstaged,
		Untracked:   status.Untracked,
		Deleted:     status.Deleted,
	}, err
}

func gitStatusClean(status GitStatus) bool {
	return len(status.Staged) == 0 && len(status.Unstaged) == 0 && len(status.Untracked) == 0 && len(status.Deleted) == 0
}

func gitStatusFingerprint(status GitStatus) string {
	parts := []string{status.Branch, status.HeadCommit, status.HeadMessage, status.RemoteURL}
	appendPaths := func(kind string, paths []string) {
		sorted := append([]string(nil), paths...)
		sort.Strings(sorted)
		for _, path := range sorted {
			parts = append(parts, kind+":"+path)
		}
	}
	appendPaths("staged", status.Staged)
	appendPaths("unstaged", status.Unstaged)
	appendPaths("untracked", status.Untracked)
	appendPaths("deleted", status.Deleted)
	return hashString(strings.Join(parts, "\n"))
}

func sourceFileSnapshot(repoRoot string, settings Settings, rules *ignore.Rules, previous map[string]string) map[string]string {
	files := map[string]string{}
	settings = NormalizeSettings(settings)
	allowed := map[string]struct{}{}
	for _, language := range settings.Languages {
		allowed[language] = struct{}{}
	}
	if rules == nil {
		rules = &ignore.Rules{}
	}
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (rules.ShouldIgnorePath(rel) || isHiddenBuildOutput(d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		language, parseable, ok := watchedFileLanguage(path)
		if !ok || (parseable && !languageAllowed(language, allowed)) || rules.ShouldIgnorePath(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		meta := language + ":" + info.ModTime().UTC().Format(time.RFC3339Nano) + ":" + fmt.Sprint(info.Size())
		if previousValue, ok := previous[rel]; ok && strings.HasPrefix(previousValue, meta+":") {
			files[rel] = previousValue
			return nil
		}
		files[rel] = meta + ":" + sourceSnapshotFileHash(path, info.Size())
		return nil
	})
	return files
}

func sourceSnapshotFileHash(path string, size int64) string {
	if size > maxSourceFileBytes {
		return "oversized"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "error:" + err.Error()
	}
	return hashBytes(data)
}

func sourceFileFingerprint(files map[string]string) string {
	h := hashString("")
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		h = hashString(h + path + files[path])
	}
	return h
}

func sourceFileContentFingerprint(files map[string]string) string {
	h := hashString("")
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		h = hashString(h + path + sourceSnapshotContentHash(files[path]))
	}
	return h
}

func sourceSnapshotContentHash(value string) string {
	idx := strings.LastIndex(value, ":")
	if idx < 0 {
		return value
	}
	return value[idx+1:]
}

func diffSourceFileSnapshots(before, after map[string]string) []SourceFileChange {
	seen := map[string]struct{}{}
	var changes []SourceFileChange
	for path, next := range after {
		seen[path] = struct{}{}
		prev, ok := before[path]
		switch {
		case !ok:
			changes = append(changes, SourceFileChange{Path: path, ChangeType: "added", Language: sourceSnapshotLanguage(next)})
		case prev != next:
			changes = append(changes, SourceFileChange{Path: path, ChangeType: "modified", Language: sourceSnapshotLanguage(next)})
		}
	}
	for path := range before {
		if _, ok := seen[path]; !ok {
			changes = append(changes, SourceFileChange{Path: path, ChangeType: "deleted", Language: sourceSnapshotLanguage(before[path])})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path == changes[j].Path {
			return changes[i].ChangeType < changes[j].ChangeType
		}
		return changes[i].Path < changes[j].Path
	})
	return changes
}

func sourceChangesFromGit(repoRoot string, logger EventLogger) []SourceFileChange {
	changes, err := tldgit.WorktreeChangesAgainstHead(repoRoot)
	if err != nil {
		logError(context.TODO(), logger, "watch.git.changes.failed", err)
		return nil
	}
	out := make([]SourceFileChange, 0, len(changes))
	for path, change := range changes {
		changeType := string(change)
		if changeType == "" {
			changeType = string(tldgit.WorktreeUpdated)
		}
		out = append(out, SourceFileChange{Path: path, ChangeType: changeType})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].ChangeType < out[j].ChangeType
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func sourceChangesSinceLatestWatchVersion(ctx context.Context, store *Store, repositoryID int64, repoRoot string, logger EventLogger) []SourceFileChange {
	changes, err := gitChangesSinceLatestWatchVersion(ctx, store, repositoryID, repoRoot)
	if err != nil {
		logError(ctx, logger, "watch.git.committed_changes.failed", err, "repository_id", repositoryID)
		return nil
	}
	return sourceFileChangesFromGitChanges(changes)
}

func gitChangesSinceLatestWatchVersion(ctx context.Context, store *Store, repositoryID int64, repoRoot string) (map[string]tldgit.WorktreeChange, error) {
	if store == nil {
		return nil, nil
	}
	latest, found, err := store.LatestWatchVersion(ctx, repositoryID)
	if err != nil || !found || strings.TrimSpace(latest.CommitHash) == "" {
		return nil, err
	}
	current, err := tldgit.DetectHeadCommit(repoRoot)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(current) == "" || current == latest.CommitHash {
		return nil, nil
	}
	return tldgit.FileChangesSince(repoRoot, latest.CommitHash)
}

func sourceFileChangesFromGitChanges(changes map[string]tldgit.WorktreeChange) []SourceFileChange {
	out := make([]SourceFileChange, 0, len(changes))
	for path, change := range changes {
		changeType := string(change)
		if changeType == "" {
			changeType = string(tldgit.WorktreeUpdated)
		}
		language, _, _ := watchedFileLanguage(path)
		out = append(out, SourceFileChange{Path: path, ChangeType: changeType, Language: language})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].ChangeType < out[j].ChangeType
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func mergeSourceFileChanges(groups ...[]SourceFileChange) []SourceFileChange {
	merged := map[string]SourceFileChange{}
	for _, group := range groups {
		for _, change := range group {
			path := filepathToSlash(change.Path)
			if path == "" {
				continue
			}
			change.Path = path
			if existing, ok := merged[path]; ok {
				if existing.ChangeType == string(tldgit.WorktreeDeleted) || existing.ChangeType == "deleted" {
					continue
				}
				if change.ChangeType == string(tldgit.WorktreeDeleted) || change.ChangeType == "deleted" || existing.Language == "" {
					merged[path] = change
				}
				continue
			}
			merged[path] = change
		}
	}
	out := make([]SourceFileChange, 0, len(merged))
	for _, change := range merged {
		out = append(out, change)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].ChangeType < out[j].ChangeType
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func sourceSnapshotLanguage(value string) string {
	if idx := strings.Index(value, ":"); idx > 0 {
		return value[:idx]
	}
	return ""
}

func sourceFileChangeLanguage(path string) string {
	language, _, ok := watchedFileLanguage(path)
	if !ok {
		return ""
	}
	return language
}

func randomToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
