import type { ImpactReport } from '../api/client'
import type { ExploreData, ExploreViewData, ViewTreeNode } from '../types'
import type { ExploreDiffLens } from './exploreDiffLens'
import type { WatchChangeType } from './watchDiffSummary'

function watchChangeType(change: string): WatchChangeType {
  if (change === 'added') return 'added'
  if (change === 'deleted') return 'deleted'
  return 'updated'
}

export function impactedElementIds(report: ImpactReport): Set<number> {
  const ids = new Set<number>()
  for (const element of [...report.changed, ...report.candidates, ...report.related]) {
    if (element.element_id) ids.add(element.element_id)
  }
  return ids
}

export function impactedConnectorIds(report: ImpactReport): Set<number> {
  const ids = new Set<number>()
  for (const edge of report.edges) {
    if (edge.connector_id) ids.add(edge.connector_id)
  }
  return ids
}

// filterExploreData keeps only the impacted elements/connectors so the canvas
// renders roughly the same subgraph as the Mermaid impact diagram.
export function filterExploreData(data: ExploreData, elementIds: Set<number>, connectorIds: Set<number>): ExploreData {
  const views: Record<string, ExploreViewData> = {}
  const keptViewIds = new Set<number>()
  for (const [key, content] of Object.entries(data.views ?? {})) {
    const placements = (content.placements ?? []).filter((placement) => elementIds.has(placement.element_id))
    const connectors = (content.connectors ?? []).filter(
      (connector) =>
        connectorIds.has(connector.id) ||
        (elementIds.has(connector.source_element_id) && elementIds.has(connector.target_element_id)),
    )
    if (placements.length > 0 || connectors.length > 0) {
      views[key] = { placements, connectors }
      keptViewIds.add(Number(key))
    }
  }

  const byId = new Map<number, ViewTreeNode>()
  const flatten = (nodes: ViewTreeNode[]) => {
    for (const node of nodes) {
      byId.set(node.id, node)
      flatten(node.children ?? [])
    }
  }
  flatten(data.tree ?? [])
  for (const id of Array.from(keptViewIds)) {
    let current = byId.get(id)
    while (current?.parent_view_id != null) {
      keptViewIds.add(current.parent_view_id)
      current = byId.get(current.parent_view_id)
    }
  }
  for (const id of keptViewIds) {
    if (!views[String(id)]) views[String(id)] = { placements: [], connectors: [] }
  }

  const prune = (nodes: ViewTreeNode[]): ViewTreeNode[] =>
    nodes.filter((node) => keptViewIds.has(node.id)).map((node) => ({ ...node, children: prune(node.children ?? []) }))

  const navigations = (data.navigations ?? []).filter(
    (navigation) => keptViewIds.has(navigation.from_view_id) && keptViewIds.has(navigation.to_view_id),
  )

  return { tree: prune(data.tree ?? []), views, navigations }
}

export function buildImpactLens(report: ImpactReport): ExploreDiffLens {
  const elementChanges = new Map<number, WatchChangeType>()
  const connectorChanges = new Map<number, WatchChangeType>()
  const contextElementIds = new Set<number>()

  for (const element of report.changed) {
    if (element.element_id) elementChanges.set(element.element_id, watchChangeType(element.change))
  }
  for (const element of [...report.candidates, ...report.related]) {
    if (element.element_id && !elementChanges.has(element.element_id)) contextElementIds.add(element.element_id)
  }
  for (const edge of report.edges) {
    if (edge.connector_id) connectorChanges.set(edge.connector_id, 'updated')
  }

  return {
    versionId: 0,
    elementChanges,
    connectorChanges,
    elementLineDeltas: new Map(),
    diffDetailsByResource: new Map(),
    orderedTargets: [],
    unplacedTargets: [],
    changedElementIds: new Set(elementChanges.keys()),
    changedConnectorIds: new Set(connectorChanges.keys()),
    ancestorElementIds: new Set(),
    siblingElementIds: new Set(),
    contextElementIds,
    contextConnectorIds: new Set(connectorChanges.keys()),
    totalAddedLines: 0,
    totalRemovedLines: 0,
  }
}
