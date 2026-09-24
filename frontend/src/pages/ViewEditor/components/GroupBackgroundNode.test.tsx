import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import GroupBackgroundNode from './GroupBackgroundNode'

vi.mock('../../../components/Icons', () => ({
  EyeIcon: () => null,
  EyeOffIcon: () => null,
}))

vi.mock('reactflow', () => ({
  useStoreApi: () => ({ getState: () => ({ transform: [0, 0, 1] }) }),
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const ButtonLike = ({ children, onClick, ...props }: { children?: React.ReactNode; onClick?: (event: { stopPropagation: () => void }) => void }) =>
    ReactModule.createElement('button', { ...props, onClick }, children)
  return {
    Box: BoxLike,
    HStack: BoxLike,
    IconButton: ButtonLike,
    Text: BoxLike,
  }
})

function renderNode(data: Record<string, unknown>) {
  return create(React.createElement(GroupBackgroundNode, { data } as never))
}

describe('GroupBackgroundNode', () => {
  it('toggles tag visibility from the eye button', () => {
    const onToggleVisibility = vi.fn()
    const renderer = renderNode({ label: 'Payments', color: '#336699', memberNodeIds: [], onToggleVisibility })

    act(() => {
      renderer.root
        .findByProps({ 'data-testid': 'vieweditor-group-visibility-toggle' })
        .props.onClick({ stopPropagation: vi.fn() })
    })

    expect(onToggleVisibility).toHaveBeenCalledTimes(1)
  })

  it('labels the eye button based on hidden state', () => {
    const hiddenRenderer = renderNode({ label: 'Payments', color: '#336699', memberNodeIds: [], hidden: true })
    const visibleRenderer = renderNode({ label: 'Payments', color: '#336699', memberNodeIds: [], hidden: false })

    expect(hiddenRenderer.root.findByProps({ 'data-testid': 'vieweditor-group-visibility-toggle' }).props['aria-label']).toBe('Show group')
    expect(visibleRenderer.root.findByProps({ 'data-testid': 'vieweditor-group-visibility-toggle' }).props['aria-label']).toBe('Hide group')
  })

  it('moves the group when dragging the badge', () => {
    const onGroupDragStart = vi.fn()
    const onGroupDragMove = vi.fn()
    const onGroupDragEnd = vi.fn()
    const renderer = renderNode({
      label: 'Payments', color: '#336699', memberNodeIds: [],
      onGroupDragStart, onGroupDragMove, onGroupDragEnd,
    })

    const badge = renderer.root.findByProps({ 'data-testid': 'vieweditor-group-badge' })
    const capture = {
      setPointerCapture: vi.fn(),
      hasPointerCapture: () => false,
      releasePointerCapture: vi.fn(),
    }

    act(() => {
      badge.props.onPointerDown({
        button: 0, pointerId: 1, clientX: 100, clientY: 100,
        preventDefault: vi.fn(), stopPropagation: vi.fn(), currentTarget: capture,
      })
    })
    expect(onGroupDragStart).toHaveBeenCalledTimes(1)

    act(() => {
      badge.props.onPointerMove({
        pointerId: 1, clientX: 130, clientY: 140,
        preventDefault: vi.fn(), stopPropagation: vi.fn(),
      })
    })
    expect(onGroupDragMove).toHaveBeenCalledWith(30, 40)

    act(() => {
      badge.props.onPointerUp({ pointerId: 1, currentTarget: capture })
    })
    expect(onGroupDragEnd).toHaveBeenCalledTimes(1)
  })
})
