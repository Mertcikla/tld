import { describe, expect, it } from 'vitest'
import { alignCodeBundleHandles, buildZUIBundleFans } from './connectorBundles'
import { nodeVisualGeometry, nodeVisualLayout } from './layoutEngine'
import { subNodeHandleAnchor } from './subElements'
import { buildEdgeSpatialIndex, findHoveredEdge } from './edgeHover'
import type { DiagramGroupLayout, LayoutNode } from './types'

function codeNode(id: string, x: number): LayoutNode {
  return { id, elementId: Number(id), diagramId: 1, label: id, type: 'file', subKind: 'code', subDetail: 'outline',
    worldX: x, worldY: 0, worldW: 520, worldH: 360, logoUrl: null, description: null, technology: null,
    tags: [], ancestorElementIds: [], pathElementIds: [], children: [], childScale: 1, childOffsetX: 0, childOffsetY: 0, edgesOut: [],
    subSource: { status: 'ready', startLine: 1, endLine: 50, tokens: [], lines: Array<string>(50).fill('function f() {}'),
      symbols: [1, 10, 20, 30, 40].map(line => ({ name: `fn${line}`, type: 'function', startLine: line, endLine: line + 2 })) },
  }
}
function edge(id: number, sourceId: string, targetId: string): DiagramGroupLayout['edges'][number] {
  return { id, sourceId, targetId, label: '', direction: 'forward', type: 'bezier',
    sourceHandle: 'left-line-1,20,40', targetHandle: 'right-line-1,20,40' }
}

describe('ZUI code connector bundles', () => {
  it('aligns visible ports on both ends and separates incoming and outgoing bundles in one lane plan', () => {
    const nodes = [codeNode('1', 0), codeNode('2', 1000), codeNode('3', 1500)]
    const edges = [edge(10, '1', '2'), edge(20, '1', '3')]
    alignCodeBundleHandles(nodes, edges)
    expect(edges[0]).toMatchObject({ sourceHandle: 'right-line-1,20,40', targetHandle: 'left-line-1,20,40' })
    expect(nodes[0].subConnectedHandles).toEqual(['right-line-1,20,40', 'right-line-1,20,40'])
    const fans = buildZUIBundleFans(nodes.map(node => ({ node, x: node.worldX, y: node.worldY, scale: 1 })), edges, 1)
    const first = fans.get('10:source')!, second = fans.get('20:source')!
    expect(first.branches).toHaveLength(3)
    expect(second.branches).toHaveLength(3)
    expect(first.y).not.toBe(second.y)
    expect(first.branches).not.toEqual(second.branches)
    expect(fans.get('10:target')!.ports.every(port => port.x === 1000)).toBe(true)
  })

  it('matches source-card ports during visual zoom capping and collapses forks in overview', () => {
    const node = codeNode('1', 100), other = codeNode('2', 1000), link = edge(10, '1', '2')
    alignCodeBundleHandles([node, other], [link])
    const zoom = 4, geometry = nodeVisualGeometry(node, zoom, 480), visual = nodeVisualLayout(node, geometry)
    const x = node.worldX + geometry.offsetX, y = node.worldY + geometry.offsetY
    const fan = buildZUIBundleFans([{ node: visual, x, y, scale: geometry.scale }], [link], zoom).get('10:source')!
    const anchor = subNodeHandleAnchor(visual, 'right-line-1', zoom * geometry.scale)!
    expect(fan.ports[0].x).toBeCloseTo(x + anchor.x * geometry.scale)
    expect(fan.ports[0].y).toBeCloseTo(y + anchor.y * geometry.scale)
    const overview = { ...node, subDetail: 'overview' as const }
    expect(buildZUIBundleFans([{ node: overview, x, y, scale: 1 }], [link], 0.1).size).toBe(0)
  })

  it('includes distant symbol branches in hover picking with the original connector identity', () => {
    const nodes = [codeNode('1', 0), codeNode('2', 1000)], edges = [edge(10, '1', '2')]
    alignCodeBundleHandles(nodes, edges)
    const fans = buildZUIBundleFans(nodes.map(node => ({ node, x: node.worldX, y: node.worldY, scale: 1 })), edges, 1)
    const port = fans.get('10:source')!.ports.at(-1)!
    const group: DiagramGroupLayout = { diagramId: 1, label: 'test', description: null, level: 0, levelLabel: null,
      worldX: 0, worldY: 0, worldW: 1800, worldH: 600, diagramW: 1800, diagramH: 600, diagramX: 0, diagramY: 0, nodes, edges }
    const index = buildEdgeSpatialIndex([group], 1, { start: 200, end: 480 })
    const hovered = findHoveredEdge(port.x + 8, port.y, index, { zoom: 1, x: 0, y: 0 })
    expect(hovered?.type).toBe('edge')
    if (hovered?.type === 'edge') expect(hovered.data.id).toBe(10)
  })
})
