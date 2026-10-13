import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import { useTagFilters } from '../hooks/useTagFilters'
import { TagFilterReset, TagFilterRow } from './TagFilterControls'

vi.mock('@chakra-ui/react', () => {
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('div', props, children)
  const ButtonLike = ({ children, isDisabled, ...props }: { children?: React.ReactNode; isDisabled?: boolean }) => React.createElement('button', { ...props, disabled: isDisabled }, children)
  return { Box: BoxLike, HStack: BoxLike, Text: BoxLike, Button: ButtonLike, IconButton: ButtonLike }
})

function Harness() {
  const filters = useTagFilters()
  return <>
    <TagFilterReset active={filters.selectedTags.length > 0} onReset={filters.resetFilters} />
    {['go', 'javascript'].map((tag) => <TagFilterRow key={tag} tag={tag} count={1} selected={filters.selectedTags.includes(tag)} hidden={filters.hiddenTags.includes(tag)} onSelect={filters.toggleTagSelection} onHover={vi.fn()} onToggleVisibility={filters.toggleTagVisibility} />)}
  </>
}

describe('tag filter controls', () => {
  it('keeps multiple selections and switches modes when the separate eye is clicked', () => {
    const renderer = create(<Harness />)
    const button = (label: string) => renderer.root.findAllByType('button').find((item) => item.props['aria-label'] === label)!
    expect(button('Reset tag filters')).toBeUndefined()
    act(() => button('Filter by tag go').props.onClick())
    act(() => button('Filter by tag javascript').props.onClick())
    expect(button('Filter by tag go').props['aria-pressed']).toBe(true)
    expect(button('Filter by tag javascript').props['aria-pressed']).toBe(true)
    const stopPropagation = vi.fn()
    act(() => renderer.root.findAllByType('button').find((item) => item.props['aria-label'] === 'Hide tag')!.props.onClick({ stopPropagation }))
    expect(stopPropagation).toHaveBeenCalledOnce()
    expect(button('Filter by tag go').props['aria-pressed']).toBe(false)
    expect(button('Filter by tag javascript').props['aria-pressed']).toBe(false)
    expect(button('Show tag')).toBeDefined()
    expect(button('Reset tag filters')).toBeUndefined()
    act(() => button('Filter by tag javascript').props.onClick())
    expect(renderer.root.findAllByType('button').some((item) => item.props['aria-label'] === 'Show tag')).toBe(false)
    act(() => button('Reset tag filters').props.onClick())
    expect(button('Reset tag filters')).toBeUndefined()
    act(() => renderer.unmount())
  })
})
