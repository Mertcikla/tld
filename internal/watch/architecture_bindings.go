package watch

import (
	"path"
	"sort"
	"strings"
)

const (
	minArchitectureBindingConfidence = 0.72
	maxArchitectureBindingTargets    = 12
)

func resolveArchitectureBindings(repo Repository, architecture architectureModel, targets []ArchitectureBindingTarget) []ArchitectureBinding {
	if len(architecture.Components) == 0 || len(targets) == 0 {
		return nil
	}
	var bindings []ArchitectureBinding
	for _, component := range sortedArchitectureComponents(architecture.Components) {
		scored := scoreArchitectureTargets(repo, component, targets)
		if len(scored) == 0 {
			continue
		}
		if architectureTopTargetsAmbiguous(scored) {
			continue
		}
		limit := min(maxArchitectureBindingTargets, len(scored))
		for i, candidate := range scored[:limit] {
			role := "source"
			if i == 0 {
				role = "primary"
			}
			bindings = append(bindings, ArchitectureBinding{
				RepositoryID:       repo.ID,
				ComponentKey:       component.Key,
				TargetRepositoryID: candidate.Target.RepositoryID,
				TargetOwnerType:    candidate.Target.OwnerType,
				TargetOwnerKey:     candidate.Target.OwnerKey,
				TargetResourceType: candidate.Target.ResourceType,
				TargetResourceID:   candidate.Target.ResourceID,
				Role:               role,
				Confidence:         candidate.Score,
				Evidence:           candidate.Evidence,
			})
		}
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].ComponentKey == bindings[j].ComponentKey {
			if bindings[i].Role == bindings[j].Role {
				return bindings[i].TargetOwnerKey < bindings[j].TargetOwnerKey
			}
			return architectureBindingRoleRank(bindings[i].Role) < architectureBindingRoleRank(bindings[j].Role)
		}
		return bindings[i].ComponentKey < bindings[j].ComponentKey
	})
	return bindings
}

type scoredArchitectureTarget struct {
	Target   ArchitectureBindingTarget
	Score    float64
	Evidence []ArchitectureBindingEvidence
}

func scoreArchitectureTargets(repo Repository, component *architectureComponent, targets []ArchitectureBindingTarget) []scoredArchitectureTarget {
	var out []scoredArchitectureTarget
	for _, target := range targets {
		score, evidence := architectureTargetScore(repo, component, target)
		if score < minArchitectureBindingConfidence {
			continue
		}
		out = append(out, scoredArchitectureTarget{Target: target, Score: score, Evidence: evidence})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		leftRank := architectureTargetOwnerRank(out[i].Target.OwnerType)
		rightRank := architectureTargetOwnerRank(out[j].Target.OwnerType)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if out[i].Target.RepositoryID != out[j].Target.RepositoryID {
			if out[i].Target.RepositoryID == repo.ID {
				return true
			}
			if out[j].Target.RepositoryID == repo.ID {
				return false
			}
		}
		return out[i].Target.OwnerKey < out[j].Target.OwnerKey
	})
	return out
}

func architectureTargetScore(repo Repository, component *architectureComponent, target ArchitectureBindingTarget) (float64, []ArchitectureBindingEvidence) {
	if component == nil {
		return 0, nil
	}
	var score float64
	var evidence []ArchitectureBindingEvidence
	add := func(kind, detail string, value float64) {
		if value <= 0 {
			return
		}
		score += value
		evidence = append(evidence, ArchitectureBindingEvidence{Kind: kind, Detail: detail, Score: value})
	}
	if target.RepositoryID == repo.ID {
		add("same-repository", repo.DisplayName, 0.04)
	}
	componentVariants := architectureComponentVariants(component)
	targetVariants := architectureTargetVariants(target)
	if architectureAnyExactVariant(componentVariants, targetVariants) {
		add("name-match", target.Name, 0.42)
	}
	if architectureAnyPathSegmentVariant(componentVariants, target.FilePath) {
		add("path-token-match", target.FilePath, 0.30)
	}
	if architectureTokenOverlap(componentVariants, architectureTargetTokens(target)) >= 2 {
		add("token-overlap", target.Name, 0.16)
	}
	for _, evidencePath := range architectureComponentEvidencePaths(component) {
		switch {
		case target.FilePath != "" && sameArchitecturePath(target.FilePath, evidencePath):
			add("evidence-path", evidencePath, 0.78)
		case target.OwnerType == "folder" && target.FilePath != "" && architecturePathContains(target.FilePath, evidencePath):
			add("evidence-under-target", evidencePath, 0.66)
		case target.FilePath != "" && sameArchitectureDir(target.FilePath, evidencePath):
			add("evidence-directory", evidencePath, 0.30)
		}
	}
	if target.OwnerType == "symbol" && architectureAnyExactVariant(componentVariants, architectureNameTokens(target.Name)) {
		add("symbol-name-match", target.Name, 0.18)
	}
	if target.OwnerType == "fact" && architectureAnyExactVariant(componentVariants, architectureNameTokens(target.Name)) {
		add("fact-name-match", target.Name, 0.20)
	}
	if target.OwnerType == "fact-summary" {
		score -= 0.08
	}
	if score > 1 {
		score = 1
	}
	return score, evidence
}

func architectureTopTargetsAmbiguous(scored []scoredArchitectureTarget) bool {
	if len(scored) < 2 {
		return false
	}
	top := scored[0]
	second := scored[1]
	if top.Score-second.Score > 0.02 {
		return false
	}
	return !architectureBindingHasConcreteEvidence(top.Evidence) && !architectureBindingHasConcreteEvidence(second.Evidence)
}

func architectureBindingHasConcreteEvidence(evidence []ArchitectureBindingEvidence) bool {
	for _, item := range evidence {
		switch item.Kind {
		case "evidence-path", "evidence-under-target", "evidence-directory":
			return true
		}
	}
	return false
}

func architectureComponentVariants(component *architectureComponent) []string {
	set := map[string]struct{}{}
	addTokens := func(tokens []string) {
		if len(tokens) == 0 {
			return
		}
		joined := strings.Join(tokens, "-")
		compact := strings.ReplaceAll(joined, "-", "")
		set[joined] = struct{}{}
		set[compact] = struct{}{}
		if root, ok := architectureServiceRootFromTokens(tokens, true); ok {
			set[root] = struct{}{}
			set[strings.ReplaceAll(root, "-", "")] = struct{}{}
		}
	}
	addTokens(architectureNameTokens(component.Name))
	for _, evidence := range component.Evidence {
		addTokens(architectureNameTokens(evidence.Note))
	}
	return sortedKeys(set)
}

func architectureTargetVariants(target ArchitectureBindingTarget) []string {
	set := map[string]struct{}{}
	for _, value := range []string{target.Name, path.Base(target.FilePath)} {
		tokens := architectureNameTokens(value)
		if len(tokens) == 0 {
			continue
		}
		joined := strings.Join(tokens, "-")
		set[joined] = struct{}{}
		set[strings.ReplaceAll(joined, "-", "")] = struct{}{}
	}
	return sortedKeys(set)
}

func architectureTargetTokens(target ArchitectureBindingTarget) []string {
	set := map[string]struct{}{}
	for _, value := range []string{target.Name, target.Kind, target.Language} {
		for _, token := range architectureNameTokens(value) {
			set[token] = struct{}{}
		}
	}
	for part := range strings.SplitSeq(filepathToSlash(target.FilePath), "/") {
		for _, token := range architectureNameTokens(part) {
			set[token] = struct{}{}
		}
	}
	for _, tag := range target.Tags {
		for _, token := range architectureNameTokens(tag) {
			set[token] = struct{}{}
		}
	}
	return sortedKeys(set)
}

func architectureComponentEvidencePaths(component *architectureComponent) []string {
	set := map[string]struct{}{}
	if path := cleanArchitecturePath(component.FilePath); path != "" {
		set[path] = struct{}{}
	}
	for _, evidence := range component.Evidence {
		if path := cleanArchitecturePath(evidence.Path); path != "" {
			set[path] = struct{}{}
		}
	}
	return sortedKeys(set)
}

func architectureAnyExactVariant(left, right []string) bool {
	set := map[string]struct{}{}
	for _, item := range left {
		if item != "" {
			set[item] = struct{}{}
		}
	}
	for _, item := range right {
		if _, ok := set[item]; ok {
			return true
		}
	}
	return false
}

func architectureAnyPathSegmentVariant(variants []string, value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	set := map[string]struct{}{}
	for _, variant := range variants {
		if variant != "" {
			set[variant] = struct{}{}
		}
	}
	for part := range strings.SplitSeq(filepathToSlash(value), "/") {
		tokens := architectureNameTokens(part)
		joined := strings.Join(tokens, "-")
		if _, ok := set[joined]; ok {
			return true
		}
		if _, ok := set[strings.ReplaceAll(joined, "-", "")]; ok {
			return true
		}
	}
	return false
}

func architectureTokenOverlap(componentVariants []string, targetTokens []string) int {
	set := map[string]struct{}{}
	for _, variant := range componentVariants {
		for token := range strings.SplitSeq(variant, "-") {
			if token != "" && !architectureRoleToken(token) {
				set[token] = struct{}{}
			}
		}
	}
	var count int
	for _, token := range targetTokens {
		if _, ok := set[token]; ok {
			count++
		}
	}
	return count
}

func cleanArchitecturePath(value string) string {
	value = filepathToSlash(strings.TrimSpace(value))
	if value == "" || value == "." {
		return ""
	}
	return path.Clean(value)
}

func sameArchitecturePath(a, b string) bool {
	return cleanArchitecturePath(a) == cleanArchitecturePath(b)
}

func sameArchitectureDir(a, b string) bool {
	a, b = cleanArchitecturePath(a), cleanArchitecturePath(b)
	if a == "" || b == "" {
		return false
	}
	return path.Dir(a) == path.Dir(b)
}

func architecturePathContains(parent, child string) bool {
	parent, child = cleanArchitecturePath(parent), cleanArchitecturePath(child)
	if parent == "" || child == "" || parent == "." {
		return false
	}
	return child == parent || strings.HasPrefix(child, parent+"/")
}

func architectureTargetOwnerRank(ownerType string) int {
	switch ownerType {
	case "folder":
		return 0
	case "file":
		return 1
	case "symbol":
		return 2
	case "fact":
		return 3
	default:
		return 4
	}
}

func architectureBindingRoleRank(role string) int {
	if role == "primary" {
		return 0
	}
	return 1
}
