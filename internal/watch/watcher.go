package watch

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/mertcikla/tld/internal/analyzer"
	"github.com/mertcikla/tld/internal/ignore"
)

type sourceWatcher struct {
	Mode     string
	Events   <-chan struct{}
	Warnings []string
	Close    func() error
}

func newSourceWatcher(ctx context.Context, root string, settings Settings, rules *ignore.Rules) sourceWatcher {
	settings = NormalizeSettings(settings)
	if settings.Watcher == WatcherPoll {
		return sourceWatcher{Mode: WatcherPoll}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		if settings.Watcher == WatcherFSNotify {
			return sourceWatcher{Mode: WatcherPoll, Warnings: []string{"fsnotify unavailable: " + err.Error()}}
		}
		return sourceWatcher{Mode: WatcherPoll, Warnings: []string{"fsnotify unavailable; using poll fallback"}}
	}
	ch := make(chan struct{}, 1)
	allowed := map[string]struct{}{}
	for _, lang := range settings.Languages {
		allowed[lang] = struct{}{}
	}
	if rules == nil {
		rules = &ignore.Rules{}
	}
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel != "." && (rules.ShouldIgnorePath(rel) || isHiddenBuildOutput(d.Name())) {
			return filepath.SkipDir
		}
		_ = watcher.Add(path)
		return nil
	})
	warnings := []string{}
	if walkErr != nil {
		warnings = append(warnings, "fsnotify setup warning: "+walkErr.Error())
	}
	go func() {
		defer close(ch)
		defer func() { _ = watcher.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) {
					if info, err := filepathAbsStat(event.Name); err == nil && info.IsDir() {
						_ = watcher.Add(event.Name)
					}
				}
				if sourceEventRelevant(root, event.Name, allowed, rules) {
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			case <-watcher.Errors:
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}()
	return sourceWatcher{Mode: WatcherFSNotify, Events: ch, Warnings: warnings, Close: watcher.Close}
}

func sourceEventRelevant(root, eventPath string, allowed map[string]struct{}, rules *ignore.Rules) bool {
	rel, err := filepath.Rel(root, eventPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rules != nil && rules.ShouldIgnorePath(rel) {
		return false
	}
	language, ok := analyzer.DetectLanguage(eventPath)
	return ok && languageAllowed(string(language), allowed)
}

func filepathAbsStat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}
