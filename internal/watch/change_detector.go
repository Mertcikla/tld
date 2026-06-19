package watch

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/ignore"
)

type changeTrigger struct {
	Poll         bool
	PendingPaths map[string]struct{}
}

type detectedChange struct {
	Trigger              string
	Git                  GitStatus
	GitFingerprint       string
	SourceChanged        bool
	SourceChanges        []SourceFileChange
	TargetedFiles        []string
	DeletedFiles         []string
	StableSourceSnapshot map[string]string
	SourceFingerprint    string
	ContentFingerprint   string
	ConsumePendingPaths  bool
}

type ChangeDetector struct {
	Store        *Store
	RepositoryID int64
	RepoRoot     string
	Settings     Settings
	Rules        *ignore.Rules
	WatcherMode  string
	Logger       EventLogger

	limitedMode            bool
	lastSourceSnapshot     map[string]string
	lastFingerprint        string
	lastContentFingerprint string
	lastHead               string
	lastGitFingerprint     string
}

func NewChangeDetector(store *Store, repositoryID int64, repoRoot string, settings Settings, rules *ignore.Rules, watcherMode string, initialScan ScanResult, initialGit GitStatus, logger EventLogger) *ChangeDetector {
	d := &ChangeDetector{
		Store:              store,
		RepositoryID:       repositoryID,
		RepoRoot:           repoRoot,
		Settings:           NormalizeSettings(settings),
		Rules:              rules,
		WatcherMode:        watcherMode,
		Logger:             logger,
		limitedMode:        initialScan.Mode == ScanStrategyLimited,
		lastHead:           initialGit.HeadCommit,
		lastGitFingerprint: gitStatusFingerprint(initialGit),
	}
	if !d.limitedMode {
		d.lastSourceSnapshot = sourceFileSnapshot(repoRoot, d.Settings, rules, nil)
		d.lastFingerprint = sourceFileFingerprint(d.lastSourceSnapshot)
		d.lastContentFingerprint = sourceFileContentFingerprint(d.lastSourceSnapshot)
	}
	return d
}

func (d *ChangeDetector) LastHead() string {
	if d == nil {
		return ""
	}
	return d.lastHead
}

func (d *ChangeDetector) MarkHeadRecorded(head string) {
	if d == nil || head == "" {
		return
	}
	d.lastHead = head
}

func (d *ChangeDetector) Detect(ctx context.Context, trigger changeTrigger) (detectedChange, bool, error) {
	if d == nil {
		return detectedChange{}, false, nil
	}
	nextGit, err := gitStatusSnapshot(d.RepoRoot)
	if err != nil {
		logError(ctx, d.Logger, "watch.git_status_snapshot_failed", err, "repository_id", d.RepositoryID)
		return detectedChange{}, false, err
	}
	nextGitFingerprint := gitStatusFingerprint(nextGit)
	change := detectedChange{
		Trigger:        "poll",
		Git:            nextGit,
		GitFingerprint: nextGitFingerprint,
	}
	if !trigger.Poll {
		change.Trigger = d.WatcherMode
	}

	if d.WatcherMode == WatcherFSNotify && !d.limitedMode {
		change.ConsumePendingPaths = true
		change.TargetedFiles, change.DeletedFiles = d.classifyPendingPaths(trigger.PendingPaths)
		stableSourceSnapshot := sourceFileSnapshot(d.RepoRoot, d.Settings, d.Rules, d.lastSourceSnapshot)
		nextFingerprint := sourceFileFingerprint(stableSourceSnapshot)
		nextContentFingerprint := sourceFileContentFingerprint(stableSourceSnapshot)

		contentChanged := nextContentFingerprint != d.lastContentFingerprint
		gitChanged := nextGitFingerprint != d.lastGitFingerprint || nextGit.HeadCommit != d.lastHead
		if !contentChanged && !gitChanged {
			logDebug(ctx, d.Logger, "watch.change.skipped", "repository_id", d.RepositoryID, "trigger", change.Trigger, "reason", "unchanged_content", "targeted_files", len(change.TargetedFiles), "deleted_files", len(change.DeletedFiles), "source_fingerprint_changed", nextFingerprint != d.lastFingerprint, "content_fingerprint_changed", false, "git_fingerprint_changed", false)
			d.updateSourceSnapshot(stableSourceSnapshot, nextFingerprint, nextContentFingerprint)
			return change, false, nil
		}
		logInfo(ctx, d.Logger, "watch.change.detected", "repository_id", d.RepositoryID, "trigger", change.Trigger, "limited_mode", d.limitedMode, "head", nextGit.HeadCommit, "source_fingerprint_changed", nextFingerprint != d.lastFingerprint, "content_fingerprint_changed", contentChanged, "git_fingerprint_changed", nextGitFingerprint != d.lastGitFingerprint)

		if len(change.TargetedFiles) > 0 || len(change.DeletedFiles) > 0 {
			change.SourceChanged = true
			for _, file := range change.TargetedFiles {
				change.SourceChanges = append(change.SourceChanges, SourceFileChange{Path: file, ChangeType: "modified", Language: sourceFileChangeLanguage(file)})
			}
			for _, file := range change.DeletedFiles {
				change.SourceChanges = append(change.SourceChanges, SourceFileChange{Path: file, ChangeType: "deleted", Language: sourceFileChangeLanguage(file)})
			}
		}
		change.StableSourceSnapshot = stableSourceSnapshot
		change.SourceFingerprint = nextFingerprint
		change.ContentFingerprint = nextContentFingerprint
		return change, true, nil
	}

	if !d.limitedMode {
		nextSourceSnapshot := sourceFileSnapshot(d.RepoRoot, d.Settings, d.Rules, d.lastSourceSnapshot)
		nextFingerprint := sourceFileFingerprint(nextSourceSnapshot)
		nextContentFingerprint := sourceFileContentFingerprint(nextSourceSnapshot)
		if nextContentFingerprint == d.lastContentFingerprint && nextGit.HeadCommit == d.lastHead && nextGitFingerprint == d.lastGitFingerprint {
			if nextFingerprint != d.lastFingerprint {
				d.updateSourceSnapshot(nextSourceSnapshot, nextFingerprint, nextContentFingerprint)
			}
			return change, false, nil
		}
		change.SourceFingerprint = nextFingerprint
		change.ContentFingerprint = nextContentFingerprint
	} else if nextGit.HeadCommit == d.lastHead && nextGitFingerprint == d.lastGitFingerprint {
		return change, false, nil
	}
	logInfo(ctx, d.Logger, "watch.change.detected", "repository_id", d.RepositoryID, "trigger", change.Trigger, "limited_mode", d.limitedMode, "head", nextGit.HeadCommit, "source_fingerprint_changed", change.SourceFingerprint != d.lastFingerprint, "content_fingerprint_changed", change.ContentFingerprint != d.lastContentFingerprint, "git_fingerprint_changed", nextGitFingerprint != d.lastGitFingerprint)

	if !d.limitedMode {
		stableSourceSnapshot := sourceFileSnapshot(d.RepoRoot, d.Settings, d.Rules, d.lastSourceSnapshot)
		change.StableSourceSnapshot = stableSourceSnapshot
		change.SourceFingerprint = sourceFileFingerprint(stableSourceSnapshot)
		change.ContentFingerprint = sourceFileContentFingerprint(stableSourceSnapshot)
		change.SourceChanged = change.ContentFingerprint != d.lastContentFingerprint
	}
	nextGit, err = gitStatusSnapshot(d.RepoRoot)
	if err != nil {
		logError(ctx, d.Logger, "watch.git_status_snapshot_failed", err, "repository_id", d.RepositoryID)
		return detectedChange{}, false, err
	}
	change.Git = nextGit
	change.GitFingerprint = gitStatusFingerprint(nextGit)
	if d.limitedMode {
		change.SourceChanges = mergeSourceFileChanges(
			sourceChangesFromGit(d.RepoRoot, d.Logger),
			sourceChangesSinceLatestWatchVersion(ctx, d.Store, d.RepositoryID, d.RepoRoot, d.Logger),
		)
		change.SourceChanged = len(change.SourceChanges) > 0
	} else {
		change.SourceChanges = diffSourceFileSnapshots(d.lastSourceSnapshot, change.StableSourceSnapshot)
	}
	return change, true, nil
}

func (d *ChangeDetector) Commit(scan ScanResult, change detectedChange) {
	if d == nil {
		return
	}
	d.limitedMode = scan.Mode == ScanStrategyLimited
	if !d.limitedMode {
		d.updateSourceSnapshot(change.StableSourceSnapshot, change.SourceFingerprint, change.ContentFingerprint)
	}
	d.lastGitFingerprint = change.GitFingerprint
}

func (d *ChangeDetector) classifyPendingPaths(pending map[string]struct{}) ([]string, []string) {
	var targetedFiles []string
	var deletedFiles []string
	for path := range pending {
		rel, err := filepath.Rel(d.RepoRoot, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		rel = filepathToSlash(rel)
		if rel == "." {
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			deletedFiles = append(deletedFiles, rel)
		} else {
			targetedFiles = append(targetedFiles, rel)
		}
	}
	sort.Strings(targetedFiles)
	sort.Strings(deletedFiles)
	return targetedFiles, deletedFiles
}

func (d *ChangeDetector) updateSourceSnapshot(snapshot map[string]string, fingerprint, contentFingerprint string) {
	if snapshot == nil {
		return
	}
	d.lastSourceSnapshot = snapshot
	d.lastFingerprint = fingerprint
	d.lastContentFingerprint = contentFingerprint
}
