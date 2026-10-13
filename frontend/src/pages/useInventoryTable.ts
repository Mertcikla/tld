import { useDeferredValue, useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import type { RowSelectionState, SortingState, VisibilityState } from '@tanstack/react-table'
import {
  filterInventoryRows,
  getInventoryQualityOptions,
  type InventoryFilters,
  type InventoryRow,
  type InventoryType,
} from './inventoryData'

import { defaultInventoryVisibility, getInventoryColumns } from './inventoryColumns'

export const INVENTORY_PAGE_SIZES = [25, 50, 100, 250]
export function useInventoryTable(rows: InventoryRow[], loading = false) {
  const [params, setParams] = useSearchParams()
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const type = params.get('type')
  const selectedType: InventoryType =
    type === 'elements' || type === 'views' || type === 'connectors' ? type : 'all'
  const columns = useMemo(() => getInventoryColumns(selectedType), [selectedType])
  const [visibilityByType, setVisibilityByType] = useState<Record<InventoryType, VisibilityState>>(() => ({
    all: defaultInventoryVisibility('all'),
    elements: defaultInventoryVisibility('elements'),
    views: defaultInventoryVisibility('views'),
    connectors: defaultInventoryVisibility('connectors'),
  }))
  const query = params.get('q') ?? ''
  const deferredQuery = useDeferredValue(query)
  const filters = useMemo<InventoryFilters>(
    () => ({
      type: selectedType,
      query: deferredQuery,
      tags: params.get('tags')?.split(',').filter(Boolean) ?? [],
      kind: params.get('kind') ?? '',
      qualities: params.get('qualities')?.split(',').filter(Boolean) ?? [],
    }),
    [params, deferredQuery, selectedType],
  )
  const pageSize = INVENTORY_PAGE_SIZES.includes(Number(params.get('pageSize')))
    ? Number(params.get('pageSize'))
    : 100
  const requestedPage = Number(params.get('page'))
  const pageIndex = Number.isInteger(requestedPage) && requestedPage > 0 ? requestedPage - 1 : 0
  const requestedSort = params.get('sort') ?? 'name'
  const sort =
    requestedSort === 'usage' && selectedType !== 'all'
      ? selectedType === 'connectors'
        ? 'view'
        : selectedType === 'views'
          ? 'elementCount'
          : 'connectorCount'
      : requestedSort
  const validSort = columns.some((column) => column.id === sort && column.enableSorting !== false)
  const sorting: SortingState = [
    {
      id: validSort ? sort : 'name',
      desc: validSort && params.get('dir') === 'desc',
    },
  ]
  const updateParams = (values: Record<string, string>, resetPage = true) => {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous)
        Object.entries(values).forEach(([key, value]) => {
          if (value) next.set(key, value)
          else next.delete(key)
        })
        if ('type' in values && (previous.get('type') ?? '') !== values.type) {
          next.delete('kind')
          const availableQualities = getInventoryQualityOptions(
            values.type === 'elements' || values.type === 'views' || values.type === 'connectors'
              ? values.type
              : 'all',
          )
          const retainedQualities = (next.get('qualities')?.split(',') ?? []).filter((value) =>
            availableQualities.some((option) => option.value === value),
          )
          if (retainedQualities.length) next.set('qualities', retainedQualities.join(','))
          else next.delete('qualities')
          const nextType =
            values.type === 'elements' || values.type === 'views' || values.type === 'connectors'
              ? values.type
              : 'all'
          if (!getInventoryColumns(nextType).some((column) => column.id === next.get('sort'))) {
            next.delete('sort')
            next.delete('dir')
          }
        }
        if (resetPage) {
          next.delete('page')
          next.delete('object')
        }
        return next
      },
      { replace: true },
    )
  }
  const table = useReactTable({
    data: rows,
    columns,
    getRowId: (row) => row.key,
    state: {
      globalFilter: filters,
      sorting,
      pagination: { pageIndex, pageSize },
      rowSelection,
      columnVisibility: visibilityByType[selectedType],
    },
    globalFilterFn: (row, _columnId, value: InventoryFilters) =>
      filterInventoryRows([row.original], value).length > 0,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    onRowSelectionChange: setRowSelection,
    onColumnVisibilityChange: (updater) =>
      setVisibilityByType((previous) => ({
        ...previous,
        [selectedType]: typeof updater === 'function' ? updater(previous[selectedType]) : updater,
      })),
    onSortingChange: (updater) => {
      const next = typeof updater === 'function' ? updater(sorting) : updater
      updateParams({ sort: next[0]?.id ?? 'name', dir: next[0]?.desc ? 'desc' : '' })
    },
    columnResizeMode: 'onChange',
    autoResetPageIndex: false,
    enableSortingRemoval: false,
  })
  const filteredRows = table.getPrePaginationRowModel().rows
  const selectedObject = params.get('object')
  const selectedRow =
    filteredRows.find((row) => row.id === selectedObject)?.original ??
    table.getRowModel().rows[0]?.original ??
    null
  // Deep links and inspector navigation reveal the selected row; deleted rows clamp the page.
  useEffect(() => {
    if (loading) return
    const selectedIndex = selectedObject ? filteredRows.findIndex((row) => row.id === selectedObject) : -1
    const targetPage =
      selectedIndex >= 0
        ? Math.floor(selectedIndex / pageSize)
        : Math.min(pageIndex, Math.max(0, Math.ceil(filteredRows.length / pageSize) - 1))
    if (targetPage !== pageIndex) {
      setParams(
        (previous) => {
          const next = new URLSearchParams(previous)
          if (targetPage) next.set('page', String(targetPage + 1))
          else next.delete('page')
          return next
        },
        { replace: true },
      )
    }
  }, [filteredRows, selectedObject, pageIndex, pageSize, setParams, loading])
  useEffect(() => {
    if (loading) return
    const keys = new Set(rows.map((row) => row.key))
    setRowSelection((previous) => {
      const next = Object.fromEntries(
        Object.entries(previous).filter(([key, selected]) => selected && keys.has(key)),
      )
      return Object.keys(next).length === Object.keys(previous).length ? previous : next
    })
  }, [rows, loading])
  const toggleFilter = (key: 'tags' | 'qualities', value: string) => {
    const current = filters[key]
    updateParams({
      [key]: (current.includes(value) ? current.filter((item) => item !== value) : [...current, value]).join(
        ',',
      ),
    })
  }
  return {
    table,
    filters,
    query,
    selectedType,
    selectedRow,
    rowSelection,
    setRowSelection,
    updateParams,
    toggleFilter,
    selectedRows: table.getSelectedRowModel().rows.map((row) => row.original),
    resetColumns: () => table.setColumnVisibility(defaultInventoryVisibility(selectedType)),
    resetFilters: () => updateParams({ q: '', tags: '', kind: '', qualities: '' }),
    selectRow: (row: InventoryRow) => updateParams({ object: row.key }, false),
    setPage: (index: number) => updateParams({ page: index > 0 ? String(index + 1) : '', object: '' }, false),
    setPageSize: (size: number) => updateParams({ pageSize: size === 100 ? '' : String(size) }),
  }
}

export type InventoryTableState = ReturnType<typeof useInventoryTable>
