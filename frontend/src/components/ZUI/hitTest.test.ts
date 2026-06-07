import { describe, expect, it } from 'vitest'
import { getExpandThresholds } from './layoutEngine'
import { hitTestZUIRenderedNode } from './hitTest'
import { isZUIPanMouseButton, zoomAround } from './useZUIInteraction'
import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'

const thresholds = getExpandThresholds(1000)

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

  it('uses rendered topmost order for overlapping nodes', () => {
    const bottom = node('bottom', 1, 0, 0)
    const top = node('top', 2, 0, 0)
    const view: ZUIViewState = { x: 0, y: 0, zoom: 1 }

    const hit = hitTestZUIRenderedNode(20, 20, [group([bottom, top])], view, thresholds)

    expect(hit?.node.id).toBe('top')
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
