import type { RepositoryImpactScene } from '../api/client'
import type { ExploreData, PlacedElement, ViewTreeNode } from '../types'
import type { ZUIChangeOverlay } from '../components/ZUI/types'
import type { ZUINodeProvenance } from '../components/ZUI/types'

export { CHANGE_OVERLAY_TAG as REPOSITORY_CHANGE_TAG } from '../components/ZUI/changeOverlay'
import { CHANGE_OVERLAY_TAG as REPOSITORY_CHANGE_TAG } from '../components/ZUI/changeOverlay'

// repositoryChangeScene adapts the backend-assembled impact scene to the ZUI
// canvas contract. Membership, transient placements and connector routing are
// owned by the backend; this only applies the client-side blast radius and
// standard/plain view filters, both of which never require a server call.
// Grounded is the only diagram: authored views pinned, plus every directly
// changed file, whether or not the workspace covers it. Uncovered files
// surface as transient placements (augmented from the graph facts), so the
// canvas always captures the full change. With no authored views in the
// scene it degrades to the mapped diagram instead of an empty canvas.
export function repositoryChangeScene(scene: RepositoryImpactScene, options: { plain?: boolean }) {
  const plain = options.plain ?? false
  const authored = new Set(scene.authoredViewIds ?? [])
  // With no authored hits there is nothing to pin, so degrade to the mapped
  // diagram instead of an empty canvas.
  const degraded = authored.size === 0
  const overlays: Record<number, ZUIChangeOverlay> = {}
  const provenance: Record<number, ZUINodeProvenance> = {}
  // Without a display radius everything computed shows: impacted means
  // carrying a change overlay.
  const impacted = new Set<number>()
  for (const elementId of Object.keys(scene.overlays)) {
    impacted.add(Number(elementId))
  }
  const shown = (element: PlacedElement) => plain ? impacted.has(element.element_id) : true
  // Transient placements carry negative element ids (backend placeMissing).
  // They are directly changed files with no workspace placement: the part of
  // the change the authored views do not cover.
  const isTransient = (element: PlacedElement) => element.element_id < 0
  const viewHasTransient = (viewId: number) =>
    (scene.views[String(viewId)]?.placements ?? []).some(isTransient)
  const isFallback = (view: ViewTreeNode) => view.id === scene.fallbackViewId && scene.fallbackViewId !== 0
  const retained = new Set<number>()
  const keep = (tree: ViewTreeNode[]): ViewTreeNode[] => tree.flatMap((view) => {
    // Retired impact views are excluded while an older watcher is still running.
    if (view.name.includes(' impact · ')) return []
    // The synthetic Changes view holds directly changed files with no
    // workspace placement, and transients can also land inside a mapped
    // target view. Dropping either would hide part of the diff, so both are
    // retained; non-authored views are pruned to just their change evidence
    // below. The blast-radius slider stays enabled so collapsed map context
    // can widen around the pins.
    const children = keep(view.children ?? [])
    if (degraded) {
      const placements = scene.views[String(view.id)]?.placements ?? []
      if (!children.length && !placements.some(shown)) return []
    } else if (!isFallback(view) && !authored.has(view.id) && !children.length && !viewHasTransient(view.id)) {
      return []
    }
    retained.add(view.id)
    return [{ ...view, children }]
  })
  keep(scene.tree)
  // Elements with a child view render nested in ZUI; without the child's
  // output they collapse to singular flat elements. Pull every drill-down
  // view owned by a retained placement into the scene recursively, keeping
  // its full output so the canvas can render children.
  const drilldown = new Set<number>()
  const childByElement = new Map<number, number>()
  for (const link of scene.navigations ?? []) {
    if (link.relation_type !== 'child' || link.element_id == null) continue
    if (!childByElement.has(link.element_id)) childByElement.set(link.element_id, link.to_view_id)
  }
  const queue = [...retained]
  for (let head = 0; head < queue.length; head++) {
    const from = queue[head]
    for (const element of scene.views[String(from)]?.placements ?? []) {
      const child = childByElement.get(element.element_id)
      if (child == null || retained.has(child)) continue
      const node = findSceneView(scene.tree, child)
      if (node?.name.includes(' impact · ')) continue
      retained.add(child)
      drilldown.add(child)
      queue.push(child)
    }
  }
  const withDrilldown = (tree: ViewTreeNode[]): ViewTreeNode[] => tree.flatMap((view) => {
    if (view.name.includes(' impact · ')) return []
    const children = withDrilldown(view.children ?? [])
    if (!retained.has(view.id) && children.length === 0) return []
    if (!retained.has(view.id)) retained.add(view.id)
    return [{ ...view, children }]
  })
  const data: ExploreData = { tree: withDrilldown(scene.tree), views: {}, navigations: [] }
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
    // Non-authored views (fallback Changes, transient carriers) are pruned to
    // change evidence only: transient orphans plus blast-radius-impacted
    // placements, with connectors filtered to visible endpoints. Authored
    // views keep their full membership so the user's diagram renders as
    // authored. Drill-down children of retained elements also keep full
    // output so nested nodes render instead of collapsing flat. Degraded
    // mode shows mapped views whole.
    const pruned = !degraded && !authored.has(id) && !drilldown.has(id)
    const placements = plain
      ? view.placements.filter((element) => shown(element) || links?.has(element.element_id))
      : pruned
        ? view.placements.filter((element) => isTransient(element) || impacted.has(element.element_id))
        : view.placements
    const visibleIds = new Set(placements.map((element) => element.element_id))
    const connectors = (plain || pruned)
      ? (view.connectors ?? []).filter((item) => visibleIds.has(item.source_element_id) && visibleIds.has(item.target_element_id))
      : view.connectors
    data.views[id] = {
      ...view,
      placements: placements.map((element) => {
        // Provenance is orthogonal to change state: transients synthesized
        // from graph facts read as augmented even inside authored views,
        // authored-view members as authored, everything else as generated.
        provenance[element.element_id] = element.element_id < 0
          ? 'augmented'
          : authored.has(id) ? 'authored' : 'generated'
        const overlay = scene.overlays[element.element_id]
        // Only direct changes glow; blast-radius context keeps its outline but no
        // spotlight so it reads as surrounding context.
        if (!overlay) return element
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
  return { data, overlays, provenance }
}

function findSceneView(tree: ViewTreeNode[], id: number): ViewTreeNode | null {
  for (const view of tree) {
    if (view.id === id) return view
    const found = findSceneView(view.children ?? [], id)
    if (found) return found
  }
  return null
}
