import { describe, expect, it } from 'vitest'
import { buildWorkspaceGraphSnapshot } from './graph'
import { resolveZUIProxyConnectors } from './resolve'
import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../types'

function placedElement(view_id: number, element_id: number, name: string): PlacedElement {
  return {
    id: view_id * 100 + element_id,
    view_id,
    element_id,
    position_x: element_id * 10,
    position_y: 0,
    name,
    description: null,
    kind: 'service',
    technology: null,
    url: null,
    logo_url: null,
    technology_connectors: [],
    tags: [],
    has_view: element_id === 1 || element_id === 3,
    view_label: null,
  }
}

function connector(id: number, view_id: number, source_element_id: number, target_element_id: number, label: string): Connector {
  return {
    id,
    view_id,
    source_element_id,
    target_element_id,
    label,
    description: null,
    relationship: null,
    direction: 'forward',
    style: 'bezier',
    url: null,
    source_handle: null,
    target_handle: null,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
  }
}

const tree: ViewTreeNode[] = [{
  id: 1,
  owner_element_id: null,
  name: 'Root',
  description: null,
  level_label: null,
  level: 0,
  depth: 0,
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
  parent_view_id: null,
  children: [{
    id: 2,
    owner_element_id: 1,
    name: 'A Child',
    description: null,
    level_label: null,
    level: 1,
    depth: 1,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    parent_view_id: 1,
    children: [{
      id: 3,
      owner_element_id: 3,
      name: 'AA Child',
      description: null,
      level_label: null,
      level: 2,
      depth: 2,
      created_at: '2024-01-01',
      updated_at: '2024-01-01',
      parent_view_id: 2,
      children: [],
    }],
  }],
}]

function baseData(connectors: Connector[]): ExploreData {
  return {
    tree,
    navigations: [
      { id: 1, element_id: 1, from_view_id: 1, to_view_id: 2, to_view_name: 'A Child', relation_type: 'child' },
      { id: 2, element_id: 3, from_view_id: 2, to_view_id: 3, to_view_name: 'AA Child', relation_type: 'child' },
    ],
    views: {
      '1': {
        placements: [placedElement(1, 1, 'A'), placedElement(1, 2, 'B')],
        connectors,
      },
      '2': {
        placements: [placedElement(2, 3, 'AA')],
        connectors: [],
      },
      '3': {
        placements: [placedElement(3, 4, 'AAA')],
        connectors: [],
      },
    },
  }
}

describe('resolveZUIProxyConnectors', () => {
  it('collapses direct-child cross-branch links into a native +N badge', () => {
    const snapshot = buildWorkspaceGraphSnapshot(baseData([
      connector(1, 1, 1, 2, 'A-B'),
      connector(2, 1, 3, 2, 'AA-B'),
    ]))

    const resolved = resolveZUIProxyConnectors(
      snapshot,
      new Map([
        [1, 'd1-o1'],
        [2, 'd1-o2'],
      ]),
      { enabled: true, depth: 5 },
    )

    expect(resolved.connectors).toHaveLength(0)
    expect(resolved.hiddenBadges).toHaveLength(1)
    expect(resolved.hiddenBadges[0]).toMatchObject({
      sourceAnchorElementId: 1,
      targetAnchorElementId: 2,
      count: 1,
    })
  })

  it('fractures the badge into direct child and parent connectors when the child is visible', () => {
    const snapshot = buildWorkspaceGraphSnapshot(baseData([
      connector(1, 1, 1, 2, 'A-B'),
      connector(2, 1, 3, 2, 'AA-B'),
    ]))

    const resolved = resolveZUIProxyConnectors(
      snapshot,
      new Map([
        [1, 'd1-o1'],
        [2, 'd1-o2'],
        [3, 'd2-o3'],
      ]),
      { enabled: true, depth: 5 },
    )

    expect(resolved.hiddenBadges).toHaveLength(0)
    expect(resolved.connectors.map((item) => [item.sourceAnchorElementId, item.targetAnchorElementId])).toEqual([[2, 3]])
  })

  it('keeps only the deepest visible connector and its parent once grandchildren are visible', () => {
    const snapshot = buildWorkspaceGraphSnapshot(baseData([
      connector(1, 1, 1, 2, 'A-B'),
      connector(2, 1, 4, 2, 'AAA-B'),
    ]))

    const resolved = resolveZUIProxyConnectors(
      snapshot,
      new Map([
        [1, 'd1-o1'],
        [2, 'd1-o2'],
        [3, 'd2-o3'],
        [4, 'd3-o4'],
      ]),
      { enabled: true, depth: 5 },
    )

    expect(resolved.hiddenBadges).toHaveLength(0)
    expect(resolved.connectors.map((item) => [item.sourceAnchorElementId, item.targetAnchorElementId]).sort()).toEqual([
      [2, 3],
      [2, 4],
    ])
  })
})
