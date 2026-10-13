import type { LayoutNode, ZUILayout } from './types'

/** Project visibility without moving nodes, changing bounds, or mutating the source layout. */
export function filterLayoutByTags(layout: ZUILayout, selectedTags: readonly string[] = [], hiddenTags: readonly string[] = []): ZUILayout {
  if (selectedTags.length === 0 && hiddenTags.length === 0) return layout
  const selected = new Set(selectedTags)
  const hidden = new Set(hiddenTags)

  function filterNodes(nodes: LayoutNode[]): LayoutNode[] {
    const kept = nodes.flatMap((node) => {
      if (node.tags.some((tag) => hidden.has(tag))) return []
      const children = filterNodes(node.children)
      if (selected.size > 0 && children.length === 0 && !node.tags.some((tag) => selected.has(tag))) return []
      return [{ ...node, children }]
    })
    const ids = new Set(kept.map((node) => node.id))
    return kept.map((node) => ({ ...node, edgesOut: node.edgesOut.filter((edge) => ids.has(edge.targetId)) }))
  }

  const groups = layout.groups.flatMap((group) => {
    const nodes = filterNodes(group.nodes)
    if (nodes.length === 0) return []
    const ids = new Set(nodes.map((node) => node.id))
    return [{ ...group, nodes, edges: group.edges.filter((edge) => ids.has(edge.sourceId) && ids.has(edge.targetId)) }]
  })
  return { ...layout, groups }
}

export function visiblePlacementKeys(layout: ZUILayout): Set<string> {
  const keys = new Set<string>()
  function visit(nodes: LayoutNode[]) {
    for (const node of nodes) {
      keys.add(`${node.diagramId}:${node.elementId}`)
      visit(node.children)
    }
  }
  for (const group of layout.groups) visit(group.nodes)
  return keys
}
