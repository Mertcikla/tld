import { clone } from '@bufbuild/protobuf'
import { ChangeKind, ImpactRole, ImpactGrounding, ImpactSceneSchema, type ImpactScene } from '@buf/tldiagramcom_diagram.bufbuild_es/codeindex/v1/codeindex_pb.js'
import type { View } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'

/** Mermaid-only projection. Never mutate the scene shared with the canvas.
 * Keep this policy in sync with internal/mermaid/selection.go. */
export function selectMermaidScene(scene: ImpactScene, radius: number, plain: boolean): ImpactScene {
  const out = clone(ImpactSceneSchema, scene)
  const ref = (view: number, id: number) => `${view}|${id}`
  const placements = new Map(Object.values(scene.views).flatMap(view => view.placements.filter(p => p.element).map(p => [ref(p.element!.viewId, p.element!.elementId), p] as const)))
  const distances = new Map<string, number>()
  const adjacent = new Map<string, string[]>()
  const queue: string[] = []
  const seed = (key: string, distance: number) => {
    if (distance < (distances.get(key) ?? Infinity)) { distances.set(key, distance); queue.push(key) }
  }
  for (const [key, p] of placements) {
    if ([ImpactRole.DIRECT, ImpactRole.CONTAINED, ImpactRole.UNSPECIFIED].includes(p.role) || (p.overlay && p.overlay.change !== ChangeKind.UNSPECIFIED)) seed(key, 0)
    if (p.role === ImpactRole.DEPENDENCY_CONTEXT && p.distance <= radius) seed(key, p.distance)
  }
  for (const file of scene.files) for (const p of file.representatives) seed(ref(p.viewId, p.elementId), 0)
  const endpoints = (viewId: number, edge: ImpactScene['views'][string]['connectors'][number], content: ImpactScene['views'][string]) => {
    const source = ref(viewId, edge.sourceElementId)
    let target = ref(viewId, edge.targetElementId)
    const cross = content.connectorTargets[String(edge.id)]
    if (!placements.has(target) && cross) target = ref(cross.viewId, cross.elementId)
    return [source, target]
  }
  for (const [viewId, content] of Object.entries(scene.views)) for (const edge of content.connectors) {
    const [a, b] = endpoints(Number(viewId), edge, content)
    if (!placements.has(a) || !placements.has(b)) continue
    adjacent.set(a, [...(adjacent.get(a) ?? []), b]); adjacent.set(b, [...(adjacent.get(b) ?? []), a])
    if (edge.tags.some(t => ['change:added', 'change:modified', 'change:removed'].includes(t))) { seed(a, 0); seed(b, 0) }
  }
  for (let i = 0; i < queue.length; i++) {
    const key = queue[i], distance = distances.get(key)!
    if (distance >= radius) continue
    for (const next of adjacent.get(key) ?? []) {
      const p = placements.get(next)
      if (!p || p.role === ImpactRole.CONTAINER || (plain && p.role === ImpactRole.AUTHORED_CONTEXT) || (p.role === ImpactRole.DEPENDENCY_CONTEXT && p.distance > radius)) continue
      seed(next, distance + 1)
    }
  }
  const kept = (key: string) => placements.has(key) && (distances.get(key) ?? Infinity) <= radius
  for (const [viewId, content] of Object.entries(out.views)) {
    content.placements = content.placements.filter(p => p.element && kept(ref(p.element.viewId, p.element.elementId)))
    content.connectors = content.connectors.filter(edge => {
      const keep = endpoints(Number(viewId), edge, content).every(kept)
      if (!keep) delete content.connectorTargets[String(edge.id)]
      return keep
    })
  }
  const relevant = new Set<number>()
  const prune = (views: View[]): View[] => views.flatMap(view => {
    view.children = prune(view.children)
    if (out.views[String(view.id)]?.placements.length || view.children.length) { relevant.add(view.id); return [view] }
    return []
  })
  out.tree = prune(out.tree)
  for (const [key, content] of Object.entries(out.views)) if (!content.placements.length && !relevant.has(Number(key))) delete out.views[key]
  if (out.comparison) { out.comparison.effectiveRadius = radius; out.comparison.plain = plain }
  if (out.summary) {
    const s = out.summary
    s.affectedPlacements = s.dependencyNeighbours = s.authoredContext = s.containers = s.ungrounded = 0
    for (const view of Object.values(out.views)) for (const p of view.placements) {
      if (p.role === ImpactRole.DIRECT || p.role === ImpactRole.CONTAINED) s.affectedPlacements++
      if (p.role === ImpactRole.DEPENDENCY_CONTEXT) s.dependencyNeighbours++
      if (p.role === ImpactRole.AUTHORED_CONTEXT) s.authoredContext++
      if (p.role === ImpactRole.CONTAINER) s.containers++
      if ([ImpactGrounding.UNLINKED, ImpactGrounding.UNRESOLVED, ImpactGrounding.AMBIGUOUS].includes(p.grounding)) s.ungrounded++
    }
  }
  return out
}
