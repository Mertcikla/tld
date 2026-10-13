import type { Connector, DependencyConnector, LibraryElement, ViewTreeNode } from '../types'
import { isElementGroupTag } from '../utils/elementGroups'
import { isGroundingIgnoreTag } from '../utils/groundingTags'

export type InventoryType = 'all' | 'elements' | 'views' | 'connectors'
export type InventoryObjectType = 'element' | 'view' | 'connector'

interface InventoryQualityOption {
  value: string
  description: string
  types: InventoryObjectType[]
}

const qualityOptions: InventoryQualityOption[] = [
  { value: 'untagged', description: 'No tags assigned.', types: ['element', 'view', 'connector'] },
  { value: 'missing description', description: 'Description is empty or only whitespace.', types: ['element', 'view', 'connector'] },
  { value: 'duplicate name', description: 'Another object of the same type has this name (ignoring case and surrounding spaces).', types: ['element', 'view'] },
  { value: 'has child view', description: 'An element with its own child view.', types: ['element'] },
  { value: 'unused element', description: 'An element with no incoming or outgoing connectors.', types: ['element'] },
  { value: 'missing kind', description: 'Element kind is not set.', types: ['element'] },
  { value: 'missing technology', description: 'Neither technology text nor technology links are set.', types: ['element'] },
  { value: 'empty view', description: 'A view with no placed elements or connectors.', types: ['view'] },
  { value: 'missing level', description: 'View level label is not set.', types: ['view'] },
  { value: 'unconnected view', description: 'Contains placed elements but no connectors.', types: ['view'] },
  { value: 'missing label', description: 'Connector label is empty or only whitespace.', types: ['connector'] },
  { value: 'missing relationship', description: 'Connector relationship type is not set.', types: ['connector'] },
  { value: 'self connection', description: 'Source and target are the same element.', types: ['connector'] },
]

export function getInventoryQualityOptions(type: InventoryType) {
  return qualityOptions.filter(option => type === 'all' || option.types.some(objectType => `${objectType}s` === type))
}

function duplicateNames(items: { name: string }[]) {
  const counts = new Map<string, number>()
  for (const item of items) {
    const name = item.name.trim().toLowerCase()
    if (name) counts.set(name, (counts.get(name) ?? 0) + 1)
  }
  return new Set([...counts].filter(([, count]) => count > 1).map(([name]) => name))
}

export interface InventoryViewContentCounts {
  placements: number
  connectors: number
}

export interface InventoryRow {
  key: string
  objectType: InventoryObjectType
  id: number
  name: string
  subtitle: string
  tags: string[]
  filterTags: string[]
  updatedAt: string
  typeLabel: string
  usageCount?: number
  usageLabel: string
  searchableText: string
  qualityFlags: string[]
  element?: LibraryElement
  view?: ViewTreeNode
  connector?: Connector
  sourceName?: string
  targetName?: string
  viewName?: string
  parentViewName?: string
  ownerElementName?: string
  viewCounts?: InventoryViewContentCounts
}

export interface InventoryFilters {
  type: InventoryType
  query: string
  tags: string[]
  kind: string
  qualities: string[]
}

export function flattenInventoryViews(views: ViewTreeNode[]): ViewTreeNode[] {
  const result: ViewTreeNode[] = []
  const walk = (items: ViewTreeNode[]) => {
    items.forEach((item) => {
      result.push(item)
      walk(item.children ?? [])
    })
  }
  walk(views)
  return result
}

export function dependencyConnectorToConnector(connector: DependencyConnector): Connector {
  return {
    id: Number(connector.id),
    view_id: Number(connector.view_id),
    source_element_id: Number(connector.source_element_id),
    target_element_id: Number(connector.target_element_id),
    label: connector.label ?? null,
    description: connector.description ?? null,
    relationship: connector.relationship_type ?? null,
    direction: connector.direction,
    style: connector.connector_type,
    url: connector.url ?? null,
    source_handle: connector.source_handle ?? null,
    target_handle: connector.target_handle ?? null,
    tags: connector.tags ?? [],
    created_at: connector.created_at,
    updated_at: connector.updated_at,
  }
}

export function buildInventoryRows(
  elements: LibraryElement[],
  views: ViewTreeNode[],
  connectors: Connector[],
  countsByView: Record<number, InventoryViewContentCounts>,
): InventoryRow[] {
  const elementById = new Map(elements.map((element) => [element.id, element]))
  const viewById = new Map(views.map((view) => [view.id, view]))
  const duplicateElementNames = duplicateNames(elements)
  const duplicateViewNames = duplicateNames(views)
  const connectorCountsByElement = new Map<number, number>()
  connectors.forEach((connector) => {
    connectorCountsByElement.set(connector.source_element_id, (connectorCountsByElement.get(connector.source_element_id) ?? 0) + 1)
    connectorCountsByElement.set(connector.target_element_id, (connectorCountsByElement.get(connector.target_element_id) ?? 0) + 1)
  })

  const elementRows = elements.map((element): InventoryRow => {
    const visibleTags = element.tags.filter((tag) => !isElementGroupTag(tag) && !isGroundingIgnoreTag(tag))
    const connectorCount = connectorCountsByElement.get(element.id) ?? 0
    const qualityFlags = [
      ...(visibleTags.length === 0 ? ['untagged'] : []),
      ...(!element.description?.trim() ? ['missing description'] : []),
      ...(!element.kind?.trim() ? ['missing kind'] : []),
      ...(!element.technology?.trim() && !element.technology_connectors.some(link => link.slug?.trim() || link.label?.trim()) ? ['missing technology'] : []),
      ...(duplicateElementNames.has(element.name.trim().toLowerCase()) ? ['duplicate name'] : []),
      ...(connectorCount === 0 ? ['unused element'] : []),
      ...(element.has_view ? ['has child view'] : []),
    ]
    const searchableParts = [
      element.name,
      element.description,
      element.kind,
      element.technology,
      element.url,
      element.repo,
      element.branch,
      element.file_path,
      ...visibleTags,
    ]
    return {
      key: `element:${element.id}`,
      objectType: 'element',
      id: element.id,
      name: element.name,
      subtitle: [element.kind, element.technology].filter(Boolean).join(' / ') || 'element',
      tags: visibleTags,
      filterTags: element.tags,
      updatedAt: element.updated_at,
      typeLabel: element.kind || 'element',
      usageCount: connectorCount,
      usageLabel: `${connectorCount} connectors${element.has_view ? ', child view' : ''}`,
      searchableText: searchableParts.filter(Boolean).join(' ').toLowerCase(),
      qualityFlags,
      element,
    }
  })

  const viewRows = views.map((view): InventoryRow => {
    const counts = countsByView[view.id] ?? { placements: 0, connectors: 0 }
    const qualityFlags = [
      ...((view.tags ?? []).length === 0 ? ['untagged'] : []),
      ...(!view.description?.trim() ? ['missing description'] : []),
      ...(!view.level_label?.trim() ? ['missing level'] : []),
      ...(counts.placements > 0 && counts.connectors === 0 ? ['unconnected view'] : []),
      ...(duplicateViewNames.has(view.name.trim().toLowerCase()) ? ['duplicate name'] : []),
      ...(counts.placements === 0 && counts.connectors === 0 ? ['empty view'] : []),
    ]
    const searchableParts = [
      view.name,
      view.description,
      view.level_label,
      ...(view.tags ?? []),
    ]
    return {
      key: `view:${view.id}`,
      objectType: 'view',
      id: view.id,
      name: view.name,
      subtitle: view.level_label || (view.parent_view_id == null ? 'root view' : 'view'),
      tags: view.tags ?? [],
      filterTags: view.tags ?? [],
      updatedAt: view.updated_at,
      typeLabel: view.level_label || 'view',
      usageCount: counts.placements,
      usageLabel: `${counts.placements} elements, ${counts.connectors} connectors`,
      searchableText: searchableParts.filter(Boolean).join(' ').toLowerCase(),
      qualityFlags,
      view,
      viewCounts: counts,
      parentViewName: view.parent_view_id == null ? '' : viewById.get(view.parent_view_id)?.name ?? `View ${view.parent_view_id}`,
      ownerElementName: view.owner_element_id == null ? '' : elementById.get(view.owner_element_id)?.name ?? `Element ${view.owner_element_id}`,
    }
  })

  const connectorRows = connectors.map((connector): InventoryRow => {
    const sourceName = elementById.get(connector.source_element_id)?.name ?? `Element ${connector.source_element_id}`
    const targetName = elementById.get(connector.target_element_id)?.name ?? `Element ${connector.target_element_id}`
    const viewName = viewById.get(connector.view_id)?.name ?? `View ${connector.view_id}`
    const name = connector.label || ''
    const qualityFlags = [
      ...((connector.tags ?? []).length === 0 ? ['untagged'] : []),
      ...(!connector.description?.trim() ? ['missing description'] : []),
      ...(!connector.label?.trim() ? ['missing label'] : []),
      ...(!connector.relationship?.trim() ? ['missing relationship'] : []),
      ...(connector.source_element_id === connector.target_element_id ? ['self connection'] : []),
    ]
    const searchableParts = [
      name,
      connector.description,
      connector.relationship,
      connector.direction,
      connector.style,
      sourceName,
      targetName,
      viewName,
      ...(connector.tags ?? []),
    ]
    return {
      key: `connector:${connector.id}`,
      objectType: 'connector',
      id: connector.id,
      name,
      subtitle: `${sourceName} -> ${targetName}`,
      tags: connector.tags ?? [],
      filterTags: connector.tags ?? [],
      updatedAt: connector.updated_at,
      typeLabel: connector.relationship || 'connector',
      usageLabel: viewName,
      searchableText: searchableParts.filter(Boolean).join(' ').toLowerCase(),
      qualityFlags,
      connector,
      sourceName,
      targetName,
      viewName,
    }
  })

  return [...elementRows, ...viewRows, ...connectorRows].sort((a, b) => {
    const aTime = Date.parse(a.updatedAt || '')
    const bTime = Date.parse(b.updatedAt || '')
    return (Number.isFinite(bTime) ? bTime : 0) - (Number.isFinite(aTime) ? aTime : 0)
  })
}

export function filterInventoryRows(rows: InventoryRow[], filters: InventoryFilters): InventoryRow[] {
  const query = filters.query.trim().toLowerCase()
  return rows.filter((row) => {
    if (filters.type !== 'all' && `${row.objectType}s` !== filters.type) return false
    if (query && !row.searchableText.includes(query)) return false
    if (filters.tags.length > 0 && !filters.tags.some((tag) => row.filterTags.includes(tag))) return false
    if (filters.kind && row.typeLabel !== filters.kind) return false
    if (filters.qualities.length > 0 && !filters.qualities.some((q) => row.qualityFlags.includes(q))) return false
    return true
  })
}
