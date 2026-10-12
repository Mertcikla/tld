package impact

import (
	"sort"

	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

func dependencyWitnesses(seeds []string, pairs map[[2]string]float64, snapshotID string, radius uint32) map[string]*model.Witness {
	adjacency := map[string]map[string]bool{}
	for pair := range pairs {
		for _, edge := range [][2]string{pair, {pair[1], pair[0]}} {
			if adjacency[edge[0]] == nil {
				adjacency[edge[0]] = map[string]bool{}
			}
			adjacency[edge[0]][edge[1]] = true
		}
	}
	queue := append([]string(nil), seeds...)
	sort.Strings(queue)
	out := map[string]*model.Witness{}
	for _, seed := range queue {
		out[seed] = &model.Witness{SnapshotId: snapshotID, Paths: []string{seed}}
	}
	for index := 0; index < len(queue); index++ {
		path := queue[index]
		witness := out[path]
		if uint32(len(witness.Paths)-1) >= radius {
			continue
		}
		for _, neighbour := range sortedKeys(adjacency[path]) {
			if out[neighbour] != nil {
				continue
			}
			out[neighbour] = &model.Witness{SnapshotId: snapshotID, Paths: append(append([]string(nil), witness.Paths...), neighbour)}
			queue = append(queue, neighbour)
		}
	}
	return out
}
