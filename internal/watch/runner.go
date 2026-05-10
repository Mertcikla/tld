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

	tldgit "github.com/mertcikla/tld/internal/git"
	"github.com/mertcikla/tld/internal/ignore"
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
	Events            chan<- Event
	Ready             chan<- RunnerResult
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
	if r.Representer == nil {
		r.Representer = NewRepresenter(r.Store)
	}
	if opts.Path == "" {
		opts.Path = "."
	}
	settings := NormalizeSettings(opts.Settings)
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

	gitStatus, _ := gitStatusSnapshot(repoRoot)
	emit(opts.Events, Event{Type: "scan.started", At: nowString(), Phase: "scan", WatcherMode: settings.Watcher, Languages: settings.Languages})
	once, err := r.RunOnce(ctx, OneShotOptions{Path: repoRoot, Rescan: opts.Rescan, Embedding: opts.Embedding, Settings: settings, DataDir: opts.DataDir, Progress: opts.Progress, Logger: opts.Logger})
	if err != nil {
		logError(ctx, opts.Logger, "watch.runner.initial_pipeline.failed", err, "repo_root", repoRoot)
		return RunnerResult{}, err
	}
	logInfo(ctx, opts.Logger, "watch.runner.initial_pipeline.completed", "repository_id", once.Repository.ID, "scan_run_id", once.Scan.ScanRunID, "representation_run_id", once.Representation.RepresentationRun)
	scan := once.Scan
	emit(opts.Events, Event{Type: "scan.completed", RepositoryID: scan.RepositoryID, At: nowString(), Data: scan, Phase: "scan", WatcherMode: settings.Watcher, Languages: settings.Languages, Warnings: scan.Warnings})
	emit(opts.Events, Event{Type: "representation.started", RepositoryID: scan.RepositoryID, At: nowString(), Phase: "represent", WatcherMode: settings.Watcher, Languages: settings.Languages, Warnings: scan.Warnings})
	repo := once.Repository
	token := randomToken()
	lock, err := r.Store.AcquireLock(ctx, repo.ID, os.Getpid(), token, LockHeartbeatTimeout)
	if err != nil {
		logError(ctx, opts.Logger, "watch.lock.acquire.failed", err, "repository_id", repo.ID)
		return RunnerResult{}, err
	}
	_ = lock
	logInfo(ctx, opts.Logger, "watch.lock.enabled", "repository_id", repo.ID, "pid", os.Getpid())
	sourceWatcher := newSourceWatcher(ctx, repoRoot, settings, r.Scanner.EffectiveRules)
	watcherMode := sourceWatcher.Mode
	warnings := append([]string{}, sourceWatcher.Warnings...)
	logInfo(ctx, opts.Logger, "watch.source_watcher.started", "repository_id", repo.ID, "mode", watcherMode, "warnings", len(warnings))
	emit(opts.Events, Event{Type: "watch.started", RepositoryID: repo.ID, At: nowString(), Data: repo.JSON(), Phase: "watch", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
	emit(opts.Events, Event{Type: "lock.enabled", RepositoryID: repo.ID, At: nowString()})
	defer func() {
		_ = r.Scanner.Close()
		_ = r.Store.ReleaseLock(context.Background(), repo.ID, token)
		logInfo(context.Background(), opts.Logger, "watch.lock.disabled", "repository_id", repo.ID)
		logInfo(context.Background(), opts.Logger, "watch.runner.stopped", "repository_id", repo.ID, "elapsed", logElapsed(started))
		emit(opts.Events, Event{Type: "lock.disabled", RepositoryID: repo.ID, At: nowString()})
		emit(opts.Events, Event{Type: "watch.stopped", RepositoryID: repo.ID, At: nowString()})
	}()

	rep := once.Representation
	emit(opts.Events, Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep, Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
	_, _ = r.Store.ApplyGitTags(ctx, repo.ID, gitStatus)
	if gitStatus.HeadCommit != "" {
		_ = r.createVersionForHead(ctx, repo.ID, gitStatus, rep.RepresentationHash, false)
	}

	result := RunnerResult{Repository: repo, InitialScan: scan, InitialRep: rep, GitStatus: gitStatus, Token: token}
	if opts.Ready != nil {
		select {
		case opts.Ready <- result:
			logInfo(ctx, opts.Logger, "watch.runner.ready", "repository_id", repo.ID, "elapsed", logElapsed(started), "watcher_mode", watcherMode)
		default:
			logInfo(ctx, opts.Logger, "watch.runner.ready_skipped", "repository_id", repo.ID)
		}
	}
	limitedMode := once.Scan.Mode == "limited"
	lastSourceSnapshot := map[string]string{}
	lastFingerprint := ""
	if !limitedMode {
		lastSourceSnapshot = sourceFileSnapshot(repoRoot, settings, r.Scanner.Rules)
		lastFingerprint = sourceFileFingerprint(lastSourceSnapshot)
	}
	lastHead := gitStatus.HeadCommit
	lastGitFingerprint := gitStatusFingerprint(gitStatus)
	heartbeat := time.NewTicker(opts.HeartbeatInterval)
	poll := time.NewTicker(opts.PollInterval)
	summary := time.NewTicker(opts.SummaryInterval)
	defer heartbeat.Stop()
	defer poll.Stop()
	defer summary.Stop()
	totalChangesProcessed := 0
	intervalChangesProcessed := 0

	for {
		select {
		case <-ctx.Done():
			logInfo(ctx, opts.Logger, "watch.runner.context_done", "repository_id", repo.ID, "error", ctx.Err())
			return result, nil
		case <-summary.C:
			logInfo(ctx, opts.Logger, "watch.summary", "repository_id", repo.ID, "total_changes_processed", totalChangesProcessed, "interval_changes_processed", intervalChangesProcessed)
			emit(opts.Events, Event{
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
		case <-heartbeat.C:
			if _, err := r.Store.HeartbeatLock(ctx, repo.ID, token); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					logInfo(ctx, opts.Logger, "watch.heartbeat.lock_missing", "repository_id", repo.ID)
					return result, nil
				}
				logError(ctx, opts.Logger, "watch.heartbeat.failed", err, "repository_id", repo.ID)
				return result, err
			}
			status, err := r.Store.LockStatus(ctx, repo.ID, token)
			if errors.Is(err, sql.ErrNoRows) {
				logInfo(ctx, opts.Logger, "watch.status.lock_missing", "repository_id", repo.ID)
				return result, nil
			}
			if err != nil {
				logError(ctx, opts.Logger, "watch.status.lock_status_failed", err, "repository_id", repo.ID)
				return result, err
			}
			if status == "stopping" {
				logInfo(ctx, opts.Logger, "watch.status.stopping", "repository_id", repo.ID)
				return result, nil
			}
			if status == "paused" {
				logInfo(ctx, opts.Logger, "watch.status.paused", "repository_id", repo.ID)
				emit(opts.Events, Event{Type: "watch.paused", RepositoryID: repo.ID, At: nowString()})
			}
			emit(opts.Events, Event{Type: "watch.heartbeat", RepositoryID: repo.ID, At: nowString(), Phase: "watch", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
		case _, ok := <-sourceWatcher.Events:
			if ok {
				logInfo(ctx, opts.Logger, "watch.source_event.received", "repository_id", repo.ID)
				poll.Reset(time.Millisecond)
			}
		case <-poll.C:
			poll.Reset(opts.PollInterval)
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
			nextGit, _ := gitStatusSnapshot(repoRoot)
			nextGitFingerprint := gitStatusFingerprint(nextGit)
			nextFingerprint := ""
			if !limitedMode {
				nextSourceSnapshot := sourceFileSnapshot(repoRoot, settings, r.Scanner.Rules)
				nextFingerprint = sourceFileFingerprint(nextSourceSnapshot)
				if nextFingerprint == lastFingerprint && nextGit.HeadCommit == lastHead && nextGitFingerprint == lastGitFingerprint {
					continue
				}
			} else if nextGit.HeadCommit == lastHead && nextGitFingerprint == lastGitFingerprint {
				continue
			}
			logInfo(ctx, opts.Logger, "watch.change.detected", "repository_id", repo.ID, "limited_mode", limitedMode, "head", nextGit.HeadCommit, "source_fingerprint_changed", nextFingerprint != lastFingerprint, "git_fingerprint_changed", nextGitFingerprint != lastGitFingerprint)
			time.Sleep(opts.Debounce)
			stableSourceSnapshot := map[string]string{}
			sourceChanged := false
			if !limitedMode {
				stableSourceSnapshot = sourceFileSnapshot(repoRoot, settings, r.Scanner.Rules)
				sourceChanged = sourceFileFingerprint(stableSourceSnapshot) != lastFingerprint
			}
			nextGit, _ = gitStatusSnapshot(repoRoot)
			nextGitFingerprint = gitStatusFingerprint(nextGit)
			var sourceChanges []SourceFileChange
			if limitedMode {
				sourceChanges = sourceChangesFromGit(repoRoot)
				sourceChanged = len(sourceChanges) > 0
			} else {
				sourceChanges = diffSourceFileSnapshots(lastSourceSnapshot, stableSourceSnapshot)
			}
			pipelineStarted := time.Now()
			logInfo(ctx, opts.Logger, "watch.change.pipeline.started", "repository_id", repo.ID, "source_changed", sourceChanged, "changed_files", len(sourceChanges), "head", nextGit.HeadCommit)
			emit(opts.Events, Event{Type: "scan.started", RepositoryID: repo.ID, At: nowString(), Phase: "scan", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: warnings})
			once, err := r.RunOnce(ctx, OneShotOptions{Path: repoRoot, Embedding: opts.Embedding, Settings: settings, DataDir: opts.DataDir, Progress: opts.Progress, Logger: opts.Logger})
			if err != nil {
				logError(ctx, opts.Logger, "watch.change.pipeline.failed", err, "elapsed", logElapsed(pipelineStarted), "repository_id", repo.ID)
				emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				continue
			}
			logInfo(ctx, opts.Logger, "watch.change.pipeline.completed", "elapsed", logElapsed(pipelineStarted), "repository_id", repo.ID, "scan_run_id", once.Scan.ScanRunID, "representation_run_id", once.Representation.RepresentationRun)
			scan := once.Scan
			eventWarnings := append(append([]string{}, warnings...), scan.Warnings...)
			emit(opts.Events, Event{Type: "scan.completed", RepositoryID: repo.ID, At: nowString(), Data: scan, Phase: "scan", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: eventWarnings})
			emit(opts.Events, Event{Type: "representation.started", RepositoryID: repo.ID, At: nowString(), Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: eventWarnings})
			rep := once.Representation
			emit(opts.Events, Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep, Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: eventWarnings})
			tagResult, _ := r.Store.ApplyGitTags(ctx, repo.ID, nextGit)
			logInfo(ctx, opts.Logger, "watch.git_tags.applied", "repository_id", repo.ID, "tags_added", tagResult.TagsAdded, "tags_removed", tagResult.TagsRemoved)
			diffs, diffErr := r.Store.BuildWatchDiffs(ctx, repo.ID, rep.RepresentationHash)
			if diffErr != nil {
				logError(ctx, opts.Logger, "watch.diffs.failed", diffErr, "repository_id", repo.ID, "representation_hash", rep.RepresentationHash)
				emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: diffErr.Error()})
			}
			for _, change := range sourceChanges {
				logInfo(ctx, opts.Logger, "watch.source.changed", "repository_id", repo.ID, "path", change.Path, "change_type", change.ChangeType, "language", change.Language, "representation_changed", sourceChangeRepresentationChanged(change, diffs))
				emit(opts.Events, Event{
					Type:         "source.changed",
					RepositoryID: repo.ID,
					At:           nowString(),
					Phase:        "watch",
					WatcherMode:  watcherMode,
					Languages:    settings.Languages,
					ChangedFiles: len(sourceChanges),
					Warnings:     eventWarnings,
					Data: SourceFileChangeResult{
						Change:                change,
						RepresentationChanged: sourceChangeRepresentationChanged(change, diffs),
						Representation:        rep,
						GitTags:               tagResult,
					},
				})
			}
			processed := len(sourceChanges)
			if processed == 0 {
				processed = 1
			}
			totalChangesProcessed += processed
			intervalChangesProcessed += processed
			result.InitialRep = rep
			emit(opts.Events, Event{Type: "git.statusChanged", RepositoryID: repo.ID, At: nowString(), Data: nextGit})
			if nextGit.HeadCommit != "" && nextGit.HeadCommit != lastHead {
				if err := r.createVersionForHead(ctx, repo.ID, nextGit, rep.RepresentationHash, !sourceChanged); err != nil {
					logError(ctx, opts.Logger, "watch.version.create.failed", err, "repository_id", repo.ID, "head", nextGit.HeadCommit)
					emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				} else {
					logInfo(ctx, opts.Logger, "watch.version.created", "repository_id", repo.ID, "head", nextGit.HeadCommit, "source_changed", sourceChanged)
					emit(opts.Events, Event{Type: "version.created", RepositoryID: repo.ID, At: nowString(), Data: map[string]string{"commit_hash": nextGit.HeadCommit}})
				}
				lastHead = nextGit.HeadCommit
			}
			limitedMode = once.Scan.Mode == "limited"
			if !limitedMode {
				lastSourceSnapshot = stableSourceSnapshot
				lastFingerprint = sourceFileFingerprint(stableSourceSnapshot)
			}
			lastGitFingerprint = nextGitFingerprint
		}
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

func (r *Runner) createVersionForHead(ctx context.Context, repositoryID int64, status GitStatus, representationHash string, baselineOnly bool) error {
	if gitStatusClean(status) {
		baselineOnly = true
		if err := r.Store.PruneDeletedMaterializedResources(ctx, repositoryID); err != nil {
			return err
		}
	}
	latest, found, err := r.Store.LatestWatchVersion(ctx, repositoryID)
	if err != nil {
		return err
	}
	if found && latest.CommitHash == status.HeadCommit && latest.RepresentationHash == representationHash {
		return nil
	}
	views, elements, connectors, err := r.Store.WorkspaceResourceCounts(ctx)
	if err != nil {
		return err
	}
	description := strings.TrimSpace(status.HeadMessage)
	if description == "" {
		description = "tld watch " + shortHash(status.HeadCommit)
	}
	workspaceVersionID, err := r.Store.CreateWorkspaceVersion(ctx, status.HeadCommit, "watch", nil, views, elements, connectors, &description, &representationHash)
	if err != nil && !strings.Contains(err.Error(), "constraint failed") {
		return err
	}
	var workspaceID *int64
	if err == nil {
		workspaceID = &workspaceVersionID
	}
	parent := ""
	if repo, err := r.Store.Repository(ctx, repositoryID); err == nil {
		parent, _ = tldgit.DetectParentCommit(repo.RepoRoot)
	}
	if parent == "" && found {
		parent = latest.CommitHash
	}
	var diffs []RepresentationDiff
	if !baselineOnly {
		diffs, err = r.Store.BuildWatchDiffs(ctx, repositoryID, representationHash)
		if err != nil {
			return err
		}
	}
	_, err = r.Store.CreateWatchVersion(ctx, repositoryID, status.HeadCommit, strings.TrimSpace(status.HeadMessage), parent, status.Branch, representationHash, workspaceID, diffs)
	return err
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

func sourceFileSnapshot(repoRoot string, settings Settings, rules *ignore.Rules) map[string]string {
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
		files[rel] = language + ":" + info.ModTime().UTC().Format(time.RFC3339Nano) + ":" + fmt.Sprint(info.Size())
		return nil
	})
	return files
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

func sourceChangesFromGit(repoRoot string) []SourceFileChange {
	changes, err := tldgit.WorktreeChangesAgainstHead(repoRoot)
	if err != nil {
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

func sourceSnapshotLanguage(value string) string {
	if idx := strings.Index(value, ":"); idx > 0 {
		return value[:idx]
	}
	return ""
}

func randomToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

func emit(ch chan<- Event, event Event) {
	if ch == nil {
		return
	}
	select {
	case ch <- event:
	default:
	}
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
