import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useCopyFeedback, type CopyAction } from './useCopyFeedback'

function Harness({ action }: { action: CopyAction }) {
  const { state, copy } = useCopyFeedback(action)
  return <button data-state={state} onClick={copy} />
}

afterEach(() => vi.useRealTimers())

describe('copy feedback', () => {
  it('waits for success, ignores duplicate presses, then resets for another copy', async () => {
    vi.useFakeTimers()
    let finish!: (success: boolean) => void
    const action = vi.fn(() => new Promise<boolean>((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(<Harness action={action} />) })
    const button = () => renderer.root.findByType('button')
    let request!: Promise<void>
    act(() => { request = button().props.onClick() })
    expect(button().props['data-state']).toBe('copying')
    await act(async () => { await button().props.onClick() })
    expect(action).toHaveBeenCalledOnce()
    await act(async () => { finish(true); await request })
    expect(button().props['data-state']).toBe('copied')
    act(() => { vi.advanceTimersByTime(1800) })
    expect(button().props['data-state']).toBe('idle')
    act(() => renderer.unmount())
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([
    ['a handled failure', () => false],
    ['a rejected clipboard write', () => Promise.reject(new Error('Clipboard denied'))],
  ] as const)('does not show success after %s', async (_label, action) => {
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(<Harness action={action} />) })
    await act(async () => { await renderer.root.findByType('button').props.onClick() })
    expect(renderer.root.findByType('button').props['data-state']).toBe('idle')
    act(() => renderer.unmount())
  })

  it('does not schedule feedback if unmounted during a copy', async () => {
    vi.useFakeTimers()
    let finish!: () => void
    const action = () => new Promise<void>((resolve) => { finish = resolve })
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(<Harness action={action} />) })
    let request!: Promise<void>
    act(() => { request = renderer.root.findByType('button').props.onClick() })
    act(() => renderer.unmount())
    await act(async () => { finish(); await request })
    expect(vi.getTimerCount()).toBe(0)
  })
})
