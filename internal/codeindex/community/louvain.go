package community

import "sort"

const (
	louvainMaxLevels = 10
	louvainMaxPasses = 20
)

// edge is one undirected weighted dependency between two local node indices.
type edge struct {
	a, b   int
	weight float64
}

// graph is an undirected weighted graph in edge-list form. Degrees count each
// incidence once, so the sum of degrees is 2m. Edges are kept sorted by
// (a, b) so every traversal order is deterministic.
type graph struct {
	n      int
	edges  []edge
	degree []float64
	m2     float64
}

func newGraph(n int, edges []edge) *graph {
	g := &graph{n: n, degree: make([]float64, n), edges: make([]edge, 0, len(edges))}
	for _, e := range edges {
		if e.a < 0 || e.b < 0 || e.a >= n || e.b >= n || e.a == e.b || e.weight <= 0 {
			continue
		}
		if e.a > e.b {
			e.a, e.b = e.b, e.a
		}
		g.edges = append(g.edges, e)
		g.degree[e.a] += e.weight
		g.degree[e.b] += e.weight
		g.m2 += 2 * e.weight
	}
	sort.Slice(g.edges, func(i, j int) bool {
		if g.edges[i].a != g.edges[j].a {
			return g.edges[i].a < g.edges[j].a
		}
		return g.edges[i].b < g.edges[j].b
	})
	return g
}

// csr is a compressed adjacency built from sorted edges. Neighbor lists are
// ascending, which is required for deterministic local moving.
type csr struct {
	start  []int32
	nbr    []int32
	weight []float64
}

func (g *graph) csr() csr {
	start := make([]int32, g.n+1)
	for _, e := range g.edges {
		start[e.a+1]++
		start[e.b+1]++
	}
	for i := 1; i <= g.n; i++ {
		start[i] += start[i-1]
	}
	pos := make([]int32, g.n)
	copy(pos, start[:g.n])
	nbr := make([]int32, start[g.n])
	weight := make([]float64, len(nbr))
	for _, e := range g.edges {
		nbr[pos[e.a]] = int32(e.b)
		weight[pos[e.a]] = e.weight
		pos[e.a]++
		nbr[pos[e.b]] = int32(e.a)
		weight[pos[e.b]] = e.weight
		pos[e.b]++
	}
	return csr{start: start, nbr: nbr, weight: weight}
}

// renumber relabels communities to 0..k-1 in first-appearance order so results
// do not depend on internal labels.
func renumber(comm []int32) []int32 {
	remap := make([]int32, 0, 8)
	seen := map[int32]int32{}
	for _, c := range comm {
		if _, ok := seen[c]; !ok {
			seen[c] = int32(len(remap))
			remap = append(remap, c)
		}
	}
	out := make([]int32, len(comm))
	for i, c := range comm {
		out[i] = seen[c]
	}
	return out
}

// localMoving runs Louvain local moving until no node changes community or the
// pass budget is exhausted. Ties resolve to the smallest community id; node
// order is index order. Both rules are required for reproducibility.
func localMoving(g *graph, adj csr, resolution float64) []int32 {
	comm := make([]int32, g.n)
	for i := range comm {
		comm[i] = int32(i)
	}
	if g.n == 0 || g.m2 == 0 {
		return comm
	}
	total := make([]float64, g.n)
	copy(total, g.degree)
	gains := make([]float64, g.n)
	touched := make([]int32, 0, 16)
	for pass := 0; pass < louvainMaxPasses; pass++ {
		moved := 0
		for i := 0; i < g.n; i++ {
			current := comm[i]
			weight := g.degree[i]
			total[current] -= weight
			touched = touched[:0]
			for p := adj.start[i]; p < adj.start[i+1]; p++ {
				other := comm[adj.nbr[p]]
				if gains[other] == 0 {
					touched = append(touched, other)
				}
				gains[other] += adj.weight[p]
			}
			best := current
			bestGain := gains[current] - resolution*total[current]*weight/g.m2
			for _, candidate := range touched {
				if candidate == current {
					continue
				}
				gain := gains[candidate] - resolution*total[candidate]*weight/g.m2
				if gain > bestGain || (gain == bestGain && candidate < best) {
					best = candidate
					bestGain = gain
				}
			}
			for _, candidate := range touched {
				gains[candidate] = 0
			}
			total[best] += weight
			if best != current {
				comm[i] = best
				moved++
			}
		}
		if moved == 0 {
			break
		}
	}
	return renumber(comm)
}

// aggregate builds the next-level graph: one node per community, weighted by
// the total edge weight between communities. Internal edges are dropped because
// they can never influence local moving at the aggregated level.
func aggregate(g *graph, comm []int32, count int) *graph {
	degree := make([]float64, count)
	for i, d := range g.degree {
		degree[comm[i]] += d
	}
	type pair struct{ a, b int32 }
	weights := map[pair]float64{}
	for _, e := range g.edges {
		a, b := comm[e.a], comm[e.b]
		if a == b {
			continue
		}
		if a > b {
			a, b = b, a
		}
		weights[pair{a: a, b: b}] += e.weight
	}
	edges := make([]edge, 0, len(weights))
	for p, w := range weights {
		edges = append(edges, edge{a: int(p.a), b: int(p.b), weight: w})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].a != edges[j].a {
			return edges[i].a < edges[j].a
		}
		return edges[i].b < edges[j].b
	})
	next := &graph{n: count, edges: edges, degree: degree}
	for _, d := range degree {
		next.m2 += d
	}
	return next
}

// levelPartition is one dendrogram level expressed over the original graph
// nodes, finest first. Modularity is evaluated with the resolution that
// produced the level.
type levelPartition struct {
	community  []int32
	count      int
	modularity float64
}

// louvainLevels runs hierarchical Louvain and returns every level's partition
// over the original nodes, from the first local-moving result to the coarsest.
func louvainLevels(g *graph, resolution float64) []levelPartition {
	var levels []levelPartition
	if g == nil || g.n == 0 || g.m2 == 0 {
		return levels
	}
	current := g
	super := make([]int32, g.n)
	for i := range super {
		super[i] = int32(i)
	}
	for level := 0; level < louvainMaxLevels; level++ {
		adj := current.csr()
		comm := localMoving(current, adj, resolution)
		count := 0
		for _, c := range comm {
			if int(c)+1 > count {
				count = int(c) + 1
			}
		}
		part := make([]int32, g.n)
		for v := range part {
			part[v] = comm[super[v]]
		}
		part = renumber(part)
		levels = append(levels, levelPartition{
			community:  part,
			count:      count,
			modularity: modularity(part, g, resolution),
		})
		if count <= 1 || count == current.n {
			break
		}
		current = aggregate(current, comm, count)
		for v := range super {
			super[v] = comm[super[v]]
		}
	}
	return levels
}

// modularity evaluates Q = 2*W_in/m2 - resolution*sum(tot^2)/m2^2 for a
// partition over g's nodes. The sum over communities uses ascending order so
// the float result is reproducible.
func modularity(part []int32, g *graph, resolution float64) float64 {
	if g.m2 == 0 {
		return 0
	}
	count := 0
	for _, c := range part {
		if int(c)+1 > count {
			count = int(c) + 1
		}
	}
	total := make([]float64, count)
	for i, d := range g.degree {
		total[part[i]] += d
	}
	wIn := 0.0
	for _, e := range g.edges {
		if part[e.a] == part[e.b] {
			wIn += e.weight
		}
	}
	sumSquares := 0.0
	for _, t := range total {
		sumSquares += t * t
	}
	return 2*wIn/g.m2 - resolution*sumSquares/(g.m2*g.m2)
}

// induced builds the subgraph over the given node members. Members are indices
// into g; the result uses local indices in members order.
func induced(g *graph, members []int) *graph {
	index := make([]int32, g.n)
	for i := range index {
		index[i] = -1
	}
	for i, member := range members {
		index[member] = int32(i)
	}
	edges := make([]edge, 0, len(g.edges))
	for _, e := range g.edges {
		a, b := index[e.a], index[e.b]
		if a < 0 || b < 0 {
			continue
		}
		edges = append(edges, edge{a: int(a), b: int(b), weight: e.weight})
	}
	return newGraph(len(members), edges)
}

// tooFragmented reports whether a partition splits the group into so many
// pieces that it is not a useful architectural subdivision.
func tooFragmented(count, members int) bool {
	return count > 1 && count*4 > members*3
}

// splitPartition picks the coarsest level with at most maxChildren communities,
// preferring a meaningful subdivision over a fragmented or trivial one.
func splitPartition(levels []levelPartition, maxChildren, members int) ([]int32, bool) {
	if len(levels) == 0 {
		return nil, false
	}
	for i := len(levels) - 1; i >= 0; i-- {
		if levels[i].count > 1 && levels[i].count <= maxChildren && !tooFragmented(levels[i].count, members) {
			return levels[i].community, true
		}
	}
	coarsest := levels[len(levels)-1]
	if coarsest.count > 1 && !tooFragmented(coarsest.count, members) {
		return coarsest.community, true
	}
	return nil, false
}

// selectRoot picks the root partition: the coarsest level that stays within the
// readable group budget. When the budget cannot be met the finest meaningful
// split wins over a single giant component.
func selectRoot(levels []levelPartition, opts Options) (levelPartition, bool) {
	if len(levels) == 0 {
		return levelPartition{}, false
	}
	for i := len(levels) - 1; i >= 0; i-- {
		if levels[i].count >= opts.MinRootGroups && levels[i].count <= opts.MaxRootGroups {
			return levels[i], true
		}
	}
	coarsest := levels[len(levels)-1]
	if coarsest.count >= opts.MinRootGroups {
		return coarsest, true
	}
	finest := levels[0]
	if finest.count >= opts.MinRootGroups {
		return finest, true
	}
	return levelPartition{}, false
}
