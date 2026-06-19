import { describe, expect, it } from 'vitest'
import {
  buildProxyConnectorRenderState,
  buildProxyConnectorSpatialIndex,
  collectVisibleNodeAnchors,
  findHoveredProxyConnector,
  getProxyBezierBadgeGeometry,
  type VisibleNodeAnchor,
} from './proxy'
import { DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA } from '../../crossBranch/settings'
import type { ZUIResolvedConnector } from '../../crossBranch/resolve'
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
  })
})
