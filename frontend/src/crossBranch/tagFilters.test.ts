import { describe, expect, it } from 'vitest'
import type { Connector, PlacedElement, ViewTreeNode } from '../types'
import { buildWorkspaceGraphSnapshot } from './graph'
import { filterSnapshotConnectorsByTags } from './tagFilters'

describe('proxy connector tag filtering', () => {
  it('filters before aggregation and preserves topology and the original snapshot', () => {
    const placements = [
      { element_id: 1, tags: ['api'] }, { element_id: 2, tags: ['storage'] }, { element_id: 3, tags: ['api'] },
    ] as PlacedElement[]
    const connectors = [
      { id: 10, view_id: 1, source_element_id: 1, target_element_id: 2, tags: ['critical'] },
      { id: 11, view_id: 1, source_element_id: 1, target_element_id: 2, tags: [] },
      { id: 12, view_id: 1, source_element_id: 1, target_element_id: 3, tags: [] },
    ] as Connector[]
    const snapshot = buildWorkspaceGraphSnapshot({
      tree: [{ id: 1, name: 'Root', children: [] } as unknown as ViewTreeNode], navigations: [],
      views: { 1: { placements, connectors } },
    })
    const filtered = filterSnapshotConnectorsByTags(snapshot, ['critical'])
    expect(filtered.connectorsByViewId[1].map((edge) => edge.id)).toEqual([10])
    expect(filtered.placementsByElementId).toBe(snapshot.placementsByElementId)
    expect(filterSnapshotConnectorsByTags(snapshot, ['api']).connectorsByViewId[1].map((edge) => edge.id)).toEqual([12])
    expect(filterSnapshotConnectorsByTags(snapshot, [], ['critical']).connectorsByViewId[1].map((edge) => edge.id)).toEqual([11, 12])
    expect(filterSnapshotConnectorsByTags(snapshot, ['critical'], ['storage']).connectorsByViewId[1]).toEqual([])
    expect(snapshot.connectorsByViewId[1]).toHaveLength(3)
    expect(filterSnapshotConnectorsByTags(snapshot)).toBe(snapshot)
  })
})
