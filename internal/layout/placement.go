package layout

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

const (
	PlacementNodeWidth        = 140.0
	PlacementNodeHeight       = 80.0
	PlacementGapX             = 260.0
	PlacementGapY             = 170.0
	PlacementMaxRowsPerColumn = 6
)

type Placement struct {
	ElementID int64
	X         float64
	Y         float64
}

type Connector struct {
	Source int64
	Target int64
}

func LayoutPlacements(placements []Placement, targets map[int64]struct{}, connectors []Connector, force bool) map[int64]Placement {
	if force || HasNoPreservedPlacements(placements, targets) {
		return OrganicPlacementLayout(targets, connectors)
	}

	next := map[int64]Placement{}
	positioned := map[int64]Placement{}
	for _, p := range placements {
		if _, isNew := targets[p.ElementID]; !isNew {
			positioned[p.ElementID] = p
		}
	}
	occupied := OccupiedPlacementCells(placements, targets)
	for _, elementID := range SortedInt64Set(targets) {
		x, y := BestIncrementalPlacementPosition(elementID, positioned, occupied, connectors)
		occupied[placementCellKey(x, y)] = struct{}{}
		positioned[elementID] = Placement{ElementID: elementID, X: x, Y: y}
		next[elementID] = positioned[elementID]
	}
	return next
}

func HasNoPreservedPlacements(placements []Placement, targets map[int64]struct{}) bool {
	if len(targets) == 0 {
		return false
	}
	for _, p := range placements {
		if _, isTarget := targets[p.ElementID]; !isTarget {
			return false
		}
	}
	return true
}

// OrganicPlacementLayout runs the force-directed layout on the target element
// set, using only the connectors that exist between those targets. Dependency
// levels are applied afterwards for a directional reading.
func OrganicPlacementLayout(targets map[int64]struct{}, connectors []Connector) map[int64]Placement {
	// #nosec G404
	return organicPlacementLayout(targets, connectors, uint64(time.Now().UnixNano()), true)
}

// DeterministicPlacementLayout is a reproducible layout for generated maps.
// Dependency levels are skipped: on real repository graphs the layered columns
// produced more edge crossings than the force layout alone.
func DeterministicPlacementLayout(targets map[int64]struct{}, connectors []Connector) map[int64]Placement {
	return organicPlacementLayout(targets, connectors, 1, false)
}

// DeterministicLayoutPlacements is LayoutPlacements for generated maps: when
// every existing placement belongs to the new target set the whole view is laid
// out with a fixed seed, otherwise new elements are placed incrementally next
// to their neighbors and preserved positions are kept.
func DeterministicLayoutPlacements(placements []Placement, targets map[int64]struct{}, connectors []Connector) map[int64]Placement {
	if HasNoPreservedPlacements(placements, targets) {
		return DeterministicPlacementLayout(targets, connectors)
	}
	return LayoutPlacements(placements, targets, connectors, false)
}

func organicPlacementLayout(targets map[int64]struct{}, connectors []Connector, seed uint64, directed bool) map[int64]Placement {
	nodeByID := make(map[int64]*Node, len(targets))
	nodes := make([]*Node, 0, len(targets))
	for _, id := range SortedInt64Set(targets) {
		n := &Node{ID: id}
		nodeByID[id] = n
		nodes = append(nodes, n)
	}

	var edges []*Edge
	for _, c := range connectors {
		src, srcOK := nodeByID[c.Source]
		tgt, tgtOK := nodeByID[c.Target]
		if srcOK && tgtOK {
			edges = append(edges, &Edge{Source: src, Target: tgt})
		}
	}

	OrganicLayoutSeeded(nodes, edges, seed)
	if directed {
		ApplyDirectedPlacementLevels(nodes, connectors, targets)
	}

	out := make(map[int64]Placement, len(nodes))
	for _, n := range nodes {
		out[n.ID] = Placement{ElementID: n.ID, X: n.X, Y: n.Y}
	}
	return out
}

func ApplyDirectedPlacementLevels(nodes []*Node, connectors []Connector, targets map[int64]struct{}) {
	if len(nodes) == 0 {
		return
	}
	level := DirectedPlacementLevels(targets, connectors)
	maxLevel := 0
	for _, value := range level {
		if value > maxLevel {
			maxLevel = value
		}
	}
	if maxLevel == 0 {
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
		for i, n := range nodes {
			n.X = float64(i/PlacementMaxRowsPerColumn) * PlacementGapX
			n.Y = float64(i%PlacementMaxRowsPerColumn) * PlacementGapY
		}
		return
	}
	nodesByLevel := map[int][]*Node{}
	for _, n := range nodes {
		nodesByLevel[level[n.ID]] = append(nodesByLevel[level[n.ID]], n)
	}
	levels := sortedPlacementNodeLevels(nodesByLevel)
	for _, col := range levels {
		group := nodesByLevel[col]
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].Y == group[j].Y {
				return group[i].ID < group[j].ID
			}
			return group[i].Y < group[j].Y
		})
	}
	ReducePlacementCrossings(nodesByLevel, levels, connectors)
	nextCol := 0
	for _, col := range levels {
		group := nodesByLevel[col]
		for row, n := range group {
			n.X = float64(nextCol+row/PlacementMaxRowsPerColumn) * PlacementGapX
			n.Y = float64(row%PlacementMaxRowsPerColumn) * PlacementGapY
		}
		nextCol += max(1, (len(group)+PlacementMaxRowsPerColumn-1)/PlacementMaxRowsPerColumn)
	}
}

// ReducePlacementCrossings reorders each level using the barycenter heuristic
// (repeated alternating sweeps) so layered views do not read as a hairball.
func ReducePlacementCrossings(byLevel map[int][]*Node, levels []int, connectors []Connector) {
	const sweeps = 6
	for sweep := 0; sweep < sweeps; sweep++ {
		changed := false
		for i := 1; i < len(levels); i++ {
			if reorderByReference(byLevel[levels[i]], byLevel[levels[i-1]], connectors) {
				changed = true
			}
		}
		for i := len(levels) - 2; i >= 0; i-- {
			if reorderByReference(byLevel[levels[i]], byLevel[levels[i+1]], connectors) {
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func reorderByReference(group, reference []*Node, connectors []Connector) bool {
	if len(group) < 2 {
		return false
	}
	refIndex := make(map[int64]int, len(reference))
	for i, n := range reference {
		refIndex[n.ID] = i
	}
	center := make(map[int64]float64, len(group))
	for i, n := range group {
		sum, count := 0.0, 0
		for _, c := range connectors {
			if c.Source == n.ID {
				if p, ok := refIndex[c.Target]; ok {
					sum += float64(p)
					count++
				}
			}
			if c.Target == n.ID {
				if p, ok := refIndex[c.Source]; ok {
					sum += float64(p)
					count++
				}
			}
		}
		if count == 0 {
			center[n.ID] = float64(i)
		} else {
			center[n.ID] = sum / float64(count)
		}
	}
	reordered := append([]*Node(nil), group...)
	sort.SliceStable(reordered, func(i, j int) bool {
		left, right := center[reordered[i].ID], center[reordered[j].ID]
		if left != right {
			return left < right
		}
		return reordered[i].ID < reordered[j].ID
	})
	changed := false
	for i := range reordered {
		if reordered[i] != group[i] {
			changed = true
			break
		}
	}
	copy(group, reordered)
	return changed
}

func DirectedPlacementLevels(targets map[int64]struct{}, connectors []Connector) map[int64]int {
	level := map[int64]int{}
	for id := range targets {
		level[id] = 0
	}
	for i := 0; i < len(targets); i++ {
		changed := false
		for _, c := range connectors {
			if _, ok := targets[c.Source]; !ok {
				continue
			}
			if _, ok := targets[c.Target]; !ok {
				continue
			}
			if level[c.Source] >= len(targets)-1 {
				continue
			}
			next := level[c.Source] + 1
			if level[c.Target] < next {
				level[c.Target] = next
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for id, value := range level {
		if value >= len(targets) {
			level[id] = 0
		}
	}
	return level
}

func sortedPlacementNodeLevels(values map[int][]*Node) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func BestIncrementalPlacementPosition(elementID int64, positioned map[int64]Placement, occupied map[string]struct{}, connectors []Connector) (float64, float64) {
	candidates := placementLayoutCandidates(positioned)
	bestX, bestY := 0.0, 0.0
	bestScore := math.Inf(1)
	for _, candidate := range candidates {
		if _, blocked := occupied[placementCellKey(candidate.X, candidate.Y)]; blocked {
			continue
		}
		score := incrementalPlacementScore(elementID, candidate, positioned, connectors)
		if score < bestScore {
			bestScore = score
			bestX, bestY = candidate.X, candidate.Y
		}
	}
	if math.IsInf(bestScore, 1) {
		return nearestFreePlacementCell(0, 0, occupied)
	}
	return bestX, bestY
}

func incrementalPlacementScore(elementID int64, candidate Placement, positioned map[int64]Placement, connectors []Connector) float64 {
	score := math.Abs(candidate.X)*0.01 + math.Abs(candidate.Y)*0.01
	candidateEdges := [][2]Placement{}
	existingEdges := [][2]Placement{}
	for _, c := range connectors {
		source, sourceOK := positioned[c.Source]
		target, targetOK := positioned[c.Target]
		if c.Source == elementID {
			source, sourceOK = candidate, true
		}
		if c.Target == elementID {
			target, targetOK = candidate, true
		}
		if sourceOK && targetOK {
			edge := [2]Placement{source, target}
			if c.Source == elementID || c.Target == elementID {
				candidateEdges = append(candidateEdges, edge)
				score += PlacementDistance(source, target)
			} else {
				existingEdges = append(existingEdges, edge)
			}
		}
	}
	if len(candidateEdges) == 0 {
		return score + nearestPlacementNeighborDistance(candidate, positioned)
	}
	for _, candidateEdge := range candidateEdges {
		for _, existingEdge := range existingEdges {
			if candidateEdge[0].ElementID == existingEdge[0].ElementID || candidateEdge[0].ElementID == existingEdge[1].ElementID ||
				candidateEdge[1].ElementID == existingEdge[0].ElementID || candidateEdge[1].ElementID == existingEdge[1].ElementID {
				continue
			}
			if placementSegmentsIntersect(candidateEdge[0], candidateEdge[1], existingEdge[0], existingEdge[1]) {
				score += 10000
			}
		}
	}
	return score
}

func placementLayoutCandidates(positioned map[int64]Placement) []Placement {
	minCol, maxCol, minRow, maxRow := 0, 4, 0, 3
	if len(positioned) > 0 {
		minCol, maxCol, minRow, maxRow = math.MaxInt, math.MinInt, math.MaxInt, math.MinInt
		for _, p := range positioned {
			col := int(math.Round(p.X / PlacementGapX))
			row := int(math.Round(p.Y / PlacementGapY))
			if col < minCol {
				minCol = col
			}
			if col > maxCol {
				maxCol = col
			}
			if row < minRow {
				minRow = row
			}
			if row > maxRow {
				maxRow = row
			}
		}
		minCol--
		maxCol += 2
		minRow--
		maxRow += 2
	}
	out := make([]Placement, 0, (maxCol-minCol+1)*(maxRow-minRow+1))
	for col := minCol; col <= maxCol; col++ {
		for row := minRow; row <= maxRow; row++ {
			out = append(out, Placement{X: float64(col) * PlacementGapX, Y: float64(row) * PlacementGapY})
		}
	}
	return out
}

func OccupiedPlacementCells(placements []Placement, ignored map[int64]struct{}) map[string]struct{} {
	occupied := map[string]struct{}{}
	for _, p := range placements {
		if _, ok := ignored[p.ElementID]; ok {
			continue
		}
		occupied[placementCellKey(p.X, p.Y)] = struct{}{}
	}
	return occupied
}

func nearestFreePlacementCell(x, y float64, occupied map[string]struct{}) (float64, float64) {
	baseCol := int(math.Round(x / PlacementGapX))
	baseRow := int(math.Round(y / PlacementGapY))
	for radius := range 200 {
		for col := baseCol - radius; col <= baseCol+radius; col++ {
			for row := baseRow - radius; row <= baseRow+radius; row++ {
				if absInt(col-baseCol) != radius && absInt(row-baseRow) != radius {
					continue
				}
				nx, ny := float64(col)*PlacementGapX, float64(row)*PlacementGapY
				if _, ok := occupied[placementCellKey(nx, ny)]; !ok {
					return nx, ny
				}
			}
		}
	}
	return x, y
}

func placementCellKey(x, y float64) string {
	return fmt.Sprintf("%d:%d", int(math.Round(x/PlacementGapX)), int(math.Round(y/PlacementGapY)))
}

func PlacementDistance(a, b Placement) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func nearestPlacementNeighborDistance(candidate Placement, positioned map[int64]Placement) float64 {
	if len(positioned) == 0 {
		return 0
	}
	best := math.Inf(1)
	for _, p := range positioned {
		if d := PlacementDistance(candidate, p); d < best {
			best = d
		}
	}
	return best
}

func placementCenter(p Placement) (float64, float64) {
	return p.X + PlacementNodeWidth/2, p.Y + PlacementNodeHeight/2
}

func placementSegmentsIntersect(a, b, c, d Placement) bool {
	ax, ay := placementCenter(a)
	bx, by := placementCenter(b)
	cx, cy := placementCenter(c)
	dx, dy := placementCenter(d)
	return segmentOrientation(ax, ay, cx, cy, dx, dy) != segmentOrientation(bx, by, cx, cy, dx, dy) &&
		segmentOrientation(ax, ay, bx, by, cx, cy) != segmentOrientation(ax, ay, bx, by, dx, dy)
}

func segmentOrientation(ax, ay, bx, by, cx, cy float64) int {
	value := (by-ay)*(cx-bx) - (bx-ax)*(cy-by)
	if math.Abs(value) < 0.000001 {
		return 0
	}
	if value > 0 {
		return 1
	}
	return -1
}

func SortedInt64Set(values map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
