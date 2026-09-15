import { describe, expect, it } from 'vitest'
import type { ImpactReport } from '../api/client'
import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../types'
import { buildImpactLens, filterExploreData, impactedConnectorIds, impactedElementIds } from './impactGraph'

function node(id: number, parentViewId: number | null, children: ViewTreeNode[] = []): ViewTreeNode {
  return {
    id,
    owner_element_id: null,
    name: `view-${id}`,
    description: null,
    level_label: null,
    tags: [],
    level: 0,
    depth: 0,
    created_at: '',
    updated_at: '',
    parent_view_id: parentViewId,
    children,
  }
}

function placement(id: number, viewId: number, elementId: number): PlacedElement {
  return {
    id,
    view_id: viewId,
    element_id: elementId,
    position_x: 0,
    position_y: 0,
    name: `el-${elementId}`,
    description: null,
    kind: 'component',
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
    relationship: 'calls',
    direction: 'forward',
    connector_type: 'default',
    style: 'solid',
    source_handle: null,
    target_handle: null,
    tags: [],
    created_at: '',
    updated_at: '',
  }
}

function exploreData(): ExploreData {
  return {
    tree: [node(1, null, [node(2, 1)])],
    views: {
      '1': {
        placements: [placement(1, 1, 10), placement(2, 1, 11), placement(3, 1, 12)],
        connectors: [connector(100, 1, 10, 11)],
      },
      '2': { placements: [placement(4, 2, 13)], connectors: [] },
    },
    navigations: [],
  }
}

function report(): ImpactReport {
  return {
    base: 'a',
    head: 'b',
    repo_root: '/repo',
    changed: [
      {
        ref: 'a',
        name: 'A',
        kind: 'component',
        element_id: 10,
        change: 'modified',
        evidence: [],
      },
    ],
    candidates: [
      {
        ref: 'c',
        name: 'C',
        kind: 'component',
        element_id: 12,
        change: 'unknown',
        evidence: [],
      },
    ],
    related: [
      {
        ref: 'b',
        name: 'B',
        kind: 'component',
        element_id: 11,
        change: 'unknown',
        evidence: [],
      },
    ],
    edges: [
      {
        source_ref: 'a',
        target_ref: 'b',
        label: 'calls',
        connector_id: 100,
        observed: false,
      },
    ],
    unmapped: [],
    changed_files: [],
    coverage: {
      applicable: true,
      complete: true,
      score: 1,
      percent: 100,
      confidence: 'high',
      source_files: 1,
      bound_source_files: 1,
      weak_source_files: 0,
      unmapped_source_files: 0,
      non_source_files: 0,
      anchored_elements: 1,
      total_elements: 1,
      gaps: [],
    },
  }
}

describe('impactGraph', () => {
  it('collects impacted element and connector ids', () => {
    const ids = impactedElementIds(report())
    expect([...ids].sort()).toEqual([10, 11, 12])
    expect([...impactedConnectorIds(report())]).toEqual([100])
  })

  it('filters explore data to the impacted subgraph and keeps ancestors', () => {
    const filtered = filterExploreData(exploreData(), new Set([10, 11, 12]), new Set([100]))
    expect(filtered.tree.map((item) => item.id)).toEqual([1])
    expect(filtered.views['1'].placements.map((item) => item.element_id).sort()).toEqual([10, 11, 12])
    expect(filtered.views['1'].connectors).toHaveLength(1)
    expect(filtered.views['2']).toBeUndefined()
  })

  it('builds a diff lens marking changed, context, and connectors', () => {
    const lens = buildImpactLens(report())
    expect(lens.elementChanges.get(10)).toBe('updated')
    expect(lens.contextElementIds.has(11)).toBe(true)
    expect(lens.contextElementIds.has(12)).toBe(true)
    expect(lens.connectorChanges.get(100)).toBe('updated')
  })
})
