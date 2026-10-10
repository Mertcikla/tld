import type { RepositoryImpactScene } from '../api/client'
import type { ExploreData, PlacedElement, ViewTreeNode } from '../types'
import type { ZUIChangeOverlay } from '../components/ZUI/types'

export { CHANGE_OVERLAY_TAG as REPOSITORY_CHANGE_TAG } from '../components/ZUI/changeOverlay'
import { CHANGE_OVERLAY_TAG as REPOSITORY_CHANGE_TAG } from '../components/ZUI/changeOverlay'

export type RepositoryChangeScope = 'mapped' | 'authored' | 'grounded'

// repositoryChangeScene adapts the backend-assembled impact scene to the ZUI
// canvas contract. Membership, transient placements and connector routing are
// owned by the backend; this only applies the client-side blast radius and
// standard/plain view filters, both of which never require a server call.
// The scope filter switches between the generated map (mapped), the
// user-authored diagrams the change touched (authored), and the grounded
// default: authored views pinned with map context collapsed around them.
// Grounded falls back to mapped when no authored view matched the change
// (e.g. live watch before anything is linked).
export function repositoryChangeScene(scene: RepositoryImpactScene, options: { radius: number; plain?: boolean; scope?: RepositoryChangeScope }) {
  const plain = options.plain ?? false
  const radius = options.radius
  const requested = options.scope ?? 'grounded'
  const authored = new Set(scene.authoredViewIds ?? [])
  // Grounded pins the authored views; with no authored hits there is nothing
  // to pin, so it degrades to the mapped diagram instead of an empty canvas.
  const scope: RepositoryChangeScope = requested === 'grounded' && authored.size === 0 ? 'mapped' : requested
  const overlays: Record<number, ZUIChangeOverlay> = {}
  const impacted = new Set<number>()
  for (const [elementId, overlay] of Object.entries(scene.overlays)) {
    if (overlay.distance <= radius) impacted.add(Number(elementId))
  }
  const shown = (element: PlacedElement) => plain ? impacted.has(element.element_id) : true
  const retained = new Set<number>()
  const keep = (tree: ViewTreeNode[]): ViewTreeNode[] => tree.flatMap((view) => {
    // Retired impact views are excluded while an older watcher is still running.
    if (view.name.includes(' impact · ')) return []
    // Authored and grounded modes drop the generated map and the synthetic
    // Changes view: they show only the views the user authored, with their
    // ancestors so the hierarchy still renders. Grounded keeps the blast-radius
    // slider enabled so collapsed map context can still widen around the pins.
    if (scope === 'authored' || scope === 'grounded') {
      if (view.id === scene.fallbackViewId && scene.fallbackViewId !== 0) return []
      const children = keep(view.children ?? [])
      if (!authored.has(view.id) && !children.length) return []
      retained.add(view.id)
      return [{ ...view, children }]
    }
    const children = keep(view.children ?? [])
    const placements = scene.views[String(view.id)]?.placements ?? []
    if (!children.length && !placements.some(shown)) return []
    retained.add(view.id)
    return [{ ...view, children }]
  })
  const data: ExploreData = { tree: keep(scene.tree), views: {}, navigations: [] }
  // Plain mode still needs the container elements that own retained child views;
  // without them the hierarchy can't be traversed. They are structural only and
  // are never annotated as changes.
  const linkElements = new Map<number, Set<number>>()
  if (plain) {
    for (const link of scene.navigations ?? []) {
      if (link.relation_type !== 'child' || link.element_id == null || !retained.has(link.to_view_id)) continue
      const elements = linkElements.get(link.from_view_id) ?? new Set<number>()
      elements.add(link.element_id)
      linkElements.set(link.from_view_id, elements)
    }
  }
  for (const id of retained) {
    const view = scene.views[String(id)]
    if (!view) continue
    const links = linkElements.get(id)
    const placements = plain ? view.placements.filter((element) => shown(element) || links?.has(element.element_id)) : view.placements
    const visibleIds = new Set(placements.map((element) => element.element_id))
    const connectors = plain ? (view.connectors ?? []).filter((item) => visibleIds.has(item.source_element_id) && visibleIds.has(item.target_element_id)) : view.connectors
    data.views[id] = {
      ...view,
      placements: placements.map((element) => {
        const overlay = scene.overlays[element.element_id]
        // Only direct changes glow; blast-radius context keeps its outline but no
        // spotlight so it reads as surrounding context.
        if (!overlay || overlay.distance > radius) return element
        overlays[element.element_id] = {
          change: overlay.change, path: overlay.path,
          linesAdded: overlay.linesAdded, linesRemoved: overlay.linesRemoved, symbols: overlay.symbols,
          reason: overlay.reason,
        }
        return overlay.change === 'unchanged' ? element : { ...element, tags: [...element.tags, REPOSITORY_CHANGE_TAG] }
      }),
      connectors,
    }
  }
  data.navigations = (scene.navigations ?? []).filter((link) => retained.has(link.from_view_id) && retained.has(link.to_view_id))
  return { data, overlays }
}
