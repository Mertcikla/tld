package impact

import (
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func dependencyWitnesses(seeds []string, pairs map[[2]string]float64, snapshotID string, radius uint32) map[string]*pb.ImpactWitness {
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
	out := map[string]*pb.ImpactWitness{}
	for _, seed := range queue {
		out[seed] = &pb.ImpactWitness{SnapshotId: snapshotID, Paths: []string{seed}}
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
			out[neighbour] = &pb.ImpactWitness{SnapshotId: snapshotID, Paths: append(append([]string(nil), witness.Paths...), neighbour)}
			queue = append(queue, neighbour)
		}
	}
	return out
}
