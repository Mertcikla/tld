import { describe, expect, it } from 'vitest'
import {
  AUTO_LAYOUT_GAP,
  AUTO_LAYOUT_GRID_X,
  AUTO_LAYOUT_GRID_Y,
  AUTO_LAYOUT_NODE_H,
  AUTO_LAYOUT_NODE_W,
  findAutoPlacementPosition,
  idealPositionForSide,
  nodeRect,
  planIncrementalLayout,
  rectsOverlap,
  sideFacingPoint,
  snapPositionToGrid,
} from './autoLayout'

const rect = { x: 0, y: 0, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }

describe('idealPositionForSide', () => {
  it('places and centers a node on each facing side', () => {
    expect(idealPositionForSide(rect, 'right', AUTO_LAYOUT_NODE_W, AUTO_LAYOUT_NODE_H, AUTO_LAYOUT_GAP))
      .toEqual({ x: AUTO_LAYOUT_NODE_W + AUTO_LAYOUT_GAP, y: 0 })
    expect(idealPositionForSide(rect, 'left', AUTO_LAYOUT_NODE_W, AUTO_LAYOUT_NODE_H, AUTO_LAYOUT_GAP))
      .toEqual({ x: -(AUTO_LAYOUT_NODE_W + AUTO_LAYOUT_GAP), y: 0 })
    expect(idealPositionForSide(rect, 'bottom', AUTO_LAYOUT_NODE_W, AUTO_LAYOUT_NODE_H, AUTO_LAYOUT_GAP))
      .toEqual({ x: 0, y: AUTO_LAYOUT_NODE_H + AUTO_LAYOUT_GAP })
    expect(idealPositionForSide(rect, 'top', AUTO_LAYOUT_NODE_W, AUTO_LAYOUT_NODE_H, AUTO_LAYOUT_GAP))
      .toEqual({ x: 0, y: -(AUTO_LAYOUT_NODE_H + AUTO_LAYOUT_GAP) })
  })
})

describe('sideFacingPoint', () => {
  it('picks the side whose anchor faces the point', () => {
    expect(sideFacingPoint(rect, { x: 1000, y: 42 })).toBe('right')
    expect(sideFacingPoint(rect, { x: -1000, y: 42 })).toBe('left')
    expect(sideFacingPoint(rect, { x: 90, y: 1000 })).toBe('bottom')
    expect(sideFacingPoint(rect, { x: 90, y: -1000 })).toBe('top')
  })
})

describe('findAutoPlacementPosition', () => {
  it('returns the fallback when nothing is in the way', () => {
    const fallback = { x: 42, y: 24 }
    expect(findAutoPlacementPosition({ fallback })).toEqual(fallback)
  })

  it('avoids obstacles when there are no connectors', () => {
    const fallback = { x: 0, y: 0 }
    const obstacle = { x: 0, y: 0, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }
    const position = findAutoPlacementPosition({ fallback, obstacles: [obstacle] })
    expect(rectsOverlap({ ...position, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }, obstacle)).toBe(false)
  })

  it('anchors to a connector neighbor facing side', () => {
    const neighbor = { rect: { x: 0, y: 0, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }, side: 'right' as const }
    const position = findAutoPlacementPosition({ neighbors: [neighbor], fallback: { x: 0, y: 0 } })
    expect(position).toEqual({ x: AUTO_LAYOUT_NODE_W + AUTO_LAYOUT_GAP, y: 0 })
  })

  it('fans out along the perpendicular axis when the ideal slot is taken', () => {
    const neighbor = { rect: { x: 0, y: 0, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }, side: 'right' as const }
    const blocker = { x: AUTO_LAYOUT_NODE_W + AUTO_LAYOUT_GAP, y: 0, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }
    const position = findAutoPlacementPosition({ neighbors: [neighbor], obstacles: [blocker], fallback: { x: 0, y: 0 } })
    expect(position.x).toBe(AUTO_LAYOUT_NODE_W + AUTO_LAYOUT_GAP)
    expect(position.y).not.toBe(0)
  })
})

describe('planIncrementalLayout', () => {
  it('chains nodes along their connectors from the center', () => {
    const positions = planIncrementalLayout(
      [1, 2, 3],
      [
        { source: 1, target: 2, side: 'right' },
        { source: 2, target: 3, side: 'right' },
      ],
      { center: { x: 0, y: 0 } },
    )
    expect(positions.get(1)!.x).toBeLessThan(positions.get(2)!.x)
    expect(positions.get(2)!.x).toBeLessThan(positions.get(3)!.x)
    expect(positions.get(2)!.y).toBe(positions.get(1)!.y)
    expect(positions.get(3)!.y).toBe(positions.get(1)!.y)
  })

  it('places every node without overlapping existing obstacles', () => {
    const obstacle = { x: 0, y: 0, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }
    const positions = planIncrementalLayout(
      [1, 2],
      [{ source: 1, target: 2, side: 'right' }],
      { center: { x: 0, y: 0 }, obstacles: [obstacle] },
    )
    for (const position of positions.values()) {
      expect(rectsOverlap({ ...position, width: AUTO_LAYOUT_NODE_W, height: AUTO_LAYOUT_NODE_H }, obstacle)).toBe(false)
    }
  })
})

describe('nodeRect', () => {
  it('falls back to default element dimensions', () => {
    expect(nodeRect({ position: { x: 10, y: 20 } })).toEqual({
      x: 10,
      y: 20,
      width: AUTO_LAYOUT_NODE_W,
      height: AUTO_LAYOUT_NODE_H,
    })
  })
})

describe('snapPositionToGrid', () => {
  it('rounds positions onto the layout lattice', () => {
    expect(snapPositionToGrid({ x: 240, y: 160 })).toEqual({ x: AUTO_LAYOUT_GRID_X, y: AUTO_LAYOUT_GRID_Y })
    expect(snapPositionToGrid({ x: 260, y: 200 })).toEqual({
      x: AUTO_LAYOUT_GRID_X * 1,
      y: AUTO_LAYOUT_GRID_Y * 1,
    })
    expect(snapPositionToGrid({ x: 520, y: 480 })).toEqual({
      x: AUTO_LAYOUT_GRID_X * 2,
      y: AUTO_LAYOUT_GRID_Y * 3,
    })
  })

  it('snaps auto placement when a grid is provided', () => {
    const position = findAutoPlacementPosition({
      fallback: { x: 120, y: 90 },
      grid: { x: AUTO_LAYOUT_GRID_X, y: AUTO_LAYOUT_GRID_Y },
    })
    expect(position.x % AUTO_LAYOUT_GRID_X).toBe(0)
    expect(position.y % AUTO_LAYOUT_GRID_Y).toBe(0)
  })
})
