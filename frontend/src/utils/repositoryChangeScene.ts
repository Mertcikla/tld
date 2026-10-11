import type { RepositoryImpactScene } from '../api/client'
import type { ExploreData, PlacedElement, ViewTreeNode } from '../types'
import type { ZUIChangeOverlay } from '../components/ZUI/types'
import { CHANGE_RING_COLORS } from '../components/ZUI/changeOverlay'
import { PROVENANCE_META, type ZUINodeProvenance } from '../components/ZUI/provenance'

export const CHANGE_STATE_TAGS = ['added', 'removed', 'unchanged'] as const
export const PROVENANCE_TAGS = ['authored', 'augmented', 'generated'] as const

/** Seed colors for overlay tags; workspace tagColors take precedence. */
export const REPOSITORY_TAG_COLORS: Record<string, string> = {
  ...CHANGE_RING_COLORS,
  authored: PROVENANCE_META.authored.color,
  augmented: PROVENANCE_META.augmented.color,
  generated: PROVENANCE_META.generated.color,
}

export function repositoryChangeScene(scene: RepositoryImpactScene, options: { plain?: boolean }) {
  const plain = options.plain ?? false
  const authored = new Set(scene.authoredViewIds ?? [])
  const degraded = authored.size === 0
  const overlays: Record<number, ZUIChangeOverlay> = {}

  const impacted = new Set<number>()
  for (const elementId of Object.keys(scene.overlays)) {
    impacted.add(Number(elementId))
  }
  const shown = (element: PlacedElement) => plain ? impacted.has(element.element_id) : true

  const isTransient = (element: PlacedElement) => element.element_id < 0
  const viewHasTransient = (viewId: number) =>
    (scene.views[String(viewId)]?.placements ?? []).some(isTransient)
  const isFallback = (view: ViewTreeNode) => view.id === scene.fallbackViewId && scene.fallbackViewId !== 0
  const retained = new Set<number>()
  const keep = (tree: ViewTreeNode[]): ViewTreeNode[] => tree.flatMap((view) => {
    if (view.name.includes(' impact · ')) return []

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

  // Container elements that own retained child views are structural: without
  // them the hierarchy can't be traversed and nested views collapse to
  // nothing. Both modes keep them; they carry no change overlay, so they
  // render as plain parents and are never annotated as changes.
  const linkElements = new Map<number, Set<number>>()
  for (const link of scene.navigations ?? []) {
    if (link.relation_type !== 'child' || link.element_id == null || !retained.has(link.to_view_id)) continue
    const elements = linkElements.get(link.from_view_id) ?? new Set<number>()
    elements.add(link.element_id)
    linkElements.set(link.from_view_id, elements)
  }
  for (const id of retained) {
    const view = scene.views[String(id)]
    if (!view) continue
    const links = linkElements.get(id)
    // Pruned views keep change evidence (transients, impacted placements)
    // plus the containers that reach retained child views: dropping those
    // orphans the nesting and blanks the canvas. Connectors follow the
    // visible endpoints.
    const pruned = !degraded && !authored.has(id) && !drilldown.has(id)
    const placements = plain
      ? view.placements.filter((element) => shown(element) || links?.has(element.element_id))
      : pruned
        ? view.placements.filter((element) => isTransient(element) || impacted.has(element.element_id) || links?.has(element.element_id))
        : view.placements
    const visibleIds = new Set(placements.map((element) => element.element_id))
    const connectors = (plain || pruned)
      ? (view.connectors ?? []).filter((item) => visibleIds.has(item.source_element_id) && visibleIds.has(item.target_element_id))
      : view.connectors
    data.views[id] = {
      ...view,
      placements: placements.map((element) => {
        const tags = (element.tags ?? []).filter((t) => t !== 'map-generated')
        const provenanceTag: ZUINodeProvenance = element.element_id < 0
          ? 'augmented'
          : (element.tags ?? []).includes('map-generated') ? 'generated' : 'authored'
        const overlay = scene.overlays[element.element_id]
        if (!overlay) return { ...element, tags: [...tags, provenanceTag] }
        overlays[element.element_id] = {
          change: overlay.change, path: overlay.path,
          linesAdded: overlay.linesAdded, linesRemoved: overlay.linesRemoved, symbols: overlay.symbols,
          reason: overlay.reason,
        }
        const changeTags = overlay.change === 'modified' ? [] : [overlay.change]
        return { ...element, tags: [...tags, provenanceTag, ...changeTags] }
      }),
      connectors,
    }
  }
  data.navigations = (scene.navigations ?? []).filter((link) => retained.has(link.from_view_id) && retained.has(link.to_view_id))
  return { data, overlays }
}

function findSceneView(tree: ViewTreeNode[], id: number): ViewTreeNode | null {
  for (const view of tree) {
    if (view.id === id) return view
    const found = findSceneView(view.children ?? [], id)
    if (found) return found
  }
  return null
}
