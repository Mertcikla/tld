import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AutosaveField from './AutosaveField'

vi.mock('@chakra-ui/react', () => {
  const Box = ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('div', props, children)
  return { Box, HStack: Box, Text: Box }
})
afterEach(() => vi.useRealTimers())

describe('inline autosave feedback', () => {
  it('keeps the spinner visible for a slow save, then replaces it with Saved', () => {
    vi.useFakeTimers()
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(<AutosaveField enabled saving field="name"><input /></AutosaveField>) })
    act(() => vi.advanceTimersByTime(5000))
    expect(renderer.root.findByProps({ role: 'status' }).props.opacity).toBe(1)
    expect(renderer.root.findAllByProps({ 'data-testid': 'autosave-field-spinner' }).filter((node) => typeof node.type === 'string')).toHaveLength(1)
    act(() => renderer.update(<AutosaveField enabled receipt={1} field="name"><input /></AutosaveField>))
    expect(renderer.root.findAllByProps({ 'data-testid': 'autosave-field-spinner' }).filter((node) => typeof node.type === 'string')).toHaveLength(0)
    expect(renderer.root.findByProps({ role: 'status' }).props.opacity).toBe(1)
    expect(renderer.root.findAllByType('div').some((node) => node.props.children === 'Saved')).toBe(true)
    act(() => renderer.unmount())
  })

  it('shows a successful confirmation for two seconds and hides it as soon as a new edit arrives', () => {
    vi.useFakeTimers()
    const render = (receipt?: number) => <AutosaveField enabled receipt={receipt} field="name"><input /></AutosaveField>
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(render()) })
    const status = () => renderer.root.findByProps({ role: 'status' })
    expect(status().props.opacity).toBe(0)
    act(() => renderer.update(render(1)))
    expect(status().props.opacity).toBe(1)
    act(() => renderer.update(render()))
    expect(status().props.opacity).toBe(0)
    act(() => renderer.update(render(2)))
    expect(status().props.opacity).toBe(1)
    act(() => vi.advanceTimersByTime(2000))
    expect(status().props.opacity).toBe(0)
    act(() => renderer.unmount())
    expect(vi.getTimerCount()).toBe(0)
  })
})
