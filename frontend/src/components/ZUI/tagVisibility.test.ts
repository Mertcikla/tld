import { describe, expect, it } from 'vitest'
import { buildSceneGraph } from './sceneGraph'
import { hitTestZUIRenderedNode } from './hitTest'
import { buildEdgeSpatialIndex, findHoveredEdge } from './edgeHover'
import { collectVisibleNodeAnchors } from './proxy'
import { filterLayoutByTags, visiblePlacementKeys } from './tagVisibility'
import type { LayoutNode, ZUILayout } from './types'

function node(id: number, tags: string[], children: LayoutNode[] = []): LayoutNode {
  return {
    id: `node-${id}`, elementId: id, diagramId: 1,
    worldX: (id - 1) * 200, worldY: 0, worldW: 100, worldH: 100,
    label: `Node ${id}`, type: 'service', logoUrl: null, description: null, technology: null,
    tags, children, childScale: 0.1, childOffsetX: 0, childOffsetY: 0,
    ancestorElementIds: [], pathElementIds: [id], edgesOut: [],
  }
}

function layout(nodes: LayoutNode[]): ZUILayout {
  return {
    bbox: { minX: 0, minY: 0, maxX: 1000, maxY: 500 },
    groups: [{ diagramId: 1, label: 'Root', description: null, level: 0, levelLabel: null,
      worldX: 0, worldY: 0, worldW: 1000, worldH: 500,
      diagramX: 0, diagramY: 0, diagramW: 1000, diagramH: 500, nodes, edges: [] }],
  }
}

describe('filterLayoutByTags', () => {
  it('matches any selected tag, excludes untagged nodes, and restores the original layout on reset', () => {
    const original = layout([node(1, ['go']), node(2, ['javascript']), node(3, []), node(4, ['go', 'javascript'])])
    const filtered = filterLayoutByTags(original, ['go', 'javascript'])
    expect(visiblePlacementKeys(filtered)).toEqual(new Set(['1:1', '1:2', '1:4']))
    expect(filtered.bbox).toBe(original.bbox)
    expect(filtered.groups[0].nodes[1].worldX).toBe(original.groups[0].nodes[1].worldX)
    expect(original.groups[0].nodes).toHaveLength(4)
    expect(filterLayoutByTags(original)).toBe(original)
  })

  it('retains ancestors, hides nonmatching siblings and descendants, and honors eye-hidden subtrees', () => {
    const original = layout([node(1, [], [node(2, ['go']), node(3, ['javascript'])]), node(4, ['go'], [node(5, [])])])
    const filtered = filterLayoutByTags(original, ['go'])
    expect(visiblePlacementKeys(filtered)).toEqual(new Set(['1:1', '1:2', '1:4']))
    expect(filtered.groups[0].nodes[0].children).toHaveLength(1)
    expect(filtered.groups[0].nodes[1].children).toHaveLength(0)
    original.groups[0].nodes[0].tags = ['parent']
    expect(visiblePlacementKeys(filterLayoutByTags(original, [], ['parent']))).toEqual(new Set(['1:4', '1:5']))
    expect(filterLayoutByTags(original, ['missing']).groups).toEqual([])
  })

  it('excludes hidden nodes and connectors from the scene, hit tests, and proxy anchors', () => {
    const original = layout([node(1, ['go']), node(2, ['javascript'])])
    const edge = { id: 1, targetId: 'node-2', label: 'connects', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight' }
    original.groups[0].nodes[0].edgesOut = [edge]
    original.groups[0].edges = [{ ...edge, sourceId: 'node-1' }]
    const filtered = filterLayoutByTags(original, ['go'])
    const view = { x: 0, y: 0, zoom: 1 }
    const thresholds = { start: 500, end: 1000 }
    expect(buildSceneGraph(filtered).nodeById.has('node-2')).toBe(false)
    expect(filtered.groups[0].nodes[0].edgesOut).toEqual([])
    expect(filtered.groups[0].edges).toEqual([])
    expect(hitTestZUIRenderedNode(250, 50, filtered.groups, view, thresholds)).toBeNull()
    expect(hitTestZUIRenderedNode(50, 50, filtered.groups, view, thresholds)?.node.elementId).toBe(1)
    expect(findHoveredEdge(150, 50, buildEdgeSpatialIndex(filtered.groups), view)).toBeNull()
    expect(Array.from(collectVisibleNodeAnchors(filtered.groups, view, 1200).visibleAnchors.keys())).toEqual([1])
  })
})
