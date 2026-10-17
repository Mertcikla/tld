import { isElementGroupTag } from './elementGroups'
import { isGroundingIgnoreTag } from './groundingTags'

export interface TaggedCanvasNode<Id> {
  id: Id
  tags?: readonly string[]
}

export interface TaggedCanvasConnector<Id, EdgeId> {
  id: EdgeId
  source: Id
  target: Id
  tags?: readonly string[]
}

/** Match authored tags only. Context endpoints never become matches themselves. */
export function deriveCanvasTagState<Id, EdgeId>(
  nodes: readonly TaggedCanvasNode<Id>[],
  connectors: readonly TaggedCanvasConnector<Id, EdgeId>[],
  selectedTags: readonly string[] = [],
  hiddenTags: readonly string[] = [],
) {
  const selected = new Set(selectedTags)
  const hidden = new Set(hiddenTags)
  const matches = (tags: readonly string[] = []) => tags.some((tag) => selected.has(tag))
  const isHidden = (tags: readonly string[] = []) => tags.some((tag) => hidden.has(tag))
  const matchedNodeIds = new Set<Id>()
  const visibleNodeIds = new Set<Id>()
  const hiddenNodeIds = new Set<Id>()
  const nodeIds = new Set(nodes.map((node) => node.id))
  for (const node of nodes) {
    if (isHidden(node.tags)) hiddenNodeIds.add(node.id)
    else {
      if (matches(node.tags)) matchedNodeIds.add(node.id)
      if (selected.size === 0 || matches(node.tags)) visibleNodeIds.add(node.id)
    }
  }
  const visibleConnectorIds = new Set<EdgeId>()
  const matchedConnectorIds = new Set<EdgeId>()
  for (const edge of connectors) {
    if (!nodeIds.has(edge.source) || !nodeIds.has(edge.target) ||
      hiddenNodeIds.has(edge.source) || hiddenNodeIds.has(edge.target) || isHidden(edge.tags)) continue
    const directMatch = matches(edge.tags)
    if (selected.size === 0 || directMatch || (matchedNodeIds.has(edge.source) && matchedNodeIds.has(edge.target))) {
      visibleConnectorIds.add(edge.id)
      visibleNodeIds.add(edge.source)
      visibleNodeIds.add(edge.target)
      if (directMatch) matchedConnectorIds.add(edge.id)
    }
  }
  const contextNodeIds = new Set([...visibleNodeIds].filter((id) => selected.size > 0 && !matchedNodeIds.has(id)))
  return { matchedNodeIds, contextNodeIds, hiddenNodeIds, visibleNodeIds, visibleConnectorIds, matchedConnectorIds }
}

export interface CanvasTagCounts {
  tagCounts: Record<string, number>
  tagElementCounts: Record<string, number>
  tagConnectorCounts: Record<string, number>
}

export function countCanvasTags(elements: readonly { tags?: readonly string[] }[], connectors: readonly { tags?: readonly string[] }[]): CanvasTagCounts {
  const tagCounts: Record<string, number> = {}
  const tagElementCounts: Record<string, number> = {}
  const tagConnectorCounts: Record<string, number> = {}
  for (const [items, counts] of [[elements, tagElementCounts], [connectors, tagConnectorCounts]] as const) {
    for (const item of items) for (const tag of new Set(item.tags ?? [])) {
      if (isElementGroupTag(tag) || isGroundingIgnoreTag(tag)) continue
      counts[tag] = (counts[tag] ?? 0) + 1
      tagCounts[tag] = (tagCounts[tag] ?? 0) + 1
    }
  }
  return { tagCounts, tagElementCounts, tagConnectorCounts }
}

export function tagCountDescription(elements: number, connectors: number): string {
  return `${elements} ${elements === 1 ? 'element' : 'elements'} · ${connectors} ${connectors === 1 ? 'connector' : 'connectors'}`
}
