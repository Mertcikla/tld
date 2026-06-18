package mermaid

type Point struct {
	X float64
	Y float64
}

type ConnectorHandles struct {
	Source string
	Target string
}

func ConnectorHandlesForDirection(direction Direction) ConnectorHandles {
	switch direction {
	case DirectionRL:
		return ConnectorHandles{Source: "left", Target: "right"}
	case DirectionTB, DirectionTD:
		return ConnectorHandles{Source: "bottom", Target: "top"}
	case DirectionBT:
		return ConnectorHandles{Source: "top", Target: "bottom"}
	default:
		return ConnectorHandles{Source: "right", Target: "left"}
	}
}

func LayoutImport(parsed *ParsedDiagram, center Point) map[string]Point {
	if parsed == nil {
		return map[string]Point{}
	}
	refs := make([]string, 0, len(parsed.Elements))
	metadataPositions := map[string]Point{}
	for _, element := range parsed.Elements {
		if element.GetRef() == "" {
			continue
		}
		refs = append(refs, element.GetRef())
		if len(element.GetPlacements()) == 0 {
			continue
		}
		placement := element.GetPlacements()[0]
		if placement.PositionX == nil || placement.PositionY == nil {
			continue
		}
		metadataPositions[element.GetRef()] = Point{X: placement.GetPositionX(), Y: placement.GetPositionY()}
	}
	if len(metadataPositions) == 0 {
		return layoutRefs(parsed, refs, center)
	}

	left, right, top, bottom := bounds(metadataPositions)
	metadataCenter := Point{X: left + (right-left)/2, Y: top + (bottom-top)/2}
	positions := map[string]Point{}
	for ref, position := range metadataPositions {
		positions[ref] = Point{X: center.X + position.X - metadataCenter.X, Y: center.Y + position.Y - metadataCenter.Y}
	}

	missingRefs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, ok := metadataPositions[ref]; !ok {
			missingRefs = append(missingRefs, ref)
		}
	}
	if len(missingRefs) == 0 {
		return positions
	}

	horizontal := parsed.Direction == DirectionLR || parsed.Direction == DirectionRL
	reverse := parsed.Direction == DirectionRL || parsed.Direction == DirectionBT
	metadataWidth := right - left
	metadataHeight := bottom - top
	offset := 0.0
	fallbackCenter := center
	if horizontal {
		offset = maxFloat(360, metadataWidth/2+320)
		if reverse {
			offset = -offset
		}
		fallbackCenter.X += offset
	} else {
		offset = maxFloat(240, metadataHeight/2+220)
		if reverse {
			offset = -offset
		}
		fallbackCenter.Y += offset
	}
	for ref, position := range layoutRefs(parsed, missingRefs, fallbackCenter) {
		positions[ref] = position
	}
	return positions
}

func layoutRefs(parsed *ParsedDiagram, refs []string, center Point) map[string]Point {
	refSet := map[string]struct{}{}
	outgoing := map[string][]string{}
	indegree := map[string]int{}
	rank := map[string]int{}
	for _, ref := range refs {
		refSet[ref] = struct{}{}
		outgoing[ref] = []string{}
		indegree[ref] = 0
		rank[ref] = 0
	}
	for _, connector := range parsed.Connectors {
		source := connector.GetSourceElementRef()
		target := connector.GetTargetElementRef()
		if _, ok := refSet[source]; !ok {
			continue
		}
		if _, ok := refSet[target]; !ok {
			continue
		}
		outgoing[source] = append(outgoing[source], target)
		indegree[target]++
	}

	queue := []string{}
	for _, ref := range refs {
		if indegree[ref] == 0 {
			queue = append(queue, ref)
		}
	}
	cursor := 0
	for cursor < len(queue) {
		ref := queue[cursor]
		cursor++
		for _, target := range outgoing[ref] {
			rank[target] = maxInt(rank[target], rank[ref]+1)
			indegree[target]--
			if indegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}

	groups := map[int][]string{}
	for index, ref := range refs {
		refRank := index / 4
		if cursor == len(refs) {
			refRank = rank[ref]
		}
		groups[refRank] = append(groups[refRank], ref)
	}

	keys := make([]int, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sortInts(keys)

	horizontal := parsed.Direction == DirectionLR || parsed.Direction == DirectionRL
	reverse := parsed.Direction == DirectionRL || parsed.Direction == DirectionBT
	rankSpacing := 280.0
	itemSpacing := 150.0
	rankCount := len(groups)
	if rankCount == 0 {
		rankCount = 1
	}
	positions := map[string]Point{}
	for _, groupRank := range keys {
		group := groups[groupRank]
		rankOffset := (float64(groupRank) - (float64(rankCount)-1)/2) * rankSpacing
		if reverse {
			rankOffset *= -1
		}
		for index, ref := range group {
			itemOffset := (float64(index) - (float64(len(group))-1)/2) * itemSpacing
			if horizontal {
				positions[ref] = Point{X: center.X + rankOffset, Y: center.Y + itemOffset}
			} else {
				positions[ref] = Point{X: center.X + itemOffset, Y: center.Y + rankOffset}
			}
		}
	}
	return positions
}

func bounds(values map[string]Point) (left, right, top, bottom float64) {
	first := true
	for _, value := range values {
		if first {
			left, right, top, bottom = value.X, value.X, value.Y, value.Y
			first = false
			continue
		}
		if value.X < left {
			left = value.X
		}
		if value.X > right {
			right = value.X
		}
		if value.Y < top {
			top = value.Y
		}
		if value.Y > bottom {
			bottom = value.Y
		}
	}
	return left, right, top, bottom
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
