import { describe, expect, it, vi } from 'vitest'
import {
  buildProxyConnectorRenderState,
  buildProxyConnectorSpatialIndex,
  collectVisibleNodeAnchors,
  drawVisibleDirectProxyBadges,
  drawVisibleProxyConnectors,
  findHoveredProxyConnector,
  getProxyBezierBadgeGeometry,
  type VisibleNodeAnchor,
} from './proxy'
import { DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA } from '../../crossBranch/settings'
import type { ZUIHiddenProxyBadge, ZUIResolvedConnector } from '../../crossBranch/resolve'
import type { LayoutNode } from './types'

function node(id: string, elementId: number, children: LayoutNode[] = []): LayoutNode {
  return {
    id,
    elementId,
    diagramId: elementId,
    worldX: 0,
    worldY: 0,
    worldW: 100,
    worldH: 100,
    label: id,
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: [],
    ancestorElementIds: [],
    pathElementIds: [elementId],
    children,
    childScale: 1,
    childOffsetX: 0,
    childOffsetY: 0,
    edgesOut: [],
  }
}

describe('collectVisibleNodeAnchors', () => {
  it('keeps fallback anchors for ancestors when only deeper descendants are self drawn', () => {
    const grandchild = node('grandchild', 3)
    const child = node('child', 2, [grandchild])
    const parent = node('parent', 1, [child])

    const anchors = collectVisibleNodeAnchors(
      [{ nodes: [parent] }],
      { x: 0, y: 0, zoom: 5 },
      1000,
    )

    expect(anchors.visibleAnchors.get(3)?.renderAlpha).toBe(1)
    expect(anchors.visibleAnchors.get(2)?.renderAlpha).toBeGreaterThan(0)
    expect(anchors.visibleAnchors.get(1)?.renderAlpha).toBeGreaterThan(0)
  })

  it('keeps fully expanded ancestor fallback anchors eligible for connectors', () => {
    const grandchild = node('grandchild', 3)
    const child = node('child', 2, [grandchild])
    const parent = node('parent', 1, [child])

    const anchors = collectVisibleNodeAnchors(
      [{ nodes: [parent] }],
      { x: 0, y: 0, zoom: 5 },
      1000,
    )

    expect(anchors.visibleAnchors.get(1)?.renderAlpha).toBeGreaterThanOrEqual(DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA)
    expect(anchors.visibleAnchors.get(2)?.renderAlpha).toBeGreaterThanOrEqual(DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA)
    expect(anchors.visibleAnchors.get(1)?.nativeRendered).toBe(true)
    expect(anchors.visibleAnchors.get(2)?.nativeRendered).toBe(true)
  })

  it('keeps ancestor anchors eligible while the body fades into the dashed border', () => {
    const child = node('child', 2)
    const parent = node('parent', 1, [child])

    const anchors = collectVisibleNodeAnchors(
      [{ nodes: [parent] }],
      { x: 0, y: 0, zoom: 3.7 },
      1000,
    )

    expect(anchors.visibleAnchors.get(1)?.renderAlpha).toBeGreaterThanOrEqual(DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA)
  })

  it('drops group member anchors when their group tag is hidden', () => {
    const member = node('member', 1)
    member.tags = ['group:12345678-1234-4234-a234-123456789012']
    const anchors = collectVisibleNodeAnchors(
      [{ nodes: [member] }],
      { x: 0, y: 0, zoom: 5 },
      1000,
      ['group:12345678-1234-4234-a234-123456789012'],
    )

    expect(anchors.visibleAnchors.get(1)).toBeFalsy()
  })
})

function anchor(partial: Partial<VisibleNodeAnchor>): VisibleNodeAnchor {
  return {
    nodeId: partial.nodeId ?? 'node',
    elementId: partial.elementId ?? 1,
    label: partial.label ?? 'node',
    worldX: partial.worldX ?? 0,
    worldY: partial.worldY ?? 0,
    worldW: partial.worldW ?? 100,
    worldH: partial.worldH ?? 100,
    pathDepth: partial.pathDepth ?? 1,
    renderAlpha: partial.renderAlpha ?? 1,
    nativeRendered: partial.nativeRendered ?? true,
  }
}

describe('getProxyBezierBadgeGeometry', () => {
  it('positions badges on the bezier curve instead of the straight connector chord', () => {
    const source = anchor({ worldX: 0, worldY: 0, worldW: 200, worldH: 100 })
    const target = anchor({ elementId: 2, worldX: 300, worldY: 180, worldW: 80, worldH: 100 })

    const geometry = getProxyBezierBadgeGeometry(source, target)

    expect(geometry.midX).not.toBe((200 + 300) / 2)
    expect(Math.hypot(geometry.tangentX, geometry.tangentY)).toBeGreaterThan(0)
  })
})

function proxyConnector(partial: Partial<ZUIResolvedConnector> & Pick<ZUIResolvedConnector, 'key'>): ZUIResolvedConnector {
  return {
    sourceElementId: 1,
    targetElementId: 2,
    sourceAnchorElementId: 1,
    targetAnchorElementId: 2,
    sourceNodeId: 'source',
    targetNodeId: 'target',
    direction: 'outgoing',
    style: 'dashed',
    label: '',
    sourceDepth: 1,
    targetDepth: 1,
    maxDepth: 1,
    details: { count: 1 } as ZUIResolvedConnector['details'],
    ...partial,
  }
}

function proxyDetails(label: string): ZUIResolvedConnector['details'] {
  return {
    key: label,
    label,
    count: 1,
    sourceAnchorId: 'source',
    targetAnchorId: 'target',
    sourceAnchorName: 'Source',
    targetAnchorName: 'Target',
    ownerViewIds: [1],
    ownerViewNames: ['View 1'],
    connectors: [],
  } as ZUIResolvedConnector['details']
}

describe('buildProxyConnectorRenderState', () => {
  it('precomputes provenance stubs without changing drawable connector order', () => {
    const renderState = buildProxyConnectorRenderState([
      proxyConnector({ key: 'deep-provenance-stub', sourceDepth: 3, targetDepth: 3, maxDepth: 3 }),
      proxyConnector({ key: 'shallow-drawable', sourceDepth: 1, targetDepth: 1, maxDepth: 1 }),
      proxyConnector({ key: 'unrelated-drawable', sourceElementId: 3, targetElementId: 4 }),
    ])

    expect(renderState.drawableConnectors.map((connector) => connector.key)).toEqual([
      'shallow-drawable',
      'unrelated-drawable',
    ])
  })

  it('keeps filtered provenance stubs out of hover detection', () => {
    const renderState = buildProxyConnectorRenderState([
      proxyConnector({
        key: 'deep-provenance-stub',
        sourceNodeId: 'deep-source',
        targetNodeId: 'deep-target',
        sourceDepth: 3,
        targetDepth: 3,
        maxDepth: 3,
        details: proxyDetails('Hidden provenance stub'),
      }),
      proxyConnector({
        key: 'shallow-drawable',
        sourceNodeId: 'source',
        targetNodeId: 'target',
        sourceDepth: 1,
        targetDepth: 1,
        maxDepth: 1,
        details: proxyDetails('Visible connector'),
      }),
    ])

    const hoverIndex = buildProxyConnectorSpatialIndex(renderState, new Map([
      ['source', anchor({ nodeId: 'source', elementId: 1, worldX: 0, worldY: 0 })],
      ['target', anchor({ nodeId: 'target', elementId: 2, worldX: 300, worldY: 0 })],
      ['deep-source', anchor({ nodeId: 'deep-source', elementId: 1, worldX: 0, worldY: 200, pathDepth: 3 })],
      ['deep-target', anchor({ nodeId: 'deep-target', elementId: 2, worldX: 300, worldY: 200, pathDepth: 3 })],
    ]))

    expect(findHoveredProxyConnector(200, 250, hoverIndex, { x: 0, y: 0, zoom: 1 })).toBeNull()
    expect(findHoveredProxyConnector(200, 50, hoverIndex, { x: 0, y: 0, zoom: 1 })?.data.label).toBe('Visible connector')
    const hovered = findHoveredProxyConnector(130, 55, hoverIndex, { x: 0, y: 0, zoom: 1 })
    expect(hovered).toMatchObject({ absX: 130, absY: 55, type: 'edge' })
    if (hovered?.type === 'edge') expect(hovered.route).toEqual([{ x: 100, y: 50 }, { x: 300, y: 50 }])
  })
})


describe('off-view connector badge visibility', () => {
  it.each(['proxy', 'direct'] as const)('hides oversized %s badges without reserving label space, and restores them on zoom in', (kind) => {
    const details = proxyDetails('proxy')
    const anchors = new Map([
      ['source', anchor({ nodeId: 'source', elementId: 1, worldW: 180, worldH: 85 })],
      ['target', anchor({ nodeId: 'target', elementId: 2, worldX: 400, worldW: 180, worldH: 85 })],
    ])
    const connector = proxyConnector({ key: 'proxy', details })
    const badge: ZUIHiddenProxyBadge = {
      key: 'badge', sourceAnchorElementId: 1, targetAnchorElementId: 2,
      sourceNodeId: 'source', targetNodeId: 'target', sourceHandle: null, targetHandle: null,
      count: details.count, details,
    }
    for (const zoom of [0.25, 1]) {
      const ctx = {
        save() {}, restore() {}, beginPath() {}, setTransform() {},
        setLineDash() {}, moveTo() {}, lineTo() {}, roundRect() {}, fill() {},
        stroke: vi.fn(), fillText: vi.fn(),
        measureText: (text: string) => ({ width: text.length * 7 }),
        getTransform: () => ({ a: zoom, b: 0, c: 0, d: zoom, e: 0, f: 0 }),
      }
      const occupiedRects: Parameters<typeof drawVisibleProxyConnectors>[7] = []
      if (kind === 'proxy') {
        drawVisibleProxyConnectors(ctx as unknown as CanvasRenderingContext2D,
          buildProxyConnectorRenderState([connector]), anchors, zoom, '#000', '#fff', occupiedRects)
      } else {
        drawVisibleDirectProxyBadges(ctx as unknown as CanvasRenderingContext2D,
          [badge], anchors, zoom, '#000', occupiedRects)
      }
      expect(ctx.fillText).toHaveBeenCalledTimes(zoom === 1 ? 1 : 0)
      expect(occupiedRects).toHaveLength(zoom === 1 ? 1 : 0)
      // Standalone proxy routes remain visible even when their badge is hidden.
      if (kind === 'proxy') expect(ctx.stroke.mock.calls.length).toBeGreaterThanOrEqual(1)
    }
  })
})

describe('off-view connector hover rendering', () => {
  it.each([
    { hoveredKey: undefined, width: 0.5, dash: [0.5, 2] },
    { hoveredKey: 'other', width: 0.5, dash: [0.5, 2] },
    { hoveredKey: 'proxy', width: 1.5, dash: [] },
  ])('uses the original path with hover key $hoveredKey', ({ hoveredKey, width, dash }) => {
    const connector = proxyConnector({ key: 'proxy', details: proxyDetails('proxy') })
    const anchors = new Map([
      ['source', anchor({ nodeId: 'source', elementId: 1, worldX: 0, worldY: 0 })],
      ['target', anchor({ nodeId: 'target', elementId: 2, worldX: 300, worldY: 0 })],
    ])
    let currentDash: number[] = []
    const strokes: Array<{ width: number; dash: number[] }> = []
    const points: number[][] = []
    const ctx = {
      lineWidth: 0,
      save() {}, restore() {}, beginPath() {},
      setLineDash(value: number[]) { currentDash = value },
      moveTo(x: number, y: number) { points.push([x, y]) },
      lineTo(x: number, y: number) { points.push([x, y]) },
      stroke() { strokes.push({ width: this.lineWidth, dash: currentDash }) },
    }
    drawVisibleProxyConnectors(
      ctx as unknown as CanvasRenderingContext2D,
      buildProxyConnectorRenderState([connector]), anchors, 2, '#000', '#123456', [], false, undefined,
      hoveredKey ? {
        type: 'edge', absX: 150, absY: 50,
        data: { sourceId: 'source', targetId: 'target', label: '', diagramId: 1, isProxy: true, details: proxyDetails(hoveredKey) },
      } : null,
    )
    expect(strokes).toEqual([{ width, dash }])
    expect(points).toEqual([[100, 50], [300, 50]])
  })
})
