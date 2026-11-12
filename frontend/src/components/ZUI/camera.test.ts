import { describe, expect, it } from 'vitest'
import { clamp01, easeOutQuart, findFirstExpandableNode, fitWorldRect, getPathAt } from './camera'
import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'

function node(id: string, children: LayoutNode[] = []): LayoutNode {
  return {
    id,
    elementId: Number(id.replace(/\D/g, '')) || 1,
    diagramId: 1,
    worldX: 0,
    worldY: 0,
    worldW: 200,
    worldH: 120,
    label: id,
    type: 'service',
    logoUrl: null,
    description: null,
    technology: null,
    tags: [],
    ancestorElementIds: [],
    pathElementIds: [],
    linkedDiagramLabel: children.length > 0 ? `${id} child view` : undefined,
    children,
    childScale: 1,
    childOffsetX: 0,
    childOffsetY: 0,
    edgesOut: [],
  }
}

function group(nodes: LayoutNode[]): DiagramGroupLayout {
  return {
    diagramId: 1,
    label: 'Root',
    description: null,
    level: 0,
    levelLabel: null,
    worldX: -100,
    worldY: -100,
    worldW: 600,
    worldH: 420,
    diagramW: 600,
    diagramH: 420,
    diagramX: 0,
    diagramY: 0,
    nodes,
    edges: [],
  }
}

describe('ZUI camera helpers', () => {
  it('descends breadcrumb path only after a centered node is visually expanded', () => {
    const leaf = node('leaf')
    const parent = node('parent', [leaf])
    const groups = [group([parent])]
    const overview: ZUIViewState = { x: 500, y: 300, zoom: 1 }
    const expanded: ZUIViewState = { x: 500, y: 300, zoom: 3 }

    expect(getPathAt(overview, groups, 1000, 600).map((item) => item.label)).toEqual(['Root', 'parent'])
    expect(getPathAt(expanded, groups, 1000, 600).map((item) => item.label)).toEqual(['Root', 'parent', 'leaf'])
  })

  it('finds the first node with children for scripted detail camera frames', () => {
    const expandable = node('expandable', [node('child')])
    const found = findFirstExpandableNode([group([node('plain'), expandable])])

    expect(found?.id).toBe('expandable')
    expect(found?.label).toBe('expandable child view')
  })

  it('fits a world rectangle inside the viewport with padding and max zoom', () => {
    const view = fitWorldRect({ x: 100, y: 50, w: 200, h: 100 }, 1000, 600, 3, 0.1)

    expect(view?.zoom).toBe(3)
    expect(view?.x).toBe(-100)
    expect(view?.y).toBe(0)
  })

  it('normalizes scripted interpolation inputs', () => {
    expect(clamp01(-1)).toBe(0)
    expect(clamp01(2)).toBe(1)
    expect(easeOutQuart(0)).toBe(0)
    expect(easeOutQuart(1)).toBe(1)
    expect(easeOutQuart(0.5)).toBeGreaterThan(0.5)
  })
})


it('keeps large-map fits at the minimum zoom and centers their bounds', () => {
  const view = fitWorldRect({ x: 100, y: 50, w: 10000, h: 10000 }, 1000, 600, 3, 0.1)!
  expect(view.zoom).toBe(0.1)
  expect(view.x + 5100 * view.zoom).toBe(500)
  expect(view.y + 5050 * view.zoom).toBe(300)
})
