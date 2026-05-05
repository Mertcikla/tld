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
	Progress          ProgressSink
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
	r.Scanner.Settings = settings

	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return RunnerResult{}, err
	}
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		return RunnerResult{}, fmt.Errorf("%s is not inside a git repository: %w", opts.Path, err)
	}

	gitStatus, _ := gitStatusSnapshot(repoRoot)
	emit(opts.Events, Event{Type: "scan.started", At: nowString(), Phase: "scan", WatcherMode: settings.Watcher, Languages: settings.Languages})
	scan, err := r.Scanner.ScanWithOptions(ctx, repoRoot, ScanOptions{Force: opts.Rescan})
	if err != nil {
		return RunnerResult{}, err
	}
	emit(opts.Events, Event{Type: "scan.completed", RepositoryID: scan.RepositoryID, At: nowString(), Data: scan, Phase: "scan", WatcherMode: settings.Watcher, Languages: settings.Languages, Warnings: scan.Warnings})
	repo, err := r.Store.Repository(ctx, scan.RepositoryID)
	if err != nil {
		return RunnerResult{}, err
	}
	token := randomToken()
	lock, err := r.Store.AcquireLock(ctx, repo.ID, os.Getpid(), token, LockHeartbeatTimeout)
	if err != nil {
		return RunnerResult{}, err
	}
	_ = lock
	sourceWatcher := newSourceWatcher(ctx, repoRoot, settings, r.Scanner.EffectiveRules)
	watcherMode := sourceWatcher.Mode
	warnings := append([]string{}, sourceWatcher.Warnings...)
	emit(opts.Events, Event{Type: "watch.started", RepositoryID: repo.ID, At: nowString(), Data: repo.JSON(), Phase: "watch", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
	emit(opts.Events, Event{Type: "lock.enabled", RepositoryID: repo.ID, At: nowString()})
	defer func() {
		_ = r.Store.ReleaseLock(context.Background(), repo.ID, token)
		emit(opts.Events, Event{Type: "lock.disabled", RepositoryID: repo.ID, At: nowString()})
		emit(opts.Events, Event{Type: "watch.stopped", RepositoryID: repo.ID, At: nowString()})
	}()

	emit(opts.Events, Event{Type: "representation.started", RepositoryID: repo.ID, At: nowString(), Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
	rep, err := r.Representer.Represent(ctx, repo.ID, RepresentRequest{Embedding: opts.Embedding, Thresholds: settings.Thresholds, Progress: opts.Progress})
	if err != nil {
		return RunnerResult{}, err
	}
	emit(opts.Events, Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep, Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
	_, _ = r.Store.ApplyGitTags(ctx, repo.ID, gitStatus)
	if gitStatus.HeadCommit != "" {
		_ = r.createVersionForHead(ctx, repo.ID, gitStatus, rep.RepresentationHash, false)
	}

	result := RunnerResult{Repository: repo, InitialScan: scan, InitialRep: rep, GitStatus: gitStatus, Token: token}
	if opts.Ready != nil {
		select {
		case opts.Ready <- result:
		default:
		}
	}
	lastSourceSnapshot := sourceFileSnapshot(repoRoot, settings, r.Scanner.Rules)
	lastFingerprint := sourceFileFingerprint(lastSourceSnapshot)
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
			return result, nil
		case <-summary.C:
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
					return result, nil
				}
				return result, err
			}
			status, err := r.Store.LockStatus(ctx, repo.ID, token)
			if errors.Is(err, sql.ErrNoRows) {
				return result, nil
			}
			if err == nil && status == "stopping" {
				return result, nil
			}
			if err == nil && status == "paused" {
				emit(opts.Events, Event{Type: "watch.paused", RepositoryID: repo.ID, At: nowString()})
			}
			emit(opts.Events, Event{Type: "watch.heartbeat", RepositoryID: repo.ID, At: nowString(), Phase: "watch", WatcherMode: watcherMode, Languages: settings.Languages, Warnings: warnings})
		case _, ok := <-sourceWatcher.Events:
			if ok {
				poll.Reset(time.Millisecond)
			}
		case <-poll.C:
			status, err := r.Store.LockStatus(ctx, repo.ID, token)
			if errors.Is(err, sql.ErrNoRows) {
				return result, nil
			}
			if err == nil && status == "paused" {
				continue
			}
			if err == nil && status == "stopping" {
				return result, nil
			}
			nextSourceSnapshot := sourceFileSnapshot(repoRoot, settings, r.Scanner.Rules)
			nextFingerprint := sourceFileFingerprint(nextSourceSnapshot)
			nextGit, _ := gitStatusSnapshot(repoRoot)
			nextGitFingerprint := gitStatusFingerprint(nextGit)
			if nextFingerprint == lastFingerprint && nextGit.HeadCommit == lastHead && nextGitFingerprint == lastGitFingerprint {
				continue
			}
			time.Sleep(opts.Debounce)
			stableSourceSnapshot := sourceFileSnapshot(repoRoot, settings, r.Scanner.Rules)
			sourceChanged := sourceFileFingerprint(stableSourceSnapshot) != lastFingerprint
			nextGit, _ = gitStatusSnapshot(repoRoot)
			nextGitFingerprint = gitStatusFingerprint(nextGit)
			sourceChanges := diffSourceFileSnapshots(lastSourceSnapshot, stableSourceSnapshot)
			emit(opts.Events, Event{Type: "scan.started", RepositoryID: repo.ID, At: nowString(), Phase: "scan", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: warnings})
			scan, err := r.Scanner.ScanWithOptions(ctx, repoRoot, ScanOptions{})
			if err != nil {
				emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				continue
			}
			eventWarnings := append(append([]string{}, warnings...), scan.Warnings...)
			emit(opts.Events, Event{Type: "scan.completed", RepositoryID: repo.ID, At: nowString(), Data: scan, Phase: "scan", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: eventWarnings})
			emit(opts.Events, Event{Type: "representation.started", RepositoryID: repo.ID, At: nowString(), Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: eventWarnings})
			rep, err := r.Representer.Represent(ctx, repo.ID, RepresentRequest{Embedding: opts.Embedding, Thresholds: settings.Thresholds, Progress: opts.Progress})
			if err != nil {
				emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				continue
			}
			emit(opts.Events, Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep, Phase: "represent", WatcherMode: watcherMode, Languages: settings.Languages, ChangedFiles: len(sourceChanges), Warnings: eventWarnings})
			tagResult, _ := r.Store.ApplyGitTags(ctx, repo.ID, nextGit)
			representationChanged := rep.RepresentationHash != result.InitialRep.RepresentationHash ||
				rep.ElementsCreated > 0 ||
				rep.ElementsUpdated > 0 ||
				rep.ConnectorsCreated > 0 ||
				rep.ConnectorsUpdated > 0 ||
				rep.ViewsCreated > 0 ||
				tagResult.TagsAdded > 0 ||
				tagResult.TagsRemoved > 0
			for _, change := range sourceChanges {
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
						RepresentationChanged: representationChanged,
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
					emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				} else {
					emit(opts.Events, Event{Type: "version.created", RepositoryID: repo.ID, At: nowString(), Data: map[string]string{"commit_hash": nextGit.HeadCommit}})
				}
				lastHead = nextGit.HeadCommit
			}
			lastSourceSnapshot = stableSourceSnapshot
			lastFingerprint = sourceFileFingerprint(stableSourceSnapshot)
			lastGitFingerprint = nextGitFingerprint
		}
	}
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
