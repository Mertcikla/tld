import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PanelSaveStatus from './PanelSaveStatus'

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const box = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  return { Box: box, HStack: box, Text: box, Button: ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('button', props, children) }
})
let renderer: ReactTestRenderer
beforeEach(() => { vi.useFakeTimers() })
afterEach(() => { act(() => renderer?.unmount()); vi.useRealTimers() })
const messages = () => renderer.root.findAllByProps({ 'data-testid': 'panel-save-status' })

describe('panel save toast', () => {
  it('skips intermediate feedback for fast saves and clears the confirmation after two seconds', () => {
    act(() => { renderer = create(<PanelSaveStatus state="idle" />) })
    expect(messages()).toHaveLength(0)
    act(() => { renderer.update(<PanelSaveStatus state="saving" />); vi.advanceTimersByTime(100) })
    expect(messages()).toHaveLength(0)
    act(() => { renderer.update(<PanelSaveStatus state="saved" />) })
    expect(messages()[0].props.children).toBe('Changes saved.')
    act(() => { vi.advanceTimersByTime(2000) })
    expect(messages()).toHaveLength(0)
  })

  it('shows progress for a slow save and retains the confirmation during a subsequent fast save', () => {
    act(() => { renderer = create(<PanelSaveStatus state="saving" />) })
    act(() => { vi.advanceTimersByTime(300) })
    expect(messages()[0].props.children).toBe('Saving changes…')
    act(() => { renderer.update(<PanelSaveStatus state="saved" />) })
    act(() => { renderer.update(<PanelSaveStatus state="saving" />) })
    act(() => { vi.advanceTimersByTime(100) })
    expect(messages()[0].props.children).toBe('Changes saved.')
    act(() => { renderer.update(<PanelSaveStatus state="saved" />) })
    act(() => { vi.advanceTimersByTime(2000) })
    expect(messages()).toHaveLength(0)
  })

  it('keeps a failure visible until retry and prevents repeated retry clicks while saving', () => {
    const retry = vi.fn()
    act(() => { renderer = create(<PanelSaveStatus state="error" onRetry={retry} />) })
    act(() => { vi.advanceTimersByTime(5000) })
    expect(messages()[0].props.role).toBe('alert')
    act(() => { renderer.root.findByType('button').props.onClick() })
    expect(retry).toHaveBeenCalledOnce()
    act(() => { renderer.update(<PanelSaveStatus state="saving" onRetry={retry} />) })
    expect(renderer.root.findByType('button').props.isDisabled).toBe(true)
    expect(renderer.root.findByType('button').props.onClick).toBeUndefined()
    act(() => { renderer.update(<PanelSaveStatus state="saved" onRetry={retry} />) })
    expect(messages()[0].props.children).toBe('Changes saved.')
  })
})
