import type { LayoutNode, ZUIChangeOverlay, ZUILayout } from './types'

export const CHANGE_OVERLAY_TAG = '__repository_change_overlay'

export function applyChangeOverlays(layout: ZUILayout, overlays: Record<number, ZUIChangeOverlay>): ZUILayout {
  const decorate = (node: LayoutNode): LayoutNode => {
    const change = overlays[node.elementId]
    const children = node.children.map(decorate)
    const affected = !!change || children.some((child) => child.tags.includes(CHANGE_OVERLAY_TAG))
    return { ...node, children, changeOverlay: change,
      tags: affected ? [...node.tags, CHANGE_OVERLAY_TAG] : node.tags,
    }
  }
  return { ...layout, groups: layout.groups.map((group) => ({ ...group, nodes: group.nodes.map(decorate) })) }
}
