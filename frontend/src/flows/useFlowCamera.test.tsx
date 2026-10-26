import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFlowCamera } from './useFlowCamera'

type Options = Parameters<typeof useFlowCamera>[0]
let renderer: ReactTestRenderer
const pending = new Map<number, FrameRequestCallback>()
let nextFrame = 0
const viewport = { x: 123, y: 456, zoom: 0.8 }
const getViewport = vi.fn(() => viewport)
const setViewport = vi.fn()
const fitView = vi.fn()
const base: Options = { viewId: 1, active: false, focus: { viewId: 1, elementIds: [1, 2], connectors: [] }, getViewport, setViewport, fitView }
function Harness(props: Options) { useFlowCamera(props); return null }
function render(options: Partial<Options> = {}) {
  act(() => {
    if (renderer) renderer.update(<Harness {...base} {...options} />)
    else renderer = create(<Harness {...base} {...options} />)
  })
}
function flush() { act(() => { const frames = [...pending.values()]; pending.clear(); frames.forEach(frame => frame(0)) }) }
beforeEach(() => {
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { pending.set(++nextFrame, callback); return nextFrame })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => pending.delete(id))
})
afterEach(() => { act(() => renderer?.unmount()); renderer = undefined as unknown as ReactTestRenderer; pending.clear(); vi.clearAllMocks(); vi.unstubAllGlobals() })

describe('flow camera ownership', () => {
  it('automatically fits selected editing highlights, suspending fitting while recording', () => {
    render({ active: true }); flush()
    expect(fitView).toHaveBeenCalledOnce()
    fitView.mockClear()
    render({ active: true, recording: true, focus: { viewId: 1, elementIds: [3], connectors: [] } }); flush()
    render({ active: true, recording: true, focus: base.focus }); flush()
    expect(fitView).not.toHaveBeenCalled()
    expect(setViewport).not.toHaveBeenCalled()
    render({ active: true, recording: false }); flush()
    expect(fitView).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ nodes: [{ id: '1' }, { id: '2' }] }))
  })

  it('fits steps and restores the original camera when the flow panel closes', () => {
    render({ active: true }); flush()
    expect(fitView).toHaveBeenCalledOnce()
    render({ active: true, focus: { ...base.focus!, connectors: [{ id: 10, reverse: true }] } }); flush()
    expect(fitView).toHaveBeenCalledOnce()
    render({ active: true, focus: { viewId: 1, elementIds: [3], connectors: [] } }); flush()
    expect(fitView).toHaveBeenCalledTimes(2)
    render({ active: false })
    // A child clearing focus before the resize frame must not lose the restore.
    render({ active: false, focus: null }); flush()
    expect(setViewport).toHaveBeenCalledExactlyOnceWith(viewport)
  })

  it('does not restore a camera belonging to a different diagram', () => {
    render({ active: true }); flush()
    render({ viewId: 2, active: false, focus: null }); flush()
    expect(setViewport).not.toHaveBeenCalled()
  })
  it('preserves the world center and zoom when the editor takes canvas space', () => {
    render({ open: false, canvasSize: { width: 1200, height: 800 } })
    render({ open: true, canvasSize: { width: 840, height: 800 } })
    expect(setViewport).toHaveBeenLastCalledWith({ x: -57, y: 456, zoom: 0.8 })
    expect(fitView).not.toHaveBeenCalled()
  })

  it('keeps the same world center when recording shrinks the compact panel', () => {
    render({ active: true, recording: true, canvasSize: { width: 390, height: 200 } }); flush()
    render({ active: true, recording: true, canvasSize: { width: 390, height: 700 } }); flush()
    expect(setViewport).toHaveBeenLastCalledWith({ x: 123, y: 706, zoom: 0.8 })
    expect(fitView).not.toHaveBeenCalled()
    render({ active: true, recording: false, canvasSize: { width: 390, height: 200 } }); flush()
    expect(fitView).toHaveBeenCalledOnce()
  })

  it('restores the editing world center after the compact panel changes canvas height', () => {
    render({ active: true, canvasSize: { width: 390, height: 500 } }); flush()
    render({ active: false, open: true, canvasSize: { width: 390, height: 200 } }); flush()
    expect(setViewport).toHaveBeenLastCalledWith({ x: 123, y: 306, zoom: 0.8 })
  })

})
