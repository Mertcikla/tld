package watch

import (
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/analyzer"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// coverageSourceExt lists source extensions beyond the analyzer's supported
// languages, so coverage does not silently ignore whole ecosystems.
var coverageSourceExt = map[string]bool{
	".rb": true, ".php": true, ".swift": true, ".kt": true, ".kts": true,
	".scala": true, ".sh": true, ".bash": true, ".zsh": true, ".pl": true,
	".lua": true, ".r": true, ".dart": true, ".cs": true, ".fs": true,
	".ex": true, ".exs": true, ".clj": true, ".hs": true, ".erl": true,
	".groovy": true, ".sql": true, ".m": true, ".mm": true, ".vue": true,
	".svelte": true, ".proto": true,
}

// isSourceFile reports whether a changed path is application source code worth
// reconciling against architecture. Docs, config, lockfiles and vendored code
// are excluded so dependency/CI PRs do not look like coverage failures.
func isSourceFile(path string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	if normalized == "" {
		return false
	}
	lower := strings.ToLower(normalized)
	for _, prefix := range []string{"vendor/", "node_modules/", "third_party/", "dist/", "build/", "generated/"} {
		if strings.HasPrefix(lower, prefix) || strings.Contains(lower, "/"+prefix) {
			return false
		}
	}
	if _, ok := analyzer.DetectLanguage(normalized); ok {
		return true
	}
	return coverageSourceExt[strings.ToLower(filepath.Ext(normalized))]
}

// bindingClass ranks how specifically a binding locates a change. Higher ranks
// are more specific and therefore produce a more useful impact diagram.
type bindingClass int

const (
	bindingClassFolder bindingClass = iota
	bindingClassGlob
	bindingClassFile
	bindingClassSymbol
)

// classifyBinding describes a strong binding's specificity and, for folder
// bindings, how deeply nested the folder is.
func classifyBinding(binding CodeBinding) (bindingClass, int) {
	pattern := strings.TrimSpace(filepathToSlash(binding.Pattern))
	if pattern == "" {
		return bindingClassFolder, 1
	}
	if isFolderBindingPattern(pattern) {
		return bindingClassFolder, folderBindingDepth(pattern)
	}
	if strings.TrimSpace(binding.Symbol) != "" {
		return bindingClassSymbol, 0
	}
	if strings.ContainsAny(pattern, "*?[") {
		return bindingClassGlob, 0
	}
	return bindingClassFile, 0
}

func isFolderBindingPattern(pattern string) bool {
	return strings.HasSuffix(pattern, "/**") || strings.HasSuffix(pattern, "/")
}

func folderBindingDepth(pattern string) int {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(pattern, "/**"), "/")
	depth := 0
	for _, segment := range strings.Split(trimmed, "/") {
		if strings.TrimSpace(segment) != "" {
			depth++
		}
	}
	if depth < 1 {
		return 1
	}
	return depth
}

// bindingWeight converts a binding class into a 0..1 coverage contribution.
// Symbol-level ownership scores highest, then exact files, then file globs, then
// folders; each extra folder level reduces the contribution further. The score
// is meant to signal how precisely the diagram will describe the change, not
// merely whether a file is reached by some binding.
func bindingWeight(class bindingClass, depth int) float64 {
	switch class {
	case bindingClassSymbol:
		return 1
	case bindingClassFile:
		return 0.7
	case bindingClassGlob:
		return 0.55
	default:
		if depth < 1 {
			depth = 1
		}
		weight := 0.4 * math.Pow(0.8, float64(depth-1))
		if weight < 0.1 {
			return 0.1
		}
		return weight
	}
}

func buildCoverage(opts ImpactOptions, sourceTotal, sourceBound int, sourceScore float64, sourceWeak, nonSource int, unmappedSource, weakSource []string) Coverage {
	coverage := Coverage{
		Applicable:       sourceTotal > 0,
		SourceFiles:      sourceTotal,
		BoundSourceFiles: sourceBound,
		WeakSourceFiles:  sourceWeak,
		UnmappedSource:   len(unmappedSource),
		NonSourceFiles:   nonSource,
		TotalElements:    len(opts.Elements),
	}
	for _, element := range opts.Elements {
		if element != nil && strings.TrimSpace(element.FilePath) != "" {
			coverage.AnchoredElements++
		}
	}
	if sourceTotal == 0 {
		coverage.Complete = true
		coverage.Percent = 100
		coverage.Confidence = "none"
		return coverage
	}
	coverage.Score = sourceScore / float64(sourceTotal)
	coverage.Percent = int(math.Round(coverage.Score * 100))
	switch {
	case coverage.Score >= 0.95:
		coverage.Confidence = "high"
	case coverage.Score >= 0.7:
		coverage.Confidence = "medium"
	default:
		coverage.Confidence = "low"
	}
	coverage.Complete = len(unmappedSource) == 0 && len(weakSource) == 0
	coverage.Gaps = coverageGaps(opts, unmappedSource, weakSource)
	return coverage
}

func coverageGaps(opts ImpactOptions, unmapped, weak []string) []CoverageGap {
	best := map[string]BindingSuggestion{}
	for _, suggestion := range opts.Suggestions {
		if strings.TrimSpace(suggestion.File) == "" {
			continue
		}
		if current, ok := best[suggestion.File]; !ok || suggestion.Score > current.Score {
			best[suggestion.File] = suggestion
		}
	}
	build := func(file, reason string) CoverageGap {
		gap := CoverageGap{
			File:   file,
			Change: string(opts.ChangedFiles[file]),
			Reason: reason,
		}
		if suggestion, ok := best[file]; ok {
			gap.SuggestedRef = suggestion.ElementRef
			gap.SuggestedName = suggestion.ElementName
			gap.SuggestedScore = suggestion.Score
			if element := opts.Elements[suggestion.ElementRef]; element != nil {
				gap.CurrentPattern = strings.TrimSpace(element.FilePath)
			}
		}
		name := suggestedElementName(file)
		gap.NewElementName = name
		gap.NewElementRef = workspace.Slugify(name)
		return gap
	}
	var gaps []CoverageGap
	for _, file := range unmapped {
		gaps = append(gaps, build(file, "no architecture element owns this file"))
	}
	for _, file := range weak {
		gaps = append(gaps, build(file, "only a weak name match; add an explicit path binding"))
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].File < gaps[j].File })
	return gaps
}

// suggestedElementName derives a readable element name from a file path, e.g.
// "src/requests/_internal_utils.py" -> "Internal Utils".
func suggestedElementName(file string) string {
	base := filepath.Base(file)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.Trim(base, "_- .")
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '_' || r == '-' || r == '.' || r == ' ' })
	if len(parts) == 0 {
		return base
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
