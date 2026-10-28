import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { placeFloatingFlowPanel, useFloatingFlowPanel } from './useFloatingFlowPanel'

const bounds = { left: 8, top: 68, right: 992, bottom: 642 }

describe('floating flow panel placement', () => {
  it('keeps the full panel inside the usable canvas, including extreme drags', () => {
    expect(placeFloatingFlowPanel({ x: -1000, y: -1000 }, { width: 400, height: 300 }, bounds, [])).toEqual({ x: 8, y: 68, maxHeight: 574 })
    expect(placeFloatingFlowPanel({ x: 5000, y: 5000 }, { width: 400, height: 300 }, bounds, [])).toEqual({ x: 592, y: 342, maxHeight: 574 })
  })

  it('avoids a popover and its gap while preserving full height', () => {
    const obstacle = { left: 450, top: 200, right: 650, bottom: 600 }
    expect(placeFloatingFlowPanel({ x: 400, y: 200 }, { width: 400, height: 300 }, bounds, [obstacle])).toEqual({ x: 42, y: 200, maxHeight: 574 })
  })

  it('shortens the panel when a narrow canvas cannot fit it beside an overlay', () => {
    const narrow = { left: 8, top: 60, right: 352, bottom: 700 }
    const obstacle = { left: 0, top: 400, right: 360, bottom: 600 }
    expect(placeFloatingFlowPanel({ x: 8, y: 350 }, { width: 344, height: 500 }, narrow, [obstacle])).toEqual({ x: 8, y: 60, maxHeight: 332 })
  })

  it('accounts for several overlays and keeps an already valid position', () => {
    const obstacles = [{ left: 0, top: 150, right: 220, bottom: 500 }, { left: 750, top: 100, right: 1000, bottom: 600 }]
    expect(placeFloatingFlowPanel({ x: 250, y: 200 }, { width: 400, height: 300 }, bounds, obstacles)).toEqual({ x: 250, y: 200, maxHeight: 574 })
  })
})

describe('floating flow panel interaction', () => {
  let renderer: ReactTestRenderer
  let floating: ReturnType<typeof useFloatingFlowPanel>
  let resize: () => void
  let mutation: (records: unknown[]) => void
  let frames: Array<() => void>
  let overlays: HTMLElement[]
  const rect = (left: number, top: number, width: number, height: number) => ({ left, top, width, height, right: left + width, bottom: top + height })

  function mount(enabled = true) {
    frames = []
    overlays = [
      { dataset: { canvasOverlay: 'top' }, getBoundingClientRect: () => rect(400, 66, 400, 44) },
      { dataset: { canvasOverlay: 'bottom' }, getBoundingClientRect: () => rect(400, 700, 400, 36) },
    ] as unknown as HTMLElement[]
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() })
    vi.stubGlobal('document', { body: {}, querySelectorAll: () => overlays })
    vi.stubGlobal('getComputedStyle', () => ({ display: 'block', visibility: 'visible', opacity: '1' }))
    vi.stubGlobal('requestAnimationFrame', (callback: () => void) => frames.push(callback))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    vi.stubGlobal('ResizeObserver', class { constructor(callback: () => void) { resize = callback } observe() {} unobserve() {} disconnect() {} })
    vi.stubGlobal('MutationObserver', class { constructor(callback: typeof mutation) { mutation = callback } observe() {} disconnect() {} })
    const canvas = { getBoundingClientRect: () => rect(100, 50, 1000, 700) }
    const panel = {
      parentElement: canvas,
      contains: () => false,
      getBoundingClientRect: () => rect(100 + Number(floating?.style?.left ?? 64), 50 + Number(floating?.style?.top ?? 64), 400, 300),
    }
    function Harness() {
      floating = useFloatingFlowPanel(enabled)
      return <div ref={floating.ref} />
    }
    act(() => { renderer = create(<Harness />, { createNodeMock: () => panel }) })
  }

  afterEach(() => { act(() => renderer.unmount()); vi.unstubAllGlobals() })

  it('supports pointer capture and keyboard movement without crossing the toolbars', () => {
    mount()
    expect(floating.style).toMatchObject({ left: 64, top: 68, maxHeight: 574 })
    const handle = { focus: vi.fn(), setPointerCapture: vi.fn(), hasPointerCapture: () => true, releasePointerCapture: vi.fn() }
    const event = { pointerId: 1, button: 0, clientX: 100, clientY: 100, currentTarget: handle, preventDefault: vi.fn(), stopPropagation: vi.fn() }
    act(() => floating.handleProps.onPointerDown(event as never))
    expect(floating.dragging).toBe(true)
    act(() => floating.handleProps.onPointerMove({ ...event, clientX: 5000, clientY: 5000 } as never))
    expect(floating.style).toMatchObject({ left: 592, top: 342 })
    act(() => floating.handleProps.onPointerCancel(event as never))
    expect(handle.releasePointerCapture).toHaveBeenCalledWith(1)
    expect(floating.dragging).toBe(false)
    act(() => floating.handleProps.onPointerMove(event as never))
    expect(floating.style).toMatchObject({ left: 592, top: 342 })
    act(() => floating.handleProps.onKeyDown({ key: 'ArrowLeft', preventDefault: vi.fn(), stopPropagation: vi.fn() } as never))
    expect(floating.style).toMatchObject({ left: 576, top: 342 })
  })

  it('reconstrains the panel when overlays resize or appear', () => {
    mount()
    overlays[0].getBoundingClientRect = () => rect(400, 66, 400, 144) as DOMRect
    act(() => { resize(); frames.shift()!() })
    expect(floating.style?.top).toBe(168)
    overlays.push({ dataset: { canvasOverlay: '' }, getBoundingClientRect: () => rect(100, 210, 300, 400) } as unknown as HTMLElement)
    act(() => { mutation([{ type: 'childList' }]); frames.shift()!() })
    expect(floating.style?.left).toBe(308)
  })

  it('leaves docked and closed panels under their existing layout', () => {
    mount(false)
    expect(floating.style).toBeUndefined()
    expect(window.addEventListener).not.toHaveBeenCalled()
  })
})
