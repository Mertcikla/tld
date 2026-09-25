import { describe, expect, it } from 'vitest'
import type { Node as RFNode } from 'reactflow'
import {
  isElementShapedMarqueeRect,
  marqueeHitsElementRect,
  planSelectionAlignment,
  planSelectionDistribution,
  selectedElementIds,
  visibleElementSelectionRects,
} from './selection'

function node(id: string, x: number, y: number, width = 100, height = 50, selected = true, type = 'elementNode'): RFNode {
  return {
    id,
    type,
    selected,
    position: { x, y },
    width,
    height,
    data: {},
  }
}

describe('ViewEditor selection helpers', () => {
  it('extracts selected element ids and ignores non-element nodes', () => {
    expect(selectedElementIds([
      node('10', 0, 0),
      node('20', 0, 0, 100, 50, false),
      node('context-left', 0, 0, 100, 50, true, 'contextNeighborNode'),
      node('30', 0, 0, 100, 50, true, 'ContextBoundaryElement'),
    ])).toEqual([10])
  })

  it('plans horizontal and vertical alignment against the selected bounds', () => {
    const nodes = [
      node('1', 10, 20, 100, 40),
      node('2', 80, 100, 80, 60),
      node('3', 200, 40, 120, 50),
    ]

    expect(planSelectionAlignment(nodes, 'left')).toEqual([
      { id: '2', elementId: 2, x: 10, y: 100 },
      { id: '3', elementId: 3, x: 10, y: 40 },
    ])
    expect(planSelectionAlignment(nodes, 'center')).toEqual([
      { id: '1', elementId: 1, x: 115, y: 20 },
      { id: '2', elementId: 2, x: 125, y: 100 },
      { id: '3', elementId: 3, x: 105, y: 40 },
    ])
    expect(planSelectionAlignment(nodes, 'right')).toEqual([
      { id: '1', elementId: 1, x: 220, y: 20 },
      { id: '2', elementId: 2, x: 240, y: 100 },
    ])
    expect(planSelectionAlignment(nodes, 'top')).toEqual([
      { id: '2', elementId: 2, x: 80, y: 20 },
      { id: '3', elementId: 3, x: 200, y: 20 },
    ])
    expect(planSelectionAlignment(nodes, 'middle')).toEqual([
      { id: '1', elementId: 1, x: 10, y: 70 },
      { id: '2', elementId: 2, x: 80, y: 60 },
      { id: '3', elementId: 3, x: 200, y: 65 },
    ])
    expect(planSelectionAlignment(nodes, 'bottom')).toEqual([
      { id: '1', elementId: 1, x: 10, y: 120 },
      { id: '3', elementId: 3, x: 200, y: 110 },
    ])
  })

  it('plans center-based distribution while preserving endpoints', () => {
    const nodes = [
      node('1', 0, 0, 100, 50),
      node('2', 300, 20, 100, 50),
      node('3', 700, 80, 100, 50),
    ]

    expect(planSelectionDistribution(nodes, 'horizontal')).toEqual([
      { id: '2', elementId: 2, x: 350, y: 20 },
    ])
    expect(planSelectionDistribution(nodes, 'vertical')).toEqual([
      { id: '2', elementId: 2, x: 300, y: 40 },
    ])
  })

  it('returns no updates for fewer than two selected elements', () => {
    expect(planSelectionAlignment([node('1', 0, 0)], 'left')).toEqual([])
    expect(planSelectionDistribution([node('1', 0, 0), node('2', 100, 0)], 'horizontal')).toEqual([])
  })

  it('finds visible element rects in viewport order', () => {
    const nodes = [
      node('3', 60, 80),
      node('2', 20, 20),
      node('1', 320, 20),
      { ...node('4', 40, 40), style: { pointerEvents: 'none' as const } },
      node('context-left', 0, 0, 100, 50, false, 'contextNeighborNode'),
    ]

    expect(visibleElementSelectionRects(nodes, { x: 0, y: 0, zoom: 1, width: 250, height: 200 }).map((rect) => rect.elementId))
      .toEqual([2, 3])
  })

  it('accepts marquee rectangles roughly matching the element shape', () => {
    expect(isElementShapedMarqueeRect({ width: 180, height: 85 })).toBe(true)
    expect(isElementShapedMarqueeRect({ width: 90, height: 45 })).toBe(true)
    expect(isElementShapedMarqueeRect({ width: 360, height: 170 })).toBe(true)
  })

  it('rejects marquee rectangles that are too small, too big, or the wrong aspect', () => {
    expect(isElementShapedMarqueeRect({ width: 0, height: 0 })).toBe(false)
    expect(isElementShapedMarqueeRect({ width: 40, height: 40 })).toBe(false)
    expect(isElementShapedMarqueeRect({ width: 1400, height: 170 })).toBe(false)
    expect(isElementShapedMarqueeRect({ width: 360, height: 45 })).toBe(false)
    expect(isElementShapedMarqueeRect({ width: 100, height: 160 })).toBe(false)
  })

  it('detects when a marquee overlaps an element node', () => {
    const element = node('1', 100, 100, 180, 85, false)

    expect(marqueeHitsElementRect({ x: 150, y: 120, width: 50, height: 50 }, [element])).toBe(true)
    expect(marqueeHitsElementRect({ x: 400, y: 400, width: 180, height: 85 }, [element])).toBe(false)
    expect(marqueeHitsElementRect({ x: 280, y: 100, width: 50, height: 50 }, [element])).toBe(false)
  })

  it('ignores non-element, hidden, and non-selectable nodes when testing overlap', () => {
    const nodes = [
      node('context-left', 100, 100, 180, 85, false, 'contextNeighborNode'),
      { ...node('2', 100, 100, 180, 85, false), style: { visibility: 'hidden' as const } },
      { ...node('3', 100, 100, 180, 85, false), selectable: false },
    ]

    expect(marqueeHitsElementRect({ x: 100, y: 100, width: 180, height: 85 }, nodes)).toBe(false)
  })
})
