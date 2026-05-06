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
		Visibility:   defaultVisibilityConfig(VisibilityConfig{}),
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
	settings.Visibility = defaultVisibilityConfig(settings.Visibility)
	return settings
}

func defaultVisibilityConfig(cfg VisibilityConfig) VisibilityConfig {
	if !cfg.CoreThresholdSet && !cfg.CoreThresholdEnabled {
		cfg.CoreThresholdEnabled = true
	}
	if cfg.CoreThreshold <= 0 {
		cfg.CoreThreshold = 1
	}
	if cfg.TierMultiplier <= 0 {
		cfg.TierMultiplier = 0.5
	}
	if cfg.MaxExpansionMultiplier <= 0 {
		cfg.MaxExpansionMultiplier = 2
	}
	defaults := VisibilityWeights{
		Changed:               100,
		Selected:              100,
		UserShow:              100,
		UserHide:              -100,
		HighSignalFact:        1.5,
		RelationshipProximity: 1,
		DependencyFact:        0.2,
		UtilityNoise:          -0.8,
		HighDegreeNoise:       -1.5,
	}
	if !cfg.WeightsSet {
		if cfg.Weights.Changed == 0 {
			cfg.Weights.Changed = defaults.Changed
		}
		if cfg.Weights.Selected == 0 {
			cfg.Weights.Selected = defaults.Selected
		}
		if cfg.Weights.UserShow == 0 {
			cfg.Weights.UserShow = defaults.UserShow
		}
		if cfg.Weights.UserHide == 0 {
			cfg.Weights.UserHide = defaults.UserHide
		}
		if cfg.Weights.HighSignalFact == 0 {
			cfg.Weights.HighSignalFact = defaults.HighSignalFact
		}
		if cfg.Weights.RelationshipProximity == 0 {
			cfg.Weights.RelationshipProximity = defaults.RelationshipProximity
		}
		if cfg.Weights.DependencyFact == 0 {
			cfg.Weights.DependencyFact = defaults.DependencyFact
		}
		if cfg.Weights.UtilityNoise == 0 {
			cfg.Weights.UtilityNoise = defaults.UtilityNoise
		}
		if cfg.Weights.HighDegreeNoise == 0 {
			cfg.Weights.HighDegreeNoise = defaults.HighDegreeNoise
		}
	}
	return cfg
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
