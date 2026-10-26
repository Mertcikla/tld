import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFlowCamera } from './useFlowCamera'

type Options = Parameters<typeof useFlowCamera>[0]
let renderer: ReactTestRenderer
const pending = new Map<number, FrameRequestCallback>()
let nextFrame = 0
const fitView = vi.fn()
const base: Options = { viewId: 1, active: false, focus: { viewId: 1, elementIds: [1, 2], connectors: [] }, fitView }
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
    render({ active: true, recording: false }); flush()
    expect(fitView).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ nodes: [{ id: '1' }, { id: '2' }] }))
  })

  it('does not move the camera when closing or reopening unchanged highlights', () => {
    render({ active: true }); flush()
    expect(fitView).toHaveBeenCalledOnce()
    render({ active: false, focus: null }); flush()
    render({ active: true }); flush()
    expect(fitView).toHaveBeenCalledOnce()
    render({ active: true, focus: { ...base.focus!, connectors: [{ id: 10, reverse: true }] } }); flush()
    expect(fitView).toHaveBeenCalledOnce()
    render({ active: true, focus: { viewId: 1, elementIds: [3], connectors: [] } }); flush()
    expect(fitView).toHaveBeenCalledTimes(2)
  })

  it('ignores highlights from another diagram and fits the new diagram when selected', () => {
    render({ active: true }); flush()
    render({ viewId: 2, active: false, focus: null }); flush()
    render({ viewId: 2, active: true }); flush()
    expect(fitView).toHaveBeenCalledOnce()
    render({ viewId: 2, active: true, focus: { viewId: 2, elementIds: [1, 2], connectors: [] } }); flush()
    expect(fitView).toHaveBeenCalledTimes(2)
  })

  it('leaves the camera alone for empty and text-only selections', () => {
    render({ focus: null }); flush()
    render({ active: true, focus: { viewId: 1, elementIds: [], connectors: [] } }); flush()
    expect(fitView).not.toHaveBeenCalled()
    render({ active: true }); flush()
    expect(fitView).toHaveBeenCalledOnce()
  })

  it('cancels a pending fit when the panel closes before the next frame', () => {
    render({ active: true })
    render({ active: false, focus: null }); flush()
    expect(fitView).not.toHaveBeenCalled()
    render({ active: true }); flush()
    expect(fitView).toHaveBeenCalledOnce()
  })

  it('cancels a pending fit when the diagram unmounts', () => {
    render({ active: true })
    act(() => renderer.unmount()); flush()
    expect(fitView).not.toHaveBeenCalled()
  })
})
