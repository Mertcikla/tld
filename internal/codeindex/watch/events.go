// Package watch provides the filesystem change detector and debounce strategy
// used by `tld index --watch`. Git remains the source of truth: fsnotify events
// only trigger a cheap Git state check, and a polling failsafe covers missed or
// unsupported events.
package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mertcikla/tld/v2/internal/codeindex/parser"
)

// Options configures the change detector.
type Options struct {
	Root string
	// Exclude lists repository-relative paths that must never trigger a scan.
	Exclude []string
	// Debounce is the quiet window events must settle for before a scan runs.
	Debounce time.Duration
	// MaxWait caps how long continuous events may delay a scan.
	MaxWait time.Duration
	// PollInterval is the failsafe poll period while fsnotify is active and the
	// primary interval when it is unavailable.
	PollInterval time.Duration
}

// Detector waits for repository changes. Callers pass a capture function that
// returns the current Git change signature; Next only returns once the
// signature differs from the one the caller last observed.
type Detector struct {
	opts    Options
	watcher *fsnotify.Watcher
	usingFS bool
	// Log, when set, records degraded-mode transitions.
	Log func(string, ...any)
}

// CaptureFunc returns the current Git change signature and may be called
// frequently, so it must stay cheap.
type CaptureFunc func(context.Context) (string, error)

// NewDetector builds a detector. fsnotify setup is best-effort: if watches
// cannot be registered the detector falls back to polling.
func NewDetector(opts Options) *Detector {
	if opts.Debounce <= 0 {
		opts.Debounce = 350 * time.Millisecond
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = 2 * time.Second
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	d := &Detector{opts: opts}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return d
	}
	if err := d.addTree(w, opts.Root); err != nil {
		_ = w.Close()
		return d
	}
	d.watcher = w
	d.usingFS = true
	return d
}

// Close releases filesystem watches.
func (d *Detector) Close() error {
	if d == nil || d.watcher == nil {
		return nil
	}
	return d.watcher.Close()
}

// Next blocks until the Git signature differs from current or the context ends.
func (d *Detector) Next(ctx context.Context, current string, capture CaptureFunc) (string, error) {
	if !d.usingFS || d.watcher == nil {
		return d.pollUntilChange(ctx, current, capture)
	}
	poll := time.NewTicker(d.failsafeInterval())
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case event, ok := <-d.watcher.Events:
			if !ok {
				d.usingFS = false
				return d.pollUntilChange(ctx, current, capture)
			}
			if !d.relevant(event) {
				continue
			}
			signature, err := d.settle(ctx, capture)
			if err != nil {
				return "", err
			}
			if signature != current {
				return signature, nil
			}
		case <-d.watcher.Errors:
			// Keep watching; the failsafe poll still detects changes.
		case <-poll.C:
			signature, err := capture(ctx)
			if err != nil {
				return "", err
			}
			if signature != current {
				return signature, nil
			}
		}
	}
}

// settle coalesces a burst of events into a single scan attempt once the tree
// has been quiet for Debounce, or after MaxWait of continuous activity.
func (d *Detector) settle(ctx context.Context, capture CaptureFunc) (string, error) {
	quiet := time.NewTimer(d.opts.Debounce)
	defer quiet.Stop()
	deadline := time.NewTimer(d.opts.MaxWait)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case event, ok := <-d.watcher.Events:
			if !ok {
				return capture(ctx)
			}
			if !d.relevant(event) {
				continue
			}
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(d.opts.Debounce)
		case <-quiet.C:
			return capture(ctx)
		case <-deadline.C:
			return capture(ctx)
		}
	}
}

func (d *Detector) pollUntilChange(ctx context.Context, current string, capture CaptureFunc) (string, error) {
	ticker := time.NewTicker(d.opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			signature, err := capture(ctx)
			if err != nil {
				return "", err
			}
			if signature != current {
				return signature, nil
			}
		}
	}
}

// failsafeInterval is the slow poll kept alive while fsnotify is active so
// events missed by the OS watcher still trigger a scan.
func (d *Detector) failsafeInterval() time.Duration {
	if d.opts.PollInterval > 10*time.Second {
		return d.opts.PollInterval
	}
	return 15 * time.Second
}

func (d *Detector) relevant(event fsnotify.Event) bool {
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}
	rel, err := filepath.Rel(d.opts.Root, event.Name)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	if event.Op&fsnotify.Create != 0 {
		if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
			if !d.ignored(rel) && d.watcher != nil {
				_ = d.addTree(d.watcher, event.Name)
			}
			return false
		}
	}
	return !d.ignored(rel)
}

// ignored reports whether a repository-relative path should never trigger a
// scan. Generated indexing artifacts are ignored to prevent feedback loops.
func (d *Detector) ignored(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return true
	}
	base := filepath.Base(rel)
	if strings.HasSuffix(base, ".scip") || strings.HasSuffix(base, ".tmp") ||
		strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") ||
		strings.HasPrefix(base, ".#") {
		return true
	}
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if part == ".tld" {
			return true
		}
		if part == ".git" {
			// Only repository metadata that changes refs or the index is
			// interesting; object and log churn must not trigger scans.
			rest := strings.Join(parts[i+1:], "/")
			switch {
			case rest == "index", rest == "HEAD", rest == "MERGE_HEAD",
				rest == "ORIG_HEAD", rest == "FETCH_HEAD",
				strings.HasPrefix(rest, "refs/"), strings.HasPrefix(rest, "rebase-merge/"),
				strings.HasPrefix(rest, "rebase-apply/"):
				return false
			default:
				return true
			}
		}
		if parser.SkipDirs[part] {
			return true
		}
	}
	for _, pattern := range d.opts.Exclude {
		pattern = strings.Trim(pattern, "/")
		if pattern == "" {
			continue
		}
		if rel == pattern || strings.HasPrefix(rel, pattern+"/") {
			return true
		}
		if matched, _ := filepath.Match(pattern, rel); matched {
			return true
		}
	}
	return false
}

// skipWatchDir reports whether a directory can be skipped when registering
// watches. Repository metadata under .git is watched shallowly so ref and index
// updates wake the watcher without subscribing to object churn.
func (d *Detector) skipWatchDir(rel string) bool {
	rel = filepath.ToSlash(rel)
	switch {
	case rel == ".git", rel == ".git/refs", rel == ".git/logs":
		return false
	case strings.HasPrefix(rel, ".git/rebase-merge"), strings.HasPrefix(rel, ".git/rebase-apply"):
		return false
	case strings.HasPrefix(rel, ".git/"):
		return true
	}
	return d.ignored(rel)
}

// addTree registers a watch on root and every non-ignored subdirectory.
func (d *Detector) addTree(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(d.opts.Root, path)
		if relErr != nil {
			return nil
		}
		if rel != "." && d.skipWatchDir(rel) {
			return filepath.SkipDir
		}
		if addErr := w.Add(path); addErr != nil {
			return addErr
		}
		return nil
	})
}
