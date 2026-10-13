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
  it('anchors the popover at the hovered part of a long connector', () => {
    const source = node('source', 1, 0, 0)
    const target = node('target', 2, 2000, 0)
    const edge = { id: 10, sourceId: 'source', targetId: 'target', label: 'calls', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight' }
    const index = buildEdgeSpatialIndex([group([source, target], [edge])])
    const hovered = expectHoveredEdge(findHoveredEdge(200, 45, index, view))

    expect(hovered.absX).toBe(200)
    expect(hovered.absY).toBe(45)
    expect(hovered.route).toEqual([{ x: 120, y: 40 }, { x: 2000, y: 40 }])
  })

  function nestedFixture() {
    const source = node('child-source', 2, 10, 20, { diagramId: 42 })
    const target = node('child-target', 3, 270, 20, { diagramId: 42 })
    source.edgesOut = [{ id: 20, targetId: target.id, label: 'Nested call', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight' }]
    const parent = node('parent', 1, 100, 100, {
      worldW: 400, worldH: 200, childScale: 0.5, childOffsetX: 10, childOffsetY: 20,
      children: [source, target],
    })
    return { parent, source, target, groups: [group([parent], [])] }
  }

  it('finds nested connectors with their owning view and world coordinates', () => {
    const { groups } = nestedFixture()
    const index = buildEdgeSpatialIndex(groups, 2, { start: 300, end: 600 })
    const hovered = expectHoveredEdge(findHoveredEdge(200, 120, index, { ...view, zoom: 2 }))

    expect(hovered.data).toMatchObject({ id: 20, diagramId: 42, sourceObjId: 2, targetObjId: 3 })
    expect(hovered.route).toEqual([{ x: 160, y: 120 }, { x: 230, y: 120 }])
  })

  it('does not expose nested connectors until the child diagram is rendered', () => {
    const { groups } = nestedFixture()
    const index = buildEdgeSpatialIndex(groups, 0.5, { start: 300, end: 600 })
    expect(findHoveredEdge(200, 120, index, { ...view, zoom: 0.5 })).toBeNull()
  })

  it('excludes hidden endpoints and their nested connectors', () => {
    const { groups, parent, source } = nestedFixture()
    source.tags = ['hidden']
    const hidden = new Set(['hidden'])
    expect(findHoveredEdge(200, 120, buildEdgeSpatialIndex(groups, 2, { start: 300, end: 600 }, hidden), view)).toBeNull()
    source.tags = []
    parent.tags = ['hidden']
    expect(findHoveredEdge(200, 120, buildEdgeSpatialIndex(groups, 2, { start: 300, end: 600 }, hidden), view)).toBeNull()
  })

  it('respects the clipping boundary of an expanded diagram', () => {
    const { groups, target } = nestedFixture()
    target.worldX = 1200
    const index = buildEdgeSpatialIndex(groups, 2, { start: 300, end: 600 })
    expect(findHoveredEdge(550, 120, index, view)).toBeNull()
    expect(findHoveredEdge(400, 120, index, view)?.type).toBe('edge')
  })

  it('matches capped leaf endpoints and distributed handles at deep zoom', () => {
    const source = node('source', 1, 0, 0)
    const top = node('top', 2, 1000, 0)
    const bottom = node('bottom', 3, 1000, 200)
    const edge = { id: 10, sourceId: 'source', targetId: 'top', label: 'top call', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight' }
    const other = { ...edge, id: 11, targetId: 'bottom', label: 'bottom call' }
    const index = buildEdgeSpatialIndex([group([source, top, bottom], [edge, other])], 10, { start: 300, end: 600 })
    // Source cap is 0.5, so its right handle is x=90 and top slot offset is -12.
    const hovered = expectHoveredEdge(findHoveredEdge(92, 28, index, { ...view, zoom: 10 }))
    expect(hovered.data.id).toBe(10)
    expect(hovered.route?.[0]).toEqual({ x: 90, y: 28 })
  })

})
