import type { ExploreData, PlacedElement, ViewTreeNode } from '../types'

export type JumpViewMode = 'explore' | 'hierarchy'

export type JumpSearchResult =
  | {
    type: 'view'
    key: string
    name: string
    viewId: number
    level: number
    levelLabel: string | null
  }
  | {
    type: 'element'
    key: string
    name: string
    viewId: number
    viewName: string
    elementId: number
    kind: string | null
  }

export function flattenTree(roots: ViewTreeNode[]): ViewTreeNode[] {
  const result: ViewTreeNode[] = []
  const traverse = (node: ViewTreeNode) => {
    result.push(node)
    node.children.forEach(traverse)
  }
  roots.forEach(traverse)
  return result
}

export function jumpResultSubtitle(result: JumpSearchResult): string {
  if (result.type === 'view') {
    return `Level ${result.level} • ${result.levelLabel || 'Diagram'}`
  }
  return `${result.kind || 'Element'} • ${result.viewName}`
}

export function jumpResultActionLabel(view: JumpViewMode): string {
  if (view === 'explore') return 'ZOOM'
  return 'OPEN'
}

function searchScore(value: string, normalizedTerm: string): number {
  const normalizedValue = value.toLowerCase()
  if (normalizedValue === normalizedTerm) return 0
  if (normalizedValue.startsWith(normalizedTerm)) return 1
  return normalizedValue.includes(normalizedTerm) ? 2 : 3
}

function placementMatches(placement: PlacedElement, normalizedTerm: string): boolean {
  return [
    placement.name,
    placement.kind,
    placement.technology,
    placement.file_path,
    ...(placement.tags ?? []),
  ]
    .filter(Boolean)
    .some((value) => String(value).toLowerCase().includes(normalizedTerm))
}

export function buildJumpSearchResults(term: string, flatTree: ViewTreeNode[], exploreData: ExploreData | null): JumpSearchResult[] {
  const normalized = term.trim().toLowerCase()
  if (normalized.length < 3) return []

  const viewById = new Map(flatTree.map((node) => [node.id, node]))
  const viewResults: JumpSearchResult[] = flatTree
    .filter((node) => node.name.toLowerCase().includes(normalized))
    .sort((a, b) => searchScore(a.name, normalized) - searchScore(b.name, normalized) || a.name.localeCompare(b.name))
    .slice(0, 4)
    .map((node) => ({
      type: 'view',
      key: `view:${node.id}`,
      name: node.name,
      viewId: node.id,
      level: node.level,
      levelLabel: node.level_label,
    }))

  const elementResults: JumpSearchResult[] = []
  if (exploreData) {
    Object.entries(exploreData.views ?? {}).forEach(([viewIdText, content]) => {
      const viewId = Number(viewIdText)
      if (!Number.isFinite(viewId)) return
      const viewName = viewById.get(viewId)?.name ?? `View ${viewId}`
      ;(content.placements ?? []).forEach((placement) => {
        if (!placementMatches(placement, normalized)) return
        elementResults.push({
          type: 'element',
          key: `element:${viewId}:${placement.element_id}`,
          name: placement.name || `Element ${placement.element_id}`,
          viewId,
          viewName,
          elementId: placement.element_id,
          kind: placement.kind,
        })
      })
    })
  }

  const dedupedElements = Array.from(
    new Map(elementResults.map((result) => [result.key, result])).values(),
  )
    .sort((a, b) => searchScore(a.name, normalized) - searchScore(b.name, normalized) || a.name.localeCompare(b.name))
    .slice(0, 6)

  return [...viewResults, ...dedupedElements].slice(0, 8)
}
