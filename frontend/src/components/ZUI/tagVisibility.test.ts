import { describe, expect, it } from 'vitest'
import { buildSceneGraph } from './sceneGraph'
import { hitTestZUIRenderedNode } from './hitTest'
import { buildEdgeSpatialIndex, findHoveredEdge } from './edgeHover'
import { collectVisibleNodeAnchors } from './proxy'
import { filterLayoutByTags, visiblePlacementKeys } from './tagVisibility'
import type { LayoutNode, ZUILayout } from './types'
import { computeLayout } from './layout'
import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../../types'

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

function connect(source: LayoutNode, target: LayoutNode, id: number, tags: string[] = []) {
  const edge = { id, targetId: target.id, label: 'connects', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight', tags }
  source.edgesOut.push(edge)
  return { ...edge, sourceId: source.id }
}

describe('filterLayoutByTags', () => {
  it('carries connector tags through real layout construction into filtering and highlighting', () => {
    const data: ExploreData = {
      tree: [{ id: 1, name: 'Root', children: [] } as unknown as ViewTreeNode], navigations: [],
      views: { 1: {
        placements: [1, 2].map((id) => ({ element_id: id, view_id: 1, name: `Element ${id}`, position_x: id * 200, position_y: 0, tags: [] } as unknown as PlacedElement)),
        connectors: [{ id: 10, view_id: 1, source_element_id: 1, target_element_id: 2, tags: ['critical'] } as Connector],
      } },
    }
    const original = computeLayout(data)
    expect(original.groups[0].edges[0].tags).toEqual(['critical'])
    const filtered = filterLayoutByTags(original, ['critical'], [], ['critical'])
    expect(filtered.groups[0].nodes).toHaveLength(2)
    expect(filtered.groups[0].nodes[0].edgesOut[0]).toMatchObject({ tags: ['critical'], tagHighlighted: true })
  })

  it('retains cross-view proxy endpoints as context without exposing their unrelated descendants', () => {
    const original = layout([node(1, [], [node(2, []), node(3, [])]), node(4, []), node(5, [])])
    const context = { selected: new Set([2, 4]), highlighted: new Set([2, 4]) }
    const filtered = filterLayoutByTags(original, ['critical'], [], ['critical'], context)
    expect(visiblePlacementKeys(filtered)).toEqual(new Set(['1:1', '1:2', '1:4']))
    expect(filtered.groups[0].nodes[0].children[0].tagContext).toBe(true)
    expect(filtered.groups[0].nodes[1].tagHoverState).toBe('context')
  })

  it('retains connector context without unrelated edges, siblings, or descendants', () => {
    const first = node(1, [], [node(5, [])])
    const second = node(2, [])
    const third = node(3, [])
    const original = layout([first, second, third])
    original.groups[0].edges = [connect(first, second, 10, ['critical']), connect(first, second, 11), connect(second, third, 12)]
    const filtered = filterLayoutByTags(original, ['critical'])
    expect(visiblePlacementKeys(filtered)).toEqual(new Set(['1:1', '1:2']))
    expect(filtered.groups[0].edges.map((edge) => edge.id)).toEqual([10])
    expect(filtered.groups[0].nodes[0].edgesOut.map((edge) => edge.id)).toEqual([10])
    expect(filtered.groups[0].nodes.every((node) => node.tagContext)).toBe(true)
    expect(filtered.groups[0].nodes[0].worldX).toBe(first.worldX)
    expect(filtered.bbox).toBe(original.bbox)
    expect(first.children).toHaveLength(1)
    expect(first.edgesOut).toHaveLength(2)
    expect(filterLayoutByTags(original)).toBe(original)
  })

  it('retains ancestors of nested connector matches and honors hidden subtrees and edges', () => {
    const source = node(2, [])
    const target = node(3, [])
    connect(source, target, 10, ['critical'])
    const parent = node(1, ['parent'], [source, target, node(4, [])])
    const original = layout([parent, node(5, [])])
    expect(visiblePlacementKeys(filterLayoutByTags(original, ['critical']))).toEqual(new Set(['1:1', '1:2', '1:3']))
    expect(filterLayoutByTags(original, ['critical'], ['parent']).groups).toEqual([])
    expect(filterLayoutByTags(original, ['critical'], ['critical']).groups).toEqual([])
    const hidden = filterLayoutByTags(original, [], ['critical'])
    expect(visiblePlacementKeys(hidden).size).toBe(5)
    expect(hidden.groups[0].nodes[0].children[0].edgesOut).toEqual([])
  })

  it('highlights connector matches and quiets endpoints without mutating their tags', () => {
    const first = node(1, [])
    const second = node(2, ['api'])
    const original = layout([first, second, node(3, [])])
    original.groups[0].edges = [connect(first, second, 10, ['critical'])]
    const highlighted = filterLayoutByTags(original, [], [], ['critical'])
    expect(highlighted.groups[0].nodes.map((node) => node.tagHoverState)).toEqual(['context', 'context', 'inactive'])
    expect(highlighted.groups[0].nodes[0].edgesOut[0].tagHighlighted).toBe(true)
    expect(highlighted.groups[0].edges[0].tagHighlighted).toBe(true)
    expect(first.tags).toEqual([])
    expect(second.tags).toEqual(['api'])
  })

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
