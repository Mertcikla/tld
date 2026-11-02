import React, { useRef } from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { PREVIEW_HOVER_DELAY, usePreviewInteraction } from './usePreviewInteraction'

function Harness({ enabled = true, moving = false }) {
  const scroller = useRef<HTMLDivElement>(null)
  const gate = usePreviewInteraction(enabled, moving, scroller)
  return <span data-phase={gate.phase} onPointerEnter={gate.onPointerEnter} onPointerLeave={gate.onPointerLeave} />
}
beforeEach(() => vi.useFakeTimers())
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })

it('requires the configured duration of continuous hover and resets when leaving or moving the canvas', () => {
  const renderer = create(<Harness />)
  const props = () => renderer.root.findByType('span').props
  act(() => props().onPointerEnter({ clientX: 20, clientY: 30 }))
  act(() => vi.advanceTimersByTime(PREVIEW_HOVER_DELAY - 1))
  expect(props()['data-phase']).toBe('waiting')
  act(() => vi.advanceTimersByTime(1))
  expect(props()['data-phase']).toBe('ready')
  act(() => props().onPointerEnter({ clientX: 25, clientY: 35 }))
  expect(props()['data-phase']).toBe('ready')
  act(() => props().onPointerLeave())
  expect(props()['data-phase']).toBe('idle')
  act(() => props().onPointerEnter({ clientX: 20, clientY: 30 }))
  act(() => vi.advanceTimersByTime(PREVIEW_HOVER_DELAY / 2))
  act(() => renderer.update(<Harness moving />))
  expect(props()['data-phase']).toBe('idle')
  act(() => renderer.update(<Harness />))
  act(() => vi.advanceTimersByTime(PREVIEW_HOVER_DELAY - 1))
  expect(props()['data-phase']).toBe('waiting')
  act(() => vi.advanceTimersByTime(1))
  expect(props()['data-phase']).toBe('ready')
  act(() => renderer.update(<Harness enabled={false} />))
  expect(props()['data-phase']).toBe('idle')
  act(() => renderer.unmount())
})
