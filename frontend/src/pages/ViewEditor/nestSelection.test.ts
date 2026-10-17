import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import type { Connector, PlacedElement } from '../../types'
import { nestSelection, sharedNestTechnology } from './nestSelection'

vi.mock('../../api/client', () => ({
  api: {
    elements: { create: vi.fn(), delete: vi.fn() },
    workspace: {
      views: {
        create: vi.fn(), delete: vi.fn(),
        placements: { add: vi.fn(), remove: vi.fn() },
      },
      connectors: { update: vi.fn(), list: vi.fn(), delete: vi.fn() },
    },
  },
}))

function placement(id: number, overrides: Partial<PlacedElement> = {}): PlacedElement {
  return {
    id, element_id: id, view_id: 10, position_x: id * 200, position_y: id * 100,
    name: `Element ${id}`, description: null, kind: null, technology: 'Go',
    logo_url: '/icons/go.svg', technology_connectors: [{ type: 'custom', label: 'Go' }],
    tags: [], url: null, has_view: false, view_label: null, ...overrides,
  }
}

function connector(id: number, source: number, target: number): Connector {
  return {
    id, view_id: 10, source_element_id: source, target_element_id: target,
    label: `Connection ${id}`, description: null, relationship: null,
    direction: 'both', style: 'bezier', source_handle: 'right', target_handle: 'left',
    url: null, tags: ['internal'], created_at: '', updated_at: '',
  }
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(api.elements.create).mockResolvedValue({ id: 99, name: 'New Nest' } as Awaited<ReturnType<typeof api.elements.create>>)
  vi.mocked(api.workspace.views.create).mockResolvedValue({ id: 20 } as Awaited<ReturnType<typeof api.workspace.views.create>>)
  vi.mocked(api.workspace.connectors.list).mockResolvedValue([])
})

describe('sharedNestTechnology', () => {
  it('copies shared technology and icon without mutating the selection', () => {
    const elements = [placement(1), placement(2)]
    expect(sharedNestTechnology(elements)).toEqual({
      technology: 'Go', logo_url: '/icons/go.svg',
      technology_connectors: [{ type: 'custom', label: 'Go', is_primary_icon: false }],
    })
    expect(elements[0].technology_connectors[0]).not.toHaveProperty('is_primary_icon')
  })

  it('keeps only common technologies and clears differing icons', () => {
    expect(sharedNestTechnology([
      placement(1, { technology: 'Go, Redis', technology_connectors: [{ type: 'custom', label: 'Go' }, { type: 'custom', label: 'Redis' }] }),
      placement(2, { logo_url: '/icons/redis.svg' }),
    ])).toEqual({
      technology: 'Go', logo_url: '', technology_connectors: [{ type: 'custom', label: 'Go', is_primary_icon: false }],
    })
  })

  it('does not inherit technology or an icon when nothing is shared', () => {
    expect(sharedNestTechnology([placement(1), placement(2, {
      technology: 'Rust', logo_url: null, technology_connectors: [{ type: 'custom', label: 'Rust' }],
    })])).toEqual({ technology: '', logo_url: '', technology_connectors: [] })
  })
})

describe('nestSelection', () => {
  it('moves internal connectors by ID, preserves externals and centers the new element', async () => {
    const internal = connector(1, 1, 2)
    const self = connector(2, 1, 1)
    vi.mocked(api.workspace.connectors.list).mockResolvedValue([
      { ...internal, view_id: 20 }, { ...self, view_id: 20 },
      { ...connector(100, 1, 2), view_id: 20 },
    ])
    const result = await nestSelection({
      viewId: 10, placements: [placement(1), placement(2)],
      connectors: [internal, self, connector(3, 1, 3), connector(4, 3, 2), connector(5, 3, 4)],
      bounds: { centerX: 500, centerY: 300 },
    })
    expect(result).toMatchObject({ id: 99, name: 'New Nest', has_view: true })
    expect(api.workspace.views.create).toHaveBeenCalledWith({ name: 'New Nest', parent_view_id: 99 })
    expect(api.workspace.connectors.update).toHaveBeenCalledTimes(2)
    expect(api.workspace.connectors.update).toHaveBeenCalledWith(20, 1, { tags: ['internal'] })
    expect(api.workspace.connectors.update).toHaveBeenCalledWith(20, 2, { tags: ['internal'] })
    expect(api.workspace.connectors.delete).toHaveBeenCalledExactlyOnceWith('', 100)
    expect(api.workspace.views.placements.add).toHaveBeenCalledWith(20, 1, 200, 100)
    expect(api.workspace.views.placements.add).toHaveBeenCalledWith(20, 2, 400, 200)
    expect(api.workspace.views.placements.add).toHaveBeenCalledWith(10, 99, 410, 257.5)
    expect(api.workspace.views.placements.remove).toHaveBeenCalledWith(10, 1)
    expect(api.workspace.views.placements.remove).toHaveBeenCalledWith(10, 2)
    expect(vi.mocked(api.workspace.connectors.update).mock.invocationCallOrder[1])
      .toBeLessThan(vi.mocked(api.workspace.views.placements.add).mock.invocationCallOrder[0])
  })

  it('restores moved connectors and removed placements if nesting fails', async () => {
    vi.mocked(api.workspace.views.placements.remove).mockResolvedValueOnce().mockRejectedValueOnce(new Error('Removal failed'))
    await expect(nestSelection({
      viewId: 10, placements: [placement(1), placement(2)], connectors: [connector(1, 1, 2)],
      bounds: { centerX: 500, centerY: 300 },
    })).rejects.toThrow('Removal failed')
    expect(api.workspace.connectors.update).toHaveBeenLastCalledWith(10, 1, { tags: ['internal'] })
    expect(api.workspace.views.placements.add).toHaveBeenLastCalledWith(10, 1, 200, 100)
    expect(api.workspace.views.delete).toHaveBeenCalledWith('', 20)
    expect(api.elements.delete).toHaveBeenCalledWith('', 99)
  })

  it('retains the nest if restoring a connector fails', async () => {
    vi.mocked(api.workspace.connectors.update).mockResolvedValueOnce({} as Connector).mockRejectedValueOnce(new Error('Offline'))
    vi.mocked(api.workspace.views.placements.add).mockRejectedValueOnce(new Error('Offline'))
    await expect(nestSelection({
      viewId: 10, placements: [placement(1), placement(2)], connectors: [connector(1, 1, 2)],
      bounds: { centerX: 500, centerY: 300 },
    })).rejects.toThrow('some content remains in New Nest')
    expect(api.workspace.views.delete).not.toHaveBeenCalled()
    expect(api.elements.delete).not.toHaveBeenCalled()
  })
})
