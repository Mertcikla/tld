import { describe, expect, it } from 'vitest'
import { buildEdgeRoutePoints, buildEdgeSpatialIndex, findHoveredEdge } from './edgeHover'
import type { DiagramGroupLayout, HoveredItem, LayoutNode, ZUIViewState } from './types'

type HoveredEdge = Extract<HoveredItem, { type: 'edge' }>

function expectHoveredEdge(hovered: HoveredItem | null): HoveredEdge {
  expect(hovered?.type).toBe('edge')
  if (!hovered || hovered.type !== 'edge') {
    throw new Error('Expected edge hover result')
  }
  return hovered
}

function node(id: string, elementId: number, x: number, y: number, options: Partial<LayoutNode> = {}): LayoutNode {
  return {
    id,
    elementId,
    diagramId: 1,
    worldX: x,
    worldY: y,
    worldW: 120,
    worldH: 80,
    label: id,
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: [],
    ancestorElementIds: [],
    pathElementIds: [elementId],
    children: [],
    childScale: 1,
    childOffsetX: 0,
    childOffsetY: 0,
    edgesOut: [],
    ...options,
  }
}

function group(nodes: LayoutNode[], edges: DiagramGroupLayout['edges']): DiagramGroupLayout {
  return {
    diagramId: 1,
    label: 'Root',
    description: null,
    level: 0,
    levelLabel: null,
    worldX: 0,
    worldY: 0,
    worldW: 600,
    worldH: 400,
    diagramW: 600,
    diagramH: 400,
    diagramX: 0,
    diagramY: 0,
    nodes,
    edges,
  }
}

const view: ZUIViewState = { x: 0, y: 0, zoom: 1 }

describe('ZUI edge hover index', () => {
  it.each(['bezier', 'step', 'straight'] as const)('hit tests %s routes', (type) => {
    const source = node('source', 1, 0, 0)
    const target = node('target', 2, 320, 120)
    const edge = {
      id: 10,
      sourceId: source.id,
      targetId: target.id,
      label: `${type} edge`,
      direction: 'forward',
      sourceHandle: null,
      targetHandle: null,
      type,
    }
    const route = buildEdgeRoutePoints(source, target, edge)
    const index = buildEdgeSpatialIndex([group([source, target], [edge])])

    const hovered = expectHoveredEdge(findHoveredEdge(route.midX, route.midY, index, view))

    expect(hovered.data.id).toBe(10)
    expect(hovered.data.label).toBe(`${type} edge`)
  })

  it('returns portal edge hover payloads', () => {
    const portal = node('Portal', 2, 200, 220, { isPortal: true, linkedDiagramId: 42 })
    const index = buildEdgeSpatialIndex([group([portal], [])])

    const hovered = expectHoveredEdge(findHoveredEdge(280, 310, index, view))

    expect(hovered.data.isPortalConn).toBe(true)
    expect(hovered.data.targetDiagId).toBe(42)
  })

  it('selects the nearest indexed edge within the hover threshold', () => {
    const a = node('a', 1, 0, 0)
    const b = node('b', 2, 300, 0)
    const c = node('c', 3, 0, 120)
    const d = node('d', 4, 300, 120)
    const top = { id: 1, sourceId: 'a', targetId: 'b', label: 'top', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight' }
    const bottom = { id: 2, sourceId: 'c', targetId: 'd', label: 'bottom', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight' }
    const index = buildEdgeSpatialIndex([group([a, b, c, d], [top, bottom])])
    const route = buildEdgeRoutePoints(c, d, bottom)

    const hovered = expectHoveredEdge(findHoveredEdge(route.midX, route.midY, index, view))

    expect(hovered.data.id).toBe(2)
  })

  it('carries edge change state into the hover payload', () => {
    const source = node('source', 1, 0, 0)
    const target = node('target', 2, 320, 120)
    const edge = {
      id: 10,
      sourceId: source.id,
      targetId: target.id,
      label: 'new edge',
      direction: 'forward',
      sourceHandle: null,
      targetHandle: null,
      type: 'straight',
      change: 'added' as const,
    }
    const route = buildEdgeRoutePoints(source, target, edge)
    const index = buildEdgeSpatialIndex([group([source, target], [edge])])

    const hovered = expectHoveredEdge(findHoveredEdge(route.midX, route.midY, index, view))

    expect(hovered.data.change).toBe('added')
  })
})
