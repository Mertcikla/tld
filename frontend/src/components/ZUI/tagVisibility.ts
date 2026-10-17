import type { LayoutNode, ZUILayout } from './types'
import { deriveCanvasTagState } from '../../utils/canvasTags'

/** Project visibility without moving nodes, changing bounds, or mutating the source layout. */
export function filterLayoutByTags(layout: ZUILayout, selectedTags: readonly string[] = [], hiddenTags: readonly string[] = [], highlightedTags: readonly string[] = [], context?: {
  selected: ReadonlySet<number>
  highlighted: ReadonlySet<number>
}): ZUILayout {
  if (selectedTags.length === 0 && hiddenTags.length === 0 && highlightedTags.length === 0) return layout
  const hidden = new Set(hiddenTags)

  function filterNodes(nodes: LayoutNode[]): LayoutNode[] {
    const edges = nodes.flatMap((node) => node.edgesOut.map((edge) => ({ ...edge, source: node.id, target: edge.targetId })))
    const state = deriveCanvasTagState(nodes, edges, selectedTags, hiddenTags)
    const hover = deriveCanvasTagState(nodes, edges, highlightedTags, hiddenTags)
    const kept = nodes.flatMap((node) => {
      if (node.tags.some((tag) => hidden.has(tag))) return []
      const children = filterNodes(node.children)
      if (selectedTags.length > 0 && children.length === 0 && !state.visibleNodeIds.has(node.id) && !context?.selected.has(node.elementId)) return []
      const tagHoverState = highlightedTags.length === 0 ? undefined
        : hover.matchedNodeIds.has(node.id) ? 'match' as const
          : hover.contextNodeIds.has(node.id) || context?.highlighted.has(node.elementId) || children.some((child) => child.tagHoverState !== 'inactive') ? 'context' as const
            : 'inactive' as const
      return [{ ...node, children,
        tagContext: selectedTags.length > 0 && !state.matchedNodeIds.has(node.id),
        tagHoverState,
        edgesOut: node.edgesOut.filter((edge) => state.visibleConnectorIds.has(edge.id)).map((edge) => ({
          ...edge,
          tagHighlighted: hover.matchedConnectorIds.has(edge.id),
          tagMuted: highlightedTags.length > 0 && !hover.visibleConnectorIds.has(edge.id),
        })),
      }]
    })
    const ids = new Set(kept.map((node) => node.id))
    return kept.map((node) => ({ ...node, edgesOut: node.edgesOut.filter((edge) => ids.has(edge.targetId)) }))
  }

  const groups = layout.groups.flatMap((group) => {
    const nodes = filterNodes(group.nodes)
    if (nodes.length === 0) return []
    const edges = new Map(nodes.flatMap((node) => node.edgesOut.map((edge) => [edge.id, edge] as const)))
    return [{ ...group, nodes, edges: group.edges.filter((edge) => edges.has(edge.id)).map((edge) => ({
      ...edge, tagHighlighted: edges.get(edge.id)?.tagHighlighted, tagMuted: edges.get(edge.id)?.tagMuted,
    })) }]
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
