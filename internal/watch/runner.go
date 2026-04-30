package watch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tldgit "github.com/mertcikla/tld/internal/git"
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
	if opts.PollInterval <= 0 {
		opts.PollInterval = time.Second
	}
	if opts.Debounce <= 0 {
		opts.Debounce = 500 * time.Millisecond
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = 2 * time.Second
	}
	if opts.SummaryInterval <= 0 {
		opts.SummaryInterval = time.Minute
	}

	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return RunnerResult{}, err
	}
	repoRoot, err := tldgit.RepoRoot(absPath)
	if err != nil {
		return RunnerResult{}, fmt.Errorf("%s is not inside a git repository: %w", opts.Path, err)
	}

	gitStatus, _ := gitStatusSnapshot(repoRoot)
	emit(opts.Events, Event{Type: "scan.started", At: nowString()})
	scan, err := r.Scanner.Scan(ctx, repoRoot)
	if err != nil {
		return RunnerResult{}, err
	}
	emit(opts.Events, Event{Type: "scan.completed", RepositoryID: scan.RepositoryID, At: nowString(), Data: scan})
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
	emit(opts.Events, Event{Type: "watch.started", RepositoryID: repo.ID, At: nowString(), Data: repo.JSON()})
	emit(opts.Events, Event{Type: "lock.enabled", RepositoryID: repo.ID, At: nowString()})
	defer func() {
		_ = r.Store.ReleaseLock(context.Background(), repo.ID, token)
		emit(opts.Events, Event{Type: "lock.disabled", RepositoryID: repo.ID, At: nowString()})
		emit(opts.Events, Event{Type: "watch.stopped", RepositoryID: repo.ID, At: nowString()})
	}()

	emit(opts.Events, Event{Type: "representation.started", RepositoryID: repo.ID, At: nowString()})
	rep, err := r.Representer.Represent(ctx, repo.ID, RepresentRequest{Embedding: opts.Embedding, Progress: opts.Progress})
	if err != nil {
		return RunnerResult{}, err
	}
	emit(opts.Events, Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep})
	_, _ = r.Store.ApplyGitTags(ctx, repo.ID, gitStatus)
	if gitStatus.HeadCommit != "" {
		_ = r.createVersionForHead(ctx, repo.ID, gitStatus, rep.RepresentationHash)
	}

	result := RunnerResult{Repository: repo, InitialScan: scan, InitialRep: rep, GitStatus: gitStatus, Token: token}
	if opts.Ready != nil {
		select {
		case opts.Ready <- result:
		default:
		}
	}
	lastSourceSnapshot := sourceFileSnapshot(repoRoot)
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
				Data: ChangeCounter{
					TotalChangesProcessed:    totalChangesProcessed,
					IntervalChangesProcessed: intervalChangesProcessed,
				},
			})
			intervalChangesProcessed = 0
		case <-heartbeat.C:
			if _, err := r.Store.HeartbeatLock(ctx, repo.ID, token); err != nil {
				return result, err
			}
			status, err := r.Store.LockStatus(ctx, repo.ID, token)
			if err == nil && status == "stopping" {
				return result, nil
			}
			if err == nil && status == "paused" {
				emit(opts.Events, Event{Type: "watch.paused", RepositoryID: repo.ID, At: nowString()})
			}
			emit(opts.Events, Event{Type: "watch.heartbeat", RepositoryID: repo.ID, At: nowString()})
		case <-poll.C:
			status, err := r.Store.LockStatus(ctx, repo.ID, token)
			if err == nil && status == "paused" {
				continue
			}
			if err == nil && status == "stopping" {
				return result, nil
			}
			nextSourceSnapshot := sourceFileSnapshot(repoRoot)
			nextFingerprint := sourceFileFingerprint(nextSourceSnapshot)
			nextGit, _ := gitStatusSnapshot(repoRoot)
			nextGitFingerprint := gitStatusFingerprint(nextGit)
			if nextFingerprint == lastFingerprint && nextGit.HeadCommit == lastHead && nextGitFingerprint == lastGitFingerprint {
				continue
			}
			time.Sleep(opts.Debounce)
			stableSourceSnapshot := sourceFileSnapshot(repoRoot)
			nextGit, _ = gitStatusSnapshot(repoRoot)
			nextGitFingerprint = gitStatusFingerprint(nextGit)
			sourceChanges := diffSourceFileSnapshots(lastSourceSnapshot, stableSourceSnapshot)
			emit(opts.Events, Event{Type: "scan.started", RepositoryID: repo.ID, At: nowString()})
			scan, err := r.Scanner.Scan(ctx, repoRoot)
			if err != nil {
				emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				continue
			}
			emit(opts.Events, Event{Type: "scan.completed", RepositoryID: repo.ID, At: nowString(), Data: scan})
			emit(opts.Events, Event{Type: "representation.started", RepositoryID: repo.ID, At: nowString()})
			rep, err := r.Representer.Represent(ctx, repo.ID, RepresentRequest{Embedding: opts.Embedding, Progress: opts.Progress})
			if err != nil {
				emit(opts.Events, Event{Type: "watch.error", RepositoryID: repo.ID, At: nowString(), Message: err.Error()})
				continue
			}
			emit(opts.Events, Event{Type: "representation.updated", RepositoryID: repo.ID, At: nowString(), Data: rep})
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
				if err := r.createVersionForHead(ctx, repo.ID, nextGit, rep.RepresentationHash); err != nil {
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

func (r *Runner) createVersionForHead(ctx context.Context, repositoryID int64, status GitStatus, representationHash string) error {
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
	description := "tld watch " + shortHash(status.HeadCommit)
	workspaceVersionID, err := r.Store.CreateWorkspaceVersion(ctx, status.HeadCommit, "watch", nil, views, elements, connectors, &description, &representationHash)
	if err != nil && !strings.Contains(err.Error(), "constraint failed") {
		return err
	}
	var workspaceID *int64
	if err == nil {
		workspaceID = &workspaceVersionID
	}
	changeType := "updated"
	if !found {
		changeType = "added"
	}
	summary := "Representation " + changeType + " for " + shortHash(status.HeadCommit)
	diff := RepresentationDiff{OwnerType: "repository", OwnerKey: fmt.Sprintf("%d", repositoryID), ChangeType: changeType, AfterHash: &representationHash, Summary: &summary}
	parent := ""
	if found {
		parent = latest.CommitHash
		diff.BeforeHash = &latest.RepresentationHash
	}
	_, err = r.Store.CreateWatchVersion(ctx, repositoryID, status.HeadCommit, parent, status.Branch, representationHash, workspaceID, []RepresentationDiff{diff})
	return err
}

func gitStatusSnapshot(repoRoot string) (GitStatus, error) {
	status, err := tldgit.StatusSnapshot(repoRoot)
	return GitStatus{
		Branch:     status.Branch,
		HeadCommit: status.HeadCommit,
		RemoteURL:  status.RemoteURL,
		Staged:     status.Staged,
		Unstaged:   status.Unstaged,
		Untracked:  status.Untracked,
		Deleted:    status.Deleted,
	}, err
}

func gitStatusFingerprint(status GitStatus) string {
	parts := []string{status.Branch, status.HeadCommit, status.RemoteURL}
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

func repositoryFingerprint(repoRoot string) string {
	return sourceFileFingerprint(sourceFileSnapshot(repoRoot))
}

func sourceFileSnapshot(repoRoot string) map[string]string {
	files := map[string]string{}
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files[filepath.ToSlash(rel)] = info.ModTime().UTC().Format(time.RFC3339Nano) + ":" + fmt.Sprint(info.Size())
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
			changes = append(changes, SourceFileChange{Path: path, ChangeType: "added"})
		case prev != next:
			changes = append(changes, SourceFileChange{Path: path, ChangeType: "modified"})
		}
	}
	for path := range before {
		if _, ok := seen[path]; !ok {
			changes = append(changes, SourceFileChange{Path: path, ChangeType: "deleted"})
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
