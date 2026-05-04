import { describe, expect, it } from 'vitest'
import { computeLayout } from './layout'
import { findDiagramFocusTarget, findElementFocusTarget, viewportForDiagramFocusTarget, viewportForElementFocusTarget, viewportForFocusTarget, type ZUIFocusTarget } from './focus'
import { calculateMaxZoom, constrainViewState } from './useZUIInteraction'
import { buildCameraTransitionRebase, getExpandThresholds } from './renderer'
import type { ExploreData, PlacedElement, ViewConnector, ViewTreeNode } from '../../types'
import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'

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

function testNode(id: string, children: LayoutNode[] = [], childScale = 0.8): LayoutNode {
  return {
    id,
    elementId: Number(id.replace(/\D/g, '')) || 1,
    diagramId: 1,
    worldX: 0,
    worldY: 0,
    worldW: 180,
    worldH: 85,
    label: id,
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: [],
    ancestorElementIds: [],
    pathElementIds: [],
    children,
    childScale,
    childOffsetX: 0,
    childOffsetY: 0,
    edgesOut: [],
  }
}

function testGroup(nodes: LayoutNode[]): DiagramGroupLayout {
  return {
    diagramId: 1,
    label: 'Root',
    description: null,
    level: 0,
    levelLabel: null,
    worldX: 0,
    worldY: 0,
    worldW: 180,
    worldH: 85,
    diagramW: 180,
    diagramH: 85,
    diagramX: 0,
    diagramY: 0,
    nodes,
    edges: [],
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

function focusMatrixExploreData(depth: number): ExploreData {
  const treeById = new Map<number, ViewTreeNode>()
  for (let viewId = depth; viewId >= 1; viewId -= 1) {
    treeById.set(
      viewId,
      treeNode(
        viewId,
        `Matrix View ${viewId}`,
        viewId === 1 ? null : 10_000 + viewId - 1,
        viewId === 1 ? null : viewId - 1,
        viewId < depth ? [treeById.get(viewId + 1)!] : [],
      ),
    )
  }

  const views: ExploreData['views'] = {}
  const navigations: ViewConnector[] = []
  for (let viewId = 1; viewId <= depth; viewId += 1) {
    const childOwnerId = 10_000 + viewId
    const childBearing = viewId < depth
      ? [placed(viewId, childOwnerId, viewId % 2 === 0 ? 1600 : -1500, viewId % 3 === 0 ? -1250 : 1350, true)]
      : []
    views[viewId] = {
      placements: [
        ...childBearing,
        placed(viewId, viewId * 100 + 1, -2600 + viewId * 37, 1800 - viewId * 53),
        placed(viewId, viewId * 100 + 2, 2400 - viewId * 41, -2100 + viewId * 47),
        placed(viewId, viewId * 100 + 3, viewId * 180, -viewId * 140),
      ],
      connectors: [],
    }
    if (viewId < depth) {
      navigations.push(navigation(viewId, childOwnerId, viewId + 1))
    }
  }

  return {
    tree: [treeById.get(1)!],
    views,
    navigations,
  }
}

function placementsIn(data: ExploreData): Array<{ viewId: number; elementId: number }> {
  return Object.entries(data.views).flatMap(([viewIdText, content]) => {
    const viewId = Number(viewIdText)
    return (content.placements ?? []).map((placement) => ({ viewId, elementId: placement.element_id }))
  })
}

function viewsIn(data: ExploreData): number[] {
  return Object.keys(data.views).map(Number).filter(Number.isFinite)
}

function screenRect(target: ZUIFocusTarget, viewport: ZUIViewState) {
  return {
    left: target.absX * viewport.zoom + viewport.x,
    top: target.absY * viewport.zoom + viewport.y,
    right: (target.absX + target.absW) * viewport.zoom + viewport.x,
    bottom: (target.absY + target.absH) * viewport.zoom + viewport.y,
    width: target.absW * viewport.zoom,
    height: target.absH * viewport.zoom,
  }
}

function interpolateViewState(from: ZUIViewState, to: ZUIViewState, t: number): ZUIViewState {
  return {
    x: from.x + (to.x - from.x) * t,
    y: from.y + (to.y - from.y) * t,
    zoom: from.zoom + (to.zoom - from.zoom) * t,
  }
}

function completeFocusNavigationFromCurrent(
  current: ZUIViewState,
  destination: ZUIViewState,
  canvasW: number,
  canvasH: number,
  bbox: { minX: number; minY: number; maxX: number; maxY: number },
): ZUIViewState {
  ;[0.15, 0.5, 0.85].forEach((t) => {
    constrainViewState(interpolateViewState(current, destination, t), canvasW, canvasH, bbox)
  })
  return constrainViewState(destination, canvasW, canvasH, bbox)
}

function expectFiniteViewport(viewport: ZUIViewState, context: string) {
  expect(Number.isFinite(viewport.x), `${context} x`).toBe(true)
  expect(Number.isFinite(viewport.y), `${context} y`).toBe(true)
  expect(Number.isFinite(viewport.zoom), `${context} zoom`).toBe(true)
  expect(viewport.zoom, `${context} zoom`).toBeGreaterThan(0)
}

function expectScreenRectVisible(
  rect: ReturnType<typeof screenRect>,
  canvasW: number,
  canvasH: number,
  context: string,
) {
  const epsilon = 0.75
  expect(rect.left, `${context} left`).toBeGreaterThanOrEqual(-epsilon)
  expect(rect.top, `${context} top`).toBeGreaterThanOrEqual(-epsilon)
  expect(rect.right, `${context} right`).toBeLessThanOrEqual(canvasW + epsilon)
  expect(rect.bottom, `${context} bottom`).toBeLessThanOrEqual(canvasH + epsilon)
  expect(rect.width, `${context} width`).toBeGreaterThan(0)
  expect(rect.height, `${context} height`).toBeGreaterThan(0)
}

describe('ZUI focus targets', () => {
  it('rebases stacked camera-center transitions without forcing ancestor expansion', () => {
    const grandchild = testNode('node-3', [])
    const child = testNode('node-2', [grandchild])
    const parent = testNode('node-1', [child])
    const groups = [testGroup([parent])]
    const thresholds = getExpandThresholds(1200)

    const rebase = buildCameraTransitionRebase(
      groups,
      { x: 420, y: 315, zoom: 2.2 },
      1200,
      800,
      thresholds,
    )

    expect(rebase.preserveChildAlphaNodeIds.has('node-1')).toBe(true)
    expect(rebase.preserveChildAlphaNodeIds.has('node-2')).toBe(false)
  })

  it('does not rebase when only one camera-center transition is active', () => {
    const child = testNode('node-2', [])
    const parent = testNode('node-1', [child])
    const groups = [testGroup([parent])]
    const thresholds = getExpandThresholds(1200)

    const rebase = buildCameraTransitionRebase(
      groups,
      { x: 420, y: 315, zoom: 1.9 },
      1200,
      800,
      thresholds,
    )

    expect(rebase.preserveChildAlphaNodeIds.size).toBe(0)
  })

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

  it('can navigate and zoom to every placed element across viewport sizes, levels, and current cameras', () => {
    const data = focusMatrixExploreData(6)
    const layout = computeLayout(data)
    const canvasCases = [
      { name: 'desktop', w: 1200, h: 800, isMobile: false, leafMinWidth: 320 },
      { name: 'mobile', w: 390, h: 720, isMobile: true, leafMinWidth: 220 },
      { name: 'ultrawide', w: 1800, h: 900, isMobile: false, leafMinWidth: 320 },
    ]
    const currentViewports: ZUIViewState[] = [
      { x: 0, y: 0, zoom: 0.4 },
      { x: -25_000, y: 18_000, zoom: 0.8 },
      { x: 40_000, y: -35_000, zoom: 24 },
    ]

    for (const { name, w, h, isMobile, leafMinWidth } of canvasCases) {
      const maxZoom = calculateMaxZoom(layout.groups, w)
      const thresholds = getExpandThresholds(w)
      for (const { viewId, elementId } of placementsIn(data)) {
        const target = findElementFocusTarget(layout.groups, viewId, elementId)
        expect(target, `${name} view ${viewId} element ${elementId} target`).not.toBeNull()
        const viewport = viewportForElementFocusTarget(target!, w, h, maxZoom, isMobile)
        expect(viewport, `${name} view ${viewId} element ${elementId} viewport`).not.toBeNull()
        expectFiniteViewport(viewport!, `${name} view ${viewId} element ${elementId}`)

        for (const current of currentViewports) {
          const finalViewport = completeFocusNavigationFromCurrent(current, viewport!, w, h, layout.bbox)
          const rect = screenRect(target!, finalViewport)
          const context = `${name} from ${current.x}/${current.y}/${current.zoom} to view ${viewId} element ${elementId}`
          expectScreenRectVisible(rect, w, h, context)

          const expectedMinWidth = target!.node?.children.length ? Math.min(leafMinWidth, thresholds.start) : leafMinWidth
          expect(rect.width, `${context} usable width`).toBeGreaterThanOrEqual(expectedMinWidth - 0.75)
        }
      }
    }
  })

  it('can navigate and zoom to every linked view target without losing the content center', () => {
    const data = focusMatrixExploreData(6)
    const layout = computeLayout(data)
    const canvasW = 1200
    const canvasH = 800
    const maxZoom = calculateMaxZoom(layout.groups, canvasW)

    for (const viewId of viewsIn(data)) {
      const target = findDiagramFocusTarget(layout.groups, viewId)
      expect(target, `view ${viewId} target`).not.toBeNull()
      const viewport = viewportForDiagramFocusTarget(target!, canvasW, canvasH, maxZoom, false)
      expect(viewport, `view ${viewId} viewport`).not.toBeNull()
      expectFiniteViewport(viewport!, `view ${viewId}`)

      const finalViewport = constrainViewState(viewport!, canvasW, canvasH, layout.bbox)
      const rect = screenRect(target!, finalViewport)
      expect(rect.width, `view ${viewId} target width`).toBeGreaterThan(0)
      expect(rect.height, `view ${viewId} target height`).toBeGreaterThan(0)
      expect((rect.left + rect.right) / 2, `view ${viewId} target center x`).toBeGreaterThanOrEqual(0)
      expect((rect.left + rect.right) / 2, `view ${viewId} target center x`).toBeLessThanOrEqual(canvasW)
      expect((rect.top + rect.bottom) / 2, `view ${viewId} target center y`).toBeGreaterThanOrEqual(0)
      expect((rect.top + rect.bottom) / 2, `view ${viewId} target center y`).toBeLessThanOrEqual(canvasH)

      if (target!.contentRect) {
        const contentRect = {
          left: target!.contentRect.x * finalViewport.zoom + finalViewport.x,
          top: target!.contentRect.y * finalViewport.zoom + finalViewport.y,
          right: (target!.contentRect.x + target!.contentRect.w) * finalViewport.zoom + finalViewport.x,
          bottom: (target!.contentRect.y + target!.contentRect.h) * finalViewport.zoom + finalViewport.y,
          width: target!.contentRect.w * finalViewport.zoom,
          height: target!.contentRect.h * finalViewport.zoom,
        }
        expect(contentRect.width, `view ${viewId} content width`).toBeGreaterThan(0)
        expect(contentRect.height, `view ${viewId} content height`).toBeGreaterThan(0)
        expect((contentRect.left + contentRect.right) / 2, `view ${viewId} content center x`).toBeGreaterThanOrEqual(0)
        expect((contentRect.left + contentRect.right) / 2, `view ${viewId} content center x`).toBeLessThanOrEqual(canvasW)
        expect((contentRect.top + contentRect.bottom) / 2, `view ${viewId} content center y`).toBeGreaterThanOrEqual(0)
        expect((contentRect.top + contentRect.bottom) / 2, `view ${viewId} content center y`).toBeLessThanOrEqual(canvasH)
      }
    }
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
