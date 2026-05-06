// Package ignore provides rule-based filtering for excluded paths and symbols
// used by tld analyze and tld check commands.
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Rules holds gitignore-style exclusion patterns loaded from the workspace configuration file.
type Rules struct {
	Exclude  []string  `yaml:"exclude,omitempty"`
	Patterns []Pattern `yaml:"-"`
}

type Pattern struct {
	Value  string
	Negate bool
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
	seenPatterns := make(map[Pattern]struct{})
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
		for _, pattern := range ruleSet.Patterns {
			pattern.Value = strings.TrimSpace(pattern.Value)
			if pattern.Value == "" {
				continue
			}
			if _, ok := seenPatterns[pattern]; ok {
				continue
			}
			seenPatterns[pattern] = struct{}{}
			merged.Patterns = append(merged.Patterns, pattern)
		}
	}
	if len(merged.Exclude) == 0 && len(merged.Patterns) == 0 {
		return nil
	}
	return merged
}

func LoadGitIgnore(root string) (*Rules, error) {
	root = filepath.Clean(root)
	var patterns []Pattern
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != ".gitignore" {
			return nil
		}
		base, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		if base == "." {
			base = ""
		}
		filePatterns, err := readGitIgnoreFile(path, base)
		if err != nil {
			return err
		}
		patterns = append(patterns, filePatterns...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return nil, nil
	}
	return &Rules{Patterns: patterns}, nil
}

// ShouldIgnorePath returns true if the given file or folder path matches any exclusion pattern.
// The path can be absolute or relative; matching is performed against both the full path and base name.
func (r *Rules) ShouldIgnorePath(path string) bool {
	if r == nil {
		return shouldIgnorePathWithPatterns(path, implicitPathExcludes)
	}
	if shouldIgnorePathWithPatterns(path, append(append([]string{}, implicitPathExcludes...), r.Exclude...)) {
		return true
	}
	return shouldIgnorePathWithOrderedPatterns(path, r.Patterns)
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

func shouldIgnorePathWithOrderedPatterns(path string, patterns []Pattern) bool {
	path = normalizePath(path)
	ignored := false
	for _, pattern := range patterns {
		if matchPathPattern(path, pattern.Value) {
			ignored = !pattern.Negate
		}
	}
	return ignored
}

func readGitIgnoreFile(path, base string) ([]Pattern, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	base = normalizePath(base)
	scanner := bufio.NewScanner(file)
	var patterns []Pattern
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := strings.HasPrefix(line, "!")
		if negate {
			line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
		}
		line = strings.TrimPrefix(line, "\\")
		for _, pattern := range expandGitIgnorePattern(base, line) {
			patterns = append(patterns, Pattern{Value: pattern, Negate: negate})
		}
	}
	return patterns, scanner.Err()
}

func expandGitIgnorePattern(base, pattern string) []string {
	pattern = normalizePattern(pattern)
	if pattern == "" || pattern == "/" {
		return nil
	}
	rooted := strings.HasPrefix(pattern, "/")
	dirOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.Trim(pattern, "/")
	if pattern == "" {
		return nil
	}
	hasSlash := strings.Contains(pattern, "/")
	var expanded []string
	add := func(value string) {
		value = normalizePattern(value)
		value = strings.TrimPrefix(value, "/")
		if value == "" {
			return
		}
		if dirOnly && !strings.HasSuffix(value, "/") {
			value += "/"
		}
		expanded = append(expanded, value)
	}
	if base != "" {
		if rooted || hasSlash {
			add(base + "/" + pattern)
		} else {
			add(base + "/" + pattern)
			add(base + "/**/" + pattern)
		}
		return expanded
	}
	if rooted || hasSlash {
		add(pattern)
	} else {
		add(pattern)
		add("**/" + pattern)
	}
	return expanded
}

func matchPathPattern(path, pattern string) bool {
	pattern = normalizePattern(pattern)
	base := filepath.Base(path)
	if matchPattern(pattern, path) || matchPattern(pattern, base) {
		return true
	}
	if before, ok := strings.CutSuffix(pattern, "/"); ok {
		trimmed := before
		if path == trimmed || strings.HasPrefix(path, trimmed+"/") || base == trimmed {
			return true
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
