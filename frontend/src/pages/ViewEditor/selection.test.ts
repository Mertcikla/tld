import { describe, expect, it } from 'vitest'
import type { Node as RFNode } from 'reactflow'
import {
  planSelectionAlignment,
  planSelectionDistribution,
  selectedElementIds,
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
})
