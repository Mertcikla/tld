import { deriveCanvasTagState } from '../utils/canvasTags'
import type { WorkspaceGraphSnapshot } from './types'

export function deriveSnapshotTagState(snapshot: WorkspaceGraphSnapshot, selectedTags: readonly string[] = [], hiddenTags: readonly string[] = []) {
  const nodes = Object.entries(snapshot.placementsByElementId).map(([id, placements]) => ({ id: Number(id), tags: placements[0]?.element.tags }))
  const connectors = Object.values(snapshot.connectorsByViewId).flat()
  return deriveCanvasTagState(nodes, connectors.map((edge) => ({
    id: edge.id, source: edge.source_element_id, target: edge.target_element_id, tags: edge.tags,
  })), selectedTags, hiddenTags)
}

/** Apply the same tag rules before aggregation, so proxies cannot restore excluded relationships. */
export function filterSnapshotConnectorsByTags(snapshot: WorkspaceGraphSnapshot, selectedTags: readonly string[] = [], hiddenTags: readonly string[] = []): WorkspaceGraphSnapshot {
  if (selectedTags.length === 0 && hiddenTags.length === 0) return snapshot
  const state = deriveSnapshotTagState(snapshot, selectedTags, hiddenTags)
  const hiddenDescendants = new Set(state.hiddenNodeIds)
  for (const id of state.hiddenNodeIds) {
    const childViewId = snapshot.childViewIdByOwnerElementId[id]
    if (childViewId === undefined) continue
    for (const viewId of snapshot.descendantsByViewId[childViewId] ?? [childViewId]) {
      for (const placement of snapshot.placementsByViewId[viewId] ?? []) hiddenDescendants.add(placement.element_id)
    }
  }
  return { ...snapshot, connectorsByViewId: Object.fromEntries(Object.entries(snapshot.connectorsByViewId).map(([viewId, edges]) => [
    viewId, edges.filter((edge) => state.visibleConnectorIds.has(edge.id) &&
      !hiddenDescendants.has(edge.source_element_id) && !hiddenDescendants.has(edge.target_element_id)),
  ])) }
}
