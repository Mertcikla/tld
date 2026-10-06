import type { RepositoryImpact } from '../api/client'
import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../types'
import type { ZUIChangeOverlay } from '../components/ZUI/types'
import { chooseConnectorHandles } from './connectorHandles'

export { CHANGE_OVERLAY_TAG as REPOSITORY_CHANGE_TAG } from '../components/ZUI/changeOverlay'
import { CHANGE_OVERLAY_TAG as REPOSITORY_CHANGE_TAG } from '../components/ZUI/changeOverlay'
const normalize = (path: string) => path.replace(/\\/g, '/').replace(/\/$/, '')

// Re-attach every connector in a view to the source/target handles that yield
// the shortest anchor distance for the overlay's final placements, mirroring the
// map pipeline's "Adjust Connectors" pass. The overlay is transient, so this
// never writes back to the workspace.
function adjustConnectorHandles(placements: PlacedElement[], connectors: Connector[]): Connector[] {
  if (!connectors.length) return connectors
  const positions = new Map(placements.map((element) => [element.element_id, { x: element.position_x, y: element.position_y }]))
  return connectors.map((connector) => {
    const source = positions.get(connector.source_element_id)
    const target = positions.get(connector.target_element_id)
    if (!source || !target) return connector
    const handles = chooseConnectorHandles(source, target)
    if (connector.source_handle === handles.source && connector.target_handle === handles.target) return connector
    return { ...connector, source_handle: handles.source, target_handle: handles.target }
  })
}

// Build a transient scene over existing workspace placements. New and removed
// files without placements get negative IDs; nothing is written to the workspace.
export function repositoryChangeOverlay(workspace: ExploreData, impact: RepositoryImpact, repositoryRoot: string) {
  const overlays: Record<number, ZUIChangeOverlay> = {}
  const filesByPath = new Map(impact.nodes.map((file) => [file.path, file]))
  const sources = new Map(impact.diff.sources.map((source) => [source.path, source]))
  const matched = new Set<string>()
  const eligibleIds = new Set(impact.nodes.filter((node) => node.context).map((node) => node.elementId))
  const belongs = (element: PlacedElement) => element.repository_id
    ? element.repository_id === impact.repositoryId
    : normalize(element.repo ?? '') === normalize(repositoryRoot) || eligibleIds.has(element.element_id)
  const overlay = (file: RepositoryImpact['nodes'][number]): ZUIChangeOverlay => ({
    change: file.change, path: file.path,
    linesAdded: sources.get(file.path)?.linesAdded,
    linesRemoved: sources.get(file.path)?.linesRemoved,
    symbols: (['added', 'removed', 'modified'] as const).flatMap((kind) => file.symbols[kind].map((symbol) => `${kind === 'added' ? '+' : kind === 'removed' ? '−' : '~'} ${symbol.name}`)),
  })
  const retained = new Set<number>()
  const keep = (tree: ViewTreeNode[]): ViewTreeNode[] => tree.flatMap((view) => {
    // Retired impact views are excluded while an older watcher is still running.
    if (view.name.includes(' impact · ')) return []
    const children = keep(view.children)
    const placements = workspace.views[view.id]?.placements ?? []
    if (!children.length && !placements.some(belongs)) return []
    retained.add(view.id)
    return [{ ...view, children }]
  })
  const data: ExploreData = { tree: keep(workspace.tree), views: {}, navigations: [] }
  for (const id of retained) {
    const view = workspace.views[id]
    if (!view) continue
    data.views[id] = { ...view, placements: view.placements.map((element) => {
      const file = belongs(element) ? filesByPath.get(element.file_path ?? '') : undefined
      if (!file) return element
      matched.add(file.path)
      overlays[element.element_id] = overlay(file)
      return { ...element, tags: [...element.tags, REPOSITORY_CHANGE_TAG] }
    }) }
  }
  data.navigations = workspace.navigations.filter((link) => retained.has(link.from_view_id) && retained.has(link.to_view_id))
  const missing = impact.nodes.filter((file) => !file.context && !matched.has(file.path))
  if (missing.length) {
    // The pipeline picks the closest suitable base view for added files; place
    // them there at the coordinates it computed. Fall back to a transient
    // catch-all view when no suitable view exists.
    const target = impact.viewId && retained.has(impact.viewId) ? data.views[impact.viewId] : undefined
    const viewId = target ? impact.viewId : -1
    const ids = new Map(missing.map((file, i) => [file.key, -(i + 1)]))
    const placements = missing.map((file, i): PlacedElement => {
      const id = ids.get(file.key)!
      overlays[id] = overlay(file)
      return { id, element_id: id, view_id: viewId, position_x: target ? file.x : (i % 3) * 240, position_y: target ? file.y : Math.floor(i / 3) * 150,
        name: file.name, kind: 'component', description: file.path, technology: null, url: null, logo_url: null,
        technology_connectors: [], tags: [REPOSITORY_CHANGE_TAG], repo: repositoryRoot, file_path: file.path, has_view: false, view_label: null }
    })
    const connectors = impact.edges.filter((edge) => ids.has(edge.fromKey) && ids.has(edge.toKey)).map((edge, i) => ({
      id: -(i + 1), view_id: viewId, source_element_id: ids.get(edge.fromKey)!, target_element_id: ids.get(edge.toKey)!,
      label: `${edge.weight} dependencies`, description: null, relationship: null, direction: 'forward', style: 'bezier',
      url: null, source_handle: null, target_handle: null, created_at: '', updated_at: '',
    }))
    if (target) {
      target.placements = [...target.placements, ...placements]
      target.connectors = [...(target.connectors ?? []), ...connectors]
    } else {
      data.tree.push({ id: viewId, name: 'Changes', description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children: [] })
      data.views[viewId] = { placements, connectors }
    }
  }
  for (const view of Object.values(data.views)) {
    view.connectors = adjustConnectorHandles(view.placements, view.connectors ?? [])
  }
  return { data, overlays }
}
