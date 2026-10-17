import React from 'react'
import { act, create } from 'react-test-renderer'
import { QueryClient, QueryClientProvider, QueryObserver } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import LayoutSection from './LayoutSection'
import { api } from '../api/client'
import type { PlacedElement, ViewTreeNode } from '../types'

vi.mock('../api/client', () => ({
  api: { workspace: {
    views: { placements: { list: vi.fn(), updatePosition: vi.fn() } },
    connectors: { list: vi.fn(), update: vi.fn() },
  } },
}))
vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({ autoLayoutMode: false, setAutoLayoutMode: vi.fn() }),
}))
vi.mock('@chakra-ui/react', () => {
  const Box = ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('div', props, children)
  return {
    Box, Button: Box, Collapse: Box, FormControl: Box, FormLabel: Box, Grid: Box,
    HStack: Box, NumberDecrementStepper: Box, NumberIncrementStepper: Box,
    NumberInput: Box, NumberInputField: Box, NumberInputStepper: Box, Select: Box,
    Switch: Box, Text: Box, VStack: Box, Icon: () => null,
    useDisclosure: () => ({ isOpen: true, onOpen: vi.fn(), onClose: vi.fn() }),
  }
})
vi.mock('./ConfirmDialog', () => ({
  default: ({ onConfirm }: { onConfirm: () => void }) => <button onClick={onConfirm}>Confirm</button>,
}))

afterEach(() => { vi.clearAllMocks(); vi.unstubAllGlobals() })

describe('layout completion in a desktop webview', () => {
  it.each(['Layered', 'Organic', 'Adjust Connectors'])('refreshes saved editor content without reloading for %s', async (action) => {
    const reload = vi.fn()
    vi.stubGlobal('window', { location: { reload } })
    let savedPosition = { x: 500, y: 500 }
    vi.mocked(api.workspace.views.placements.list).mockResolvedValue([
      { element_id: 1, position_x: 500, position_y: 500 } as PlacedElement,
    ])
    vi.mocked(api.workspace.connectors.list).mockResolvedValue([])
    vi.mocked(api.workspace.views.placements.updatePosition).mockImplementation(async (_viewId, _elementId, x, y) => {
      savedPosition = { x, y }
    })

    const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
    const queryKey = ['workspace', 'views', 7, 'editor-content']
    const unrelatedKey = ['workspace', 'views', 8, 'editor-content']
    client.setQueryData(queryKey, savedPosition)
    client.setQueryData(unrelatedKey, 'another view')
    const fetchContent = vi.fn(async () => savedPosition)
    const observer = new QueryObserver(client, { queryKey, queryFn: fetchContent })
    const unsubscribe = observer.subscribe(() => {})
    let renderer!: ReturnType<typeof create>
    try {
      act(() => {
        renderer = create(<QueryClientProvider client={client}>
          <LayoutSection view={{ id: 7 } as ViewTreeNode} canEdit />
        </QueryClientProvider>)
      })
      const control = (label: string) => renderer.root.findAll((node) =>
        node.type === 'div' && typeof node.props.onClick === 'function' && node.props.children === label,
      )[0]
      if (action === 'Organic') act(() => control('Organic').props.onClick())
      if (action === 'Adjust Connectors') {
        act(() => renderer.root.findByType('button').props.onClick())
        await vi.waitFor(() => expect(fetchContent).toHaveBeenCalledTimes(1))
      } else {
        await act(async () => { await control('Apply Layout').props.onClick() })
      }
      expect(api.workspace.views.placements.updatePosition).toHaveBeenCalled()
      expect(fetchContent).toHaveBeenCalledTimes(1)
      expect(client.getQueryData(queryKey)).toEqual(savedPosition)
      expect(client.getQueryState(unrelatedKey)?.isInvalidated).toBe(false)
      expect(reload).not.toHaveBeenCalled()
    } finally {
      if (renderer) act(() => renderer.unmount())
      unsubscribe()
      client.clear()
    }
  })
})
