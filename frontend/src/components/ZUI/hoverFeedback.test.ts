import { describe, expect, it } from 'vitest'
import { getHoverScreenRect } from './hoverFeedback'
import type { HoveredItem } from './types'

const view = { x: 500, y: 300, originX: 1e9, originY: 2e9, zoom: 2 }
const edge: HoveredItem = {
  type: 'edge', data: { sourceId: 'a', targetId: 'b', label: 'calls', diagramId: 1 },
  absX: 1e9 + 20, absY: 2e9 + 30,
  route: [{ x: 1e9, y: 2e9 }, { x: 1e9 + 100, y: 2e9 + 50 }],
}

describe('ZUI hover feedback', () => {
  it('keeps the connector anchor beside the pointer with a rebased camera', () => {
    expect(getHoverScreenRect(edge, view)).toEqual({ sx: 540, sy: 360, sw: 1, sh: 1 })
    expect(getHoverScreenRect(null, view)).toBeNull()
  })

})
