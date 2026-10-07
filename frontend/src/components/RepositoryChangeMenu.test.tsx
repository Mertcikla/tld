import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import RepositoryChangeMenu from './RepositoryChangeMenu'

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const ButtonLike = ({ children, onClick, isDisabled, ...props }: { children?: React.ReactNode; onClick?: () => void; isDisabled?: boolean }) =>
    ReactModule.createElement('button', { ...props, disabled: isDisabled, onClick }, children)
  return {
    Box: BoxLike,
    Button: ButtonLike,
    HStack: BoxLike,
    Text: BoxLike,
    Tooltip: BoxLike,
    VStack: BoxLike,
  }
})

function renderMenu(overrides: Partial<React.ComponentProps<typeof RepositoryChangeMenu>> = {}) {
  return create(
    <RepositoryChangeMenu
      radius={0}
      maxRadius={2}
      onRadiusChange={vi.fn()}
      viewMode="standard"
      onViewModeChange={vi.fn()}
      {...overrides}
    />,
  )
}

describe('RepositoryChangeMenu', () => {
  it('renders the blast radius stops and reports changes', () => {
    const onRadiusChange = vi.fn()
    const renderer = renderMenu({ radius: 0, maxRadius: 2, onRadiusChange })

    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-radius-2' }).length).toBeGreaterThanOrEqual(1)

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-radius-2' }).props.onClick()
    })

    expect(onRadiusChange).toHaveBeenCalledWith(2)
  })

  it('switches between standard and plain view modes', () => {
    const onViewModeChange = vi.fn()
    const renderer = renderMenu({ viewMode: 'standard', onViewModeChange })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-view-mode-plain' }).props.onClick()
    })

    expect(onViewModeChange).toHaveBeenCalledWith('plain')
  })

  it('leaves the change diagram toggle to the panel itself', () => {
    const renderer = renderMenu()

    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-change-mermaid-toggle' })).toHaveLength(0)
  })
})
