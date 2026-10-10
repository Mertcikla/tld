import type { LayoutNode, ZUILayout, ZUINodeProvenance } from './types'

export type { ZUINodeProvenance }

export const PROVENANCE_META: Record<ZUINodeProvenance, { label: string; blurb: string; color: string; glyph: string; dash: number[] }> = {
  authored: { label: 'Authored', blurb: 'drawn in the workspace', color: '#94a3b8', glyph: '●', dash: [] },
  augmented: { label: 'Augmented', blurb: 'from graph facts · not in the workspace', color: '#a78bfa', glyph: '◇', dash: [7, 4] },
  generated: { label: 'Generated', blurb: 'materialized by the code map', color: '#2dd4bf', glyph: '▦', dash: [2, 4] },
}

export function applyProvenanceOverlays(layout: ZUILayout, provenance: Record<number, ZUINodeProvenance>): ZUILayout {
  const decorate = (node: LayoutNode): LayoutNode => ({
    ...node,
    children: node.children.map(decorate),
    provenance: provenance[node.elementId] ?? node.provenance,
  })
  return { ...layout, groups: layout.groups.map((group) => ({ ...group, nodes: group.nodes.map(decorate) })) }
}
