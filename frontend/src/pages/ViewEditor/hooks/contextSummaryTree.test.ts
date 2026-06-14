import { describe, expect, it } from 'vitest'
import type { CrossBranchContextSettings, ProxyEndpoint } from '../../../crossBranch/types'
import {
  autoExpandLeafBudget,
  buildContextSummaryForest,
  buildVisibleContextSummaryForest,
  contextSummaryBranchPath,
} from './contextSummaryTree'

function settings(connectorBudget: number): CrossBranchContextSettings {
  return {
    enabled: true,
    depth: 5,
    connectorBudget,
    connectorPriority: 'external',
  }
}

function endpoint(overrides: Partial<ProxyEndpoint> = {}): ProxyEndpoint {
  return {
    actualElementId: 6,
    actualElementName: 'Leaf',
    anchorElementId: 5,
    anchorElementName: 'Branch',
    anchorViewId: 1,
    anchorViewName: 'Root',
    placementViewId: 3,
    placementViewName: 'Sibling',
    depth: 1,
    externalToView: true,
    currentBranchElementId: 1,
    commonAncestorViewId: 1,
    commonAncestorViewName: 'Root',
    mergeAncestorElementId: null,
    contextPathElementIds: [],
    branchPathElementIds: [5, 6],
    ...overrides,
  }
}

describe('contextSummaryTree', () => {
  it('prefers additive branch ancestry metadata over legacy context paths', () => {
    expect(contextSummaryBranchPath(endpoint())).toEqual([5, 6])
    expect(contextSummaryBranchPath(endpoint({ branchPathElementIds: [], contextPathElementIds: [8, 9] }))).toEqual([8, 9])
  })

  it('auto-expands small roots once density is rich enough', () => {
    const forest = buildContextSummaryForest([
      endpoint({ actualElementId: 6, branchPathElementIds: [5, 6] }),
      endpoint({ actualElementId: 7, actualElementName: 'Leaf 2', branchPathElementIds: [5, 7] }),
    ])

    const visible = buildVisibleContextSummaryForest(forest, new Set(), settings(100))
    const rootId = forest.rootIds[0]
    const root = visible.nodesById[rootId]

    expect(root?.isAutoExpanded).toBe(true)
    expect(root?.visibleChildIds).toHaveLength(2)
    expect(Object.values(visible.leafToVisibleNodeId).sort()).toEqual(root.visibleChildIds.slice().sort())
  })

  it('keeps one-leaf roots collapsed even at rich density', () => {
    const forest = buildContextSummaryForest([
      endpoint({ actualElementId: 6, branchPathElementIds: [5, 6] }),
    ])

    const visible = buildVisibleContextSummaryForest(forest, new Set(), settings(200))
    const rootId = forest.rootIds[0]

    expect(visible.nodesById[rootId]?.isAutoExpanded).toBe(false)
    expect(Object.keys(visible.nodesById)).toEqual([rootId])
    expect(Object.values(visible.leafToVisibleNodeId)).toEqual([rootId])
  })

  it('keeps two-leaf roots collapsed at normal density', () => {
    const forest = buildContextSummaryForest([
      endpoint({ actualElementId: 6, branchPathElementIds: [5, 6] }),
      endpoint({ actualElementId: 7, actualElementName: 'Leaf 2', branchPathElementIds: [5, 7] }),
    ])

    const visible = buildVisibleContextSummaryForest(forest, new Set(), settings(50))
    const rootId = forest.rootIds[0]

    expect(Object.keys(visible.nodesById)).toEqual([rootId])
    expect(Object.values(visible.leafToVisibleNodeId)).toEqual([rootId, rootId])
  })

  it('collapses denser roots into one summary node by default', () => {
    const forest = buildContextSummaryForest([
      endpoint({ actualElementId: 6, branchPathElementIds: [5, 6] }),
      endpoint({ actualElementId: 7, actualElementName: 'Leaf 2', branchPathElementIds: [5, 7] }),
      endpoint({ actualElementId: 8, actualElementName: 'Leaf 3', branchPathElementIds: [5, 8] }),
    ])

    const visible = buildVisibleContextSummaryForest(forest, new Set(), settings(50))
    const rootId = forest.rootIds[0]

    expect(Object.keys(visible.nodesById)).toEqual([rootId])
    expect(Object.values(visible.leafToVisibleNodeId)).toEqual([rootId, rootId, rootId])
  })

  it('expands one level at a time for denser nested branches', () => {
    const forest = buildContextSummaryForest([
      endpoint({ actualElementId: 8, branchPathElementIds: [5, 6, 8] }),
      endpoint({ actualElementId: 9, actualElementName: 'Leaf 2', branchPathElementIds: [5, 6, 9] }),
      endpoint({ actualElementId: 10, actualElementName: 'Leaf 3', branchPathElementIds: [5, 7, 10] }),
    ])

    const rootId = forest.rootIds[0]
    const rootExpanded = buildVisibleContextSummaryForest(forest, new Set([rootId]), settings(50))
    const branchSix = Object.values(rootExpanded.nodesById).find((node) => node.pathElementIds.join(':') === '5:6')

    expect(rootExpanded.nodesById[rootId]?.visibleChildIds).toHaveLength(2)
    expect(branchSix?.visibleChildIds).toHaveLength(0)
    expect(rootExpanded.leafToVisibleNodeId).toMatchObject({
      [forest.nodesById[branchSix?.id ?? '']?.descendantLeafIds[0] ?? '']: branchSix?.id,
    })

    const branchExpanded = buildVisibleContextSummaryForest(forest, new Set([rootId, branchSix?.id ?? '']), settings(50))
    expect(branchExpanded.nodesById[branchSix?.id ?? '']?.visibleChildIds).toHaveLength(2)
  })

  it('tightens quiet-mode thresholds as connector budget drops', () => {
    expect(autoExpandLeafBudget(settings(10))).toBe(1)
    expect(autoExpandLeafBudget(settings(50))).toBe(1)
    expect(autoExpandLeafBudget(settings(100))).toBe(2)
    expect(autoExpandLeafBudget(settings(200))).toBe(4)
  })
})
