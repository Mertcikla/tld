import { describe, expect, it } from 'vitest'
import { createNodeScreenState, type SceneNode } from './sceneGraph'
import { edgeLabelDrawRectFromCenter, nodeConnectorEndpointAlphaFromState, pickEdgeLabelPosition, shouldDrawConnectorDetailLabel } from './renderer'
import type { LayoutNode } from './types'

function layoutNode(id: string, elementId: number, children: LayoutNode[] = []): LayoutNode {
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

function sceneNode(layout: LayoutNode, partialState: Partial<SceneNode['state']> = {}): SceneNode {
  return {
    layout,
    children: [],
    state: {
      ...createNodeScreenState(),
      ...partialState,
    },
  }
}

describe('nodeConnectorEndpointAlphaFromState', () => {
  it('keeps expanded parent boundary anchors usable for native connectors', () => {
    const node = sceneNode(layoutNode('parent', 1, [layoutNode('child', 2)]), {
      inheritedAlpha: 1,
      parentAlpha: 0,
      t: 1,
    })

    expect(nodeConnectorEndpointAlphaFromState(node)).toBeGreaterThan(0)
  })
})

describe('shouldDrawConnectorDetailLabel', () => {
  it('follows the visible arrowhead endpoint for forward connectors', () => {
    expect(shouldDrawConnectorDetailLabel('forward', 240, 120)).toBe(false)
    expect(shouldDrawConnectorDetailLabel('forward', 80, 121)).toBe(true)
  })

  it('follows the visible arrowhead endpoint for backward connectors', () => {
    expect(shouldDrawConnectorDetailLabel('backward', 120, 240)).toBe(false)
    expect(shouldDrawConnectorDetailLabel('backward', 121, 80)).toBe(true)
  })

  it('allows bidirectional and nondirectional labels once either endpoint has connector detail', () => {
    expect(shouldDrawConnectorDetailLabel('both', 80, 121)).toBe(true)
    expect(shouldDrawConnectorDetailLabel('bidirectional', 121, 80)).toBe(true)
    expect(shouldDrawConnectorDetailLabel('none', 80, 120)).toBe(false)
    expect(shouldDrawConnectorDetailLabel('none', 121, 80)).toBe(true)
  })
})

describe('edge label positioning', () => {
  it('draws the label box centered on the picked connector point', () => {
    const matrix = { a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 } as DOMMatrix
    const labelCenter = pickEdgeLabelPosition(matrix, 100, 50, 40, 20, 120, 0, [])
    const labelRect = edgeLabelDrawRectFromCenter(labelCenter, 40, 20)

    expect(labelCenter).toEqual({ x: 100, y: 50 })
    expect(labelRect).toEqual({ x: 80, y: 40, width: 40, height: 20 })
  })
})
