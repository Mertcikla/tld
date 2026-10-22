import type { AgentWorkspaceEdit, PlanResource } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb'
import type { ExploreData, PlacedElement, Connector, ViewTreeNode } from '../types'
import type { ZUIChangeOverlay } from '../components/ZUI/types'

// Build a review-only overlay. Neither the live scene nor workspace data is mutated.
export function agentWorkspaceOverlay(base: { data: ExploreData; overlays: Record<number, ZUIChangeOverlay> }, edits: AgentWorkspaceEdit[], context: PlanResource[]) {
  const pending = edits.filter(edit => !['committed', 'skipped', 'discarded'].includes(edit.status))
  if (!pending.length) return base
  const data: ExploreData = { ...base.data, tree: [...base.data.tree], views: Object.fromEntries(Object.entries(base.data.views).map(([id, v]) => [id, { placements: [...v.placements], connectors: [...v.connectors] }])) }
  const overlays = { ...base.overlays }
  const resources = new Map(context.map(r => [r.key, r]))
  const changed = new Set<string>()
  const removed = new Set<string>()
  const created = new Set<string>()
  for (const edit of pending) for (const op of edit.operations) {
    const action = op.action
    const resource = action.case === 'create' ? action.value : action.case === 'update' ? action.value.resource : action.case === 'remove' ? op.preconditions.find(p => p.before?.key === action.value?.key)?.before : undefined
    if (!resource) continue
    if (action.case === 'create') created.add(resource.key)
    if (action.case === 'remove' && created.has(resource.key)) {
      changed.delete(resource.key)
      resources.delete(resource.key)
      continue
    }
    resources.set(resource.key, resource)
    changed.add(resource.key)
    if (action.case === 'remove') removed.add(resource.key)
    else removed.delete(resource.key)
  }
  const byType = <T extends NonNullable<PlanResource['value']['case']>>(kind: T) => [...resources.values()].filter(r => r.value.case === kind)
  const elements = byType('element').flatMap(r => r.value.case === 'element' ? [r.value.value] : [])
  const ensureView = (viewId: number) => {
    if (data.views[viewId]) return data.views[viewId]
    const resource = resources.get(`view:${viewId}`)
    const name = resource?.value.case === 'view' ? resource.value.value.name : 'Staged agent work'
    data.tree.push({ id: viewId, name, description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children: [] })
    return (data.views[viewId] = { placements: [], connectors: [] })
  }
  const mark = (id: number, deleted = false) => {
    overlays[id] = { ...(overlays[id] || { change: 'modified', path: '', symbols: [], reason: 'direct' }), agentStaged: true, path: deleted ? 'Staged removal' : 'Staged agent edit' }
  }
  const place = (elementId: number, viewId: number, x = 0, y = 0, placementId = 0) => {
    const element = elements.find(e => e.id === elementId)
    if (!element) return
    const view = ensureView(viewId)
    const index = view.placements.findIndex(p => p.element_id === elementId)
    const old = view.placements[index]
    const placed: PlacedElement = { ...old, id: placementId || old?.id || elementId, view_id: viewId, element_id: elementId, position_x: old?.position_x ?? x, position_y: old?.position_y ?? y, name: element.name, description: element.description ?? null, kind: element.kind ?? null, technology: element.technology ?? null, url: element.url ?? null, logo_url: element.logoUrl ?? null, technology_connectors: old?.technology_connectors ?? [], tags: [...element.tags, 'agent:staged'], has_view: old?.has_view ?? false, view_label: old?.view_label ?? null }
    if (index < 0) view.placements.push(placed); else view.placements[index] = placed
  }
  for (const resource of resources.values()) {
    if (resource.value.case !== 'placement') continue
    const p = resource.value.value
    if (changed.has(resource.key) || changed.has(`element:${p.elementId}`)) {
      place(p.elementId, p.viewId, p.positionX, p.positionY, p.id)
      mark(p.elementId, removed.has(resource.key) || removed.has(`element:${p.elementId}`))
    }
  }
  for (const resource of resources.values()) {
    if (!changed.has(resource.key)) continue
    if (resource.value.case === 'view') {
      const v = resource.value.value
      ensureView(v.id)
      if (v.ownerElementId) mark(v.ownerElementId, removed.has(resource.key))
      for (const p of data.views[v.id].placements) mark(p.element_id, removed.has(resource.key))
    }
    if (resource.value.case === 'layer') {
      const view = data.views[resource.value.value.viewId]
      for (const p of view?.placements ?? []) mark(p.element_id)
    }
    if (resource.value.case === 'element') {
      const id = resource.value.value.id
      const views = Object.entries(data.views).filter(([, v]) => v.placements.some(p => p.element_id === id))
      if (!views.length) place(id, -900000001, (ensureView(-900000001).placements.length % 4) * 240, Math.floor(ensureView(-900000001).placements.length / 4) * 180)
      else for (const [viewId] of views) place(id, Number(viewId))
      mark(id, removed.has(resource.key))
    }
    if (resource.value.case === 'connector') {
      const c = resource.value.value
      const view = ensureView(c.viewId)
      for (const id of [c.sourceElementId, c.targetElementId]) if (!view.placements.some(p => p.element_id === id)) place(id, c.viewId, view.placements.length * 240)
      const edge: Connector = { id: c.id, view_id: c.viewId, source_element_id: c.sourceElementId, target_element_id: c.targetElementId, label: c.label ?? null, description: c.description ?? null, relationship: c.relationship ?? null, direction: c.direction, style: c.style, url: c.url ?? null, source_handle: c.sourceHandle ?? null, target_handle: c.targetHandle ?? null, tags: [...c.tags, 'agent:staged'], created_at: '', updated_at: '' }
      const index = view.connectors.findIndex(e => e.id === c.id)
      if (index < 0) view.connectors.push(edge); else view.connectors[index] = edge
    }
  }
  const stagedViews = new Set([...resources.values()].flatMap(r => {
    if (!changed.has(r.key)) return []
    if (r.value.case === 'view') return [r.value.value.id]
    if (r.value.case === 'layer') return [r.value.value.viewId]
    return []
  }))
  const decorateViews = (tree: ViewTreeNode[]): ViewTreeNode[] => tree.map(v => {
    const resource = resources.get(`view:${v.id}`)
    const staged = stagedViews.has(v.id)
    return { ...v, name: staged && resource?.value.case === 'view' ? resource.value.value.name : v.name, tags: staged ? [...(v.tags ?? []), 'agent:staged'] : v.tags, children: decorateViews(v.children) }
  })
  data.tree = decorateViews(data.tree)
  return { data, overlays }
}
