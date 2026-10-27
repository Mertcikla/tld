import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useZUIInteraction, type ZUIInteraction } from './useZUIInteraction'
import { hitTestZUIRenderedNode } from './hitTest'

vi.mock('./hitTest', () => ({ hitTestZUIRenderedNode: vi.fn(), warmZUIHitTestIndexes: vi.fn() }))

class TestNode extends EventTarget {
  children: TestNode[] = []
  contains(target: TestNode) { return target === this || this.children.some(child => child.contains(target)) }
}

let interaction: ZUIInteraction
let renderer: ReactTestRenderer
let eventWindow: EventTarget
let canvasRef: { current: HTMLCanvasElement }

function Harness() {
  interaction = useZUIInteraction(canvasRef, { x: 0, y: 0, zoom: 1 }, [])
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
    canvasRef = { current: Object.assign(new TestNode(), {
      style: {},
      getBoundingClientRect: () => ({ left: 0, top: 0, width: 1000, height: 800 }),
    }) as unknown as HTMLCanvasElement }
    vi.mocked(hitTestZUIRenderedNode).mockReturnValue({ node: { id: 'service' }, absX: 0, absY: 0, absW: 80, absH: 60 } as ReturnType<typeof hitTestZUIRenderedNode>)
    act(() => { renderer = create(<Harness />) })
  })

  afterEach(() => {
    act(() => renderer.unmount())
    vi.clearAllMocks()
    vi.unstubAllGlobals()
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
})
