import type { NodeData } from './ElementNode'

export type HandleReconnectCandidate = NonNullable<NodeData['reconnectCandidates']>[number]

export function compareReconnectCandidates(left: HandleReconnectCandidate, right: HandleReconnectCandidate) {
  if (left.selected !== right.selected) return left.selected ? -1 : 1
  return left.edgeId.localeCompare(right.edgeId)
}

