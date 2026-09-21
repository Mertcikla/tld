package watch

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/mertcikla/tld/v2/internal/analyzer"
)

// SettingsValidationError describes a single invalid watch setting.
type SettingsValidationError struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

func (e SettingsValidationError) Error() string {
	if e.Key == "" {
		return e.Message
	}
	return e.Key + ": " + e.Message
}

// SettingsValidationErrors aggregates validation failures so the API can
// report every offending field at once.
type SettingsValidationErrors []SettingsValidationError

func (e SettingsValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	return fmt.Sprintf("%s (+%d more)", e[0].Error(), len(e)-1)
}

// ValidateSettings checks watch Settings for user-facing constraints. It
// validates raw values (before normalization) so invalid input is rejected
// rather than silently coerced. Durations and counts use zero as "unset" and
// are only rejected when explicitly negative.
func ValidateSettings(settings Settings) error {
	var errs SettingsValidationErrors
	add := func(key, msg string) {
		errs = append(errs, SettingsValidationError{Key: key, Message: msg})
	}

	switch strings.ToLower(strings.TrimSpace(settings.Watcher)) {
	case "", "auto", "fsnotify", "poll":
	default:
		add("watch.watcher", "must be auto, fsnotify, or poll")
	}
	if settings.PollInterval < 0 {
		add("watch.poll_interval", "must be a positive duration such as 500ms or 1s")
	}
	if settings.Debounce < 0 {
		add("watch.debounce", "must be a positive duration such as 500ms or 1s")
	}
	for _, language := range settings.Languages {
		if !validWatchLanguage(language) {
			add("watch.languages", fmt.Sprintf("unsupported language %q", language))
			break
		}
	}
	switch strings.ToLower(strings.TrimSpace(settings.Scale.Strategy)) {
	case "", "auto", "full", "limited", "abort":
	default:
		add("watch.scale.strategy", "must be auto, full, limited, or abort")
	}
	for _, item := range []struct {
		key   string
		value int
	}{
		{"watch.thresholds.max_elements_per_view", settings.Thresholds.MaxElementsPerView},
		{"watch.thresholds.max_connectors_per_view", settings.Thresholds.MaxConnectorsPerView},
		{"watch.thresholds.max_incoming_per_element", settings.Thresholds.MaxIncomingPerElement},
		{"watch.thresholds.max_outgoing_per_element", settings.Thresholds.MaxOutgoingPerElement},
		{"watch.thresholds.max_expanded_connectors_per_group", settings.Thresholds.MaxExpandedConnectorsPerGroup},
		{"watch.scale.max_tracked_files", settings.Scale.MaxTrackedFiles},
		{"watch.scale.max_limited_files", settings.Scale.MaxLimitedFiles},
		{"watch.scale.max_recent_files", settings.Scale.MaxRecentFiles},
		{"watch.scale.max_caller_depth", settings.Scale.MaxCallerDepth},
	} {
		if item.value < 0 {
			add(item.key, "must be positive")
		}
	}
	if settings.Scale.MaxBlastRadiusHops < 0 {
		add("watch.scale.max_blast_radius_hops", "must be zero or positive")
	}
	if settings.LSP.HealthInterval < 0 {
		add("watch.lsp.health_interval", "must be a positive duration such as 1m")
	}
	if settings.LSP.MemoryLimitBytes < 0 {
		add("watch.lsp.memory_limit_bytes", "must be positive")
	}
	for language := range settings.LSP.Commands {
		if !validWatchLanguage(language) {
			add("watch.lsp.commands."+language, "unsupported language")
		}
	}
	for _, item := range []struct {
		key   string
		value float64
	}{
		{"watch.visibility.core_threshold", settings.Visibility.CoreThreshold},
		{"watch.visibility.tier_multiplier", settings.Visibility.TierMultiplier},
		{"watch.visibility.max_expansion_multiplier", settings.Visibility.MaxExpansionMultiplier},
	} {
		if item.value < 0 {
			add(item.key, "must be positive")
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ValidateEmbeddingConfig checks a raw embedding configuration. Zero values
// are treated as unset; negative values and invalid enums are rejected.
func ValidateEmbeddingConfig(embedding EmbeddingConfig) error {
	var errs SettingsValidationErrors
	add := func(key, msg string) {
		errs = append(errs, SettingsValidationError{Key: key, Message: msg})
	}
	provider := strings.TrimSpace(embedding.Provider)
	switch provider {
	case "", "none", "openai", "ollama", "local-lexical", "local-deterministic-test":
	default:
		add("watch.embedding.provider", "must be none, openai, ollama, local-lexical, or local-deterministic-test")
	}
	if embedding.Dimension < 0 {
		add("watch.embedding.dimension", "must be non-negative")
	}
	if embedding.MaxTokens < 0 {
		add("watch.embedding.max_tokens", "must be non-negative")
	}
	if provider == "openai" || provider == "ollama" {
		endpoints := embedding.Endpoints
		if len(endpoints) == 0 && strings.TrimSpace(embedding.Endpoint) != "" {
			endpoints = []string{embedding.Endpoint}
		}
		if len(endpoints) == 0 {
			add("watch.embedding.endpoint", "must be a valid URL for the selected provider")
		}
		for _, endpoint := range endpoints {
			if !validWatchHTTPURL(endpoint) {
				add("watch.embedding.endpoint", "must be a valid URL for the selected provider")
				break
			}
		}
		if strings.TrimSpace(embedding.Model) == "" {
			add("watch.embedding.model", "must be non-empty for the selected provider")
		}
		if embedding.HealthThreshold < 0 || embedding.HealthThreshold > 1 {
			add("watch.embedding.health_threshold", "must be greater than 0 and at most 1")
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validWatchLanguage(language string) bool {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return false
	}
	_, ok := analyzer.LanguageSpecFor(analyzer.Language(language))
	return ok
}

func validWatchHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}
