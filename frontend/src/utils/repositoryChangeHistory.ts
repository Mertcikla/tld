import type { RepositoryImpactScene } from '../api/client'
import type { ViewTreeNode } from '../types'

export type MapChangeKind = 'element' | 'view' | 'connector'
export type MapChangeAction = 'added' | 'modified' | 'removed'
export type MapChangeTarget = {
  key: string
  kind: MapChangeKind
  viewId: number
  elementIds: Array<{ viewId: number; elementId: number }>
  ancestorViewIds: number[]
}
export type MapChange = {
  key: string
  kind: MapChangeKind
  name: string
  path: string
  action: MapChangeAction
  target: MapChangeTarget
}
export type MapChangeGroup = {
  path: string
  changes: MapChange[]
  counts: Record<MapChangeKind, number>
  updates: number
  updatedAt: number
}
export const CHANGE_HISTORY_FILES = 40

type Resource = Omit<MapChange, 'action' | 'path'> & { paths: string[]; signature: string; initialAction?: MapChangeAction }
const unique = <T,>(items: T[]) => [...new Set(items)].sort()
const normalizedPath = (path: string) => path.replace(/^\.\//, '').replace(/\/$/, '')
const matchesPath = (scope: string, path: string) => scope === path || (!!scope && path.startsWith(`${scope}/`))
const changeAction = (value: string | undefined): MapChangeAction | undefined => value === 'added' || value === 'removed' || value === 'modified' ? value : undefined
const fileSignatures = (scene: RepositoryImpactScene | null) => new Map((scene?.files ?? []).map((file) => [file.path, JSON.stringify([file.baseHash, file.headHash, file.change, file.linesAdded, file.linesRemoved, file.binary, file.metadataOnly])]))

/** Compare semantic scene content. Layout, timestamps, provenance diagnostics,
 * and version-only pushes must never appear as code changes. */
function resources(scene: RepositoryImpactScene): Map<string, Resource> {
  const result = new Map<string, Resource>()
  const views = new Map<number, ViewTreeNode>()
  const visit = (nodes: ViewTreeNode[]) => { for (const node of nodes) { views.set(node.id, node); visit(node.children ?? []) } }
  visit(scene.tree)
  const ancestors = (viewId: number) => {
    const ids: number[] = []
    let parent = views.get(viewId)?.parent_view_id
    while (parent != null && !ids.includes(parent)) { ids.push(parent); parent = views.get(parent)?.parent_view_id }
    return ids
  }
  const pathsByView = new Map<number, string[]>()
  const files = fileSignatures(scene)
  // Index file and directory scopes once; large scenes must not scan the full
  // file manifest for every materialized placement.
  const hashesByScope = new Map<string, Array<[string, string]>>()
  for (const [path, signature] of files) {
    const segments = path.split('/')
    while (segments.length) {
      const scope = segments.join('/')
      const entries = hashesByScope.get(scope) ?? []
      entries.push([path, signature])
      hashesByScope.set(scope, entries)
      segments.pop()
    }
  }
  for (const [id, content] of Object.entries(scene.views)) {
    const viewId = Number(id)
    const viewPaths: string[] = []
    for (const element of content.placements) {
      const placementKey = `placement|${id}|${element.element_id}`
      const metadata = scene.placements?.[placementKey]
      const overlay = scene.overlays[element.element_id]
      const paths = unique([...(metadata?.paths ?? []), element.file_path, overlay?.path].filter((path): path is string => !!path).map(normalizedPath))
      viewPaths.push(...paths)
      const key = `element|${element.element_id}`
      const initialAction = changeAction(overlay?.change)
      const hashes = [...new Map(paths.flatMap((path) => hashesByScope.get(path) ?? []))].sort(([a], [b]) => a.localeCompare(b))
      // Overlay totals are a fallback for older scenes with no hash manifest.
      const signature = JSON.stringify([element.name, element.description, element.kind, element.technology, element.url, element.language, paths, unique(element.tags ?? []), hashes, !scene.files && overlay ? [overlay.change, overlay.linesAdded, overlay.linesRemoved, overlay.symbols] : null])
      const previous = result.get(key)
      const target = { key: placementKey, kind: 'element' as const, viewId, elementIds: [{ viewId, elementId: element.element_id }], ancestorViewIds: ancestors(viewId) }
      // A resource placed in several views remains one change in the log.
      if (!previous || (initialAction && !previous.initialAction)) result.set(key, { key, kind: 'element', name: element.name, paths, signature, initialAction, target })
    }
    pathsByView.set(viewId, unique(viewPaths))
    for (const connector of content.connectors) {
      const key = `connector|${viewId}|${connector.id}`
      const source = result.get(`element|${connector.source_element_id}`)
      const destination = scene.connectorTargets?.[key] ?? { viewId, elementId: connector.target_element_id }
      const targetElement = scene.views[destination.viewId]?.placements.find((element) => element.element_id === destination.elementId)
      const targetPaths = scene.placements?.[`placement|${destination.viewId}|${destination.elementId}`]?.paths ?? (targetElement?.file_path ? [targetElement.file_path] : [])
      const paths = unique([...(source?.paths ?? []), ...targetPaths.map(normalizedPath)])
      const name = connector.label || `${source?.name || 'Element'} → ${targetElement?.name || 'Element'}`
      const signature = JSON.stringify([connector.source_element_id, destination, connector.label, connector.description, connector.relationship, connector.direction, connector.style, connector.url, unique(connector.tags ?? [])])
      result.set(key, { key, kind: 'connector', name, paths, signature, initialAction: (connector.tags ?? []).map(changeAction).find(Boolean), target: { key, kind: 'connector', viewId, elementIds: [{ viewId, elementId: connector.source_element_id }, destination], ancestorViewIds: ancestors(viewId) } })
    }
  }
  for (const [viewId, node] of views) {
    const content = scene.views[viewId]
    const key = `view|${viewId}`
    const paths = unique([...(pathsByView.get(viewId) ?? []), ...[...pathsByView].filter(([id]) => ancestors(id).includes(viewId)).flatMap(([, paths]) => paths)])
    const signature = JSON.stringify([node.name, node.description, node.level_label, node.parent_view_id, node.owner_element_id, unique(node.tags ?? []), unique((content?.placements ?? []).map((element) => element.element_id)), unique((content?.connectors ?? []).map((connector) => connector.id))])
    result.set(key, { key, kind: 'view', name: node.name, paths, signature, target: { key, kind: 'view', viewId, elementIds: [], ancestorViewIds: ancestors(viewId) } })
  }
  return result
}

export function materializedSceneChanges(previous: RepositoryImpactScene | null, next: RepositoryImpactScene): MapChange[] {
  if (previous?.repositoryId !== next.repositoryId) previous = null
  if (previous?.version === next.version) return []
  const before = previous ? resources(previous) : new Map<string, Resource>()
  const after = resources(next)
  const beforeFiles = fileSignatures(previous), afterFiles = fileSignatures(next)
  const changedPaths = unique([...beforeFiles.keys(), ...afterFiles.keys()].filter((path) => beforeFiles.get(path) !== afterFiles.get(path)))
  const changes: MapChange[] = []
  for (const key of unique([...before.keys(), ...after.keys()])) {
    const old = before.get(key), current = after.get(key)
    const resource = current ?? old!
    // The initial scene seeds existing annotated changes, not every loaded resource.
    const action = !previous ? current?.initialAction : !current ? 'removed' : !old ? 'added' : old.signature !== current.signature ? 'modified' : undefined
    if (!action) continue
    const paths = unique([...(current?.paths ?? []), ...(old?.paths ?? [])])
    const path = changedPaths.find((path) => paths.some((scope) => matchesPath(scope, path))) ?? paths[0] ?? ''
    changes.push({ key, kind: resource.kind, name: resource.name, path, action, target: resource.target })
  }
  return changes
}

/** One latest materialization per file. Repeated edits replace its details and
 * advance its counter instead of appending unbounded rows or resource IDs. */
export function mergeChangeHistory(history: MapChangeGroup[], changes: MapChange[], updatedAt: number): MapChangeGroup[] {
  if (!changes.length) return history
  const byPath = new Map<string, MapChange[]>()
  for (const change of changes) {
    const entries = byPath.get(change.path) ?? []
    entries.push(change)
    byPath.set(change.path, entries)
  }
  const changed = [...byPath].map(([path, entries]) => ({
    path,
    changes: entries,
    counts: { element: entries.filter((entry) => entry.kind === 'element').length, view: entries.filter((entry) => entry.kind === 'view').length, connector: entries.filter((entry) => entry.kind === 'connector').length },
    updates: (history.find((group) => group.path === path)?.updates ?? 0) + 1,
    updatedAt,
  }))
  return [...changed, ...history.filter((group) => !byPath.has(group.path))].slice(0, CHANGE_HISTORY_FILES)
}
