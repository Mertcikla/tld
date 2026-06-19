package watch

import (
	"sort"
	"strings"
	"time"

	"github.com/mertcikla/tld/v2/internal/analyzer"
)

const (
	WatcherAuto     = "auto"
	WatcherFSNotify = "fsnotify"
	WatcherPoll     = "poll"

	ScanStrategyAuto    = "auto"
	ScanStrategyFull    = "full"
	ScanStrategyLimited = "limited"
	ScanStrategyAbort   = "abort"

	defaultMaxTrackedFiles = 20000
	defaultMaxLimitedFiles = 2000
	defaultMaxRecentFiles  = 1000
	defaultMaxCallerDepth  = 10
	defaultBlastRadiusHops = 1
	defaultLSPMemoryLimit  = 4294967296
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
		PollInterval: 10 * time.Second,
		Debounce:     500 * time.Millisecond,
		Dependencies: DependencyConfig{
			Enabled: false,
		},
		Thresholds: defaultThresholds(Thresholds{}),
		Visibility: defaultVisibilityConfig(VisibilityConfig{}),
		Scale: ScaleConfig{
			Strategy:           ScanStrategyAuto,
			MaxTrackedFiles:    defaultMaxTrackedFiles,
			MaxLimitedFiles:    defaultMaxLimitedFiles,
			MaxRecentFiles:     defaultMaxRecentFiles,
			MaxCallerDepth:     defaultMaxCallerDepth,
			MaxBlastRadiusHops: defaultBlastRadiusHops,
		},
		LSP: LSPConfig{
			Enabled:          true,
			HealthInterval:   time.Minute,
			MemoryLimitBytes: defaultLSPMemoryLimit,
		},
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
	settings.Scale = defaultScaleConfig(settings.Scale)
	settings.LSP = defaultLSPConfig(settings.LSP)
	return settings
}

func defaultLSPConfig(cfg LSPConfig) LSPConfig {
	if cfg.HealthInterval <= 0 {
		cfg.HealthInterval = time.Minute
	}
	if cfg.MemoryLimitBytes <= 0 {
		cfg.MemoryLimitBytes = defaultLSPMemoryLimit
	}
	cfg.Commands = normalizeLSPCommands(cfg.Commands)
	return cfg
}

func normalizeLSPCommands(commands map[string]string) map[string]string {
	if len(commands) == 0 {
		return nil
	}
	normalized := map[string]string{}
	for language, command := range commands {
		language = strings.ToLower(strings.TrimSpace(language))
		command = strings.TrimSpace(command)
		if language == "" || command == "" {
			continue
		}
		normalized[language] = command
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func defaultScaleConfig(cfg ScaleConfig) ScaleConfig {
	switch strings.ToLower(strings.TrimSpace(cfg.Strategy)) {
	case ScanStrategyFull:
		cfg.Strategy = ScanStrategyFull
	case ScanStrategyLimited:
		cfg.Strategy = ScanStrategyLimited
	case ScanStrategyAbort:
		cfg.Strategy = ScanStrategyAbort
	default:
		cfg.Strategy = ScanStrategyAuto
	}
	if cfg.MaxTrackedFiles <= 0 {
		cfg.MaxTrackedFiles = defaultMaxTrackedFiles
	}
	if cfg.MaxLimitedFiles <= 0 {
		cfg.MaxLimitedFiles = defaultMaxLimitedFiles
	}
	if cfg.MaxRecentFiles <= 0 {
		cfg.MaxRecentFiles = defaultMaxRecentFiles
	}
	if cfg.MaxCallerDepth <= 0 {
		cfg.MaxCallerDepth = defaultMaxCallerDepth
	}
	if cfg.MaxBlastRadiusHops < 0 {
		cfg.MaxBlastRadiusHops = defaultBlastRadiusHops
	}
	return cfg
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
