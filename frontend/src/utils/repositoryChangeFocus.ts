import type { RepositoryImpactScene } from '../api/client'
import type { ZUICanvasHandle } from '../components/ZUI'
import type { MapChangeTarget } from './repositoryChangeHistory'

export type MapChangeFocusRequest = { repositoryId: string; targets: MapChangeTarget[] }

/** Resolve history against the current scene: resources may have moved or
 * disappeared since the entry was recorded. Removed resources use surviving
 * endpoints, their containing view, or the closest surviving ancestor. */
export function focusMapChanges(scene: RepositoryImpactScene, targets: MapChangeTarget[], canvas: Pick<ZUICanvasHandle, 'focusElement' | 'focusElements' | 'focusDiagram'>): boolean {
  const elements = new Map<string, { viewId: number; elementId: number }>()
  const views: number[] = []
  for (const target of targets) {
    let refs = target.elementIds
    if (target.kind === 'connector') {
      const id = Number(target.key.split('|')[2])
      const connector = scene.views[target.viewId]?.connectors.find((item) => item.id === id)
      if (connector) refs = [{ viewId: target.viewId, elementId: connector.source_element_id }, scene.connectorTargets?.[target.key] ?? { viewId: target.viewId, elementId: connector.target_element_id }]
    }
    for (const ref of refs) {
      const viewId = scene.views[ref.viewId]?.placements.some((item) => item.element_id === ref.elementId) ? ref.viewId : Number(Object.keys(scene.views).find((id) => scene.views[id].placements.some((item) => item.element_id === ref.elementId)))
      if (Number.isFinite(viewId)) elements.set(`${viewId}:${ref.elementId}`, { viewId, elementId: ref.elementId })
    }
    views.push(target.viewId, ...target.ancestorViewIds)
  }
  const refs = [...elements.values()]
  if (refs.length === 1 && canvas.focusElement(refs[0].viewId, refs[0].elementId)) return true
  if (refs.length > 1 && canvas.focusElements(refs)) return true
  for (const viewId of [...new Set([...views, scene.fallbackViewId])]) {
    if (scene.views[viewId] && canvas.focusDiagram(viewId)) return true
  }
  return false
}
