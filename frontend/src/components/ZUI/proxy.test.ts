import { describe, expect, it } from 'vitest'
import { collectVisibleNodeAnchors, getProxyBezierBadgeGeometry, type VisibleNodeAnchor } from './proxy'
import { DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA } from '../../crossBranch/settings'
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
