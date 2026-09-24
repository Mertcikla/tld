import type { Connector, DependencyConnector, LibraryElement, ViewTreeNode } from '../types'
import { isElementGroupTag } from '../utils/elementGroups'

export type InventoryType = 'all' | 'elements' | 'views' | 'connectors'
export type InventoryObjectType = 'element' | 'view' | 'connector'

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
  updatedAt: string
  typeLabel: string
  usageLabel: string
  searchableText: string
  qualityFlags: string[]
  element?: LibraryElement
  view?: ViewTreeNode
  connector?: Connector
  sourceName?: string
  targetName?: string
  viewName?: string
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
  const connectorCountsByElement = new Map<number, number>()
  connectors.forEach((connector) => {
    connectorCountsByElement.set(connector.source_element_id, (connectorCountsByElement.get(connector.source_element_id) ?? 0) + 1)
    connectorCountsByElement.set(connector.target_element_id, (connectorCountsByElement.get(connector.target_element_id) ?? 0) + 1)
  })

  const elementRows = elements.map((element): InventoryRow => {
    const visibleTags = element.tags.filter((tag) => !isElementGroupTag(tag))
    const connectorCount = connectorCountsByElement.get(element.id) ?? 0
    const qualityFlags = [
      ...(visibleTags.length === 0 ? ['untagged'] : []),
      ...(!element.description ? ['missing description'] : []),
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
      updatedAt: element.updated_at,
      typeLabel: element.kind || 'element',
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
      ...(!view.description ? ['missing description'] : []),
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
      updatedAt: view.updated_at,
      typeLabel: view.level_label || 'view',
      usageLabel: `${counts.placements} elements, ${counts.connectors} connectors`,
      searchableText: searchableParts.filter(Boolean).join(' ').toLowerCase(),
      qualityFlags,
      view,
    }
  })

  const connectorRows = connectors.map((connector): InventoryRow => {
    const sourceName = elementById.get(connector.source_element_id)?.name ?? `Element ${connector.source_element_id}`
    const targetName = elementById.get(connector.target_element_id)?.name ?? `Element ${connector.target_element_id}`
    const viewName = viewById.get(connector.view_id)?.name ?? `View ${connector.view_id}`
    const name = connector.label || `${sourceName} -> ${targetName}`
    const qualityFlags = [
      ...((connector.tags ?? []).length === 0 ? ['untagged'] : []),
      ...(!connector.description ? ['missing description'] : []),
      ...(!connector.label ? ['missing label'] : []),
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
      updatedAt: connector.updated_at,
      typeLabel: connector.relationship || connector.direction || 'connector',
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
    if (filters.tags.length > 0 && !filters.tags.some((tag) => row.tags.includes(tag))) return false
    if (filters.kind && row.typeLabel !== filters.kind) return false
    if (filters.qualities.length > 0 && !filters.qualities.some((q) => row.qualityFlags.includes(q))) return false
    return true
  })
}
