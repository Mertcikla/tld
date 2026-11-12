import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useZUIInteraction, type ZUIInteraction } from './useZUIInteraction'
import * as edgeHover from './edgeHover'
import { PREVIEW_HOVER_DELAY } from '../../hooks/usePreviewInteraction'
import { scrollSubElement, subNodeSymbols } from './subElements'
import type { LayoutNode, DiagramGroupLayout } from './types'
import { hitTestZUIRenderedNode } from './hitTest'

vi.mock('./hitTest', () => ({ hitTestZUIRenderedNode: vi.fn(), warmZUIHitTestIndexes: vi.fn() }))

class TestNode extends EventTarget {
  children: TestNode[] = []
  closest() { return null }
  contains(target: TestNode) { return target === this || this.children.some(child => child.contains(target)) }
}

let interaction: ZUIInteraction
let renderer: ReactTestRenderer
let eventWindow: EventTarget
let canvasRef: { current: HTMLCanvasElement }

const emptyGroups: DiagramGroupLayout[] = []
function Harness() {
  interaction = useZUIInteraction(canvasRef, { x: 0, y: 0, zoom: 1 }, emptyGroups)
  return null
}

function move(target: EventTarget, clientX = 20, clientY = 20) {
  const event = new Event('mousemove')
  Object.defineProperties(event, { target: { value: target }, clientX: { value: clientX }, clientY: { value: clientY } })
  act(() => { eventWindow.dispatchEvent(event) })
}

describe('ZUI hover over overlays', () => {
  beforeEach(() => {
    eventWindow = new EventTarget()
    vi.stubGlobal('window', eventWindow)
    vi.stubGlobal('Node', TestNode)
    vi.stubGlobal('Element', TestNode)
    canvasRef = { current: Object.assign(new TestNode(), {
      style: {},
      getBoundingClientRect: () => ({ left: 0, top: 0, right: 1000, bottom: 800, width: 1000, height: 800 }),
    }) as unknown as HTMLCanvasElement }
    vi.mocked(hitTestZUIRenderedNode).mockReturnValue({ node: { id: 'service' }, absX: 0, absY: 0, absW: 80, absH: 60 } as ReturnType<typeof hitTestZUIRenderedNode>)
    act(() => { renderer = create(<Harness />) })
  })

  afterEach(() => {
    act(() => renderer.unmount())
    vi.clearAllMocks()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('rebuilds connector hit testing lazily after source scrolling', () => {
    const build = vi.spyOn(edgeHover, 'buildEdgeSpatialIndex')
    vi.mocked(hitTestZUIRenderedNode).mockReturnValue(null)
    const code = {
      id: 'lazy-scroll-index', elementId: 991, diagramId: 1, type: 'code', subKind: 'code', subDetail: 'code',
      worldW: 520, worldH: 160, edgesOut: [], children: [],
      subSource: { status: 'ready', startLine: 1, endLine: 100, lines: Array<string>(100).fill('source line'), symbols: [], tokens: [] },
    } as unknown as LayoutNode
    try {
      move(canvasRef.current)
      expect(build).toHaveBeenCalledOnce()
      move(canvasRef.current)
      expect(build).toHaveBeenCalledOnce()
      act(() => { expect(scrollSubElement(code, 24, 1)).toBe(true) })
      // Scrolling schedules paint only; it must not rebuild every connector.
      expect(build).toHaveBeenCalledOnce()
      move(canvasRef.current)
      expect(build).toHaveBeenCalledTimes(2)
    } finally { build.mockRestore() }
  })

  it('does not hit-test canvas nodes underneath a panel or its controls', () => {
    const panel = new TestNode(), control = new TestNode()
    panel.children.push(control)
    move(panel)
    move(control)
    expect(hitTestZUIRenderedNode).not.toHaveBeenCalled()
    expect(interaction.hoveredItem).toBeNull()

    move(canvasRef.current)
    expect(interaction.hoveredItem?.type).toBe('node')
    expect(hitTestZUIRenderedNode).toHaveBeenCalledOnce()
  })

  it('immediately clears an existing hover when entering another panel, even in the safe corridor', () => {
    const popover = Object.assign(new TestNode(), {
      getBoundingClientRect: () => ({ left: 90, top: 90, right: 200, bottom: 200 }),
    })
    interaction.setHoverPopoverElement(popover as unknown as HTMLElement)
    move(canvasRef.current)
    act(() => interaction.setHoverLocked(true))
    move(new TestNode(), 100, 100)
    expect(interaction.hoveredItem).toBeNull()
    expect(hitTestZUIRenderedNode).toHaveBeenCalledOnce()
  })

  it('preserves the canvas hover while interacting with its own popover controls', () => {
    const popover = new TestNode(), control = new TestNode()
    popover.children.push(control)
    interaction.setHoverPopoverElement(popover as unknown as HTMLElement)
    move(canvasRef.current)
    const hovered = interaction.hoveredItem
    move(control, 100, 100)
    expect(interaction.hoveredItem).toBe(hovered)
    expect(hitTestZUIRenderedNode).toHaveBeenCalledOnce()
  })

  function source(id: string, detail: NonNullable<LayoutNode['subDetail']> = 'code') {
    const node = {
      id, elementId: 991, diagramId: 1, type: 'file', subKind: 'code', subDetail: detail,
      worldW: 520, worldH: 360, edgesOut: [], children: [],
      subSource: { status: 'ready', startLine: 1, endLine: 100, lines: Array<string>(100).fill('a very long source line '.repeat(20)),
        symbols: [{ name: 'first', startLine: 1, endLine: 1 }, { name: 'later', startLine: 30, endLine: 30 }], tokens: [] },
    } as unknown as LayoutNode
    vi.mocked(hitTestZUIRenderedNode).mockReturnValue({ node, absX: 0, absY: 0, absW: 520, absH: 360, visualScale: 1, cumulativeScale: 1 })
    return node
  }
  function input(target: EventTarget, type: string, fields: Record<string, unknown>) {
    const event = new Event(type, { cancelable: true })
    Object.defineProperties(event, Object.fromEntries(Object.entries({ target: canvasRef.current, ...fields }).map(([key, value]) => [key, { value }])))
    act(() => { target.dispatchEvent(event) })
    return event
  }
  function activate() {
    move(canvasRef.current, 100, 120)
    expect(interaction.previewInteraction.phase).toBe('waiting')
    act(() => vi.advanceTimersByTime(PREVIEW_HOVER_DELAY - 1))
    expect(interaction.previewInteraction.phase).toBe('waiting')
    act(() => vi.advanceTimersByTime(1))
    expect(interaction.previewInteraction.phase).toBe('ready')
  }

  it('uses the editor hover delay, then scrolls source contents without moving the canvas', () => {
    vi.useFakeTimers()
    const node = source('hover-wheel-source')
    const before = subNodeSymbols(node, 1).areas[1].y
    const camera = interaction.viewStateRef.current
    activate()
    const wheel = input(eventWindow, 'wheel', { clientX: 100, clientY: 120, deltaX: 30, deltaY: 20, deltaMode: 0, ctrlKey: false })
    expect(wheel.defaultPrevented).toBe(true)
    expect(subNodeSymbols(node, 1).areas[1].y).toBeCloseTo(before - 20)
    expect(interaction.viewStateRef.current).toBe(camera)
    move(new TestNode())
    expect(interaction.previewInteraction.phase).toBe('idle')
  })

  it('pans an activated source with middle or right drag and resets on camera changes', () => {
    vi.useFakeTimers()
    const node = source('hover-pan-source')
    activate()
    const camera = interaction.viewStateRef.current
    const before = subNodeSymbols(node, 1).areas[1].y
    for (const button of [1, 2]) {
      input(canvasRef.current, 'mousedown', { button, clientX: 100, clientY: 120 })
      move(canvasRef.current, 80, 100)
      input(eventWindow, 'mouseup', { button, clientX: 80, clientY: 100 })
    }
    expect(subNodeSymbols(node, 1).areas[1].y).toBeCloseTo(before - 40)
    expect(interaction.viewStateRef.current).toBe(camera)
    act(() => interaction.setViewState({ ...camera, x: -10 }))
    expect(interaction.previewInteraction.phase).toBe('idle')
  })

  it('reveals hidden symbols through both banners after activation', () => {
    vi.useFakeTimers()
    const node = source('hover-banner-source')
    activate()
    const camera = interaction.viewStateRef.current
    expect(subNodeSymbols(node, 1).below.areas.map(area => area.symbol.name)).toContain('later')
    input(canvasRef.current, 'mousedown', { button: 0, clientX: 100, clientY: 359 })
    input(eventWindow, 'mouseup', { button: 0, clientX: 100, clientY: 359 })
    expect(subNodeSymbols(node, 1).areas[1].visible).toBe(true)
    expect(subNodeSymbols(node, 1).above.areas.map(area => area.symbol.name)).toContain('first')
    input(canvasRef.current, 'mousedown', { button: 0, clientX: 100, clientY: 85 })
    input(eventWindow, 'mouseup', { button: 0, clientX: 100, clientY: 85 })
    expect(subNodeSymbols(node, 1).areas[0].visible).toBe(true)
    expect(interaction.viewStateRef.current).toBe(camera)
  })

  it('does not activate card layouts or headers and restarts the wait on another source', () => {
    vi.useFakeTimers()
    source('hover-card-source', 'overview')
    move(canvasRef.current, 100, 120)
    act(() => vi.advanceTimersByTime(PREVIEW_HOVER_DELAY))
    expect(interaction.previewInteraction.phase).toBe('idle')
    source('hover-header-source')
    move(canvasRef.current, 100, 20)
    act(() => vi.advanceTimersByTime(PREVIEW_HOVER_DELAY))
    expect(interaction.previewInteraction.phase).toBe('idle')
    activate()
    source('hover-other-source')
    move(canvasRef.current, 100, 120)
    expect(interaction.previewInteraction.phase).toBe('waiting')
  })


  it.each(['structure', 'summary', 'outline'] as const)('activates the %s layout with the same hover delay', detail => {
    vi.useFakeTimers()
    source(`hover-${detail}-source`, detail)
    activate()
  })


  it('keeps wheel input on the canvas until the source hover delay finishes', () => {
    vi.useFakeTimers()
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    Object.assign(eventWindow, { location: { href: '' } })
    const node = source('hover-waiting-wheel')
    const before = subNodeSymbols(node, 1).areas[1].y
    move(canvasRef.current, 100, 120)
    expect(interaction.previewInteraction.phase).toBe('waiting')
    const wheel = input(eventWindow, 'wheel', { clientX: 100, clientY: 120, deltaX: 0, deltaY: 3, deltaMode: 0, ctrlKey: false })
    expect(wheel.defaultPrevented).toBe(true)
    expect(subNodeSymbols(node, 1).areas[1].y).toBe(before)
    expect(interaction.viewStateRef.current.y).toBe(-3)
    expect(interaction.previewInteraction.phase).toBe('idle')
  })

})
