package parser

import (
	"net/url"
	"regexp"
	"strings"
)

var dynamicSegment = regexp.MustCompile(`^(\{[^}]+\}|:[A-Za-z_][A-Za-z0-9_]*|<[^>]+>|[0-9]+)$`)

// normalizeRoute keeps static path segments and canonicalizes parameter spans.
func normalizeRoute(raw string) string {
	path := raw
	if u, e := url.Parse(raw); e == nil && u.IsAbs() {
		path = u.Path
	}
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	parts := strings.Split(path, "/")
	for i, v := range parts {
		if dynamicSegment.MatchString(v) {
			parts[i] = "{}"
		}
	}
	normalized := strings.TrimSuffix(strings.Join(parts, "/"), "/")
	if normalized == "" {
		return "/"
	}
	return normalized
}

// normalizeTopic preserves case and hierarchy, which can be significant to a
// broker, while removing presentation-only whitespace and duplicate slashes.
// Retained for topic Fact normalization; referenced once topic projection lands.
//
//nolint:unused
func normalizeTopic(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, "\"'"))
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	return strings.TrimSuffix(value, "/")
}
