import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFlowPlayback } from './useFlowPlayback'
import { newStep, type StoryFlow } from './model'

const diagram = { elements: [], connectors: [] }
let playback: ReturnType<typeof useFlowPlayback>
let renderer: ReactTestRenderer
function Harness({ flow }: { flow: StoryFlow }) { playback = useFlowPlayback(flow); return null }
function mount(steps: StoryFlow['steps']) {
  const flow: StoryFlow = { id: 'flow', viewId: 1, name: 'Flow', description: '', revision: 1, steps }
  act(() => { renderer = create(<Harness flow={flow} />) })
  return flow
}
afterEach(() => { act(() => renderer?.unmount()); vi.useRealTimers() })

describe('shared autoplay', () => {
  it('advances, pauses, finishes, restarts and cleans up its timer', () => {
    vi.useFakeTimers()
    mount([newStep(diagram), newStep(diagram)])
    act(() => playback.toggle())
    expect(playback.index).toBe(0)
    act(() => vi.advanceTimersByTime(3000))
    expect(playback.index).toBe(1)
    act(() => vi.advanceTimersByTime(3000))
    expect(playback.playing).toBe(false)
    act(() => playback.toggle())
    expect(playback.index).toBe(0)
    act(() => playback.pause())
    act(() => vi.advanceTimersByTime(10000))
    expect(playback.index).toBe(0)
    act(() => playback.toggle())
    act(() => renderer.unmount())
    expect(vi.getTimerCount()).toBe(0)
  })

  it('plays text-only and multi-highlight steps in sequence and loops when enabled', () => {
    vi.useFakeTimers()
    const empty = newStep(diagram)
    const highlighted = { ...newStep(diagram), elementIds: [1, 2], connectors: [{ id: 10, reverse: true }, { id: 11, reverse: false }] }
    mount([empty, highlighted])
    act(() => playback.setLoop(true))
    act(() => playback.setSeconds(1))
    act(() => playback.toggle())
    expect(playback.frame?.steps).toEqual([empty])
    act(() => vi.advanceTimersByTime(1000))
    expect(playback.frame?.steps).toEqual([highlighted])
    act(() => vi.advanceTimersByTime(1000))
    expect(playback.frame?.steps).toEqual([empty])
    expect(playback.playing).toBe(true)
  })

  it('preserves the active step on reorder and stops when it is deleted or the flow changes', () => {
    vi.useFakeTimers()
    const a = newStep(diagram), b = newStep(diagram)
    const flow = mount([a, b])
    act(() => playback.select(b.id))
    act(() => renderer.update(<Harness flow={{ ...flow, steps: [b, a] }} />))
    expect(playback.index).toBe(0)
    act(() => playback.toggle())
    act(() => renderer.update(<Harness flow={{ ...flow, steps: [a] }} />))
    expect(playback.playing).toBe(false)
    expect(playback.index).toBe(-1)
    act(() => renderer.update(<Harness flow={{ ...flow, id: 'another' }} />))
    expect(playback.index).toBe(-1)
  })
  it('keeps the selected step when a new draft receives its saved ID', () => {
    const step = newStep(diagram)
    const flow = mount([step])
    act(() => renderer.update(<Harness flow={{ ...flow, id: '', revision: 0 }} />))
    act(() => playback.select(step.id))
    act(() => renderer.update(<Harness flow={{ ...flow, id: 'saved' }} />))
    expect(playback.frame?.id).toBe(step.id)
    act(() => renderer.update(<Harness flow={{ ...flow, id: 'different' }} />))
    expect(playback.frame).toBeUndefined()
  })

})
