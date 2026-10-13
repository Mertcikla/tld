import type { ColumnDef, VisibilityState } from '@tanstack/react-table'
import type { InventoryRow, InventoryType } from './inventoryData'

type InventoryColumn = ColumnDef<InventoryRow> & { id: string; header: string }

const shared: InventoryColumn[] = [
  {
    id: 'name',
    accessorFn: (row) => row.name || row.subtitle,
    header: 'Name',
    size: 300,
    minSize: 200,
    enableHiding: false,
  },
  { id: 'type', accessorKey: 'typeLabel', header: 'Kind', size: 140, minSize: 100 },
  {
    id: 'tags',
    accessorFn: (row) => row.tags.length,
    header: 'Tags',
    sortingFn: 'basic',
    size: 160,
    minSize: 100,
  },
  {
    id: 'usage',
    accessorKey: 'usageLabel',
    header: 'Usage',
    size: 160,
    minSize: 100,
    sortingFn: (a, b) => {
      const typeOrder = a.original.objectType.localeCompare(b.original.objectType)
      if (typeOrder) return typeOrder
      if (a.original.objectType === 'connector')
        return a.original.usageLabel.localeCompare(b.original.usageLabel, undefined, { numeric: true })
      return (a.original.usageCount ?? 0) - (b.original.usageCount ?? 0)
    },
  },
  { id: 'objectType', accessorKey: 'objectType', header: 'Object type', size: 120 },
  {
    id: 'description',
    accessorFn: (row) =>
      row.element?.description ?? row.view?.description ?? row.connector?.description ?? '',
    header: 'Description',
    size: 280,
    minSize: 150,
  },
  {
    id: 'url',
    accessorFn: (row) => row.element?.url ?? row.connector?.url ?? '',
    header: 'URL',
    size: 240,
    minSize: 120,
  },
  {
    id: 'createdAt',
    accessorFn: (row) => Date.parse(inventoryCreatedAt(row)) || 0,
    header: 'Created',
    sortingFn: 'basic',
    size: 110,
    minSize: 90,
  },
  {
    id: 'updatedAt',
    accessorFn: (row) => Date.parse(row.updatedAt) || 0,
    header: 'Updated',
    sortingFn: 'basic',
    size: 110,
    minSize: 90,
  },
  { id: 'id', accessorKey: 'id', header: 'ID', sortingFn: 'basic', size: 90, minSize: 70 },
]

const elements: InventoryColumn[] = [
  { id: 'technology', accessorFn: (row) => row.element?.technology ?? '', header: 'Technology', size: 150 },
  {
    id: 'connectorCount',
    accessorFn: (row) => row.usageCount ?? 0,
    header: 'Connections',
    sortingFn: 'basic',
    size: 120,
  },
  { id: 'repo', accessorFn: (row) => row.element?.repo ?? '', header: 'Repository', size: 200 },
  { id: 'branch', accessorFn: (row) => row.element?.branch ?? '', header: 'Branch', size: 130 },
  { id: 'filePath', accessorFn: (row) => row.element?.file_path ?? '', header: 'File path', size: 260 },
  { id: 'language', accessorFn: (row) => row.element?.language ?? '', header: 'Language', size: 120 },
  {
    id: 'hasChildView',
    accessorFn: (row) => !!row.element?.has_view,
    header: 'Has child view',
    sortingFn: 'basic',
    size: 140,
  },
  {
    id: 'childViewLabel',
    accessorFn: (row) => row.element?.view_label ?? '',
    header: 'Child view label',
    size: 160,
  },
  {
    id: 'bypassNoiseGate',
    accessorFn: (row) => !!row.element?.bypass_noise_gate,
    header: 'Always visible',
    sortingFn: 'basic',
    size: 140,
  },
  {
    id: 'technologyLinks',
    accessorFn: (row) =>
      (row.element?.technology_connectors ?? [])
        .map((link) => link.label || link.slug || '')
        .filter(Boolean)
        .join(', '),
    header: 'Technology links',
    size: 220,
  },
  { id: 'logoUrl', accessorFn: (row) => row.element?.logo_url ?? '', header: 'Logo URL', size: 240 },
  {
    id: 'repositoryId',
    accessorFn: (row) => row.element?.repository_id ?? '',
    header: 'Repository ID',
    size: 200,
  },
]

const connectors: InventoryColumn[] = [
  { id: 'source', accessorKey: 'sourceName', header: 'Source', size: 200, minSize: 140 },
  { id: 'target', accessorKey: 'targetName', header: 'Target', size: 200, minSize: 140 },
  { id: 'direction', accessorFn: (row) => row.connector?.direction ?? '', header: 'Direction', size: 120 },
  { id: 'style', accessorFn: (row) => row.connector?.style ?? '', header: 'Style', size: 140 },
  { id: 'view', accessorKey: 'viewName', header: 'View', size: 180, minSize: 120 },
  {
    id: 'sourceHandle',
    accessorFn: (row) => row.connector?.source_handle ?? '',
    header: 'Source handle',
    size: 150,
  },
  {
    id: 'targetHandle',
    accessorFn: (row) => row.connector?.target_handle ?? '',
    header: 'Target handle',
    size: 150,
  },
  {
    id: 'sourceId',
    accessorFn: (row) => row.connector?.source_element_id ?? 0,
    header: 'Source ID',
    sortingFn: 'basic',
    size: 110,
  },
  {
    id: 'targetId',
    accessorFn: (row) => row.connector?.target_element_id ?? 0,
    header: 'Target ID',
    sortingFn: 'basic',
    size: 110,
  },
  {
    id: 'viewId',
    accessorFn: (row) => row.connector?.view_id ?? 0,
    header: 'View ID',
    sortingFn: 'basic',
    size: 100,
  },
]

const views: InventoryColumn[] = [
  {
    id: 'elementCount',
    accessorFn: (row) => row.viewCounts?.placements ?? 0,
    header: 'Elements',
    sortingFn: 'basic',
    size: 110,
  },
  {
    id: 'connectorCount',
    accessorFn: (row) => row.viewCounts?.connectors ?? 0,
    header: 'Connectors',
    sortingFn: 'basic',
    size: 120,
  },
  { id: 'parentView', accessorKey: 'parentViewName', header: 'Parent view', size: 200 },
  { id: 'ownerElement', accessorKey: 'ownerElementName', header: 'Owner element', size: 200 },
  {
    id: 'levelNumber',
    accessorFn: (row) => row.view?.level ?? 0,
    header: 'Level number',
    sortingFn: 'basic',
    size: 130,
  },
  { id: 'depth', accessorFn: (row) => row.view?.depth ?? 0, header: 'Depth', sortingFn: 'basic', size: 100 },
  {
    id: 'childViewCount',
    accessorFn: (row) => row.view?.children.length ?? 0,
    header: 'Child views',
    sortingFn: 'basic',
    size: 120,
  },
  {
    id: 'parentViewId',
    accessorFn: (row) => row.view?.parent_view_id ?? null,
    header: 'Parent view ID',
    sortingFn: 'basic',
    size: 140,
  },
  {
    id: 'ownerElementId',
    accessorFn: (row) => row.view?.owner_element_id ?? null,
    header: 'Owner element ID',
    sortingFn: 'basic',
    size: 150,
  },
]

export const INVENTORY_DEFAULT_COLUMNS: Record<InventoryType, string[]> = {
  all: ['name', 'type', 'tags', 'usage', 'updatedAt'],
  elements: ['name', 'type', 'technology', 'tags', 'connectorCount', 'updatedAt'],
  views: ['name', 'type', 'tags', 'elementCount', 'connectorCount', 'updatedAt'],
  connectors: ['name', 'source', 'target', 'type', 'direction', 'view', 'tags', 'updatedAt'],
}

export function getInventoryColumns(type: InventoryType): InventoryColumn[] {
  const common = shared
    .filter(
      (column) =>
        (column.id !== 'usage' || type === 'all') &&
        (column.id !== 'objectType' || type === 'all') &&
        (column.id !== 'url' || type !== 'views'),
    )
    .map((column) => {
      if (column.id === 'type')
        return {
          ...column,
          header: inventoryKindLabel(type),
          accessorFn: (row: InventoryRow) =>
            type === 'connectors'
              ? (row.connector?.relationship ?? '')
              : type === 'views'
                ? (row.view?.level_label ?? '')
                : type === 'elements'
                  ? (row.element?.kind ?? '')
                  : row.typeLabel,
        }
      if (column.id === 'name' && type === 'connectors')
        return { ...column, header: 'Label', size: 240, accessorFn: (row: InventoryRow) => row.name }
      return column
    })
  const available = [
    ...common,
    ...(type === 'elements' ? elements : type === 'views' ? views : type === 'connectors' ? connectors : []),
  ]
  const defaults = INVENTORY_DEFAULT_COLUMNS[type]
  return [
    ...defaults.map((id) => available.find((column) => column.id === id)!),
    ...available.filter((column) => !defaults.includes(column.id)),
  ].map((column) => ({ ...column, enableGlobalFilter: column.id === 'name' }))
}

export function defaultInventoryVisibility(type: InventoryType): VisibilityState {
  return Object.fromEntries(
    getInventoryColumns(type).map((column) => [
      column.id,
      INVENTORY_DEFAULT_COLUMNS[type].includes(column.id),
    ]),
  )
}

export function inventoryKindLabel(type: InventoryType) {
  return type === 'views' ? 'Level' : type === 'connectors' ? 'Relationship' : 'Kind'
}

export function inventoryCreatedAt(row: InventoryRow) {
  return row.element?.created_at ?? row.view?.created_at ?? row.connector?.created_at ?? ''
}
