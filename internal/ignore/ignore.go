// Package ignore provides rule-based filtering for excluded paths and symbols
// used by tld analyze and tld check commands.
package ignore

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Rules holds gitignore-style exclusion patterns loaded from the workspace configuration file.
type Rules struct {
	Exclude []string `yaml:"exclude,omitempty"`
}

var implicitPathExcludes = []string{
	"vendor/",
	"node_modules/",
	".venv/",
	".git/",
}

// Merge combines multiple rule sets into a single exclusion list.
func Merge(rules ...*Rules) *Rules {
	merged := &Rules{}
	seen := make(map[string]struct{})
	for _, ruleSet := range rules {
		if ruleSet == nil {
			continue
		}
		for _, pattern := range ruleSet.Exclude {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			if _, ok := seen[pattern]; ok {
				continue
			}
			seen[pattern] = struct{}{}
			merged.Exclude = append(merged.Exclude, pattern)
		}
	}
	if len(merged.Exclude) == 0 {
		return nil
	}
	return merged
}

// ShouldIgnorePath returns true if the given file or folder path matches any exclusion pattern.
// The path can be absolute or relative; matching is performed against both the full path and base name.
func (r *Rules) ShouldIgnorePath(path string) bool {
	if r == nil {
		return shouldIgnorePathWithPatterns(path, implicitPathExcludes)
	}
	return shouldIgnorePathWithPatterns(path, append(append([]string{}, implicitPathExcludes...), r.Exclude...))
}

func shouldIgnorePathWithPatterns(path string, patterns []string) bool {
	path = normalizePath(path)
	base := filepath.Base(path)
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		normalizedPattern := normalizePattern(pattern)
		if matchPattern(normalizedPattern, path) || matchPattern(normalizedPattern, base) {
			return true
		}
		if before, ok := strings.CutSuffix(normalizedPattern, "/"); ok {
			trimmed := before
			if path == trimmed || strings.HasPrefix(path, trimmed+"/") || base == trimmed {
				return true
			}
		}
	}
	return false
}

// ShouldIgnoreFile returns true if the given file path is excluded.
func (r *Rules) ShouldIgnoreFile(path string) bool {
	return r.ShouldIgnorePath(path)
}

// ShouldIgnoreFolder returns true if the given folder path is excluded.
func (r *Rules) ShouldIgnoreFolder(path string) bool {
	return r.ShouldIgnorePath(path)
}

// ShouldIgnoreSymbol returns true if the given symbol name matches any exclusion pattern.
func (r *Rules) ShouldIgnoreSymbol(name string) bool {
	if r == nil {
		return false
	}
	name = strings.TrimSpace(name)
	for _, pattern := range r.Exclude {
		if pattern == "" {
			continue
		}
		normalizedPattern := normalizePattern(pattern)
		if matchPattern(normalizedPattern, name) {
			return true
		}
	}
	return false
}

// matchPattern matches a value against a pattern using gitignore-style glob syntax.
// It falls back to exact string equality if the glob is invalid.
func matchPattern(pattern, value string) bool {
	matched, err := doublestar.Match(pattern, value)
	if err != nil {
		return pattern == value
	}
	return matched
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return path
}

func normalizePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = filepath.ToSlash(pattern)
	pattern = strings.TrimPrefix(pattern, "./")
	return pattern
}
