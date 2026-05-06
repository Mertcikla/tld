package codeowners

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var candidateFiles = []string{
	"CODEOWNERS",
	filepath.Join(".github", "CODEOWNERS"),
	filepath.Join("docs", "CODEOWNERS"),
}

type Matcher struct {
	rules []rule
}

type rule struct {
	pattern string
	owners  []string
}

func Load(repoRoot string) (*Matcher, error) {
	for _, name := range candidateFiles {
		path := filepath.Join(repoRoot, name)
		data, err := os.ReadFile(path)
		if err == nil {
			return Parse(string(data)), nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return &Matcher{}, nil
}

func Parse(data string) *Matcher {
	scanner := bufio.NewScanner(strings.NewReader(data))
	var rules []rule
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pattern := normalizePattern(fields[0])
		if pattern == "" {
			continue
		}
		owners := parseOwners(fields[1:])
		if len(owners) == 0 {
			continue
		}
		rules = append(rules, rule{pattern: pattern, owners: owners})
	}
	return &Matcher{rules: rules}
}

func (m *Matcher) TagsForPath(path string) []string {
	if m == nil {
		return nil
	}
	clean := normalizePath(path)
	if clean == "" {
		return nil
	}
	var owners []string
	for _, rule := range m.rules {
		if rule.matches(clean) {
			owners = rule.owners
		}
	}
	return ownerTags(owners)
}

func parseOwners(fields []string) []string {
	seen := map[string]struct{}{}
	var owners []string
	for _, field := range fields {
		if strings.HasPrefix(field, "#") {
			break
		}
		if !strings.Contains(field, "@") {
			continue
		}
		if idx := strings.IndexByte(field, ':'); idx >= 0 {
			field = field[:idx]
		}
		field = strings.TrimSpace(field)
		if field == "" || !strings.HasPrefix(field, "@") {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		owners = append(owners, field)
	}
	sort.Strings(owners)
	return owners
}

func ownerTags(owners []string) []string {
	if len(owners) == 0 {
		return nil
	}
	tags := make([]string, 0, len(owners))
	for _, owner := range owners {
		tags = append(tags, "owner:"+owner)
	}
	sort.Strings(tags)
	return tags
}

func normalizePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "!" {
		return ""
	}
	pattern = strings.TrimPrefix(pattern, "!")
	rooted := strings.HasPrefix(pattern, "/")
	dirPattern := strings.HasSuffix(pattern, "/")
	pattern = strings.Trim(pattern, "/")
	pattern = normalizePath(pattern)
	if pattern == "" {
		return ""
	}
	if rooted {
		pattern = "/" + pattern
	}
	if dirPattern {
		pattern += "/"
	}
	return pattern
}

func normalizePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	path = strings.TrimPrefix(path, "./")
	path = strings.Trim(path, "/")
	if path == "." {
		return ""
	}
	return path
}

func (r rule) matches(candidate string) bool {
	pattern := r.pattern
	rooted := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	dirPattern := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return false
	}
	if rooted {
		return matchAnchored(pattern, candidate, dirPattern)
	}
	if !strings.Contains(pattern, "/") {
		for _, part := range strings.Split(candidate, "/") {
			if matchSegment(pattern, part) {
				return true
			}
		}
		return false
	}
	if matchAnchored(pattern, candidate, dirPattern) {
		return true
	}
	parts := strings.Split(candidate, "/")
	for i := 1; i < len(parts); i++ {
		if matchAnchored(pattern, strings.Join(parts[i:], "/"), dirPattern) {
			return true
		}
	}
	return false
}

func matchAnchored(pattern, candidate string, dirPattern bool) bool {
	if dirPattern && (candidate == pattern || strings.HasPrefix(candidate, pattern+"/")) {
		return true
	}
	if matchGlob(pattern, candidate) {
		return true
	}
	if !strings.ContainsAny(pattern, "*?[") && (candidate == pattern || strings.HasPrefix(candidate, pattern+"/")) {
		return true
	}
	return patternOwnsCandidateFolder(pattern, candidate)
}

func patternOwnsCandidateFolder(pattern, candidate string) bool {
	if strings.ContainsAny(candidate, "*?[") || candidate == "" {
		return false
	}
	staticPrefix := pattern
	if idx := strings.IndexAny(staticPrefix, "*?["); idx >= 0 {
		staticPrefix = staticPrefix[:idx]
	}
	staticPrefix = strings.Trim(staticPrefix, "/")
	return staticPrefix != "" && (staticPrefix == candidate || strings.HasPrefix(staticPrefix, candidate+"/"))
}

func matchSegment(pattern, candidate string) bool {
	ok, err := filepath.Match(pattern, candidate)
	return err == nil && ok
}

func matchGlob(pattern, candidate string) bool {
	patternParts := strings.Split(pattern, "/")
	candidateParts := strings.Split(candidate, "/")
	if len(patternParts) != len(candidateParts) {
		return false
	}
	for i := range patternParts {
		if !matchSegment(patternParts[i], candidateParts[i]) {
			return false
		}
	}
	return true
}
