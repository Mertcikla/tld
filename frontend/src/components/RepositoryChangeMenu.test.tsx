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
      mermaidOpen
      onToggleMermaid={vi.fn()}
      hasMermaid
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

  it('toggles the mermaid pane', () => {
    const onToggleMermaid = vi.fn()
    const renderer = renderMenu({ onToggleMermaid })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-toggle' }).props.onClick()
    })

    expect(onToggleMermaid).toHaveBeenCalledOnce()
  })

  it('disables the mermaid toggle without a diagram', () => {
    const renderer = renderMenu({ hasMermaid: false })

    expect(renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-toggle' }).props.isDisabled).toBe(true)
  })
})
