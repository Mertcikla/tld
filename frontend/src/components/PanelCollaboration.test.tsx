import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Connector, LibraryElement } from '../types'
import ElementPanelCollaboration from './ElementPanelCollaboration'
import ConnectorPanelCollaboration from './ConnectorPanelCollaboration'

const mocks = vi.hoisted(() => ({
  viewId: 1,
  listElementThreads: vi.fn(),
  listConnectorThreads: vi.fn(),
  listReactions: vi.fn(),
}))
vi.mock('../api/client', () => ({ api: { workspace: { views: {
  threads: { listForElement: mocks.listElementThreads, listForConnector: mocks.listConnectorThreads },
  reactions: { list: mocks.listReactions },
} } } }))
vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({ viewId: mocks.viewId, selectedElement: null, selectedConnector: null }),
}))
vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const box = ReactModule.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(({ children, ...props }, ref) => ReactModule.createElement('div', { ...props, ref }, children))
  box.displayName = 'Box'
  return {
    ...Object.fromEntries(['Badge', 'Box', 'FormLabel', 'HStack', 'SimpleGrid', 'Text', 'Tooltip', 'VStack'].map((name) => [name, box])),
    Button: ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('button', props, children),
    Input: (props: Record<string, unknown>) => ReactModule.createElement('input', props),
  }
})

const element: LibraryElement = {
  id: 10, name: 'API', kind: 'service', description: null, technology: null, url: null,
  logo_url: null, technology_connectors: [], tags: [], created_at: '', updated_at: '', has_view: false, view_label: null,
}
const connector: Connector = {
  id: 10, view_id: 1, source_element_id: 1, target_element_id: 2, label: 'calls', description: null,
  relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: null, target_handle: null,
  created_at: '', updated_at: '', tags: [],
}
let renderer: ReactTestRenderer
let queryClient: QueryClient
beforeEach(() => {
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
  mocks.viewId = 1
  mocks.listElementThreads.mockReset().mockResolvedValue([])
  mocks.listConnectorThreads.mockReset().mockResolvedValue([])
  mocks.listReactions.mockReset().mockResolvedValue([])
})
afterEach(() => {
  act(() => renderer?.unmount())
  queryClient.clear()
})

describe.each(['element', 'connector'] as const)('%s collaboration panel', (kind) => {
  const listThreads = kind === 'element' ? mocks.listElementThreads : mocks.listConnectorThreads
  const panel = (id = 10, description = '') => (
    <QueryClientProvider client={queryClient}>
      {kind === 'element'
        ? <ElementPanelCollaboration element={{ ...element, id, description }} />
        : <ConnectorPanelCollaboration connector={{ ...connector, id, description }} />}
    </QueryClientProvider>
  )

  it('keeps loaded comments visible when autosave replaces the metadata object', async () => {
    await act(async () => { renderer = create(panel()) })
    await vi.waitFor(async () => {
      await act(async () => {})
      expect(renderer.root.findAllByType('button').find((button) => button.props.children === 'Post')?.props.isLoading).toBe(false)
    })
    expect(listThreads).toHaveBeenCalledOnce()
    // An unnecessary refresh would hide the empty state until this pending request finishes.
    listThreads.mockImplementation(() => new Promise(() => {}))
    await act(async () => { renderer.update(panel(10, 'Saved metadata change')) })
    expect(listThreads).toHaveBeenCalledOnce()
    expect(renderer.root.findAllByType('button').find((button) => button.props.children === 'Post')?.props.isLoading).toBe(false)
    expect(renderer.root.findAllByType('div').some((node) => node.props.children === 'No threads yet.')).toBe(true)
    if (kind === 'element') expect(mocks.listReactions).toHaveBeenCalledOnce()
  })

  it('loads comments when selecting a different item', async () => {
    await act(async () => { renderer = create(panel()) })
    await act(async () => { renderer.update(panel(11)) })
    expect(listThreads).toHaveBeenCalledTimes(2)
    expect(listThreads).toHaveBeenLastCalledWith(1, 11)
  })

  it('reloads comments when the view changes', async () => {
    await act(async () => { renderer = create(panel()) })
    mocks.viewId = 2
    await act(async () => { renderer.update(panel()) })
    expect(listThreads).toHaveBeenCalledTimes(2)
    expect(listThreads).toHaveBeenLastCalledWith(2, 10)
  })
})
