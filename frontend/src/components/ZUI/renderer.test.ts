import { describe, expect, it } from 'vitest'
import type { ViewLayer } from '../../types'
import { createNodeScreenState, type SceneNode } from './sceneGraph'
import { edgeLabelDrawRectFromCenter, computeNodeNameLayout, getElementGroupBounds, nodeConnectorEndpointAlphaFromState, pickEdgeLabelPosition, setHiddenTags, shouldDrawConnectorDetailLabel } from './renderer'
import { ELEMENT_NAME_INSET_RATIO } from '../../utils/elementName'
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

describe('computeNodeNameLayout', () => {
  const measure = (text: string, fontSize: number) => text.length * fontSize * 0.6
  const base = {
    worldWidth: 180,
    worldHeight: 85,
    drawZoom: 1,
    showLogo: false,
    showType: true,
    childrenAnimating: false,
    measure,
  }

  it('keeps short names on one line at the base size', () => {
    const layout = computeNodeNameLayout({ ...base, label: 'Payments' })
    expect(layout.lines).toEqual(['Payments'])
    expect(layout.fontSize).toBe(20)
    expect(layout.centerY).toBeCloseTo(85 * 0.42)
    expect(layout.truncated).toBe(false)
  })

  it('wraps long names into two balanced lines', () => {
    const layout = computeNodeNameLayout({ ...base, label: 'PaymentProcessingService' })
    expect(layout.lines).toHaveLength(2)
    expect(layout.lines.join('')).toBe('PaymentProcessingService')
    expect(layout.fontSize).toBeLessThan(20)
    expect(layout.truncated).toBe(false)
  })

  it('limits logo nodes to a single line', () => {
    const layout = computeNodeNameLayout({
      ...base,
      label: 'PaymentProcessingService',
      showLogo: true,
    })
    expect(layout.lines).toHaveLength(1)
  })

  it('limits animating child previews to a single line', () => {
    const layout = computeNodeNameLayout({
      ...base,
      label: 'PaymentProcessingService',
      childrenAnimating: true,
    })
    expect(layout.lines).toHaveLength(1)
  })

  it('middle-elides unbreakable names as a last resort', () => {
    const layout = computeNodeNameLayout({
      ...base,
      label: 'abcdefghijklmnopqrstuvwxyz0123456789',
    })
    expect(layout.truncated).toBe(true)
    expect(layout.lines[0]).toContain('\u2026')
    expect(measure(layout.lines[0], layout.fontSize)).toBeLessThanOrEqual(
      base.worldWidth * (1 - ELEMENT_NAME_INSET_RATIO),
    )
  })
})

describe('Explore group backgrounds', () => {
  it('bounds every group member even when some nodes are outside the viewport', () => {
    const marker = 'group:12345678-1234-4234-a234-123456789012'
    const layer: ViewLayer = {
      id: 9,
      diagram_id: 1,
      name: 'Payments',
      tags: [marker],
      color: '#4299E1',
    }
    const first = layoutNode('first', 1)
    first.diagramId = 1
    first.worldX = 100
    first.worldY = 200
    first.tags = [marker]
    const second = layoutNode('second', 2)
    second.diagramId = 1
    second.worldX = 400
    second.worldY = 300
    second.tags = [marker]

    setHiddenTags(new Set())
    expect(getElementGroupBounds([layer], [sceneNode(first, { isVisible: true }), sceneNode(second, { isVisible: false })], 1)).toEqual([
      expect.objectContaining({
        marker,
        x: 76,
        y: 162,
        width: 448,
        height: 258,
      }),
    ])

    setHiddenTags(new Set([marker]))
    expect(getElementGroupBounds([layer], [sceneNode(first, { isVisible: true }), sceneNode(second, { isVisible: false })], 1)).toEqual([])
    setHiddenTags(new Set())
  })
})
