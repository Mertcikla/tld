import type { WatchDiff } from '../api/client'
import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../types'
import { normalizeWatchChangeType, type WatchChangeType } from './watchDiffSummary'

export interface ExploreDiffLineDelta {
  added: number
  removed: number
}

export interface ExploreDiffDetail {
  key: string
  resourceType: string
  resourceId?: number
  changeType: WatchChangeType
  summary?: string
  ownerType: string
  ownerKey: string
  language?: string
  addedLines: number
  removedLines: number
  sourcePath?: string
  line?: number
}

export interface ExploreDiffTarget extends ExploreDiffDetail {
  label: string
  viewId?: number
  viewName?: string
  unplaced: boolean
}

export interface ExploreDiffLens {
  versionId: number
  elementChanges: Map<number, WatchChangeType>
  connectorChanges: Map<number, WatchChangeType>
  elementLineDeltas: Map<number, ExploreDiffLineDelta>
  diffDetailsByResource: Map<string, ExploreDiffDetail>
  orderedTargets: ExploreDiffTarget[]
  unplacedTargets: ExploreDiffTarget[]
  changedElementIds: Set<number>
  changedConnectorIds: Set<number>
  ancestorElementIds: Set<number>
  siblingElementIds: Set<number>
  contextElementIds: Set<number>
  contextConnectorIds: Set<number>
  totalAddedLines: number
  totalRemovedLines: number
}

export function diffResourceKey(resourceType: string | null | undefined, resourceId: number | null | undefined): string {
  return `${resourceType || 'resource'}:${resourceId ?? 'unknown'}`
}

function flattenViews(nodes: ViewTreeNode[], out = new Map<number, ViewTreeNode>()): Map<number, ViewTreeNode> {
  nodes.forEach((node) => {
    out.set(node.id, node)
    flattenViews(node.children ?? [], out)
  })
  return out
}

function firstPathLikePart(parts: string[], startIndex: number): string | undefined {
  for (let i = startIndex; i < parts.length; i += 1) {
    const part = parts[i]?.trim()
    if (!part) continue
    if (part.includes('/') || part.includes('\\') || part.includes('.')) return part.replace(/\\/g, '/')
  }
  return undefined
}

export function sourcePathFromDiff(diff: Pick<WatchDiff, 'owner_type' | 'owner_key'>): string | undefined {
  const ownerType = diff.owner_type?.trim()
  const ownerKey = diff.owner_key?.trim()
  if (!ownerKey) return undefined
  if (ownerType === 'file') return ownerKey.replace(/^file:/, '').replace(/\\/g, '/')
  if (ownerKey.startsWith('file:')) return ownerKey.slice('file:'.length).replace(/\\/g, '/')

  const parts = ownerKey.split(':')
  if (ownerType === 'symbol' && parts.length >= 4) {
    return parts[1]?.replace(/\\/g, '/')
  }
  if (ownerKey.startsWith('symbol:') && parts.length >= 5) {
    return parts[2]?.replace(/\\/g, '/')
  }
  return firstPathLikePart(parts, 1)
}

function connectorName(connector: Connector): string {
  return connector.label || connector.relationship || `connector ${connector.id}`
}

function targetBase(diff: WatchDiff): ExploreDiffDetail {
  const addedLines = Math.max(0, diff.added_lines ?? 0)
  const removedLines = Math.max(0, diff.removed_lines ?? 0)
  const detail: ExploreDiffDetail = {
    key: diffResourceKey(diff.resource_type, diff.resource_id),
    resourceType: diff.resource_type || 'resource',
    resourceId: diff.resource_id,
    changeType: normalizeWatchChangeType(diff.change_type),
    summary: diff.summary,
    ownerType: diff.owner_type,
    ownerKey: diff.owner_key,
    language: diff.language,
    addedLines,
    removedLines,
    sourcePath: sourcePathFromDiff(diff),
  }
  return detail
}

export function buildExploreDiffLens(data: ExploreData, diffs: WatchDiff[] | null | undefined, versionId: number): ExploreDiffLens {
  const views = flattenViews(data.tree ?? [])
  const viewByOwnerElement = new Map<number, ViewTreeNode>()
  views.forEach((view) => {
    if (view.owner_element_id != null) viewByOwnerElement.set(view.owner_element_id, view)
  })

  const placementsByElement = new Map<number, PlacedElement[]>()
  const connectorsById = new Map<number, Connector>()
  Object.values(data.views ?? {}).forEach((content) => {
    ;(content.placements ?? []).forEach((placement) => {
      const list = placementsByElement.get(placement.element_id) ?? []
      list.push(placement)
      placementsByElement.set(placement.element_id, list)
    })
    ;(content.connectors ?? []).forEach((connector) => {
      connectorsById.set(connector.id, connector)
    })
  })

  const elementChanges = new Map<number, WatchChangeType>()
  const connectorChanges = new Map<number, WatchChangeType>()
  const elementLineDeltas = new Map<number, ExploreDiffLineDelta>()
  const diffDetailsByResource = new Map<string, ExploreDiffDetail>()
  const orderedTargets: ExploreDiffTarget[] = []
  const unplacedTargets: ExploreDiffTarget[] = []
  const changedElementIds = new Set<number>()
  const changedConnectorIds = new Set<number>()
  const ancestorElementIds = new Set<number>()
  const siblingElementIds = new Set<number>()
  const contextElementIds = new Set<number>()
  const contextConnectorIds = new Set<number>()
  let totalAddedLines = 0
  let totalRemovedLines = 0

  const addContextForViewPath = (viewId: number) => {
    let current = views.get(viewId)
    while (current) {
      const content = data.views[String(current.id)]
      ;(content?.placements ?? []).forEach((placement) => {
        siblingElementIds.add(placement.element_id)
        contextElementIds.add(placement.element_id)
      })
      if (current.owner_element_id != null) {
        ancestorElementIds.add(current.owner_element_id)
        contextElementIds.add(current.owner_element_id)
      }
      current = current.parent_view_id != null ? views.get(current.parent_view_id) : undefined
    }
  }

  ;(Array.isArray(diffs) ? diffs : []).forEach((diff) => {
    const detail = targetBase(diff)
    totalAddedLines += detail.addedLines
    totalRemovedLines += detail.removedLines
    diffDetailsByResource.set(detail.key, detail)

    if (diff.resource_type === 'element' && diff.resource_id) {
      elementChanges.set(diff.resource_id, detail.changeType)
      changedElementIds.add(diff.resource_id)
      if (detail.addedLines > 0 || detail.removedLines > 0) {
        elementLineDeltas.set(diff.resource_id, { added: detail.addedLines, removed: detail.removedLines })
      }

      const placements = placementsByElement.get(diff.resource_id) ?? []
      if (placements.length === 0) {
        const ownedView = viewByOwnerElement.get(diff.resource_id)
        unplacedTargets.push({
          ...detail,
          label: detail.summary || `element ${diff.resource_id}`,
          viewId: ownedView?.id,
          viewName: ownedView?.name,
          unplaced: true,
        })
      } else {
        placements.forEach((placement) => {
          const view = views.get(placement.view_id)
          addContextForViewPath(placement.view_id)
          orderedTargets.push({
            ...detail,
            key: `${detail.key}:view:${placement.view_id}`,
            label: placement.name || detail.summary || `element ${diff.resource_id}`,
            viewId: placement.view_id,
            viewName: view?.name ?? `View ${placement.view_id}`,
            unplaced: false,
          })
        })
      }
    } else if (diff.resource_type === 'connector' && diff.resource_id) {
      connectorChanges.set(diff.resource_id, detail.changeType)
      changedConnectorIds.add(diff.resource_id)
      const connector = connectorsById.get(diff.resource_id)
      if (connector) {
        addContextForViewPath(connector.view_id)
        contextElementIds.add(connector.source_element_id)
        contextElementIds.add(connector.target_element_id)
        orderedTargets.push({
          ...detail,
          label: connectorName(connector),
          viewId: connector.view_id,
          viewName: views.get(connector.view_id)?.name ?? `View ${connector.view_id}`,
          unplaced: false,
        })
      } else {
        unplacedTargets.push({
          ...detail,
          label: detail.summary || `connector ${diff.resource_id}`,
          unplaced: true,
        })
      }
    } else if (detail.sourcePath || diff.resource_type === 'file' || diff.owner_type === 'file') {
      unplacedTargets.push({
        ...detail,
        label: detail.summary || detail.sourcePath || detail.ownerKey,
        unplaced: true,
      })
    }
  })

  Object.values(data.views ?? {}).forEach((content) => {
    ;(content.connectors ?? []).forEach((connector) => {
      if (changedConnectorIds.has(connector.id)) return
      if (contextElementIds.has(connector.source_element_id) || contextElementIds.has(connector.target_element_id)) {
        contextConnectorIds.add(connector.id)
      }
    })
  })

  changedElementIds.forEach((id) => {
    contextElementIds.delete(id)
    siblingElementIds.delete(id)
  })

  return {
    versionId,
    elementChanges,
    connectorChanges,
    elementLineDeltas,
    diffDetailsByResource,
    orderedTargets,
    unplacedTargets,
    changedElementIds,
    changedConnectorIds,
    ancestorElementIds,
    siblingElementIds,
    contextElementIds,
    contextConnectorIds,
    totalAddedLines,
    totalRemovedLines,
  }
}
