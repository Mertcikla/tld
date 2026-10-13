import { describe, expect, it } from 'vitest'
import type { Connector, ExploreData, LibraryElement, PlacedElement } from '../types'
import { buildWorkspaceGraphSnapshot, overrideViewContentInSnapshot, removeConnectorFromSnapshot, removePlacementFromSnapshot, upsertConnectorInSnapshot, upsertPlacementInSnapshot } from './graph'
import { resolveViewProxyGraph, resolveZUIProxyConnectors } from './resolve'
import type { CrossBranchContextSettings } from './types'
import { UNPLACED_ELEMENTS_BOUNDARY_ID, UNPLACED_ELEMENTS_NAME, UNPLACED_ELEMENTS_VIEW_ID, withUnplacedElements } from './unplacedElements'
import { buildContextSummaryForest } from '../pages/ViewEditor/hooks/contextSummaryTree'

const settings: CrossBranchContextSettings = { enabled: true, depth: 5, connectorBudget: 50, connectorPriority: 'external' }
const element = (id: number, name: string): LibraryElement => ({
  id, name, kind: 'service', description: 'Description', technology: 'Go',
  logo_url: null, url: null, technology_connectors: [], tags: [],
  created_at: '', updated_at: '', has_view: false, view_label: null,
})
const placement = (id: number, name: string): PlacedElement => ({
  ...element(id, name), id: id * 100, element_id: id, view_id: 1, position_x: 0, position_y: 0,
})
const connector = (id: number, source: number, target: number): Connector => ({
  id, view_id: 1, source_element_id: source, target_element_id: target,
  label: 'calls', description: null, relationship: null, direction: 'forward', style: 'bezier',
  url: null, source_handle: null, target_handle: null, created_at: '', updated_at: '',
})
function data(connectors = [connector(1, 1, 2)]): ExploreData {
  return {
    tree: [{ id: 1, name: 'Root', owner_element_id: null, parent_view_id: null, description: null, level: 0, depth: 0, level_label: null, children: [], created_at: '', updated_at: '' }],
    views: { '1': { placements: [placement(1, 'API')], connectors } },
    navigations: [],
    elements: [element(1, 'API'), element(2, 'Billing'), element(3, 'Queue'), element(4, 'Unused')],
  }
}

describe('unplaced element boundary', () => {
  it('groups only connected elements with no real placement, without changing source data', () => {
    const source = data([connector(1, 1, 2), connector(2, 2, 3), connector(3, 1, 2)])
    const before = structuredClone(source)
    const derived = withUnplacedElements(source)
    const boundary = derived.views[UNPLACED_ELEMENTS_VIEW_ID]
    expect(boundary.placements.map((item) => [item.element_id, item.name])).toEqual([[2, 'Billing'], [3, 'Queue']])
    expect(new Set(boundary.placements.map((item) => `${item.position_x}:${item.position_y}`)).size).toBe(2)
    expect(derived.tree.at(-1)?.name).toBe(UNPLACED_ELEMENTS_NAME)
    expect(withUnplacedElements(derived)).toEqual(derived)
    expect(source).toEqual(before)
    const snapshot = buildWorkspaceGraphSnapshot(derived)
    expect(snapshot.source.views[UNPLACED_ELEMENTS_VIEW_ID]).toBeUndefined()
    expect(snapshot.source.tree.map((view) => view.id)).toEqual([1])
  })

  it('moves elements out of the synthetic boundary when placed and back when their last placement is removed', () => {
    const initial = buildWorkspaceGraphSnapshot(data())
    const placed = upsertPlacementInSnapshot(initial, 1, placement(2, 'Billing'))!
    expect(placed.placementsByElementId[2].map((ref) => ref.viewId)).toEqual([1])
    expect(placed.viewById[UNPLACED_ELEMENTS_VIEW_ID]).toBeUndefined()
    const removed = removePlacementFromSnapshot(placed, 1, 2)!
    expect(removed.placementsByElementId[2][0].viewId).toBe(UNPLACED_ELEMENTS_VIEW_ID)
    expect(removed.placementsByElementId[2][0].element.name).toBe('Billing')
    const disconnected = removeConnectorFromSnapshot(removed, 1, 1)!
    expect(disconnected.viewById[UNPLACED_ELEMENTS_VIEW_ID]).toBeUndefined()
  })

  it('preserves loaded names when the editor has no inventory cache and includes freshly selected targets', () => {
    const snapshot = buildWorkspaceGraphSnapshot(data())
    const overridden = overrideViewContentInSnapshot(snapshot, 1, snapshot.placementsByViewId[1], [], [])!
    expect(overridden.placementsByElementId[2][0].element.name).toBe('Billing')
    const fresh = upsertConnectorInSnapshot(overridden, connector(2, 1, 5), [element(5, 'New target')])!
    expect(fresh.placementsByElementId[5][0].element.name).toBe('New target')
  })

  it('resolves off-view connectors through one expandable boundary without inventing navigation destinations', () => {
    const snapshot = buildWorkspaceGraphSnapshot(data([connector(1, 1, 2), connector(2, 1, 3)]))
    const resolved = resolveViewProxyGraph(snapshot, 1, snapshot.placementsByViewId[1], settings)
    expect(resolved.proxyConnectors).toHaveLength(2)
    const endpoints = resolved.proxyConnectors.flatMap((item) => item.details.connectors.flatMap((leaf) => [leaf.source, leaf.target])).filter((endpoint) => endpoint.externalToView)
    expect(endpoints.map((endpoint) => endpoint.actualElementName).sort()).toEqual(['Billing', 'Queue'])
    expect(endpoints.every((endpoint) => endpoint.anchorViewId === null && endpoint.placementViewId === null)).toBe(true)
    const forest = buildContextSummaryForest(endpoints)
    expect(forest.rootIds).toHaveLength(1)
    expect(forest.nodesById[forest.rootIds[0]].elementId).toBe(UNPLACED_ELEMENTS_BOUNDARY_ID)
    expect(forest.nodesById[forest.rootIds[0]].childIds).toHaveLength(2)
  })

  it('renders the target and its connector in Explore using virtual anchors', () => {
    const snapshot = buildWorkspaceGraphSnapshot(data())
    const resolved = resolveZUIProxyConnectors(snapshot, new Map([[1, 'd1-o1'], [2, `d${UNPLACED_ELEMENTS_VIEW_ID}-o2`]]), settings)
    expect(resolved.connectors).toHaveLength(1)
    expect(resolved.connectors[0].details.connectors[0].target.actualElementName).toBe('Billing')
    expect(resolved.connectors[0].details.connectors[0].target.anchorViewId).toBe(UNPLACED_ELEMENTS_VIEW_ID)
  })

  it('keeps connectors visible even when a dataset has no endpoint metadata or real placements', () => {
    const source = data([connector(1, 2, 3)])
    source.elements = []
    source.views['1'].placements = []
    const snapshot = buildWorkspaceGraphSnapshot(source)
    expect(snapshot.placementsByViewId[UNPLACED_ELEMENTS_VIEW_ID].map((item) => item.name)).toEqual(['Element 2', 'Element 3'])
    const resolved = resolveZUIProxyConnectors(snapshot, new Map([[2, `d${UNPLACED_ELEMENTS_VIEW_ID}-o2`], [3, `d${UNPLACED_ELEMENTS_VIEW_ID}-o3`]]), settings)
    expect(resolved.connectors).toHaveLength(1)
  })
})
