import { api } from '../../api/client'
import type { Connector, LibraryElement, PlacedElement, TechnologyConnector } from '../../types'
import { resolveElementIconUrl } from '../../utils/elementIcon'
import { ELEMENT_NODE_HEIGHT, ELEMENT_NODE_WIDTH } from './selection'

function technologyKey(link: TechnologyConnector) {
  return JSON.stringify([link.type, link.slug ?? '', link.label])
}

export function sharedNestTechnology(elements: PlacedElement[]) {
  const first = elements[0]
  if (!first) return { technology: null, logo_url: '', technology_connectors: [] }
  const sharedLinks = (first.technology_connectors ?? []).filter((link) =>
    elements.every((element) => (element.technology_connectors ?? []).some((other) => technologyKey(other) === technologyKey(link))),
  )
  const icon = resolveElementIconUrl(first.logo_url, first.technology_connectors)
  const sharedIcon = elements.every((element) => resolveElementIconUrl(element.logo_url, element.technology_connectors) === icon)
  return {
    technology: elements.every((element) => element.technology === first.technology)
      ? first.technology
      : sharedLinks.map((link) => link.label).join(', '),
    // Preserve an explicitly cleared icon, and avoid choosing an arbitrary icon
    // from the common technology links when the selected icons differ.
    logo_url: sharedIcon ? (first.logo_url ?? icon ?? '') : '',
    technology_connectors: sharedLinks.map((link) => ({
      ...link,
      is_primary_icon: sharedIcon && !!(link.is_primary_icon ?? link.isPrimaryIcon),
    })),
  }
}

export async function nestSelection({
  viewId,
  placements,
  connectors,
  bounds,
}: {
  viewId: number
  placements: PlacedElement[]
  connectors: Connector[]
  bounds: { centerX: number; centerY: number }
}): Promise<LibraryElement> {
  if (placements.length < 2) throw new Error('Select at least two elements to nest')
  const selectedIds = new Set(placements.map((placement) => placement.element_id))
  const internal = connectors.filter((connector) => connector.view_id === viewId
    && selectedIds.has(connector.source_element_id) && selectedIds.has(connector.target_element_id))
  const nest = await api.elements.create({ name: 'New Nest', ...sharedNestTechnology(placements) })
  let childViewId: number | null = null
  const moved: Connector[] = []
  const removed: PlacedElement[] = []
  try {
    const childView = await api.workspace.views.create({ name: nest.name, parent_view_id: nest.id })
    childViewId = childView.id

    // Move the original connectors first so placement auto-inclusion cannot
    // duplicate them. Their IDs, metadata and endpoints stay intact.
    for (const connector of internal) {
      await api.workspace.connectors.update(childView.id, connector.id, { tags: connector.tags ?? [] })
      moved.push(connector)
    }
    for (const placement of placements) {
      await api.workspace.views.placements.add(childView.id, placement.element_id, placement.position_x, placement.position_y)
    }
    // Placing elements can also include connectors from unrelated views. The
    // new view should contain exactly the internal connectors of this selection.
    const internalIds = new Set(internal.map((connector) => connector.id))
    for (const connector of await api.workspace.connectors.list(0)) {
      if (connector.view_id === childView.id && !internalIds.has(connector.id)) await api.workspace.connectors.delete('', connector.id)
    }
    await api.workspace.views.placements.add(viewId, nest.id,
      bounds.centerX - ELEMENT_NODE_WIDTH / 2, bounds.centerY - ELEMENT_NODE_HEIGHT / 2)
    for (const placement of placements) {
      await api.workspace.views.placements.remove(viewId, placement.element_id)
      removed.push(placement)
    }
    return { ...nest, has_view: true }
  } catch (error) {
    // Restore the source before removing the newly created container. Keep the
    // container if recovery fails so no moved content is deleted.
    try {
      for (const connector of moved) {
        await api.workspace.connectors.update(viewId, connector.id, { tags: connector.tags ?? [] })
      }
      for (const placement of removed) {
        await api.workspace.views.placements.add(viewId, placement.element_id, placement.position_x, placement.position_y)
      }
      if (childViewId !== null) await api.workspace.views.delete('', childViewId)
      await api.elements.delete('', nest.id)
    } catch (recoveryError) {
      throw new Error(`Nesting failed; some content remains in New Nest. ${String(error)}; recovery: ${String(recoveryError)}`)
    }
    throw error
  }
}
