import { describe, expect, it } from 'vitest'
import type { LayoutNode, ZUILayout } from './types'
import { applyProvenanceOverlays, PROVENANCE_META } from './provenance'

function node(elementId: number, children: LayoutNode[] = []): LayoutNode {
  return {
    id: `d1-o${elementId}`,
    elementId,
    diagramId: 1,
    worldX: 0,
    worldY: 0,
    worldW: 100,
    worldH: 80,
    label: `node-${elementId}`,
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: [],
    ancestorElementIds: [],
    pathElementIds: [elementId],
    children,
    childScale: 1,
    childOffsetX: 0,
    childOffsetY: 0,
    edgesOut: [],
  }
}

const layout = (nodes: LayoutNode[]): ZUILayout => ({
  groups: [{
    diagramId: 1,
    label: 'g',
    description: null,
    level: 0,
    levelLabel: null,
    worldX: 0,
    worldY: 0,
    worldW: 100,
    worldH: 100,
    diagramW: 100,
    diagramH: 100,
    diagramX: 0,
    diagramY: 0,
    nodes,
    edges: [],
  }],
  bbox: { minX: 0, minY: 0, maxX: 100, maxY: 100 },
})

describe('applyProvenanceOverlays', () => {
  it('marks nodes by element id and recurses into children', () => {
    const rendered = applyProvenanceOverlays(
      layout([node(1, [node(2)]), node(-5)]),
      { 1: 'authored', 2: 'generated', [-5]: 'augmented' },
    )
    expect(rendered.groups[0].nodes[0].provenance).toBe('authored')
    expect(rendered.groups[0].nodes[0].children[0].provenance).toBe('generated')
    expect(rendered.groups[0].nodes[1].provenance).toBe('augmented')
  })

  it('leaves unmapped nodes untouched', () => {
    const rendered = applyProvenanceOverlays(layout([node(9)]), {})
    expect(rendered.groups[0].nodes[0].provenance).toBeUndefined()
  })

  it('covers every provenance kind in metadata', () => {
    for (const kind of ['authored', 'augmented', 'generated'] as const) {
      expect(PROVENANCE_META[kind].label).toBeTruthy()
      expect(PROVENANCE_META[kind].color).toMatch(/^#/)
      expect(PROVENANCE_META[kind].glyph).toBeTruthy()
    }
  })
})
