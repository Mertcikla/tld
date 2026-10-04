// Package community derives source-backed architectural groups from the
// codeindex dependency graph. Grouping is deterministic: communities come only
// from resolved file edges, directories are used for naming and for files with
// no resolved edges, and every decision (node order, tie-breaking, naming)
// is reproducible.
package community

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// File is one input file fact. ID is stable across snapshots and is what group
// keys are derived from.
type File struct {
	ID          string
	Path        string
	DisplayName string
	Language    string
}

// Edge is one aggregated undirected dependency between file indices.
type Edge struct {
	A, B   int
	Weight float64
}

// Group is one architectural group. Members are direct member file indices;
// Children are nested subgroups. Files counts all descendant files, Internal
// and External are edge weights wholly inside and crossing the group.
type Group struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Source   string  `json:"source"`
	Members  []int    `json:"members"`
	Children []*Group `json:"children,omitempty"`
	Isolated bool    `json:"isolated,omitempty"`
	Files    int     `json:"files"`
	Internal float64 `json:"internal"`
	External float64 `json:"external"`
}

// Metrics summarizes a grouping run.
type Metrics struct {
	Files          int     `json:"files"`
	Groups         int     `json:"groups"`
	RootGroups     int     `json:"root_groups"`
	Modularity     float64 `json:"modularity"`
	CrossWeight    float64 `json:"cross_weight"`
	MaxRootDegree  int     `json:"max_root_degree"`
	IsolatedFiles  int     `json:"isolated_files"`
	FolderCoverage float64 `json:"folder_coverage"`
	RuntimeSeconds float64 `json:"runtime_seconds"`
}

// Result is the grouping hierarchy plus provenance metrics.
type Result struct {
	Groups  []*Group          `json:"groups"`
	Metrics Metrics           `json:"metrics"`
	Params  map[string]string `json:"params"`
}

// Options controls grouping. Zero values are replaced by DefaultOptions.
type Options struct {
	// Resolution is the Louvain modularity gamma. Higher values produce more,
	// smaller communities.
	Resolution float64
	// MinGroupSize merges tiny leaf groups into their strongest sibling.
	MinGroupSize int
	// MinRootGroups and MaxRootGroups bound the readable number of root groups.
	MinRootGroups int
	MaxRootGroups int
	// MaxChildren bounds the fan-out of one subdivision.
	MaxChildren int
	// MaxDepth bounds the grouping tree depth.
	MaxDepth int
	// MaxLeafFiles forces a leaf group to be subdivided when it grows larger.
	MaxLeafFiles int
	// NameFallback infers a name from member indices when directory structure
	// does not discriminate. It must be deterministic.
	NameFallback func(members []int) string
}

// DefaultOptions returns production defaults: roughly 3-20 root components,
// subdivision fan-out of eight, at most four levels and forty files per view.
func DefaultOptions() Options {
	return Options{
		Resolution:    1.0,
		MinGroupSize:  2,
		MinRootGroups: 3,
		MaxRootGroups: 20,
		MaxChildren:   8,
		MaxDepth:      4,
		MaxLeafFiles:  40,
	}
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.Resolution <= 0 {
		o.Resolution = d.Resolution
	}
	if o.MinGroupSize < 1 {
		o.MinGroupSize = d.MinGroupSize
	}
	if o.MinRootGroups < 1 {
		o.MinRootGroups = d.MinRootGroups
	}
	if o.MaxRootGroups < o.MinRootGroups {
		o.MaxRootGroups = d.MaxRootGroups
	}
	if o.MaxChildren < 2 {
		o.MaxChildren = d.MaxChildren
	}
	if o.MaxDepth < 0 {
		o.MaxDepth = d.MaxDepth
	}
	if o.MaxLeafFiles < 1 {
		o.MaxLeafFiles = d.MaxLeafFiles
	}
	return o
}

func (o Options) params() map[string]string {
	return map[string]string{
		"algorithm":       "louvain-hierarchical",
		"version":         "1",
		"resolution":      strconv.FormatFloat(o.Resolution, 'g', -1, 64),
		"min_group_size":  strconv.Itoa(o.MinGroupSize),
		"min_root_groups": strconv.Itoa(o.MinRootGroups),
		"max_root_groups": strconv.Itoa(o.MaxRootGroups),
		"max_children":    strconv.Itoa(o.MaxChildren),
		"max_depth":       strconv.Itoa(o.MaxDepth),
		"max_leaf_files":  strconv.Itoa(o.MaxLeafFiles),
	}
}

type rawEdge struct {
	a, b   int
	weight float64
}

// Build groups the files using aggregated dependency edges. Node order follows
// the input order, which callers set from the snapshot's stable fact order.
func Build(files []File, edges []Edge, options Options) (*Result, error) {
	started := time.Now()
	opts := options.withDefaults()
	if err := opts.validate(); err != nil {
		return nil, err
	}
	result := &Result{Params: opts.params()}
	n := len(files)
	if n == 0 {
		result.Metrics.RuntimeSeconds = time.Since(started).Seconds()
		return result, nil
	}

	raw := make([]rawEdge, 0, len(edges))
	degree := make([]float64, n)
	for _, e := range edges {
		if e.A < 0 || e.B < 0 || e.A >= n || e.B >= n || e.A == e.B {
			continue
		}
		weight := e.Weight
		if !(weight > 0) {
			weight = 1
		}
		a, b := e.A, e.B
		if a > b {
			a, b = b, a
		}
		raw = append(raw, rawEdge{a: a, b: b, weight: weight})
		degree[a] += weight
		degree[b] += weight
	}
	sort.Slice(raw, func(i, j int) bool {
		if raw[i].a != raw[j].a {
			return raw[i].a < raw[j].a
		}
		return raw[i].b < raw[j].b
	})

	activeOf := make([]int, n)
	for i := range activeOf {
		activeOf[i] = -1
	}
	activeMembers := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if degree[i] > 0 {
			activeOf[i] = len(activeMembers)
			activeMembers = append(activeMembers, i)
		}
	}
	activeEdges := make([]edge, 0, len(raw))
	for _, e := range raw {
		a, b := activeOf[e.a], activeOf[e.b]
		if a < 0 || b < 0 {
			continue
		}
		activeEdges = append(activeEdges, edge{a: a, b: b, weight: e.weight})
	}
	g := newGraph(len(activeMembers), activeEdges)

	var roots []*Group
	mod := 0.0
	if g.n > 0 && g.m2 > 0 {
		partition, resolution := rootPartition(g, opts)
		mod = modularity(partition, g, resolution)
		communities := groupByCommunity(partition)
		for _, members := range communities {
			roots = append(roots, splitMembers(g, members, opts.Resolution, 0, opts))
		}
		roots = finalizeSiblings(roots, g, opts)
		for _, root := range roots {
			convertIndices(root, activeMembers)
		}
	}

	isolated := make([]int, 0)
	for i := 0; i < n; i++ {
		if degree[i] <= 0 {
			isolated = append(isolated, i)
		}
	}
	if len(isolated) > 0 {
		roots = append(roots, isolatedGroups(files, isolated, opts)...)
	}

	var lex *lexicalIndex
	if opts.NameFallback == nil {
		lex = newLexicalIndex(files)
	}
	finalizeGroups(roots, files, opts, lex)

	result.Groups = roots
	result.Metrics = measure(roots, g, files, activeMembers, n, mod, time.Since(started))
	return result, nil
}

func (o Options) validate() error {
	if o.Resolution <= 0 || o.MinGroupSize < 1 || o.MinRootGroups < 1 ||
		o.MaxRootGroups < o.MinRootGroups || o.MaxChildren < 2 ||
		o.MaxDepth < 0 || o.MaxLeafFiles < 1 {
		return fmt.Errorf("invalid community options")
	}
	return nil
}

// rootPartition runs Louvain and selects the root cut. It retries with a higher
// resolution when the graph collapses into a single community.
func rootPartition(g *graph, opts Options) ([]int32, float64) {
	resolution := opts.Resolution
	var levels []levelPartition
	for attempt := 0; attempt < 4; attempt++ {
		levels = louvainLevels(g, resolution)
		if selected, ok := selectRoot(levels, opts); ok {
			return selected.community, resolution
		}
		resolution *= 1.5
	}
	if len(levels) > 0 {
		coarsest := levels[len(levels)-1]
		return coarsest.community, resolution
	}
	singletons := make([]int32, g.n)
	for i := range singletons {
		singletons[i] = int32(i)
	}
	return singletons, resolution
}

func groupByCommunity(partition []int32) [][]int {
	byCommunity := map[int32][]int{}
	var order []int32
	for i, c := range partition {
		if _, ok := byCommunity[c]; !ok {
			order = append(order, c)
		}
		byCommunity[c] = append(byCommunity[c], i)
	}
	groups := make([][]int, 0, len(order))
	for _, c := range order {
		groups = append(groups, byCommunity[c])
	}
	return groups
}

// splitMembers recursively subdivides a group until it fits the leaf budget or
// reaches the depth cap. Members are local graph node indices.
func splitMembers(g *graph, members []int, resolution float64, depth int, opts Options) *Group {
	group := &Group{Members: append([]int(nil), members...)}
	if len(members) <= opts.MaxLeafFiles || depth >= opts.MaxDepth {
		return group
	}
	current := resolution
	for attempt := 0; attempt < 3; attempt++ {
		sub := induced(g, members)
		levels := louvainLevels(sub, current)
		partition, ok := splitPartition(levels, opts.MaxChildren, len(members))
		if ok {
			communities := groupByCommunity(partition)
			group.Members = nil
			group.Children = make([]*Group, 0, len(communities))
			for _, subset := range communities {
				global := make([]int, len(subset))
				for i, local := range subset {
					global[i] = members[local]
				}
				group.Children = append(group.Children, splitMembers(g, global, current, depth+1, opts))
			}
			return group
		}
		current *= 1.5
	}
	return group
}

// finalizeSiblings recursively merges undersized leaf groups into the sibling
// with the strongest edge connection.
func finalizeSiblings(children []*Group, g *graph, opts Options) []*Group {
	for _, child := range children {
		if len(child.Children) > 0 {
			child.Children = finalizeSiblings(child.Children, g, opts)
		}
	}
	for {
		merged := false
		order := make([]int, len(children))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool {
			left, right := children[order[i]], children[order[j]]
			if groupFileCount(left) != groupFileCount(right) {
				return groupFileCount(left) < groupFileCount(right)
			}
			return firstMember(left) < firstMember(right)
		})
		for _, index := range order {
			small := children[index]
			if len(small.Children) > 0 || groupFileCount(small) >= opts.MinGroupSize {
				continue
			}
			bestIndex := -1
			bestWeight := 0.0
			for j, target := range children {
				if j == index || len(target.Children) > 0 {
					continue
				}
				weight := crossWeight(small, target, g)
				if weight > bestWeight {
					bestWeight = weight
					bestIndex = j
				}
			}
			if bestIndex < 0 {
				continue
			}
			children[bestIndex].Members = append(children[bestIndex].Members, small.Members...)
			children = append(children[:index], children[index+1:]...)
			merged = true
			break
		}
		if !merged {
			return children
		}
	}
}

func groupFileCount(g *Group) int {
	count := len(g.Members)
	for _, child := range g.Children {
		count += groupFileCount(child)
	}
	return count
}

func firstMember(g *Group) int {
	first := -1
	for _, member := range g.Members {
		if first < 0 || member < first {
			first = member
		}
	}
	for _, child := range g.Children {
		if member := firstMember(child); member >= 0 && (first < 0 || member < first) {
			first = member
		}
	}
	return first
}

// crossWeight sums the edge weight between two disjoint member sets.
func crossWeight(a, b *Group, g *graph) float64 {
	if len(a.Members) == 0 || len(b.Members) == 0 {
		return 0
	}
	members := make(map[int]struct{}, len(a.Members))
	for _, member := range a.Members {
		members[member] = struct{}{}
	}
	target := make(map[int]struct{}, len(b.Members))
	for _, member := range b.Members {
		target[member] = struct{}{}
	}
	weight := 0.0
	for _, e := range g.edges {
		_, aSide := members[e.a]
		_, bSide := members[e.b]
		if aSide == bSide {
			continue
		}
		other := e.b
		if !aSide {
			other = e.a
		}
		if _, ok := target[other]; ok {
			weight += e.weight
		}
	}
	return weight
}

// convertIndices maps local graph indices back to input file indices.
func convertIndices(group *Group, activeMembers []int) {
	for i, member := range group.Members {
		if member >= 0 && member < len(activeMembers) {
			group.Members[i] = activeMembers[member]
		}
	}
	for _, child := range group.Children {
		convertIndices(child, activeMembers)
	}
}

// isolatedGroups buckets files with no resolved edges by directory so they stay
// discoverable without cluttering the root view. Distinct top-level folders are
// always separate groups; a subtree collapses into one group once it fits the
// leaf budget.
func isolatedGroups(files []File, members []int, opts Options) []*Group {
	return directoryLevel(files, members, 0, false, opts)
}

func directoryLevel(files []File, members []int, depth int, shared bool, opts Options) []*Group {
	if len(members) == 0 {
		return nil
	}
	if shared && (len(members) <= opts.MaxLeafFiles || depth >= opts.MaxDepth) {
		return []*Group{{Members: sortedCopy(members), Isolated: true}}
	}
	buckets := map[string][]int{}
	direct := make([]int, 0)
	for _, member := range members {
		segments := folderSegments(folderOf(files[member].Path))
		if len(segments) <= depth {
			direct = append(direct, member)
			continue
		}
		key := segments[depth]
		buckets[key] = append(buckets[key], member)
	}
	if len(buckets) == 0 {
		return []*Group{{Members: sortedCopy(members), Isolated: true}}
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if !shared {
		// Top level: folders and root-level files are peer groups, not nested.
		out := make([]*Group, 0, len(keys)+1)
		if len(direct) > 0 {
			out = append(out, &Group{Members: sortedCopy(direct), Isolated: true})
		}
		for _, key := range keys {
			out = append(out, directoryLevel(files, buckets[key], depth+1, true, opts)...)
		}
		return out
	}
	if len(keys) == 1 && len(direct) == 0 {
		// A single child owns the whole subtree: skip the empty level so the
		// hierarchy does not grow meaningless containers.
		return directoryLevel(files, buckets[keys[0]], depth+1, true, opts)
	}
	group := &Group{Members: sortedCopy(direct), Isolated: true}
	for _, key := range keys {
		group.Children = append(group.Children, directoryLevel(files, buckets[key], depth+1, true, opts)...)
	}
	return []*Group{group}
}

func sortedCopy(values []int) []int {
	out := append([]int(nil), values...)
	sort.Ints(out)
	return out
}

// finalizeGroups computes keys, names, counts and returns descendant files.
func finalizeGroups(groups []*Group, files []File, opts Options, lex *lexicalIndex) {
	for _, group := range groups {
		finalizeGroup(group, files, opts, lex)
	}
}

func finalizeGroup(group *Group, files []File, opts Options, lex *lexicalIndex) []int {
	sort.Ints(group.Members)
	descendants := append([]int(nil), group.Members...)
	for _, child := range group.Children {
		descendants = append(descendants, finalizeGroup(child, files, opts, lex)...)
	}
	sort.Ints(descendants)
	group.Files = len(descendants)
	group.Key = groupKey(files, descendants)
	group.Name, group.Source = nameGroup(descendants, files, opts, group.Isolated, lex)
	return descendants
}

func groupKey(files []File, members []int) string {
	hash := sha256.New()
	for _, member := range members {
		if member < 0 || member >= len(files) {
			continue
		}
		_, _ = hash.Write([]byte(files[member].ID))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// nameGroup chooses a deterministic, source-backed label: the dominant folder
// when one folder covers most members, then the longest common folder prefix,
// then a distinctive identifier token, then a key-derived placeholder. Generic
// repository roots such as "internal" are skipped in favour of a distinctive
// name so sibling groups do not share a meaningless label.
func nameGroup(members []int, files []File, opts Options, isolated bool, lex *lexicalIndex) (string, string) {
	if len(members) == 0 {
		return "unmapped", "fallback"
	}
	counts := map[string]int{}
	for _, member := range members {
		counts[folderOf(files[member].Path)]++
	}
	best, bestCount := "", 0
	for folder, count := range counts {
		if count > bestCount || (count == bestCount && folder < best) {
			best, bestCount = folder, count
		}
	}
	dominant := ""
	if bestCount*5 >= len(members)*3 && best != "." && best != "" {
		dominant = best
	}
	prefix := commonFolderPrefix(members, files)
	if isolated {
		// Directory buckets are structural: their folder name is the honest
		// label, and it must not be replaced by an inferred token.
		if dominant != "" {
			return dominant, "folder"
		}
		if prefix != "" {
			return prefix, "common_prefix"
		}
		return "unmapped", "folder"
	}
	if dominant != "" && !genericPrefix(dominant) {
		return dominant, "folder"
	}
	if prefix != "" && !genericPrefix(prefix) {
		return prefix, "common_prefix"
	}
	if opts.NameFallback != nil {
		if name := strings.TrimSpace(opts.NameFallback(members)); name != "" {
			return name, "lexical"
		}
	} else if name := lex.name(members); name != "" {
		return name, "lexical"
	}
	if dominant != "" {
		return dominant, "folder"
	}
	if prefix != "" {
		return prefix, "common_prefix"
	}
	key := groupKey(files, members)
	return "group-" + key[:8], "fallback"
}

// measure computes structural metrics over the finished tree. Paths index the
// active graph nodes so every edge lands on its lowest common group.
func measure(roots []*Group, g *graph, files []File, activeMembers []int, totalFiles int, mod float64, elapsed time.Duration) Metrics {
	metrics := Metrics{
		Files:          totalFiles,
		RootGroups:     len(roots),
		Modularity:     mod,
		RuntimeSeconds: elapsed.Seconds(),
	}
	var nodes []*Group
	paths := make([][]int32, totalFiles)
	var walk func(group *Group, path []int32)
	walk = func(group *Group, path []int32) {
		id := int32(len(nodes))
		nodes = append(nodes, group)
		current := make([]int32, len(path)+1)
		copy(current, path)
		current[len(path)] = id
		for _, member := range group.Members {
			if member >= 0 && member < len(paths) {
				paths[member] = current
			}
		}
		for _, child := range group.Children {
			walk(child, current)
		}
	}
	for _, root := range roots {
		walk(root, nil)
	}
	metrics.Groups = len(nodes)

	rootDegree := map[int32]map[int32]struct{}{}
	isolated := 0
	for _, group := range nodes {
		if group.Isolated {
			isolated += group.Files
		}
	}
	for _, e := range g.edges {
		from := paths[activeMembers[e.a]]
		to := paths[activeMembers[e.b]]
		if len(from) == 0 || len(to) == 0 {
			continue
		}
		index := 0
		for index < len(from) && index < len(to) && from[index] == to[index] {
			index++
		}
		if index == len(from) && index == len(to) {
			for _, id := range from {
				nodes[id].Internal += e.weight
			}
			continue
		}
		for i := 0; i < index; i++ {
			nodes[from[i]].Internal += e.weight
		}
		if index < len(from) {
			nodes[from[index]].External += e.weight
		}
		if index < len(to) {
			nodes[to[index]].External += e.weight
		}
		if from[0] != to[0] {
			metrics.CrossWeight += e.weight
			if rootDegree[from[0]] == nil {
				rootDegree[from[0]] = map[int32]struct{}{}
			}
			if rootDegree[to[0]] == nil {
				rootDegree[to[0]] = map[int32]struct{}{}
			}
			rootDegree[from[0]][to[0]] = struct{}{}
			rootDegree[to[0]][from[0]] = struct{}{}
		}
	}
	for _, neighbors := range rootDegree {
		if len(neighbors) > metrics.MaxRootDegree {
			metrics.MaxRootDegree = len(neighbors)
		}
	}
	metrics.IsolatedFiles = isolated
	metrics.FolderCoverage = folderCoverage(roots, files)
	return metrics
}

// folderCoverage is the fraction of files whose folder is the dominant folder
// of their root group. It is informational: low values mean community
// boundaries cut across the repository's folder layout.
func folderCoverage(roots []*Group, files []File) float64 {
	total := 0
	matched := 0
	for _, root := range roots {
		members := collectMembers(root)
		if len(members) == 0 {
			continue
		}
		counts := map[string]int{}
		for _, member := range members {
			counts[folderOf(files[member].Path)]++
		}
		dominant, best := "", 0
		for folder, count := range counts {
			if count > best || (count == best && folder < dominant) {
				dominant, best = folder, count
			}
		}
		for _, member := range members {
			if folderOf(files[member].Path) == dominant {
				matched++
			}
		}
		total += len(members)
	}
	if total == 0 {
		return 1
	}
	return float64(matched) / float64(total)
}

func collectMembers(group *Group) []int {
	out := append([]int(nil), group.Members...)
	for _, child := range group.Children {
		out = append(out, collectMembers(child)...)
	}
	return out
}
