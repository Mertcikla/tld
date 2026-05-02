import { describe, expect, it } from 'vitest'
import { computeLayout } from './layout'
import { findDiagramFocusTarget, findElementFocusTarget, viewportForFocusTarget, type ZUIFocusTarget } from './focus'
import { constrainViewState } from './useZUIInteraction'
import type { ExploreData, PlacedElement, ViewConnector, ViewTreeNode } from '../../types'
import type { ZUIViewState } from './types'

function treeNode(id: number, name: string, ownerElementId: number | null, parentViewId: number | null, children: ViewTreeNode[] = []): ViewTreeNode {
  return {
    id,
    owner_element_id: ownerElementId,
    name,
    description: null,
    level_label: null,
    level: 0,
    depth: parentViewId == null ? 0 : 1,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    parent_view_id: parentViewId,
    children,
  }
}

function placed(viewId: number, elementId: number, x: number, y: number, hasView = false): PlacedElement {
  return {
    id: viewId * 1000 + elementId,
    view_id: viewId,
    element_id: elementId,
    position_x: x,
    position_y: y,
    name: `Element ${elementId}`,
    description: null,
    kind: 'service',
    technology: null,
    url: null,
    logo_url: null,
    technology_connectors: [],
    tags: [],
    has_view: hasView,
    view_label: null,
  }
}

function navigation(fromViewId: number, elementId: number, toViewId: number): ViewConnector {
  return {
    id: toViewId,
    element_id: elementId,
    from_view_id: fromViewId,
    to_view_id: toViewId,
    to_view_name: `View ${toViewId}`,
    relation_type: 'child',
  }
}

function nestedExploreData(): ExploreData {
  return {
    tree: [
      treeNode(1, 'Root', null, null, [
        treeNode(2, 'Second', 101, 1, [
          treeNode(3, 'Third', 201, 2, [
            treeNode(4, 'Fourth', 301, 3),
          ]),
        ]),
      ]),
    ],
    views: {
      1: { placements: [placed(1, 101, 120, 100, true)], connectors: [] },
      2: { placements: [placed(2, 201, 200, 160, true)], connectors: [] },
      3: { placements: [placed(3, 301, 300, 220, true)], connectors: [] },
      4: {
        placements: [
          placed(4, 401, 40, 60),
          placed(4, 499, 10_000, 8_000),
        ],
        connectors: [],
      },
    },
    navigations: [
      navigation(1, 101, 2),
      navigation(2, 201, 3),
      navigation(3, 301, 4),
    ],
  }
}

function deepSingleChainExploreData(depth: number): ExploreData {
  const treeById = new Map<number, ViewTreeNode>()
  for (let viewId = depth; viewId >= 1; viewId -= 1) {
    treeById.set(
      viewId,
      treeNode(
        viewId,
        `View ${viewId}`,
        viewId === 1 ? null : 1000 + viewId - 1,
        viewId === 1 ? null : viewId - 1,
        viewId < depth ? [treeById.get(viewId + 1)!] : [],
      ),
    )
  }

  const views: ExploreData['views'] = {}
  const navigations: ViewConnector[] = []
  for (let viewId = 1; viewId <= depth; viewId += 1) {
    const elementId = viewId === depth ? 9001 : 1000 + viewId
    views[viewId] = {
      placements: [placed(viewId, elementId, viewId * 15, viewId * 10, viewId < depth)],
      connectors: [],
    }
    if (viewId < depth) {
      navigations.push(navigation(viewId, elementId, viewId + 1))
    }
  }

  return {
    tree: [treeById.get(1)!],
    views,
    navigations,
  }
}

function screenRect(target: ZUIFocusTarget, viewport: ZUIViewState) {
  return {
    left: target.absX * viewport.zoom + viewport.x,
    top: target.absY * viewport.zoom + viewport.y,
    right: (target.absX + target.absW) * viewport.zoom + viewport.x,
    bottom: (target.absY + target.absH) * viewport.zoom + viewport.y,
    width: target.absW * viewport.zoom,
  }
}

describe('ZUI focus targets', () => {
  it('finds and centers an element inside a deeply nested view', () => {
    const layout = computeLayout(nestedExploreData())
    const target = findElementFocusTarget(layout.groups, 4, 401)
    expect(target).not.toBeNull()

    const viewport = viewportForFocusTarget(target!, 1200, 800, 100_000, 0.18, {
      minTargetScreenW: 320,
      keepParentVisible: true,
    })
    expect(viewport).not.toBeNull()

    const constrained = constrainViewState(viewport!, 1200, 800, layout.bbox)
    const rect = screenRect(target!, constrained)
    expect(rect.left).toBeGreaterThanOrEqual(0)
    expect(rect.top).toBeGreaterThanOrEqual(0)
    expect(rect.right).toBeLessThanOrEqual(1200)
    expect(rect.bottom).toBeLessThanOrEqual(800)
    expect(rect.width).toBeGreaterThanOrEqual(320)
  })

  it('zooms nested view navigation far enough for the selected view contents to render', () => {
    const layout = computeLayout(nestedExploreData())
    const viewTarget = findDiagramFocusTarget(layout.groups, 4)
    const elementTarget = findElementFocusTarget(layout.groups, 4, 401)
    expect(viewTarget?.contentRect).toBeTruthy()
    expect(elementTarget).not.toBeNull()

    const viewport = viewportForFocusTarget(viewTarget!, 1200, 800, 100_000, 0.16, {
      preferContent: true,
      minTargetScreenW: 260,
      minChildScreenW: 104,
    })
    expect(viewport).not.toBeNull()

    const constrained = constrainViewState(viewport!, 1200, 800, layout.bbox)
    const rect = screenRect(elementTarget!, constrained)
    expect(rect.width).toBeGreaterThanOrEqual(104)
  })

  it('does not inflate sub-pixel nested content bounds when centering a deep view', () => {
    const layout = computeLayout(deepSingleChainExploreData(8))
    const viewTarget = findDiagramFocusTarget(layout.groups, 8)
    const elementTarget = findElementFocusTarget(layout.groups, 8, 9001)
    expect(viewTarget?.contentRect).toBeTruthy()
    expect(elementTarget).not.toBeNull()

    const viewport = viewportForFocusTarget(viewTarget!, 1200, 800, 1_000_000, 0.16, {
      preferContent: true,
      minTargetScreenW: 260,
      minChildScreenW: 104,
    })
    expect(viewport).not.toBeNull()

    const rect = screenRect(elementTarget!, constrainViewState(viewport!, 1200, 800, layout.bbox))
    expect(rect.left).toBeGreaterThanOrEqual(0)
    expect(rect.top).toBeGreaterThanOrEqual(0)
    expect(rect.right).toBeLessThanOrEqual(1200)
    expect(rect.bottom).toBeLessThanOrEqual(800)
    expect(rect.width).toBeGreaterThanOrEqual(104)
  })

  it('keeps a sub-pixel expandable element visible when capping child zoom', () => {
    const layout = computeLayout(deepSingleChainExploreData(8))
    const target = findElementFocusTarget(layout.groups, 7, 1007)
    expect(target?.node?.children.length).toBe(1)

    const viewport = viewportForFocusTarget(target!, 1200, 800, 1_000_000, 0.18, {
      minTargetScreenW: 320,
      keepParentVisible: true,
    })
    expect(viewport).not.toBeNull()

    const rect = screenRect(target!, constrainViewState(viewport!, 1200, 800, layout.bbox))
    expect(rect.left).toBeGreaterThanOrEqual(0)
    expect(rect.top).toBeGreaterThanOrEqual(0)
    expect(rect.right).toBeLessThanOrEqual(1200)
    expect(rect.bottom).toBeLessThanOrEqual(800)
    expect(rect.width).toBeGreaterThanOrEqual(320)
  })

  it('keeps focus centering available when the canvas is smaller than the old fixed padding', () => {
    const targetView = { x: 400, y: 300, zoom: 1 }
    const constrained = constrainViewState(targetView, 1000, 800, {
      minX: 0,
      minY: 0,
      maxX: 1600,
      maxY: 1200,
    })

    expect(constrained.x).toBe(targetView.x)
    expect(constrained.y).toBe(targetView.y)
  })
})
