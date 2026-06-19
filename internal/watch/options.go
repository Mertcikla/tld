package watch

import (
	"strings"
	"time"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func ResolveEmbeddingConfig(cfg *workspace.Config, provider, endpoint, model string, dimension int, maxTokens int, runtimePath ...string) EmbeddingConfig {
	embedding := EmbeddingConfig{}
	if cfg != nil {
		embedding.Provider = cfg.Watch.Embedding.Provider
		embedding.Endpoint = cfg.Watch.Embedding.Endpoint.String()
		embedding.Model = cfg.Watch.Embedding.Model
		embedding.Dimension = cfg.Watch.Embedding.Dimension
		embedding.RuntimePath = cfg.Watch.Embedding.RuntimePath
		embedding.HealthThreshold = cfg.Watch.Embedding.HealthThreshold
		embedding.MaxTokens = cfg.Watch.Embedding.MaxTokens
	}
	if provider != "" {
		embedding.Provider = provider
	}
	if endpoint != "" {
		embedding.Endpoint = endpoint
	}
	if model != "" {
		embedding.Model = model
	}
	if dimension > 0 {
		embedding.Dimension = dimension
	}
	if maxTokens > 0 {
		embedding.MaxTokens = maxTokens
	}
	if len(runtimePath) > 0 && runtimePath[0] != "" {
		embedding.RuntimePath = runtimePath[0]
	}
	return NormalizeEmbeddingConfig(embedding)
}

func ResolveSettings(cfg *workspace.Config, languages []string, watcherMode, pollInterval, debounce string, maxElements, maxConnectors, maxIncoming, maxOutgoing, maxExpandedGroup int) Settings {
	settings := DefaultSettings()
	if cfg != nil {
		settings.Languages = cfg.Watch.Languages
		settings.Watcher = cfg.Watch.Watcher
		settings.PollInterval = parseDurationOrZero(cfg.Watch.PollInterval)
		settings.Debounce = parseDurationOrZero(cfg.Watch.Debounce)
		settings.Dependencies = DependencyConfig{
			Enabled: cfg.Watch.Dependencies.Enabled,
		}
		settings.Thresholds = Thresholds{
			MaxElementsPerView:            cfg.Watch.Thresholds.MaxElementsPerView,
			MaxConnectorsPerView:          cfg.Watch.Thresholds.MaxConnectorsPerView,
			MaxIncomingPerElement:         cfg.Watch.Thresholds.MaxIncomingPerElement,
			MaxOutgoingPerElement:         cfg.Watch.Thresholds.MaxOutgoingPerElement,
			MaxExpandedConnectorsPerGroup: cfg.Watch.Thresholds.MaxExpandedConnectorsPerGroup,
		}
		settings.Visibility = VisibilityConfig{
			CoreThresholdEnabled:   cfg.Watch.Visibility.CoreThresholdEnabled,
			CoreThreshold:          cfg.Watch.Visibility.CoreThreshold,
			TierMultiplier:         cfg.Watch.Visibility.TierMultiplier,
			MaxExpansionMultiplier: cfg.Watch.Visibility.MaxExpansionMultiplier,
			CoreThresholdSet:       true,
			WeightsSet:             true,
			Weights: VisibilityWeights{
				Changed:               cfg.Watch.Visibility.Weights.Changed,
				Selected:              cfg.Watch.Visibility.Weights.Selected,
				UserShow:              cfg.Watch.Visibility.Weights.UserShow,
				UserHide:              cfg.Watch.Visibility.Weights.UserHide,
				HighSignalFact:        cfg.Watch.Visibility.Weights.HighSignalFact,
				RelationshipProximity: cfg.Watch.Visibility.Weights.RelationshipProximity,
				DependencyFact:        cfg.Watch.Visibility.Weights.DependencyFact,
				UtilityNoise:          cfg.Watch.Visibility.Weights.UtilityNoise,
				HighDegreeNoise:       cfg.Watch.Visibility.Weights.HighDegreeNoise,
			},
		}
		settings.Scale = ScaleConfig{
			Strategy:           cfg.Watch.Scale.Strategy,
			MaxTrackedFiles:    cfg.Watch.Scale.MaxTrackedFiles,
			MaxLimitedFiles:    cfg.Watch.Scale.MaxLimitedFiles,
			MaxRecentFiles:     cfg.Watch.Scale.MaxRecentFiles,
			MaxCallerDepth:     cfg.Watch.Scale.MaxCallerDepth,
			MaxBlastRadiusHops: cfg.Watch.Scale.MaxBlastRadiusHops,
		}
		settings.LSP = LSPConfig{
			Enabled:          cfg.Watch.LSP.Enabled,
			HealthInterval:   parseDurationOrZero(cfg.Watch.LSP.HealthInterval),
			MemoryLimitBytes: cfg.Watch.LSP.MemoryLimitBytes,
			Commands:         cfg.Watch.LSP.Commands,
		}
	}
	if len(languages) > 0 {
		settings.Languages = languages
	}
	if watcherMode != "" {
		settings.Watcher = watcherMode
	}
	if pollInterval != "" {
		settings.PollInterval = parseDurationOrZero(pollInterval)
	}
	if debounce != "" {
		settings.Debounce = parseDurationOrZero(debounce)
	}
	if maxElements > 0 {
		settings.Thresholds.MaxElementsPerView = maxElements
	}
	if maxConnectors > 0 {
		settings.Thresholds.MaxConnectorsPerView = maxConnectors
	}
	if maxIncoming > 0 {
		settings.Thresholds.MaxIncomingPerElement = maxIncoming
	}
	if maxOutgoing > 0 {
		settings.Thresholds.MaxOutgoingPerElement = maxOutgoing
	}
	if maxExpandedGroup > 0 {
		settings.Thresholds.MaxExpandedConnectorsPerGroup = maxExpandedGroup
	}
	return NormalizeSettings(settings)
}

func parseDurationOrZero(value string) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}
