package watch

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// EvidenceLevel describes how strongly a finding is grounded in code.
type EvidenceLevel string

const (
	EvidenceStrong   EvidenceLevel = "strong"
	EvidenceModerate EvidenceLevel = "moderate"
	EvidenceWeak     EvidenceLevel = "weak"
)

// ImpactEvidence is a single justification for a binding or finding.
type ImpactEvidence struct {
	Level    EvidenceLevel `json:"level"`
	Kind     string        `json:"kind"`
	Path     string        `json:"path,omitempty"`
	Detail   string        `json:"detail,omitempty"`
	Observed bool          `json:"observed"`
}

// ImpactElement is an authored architecture element touched by the change set.
type ImpactElement struct {
	Ref      string           `json:"ref"`
	Name     string           `json:"name"`
	Kind     string           `json:"kind"`
	Owner    string           `json:"owner,omitempty"`
	Evidence []ImpactEvidence `json:"evidence,omitempty"`
}

// ImpactEdge is a relationship between two impacted elements. Declared edges
// come from authored connectors; observed edges are inferred from code evidence
// and must not be treated as architecture.
type ImpactEdge struct {
	SourceRef string `json:"source_ref"`
	TargetRef string `json:"target_ref"`
	Label     string `json:"label,omitempty"`
	Origin    string `json:"origin"` // "declared" or "observed"
	Observed  bool   `json:"observed"`
}

// ChangedFile is a file in the analyzed diff with its change type and line
// counts. Change is one of added, updated, or deleted.
type ChangedFile struct {
	Path    string `json:"path"`
	Change  string `json:"change"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

// ImpactReport is the deterministic output of `tld impact`.
type ImpactReport struct {
	Base         string          `json:"base,omitempty"`
	Head         string          `json:"head,omitempty"`
	RepoRoot     string          `json:"repo_root,omitempty"`
	Changed      []ImpactElement `json:"changed"`
	Candidates   []ImpactElement `json:"candidates,omitempty"`
	Related      []ImpactElement `json:"related"`
	Edges        []ImpactEdge    `json:"edges,omitempty"`
	Unmapped     []string        `json:"unmapped"`
	Coverage     Coverage        `json:"coverage"`
	ChangedFiles []ChangedFile   `json:"changed_files,omitempty"`
}

// Coverage describes how completely a change set could be reconciled against
// authored architecture. It is computed from deterministic path bindings only
// (never from the evidence store), so the score is stable across runs.
type Coverage struct {
	Applicable       bool          `json:"applicable"`
	Complete         bool          `json:"complete"`
	Score            float64       `json:"score"`
	Percent          int           `json:"percent"`
	Confidence       string        `json:"confidence"` // high | medium | low | none
	SourceFiles      int           `json:"source_files"`
	BoundSourceFiles int           `json:"bound_source_files"`
	WeakSourceFiles  int           `json:"weak_source_files"`
	UnmappedSource   int           `json:"unmapped_source_files"`
	NonSourceFiles   int           `json:"non_source_files"`
	AnchoredElements int           `json:"anchored_elements"`
	TotalElements    int           `json:"total_elements"`
	Gaps             []CoverageGap `json:"gaps,omitempty"`
}

// CoverageGap is a changed source file that is not reliably owned by any
// architecture element, with enough context for a human or LLM agent to bind it.
type CoverageGap struct {
	File           string  `json:"file"`
	Change         string  `json:"change,omitempty"`
	Reason         string  `json:"reason"`
	SuggestedRef   string  `json:"suggested_element_ref,omitempty"`
	SuggestedName  string  `json:"suggested_element_name,omitempty"`
	SuggestedScore float64 `json:"suggested_score,omitempty"`
	CurrentPattern string  `json:"suggested_element_pattern,omitempty"`
	NewElementName string  `json:"suggested_new_element,omitempty"`
	NewElementRef  string  `json:"suggested_new_ref,omitempty"`
}

// CodeBinding connects an authored element to code paths it is expected to own.
// Bindings are auto-derived from element metadata only; they are never authored
// separately and never mutate the architecture.
type CodeBinding struct {
	ElementRef string
	Name       string
	Kind       string
	Repo       string
	Pattern    string
	Symbol     string
	Level      EvidenceLevel
}

// RelationshipEvidence is observed code evidence that two bound elements are
// related (for example a resolved call between their symbols).
type RelationshipEvidence struct {
	SourceRef string
	TargetRef string
	File      string
	Line      int
	Kind      string
	Level     EvidenceLevel
	Observed  bool
}

// BindingSuggestion is an inferred (weak) guess that unmapped code belongs to an
// authored element. It must be accepted by a human before it becomes a binding.
type BindingSuggestion struct {
	File        string
	ElementRef  string
	ElementName string
	Score       float64
}

// ImpactOptions carries everything AnalyzeImpact needs. ChangedFiles is keyed by
// repository-relative path.
type ImpactOptions struct {
	Base         string
	Head         string
	RepoRoot     string
	RemoteURL    string
	Elements     map[string]*workspace.Element
	Connectors   map[string]*workspace.Connector
	ChangedFiles map[string]tldgit.WorktreeChange
	// LineStats carries added/removed line counts for changed paths, keyed by
	// repository-relative path. Missing entries are treated as zero.
	LineStats map[string]tldgit.LineDiff
	// IncludeNameHeuristics enables weak name/path token bindings for elements
	// that declare no explicit file path. Such matches are reported as
	// candidates, never as observed changes.
	IncludeNameHeuristics bool
	Relationships         []RelationshipEvidence
	Suggestions           []BindingSuggestion
}

// DeriveBindings builds code bindings from authored element metadata. Explicit
// file paths and symbols yield strong bindings; name/path token overlap yields
// weak bindings used only when no explicit path is present.
func DeriveBindings(elements map[string]*workspace.Element, repoRoot, remoteURL string) []CodeBinding {
	refs := make([]string, 0, len(elements))
	for ref := range elements {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	var bindings []CodeBinding
	for _, ref := range refs {
		element := elements[ref]
		if element == nil {
			continue
		}
		if !repoMatches(element.Repo, remoteURL, repoRoot) {
			continue
		}
		pattern := normalizeCodePath(element.FilePath)
		if pattern != "" {
			if isDirectoryPath(repoRoot, pattern) {
				pattern = strings.TrimSuffix(pattern, "/") + "/**"
			}
			bindings = append(bindings, CodeBinding{
				ElementRef: ref,
				Name:       element.Name,
				Kind:       element.Kind,
				Repo:       element.Repo,
				Pattern:    pattern,
				Symbol:     strings.TrimSpace(element.Symbol),
				Level:      EvidenceStrong,
			})
			continue
		}
		if strings.TrimSpace(element.Name) != "" {
			bindings = append(bindings, CodeBinding{
				ElementRef: ref,
				Name:       element.Name,
				Kind:       element.Kind,
				Repo:       element.Repo,
				Level:      EvidenceWeak,
			})
		}
	}
	return bindings
}

// AnalyzeImpact reconciles a change set against authored architecture and
// produces a deterministic report. It never writes to the architecture.
func AnalyzeImpact(opts ImpactOptions) ImpactReport {
	report := ImpactReport{Base: opts.Base, Head: opts.Head, RepoRoot: opts.RepoRoot}
	report.ChangedFiles = buildChangedFiles(opts)
	bindings := DeriveBindings(opts.Elements, opts.RepoRoot, opts.RemoteURL)

	files := sortedChangePaths(opts.ChangedFiles)
	elementEvidence := map[string][]ImpactEvidence{}
	elementStrong := map[string]bool{}
	elementOrder := []string{}

	addEvidence := func(ref string, evidence ImpactEvidence) {
		if _, ok := elementEvidence[ref]; !ok {
			elementOrder = append(elementOrder, ref)
		}
		elementEvidence[ref] = append(elementEvidence[ref], evidence)
		if evidence.Level == EvidenceStrong {
			elementStrong[ref] = true
		}
	}

	sourceTotal, sourceBound, sourceWeak, nonSource := 0, 0, 0, 0
	var sourceScore float64
	var unmappedSource, weakSource []string
	for _, file := range files {
		strongMatched, weakMatched := false, false
		bestClass, bestDepth, haveBinding := bindingClassFolder, 1, false
		for _, binding := range bindings {
			var ok bool
			var kind string
			switch {
			case binding.Pattern != "":
				ok = bindingPatternMatch(binding.Pattern, file)
				kind = "path"
				if ok {
					strongMatched = true
					class, depth := classifyBinding(binding)
					if !haveBinding || class > bestClass || (class == bestClass && class == bindingClassFolder && depth > bestDepth) {
						bestClass, bestDepth, haveBinding = class, depth, true
					}
				}
			case binding.Name != "" && opts.IncludeNameHeuristics:
				ok = namePathMatch(binding.Name, file)
				kind = "name"
				if ok {
					weakMatched = true
				}
			}
			if !ok {
				continue
			}
			addEvidence(binding.ElementRef, ImpactEvidence{
				Level:    binding.Level,
				Kind:     kind,
				Path:     file,
				Detail:   bindingDetail(binding),
				Observed: binding.Level == EvidenceStrong,
			})
		}
		if !strongMatched && !weakMatched {
			report.Unmapped = append(report.Unmapped, file)
		}
		if isSourceFile(file) {
			sourceTotal++
			switch {
			case strongMatched:
				sourceBound++
				sourceScore += bindingWeight(bestClass, bestDepth)
			case weakMatched:
				sourceWeak++
				weakSource = append(weakSource, file)
			default:
				unmappedSource = append(unmappedSource, file)
			}
		} else {
			nonSource++
		}
	}

	sort.Strings(elementOrder)
	changedRefs := map[string]struct{}{}
	for _, ref := range elementOrder {
		element := opts.Elements[ref]
		if element == nil {
			continue
		}
		impactElement := ImpactElement{
			Ref:      ref,
			Name:     element.Name,
			Kind:     element.Kind,
			Owner:    element.Owner,
			Evidence: elementEvidence[ref],
		}
		if elementStrong[ref] {
			report.Changed = append(report.Changed, impactElement)
			changedRefs[ref] = struct{}{}
		} else {
			report.Candidates = append(report.Candidates, impactElement)
		}
	}

	report.Related, report.Edges = relatedImpact(opts, changedRefs)
	observedRelated, observedEdges := observedRelationshipEdges(opts, changedRefs)
	report.Related = mergeImpactElements(report.Related, observedRelated)
	report.Edges = append(report.Edges, observedEdges...)

	report.Coverage = buildCoverage(opts, sourceTotal, sourceBound, sourceScore, sourceWeak, nonSource, unmappedSource, weakSource)
	return report
}

func relatedImpact(opts ImpactOptions, changedRefs map[string]struct{}) ([]ImpactElement, []ImpactEdge) {
	if len(changedRefs) == 0 || len(opts.Connectors) == 0 {
		return nil, nil
	}
	connectorKeys := make([]string, 0, len(opts.Connectors))
	for key := range opts.Connectors {
		connectorKeys = append(connectorKeys, key)
	}
	sort.Strings(connectorKeys)

	related := map[string]*ImpactElement{}
	var edges []ImpactEdge
	for _, key := range connectorKeys {
		connector := opts.Connectors[key]
		if connector == nil {
			continue
		}
		sourceChanged := containsRef(changedRefs, connector.Source)
		targetChanged := containsRef(changedRefs, connector.Target)
		if !sourceChanged && !targetChanged {
			continue
		}
		edges = append(edges, ImpactEdge{
			SourceRef: connector.Source,
			TargetRef: connector.Target,
			Label:     connector.Label,
			Origin:    "declared",
			Observed:  false,
		})
		for _, endpoint := range []struct {
			ref     string
			changed bool
		}{
			{ref: connector.Source, changed: sourceChanged},
			{ref: connector.Target, changed: targetChanged},
		} {
			if endpoint.changed || endpoint.ref == "" {
				continue
			}
			element := opts.Elements[endpoint.ref]
			if element == nil {
				continue
			}
			entry, ok := related[endpoint.ref]
			if !ok {
				entry = &ImpactElement{Ref: endpoint.ref, Name: element.Name, Kind: element.Kind, Owner: element.Owner}
				related[endpoint.ref] = entry
			}
			entry.Evidence = append(entry.Evidence, ImpactEvidence{
				Level:    EvidenceStrong,
				Kind:     "connector",
				Detail:   "connected via " + connectorLabel(connector),
				Observed: true,
			})
		}
	}

	refs := make([]string, 0, len(related))
	for ref := range related {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	out := make([]ImpactElement, 0, len(refs))
	for _, ref := range refs {
		out = append(out, *related[ref])
	}
	return out, edges
}

// observedRelationshipEdges surfaces code-observed relationships that are not
// declared as authored connectors, so they appear on the diagram as inferred
// (dashed) edges.
func observedRelationshipEdges(opts ImpactOptions, changedRefs map[string]struct{}) ([]ImpactElement, []ImpactEdge) {
	if len(opts.Relationships) == 0 {
		return nil, nil
	}
	existing := existingConnectorPairs(opts.Connectors)
	related := map[string]*ImpactElement{}
	seen := map[string]struct{}{}
	var edges []ImpactEdge
	for _, rel := range opts.Relationships {
		if rel.SourceRef == "" || rel.TargetRef == "" || rel.SourceRef == rel.TargetRef {
			continue
		}
		if !containsRef(changedRefs, rel.SourceRef) && !containsRef(changedRefs, rel.TargetRef) {
			continue
		}
		if existing[connectorPairKey(rel.SourceRef, rel.TargetRef)] {
			continue
		}
		key := rel.SourceRef + "\x00" + rel.TargetRef
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		edges = append(edges, ImpactEdge{
			SourceRef: rel.SourceRef,
			TargetRef: rel.TargetRef,
			Label:     firstNonEmpty(strings.TrimSpace(rel.Kind), "call"),
			Origin:    "observed",
			Observed:  true,
		})
		for _, endpoint := range []string{rel.SourceRef, rel.TargetRef} {
			if containsRef(changedRefs, endpoint) {
				continue
			}
			element := opts.Elements[endpoint]
			if element == nil {
				continue
			}
			entry, ok := related[endpoint]
			if !ok {
				entry = &ImpactElement{Ref: endpoint, Name: element.Name, Kind: element.Kind, Owner: element.Owner}
				related[endpoint] = entry
			}
			entry.Evidence = append(entry.Evidence, ImpactEvidence{
				Level:    rel.Level,
				Kind:     "code",
				Path:     rel.File,
				Detail:   "observed " + firstNonEmpty(rel.Kind, "call") + " at " + codeLocation(rel.File, rel.Line),
				Observed: rel.Observed,
			})
		}
	}
	refs := make([]string, 0, len(related))
	for ref := range related {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	out := make([]ImpactElement, 0, len(refs))
	for _, ref := range refs {
		out = append(out, *related[ref])
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].SourceRef != edges[j].SourceRef {
			return edges[i].SourceRef < edges[j].SourceRef
		}
		return edges[i].TargetRef < edges[j].TargetRef
	})
	return out, edges
}

func mergeImpactElements(base, extra []ImpactElement) []ImpactElement {
	if len(extra) == 0 {
		return base
	}
	index := make(map[string]int, len(base))
	for i, element := range base {
		index[element.Ref] = i
	}
	for _, element := range extra {
		if i, ok := index[element.Ref]; ok {
			base[i].Evidence = append(base[i].Evidence, element.Evidence...)
			continue
		}
		index[element.Ref] = len(base)
		base = append(base, element)
	}
	return base
}

func existingConnectorPairs(connectors map[string]*workspace.Connector) map[string]bool {
	out := make(map[string]bool, len(connectors))
	for _, connector := range connectors {
		if connector == nil {
			continue
		}
		out[connectorPairKey(connector.Source, connector.Target)] = true
		out[connectorPairKey(connector.Target, connector.Source)] = true
	}
	return out
}

func connectorPairKey(source, target string) string {
	return strings.TrimSpace(source) + "\x00" + strings.TrimSpace(target)
}

func connectorLabel(connector *workspace.Connector) string {
	if connector == nil {
		return ""
	}
	if label := strings.TrimSpace(connector.Label); label != "" {
		return label
	}
	if view := strings.TrimSpace(connector.View); view != "" {
		return view
	}
	return "connector"
}

func bindingDetail(binding CodeBinding) string {
	if binding.Symbol != "" {
		return binding.Symbol
	}
	if binding.Pattern != "" {
		return binding.Pattern
	}
	return "name match"
}

func codeLocation(file string, line int) string {
	if file == "" {
		return ""
	}
	if line > 0 {
		return file + ":" + itoa(line)
	}
	return file
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func sortedChangePaths(changes map[string]tldgit.WorktreeChange) []string {
	files := make([]string, 0, len(changes))
	for file := range changes {
		file = normalizeCodePath(file)
		if file == "" {
			continue
		}
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

func buildChangedFiles(opts ImpactOptions) []ChangedFile {
	if len(opts.ChangedFiles) == 0 {
		return nil
	}
	files := make([]ChangedFile, 0, len(opts.ChangedFiles))
	for path, change := range opts.ChangedFiles {
		normalized := normalizeCodePath(path)
		if normalized == "" {
			continue
		}
		stats := opts.LineStats[path]
		files = append(files, ChangedFile{
			Path:    normalized,
			Change:  string(change),
			Added:   stats.Added,
			Removed: stats.Removed,
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func containsRef(refs map[string]struct{}, ref string) bool {
	_, ok := refs[strings.TrimSpace(ref)]
	return ok
}

func normalizeCodePath(value string) string {
	value = strings.TrimSpace(filepathToSlash(value))
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	value = path.Clean(value)
	if value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return ""
	}
	return value
}

func isDirectoryPath(repoRoot, rel string) bool {
	if repoRoot == "" || rel == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	return err == nil && info.IsDir()
}

func bindingPatternMatch(pattern, file string) bool {
	rawPattern := strings.TrimSpace(filepathToSlash(pattern))
	file = normalizeCodePath(file)
	if file == "" || rawPattern == "" {
		return false
	}
	if strings.HasSuffix(rawPattern, "/**") {
		prefix := normalizeCodePath(strings.TrimSuffix(rawPattern, "/**"))
		if prefix == "" {
			return true
		}
		return file == prefix || strings.HasPrefix(file, prefix+"/")
	}
	if strings.HasSuffix(rawPattern, "/") {
		prefix := normalizeCodePath(rawPattern)
		if prefix == "" {
			return true
		}
		return file == prefix || strings.HasPrefix(file, prefix+"/")
	}
	pattern = normalizeCodePath(rawPattern)
	if pattern == "" {
		return false
	}
	if pattern == file {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		rest := strings.TrimPrefix(pattern, "**/")
		return file == rest || strings.HasSuffix(file, "/"+rest)
	}
	if strings.ContainsAny(pattern, "*?[") {
		matched, err := path.Match(pattern, file)
		return err == nil && matched
	}
	return false
}

var impactGenericTokens = map[string]struct{}{
	"app": {}, "apps": {}, "api": {}, "apis": {}, "service": {}, "services": {},
	"server": {}, "client": {}, "web": {}, "core": {}, "main": {}, "internal": {},
	"cmd": {}, "pkg": {}, "src": {}, "lib": {}, "libs": {}, "util": {}, "utils": {},
	"common": {}, "config": {}, "test": {}, "tests": {}, "model": {}, "models": {},
	"handler": {}, "handlers": {}, "controller": {}, "controllers": {}, "repository": {},
	"repositories": {}, "data": {}, "db": {}, "database": {},
}

func namePathMatch(name, file string) bool {
	nameTokens := impactNameTokens(name)
	if len(nameTokens) == 0 {
		return false
	}
	fileTokens := impactPathTokens(file)
	if len(fileTokens) == 0 {
		return false
	}
	for _, token := range nameTokens {
		if _, ok := fileTokens[token]; ok {
			return true
		}
	}
	return false
}

func impactNameTokens(value string) []string {
	raw := splitImpactTokens(value)
	out := raw[:0]
	for _, token := range raw {
		if _, generic := impactGenericTokens[token]; generic {
			continue
		}
		out = append(out, token)
	}
	return out
}

func impactPathTokens(file string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, segment := range strings.Split(file, "/") {
		for _, token := range splitImpactTokens(segment) {
			out[token] = struct{}{}
		}
	}
	return out
}

func splitImpactTokens(value string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() >= 3 {
			tokens = append(tokens, strings.ToLower(current.String()))
		}
		current.Reset()
	}
	var prevLower bool
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			if prevLower {
				flush()
			}
			current.WriteRune(r)
			prevLower = false
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			current.WriteRune(r)
			prevLower = true
		default:
			flush()
			prevLower = false
		}
	}
	flush()
	return tokens
}

func repoMatches(elementRepo, remoteURL, repoRoot string) bool {
	elementRepo = normalizeRepoIdentity(elementRepo)
	if elementRepo == "" {
		return true
	}
	for _, candidate := range []string{remoteURL, repoRoot, filepath.Base(repoRoot)} {
		if normalizeRepoIdentity(candidate) == elementRepo {
			return true
		}
	}
	return false
}

func normalizeRepoIdentity(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = strings.TrimSuffix(value, ".git")
	value = strings.TrimRight(value, "/")
	if idx := strings.LastIndexAny(value, "/:"); idx >= 0 && idx+1 < len(value) {
		return value[idx+1:]
	}
	return value
}
