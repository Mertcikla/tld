import { describe, expect, it } from 'vitest'
import type { WatchDiff } from '../api/client'
import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../types'
import { buildExploreDiffLens, sourcePathFromDiff } from './exploreDiffLens'

function viewNode(id: number, name: string, parentViewId: number | null = null, ownerElementId: number | null = null, children: ViewTreeNode[] = []): ViewTreeNode {
  return {
    id,
    owner_element_id: ownerElementId,
    name,
    description: null,
    level_label: null,
    level: parentViewId ? 2 : 1,
    depth: parentViewId ? 1 : 0,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    parent_view_id: parentViewId,
    children,
  }
}

function placed(viewId: number, elementId: number, name: string): PlacedElement {
  return {
    id: viewId * 1000 + elementId,
    view_id: viewId,
    element_id: elementId,
    position_x: 0,
    position_y: 0,
    name,
    description: null,
    kind: 'service',
    technology: null,
    url: null,
    logo_url: null,
    technology_connectors: [],
    tags: [],
    has_view: false,
    view_label: null,
  }
}

function connector(id: number, viewId: number, source: number, target: number): Connector {
  return {
    id,
    view_id: viewId,
    source_element_id: source,
    target_element_id: target,
    label: 'calls',
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

function diff(overrides: Partial<WatchDiff>): WatchDiff {
  return {
    id: 1,
    version_id: 9,
    owner_type: 'symbol',
    owner_key: 'go:services/api/main.go:function:Serve',
    change_type: 'updated',
    resource_type: 'element',
    resource_id: 301,
    summary: 'Serve',
    added_lines: 3,
    removed_lines: 1,
    ...overrides,
  }
}

describe('explore diff lens', () => {
  const tree = [
    viewNode(1, 'Root', null, null, [
      viewNode(2, 'Payments', 1, 101),
    ]),
  ]
  const data: ExploreData = {
    tree,
    navigations: [],
    views: {
      1: {
        placements: [placed(1, 101, 'Payments'), placed(1, 102, 'Identity')],
        connectors: [connector(501, 1, 101, 102)],
      },
      2: {
        placements: [placed(2, 301, 'Checkout API'), placed(2, 302, 'Ledger')],
        connectors: [connector(601, 2, 301, 302)],
      },
    },
  }

  it('keeps changed nodes, ancestor path, and sibling context', () => {
    const lens = buildExploreDiffLens(data, [diff({})], 9)

    expect(lens.changedElementIds.has(301)).toBe(true)
    expect(lens.ancestorElementIds.has(101)).toBe(true)
    expect(lens.siblingElementIds.has(302)).toBe(true)
    expect(lens.siblingElementIds.has(102)).toBe(true)
    expect(lens.contextElementIds.has(101)).toBe(true)
    expect(lens.contextElementIds.has(302)).toBe(true)
    expect(lens.contextElementIds.has(301)).toBe(false)
  })

  it('indexes connector changes and adjacent context connectors', () => {
    const lens = buildExploreDiffLens(data, [diff({ resource_type: 'connector', resource_id: 601, summary: 'calls' })], 9)

    expect(lens.connectorChanges.get(601)).toBe('updated')
    expect(lens.contextElementIds.has(301)).toBe(true)
    expect(lens.contextElementIds.has(302)).toBe(true)
    expect(lens.contextConnectorIds.has(501)).toBe(true)
  })

  it('places missing resources into the unplaced tray', () => {
    const lens = buildExploreDiffLens(data, [diff({ resource_id: 999, summary: 'Removed service', change_type: 'deleted' })], 9)

    expect(lens.orderedTargets).toHaveLength(0)
    expect(lens.unplacedTargets).toEqual([
      expect.objectContaining({
        resourceId: 999,
        changeType: 'deleted',
        label: 'Removed service',
        unplaced: true,
      }),
    ])
  })

  it('keeps file-only changes available in the unplaced tray', () => {
    const lens = buildExploreDiffLens(data, [
      diff({
        owner_type: 'file',
        owner_key: 'services/api/main.go',
        resource_type: 'file',
        resource_id: undefined,
        summary: 'services/api/main.go',
      }),
    ], 9)

    expect(lens.unplacedTargets).toEqual([
      expect.objectContaining({
        resourceType: 'file',
        label: 'services/api/main.go',
        sourcePath: 'services/api/main.go',
      }),
    ])
  })

  it('preserves summaries, line deltas, and source paths', () => {
    const lens = buildExploreDiffLens(data, [diff({})], 9)

    expect(lens.elementLineDeltas.get(301)).toEqual({ added: 3, removed: 1 })
    expect(lens.diffDetailsByResource.get('element:301')).toEqual(expect.objectContaining({
      summary: 'Serve',
      addedLines: 3,
      removedLines: 1,
      sourcePath: 'services/api/main.go',
    }))
    expect(lens.totalAddedLines).toBe(3)
    expect(lens.totalRemovedLines).toBe(1)
  })

  it('extracts source paths from common owner key shapes', () => {
    expect(sourcePathFromDiff({ owner_type: 'file', owner_key: 'cmd/root.go' })).toBe('cmd/root.go')
    expect(sourcePathFromDiff({ owner_type: 'symbol', owner_key: 'go:internal/app/app.go:function:Run' })).toBe('internal/app/app.go')
    expect(sourcePathFromDiff({ owner_type: 'reference', owner_key: 'symbol:go:a.go:function:A:go:b.go:function:B:call' })).toBe('a.go')
  })
})
