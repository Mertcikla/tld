package watch

import (
	"sort"
	"strings"
	"time"

	"github.com/mertcikla/tld/internal/analyzer"
)

const (
	WatcherAuto     = "auto"
	WatcherFSNotify = "fsnotify"
	WatcherPoll     = "poll"
)

func DefaultSettings() Settings {
	langs := make([]string, 0, len(analyzer.SupportedLanguages()))
	for _, spec := range analyzer.SupportedLanguages() {
		langs = append(langs, string(spec.Language))
	}
	sort.Strings(langs)
	return Settings{
		Languages:    langs,
		Watcher:      WatcherAuto,
		PollInterval: time.Second,
		Debounce:     500 * time.Millisecond,
		Thresholds:   defaultThresholds(Thresholds{}),
	}
}

func NormalizeSettings(settings Settings) Settings {
	defaults := DefaultSettings()
	if len(settings.Languages) == 0 {
		settings.Languages = defaults.Languages
	} else {
		settings.Languages = normalizeLanguages(settings.Languages)
	}
	switch strings.ToLower(strings.TrimSpace(settings.Watcher)) {
	case WatcherFSNotify:
		settings.Watcher = WatcherFSNotify
	case WatcherPoll:
		settings.Watcher = WatcherPoll
	default:
		settings.Watcher = WatcherAuto
	}
	if settings.PollInterval <= 0 {
		settings.PollInterval = defaults.PollInterval
	}
	if settings.Debounce <= 0 {
		settings.Debounce = defaults.Debounce
	}
	settings.Thresholds = defaultThresholds(settings.Thresholds)
	return settings
}

func normalizeLanguages(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		lang := strings.ToLower(strings.TrimSpace(value))
		if lang == "" {
			continue
		}
		if _, ok := analyzer.LanguageSpecFor(analyzer.Language(lang)); !ok {
			continue
		}
		seen[lang] = struct{}{}
	}
	if len(seen) == 0 {
		return DefaultSettings().Languages
	}
	out := make([]string, 0, len(seen))
	for lang := range seen {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

func languageAllowed(language string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[strings.ToLower(language)]
	return ok
}
