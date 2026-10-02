package mapper

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Domain is one semantic cluster with its member input indices and tightness.
type Domain struct {
	Members   []int   `json:"members"`
	Tightness float64 `json:"tightness"`
}

// Size returns the number of members.
func (d Domain) Size() int { return len(d.Members) }

// Counts records pipeline stage tallies.
type Counts struct {
	CoreClusters       int `json:"core_clusters"`
	SplitAttempts      int `json:"split_attempts"`
	AcceptedSplits     int `json:"accepted_splits"`
	FallbackPartitions int `json:"fallback_partitions"`
	SweepAssignments   int `json:"sweep_assignments"`
}

// Metrics summarizes a pipeline run. Counts is embedded so its fields flatten
// into the JSON object, matching the Rust serde flatten.
type Metrics struct {
	FactCount                   int                `json:"fact_count"`
	ClusteredCount              int                `json:"clustered_count"`
	UnclusteredCount            int                `json:"unclustered_count"`
	UnclusteredFraction         float64            `json:"unclustered_fraction"`
	ExcessUnclusteredFraction   float64            `json:"excess_unclustered_fraction"`
	WeakPairFraction            float64            `json:"weak_pair_fraction"`
	PairCount                   int                `json:"pair_count"`
	WeightedTightness           float64            `json:"weighted_tightness"`
	ClusterOutsideIdealFraction float64            `json:"cluster_outside_ideal_fraction"`
	ClusterHardSizeViolations   int                `json:"cluster_hard_size_violations"`
	ClusterSizePercentiles      map[string]float64 `json:"cluster_size_percentiles"`
	Counts
	RuntimeSeconds  float64 `json:"runtime_seconds"`
	PeakMemoryBytes *uint64 `json:"peak_memory_bytes"`
	Configuration   Options `json:"configuration"`
}

// PipelineResult is the full clustering output.
type PipelineResult struct {
	Domains         []Domain       `json:"domains"`
	Leftovers       []int          `json:"leftovers"`
	LeftoverReasons map[int]string `json:"leftover_reasons"`
	Metrics         Metrics        `json:"metrics"`
	Options         Options        `json:"options"`
}

type topPair struct {
	id    int
	score float64
}

// neighborhoods streams exact top-k neighbors by blocks, retaining only the
// best k entries per row. Output neighbors are ascending (score, id); density is
// the mean retained score per row.
func neighborhoods(rows [][]float64, subset []int, opt *Options, floor float64, progress ProgressFunc) ([][]int, []float64) {
	n := len(subset)
	k := opt.Neighbors
	if n-1 < k {
		k = n - 1
	}
	if k < 0 {
		k = 0
	}
	valid := make([]bool, n)
	for i, idx := range subset {
		for _, x := range rows[idx] {
			if x != 0.0 {
				valid[i] = true
				break
			}
		}
	}
	nearest := make([][]topPair, n)
	blocks := (n + opt.BlockSize - 1) / opt.BlockSize
	if blocks < 1 {
		blocks = 1
	}
	totalIterations := blocks * blocks
	iteration := 0
	for start := 0; start < n; start += opt.BlockSize {
		startEnd := min(start+opt.BlockSize, n)
		for other := 0; other < n; other += opt.BlockSize {
			otherEnd := min(other+opt.BlockSize, n)
			for i := start; i < startEnd; i++ {
				if !valid[i] {
					continue
				}
				for j := other; j < otherEnd; j++ {
					if i == j || !valid[j] || k == 0 {
						continue
					}
					score := dot(rows[subset[i]], rows[subset[j]])
					if score < -1.0 {
						score = -1.0
					} else if score > 1.0 {
						score = 1.0
					}
					if score < floor {
						continue
					}
					list := nearest[i]
					position := len(list)
					for p := 0; p < len(list); p++ {
						c := totalCmp(list[p].score, score)
						if c > 0 || (c == 0 && list[p].id > j) {
							position = p
							break
						}
					}
					if position < k {
						list = append(list, topPair{})
						copy(list[position+1:], list[position:])
						list[position] = topPair{id: j, score: score}
						if len(list) > k {
							// The list is ascending (worst first); drop the worst
							// so the best k by (score desc, id asc) are retained.
							copy(list, list[1:])
							list = list[:k]
						}
						nearest[i] = list
					}
				}
			}
			iteration++
			if iteration%64 == 0 || iteration == totalIterations {
				report(progress, "clustering", iteration, totalIterations, "neighbors")
			}
		}
	}
	neighbors := make([][]int, n)
	density := make([]float64, n)
	for i, entries := range nearest {
		ids := make([]int, len(entries))
		sum := 0.0
		for j, entry := range entries {
			ids[j] = entry.id
			sum += entry.score
		}
		neighbors[i] = ids
		if len(entries) > 0 {
			density[i] = sum / float64(len(entries))
		}
	}
	return neighbors, density
}

func mergeSorted(a, b []int) []int {
	out := make([]int, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		var value int
		switch {
		case i >= len(a):
			value = b[j]
			j++
		case j >= len(b):
			value = a[i]
			i++
		case a[i] < b[j]:
			value = a[i]
			i++
		case a[i] > b[j]:
			value = b[j]
			j++
		default:
			value = a[i]
			i++
			j++
		}
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

// grow visits densest seeds first and grows by average similarity to current
// members. Candidate growth unions incoming links when symmetric_neighbors is on.
func grow(rows [][]float64, subset []int, opt *Options, floor float64, progress ProgressFunc) [][]int {
	neighbors, density := neighborhoods(rows, subset, opt, floor, progress)
	n := len(subset)
	sets := make([]map[int]struct{}, n)
	for i := range sets {
		sets[i] = make(map[int]struct{}, len(neighbors[i]))
		for _, j := range neighbors[i] {
			sets[i][j] = struct{}{}
		}
	}
	if opt.SymmetricNeighbors {
		for i, ids := range neighbors {
			for _, j := range ids {
				sets[j][i] = struct{}{}
			}
		}
	}
	links := make([][]int, n)
	for i, set := range sets {
		list := make([]int, 0, len(set))
		for value := range set {
			list = append(list, value)
		}
		sort.Ints(list)
		links[i] = list
	}
	seeds := make([]int, n)
	for i := range seeds {
		seeds[i] = i
	}
	sort.SliceStable(seeds, func(a, b int) bool {
		x, y := seeds[a], seeds[b]
		c := totalCmp(density[y], density[x])
		if c != 0 {
			return c < 0
		}
		return x < y
	})
	assigned := make([]bool, n)
	groups := make([][]int, 0)
	for seedIndex, seed := range seeds {
		if assigned[seed] || len(neighbors[seed]) == 0 {
			continue
		}
		members := []int{subset[seed]}
		sum := append([]float64(nil), rows[subset[seed]]...)
		assigned[seed] = true
		candidates := append([]int(nil), links[seed]...)
		for {
			mean := make([]float64, len(sum))
			for i := range sum {
				mean[i] = sum[i] / float64(len(members))
			}
			bestID := -1
			bestScore := 0.0
			for _, candidate := range candidates {
				if assigned[candidate] {
					continue
				}
				score := dot(rows[subset[candidate]], mean)
				if bestID == -1 || score > bestScore {
					bestID = candidate
					bestScore = score
				}
			}
			if bestID == -1 || bestScore < floor {
				break
			}
			member := subset[bestID]
			members = append(members, member)
			add(sum, rows[member])
			assigned[bestID] = true
			candidates = mergeSorted(candidates, links[bestID])
		}
		sort.Ints(members)
		groups = append(groups, members)
		if seedIndex%64 == 0 {
			report(progress, "clustering", seedIndex+1, n, "grow")
		}
	}
	return groups
}

// balancedPartition splits an oversized cluster with farthest-first seeds and
// balanced quotas, then prunes weak members and dissolves undersized groups.
func balancedPartition(rows [][]float64, members []int, opt *Options) ([][]int, []int) {
	count := (len(members) + opt.ClusterSizes.IdealMax - 1) / opt.ClusterSizes.IdealMax
	seeds := []int{0}
	closest := make([]float64, len(members))
	for i, idx := range members {
		closest[i] = dot(rows[idx], rows[members[0]])
	}
	for s := 1; s < count; s++ {
		for _, seed := range seeds {
			closest[seed] = math.Inf(1)
		}
		seed := 0
		for i := 1; i < len(members); i++ {
			c := totalCmp(closest[i], closest[seed])
			if c < 0 || (c == 0 && i < seed) {
				seed = i
			}
		}
		seeds = append(seeds, seed)
		for i := range closest {
			d := dot(rows[members[i]], rows[members[seed]])
			if d > closest[i] {
				closest[i] = d
			}
		}
	}
	quotas := make([]int, count)
	for i := range quotas {
		quotas[i] = len(members) / count
		if i < len(members)%count {
			quotas[i]++
		}
	}
	groups := make([][]int, count)
	sums := make([][]float64, count)
	for i, seed := range seeds {
		groups[i] = []int{members[seed]}
		sums[i] = append([]float64(nil), rows[members[seed]]...)
	}
	seedSet := make(map[int]struct{}, len(seeds))
	for _, seed := range seeds {
		seedSet[seed] = struct{}{}
	}
	type candidate struct {
		position   int
		confidence float64
	}
	remaining := make([]candidate, 0, len(members))
	for i := range members {
		if _, ok := seedSet[i]; ok {
			continue
		}
		confidence := math.Inf(-1)
		for _, seed := range seeds {
			d := dot(rows[members[i]], rows[members[seed]])
			if d > confidence {
				confidence = d
			}
		}
		remaining = append(remaining, candidate{i, confidence})
	}
	sort.SliceStable(remaining, func(a, b int) bool {
		x, y := remaining[a], remaining[b]
		c := totalCmp(y.confidence, x.confidence)
		if c != 0 {
			return c < 0
		}
		return members[x.position] < members[y.position]
	})
	for _, entry := range remaining {
		best := -1
		bestScore := 0.0
		for j := 0; j < count; j++ {
			if len(groups[j]) >= quotas[j] {
				continue
			}
			score := dot(rows[members[entry.position]], sums[j]) / float64(len(groups[j]))
			if best == -1 || score > bestScore {
				best = j
				bestScore = score
			}
		}
		groups[best] = append(groups[best], members[entry.position])
		add(sums[best], rows[members[entry.position]])
	}
	retained := make([][]int, 0)
	rejected := make([]int, 0)
	for _, group := range groups {
		sort.Ints(group)
		for len(group) >= opt.ClusterSizes.Min {
			sum := total(rows, group)
			worst := 0
			worstScore := 0.0
			for j, idx := range group {
				score := (dot(rows[idx], sum) - dot(rows[idx], rows[idx])) / float64(len(group)-1)
				if j == 0 || totalCmp(score, worstScore) < 0 {
					worst = j
					worstScore = score
				}
			}
			if worstScore >= opt.MemberFloor && tightness(rows, group) >= opt.TightnessFloor {
				break
			}
			rejected = append(rejected, group[worst])
			group = append(group[:worst], group[worst+1:]...)
		}
		if len(group) < opt.ClusterSizes.Min {
			rejected = append(rejected, group...)
		} else {
			retained = append(retained, group)
		}
	}
	return retained, rejected
}

type splitter struct {
	rows     [][]float64
	opt      *Options
	reasons  map[int]string
	counts   *Counts
	progress ProgressFunc
}

func (s *splitter) split(members []int, floor float64, depth int) [][]int {
	sizes := s.opt.ClusterSizes
	if len(members) <= sizes.IdealMax {
		return [][]int{members}
	}
	if depth < s.opt.SplitMaxPasses {
		for step := 1; step <= s.opt.SplitMaxPasses; step++ {
			threshold := floor + s.opt.SplitStep*float64(step)
			if threshold > 1.0 {
				threshold = 1.0
			}
			s.counts.SplitAttempts++
			childrenRaw := grow(s.rows, members, s.opt, threshold, nil)
			children := make([][]int, 0, len(childrenRaw))
			for _, child := range childrenRaw {
				if len(child) >= sizes.Min {
					children = append(children, child)
				}
			}
			oversized := 0
			for _, child := range children {
				if len(child) > sizes.IdealMax {
					oversized += len(child)
				}
			}
			accepted := len(children) > 0 && oversized < len(members)
			if accepted {
				for _, child := range children {
					if tightness(s.rows, child) < s.opt.TightnessFloor {
						accepted = false
						break
					}
				}
			}
			if accepted {
				s.counts.AcceptedSplits++
				claimed := make(map[int]struct{})
				for _, child := range children {
					for _, i := range child {
						claimed[i] = struct{}{}
					}
				}
				for _, i := range members {
					if _, ok := claimed[i]; !ok {
						s.reasons[i] = "split_rejected"
					}
				}
				result := make([][]int, 0)
				for _, child := range children {
					result = append(result, s.split(child, threshold, depth+1)...)
				}
				return result
			}
			if threshold == 1.0 {
				break
			}
		}
	}
	if len(members) > sizes.Max {
		s.counts.FallbackPartitions++
		children, rejected := balancedPartition(s.rows, members, s.opt)
		for _, i := range rejected {
			s.reasons[i] = "fallback_quality"
		}
		return children
	}
	return [][]int{members}
}

type proposal struct {
	destination int // -1 means rejected
	score       float64
	margin      float64
	reason      string
}

type sweepState struct {
	sums    [][]float64
	sizes   []int
	squared []float64
}

func (s *sweepState) destination(value []float64, opt *Options) proposal {
	nonzero := false
	for _, x := range value {
		if x != 0.0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		return proposal{destination: -1, reason: "invalid_vector"}
	}
	nonfull := make([]int, 0, len(s.sizes))
	for j, size := range s.sizes {
		if size < opt.ClusterSizes.Max {
			nonfull = append(nonfull, j)
		}
	}
	if len(nonfull) == 0 {
		return proposal{destination: -1, reason: "cluster_capacity"}
	}
	dots := make([]float64, len(s.sums))
	for j, sum := range s.sums {
		dots[j] = dot(sum, value)
	}
	centroid := make([]float64, len(s.sums))
	for j, sum := range s.sums {
		norm := math.Sqrt(dot(sum, sum))
		if norm > 0.0 {
			centroid[j] = dots[j] / norm
		} else {
			centroid[j] = dots[j]
		}
	}
	eligible := make([]int, 0, len(nonfull))
	for _, j := range nonfull {
		if centroid[j] >= *opt.SweepFloor {
			eligible = append(eligible, j)
		}
	}
	if len(eligible) == 0 {
		return proposal{destination: -1, reason: "below_centroid_floor"}
	}
	filtered := eligible[:0]
	for _, j := range eligible {
		if dots[j]/float64(s.sizes[j]) >= opt.MemberFloor {
			filtered = append(filtered, j)
		}
	}
	eligible = filtered
	if len(eligible) == 0 {
		return proposal{destination: -1, reason: "below_member_floor"}
	}
	filtered = eligible[:0]
	for _, j := range eligible {
		size := float64(s.sizes[j])
		projected := (dot(s.sums[j], s.sums[j]) - s.squared[j] + 2.0*dots[j]) / (size * (size + 1.0))
		if projected >= opt.TightnessFloor {
			filtered = append(filtered, j)
		}
	}
	eligible = filtered
	if len(eligible) == 0 {
		return proposal{destination: -1, reason: "tightness_floor"}
	}
	sort.SliceStable(eligible, func(a, b int) bool {
		x, y := eligible[a], eligible[b]
		c := totalCmp(centroid[y], centroid[x])
		if c != 0 {
			return c < 0
		}
		xf := s.sizes[x] >= opt.ClusterSizes.IdealMax
		yf := s.sizes[y] >= opt.ClusterSizes.IdealMax
		if xf != yf {
			return !xf
		}
		return x < y
	})
	best := eligible[0]
	margin := centroid[best]
	if len(eligible) > 1 {
		margin = centroid[best] - centroid[eligible[1]]
	}
	if opt.AmbiguityMargin > 0.0 && margin < opt.AmbiguityMargin {
		return proposal{destination: -1, score: centroid[best], margin: margin, reason: "ambiguous"}
	}
	return proposal{destination: best, score: centroid[best], margin: margin}
}

func sweep(rows [][]float64, groups [][]int, opt *Options, reasons map[int]string, counts *Counts) {
	if opt.SweepFloor == nil || opt.SweepMaxPasses == 0 || len(groups) == 0 {
		return
	}
	state := sweepState{
		sums:    make([][]float64, len(groups)),
		sizes:   make([]int, len(groups)),
		squared: make([]float64, len(groups)),
	}
	for i, group := range groups {
		state.sums[i] = total(rows, group)
		state.sizes[i] = len(group)
		for _, member := range group {
			state.squared[i] += dot(rows[member], rows[member])
		}
	}
	type pending struct {
		index    int
		proposal proposal
	}
	for pass := 0; pass < opt.SweepMaxPasses; pass++ {
		assigned := make(map[int]struct{})
		for _, group := range groups {
			for _, i := range group {
				assigned[i] = struct{}{}
			}
		}
		proposals := make([]pending, 0)
		for i := 0; i < len(rows); i++ {
			if _, ok := assigned[i]; ok {
				continue
			}
			proposals = append(proposals, pending{i, state.destination(rows[i], opt)})
		}
		sort.SliceStable(proposals, func(a, b int) bool {
			x, y := proposals[a], proposals[b]
			if c := totalCmp(y.proposal.margin, x.proposal.margin); c != 0 {
				return c < 0
			}
			if c := totalCmp(y.proposal.score, x.proposal.score); c != 0 {
				return c < 0
			}
			return x.index < y.index
		})
		changed := false
		for _, item := range proposals {
			i := item.index
			prop := state.destination(rows[i], opt)
			if prop.destination < 0 {
				origin, ok := reasons[i]
				if !ok {
					origin = "no_qualified_cluster"
				}
				if idx := strings.Index(origin, "; sweep:"); idx >= 0 {
					origin = origin[:idx]
				}
				reasons[i] = origin + "; sweep:" + prop.reason
				continue
			}
			best := prop.destination
			groups[best] = append(groups[best], i)
			add(state.sums[best], rows[i])
			state.sizes[best]++
			state.squared[best] += dot(rows[i], rows[i])
			delete(reasons, i)
			counts.SweepAssignments++
			changed = true
		}
		if !changed {
			break
		}
	}
}

// Assignments maps input indices to domain ranks, or -1 when unassigned. It
// rejects duplicate or out-of-range membership.
func Assignments(domains []Domain, n int) ([]int, error) {
	labels := make([]int, n)
	for i := range labels {
		labels[i] = -1
	}
	for rank, domain := range domains {
		for _, i := range domain.Members {
			if i < 0 || i >= n || labels[i] != -1 {
				return nil, errDuplicateMembership
			}
			labels[i] = rank
		}
	}
	return labels, nil
}

// RunPipeline runs the clustering pipeline with default-constructed progress
// disabled.
func RunPipeline(vectors [][]float64, opt *Options) (*PipelineResult, error) {
	return RunPipelineProgress(vectors, opt, nil)
}

// RunPipelineProgress runs the clustering pipeline and reports coarse progress.
func RunPipelineProgress(vectors [][]float64, opt *Options, progress ProgressFunc) (*PipelineResult, error) {
	if err := opt.Validate(); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(Progress{Stage: "clustering", Detail: "normalize"})
	}
	started := time.Now()
	rows, err := normalize(vectors)
	if err != nil {
		return nil, err
	}
	reasons := make(map[int]string, len(rows))
	for i, vector := range rows {
		nonzero := false
		for _, x := range vector {
			if x != 0.0 {
				nonzero = true
				break
			}
		}
		if nonzero {
			reasons[i] = "no_qualified_cluster"
		} else {
			reasons[i] = "invalid_vector"
		}
	}
	subset := make([]int, len(rows))
	for i := range subset {
		subset[i] = i
	}
	coreRaw := grow(rows, subset, opt, opt.MinSimilarity, progress)
	core := make([][]int, 0, len(coreRaw))
	for _, group := range coreRaw {
		if len(group) >= opt.ClusterSizes.Min {
			core = append(core, group)
		}
	}
	counts := Counts{CoreClusters: len(core)}
	groups := make([][]int, 0)
	activeSplitter := &splitter{rows: rows, opt: opt, reasons: reasons, counts: &counts, progress: progress}
	for index, group := range core {
		groups = append(groups, activeSplitter.split(group, opt.MinSimilarity, 0)...)
		report(progress, "clustering", index+1, len(core), "split")
	}
	sweep(rows, groups, opt, reasons, &counts)
	domains := make([]Domain, len(groups))
	for i, members := range groups {
		sort.Ints(members)
		domains[i] = Domain{Members: members, Tightness: tightness(rows, members)}
	}
	sort.SliceStable(domains, func(a, b int) bool {
		x, y := domains[a], domains[b]
		if c := totalCmp(y.Tightness, x.Tightness); c != 0 {
			return c < 0
		}
		if len(x.Members) != len(y.Members) {
			return len(x.Members) > len(y.Members)
		}
		return x.Members[0] < y.Members[0]
	})
	labels, err := Assignments(domains, len(rows))
	if err != nil {
		return nil, err
	}
	leftovers := make([]int, 0)
	for i, rank := range labels {
		if rank == -1 {
			leftovers = append(leftovers, i)
		}
	}
	for i := range reasons {
		if labels[i] != -1 {
			delete(reasons, i)
		}
	}
	pairs := 0
	weak := 0
	for _, domain := range domains {
		for position, i := range domain.Members {
			for _, j := range domain.Members[position+1:] {
				pairs++
				if dot(rows[i], rows[j]) < opt.WeakPairFloor {
					weak++
				}
			}
		}
	}
	clustered := len(rows) - len(leftovers)
	fraction := ratio(len(leftovers), len(rows))
	sizes := make([]int, len(domains))
	for i, domain := range domains {
		sizes[i] = domain.Size()
	}
	outside, violations, percentiles := sizeStats(sizes, opt.ClusterSizes)
	weightedTightness := 0.0
	if clustered > 0 {
		sum := 0.0
		for _, domain := range domains {
			sum += domain.Tightness * float64(domain.Size())
		}
		weightedTightness = sum / float64(clustered)
	}
	metrics := Metrics{
		FactCount:                   len(rows),
		ClusteredCount:              clustered,
		UnclusteredCount:            len(leftovers),
		UnclusteredFraction:         fraction,
		ExcessUnclusteredFraction:   math.Max(fraction-opt.UnclusteredTarget, 0.0),
		WeakPairFraction:            ratio(weak, pairs),
		PairCount:                   pairs,
		WeightedTightness:           weightedTightness,
		ClusterOutsideIdealFraction: outside,
		ClusterHardSizeViolations:   violations,
		ClusterSizePercentiles:      percentiles,
		Counts:                      counts,
		RuntimeSeconds:              time.Since(started).Seconds(),
		Configuration:               *opt,
	}
	report(progress, "clustering", 1, 1, "done")
	return &PipelineResult{
		Domains:         domains,
		Leftovers:       leftovers,
		LeftoverReasons: reasons,
		Metrics:         metrics,
		Options:         *opt,
	}, nil
}

// DiscoverDomains is a convenience wrapper returning only the domains.
func DiscoverDomains(vectors [][]float64, options *Options) ([]Domain, error) {
	result, err := RunPipeline(vectors, options)
	if err != nil {
		return nil, err
	}
	return result.Domains, nil
}
