import React from 'react'
import { MemoryRouter } from 'react-router-dom'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import type { DependencyConnector, LibraryElement, ViewTreeNode } from '../types'
import Inventory from './Inventory'
import InventoryList from './InventoryList'

vi.mock('../api/client', () => ({
  api: {
    elements: { list: vi.fn() },
    dependencies: { list: vi.fn() },
    workspace: {
      elements: { delete: vi.fn(), update: vi.fn() },
      views: { gridData: vi.fn(), delete: vi.fn(), layers: { list: vi.fn(async () => []) } },
      connectors: { delete: vi.fn() },
      orgs: { tagColors: { list: vi.fn(async () => ({})) } },
    },
  },
}))

vi.mock('@chakra-ui/react', () => Object.fromEntries(
  ['Box', 'Flex', 'HStack', 'IconButton', 'Tooltip'].map((name) => [
    name,
    ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
  ]),
))
vi.mock('@chakra-ui/icons', () => ({ EditIcon: () => null }))
vi.mock('../components/Icons', () => ({ ZoomInIcon: () => null }))
vi.mock('../components/ConnectorPanel', () => ({ default: () => null }))
vi.mock('../components/ElementPanel', () => ({ default: () => null }))
vi.mock('../components/ViewPanel', () => ({ default: () => null }))
vi.mock('../components/InventoryInspector', () => ({ default: () => null }))
vi.mock('./InventoryCreateDialog', () => ({ default: () => null }))
vi.mock('./InventoryList', () => ({ default: () => null }))

// Deliberately share numeric IDs across object types to exercise typed row keys.
const element: LibraryElement = {
  id: 1, name: 'Service', kind: 'service', description: null, technology: null,
  url: null, logo_url: null, technology_connectors: [], tags: [],
  created_at: '', updated_at: '', has_view: true, view_label: null,
}
const view: ViewTreeNode = {
  id: 1, name: 'Child view', owner_element_id: 1, description: null, level_label: null,
  level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children: [],
}
const connector: DependencyConnector = {
  id: '1', view_id: '1', source_element_id: '1', target_element_id: '1',
  direction: 'forward', connector_type: 'bezier', tags: [], created_at: '', updated_at: '',
}

function snapshot(elements: LibraryElement[], views: ViewTreeNode[], connectors: DependencyConnector[]) {
  vi.mocked(api.elements.list).mockResolvedValue(elements)
  vi.mocked(api.workspace.views.gridData).mockResolvedValue({ views, content: {} })
  vi.mocked(api.dependencies.list).mockResolvedValue({
    elements: elements.map((item) => ({ ...item, id: String(item.id) })),
    connectors,
  })
}

let renderer: ReactTestRenderer
const list = () => renderer.root.findByType(InventoryList).props as React.ComponentProps<typeof InventoryList>

async function mount() {
  await act(async () => {
    renderer = create(<MemoryRouter><Inventory /></MemoryRouter>)
  })
  act(() => list().state.table.toggleAllRowsSelected(true))
}

async function deleteSelected() {
  let success = false
  await act(async () => {
    success = await list().onDelete(list().state.selectedRows)
  })
  return success
}

beforeEach(() => {
  vi.clearAllMocks()
  snapshot([element], [view], [connector])
  vi.mocked(api.workspace.elements.delete).mockResolvedValue(undefined)
  vi.mocked(api.workspace.views.delete).mockRejectedValue(new Error('[not_found] view not found'))
  vi.mocked(api.workspace.connectors.delete).mockRejectedValue(new Error('[not_found] connector not found'))
})

afterEach(() => {
  act(() => renderer?.unmount())
})

describe('inventory bulk deletion', () => {
  it('counts objects removed by cascade as deleted even when their own requests reject', async () => {
    await mount()
    expect(list().state.selectedRows).toHaveLength(3)
    snapshot([], [], [])

    expect(await deleteSelected()).toBe(true)

    expect(api.workspace.elements.delete).toHaveBeenCalledWith('', 1)
    expect(api.workspace.views.delete).toHaveBeenCalledWith('', 1)
    expect(api.workspace.connectors.delete).toHaveBeenCalledWith('', 1)
    expect(api.elements.list).toHaveBeenCalledTimes(2)
    expect(list().rows).toEqual([])
    expect(list().error).toBeNull()
    expect(list().state.selectedRows).toEqual([])
    expect(list().busy).toBe(false)
  })

  it('reports and retains only failed objects still present after refresh, allowing retry', async () => {
    await mount()
    snapshot([], [view], [])

    expect(await deleteSelected()).toBe(false)

    expect(list().error).toBe('2 of 3 objects deleted. 1 failed and remain selected; retry the action.')
    expect(list().state.selectedRows.map((row) => row.key)).toEqual(['view:1'])

    vi.mocked(api.workspace.views.delete).mockResolvedValue(undefined)
    snapshot([], [], [])
    expect(await deleteSelected()).toBe(true)
    expect(list().error).toBeNull()
    expect(list().state.selectedRows).toEqual([])
  })

  it('preserves failures when refresh cannot confirm the objects are gone', async () => {
    await mount()
    vi.mocked(api.elements.list).mockRejectedValue(new Error('Could not load inventory.'))

    expect(await deleteSelected()).toBe(false)

    expect(list().error).toBe('1 of 3 objects deleted. 2 failed and remain selected; retry the action.')
    expect(list().state.selectedRows.map((row) => row.key).sort()).toEqual(['connector:1', 'view:1'])
    expect(list().busy).toBe(false)
  })

  it('does not treat a failed tag update as successful when the object disappears', async () => {
    snapshot([element], [], [])
    await mount()
    vi.mocked(api.workspace.elements.update).mockRejectedValue(new Error('Update failed'))
    snapshot([], [], [])

    await act(async () => list().onTag('backend', false))

    expect(api.workspace.elements.update).toHaveBeenCalledWith(1, { tags: ['backend'] })
    expect(list().error).toBe('0 of 1 objects updated. 1 failed and remain selected; retry the action.')
  })
})
