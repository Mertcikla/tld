import { subNodeHandleAnchor, subSymbolAtPoint } from './subElements'
import { sourceDetailZoom, sourceWidthCapScale } from './sourceDetailScale'
import { describe, expect, it } from 'vitest'
import { buildSceneGraph } from './sceneGraph'
import { subDetail as codeDetail } from '../../utils/subElements'
import { updateScene, nodeVisualGeometry, nodeVisualLayout } from './layoutEngine'
import { getExpandThresholds } from './layoutEngine'
import { hitTestZUIRenderedNode } from './hitTest'
import { calculateMaxZoom, isZUIPanMouseButton, shouldZoomZUIWheel, zoomAround } from './useZUIInteraction'
import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'
import type { WheelDeltaLike } from '../../utils/wheel'

const thresholds = getExpandThresholds(1000)

function wheel(overrides: Partial<WheelDeltaLike>): WheelDeltaLike {
  return {
    deltaX: 0,
    deltaY: 0,
    deltaMode: 0,
    ctrlKey: false,
    ...overrides,
  }
}

function node(
  id: string,
  elementId: number,
  x: number,
  y: number,
  options: Partial<Pick<LayoutNode, 'worldW' | 'worldH' | 'children' | 'childScale' | 'childOffsetX' | 'childOffsetY' | 'tags'>> = {},
): LayoutNode {
  return {
    id,
    elementId,
    diagramId: 1,
    worldX: x,
    worldY: y,
    worldW: options.worldW ?? 100,
    worldH: options.worldH ?? 80,
    label: id,
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: options.tags ?? [],
    ancestorElementIds: [],
    pathElementIds: [elementId],
    children: options.children ?? [],
    childScale: options.childScale ?? 1,
    childOffsetX: options.childOffsetX ?? 0,
    childOffsetY: options.childOffsetY ?? 0,
    edgesOut: [],
  }
}

function group(nodes: LayoutNode[]): DiagramGroupLayout {
  return {
    diagramId: 1,
    label: 'Root',
    description: null,
    level: 0,
    levelLabel: null,
    worldX: 0,
    worldY: 0,
    worldW: 600,
    worldH: 400,
    diagramW: 600,
    diagramH: 400,
    diagramX: 0,
    diagramY: 0,
    nodes,
    edges: [],
  }
}

describe('ZUI node hit testing', () => {
  it('does not return fully transitioned parents as hover targets', () => {
    const child = node('child', 2, 10, 10, { worldW: 20, worldH: 20 })
    const parent = node('parent', 1, 0, 0, { children: [child] })
    const view: ZUIViewState = { x: 0, y: 0, zoom: 5 }

    const hit = hitTestZUIRenderedNode(80, 70, [group([parent])], view, thresholds)

    expect(hit).toBeNull()
  })

  it('returns visible nested children under faded parents', () => {
    const child = node('child', 2, 10, 10, { worldW: 20, worldH: 20 })
    const parent = node('parent', 1, 0, 0, { children: [child] })
    const view: ZUIViewState = { x: 0, y: 0, zoom: 5 }

    const hit = hitTestZUIRenderedNode(15, 15, [group([parent])], view, thresholds)

    expect(hit?.node.id).toBe('child')
  })

  it('skips hidden-tag nodes so they do not block lower rendered nodes', () => {
    const bottom = node('bottom', 1, 0, 0)
    const topHidden = node('top-hidden', 2, 0, 0, { tags: ['infra'] })
    const view: ZUIViewState = { x: 0, y: 0, zoom: 1 }

    const hit = hitTestZUIRenderedNode(20, 20, [group([bottom, topHidden])], view, thresholds, new Set(['infra']))

    expect(hit?.node.id).toBe('bottom')
  })

  it('hides group members when their group tag is hidden', () => {
    const marker = 'group:12345678-1234-4234-a234-123456789012'
    const member = node('member', 1, 0, 0, { tags: [marker] })
    const view: ZUIViewState = { x: 0, y: 0, zoom: 1 }

    const hit = hitTestZUIRenderedNode(20, 20, [group([member])], view, thresholds, new Set([marker]))

    expect(hit).toBeNull()
  })

  it('uses rendered topmost order for overlapping nodes', () => {
    const bottom = node('bottom', 1, 0, 0)
    const top = node('top', 2, 0, 0)
    const view: ZUIViewState = { x: 0, y: 0, zoom: 1 }

    const hit = hitTestZUIRenderedNode(20, 20, [group([bottom, top])], view, thresholds)

    expect(hit?.node.id).toBe('top')
  })
  it.each(['outline', 'code'] as const)('caps detailed %s cards and keeps their hit bounds aligned', subDetail => {
    const leaf: LayoutNode = { ...node('code', 1, 0, 0, { worldW: 520, worldH: 360 }), type: 'class', subKind: 'code', subDetail }
    const groups = [group([leaf])]
    const view = { x: -2400, y: -1600, zoom: 10 }
    const graph = buildSceneGraph({ groups, bbox: { minX: 0, minY: 0, maxX: 520, maxY: 360 } })
    updateScene(graph, view, 1000, 600, thresholds)
    const state = graph.nodeById.get(leaf.id)!.state
    expect(state.isLeafCapped).toBe(true)
    expect(state.screenW).toBeCloseTo(520 * sourceWidthCapScale)
    const hit = hitTestZUIRenderedNode(260, 180, groups, view, thresholds)!
    expect(hit.absW * view.zoom).toBeCloseTo(520 * sourceWidthCapScale)
    expect(hit.visualScale * view.zoom).toBeCloseTo(state.drawZoom)
    expect(hit.cumulativeScale).toBe(1)
    expect(hitTestZUIRenderedNode(10, 10, groups, view, thresholds)).toBeNull()
  })

  it('limits deep-zoom leaf hit areas to the visible capped card', () => {
    const leaf = node('leaf', 1, 0, 0)
    const view = { x: 0, y: 0, zoom: 10 }
    expect(hitTestZUIRenderedNode(5, 40, [group([leaf])], view, thresholds)).toBeNull()
    const hit = hitTestZUIRenderedNode(50, 40, [group([leaf])], view, thresholds)
    expect(hit).toMatchObject({ absX: 30, absY: 24, absW: 40, absH: 32 })
  })

})

describe('ZUI gesture zoom', () => {
  it('uses the same max zoom over a parent node, a leaf node, and empty canvas', () => {
    const child = node('child', 3, 10, 10, { worldW: 20, worldH: 20 })
    const parent = node('parent', 1, 0, 0, { children: [child] })
    const leaf = node('leaf', 2, 200, 0)
    const groups = [group([parent, leaf])]
    const view: ZUIViewState = { x: 0, y: 0, zoom: 1 }
    const maxZoom = 40

    expect(hitTestZUIRenderedNode(20, 20, groups, view, thresholds)?.node.id).toBe('parent')
    expect(hitTestZUIRenderedNode(220, 20, groups, view, thresholds)?.node.id).toBe('leaf')
    expect(hitTestZUIRenderedNode(520, 320, groups, view, thresholds)).toBeNull()

    const overParent = zoomAround(view, 20, 20, 100, maxZoom)
    const overLeaf = zoomAround(view, 220, 20, 100, maxZoom)
    const overEmpty = zoomAround(view, 520, 320, 100, maxZoom)

    expect(overParent.zoom).toBe(maxZoom)
    expect(overLeaf.zoom).toBe(maxZoom)
    expect(overEmpty.zoom).toBe(maxZoom)
  })
})

describe('ZUI mouse pan buttons', () => {
  it('allows primary and middle mouse drags to pan', () => {
    expect(isZUIPanMouseButton(0)).toBe(true)
    expect(isZUIPanMouseButton(1)).toBe(true)
    expect(isZUIPanMouseButton(2)).toBe(false)
  })
})

describe('ZUI wheel gestures', () => {
  it('zooms notched pixel-mode and line-mode mouse wheels', () => {
    expect(shouldZoomZUIWheel(wheel({ deltaY: 120 }), false)).toBe(true)
    expect(shouldZoomZUIWheel(wheel({ deltaY: 3, deltaMode: 1 }), false)).toBe(true)
  })

  it('pans smooth vertical trackpad wheel input instead of zooming', () => {
    expect(shouldZoomZUIWheel(wheel({ deltaY: 6 }), false)).toBe(false)
    expect(shouldZoomZUIWheel(wheel({ deltaY: 20.5 }), false)).toBe(false)
  })

  it('keeps two-axis trackpad wheel gestures available for panning', () => {
    expect(shouldZoomZUIWheel(wheel({ deltaX: 8, deltaY: 20 }), false)).toBe(false)
    expect(shouldZoomZUIWheel(wheel({ deltaY: 6 }), true)).toBe(false)
  })
})


it.each([320, 800])('allows full code detail on a %spx canvas with aligned hit bounds', canvasW => {
  const leaf: LayoutNode = { ...node('source', 1, 0, 0, { worldW: 520, worldH: 360 }), type: 'code', subKind: 'code' }
  const groups = [group([leaf])]
  const thresholds = getExpandThresholds(canvasW)
  const maxZoom = calculateMaxZoom(groups, canvasW, { x: 0, y: 0, zoom: 0.5 }, 600)
  const view = { x: canvasW / 2 - 260 * maxZoom, y: 300 - 180 * maxZoom, zoom: maxZoom }
  const graph = buildSceneGraph({ groups, bbox: { minX: 0, minY: 0, maxX: 520, maxY: 360 } })
  updateScene(graph, view, canvasW, 600, thresholds)
  const state = graph.nodeById.get(leaf.id)!.state
  expect(codeDetail(leaf.worldW * state.drawZoom, leaf.worldH * state.drawZoom)).toBe('code')
  const hit = hitTestZUIRenderedNode(260, 180, groups, view, thresholds)!
  expect(hit.visualScale * view.zoom).toBeCloseTo(state.drawZoom)
  expect(hit.absW * view.zoom).toBeCloseTo(state.screenW)
})


it.each([320, 1000, 2000])('extends compact code leaves to detail level 300 on a %spx canvas', canvasW => {
  const leaf: LayoutNode = { ...node('compact-source', 1, 0, 0, { worldW: 180, worldH: 85 }), type: 'file', subKind: 'code' }
  const groups = [group([leaf])]
  // Independent reference mapping: level 100 used to cap the leaf; 300 extends it.
  const expectedZoom = 0.1 + 299 / 99 * (281 / 85 - 0.1)
  const maxZoom = calculateMaxZoom(groups, canvasW, { x: 0, y: 0, zoom: 1 }, 600)
  expect(maxZoom).toBeCloseTo(expectedZoom)
  const view = { x: canvasW / 2 - 90 * maxZoom, y: 300 - 42.5 * maxZoom, zoom: maxZoom * 2 }
  const graph = buildSceneGraph({ groups, bbox: { minX: 0, minY: 0, maxX: 180, maxY: 85 } })
  const thresholds = getExpandThresholds(canvasW)
  updateScene(graph, view, canvasW, 600, thresholds)
  const state = graph.nodeById.get(leaf.id)!.state
  expect(state.isLeafCapped).toBe(true)
  expect(state.screenW).toBeCloseTo(180 * (0.1 + 179 / 99 * (281 / 85 - 0.1)))
  expect(codeDetail(state.screenW, 85 * state.drawZoom)).toBe('code')
  const hit = hitTestZUIRenderedNode(90, 42.5, groups, view, thresholds)!
  expect(hit.visualScale * view.zoom).toBeCloseTo(state.drawZoom)
  expect(hit.absW * view.zoom).toBeCloseTo(state.screenW)
})


it('freezes source headers at 100, width at 180, and extends only the bottom through 300', () => {
  const leaf: LayoutNode = { ...node('growing-source', 1, 0, 0, { worldW: 180, worldH: 85 }), type: 'file', subKind: 'code' }
  const metrics = (level: number) => {
    const zoom = sourceDetailZoom(level)
    const geometry = nodeVisualGeometry(leaf, zoom, thresholds.end)
    return {
      width: geometry.width * zoom, height: geometry.height * zoom,
      header: geometry.headerHeight * geometry.scale * zoom,
      top: (geometry.offsetY - leaf.worldH / 2) * zoom,
    }
  }
  const at100 = metrics(100), at180 = metrics(180), at240 = metrics(240), at300 = metrics(300), at400 = metrics(400)
  expect(at180.width).toBeGreaterThan(at100.width)
  expect(at180.header).toBeCloseTo(at100.header)
  expect(at240.header).toBeCloseTo(at100.header)
  expect(at300.header).toBeCloseTo(at100.header)
  expect(at240.width).toBeCloseTo(at180.width)
  expect(at300.width).toBeCloseTo(at180.width)
  expect(at240.top).toBeCloseTo(at180.top)
  expect(at300.top).toBeCloseTo(at180.top)
  expect(at240.height).toBeGreaterThan(at180.height)
  expect(at300.height).toBeGreaterThan(at240.height)
  expect(at400.height).toBeCloseTo(at300.height)
})

it('keeps newly revealed source rows, ports and bottom hit areas aligned', () => {
  const leaf: LayoutNode = { ...node('growing-source', 1, 0, 0, { worldW: 180, worldH: 85 }), type: 'file', subKind: 'code',
    subSource: { status: 'ready', lines: Array.from({ length: 100 }, (_, i) => `line ${i + 1}`), tokens: [], symbols: [{ name: 'late', startLine: 30, endLine: 30 }], startLine: 1, endLine: 100 },
    subConnectedHandles: ['right-line-30'],
  }
  const groups = [group([leaf])]
  const zoom = sourceDetailZoom(300)
  const geometry = nodeVisualGeometry(leaf, zoom, thresholds.end)
  const localY = geometry.offsetY + geometry.height - 1
  expect(localY).toBeGreaterThan(leaf.worldH)
  const hit = hitTestZUIRenderedNode(90, localY, groups, { x: 0, y: 0, zoom }, thresholds)!
  expect(hit.node).toBe(leaf)
  expect(hit.absY + hit.absH).toBeCloseTo(geometry.offsetY + geometry.height)
  const preview = nodeVisualLayout(leaf, geometry)
  const anchor = subNodeHandleAnchor(preview, 'right-line-30', zoom * geometry.scale)!
  expect(anchor.y * geometry.scale).toBeLessThan(geometry.height - 5)
  expect(subSymbolAtPoint(preview, zoom * geometry.scale, 20, anchor.y)).toBe('right-symbol-%3Alate')
})
