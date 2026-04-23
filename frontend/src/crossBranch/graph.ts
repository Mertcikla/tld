import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../types'
import type { GraphPlacementRef, WorkspaceGraphSnapshot } from './types'

function cloneViewTree(nodes: ViewTreeNode[]): ViewTreeNode[] {
  return nodes.map((node) => ({
    ...node,
    children: cloneViewTree(node.children ?? []),
  }))
}

function flattenTree(nodes: ViewTreeNode[], out: ViewTreeNode[] = []): ViewTreeNode[] {
  for (const node of nodes) {
    out.push(node)
    flattenTree(node.children ?? [], out)
  }
  return out
}

function collectDescendants(viewById: Record<number, ViewTreeNode>, parentId: number): number[] {
  const result: number[] = []
  const visit = (viewId: number) => {
    result.push(viewId)
    const node = viewById[viewId]
    for (const child of node?.children ?? []) visit(child.id)
  }
  visit(parentId)
  return result
}

function viewNameOf(viewById: Record<number, ViewTreeNode>, viewId: number): string {
  return viewById[viewId]?.name ?? `View ${viewId}`
}

function deepCloneExploreData(data: ExploreData): ExploreData {
  return {
    tree: cloneViewTree(data.tree ?? []),
    navigations: [...(data.navigations ?? [])],
    views: Object.fromEntries(
      Object.entries(data.views ?? {}).map(([key, value]) => [
        key,
        {
          placements: [...(value.placements ?? [])],
          connectors: [...(value.connectors ?? [])],
        },
      ]),
    ),
  }
}

export function buildWorkspaceGraphSnapshot(data: ExploreData): WorkspaceGraphSnapshot {
  const tree = cloneViewTree(data.tree ?? [])
  const flatTree = flattenTree(tree)
  const viewById = Object.fromEntries(flatTree.map((view) => [view.id, view])) as Record<number, ViewTreeNode>

  const placementsByViewId: Record<number, PlacedElement[]> = {}
  const connectorsByViewId: Record<number, Connector[]> = {}
  const placementsByElementId: Record<number, GraphPlacementRef[]> = {}

  for (const view of flatTree) {
    const content = data.views?.[String(view.id)]
    const placements = [...(content?.placements ?? [])]
    const connectors = [...(content?.connectors ?? [])]
    placementsByViewId[view.id] = placements
    connectorsByViewId[view.id] = connectors

    for (const placement of placements) {
      placementsByElementId[placement.element_id] ??= []
      placementsByElementId[placement.element_id].push({
        viewId: view.id,
        viewName: view.name,
        element: placement,
      })
    }
  }

  const childViewIdByOwnerElementId = flatTree.reduce<Record<number, number>>((acc, view) => {
    if (view.owner_element_id != null) acc[view.owner_element_id] = view.id
    return acc
  }, {})

  const ancestorsByViewId = flatTree.reduce<Record<number, number[]>>((acc, view) => {
    const lineage: number[] = []
    let cursor: ViewTreeNode | undefined = view
    while (cursor) {
      lineage.push(cursor.id)
      cursor = cursor.parent_view_id != null ? viewById[cursor.parent_view_id] : undefined
    }
    acc[view.id] = lineage.reverse()
    return acc
  }, {})

  const descendantsByViewId = flatTree.reduce<Record<number, number[]>>((acc, view) => {
    acc[view.id] = collectDescendants(viewById, view.id)
    return acc
  }, {})

  const views = flatTree.reduce<Record<number, { view: ViewTreeNode; placements: PlacedElement[]; connectors: Connector[] }>>((acc, view) => {
    acc[view.id] = {
      view,
      placements: placementsByViewId[view.id] ?? [],
      connectors: connectorsByViewId[view.id] ?? [],
    }
    return acc
  }, {})

  return {
    source: deepCloneExploreData(data),
    tree,
    views,
    viewById,
    placementsByViewId,
    connectorsByViewId,
    placementsByElementId,
    childViewIdByOwnerElementId,
    descendantsByViewId,
    ancestorsByViewId,
  }
}

export function isDescendantView(snapshot: WorkspaceGraphSnapshot, maybeDescendantId: number | null | undefined, ancestorId: number | null | undefined): boolean {
  if (maybeDescendantId == null || ancestorId == null) return false
  return snapshot.ancestorsByViewId[maybeDescendantId]?.includes(ancestorId) ?? false
}

export function findLowestCommonAncestorViewId(snapshot: WorkspaceGraphSnapshot, leftViewId: number | null | undefined, rightViewId: number | null | undefined): number | null {
  if (leftViewId == null || rightViewId == null) return null
  const leftAncestors = snapshot.ancestorsByViewId[leftViewId] ?? []
  const rightAncestors = new Set(snapshot.ancestorsByViewId[rightViewId] ?? [])
  let best: number | null = null
  for (const candidate of leftAncestors) {
    if (rightAncestors.has(candidate)) best = candidate
  }
  return best
}

export function relativeOwnerElementPath(snapshot: WorkspaceGraphSnapshot, ancestorViewId: number, descendantViewId: number): number[] {
  const descendantAncestors = snapshot.ancestorsByViewId[descendantViewId] ?? []
  const ancestorIndex = descendantAncestors.indexOf(ancestorViewId)
  if (ancestorIndex === -1) return []
  const relativeViewIds = descendantAncestors.slice(ancestorIndex + 1)
  return relativeViewIds
    .map((viewId) => snapshot.viewById[viewId]?.owner_element_id ?? null)
    .filter((elementId): elementId is number => elementId != null)
}

export function viewName(snapshot: WorkspaceGraphSnapshot, viewId: number | null | undefined): string | null {
  if (viewId == null) return null
  return viewNameOf(snapshot.viewById, viewId)
}

export function allConnectors(snapshot: WorkspaceGraphSnapshot): Connector[] {
  return Object.values(snapshot.connectorsByViewId).flat()
}

export function upsertConnectorInSnapshot(snapshot: WorkspaceGraphSnapshot | null, connector: Connector): WorkspaceGraphSnapshot | null {
  if (!snapshot) return snapshot
  const next = deepCloneExploreData(snapshot.source)
  const viewKey = String(connector.view_id)
  next.views[viewKey] ??= { placements: [], connectors: [] }
  next.views[viewKey].connectors = [
    connector,
    ...next.views[viewKey].connectors.filter((existing) => existing.id !== connector.id),
  ]
  return buildWorkspaceGraphSnapshot(next)
}

export function removeConnectorFromSnapshot(snapshot: WorkspaceGraphSnapshot | null, viewId: number, connectorId: number): WorkspaceGraphSnapshot | null {
  if (!snapshot) return snapshot
  const next = deepCloneExploreData(snapshot.source)
  const viewKey = String(viewId)
  if (next.views[viewKey]) {
    next.views[viewKey].connectors = next.views[viewKey].connectors.filter((connector) => connector.id !== connectorId)
  }
  return buildWorkspaceGraphSnapshot(next)
}

export function upsertPlacementInSnapshot(snapshot: WorkspaceGraphSnapshot | null, viewId: number, placement: PlacedElement): WorkspaceGraphSnapshot | null {
  if (!snapshot) return snapshot
  const next = deepCloneExploreData(snapshot.source)
  const viewKey = String(viewId)
  next.views[viewKey] ??= { placements: [], connectors: [] }
  next.views[viewKey].placements = [
    placement,
    ...next.views[viewKey].placements.filter((existing) => existing.element_id !== placement.element_id),
  ]
  return buildWorkspaceGraphSnapshot(next)
}

export function removePlacementFromSnapshot(snapshot: WorkspaceGraphSnapshot | null, viewId: number, elementId: number): WorkspaceGraphSnapshot | null {
  if (!snapshot) return snapshot
  const next = deepCloneExploreData(snapshot.source)
  const viewKey = String(viewId)
  if (next.views[viewKey]) {
    next.views[viewKey].placements = next.views[viewKey].placements.filter((placement) => placement.element_id !== elementId)
  }
  return buildWorkspaceGraphSnapshot(next)
}

export function overrideViewContentInSnapshot(snapshot: WorkspaceGraphSnapshot | null, viewId: number, placements: PlacedElement[], connectors: Connector[]): WorkspaceGraphSnapshot | null {
  if (!snapshot) return snapshot
  const next = deepCloneExploreData(snapshot.source)
  next.views[String(viewId)] = {
    placements: [...placements],
    connectors: [...connectors],
  }
  return buildWorkspaceGraphSnapshot(next)
}
