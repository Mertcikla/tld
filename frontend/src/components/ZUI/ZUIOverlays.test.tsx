import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import { ZUIHoverPopover } from './ZUIOverlays'
import type { HoveredItem, LayoutNode } from './types'

vi.mock('@chakra-ui/icons', () => ({
  ExternalLinkIcon: () => React.createElement('span'),
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const ButtonLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('button', props, children)

  return {
    Badge: BoxLike,
    Box: BoxLike,
    Breadcrumb: BoxLike,
    BreadcrumbItem: BoxLike,
    BreadcrumbLink: BoxLike,
    Button: ButtonLike,
    Divider: BoxLike,
    HStack: BoxLike,
    Icon: BoxLike,
    Image: BoxLike,
    Popover: BoxLike,
    PopoverArrow: BoxLike,
    PopoverBody: BoxLike,
    PopoverContent: ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', { 'data-testid': 'zui-popover-content', ...props }, children),
    PopoverHeader: BoxLike,
    PopoverTrigger: BoxLike,
    Portal: BoxLike,
    Text: BoxLike,
    VStack: BoxLike,
  }
})

vi.mock('react-router-dom', () => ({
  Link: ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('a', props, children),
}))

function node(): LayoutNode {
  return {
    id: 'd1-o1',
    elementId: 1,
    diagramId: 1,
    worldX: 0,
    worldY: 0,
    worldW: 100,
    worldH: 80,
    label: 'Service',
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: [],
    ancestorElementIds: [],
    pathElementIds: [1],
    children: [],
    childScale: 1,
    childOffsetX: 0,
    childOffsetY: 0,
    edgesOut: [],
  }
}

function hoveredNode(): HoveredItem {
  return {
    type: 'node',
    data: node(),
    absX: 0,
    absY: 0,
    absW: 100,
    absH: 80,
  }
}

function renderPopover(open: boolean, onHoverLock: (locked: boolean) => void) {
  return (
    <ZUIHoverPopover
      hoveredItem={open ? hoveredNode() : null}
      hoveredScreenRect={open ? { sx: 0, sy: 0, sw: 100, sh: 80 } : null}
      isHoveredItemFullyVisible={open}
      hoveredDiffDetail={null}
      onOpenSource={vi.fn()}
      onHoverLock={onHoverLock}
    />
  )
}

describe('ZUIHoverPopover hover lock', () => {
  it('releases the lock when the popover closes without a mouseleave', () => {
    const onHoverLock = vi.fn()
    const renderer = create(renderPopover(true, onHoverLock))
    const content = renderer.root.findByProps({ 'data-testid': 'zui-hover-popover' })

    act(() => {
      content.props.onMouseEnter()
    })

    expect(onHoverLock).toHaveBeenLastCalledWith(true)

    act(() => {
      renderer.update(renderPopover(false, onHoverLock))
    })

    expect(onHoverLock).toHaveBeenLastCalledWith(false)
  })
})
