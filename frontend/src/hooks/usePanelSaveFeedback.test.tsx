import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { usePanelSaveFeedback } from './usePanelSaveFeedback'

const toast = vi.hoisted(() => vi.fn())
vi.mock('@chakra-ui/react', () => ({ useToast: () => toast }))

let renderer: ReactTestRenderer
let feedback: ReturnType<typeof usePanelSaveFeedback>
function Harness({ identity }: { identity: string }) {
  feedback = usePanelSaveFeedback(identity)
  return <span>{feedback.state}</span>
}
beforeEach(() => { toast.mockClear() })
afterEach(() => { act(() => renderer?.unmount()) })

describe('panel save feedback', () => {
  it.each(['closed', 'element-2'])('clears previous feedback when returning from %s', (nextIdentity) => {
    act(() => { renderer = create(<Harness identity="element-1" />) })
    act(() => { feedback.failed() })
    act(() => { renderer.update(<Harness identity="element-1" />) })
    expect(feedback.state).toBe('error')

    act(() => { renderer.update(<Harness identity={nextIdentity} />) })
    expect(feedback.state).toBe('idle')
    act(() => { renderer.update(<Harness identity="element-1" />) })
    expect(feedback.state).toBe('idle')
    expect(toast).not.toHaveBeenCalled()

    act(() => { feedback.startSaving() })
    act(() => { feedback.saved() })
    expect(feedback.state).toBe('saved')
  })

  it('keeps late results from a previous session out of the reopened panel', () => {
    act(() => { renderer = create(<Harness identity="element-1" />) })
    act(() => { feedback.startSaving() })
    const oldRequest = feedback
    act(() => { renderer.update(<Harness identity="closed" />) })
    act(() => { renderer.update(<Harness identity="element-1" />) })
    act(() => { oldRequest.saved() })
    expect(feedback.state).toBe('idle')
    act(() => { oldRequest.failed() })
    expect(feedback.state).toBe('idle')
    expect(toast).toHaveBeenCalledOnce()
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ status: 'error' }))
  })

  it('leaves active-panel failures to the local toast without duplicating notifications', () => {
    act(() => { renderer = create(<Harness identity="element-1" />) })
    act(() => { feedback.failed() })
    expect(feedback.state).toBe('error')
    expect(toast).not.toHaveBeenCalled()
  })

  it('keeps a previous panel’s late result from changing the selected panel’s status', () => {
    act(() => { renderer = create(<Harness identity="element-1" />) })
    act(() => { feedback.startSaving() })
    const oldRequest = feedback
    act(() => { renderer.update(<Harness identity="element-2" />) })
    expect(feedback.state).toBe('idle')
    act(() => { oldRequest.saved() })
    expect(feedback.state).toBe('idle')
  })

  it('still reports a failed request after its panel has closed', () => {
    act(() => { renderer = create(<Harness identity="element-1" />) })
    const oldRequest = feedback
    act(() => { renderer.update(<Harness identity="closed" />) })
    act(() => { oldRequest.failed() })
    expect(feedback.state).toBe('idle')
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ status: 'error', title: 'Changes weren’t saved' }))
  })
})
