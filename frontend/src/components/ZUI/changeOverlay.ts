import type { LayoutNode, ZUIChangeOverlay, ZUILayout } from './types'

/** Node ring colors by change kind; doubles as the tag colors for change tags. */
export const CHANGE_RING_COLORS: Record<ZUIChangeOverlay['change'], string> = {
  added: '#48bb78',
  removed: '#fc8181',
  modified: '#ecc94b',
  unchanged: '#718096',
}

export function applyChangeOverlays(layout: ZUILayout, overlays: Record<number, ZUIChangeOverlay>): ZUILayout {
  const decorate = (node: LayoutNode): LayoutNode => {
    const change = overlays[node.elementId]
    const children = node.children.map(decorate)
    return { ...node, children, changeOverlay: change }
  }
  return { ...layout, groups: layout.groups.map((group) => ({ ...group, nodes: group.nodes.map(decorate) })) }
}
