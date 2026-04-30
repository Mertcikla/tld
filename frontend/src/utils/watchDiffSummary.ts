import type { Connector, ExploreData, ViewTreeNode } from '../types'
import type { WatchDiff } from '../api/client'

export type WatchChangeType = 'added' | 'updated' | 'deleted' | 'changed'

export interface WatchResourceStat {
  added: number
  updated: number
  deleted: number
  changed: number
  addedLines: number
  removedLines: number
}

export interface WatchDiffLocation {
  key: string
  label: string
  resourceType: string
  resourceId?: number
  changeType: WatchChangeType
  summary?: string
  addedLines: number
  removedLines: number
  viewId: number
  viewName: string
}

export interface WatchDiffSummary {
  files: WatchResourceStat
  elements: WatchResourceStat
  connectors: WatchResourceStat
}

export function normalizeWatchChangeType(value: string): WatchChangeType {
  if (value === 'added' || value === 'updated' || value === 'deleted') return value
  return 'changed'
}

export function emptyWatchResourceStat(): WatchResourceStat {
  return { added: 0, updated: 0, deleted: 0, changed: 0, addedLines: 0, removedLines: 0 }
}

export function summarizeWatchDiffs(diffs: WatchDiff[] | null | undefined): WatchDiffSummary {
  const summary = {
    files: emptyWatchResourceStat(),
    elements: emptyWatchResourceStat(),
    connectors: emptyWatchResourceStat(),
  }
  ;(Array.isArray(diffs) ? diffs : []).forEach((diff) => {
    const bucket =
      diff.resource_type === 'file' || diff.owner_type === 'file'
        ? summary.files
        : diff.resource_type === 'element'
          ? summary.elements
          : diff.resource_type === 'connector'
            ? summary.connectors
            : null
    if (!bucket) return
    bucket[normalizeWatchChangeType(diff.change_type)] += 1
    bucket.addedLines += Math.max(0, diff.added_lines ?? 0)
    bucket.removedLines += Math.max(0, diff.removed_lines ?? 0)
  })
  return summary
}

export function formatStatLine(label: string, stat: WatchResourceStat): string {
  const total = stat.added + stat.updated + stat.deleted + stat.changed
  const parts = [`${total} ${label}${total === 1 ? '' : 's'} changed`]
  if (stat.addedLines > 0) parts.push(`+${stat.addedLines}`)
  if (stat.removedLines > 0) parts.push(`-${stat.removedLines}`)
  return parts.join(', ')
}

export function formatTldStatLine(summary: WatchDiffSummary): string {
  return [
    formatStatLine('element', summary.elements),
    formatStatLine('connector', summary.connectors),
  ].join(' · ')
}

function flattenViews(nodes: ViewTreeNode[], out = new Map<number, ViewTreeNode>()): Map<number, ViewTreeNode> {
  nodes.forEach((node) => {
    out.set(node.id, node)
    flattenViews(node.children ?? [], out)
  })
  return out
}

function connectorName(connector: Connector): string {
  return connector.label || connector.relationship || `connector ${connector.id}`
}

export function buildWatchDiffLocations(data: ExploreData, diffs: WatchDiff[] | null | undefined): WatchDiffLocation[] {
  const views = flattenViews(data.tree ?? [])
  const elementViews = new Map<number, WatchDiffLocation[]>()
  const connectorViews = new Map<number, WatchDiffLocation>()

  Object.entries(data.views ?? {}).forEach(([viewIdText, content]) => {
    const viewId = Number(viewIdText)
    if (!Number.isFinite(viewId)) return
    const viewName = views.get(viewId)?.name ?? `View ${viewId}`
    ;(content.placements ?? []).forEach((placement) => {
      const list = elementViews.get(placement.element_id) ?? []
      list.push({
        key: `element:${placement.element_id}:${viewId}`,
        label: placement.name || `element ${placement.element_id}`,
        resourceType: 'element',
        resourceId: placement.element_id,
        changeType: 'changed',
        addedLines: 0,
        removedLines: 0,
        viewId,
        viewName,
      })
      elementViews.set(placement.element_id, list)
    })
    ;(content.connectors ?? []).forEach((connector) => {
      connectorViews.set(connector.id, {
        key: `connector:${connector.id}:${viewId}`,
        label: connectorName(connector),
        resourceType: 'connector',
        resourceId: connector.id,
        changeType: 'changed',
        addedLines: 0,
        removedLines: 0,
        viewId,
        viewName,
      })
    })
  })

  const locations: WatchDiffLocation[] = []
  ;(Array.isArray(diffs) ? diffs : []).forEach((diff) => {
    if (!diff.resource_id) return
    const base = {
      changeType: normalizeWatchChangeType(diff.change_type),
      summary: diff.summary,
      addedLines: Math.max(0, diff.added_lines ?? 0),
      removedLines: Math.max(0, diff.removed_lines ?? 0),
    }
    if (diff.resource_type === 'element') {
      ;(elementViews.get(diff.resource_id) ?? []).forEach((location) => {
        locations.push({ ...location, ...base })
      })
    }
    if (diff.resource_type === 'connector') {
      const location = connectorViews.get(diff.resource_id)
      if (location) locations.push({ ...location, ...base })
    }
  })

  const seen = new Set<string>()
  return locations.filter((location) => {
    const key = `${location.resourceType}:${location.resourceId}:${location.viewId}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}
