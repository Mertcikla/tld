import { describe, expect, it, vi } from 'vitest'
import type { ViewLayer } from '../../types'
import { createNodeScreenState, type SceneNode } from './sceneGraph'
import { drawNodeTags, type RenderContext, drawArrowHead, connectorDetailScreenWidth, connectorLabelFitsEndpoints, edgeLabelDrawRectFromCenter, computeNodeNameLayout, edgeLayerStyle, getElementGroupBounds, nodeConnectorEndpointAlphaFromState, pickEdgeLabelPosition, setHiddenTags, shouldDrawConnectorDetailLabel } from './renderer'
import { ELEMENT_NAME_INSET_RATIO } from '../../utils/elementName'
import type { LayoutNode } from './types'
import { connectorTagHighlightColor, setHighlightColor, setHighlightedTags, setTagColors } from './renderer'

describe('connector tag highlight color', () => {
  it('uses the hovered layer color or tag color and clears after hover', () => {
    setHighlightedTags(new Set(['critical']))
    setTagColors({ critical: { color: '#f00' } })
    expect(connectorTagHighlightColor(['critical'], '#aaa')).toBe('#f00')
    expect(connectorTagHighlightColor(['other'], '#aaa')).toBeUndefined()
    setHighlightColor('#0f0')
    expect(connectorTagHighlightColor(['critical'], '#aaa')).toBe('#0f0')
    setHighlightedTags(new Set())
    expect(connectorTagHighlightColor(['critical'], '#aaa')).toBeUndefined()
    setHighlightColor('')
    setTagColors({})
  })
})

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

describe('connector label size relative to endpoints', () => {
  it('hides labels that dominate either endpoint and restores them as elements grow', () => {
    const badge = { width: 30, height: 24 }
    const small = { width: 60, height: 28 }
    const detailed = { width: 180, height: 85 }
    expect(connectorLabelFitsEndpoints(badge, small, small)).toBe(false)
    expect(connectorLabelFitsEndpoints(badge, detailed, small)).toBe(false)
    expect(connectorLabelFitsEndpoints(badge, small, detailed)).toBe(false)
    expect(connectorLabelFitsEndpoints(badge, detailed, detailed)).toBe(true)
  })

  it('also hides long labels and large counts that overwhelm element widths', () => {
    const endpoint = { width: 180, height: 85 }
    expect(connectorLabelFitsEndpoints({ width: 136, height: 24 }, endpoint, endpoint)).toBe(false)
    expect(connectorLabelFitsEndpoints({ width: 100, height: 24 }, endpoint, endpoint)).toBe(true)
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

  it.each([2, 8, 40])('caps names at 24 screen pixels at zoom %s', drawZoom => {
    const layout = computeNodeNameLayout({ ...base, label: 'Payments', worldHeight: 360, worldWidth: 520, drawZoom })
    expect(layout.fontSize * drawZoom).toBeLessThanOrEqual(24)
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

describe('edgeLayerStyle', () => {
  const overlay = (change: 'added' | 'removed' | 'modified' | 'unchanged') => ({ change, path: `${change}.go`, symbols: [], reason: 'direct' as const })
  const accent = '#63b3ed'
  const borderGrey = 'rgba(160,174,192,0.5)'

  it('keeps the accent when both endpoints are direct impact', () => {
    const source = layoutNode('source', 1)
    const target = layoutNode('target', 2)
    source.changeOverlay = overlay('modified')
    target.changeOverlay = overlay('added')
    expect(edgeLayerStyle(source, target, accent)).toEqual({ color: accent, alphaFactor: 1 })
  })

  it('fades to 50% when only one endpoint is direct impact', () => {
    const source = layoutNode('source', 1)
    const target = layoutNode('target', 2)
    source.changeOverlay = overlay('modified')
    target.changeOverlay = overlay('unchanged')
    expect(edgeLayerStyle(source, target, accent)).toEqual({ color: accent, alphaFactor: 0.5 })
  })

  it('uses the node border grey when neither endpoint is direct impact', () => {
    const source = layoutNode('source', 1)
    const target = layoutNode('target', 2)
    source.changeOverlay = overlay('unchanged')
    target.changeOverlay = overlay('unchanged')
    expect(edgeLayerStyle(source, target, accent)).toEqual({ color: borderGrey, alphaFactor: 1 })
  })

  it('treats a container with a direct change in its subtree as impacted', () => {
    const container = layoutNode('container', 1, [layoutNode('child', 2)])
    container.children[0].changeOverlay = overlay('modified')
    const context = layoutNode('context', 3)
    context.changeOverlay = overlay('unchanged')
    expect(edgeLayerStyle(container, context, accent)).toEqual({ color: accent, alphaFactor: 0.5 })
  })
})

describe('code card connector detail visibility', () => {
  it.each(['code', 'class', 'service'])('uses ordinary-card scale for a large %s source preview', type => {
    const card = { worldW: 2400, type, subKind: 'code' as const }
    // Both cards are at 50% zoom, even though the preview occupies 1,200 pixels.
    const equivalentWidth = connectorDetailScreenWidth(card, 1200)
    expect(equivalentWidth).toBe(90)
    expect(shouldDrawConnectorDetailLabel('forward', 500, equivalentWidth)).toBe(false)
    expect(shouldDrawConnectorDetailLabel('backward', equivalentWidth, 500)).toBe(false)
  })

  it('normalizes explicit class cards without a source preview marker', () => {
    expect(connectorDetailScreenWidth({ worldW: 520, type: 'class' }, 260)).toBe(90)
  })

  it('accounts for the visible card cap before evaluating detail visibility', () => {
    const card = { worldW: 520, type: 'code' }
    // At zoom 2, a 520-world-unit card has been capped to 600 screen pixels.
    expect(connectorDetailScreenWidth(card, 600)).toBeCloseTo(180 * (600 / 1040) * 2)
  })

  it('shows details at the same effective zoom as an ordinary card', () => {
    const previewWidth = connectorDetailScreenWidth({ worldW: 520, type: 'code' }, 520)
    const ordinaryWidth = connectorDetailScreenWidth({ worldW: 180, type: 'service' }, 180)
    expect(previewWidth).toBe(ordinaryWidth)
    expect(shouldDrawConnectorDetailLabel('forward', 0, previewWidth)).toBe(true)
  })
})

describe('Explore arrowhead shape', () => {
  it('draws the editor closed-arrow proportions with rounded, matching-color joins', () => {
    const context = { save: vi.fn(), restore: vi.fn(), translate: vi.fn(), rotate: vi.fn(), beginPath: vi.fn(),
      moveTo: vi.fn(), lineTo: vi.fn(), closePath: vi.fn(), fill: vi.fn(), stroke: vi.fn(),
      fillStyle: '', strokeStyle: '', lineWidth: 0, lineJoin: '', lineCap: '' }
    drawArrowHead(context as unknown as CanvasRenderingContext2D, 100, 200, Math.PI / 2, 10, '#facc15')
    expect(context.translate).toHaveBeenCalledWith(100, 200)
    expect(context.rotate).toHaveBeenCalledWith(Math.PI / 2)
    expect(context.moveTo).toHaveBeenCalledWith(-7.5, -6)
    expect(context.lineTo.mock.calls).toEqual([[0, 0], [-7.5, 6]])
    expect(context.lineJoin).toBe('round')
    expect(context.lineCap).toBe('round')
    expect(context.lineWidth).toBe(1.5)
    expect(context.fillStyle).toBe('#facc15')
    expect(context.strokeStyle).toBe('#facc15')
    expect(context.fill).toHaveBeenCalledOnce()
    expect(context.stroke).toHaveBeenCalledOnce()
    expect(context.restore).toHaveBeenCalledOnce()
  })
})

describe('Explore tag alignment with the editor', () => {
  const renderContext: RenderContext = {
    canvasBg: '#000', nodeBg: '#222', accent: '#facc15', labelBg: '#111', canvasW: 1200, canvasH: 800,
    thresholds: { start: 300, end: 600 }, lowDetail: false, groupLayersByDiagram: new Map(),
  }
  const context = () => ({ save() {}, restore() {}, beginPath() {}, fill() {}, stroke() {},
    measureText: (value: string) => ({ width: value.length * 6 }), arc: vi.fn(), roundRect: vi.fn(), fillText: vi.fn() })

  it.each([180, 520, 2400])('keeps tag dots at the editor offsets on a %s-wide card', width => {
    const ctx = context()
    drawNodeTags(ctx as unknown as CanvasRenderingContext2D, { ...layoutNode('tags', 1), tags: ['a', 'b'] }, renderContext,
      { x: 0, y: 0, w: width, h: 84, drawZoom: 1, drawScreenW: width, parentAlpha: 1, isHovered: false, accent: '#facc15' })
    expect(ctx.arc.mock.calls[0].slice(0, 3)).toEqual([13, 73, 3])
    expect(ctx.arc.mock.calls[1].slice(0, 3)).toEqual([23, 73, 3])
  })

  it.each([180, 520, 2400])('anchors expanded tags at the editor popup offset on a %s-wide card', width => {
    const ctx = context()
    drawNodeTags(ctx as unknown as CanvasRenderingContext2D, { ...layoutNode('tags', 1), tags: ['a'] }, renderContext,
      { x: 0, y: 0, w: width, h: 84, drawZoom: 1, drawScreenW: width, parentAlpha: 1, isHovered: true, accent: '#facc15' })
    const [x, y, , height] = ctx.roundRect.mock.calls[0]
    expect(x).toBe(6)
    expect(y + height).toBe(80)
  })
})

describe('code preview tags across zoom stages', () => {
  const renderContext: RenderContext = {
    canvasBg: '#000', nodeBg: '#222', accent: '#facc15', labelBg: '#111', canvasW: 1200, canvasH: 800,
    thresholds: { start: 300, end: 600 }, lowDetail: false, groupLayersByDiagram: new Map(),
  }
  const context = () => ({ save() {}, restore() {}, beginPath() {}, fill() {}, stroke() {},
    measureText: (value: string) => ({ width: value.length * 6 }), arc: vi.fn(), roundRect: vi.fn(), fillText: vi.fn() })
  const card = { ...layoutNode('code-tags', 1), type: 'code', worldW: 520, worldH: 360, tags: ['a'] }
  const bounds = { x: 0, y: 0, w: 520, h: 84, drawZoom: 1, drawScreenW: 520, parentAlpha: 1, isHovered: false, accent: '#facc15' }

  it('places summary tag dots at the bottom left of the body', () => {
    const ctx = context()
    drawNodeTags(ctx as unknown as CanvasRenderingContext2D, { ...card, subDetail: 'summary' }, renderContext, bounds)
    expect(ctx.arc.mock.calls[0].slice(0, 3)).toEqual([13, 349, 3])
  })

  it('anchors expanded summary tags at the bottom left of the body', () => {
    const ctx = context()
    drawNodeTags(ctx as unknown as CanvasRenderingContext2D, { ...card, subDetail: 'summary' }, renderContext, { ...bounds, isHovered: true })
    const [x, y, , height] = ctx.roundRect.mock.calls[0]
    expect(x).toBe(6)
    expect(y + height).toBe(356)
  })

  it.each(['overview', 'structure'] as const)('hides tags in %s even on hover', subDetail => {
    const ctx = context()
    drawNodeTags(ctx as unknown as CanvasRenderingContext2D, { ...card, subDetail }, renderContext, { ...bounds, isHovered: true })
    expect(ctx.arc).not.toHaveBeenCalled()
    expect(ctx.roundRect).not.toHaveBeenCalled()
    expect(ctx.fillText).not.toHaveBeenCalled()
  })

  it.each(['outline', 'code'] as const)('keeps header tags in %s', subDetail => {
    const ctx = context()
    drawNodeTags(ctx as unknown as CanvasRenderingContext2D, { ...card, subDetail }, renderContext, bounds)
    expect(ctx.arc.mock.calls[0].slice(0, 3)).toEqual([13, 73, 3])
  })

  it.each([0.1, 0.25, 0.4])('uses the actual zoom stage for tag placement at zoom %s', drawZoom => {
    const ctx = context()
    drawNodeTags(ctx as unknown as CanvasRenderingContext2D, card, renderContext, { ...bounds, drawZoom, drawScreenW: card.worldW * drawZoom })
    if (drawZoom < 0.4) expect(ctx.arc).not.toHaveBeenCalled()
    else expect(ctx.arc.mock.calls[0].slice(0, 3)).toEqual([13, 349, 3])
  })
})
