import { describe, expect, it } from 'vitest'
import {
  buildJumpSearchResults,
  flattenTree,
  jumpResultActionLabel,
  jumpResultSubtitle,
} from './viewsJumpSearch'
import type { ExploreData, PlacedElement, ViewTreeNode } from '../types'

function viewNode(id: number, name: string, children: ViewTreeNode[] = [], level = 1): ViewTreeNode {
  return {
    id,
    owner_element_id: null,
    name,
    description: null,
    level_label: level === 1 ? 'System' : null,
    level,
    depth: level - 1,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    parent_view_id: null,
    children,
  }
}

function placed(viewId: number, elementId: number, name: string, overrides: Partial<PlacedElement> = {}): PlacedElement {
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
    ...overrides,
  }
}

function exploreData(tree: ViewTreeNode[], placementsByView: Record<string, PlacedElement[]>): ExploreData {
  return {
    tree,
    navigations: [],
    views: Object.fromEntries(
      Object.entries(placementsByView).map(([viewId, placements]) => [
        viewId,
        { placements, connectors: [] },
      ]),
    ),
  }
}

describe('views jump search', () => {
  const tree = [
    viewNode(1, 'Workspace', [
      viewNode(2, 'Payments', [], 2),
      viewNode(3, 'Platform Payments', [], 2),
      viewNode(4, 'Identity', [], 2),
    ]),
  ]
  const flatTree = flattenTree(tree)

  it('flattens the view tree in navigation order', () => {
    expect(flatTree.map((node) => node.name)).toEqual([
      'Workspace',
      'Payments',
      'Platform Payments',
      'Identity',
    ])
  })

  it('returns view results without requiring workspace placement data', () => {
    const results = buildJumpSearchResults('payments', flatTree, null)

    expect(results).toEqual([
      {
        type: 'view',
        key: 'view:2',
        name: 'Payments',
        viewId: 2,
        level: 2,
        levelLabel: null,
      },
      {
        type: 'view',
        key: 'view:3',
        name: 'Platform Payments',
        viewId: 3,
        level: 2,
        levelLabel: null,
      },
    ])
  })

  it('builds element navigation results from names and metadata fields', () => {
    const data = exploreData(tree, {
      2: [
        placed(2, 201, 'Checkout API', {
          kind: 'api',
          technology: 'Node',
          file_path: 'services/checkout/index.ts',
          tags: ['critical-path'],
        }),
      ],
      4: [
        placed(4, 401, 'Session Store', {
          kind: 'database',
          technology: 'Redis',
          tags: ['auth'],
        }),
      ],
    })

    const byName = buildJumpSearchResults('checkout', flatTree, data)
    expect(byName).toContainEqual({
      type: 'element',
      key: 'element:2:201',
      name: 'Checkout API',
      viewId: 2,
      viewName: 'Payments',
      elementId: 201,
      kind: 'api',
    })

    const byTag = buildJumpSearchResults('critical', flatTree, data)
    expect(byTag.map((result) => result.key)).toEqual(['element:2:201'])

    const byTechnology = buildJumpSearchResults('redis', flatTree, data)
    expect(byTechnology.map((result) => result.key)).toEqual(['element:4:401'])

    const byPath = buildJumpSearchResults('services/checkout', flatTree, data)
    expect(byPath.map((result) => result.key)).toEqual(['element:2:201'])
  })

  it('keeps same-element placements in different views as distinct navigation targets', () => {
    const data = exploreData(tree, {
      2: [placed(2, 501, 'Shared Logger')],
      3: [placed(3, 501, 'Shared Logger')],
    })

    const results = buildJumpSearchResults('shared logger', flatTree, data)

    expect(results).toEqual([
      expect.objectContaining({ type: 'element', key: 'element:2:501', viewId: 2, elementId: 501 }),
      expect.objectContaining({ type: 'element', key: 'element:3:501', viewId: 3, elementId: 501 }),
    ])
  })

  it('caps result groups while preserving view-first ordering', () => {
    const manyViews = [
      viewNode(10, 'Alpha Root', [
        viewNode(11, 'Alpha Billing', [], 2),
        viewNode(12, 'Alpha Catalog', [], 2),
        viewNode(13, 'Alpha Checkout', [], 2),
        viewNode(14, 'Alpha Delivery', [], 2),
        viewNode(15, 'Alpha Events', [], 2),
      ]),
    ]
    const manyFlatTree = flattenTree(manyViews)
    const data = exploreData(manyViews, {
      11: Array.from({ length: 8 }, (_, index) => placed(11, 700 + index, `Alpha Element ${index}`)),
    })

    const results = buildJumpSearchResults('alpha', manyFlatTree, data)

    expect(results).toHaveLength(8)
    expect(results.filter((result) => result.type === 'view')).toHaveLength(4)
    expect(results.filter((result) => result.type === 'element')).toHaveLength(4)
    expect(results.slice(0, 4).every((result) => result.type === 'view')).toBe(true)
  })

  it('formats result hints used by the toolbar', () => {
    expect(buildJumpSearchResults('id', flatTree, null)).toEqual([])
    expect(jumpResultActionLabel('explore')).toBe('ZOOM')
    expect(jumpResultActionLabel('hierarchy')).toBe('OPEN')

    const [viewResult] = buildJumpSearchResults('payments', flatTree, null)
    expect(jumpResultSubtitle(viewResult)).toBe('Level 2 • Diagram')

    const data = exploreData(tree, {
      4: [placed(4, 401, 'Session Store', { kind: null })],
    })
    const [elementResult] = buildJumpSearchResults('session', flatTree, data)
    expect(jumpResultSubtitle(elementResult)).toBe('Element • Identity')
  })
})
