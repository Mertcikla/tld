import React from 'react'
import { act, create } from 'react-test-renderer'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import { api } from '../api/client'
import type { LibraryElement, ViewThread } from '../types'
import ElementPanelCollaboration from './ElementPanelCollaboration'

vi.mock('../api/client', () => ({ api: { workspace: { views: {
  threads: { listForElement: vi.fn() },
  reactions: { list: vi.fn() },
} } } }))
vi.mock('../pages/ViewEditor/context', () => ({
  useViewEditorContext: () => ({ viewId: 1, selectedElement: null }),
}))
vi.mock('@chakra-ui/react', () => {
  const Box = React.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(
    ({ children, ...props }, ref) => <div ref={ref} {...props}>{children}</div>,
  )
  return Object.fromEntries(['Badge', 'Box', 'Button', 'FormLabel', 'HStack', 'Input', 'SimpleGrid', 'Text', 'Tooltip', 'VStack'].map((name) => [name, Box]))
})

function thread(elementId: number, body: string): ViewThread {
  return {
    id: elementId, org_id: '', view_id: 1, diagram_id: 1, element_id: elementId,
    connector_id: null, created_by: '', created_by_username: '', status: 'open',
    created_at: '', resolved_at: null,
    comments: [{ id: elementId, org_id: '', view_id: 1, diagram_id: 1, thread_id: elementId,
      author_id: '', author_username: '', body, created_at: '', updated_at: '' }],
  }
}

it('keeps a late collaboration response from replacing the newly selected element’s comments and reactions', async () => {
  let finishFirst!: (threads: ViewThread[]) => void
  const firstRequest = new Promise<ViewThread[]>((resolve) => { finishFirst = resolve })
  vi.mocked(api.workspace.views.threads.listForElement).mockImplementation((_viewId, elementId) => (
    elementId === 1 ? firstRequest : Promise.resolve([thread(2, 'Second element comment')])
  ))
  vi.mocked(api.workspace.views.reactions.list).mockResolvedValue([
    { element_id: 1, emoji: '🍋', count: 1, reacted_by_me: false },
    { element_id: 2, emoji: '🍎', count: 1, reacted_by_me: false },
  ])
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
  const render = (id: number) => (
    <QueryClientProvider client={client}>
      <ElementPanelCollaboration element={{ id } as LibraryElement} />
    </QueryClientProvider>
  )
  let renderer!: ReturnType<typeof create>
  const content = () => JSON.stringify(renderer.toJSON())
  try {
    act(() => { renderer = create(render(1)) })
    act(() => { renderer.update(render(2)) })
    await vi.waitFor(async () => {
      await act(async () => {})
      expect(content()).toContain('Second element comment')
    })
    expect(content()).toContain('🍎')
    await act(async () => {
      finishFirst([thread(1, 'First element comment')])
      await firstRequest
    })
    expect(content()).toContain('Second element comment')
    expect(content()).not.toContain('First element comment')
    expect(content()).not.toContain('🍋')
  } finally {
    act(() => { renderer.unmount() })
    client.clear()
  }
})
