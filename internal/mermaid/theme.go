package mermaid

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed theme.json
var defaultThemeJSON []byte

// Theme is a versioned presentation theme, independent of scene selection.
// Use ParseTheme to merge and validate a JSON override of the embedded default.
type Theme struct {
	Version int `json:"version"`
	Labels  struct {
		Change  bool `json:"change"`
		Context bool `json:"context"`
		Churn   bool `json:"churn"`
	} `json:"labels"`
	Nodes ThemeStates `json:"nodes"`
	Edges ThemeStates `json:"edges"`
}

type ThemeStates struct {
	Context  map[string]string `json:"context"`
	Modified map[string]string `json:"modified"`
	Added    map[string]string `json:"added"`
	Removed  map[string]string `json:"removed"`
}

func (s ThemeStates) style(state string) map[string]string {
	switch state {
	case "added":
		return s.Added
	case "removed":
		return s.Removed
	case "modified":
		return s.Modified
	default:
		return s.Context
	}
}

// DefaultTheme returns an independent copy so callers cannot mutate defaults.
func DefaultTheme() *Theme {
	var theme Theme
	if err := json.Unmarshal(defaultThemeJSON, &theme); err != nil {
		panic("invalid embedded Mermaid theme: " + err.Error())
	}
	return &theme
}

// ParseTheme overlays supplied properties onto the embedded theme. An empty
// string removes a style property. Nulls, unknown fields/properties, unsupported
// versions and values that could escape a Mermaid statement are rejected.
func ParseTheme(data []byte) (*Theme, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid Mermaid theme: %w", err)
	}
	if _, ok := raw.(map[string]any); !ok || hasNull(raw) {
		return nil, fmt.Errorf("theme must be an object without null values")
	}
	theme := DefaultTheme()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(theme); err != nil {
		return nil, fmt.Errorf("invalid Mermaid theme: %w", err)
	}
	if theme.Version != 1 {
		return nil, fmt.Errorf("unsupported Mermaid theme version %d (expected 1)", theme.Version)
	}
	for _, group := range []struct {
		name   string
		styles ThemeStates
	}{{"nodes", theme.Nodes}, {"edges", theme.Edges}} {
		for _, state := range []string{"context", "modified", "added", "removed"} {
			for key, value := range group.styles.style(state) {
				pattern, ok := themeProperties[key]
				if !ok || (value != "" && !pattern.MatchString(value)) {
					return nil, fmt.Errorf("invalid Mermaid theme %s.%s.%s: %q", group.name, state, key, value)
				}
			}
		}
	}
	return theme, nil
}

func hasNull(value any) bool {
	switch value := value.(type) {
	case nil:
		return true
	case map[string]any:
		for _, child := range value {
			if hasNull(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if hasNull(child) {
				return true
			}
		}
	}
	return false
}

var themeProperties = map[string]*regexp.Regexp{
	"fill":             regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|transparent|none)$`),
	"stroke":           regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|transparent|none)$`),
	"color":            regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6})$`),
	"stroke-width":     regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(px)?$`),
	"stroke-dasharray": regexp.MustCompile(`^(none|[0-9]+(\.[0-9]+)?( [0-9]+(\.[0-9]+)?)*)$`),
	"font-size":        regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(px|em|rem|%)$`),
	"font-weight":      regexp.MustCompile(`^(normal|bold|[1-9]00)$`),
	"font-style":       regexp.MustCompile(`^(normal|italic)$`),
	"font-family":      regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9 -]*$`),
}

func themeStyle(style map[string]string) string {
	keys := make([]string, 0, len(style))
	for key, value := range style {
		if value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+":"+style[key])
	}
	return strings.Join(parts, ",")
}
