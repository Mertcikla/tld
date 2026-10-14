import type { ExploreData, LibraryElement, PlacedElement, ViewTreeNode } from '../types'

// Reserved display-only IDs. These must never be sent to a workspace mutation.
export const UNPLACED_ELEMENTS_VIEW_ID = -2147483648
export const UNPLACED_ELEMENTS_BOUNDARY_ID = UNPLACED_ELEMENTS_VIEW_ID
export const UNPLACED_ELEMENTS_NAME = 'Unplaced elements'

export function withoutUnplacedElements(data: ExploreData): ExploreData {
  return {
    ...data,
    tree: data.tree.filter((view) => view.id !== UNPLACED_ELEMENTS_VIEW_ID),
    views: Object.fromEntries(Object.entries(data.views).filter(([id]) => Number(id) !== UNPLACED_ELEMENTS_VIEW_ID)),
  }
}

export function withUnplacedElements(data: ExploreData): ExploreData {
  const source = withoutUnplacedElements(data)
  const elementsById = new Map((source.elements ?? []).map((element) => [element.id, element]))
  const placedIds = new Set<number>()
  const connectedIds = new Set<number>()
  for (const content of Object.values(source.views)) {
    for (const placement of content.placements) {
      placedIds.add(placement.element_id)
      elementsById.set(placement.element_id, {
        ...elementsById.get(placement.element_id),
        ...placement,
        id: placement.element_id,
        created_at: elementsById.get(placement.element_id)?.created_at ?? '',
        updated_at: elementsById.get(placement.element_id)?.updated_at ?? '',
      })
    }
    for (const connector of content.connectors) {
      connectedIds.add(connector.source_element_id)
      connectedIds.add(connector.target_element_id)
    }
  }
  const unplacedIds = [...connectedIds].filter((id) => id > 0 && !placedIds.has(id)).sort((a, b) => a - b)
  const result = { ...source, elements: [...elementsById.values()] }
  if (unplacedIds.length === 0) return result

  const columns = Math.min(4, Math.ceil(Math.sqrt(unplacedIds.length)))
  const placements: PlacedElement[] = unplacedIds.map((id, index) => {
    const element: LibraryElement = elementsById.get(id) ?? {
      id, name: `Element ${id}`, kind: null, description: null, technology: null,
      url: null, logo_url: null, technology_connectors: [], tags: [],
      has_view: false, view_label: null, created_at: '', updated_at: '',
    }
    return {
      ...element,
      id: -id,
      element_id: id,
      view_id: UNPLACED_ELEMENTS_VIEW_ID,
      position_x: (index % columns) * 240,
      position_y: Math.floor(index / columns) * 140,
    }
  })
  const boundary: ViewTreeNode = {
    id: UNPLACED_ELEMENTS_VIEW_ID,
    owner_element_id: null,
    name: UNPLACED_ELEMENTS_NAME,
    description: 'Connected elements that are not placed in any view.',
    level_label: null,
    level: 0,
    depth: 0,
    parent_view_id: null,
    created_at: '',
    updated_at: '',
    children: [],
  }
  return {
    ...result,
    tree: [...result.tree, boundary],
    views: { ...result.views, [UNPLACED_ELEMENTS_VIEW_ID]: { placements, connectors: [] } },
  }
}
