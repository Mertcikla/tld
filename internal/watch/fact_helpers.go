package watch

import (
	"encoding/json"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func inferredComponentFromFact(fact Fact, attrs map[string]string) string {
	if value := attrs["source"]; value != "" {
		return value
	}
	return componentFromPath(fact.FilePath)
}

func componentFromPath(rel string) string {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	parts := strings.Split(rel, "/")
	for i := len(parts) - 2; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "." && part != "" && !architecturePathLayoutToken(part) {
			return part
		}
	}
	return ""
}

func architecturePathLayoutToken(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "app", "apps", "cmd", "internal", "lib", "libs", "pkg", "packages", "service", "services", "source", "src":
		return true
	default:
		return false
	}
}

func normalizedArchitectureConnectorDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "backward":
		return "backward"
	case "both", "bidirectional":
		return "both"
	case "none":
		return "none"
	default:
		return "forward"
	}
}

func mustRel(root, absPath string) string {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

func factAttributes(fact Fact) map[string]string {
	var attrs map[string]string
	if fact.AttributesJSON != "" {
		_ = json.Unmarshal([]byte(fact.AttributesJSON), &attrs)
	}
	if attrs == nil {
		attrs = map[string]string{}
	}
	return attrs
}

var nonEndpointNameRe = regexp.MustCompile(`[^a-z0-9-]+`)

func normalizeFactEndpoint(value string) string {
	value = strings.Trim(strings.TrimSpace(strings.ToLower(value)), `"'`)
	if value == "" || strings.Contains(value, "{{") || strings.HasPrefix(value, "$") {
		return ""
	}
	if strings.Contains(value, "://") {
		value = strings.SplitN(value, "://", 2)[1]
	}
	if strings.Contains(value, ":") {
		value = strings.Split(value, ":")[0]
	}
	value = strings.Split(value, ".")[0]
	value = strings.Trim(value, "/")
	value = nonEndpointNameRe.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
