import React from 'react'
import { act, create } from 'react-test-renderer'
import { Button } from '@chakra-ui/react'
import { expect, it, vi } from 'vitest'
import ElementNavigation from './ElementNavigation'
import type { NodeData } from './ElementNode'

vi.mock('reactflow', () => ({ useStore: (select: (state: { transform: number[] }) => unknown) => select({ transform: [0, 0, 1] }) }))
vi.mock('@chakra-ui/react', async () => ({
  ...await vi.importActual<typeof import('@chakra-ui/react')>('@chakra-ui/react'),
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

it.each(['in', 'out'] as const)('uses the shared %s menu to navigate among multiple views', direction => {
  const navigate = vi.fn()
  const links = [1, 2].map(id => ({ id, to_view_id: id + 10, from_view_id: id + 20, to_view_name: `View ${id}` }))
  const data = { element_id: 1, links, parentLinks: links, onNavigateToDiagram: navigate, onZoomIn: vi.fn(), onZoomOut: vi.fn(), onHoverZoom: vi.fn() } as unknown as NodeData
  const renderer = create(<ElementNavigation data={data} />)
  act(() => renderer.root.findByProps({ 'data-testid': `vieweditor-node-zoom-${direction}` }).props.onClick({ stopPropagation() {} }))
  const buttons = renderer.root.findAllByType(Button)
  expect(buttons).toHaveLength(3)
  act(() => buttons[0].props.onClick({ stopPropagation() {} }))
  expect(navigate).toHaveBeenCalledWith(direction === 'in' ? 11 : 21)
  expect(renderer.root.findAllByType(Button)).toHaveLength(0)
  act(() => renderer.unmount())
})
