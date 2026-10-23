import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import { ViewSearch } from './ViewSearch'

vi.mock('@chakra-ui/icons', () => ({ CloseIcon: () => null, SearchIcon: () => null }))
vi.mock('@chakra-ui/react', () => {
  const Box = ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('div', props, children)
  const Input = React.forwardRef<HTMLInputElement>((props, ref) => React.createElement('input', { ...props, ref }))
  return { Box, Input, InputGroup: Box, InputLeftElement: Box, InputRightElement: Box,
    IconButton: (props: object) => React.createElement('button', props) }
})

describe('view search clear control', () => {
  it('clears the controlled query and returns keyboard focus to the search', () => {
    const setQuery = vi.fn()
    const focus = vi.fn()
    const onKeyDown = vi.fn()
    const renderer = create(<ViewSearch query="api" setQuery={setQuery} activeFilter="in" onKeyDown={onKeyDown} />, {
      createNodeMock: (element) => element.type === 'input' ? { focus } : null,
    })
    expect(renderer.root.findByType('input').props['aria-label']).toBe('Search children')
    expect(renderer.root.findByType('input').props.onKeyDown).toBe(onKeyDown)
    const clear = renderer.root.findByType('button')
    expect(clear.props.tabIndex).toBe(0)
    act(() => clear.props.onClick())
    expect(setQuery).toHaveBeenCalledWith('')
    expect(focus).toHaveBeenCalledOnce()
    act(() => renderer.update(<ViewSearch query="" setQuery={setQuery} activeFilter={null} />))
    expect(renderer.root.findByType('button').props.isDisabled).toBe(true)
    expect(renderer.root.findByType('button').props.tabIndex).toBe(-1)
    act(() => renderer.unmount())
  })
})
