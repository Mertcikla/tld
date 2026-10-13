import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { useInventoryTable, type InventoryTableState } from './useInventoryTable'
import type { InventoryRow } from './inventoryData'

function row(id: number, overrides: Partial<InventoryRow> = {}): InventoryRow {
  return {
    key: `element:${id}`,
    id,
    objectType: 'element',
    name: `Item ${String(id).padStart(3, '0')}`,
    subtitle: 'service',
    tags: ['backend'],
    filterTags: ['backend'],
    updatedAt: '2026-01-01',
    typeLabel: 'service',
    usageCount: id,
    usageLabel: `${id} connectors`,
    searchableText: `item ${id} backend`,
    qualityFlags: ['untagged'],
    ...overrides,
  }
}

function harness(rows: InventoryRow[], url = '/inventory', loading = false) {
  let state: InventoryTableState
  let search = ''
  function Probe({ data, pending }: { data: InventoryRow[]; pending: boolean }) {
    state = useInventoryTable(data, pending)
    search = useLocation().search
    return null
  }
  let renderer: ReactTestRenderer
  act(() => {
    renderer = create(
      <MemoryRouter initialEntries={[url]}>
        <Probe data={rows} pending={loading} />
      </MemoryRouter>,
    )
  })
  return {
    get state() {
      return state!
    },
    get params() {
      return new URLSearchParams(search)
    },
    update(data: InventoryRow[], pending = false) {
      act(() =>
        renderer.update(
          <MemoryRouter initialEntries={[url]}>
            <Probe data={data} pending={pending} />
          </MemoryRouter>,
        ),
      )
    },
    close() {
      act(() => renderer.unmount())
    },
  }
}

describe('inventory table state', () => {
  it('composes filters and persists sorting while clearing stale page and object state', () => {
    const app = harness(
      [row(1), row(2, { typeLabel: 'database', filterTags: ['storage'] }), row(3)],
      '/inventory?page=2&object=element:1',
    )
    act(() => app.state.toggleFilter('tags', 'backend'))
    act(() => app.state.updateParams({ kind: 'service', q: 'item 3' }))
    expect(app.state.table.getRowModel().rows.map((row) => row.id)).toEqual(['element:3'])
    expect(app.params.get('object')).toBeNull()
    expect(app.params.get('page')).toBeNull()
    act(() => app.state.table.setSorting([{ id: 'usage', desc: true }]))
    expect(app.params.get('sort')).toBe('usage')
    expect(app.params.get('dir')).toBe('desc')
    act(() => app.state.resetFilters())
    expect(app.state.table.getRowModel().rows.map((row) => row.id)).toEqual([
      'element:3',
      'element:2',
      'element:1',
    ])
    app.close()
  })

  it('sorts usage numerically and unnamed connectors by their displayed endpoints', () => {
    const app = harness([
      row(10),
      row(2),
      row(1, { key: 'connector:1', objectType: 'connector', name: '', subtitle: 'Zebra -> Worker' }),
      row(3, { key: 'connector:3', objectType: 'connector', name: '', subtitle: 'Alpha -> Worker' }),
    ])
    expect(app.state.table.getRowModel().rows.map((row) => row.id)).toEqual([
      'connector:3',
      'element:2',
      'element:10',
      'connector:1',
    ])
    act(() => app.state.table.setSorting([{ id: 'usage', desc: false }]))
    expect(
      app.state.table
        .getRowModel()
        .rows.filter((row) => row.original.objectType === 'element')
        .map((row) => row.id),
    ).toEqual(['element:2', 'element:10'])
    app.close()
  })

  it('sorts visible tag counts numerically in both directions and restores URL sorting', () => {
    const app = harness(
      [
        row(1, { tags: [], filterTags: ['group:internal'] }),
        row(2, { tags: ['a', 'b'] }),
        row(3, { tags: Array.from({ length: 10 }, (_, index) => `tag-${index}`) }),
      ],
      '/inventory?sort=tags&dir=desc',
    )
    expect(app.state.table.getRowModel().rows.map((row) => row.id)).toEqual([
      'element:3',
      'element:2',
      'element:1',
    ])
    act(() => app.state.table.getColumn('tags')!.toggleSorting(false))
    expect(app.state.table.getRowModel().rows.map((row) => row.id)).toEqual([
      'element:1',
      'element:2',
      'element:3',
    ])
    expect(app.params.get('sort')).toBe('tags')
    expect(app.params.get('dir')).toBeNull()
    app.close()
  })

  it('separates page selection from all matching rows and retains selection across pages', () => {
    const app = harness(
      Array.from({ length: 60 }, (_, index) => row(index + 1)),
      '/inventory?pageSize=25',
    )
    act(() => app.state.table.toggleAllPageRowsSelected(true))
    expect(app.state.selectedRows).toHaveLength(25)
    act(() => app.state.setPage(1))
    expect(app.state.selectedRows).toHaveLength(25)
    expect(app.state.table.getIsAllPageRowsSelected()).toBe(false)
    act(() => app.state.table.toggleAllRowsSelected(true))
    expect(app.state.selectedRows).toHaveLength(60)
    act(() => app.state.updateParams({ q: 'item 60' }))
    expect(app.state.selectedRows).toHaveLength(60)
    act(() => app.state.table.resetRowSelection())
    act(() => app.state.table.toggleAllRowsSelected(true))
    expect(app.state.selectedRows.map((row) => row.key)).toEqual(['element:60'])
    app.update([row(1)])
    expect(app.state.selectedRows).toHaveLength(0)
    app.close()
  })

  it('switches type-specific columns and remembers independent visibility preferences', () => {
    const app = harness([row(1)], '/inventory?type=elements')
    const visible = () => app.state.table.getVisibleLeafColumns().map((column) => column.id)
    expect(visible()).toEqual(['name', 'type', 'technology', 'tags', 'connectorCount', 'updatedAt'])
    act(() => app.state.table.getColumn('technology')!.toggleVisibility(false))
    act(() => app.state.table.getColumn('repo')!.toggleVisibility(true))
    act(() => app.state.updateParams({ type: 'views' }))
    expect(visible()).toEqual(['name', 'type', 'tags', 'elementCount', 'connectorCount', 'updatedAt'])
    expect(app.state.table.getColumn('type')!.columnDef.header).toBe('Level')
    expect(app.state.table.getAllLeafColumns().some((column) => column.id === 'repo')).toBe(false)
    act(() => app.state.table.getColumn('parentView')!.toggleVisibility(true))
    act(() => app.state.updateParams({ type: 'connectors' }))
    expect(visible()).toEqual(['name', 'source', 'target', 'type', 'direction', 'view', 'tags', 'updatedAt'])
    expect(app.state.table.getColumn('type')!.columnDef.header).toBe('Relationship')
    expect(app.state.table.getColumn('name')!.columnDef.header).toBe('Label')
    act(() => app.state.updateParams({ type: 'elements' }))
    expect(visible()).toContain('repo')
    expect(visible()).not.toContain('technology')
    act(() => app.state.resetColumns())
    expect(visible()).toEqual(['name', 'type', 'technology', 'tags', 'connectorCount', 'updatedAt'])
    act(() => app.state.updateParams({ type: 'views' }))
    expect(visible()).toContain('parentView')
    act(() => app.state.updateParams({ type: '' }))
    expect(visible()).toEqual(['name', 'type', 'tags', 'usage', 'updatedAt'])
    app.close()
  })

  it('clears incompatible sorts and kinds when changing type while retaining search and tags', () => {
    const app = harness(
      [row(1)],
      '/inventory?type=elements&sort=filePath&dir=desc&kind=service&q=item&tags=backend',
    )
    expect(app.state.table.getState().sorting[0].id).toBe('filePath')
    act(() => app.state.updateParams({ type: 'views' }))
    expect(app.params.get('kind')).toBeNull()
    expect(app.params.get('sort')).toBeNull()
    expect(app.params.get('dir')).toBeNull()
    expect(app.params.get('q')).toBe('item')
    expect(app.params.get('tags')).toBe('backend')
    expect(app.state.table.getState().sorting).toEqual([{ id: 'name', desc: false }])
    act(() => app.state.table.setSorting([{ id: 'connectorCount', desc: true }]))
    act(() => app.state.updateParams({ type: 'elements' }))
    expect(app.state.table.getState().sorting).toEqual([{ id: 'connectorCount', desc: true }])
    app.close()
  })

  it('sorts view contents as numbers and resolves old usage sorting to the type-specific field', () => {
    const app = harness(
      [
        row(1, { key: 'view:1', objectType: 'view', viewCounts: { placements: 2, connectors: 12 } }),
        row(2, { key: 'view:2', objectType: 'view', viewCounts: { placements: 10, connectors: 3 } }),
      ],
      '/inventory?type=views&sort=usage&dir=desc',
    )
    expect(app.state.table.getState().sorting).toEqual([{ id: 'elementCount', desc: true }])
    expect(app.state.table.getRowModel().rows.map((row) => row.id)).toEqual(['view:2', 'view:1'])
    act(() => app.state.table.setSorting([{ id: 'connectorCount', desc: true }]))
    expect(app.state.table.getRowModel().rows.map((row) => row.id)).toEqual(['view:1', 'view:2'])
    app.close()
  })

  it('keeps shared quality filters and drops incompatible ones when switching types', () => {
    const app = harness(
      [row(1)],
      '/inventory?type=elements&qualities=missing%20kind,missing%20description,duplicate%20name,has%20child%20view',
    )
    act(() => app.state.updateParams({ type: 'views' }))
    expect(app.params.get('qualities')).toBe('missing description,duplicate name')
    act(() => app.state.updateParams({ type: 'connectors' }))
    expect(app.params.get('qualities')).toBe('missing description')
    act(() => app.state.updateParams({ qualities: 'self connection' }))
    act(() => app.state.updateParams({ type: 'elements' }))
    expect(app.params.get('qualities')).toBeNull()
    app.close()
  })

  it('preserves requested pages while loading, reveals deep links, and clamps after deletions', () => {
    const app = harness([], '/inventory?page=2&pageSize=25', true)
    expect(app.params.get('page')).toBe('2')
    const rows = Array.from({ length: 60 }, (_, index) => row(index + 1))
    app.update(rows)
    expect(app.state.table.getRowModel().rows[0].id).toBe('element:26')
    act(() => app.state.selectRow(rows[59]))
    expect(app.state.table.getState().pagination.pageIndex).toBe(2)
    expect(app.state.selectedRow?.key).toBe('element:60')
    app.update(rows.slice(0, 10))
    expect(app.state.table.getState().pagination.pageIndex).toBe(0)
    expect(app.state.table.getRowModel().rows).toHaveLength(10)
    app.close()
  })
})
