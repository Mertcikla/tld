import { describe, expect, it } from 'vitest'
import { getHoverScreenRect, isWithinHoverSafeArea, preserveHoverAnchor } from './hoverFeedback'
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


describe('stable connector hover anchors', () => {
  it('keeps the first anchor while moving along the same connector', () => {
    const connector = { ...edge, data: { ...edge.data, id: 7 } }
    expect(preserveHoverAnchor(connector, { ...connector, absX: connector.absX + 200 })).toBe(connector)
    const other = { ...connector, data: { ...connector.data, id: 8 } }
    expect(preserveHoverAnchor(connector, other)).toBe(other)
    expect(preserveHoverAnchor(connector, null)).toBeNull()
  })

  it('distinguishes proxy groups from native connectors and other proxy groups', () => {
    const proxy: HoveredItem = { ...edge, data: { ...edge.data, isProxy: true, details: { key: 'proxy-a' } as NonNullable<typeof edge.data.details> } }
    expect(preserveHoverAnchor(proxy, { ...proxy, absX: proxy.absX + 200 })).toBe(proxy)
    expect(preserveHoverAnchor(proxy, edge)).toBe(edge)
    const other: HoveredItem = { ...proxy, data: { ...proxy.data, details: { ...proxy.data.details!, key: 'proxy-b' } } }
    expect(preserveHoverAnchor(proxy, other)).toBe(other)
  })
})

describe('safe hover corridor', () => {
  const bounds = { left: 100, top: 100, right: 300, bottom: 300 }
  it.each([
    [{ x: 0, y: 200 }, { x: 50, y: 180 }],
    [{ x: 400, y: 200 }, { x: 350, y: 180 }],
    [{ x: 200, y: 0 }, { x: 180, y: 50 }],
    [{ x: 200, y: 400 }, { x: 180, y: 350 }],
  ])('protects travel from %j toward a flipped popover', (anchor, pointer) => {
    expect(isWithinHoverSafeArea(pointer, anchor, bounds)).toBe(true)
  })

  it('allows small pointer jitter and entry near the popover border', () => {
    expect(isWithinHoverSafeArea({ x: -3, y: 203 }, { x: 0, y: 200 }, bounds)).toBe(true)
    expect(isWithinHoverSafeArea({ x: 95, y: 200 }, { x: 0, y: 200 }, bounds)).toBe(true)
    expect(isWithinHoverSafeArea({ x: 200, y: 200 }, { x: 0, y: 200 }, bounds)).toBe(true)
  })

  it('does not lock the rest of the canvas or protect hidden popovers', () => {
    expect(isWithinHoverSafeArea({ x: 50, y: 50 }, { x: 0, y: 200 }, bounds)).toBe(false)
    expect(isWithinHoverSafeArea({ x: -50, y: 200 }, { x: 0, y: 200 }, bounds)).toBe(false)
    expect(isWithinHoverSafeArea({ x: 50, y: 200 }, { x: 0, y: 200 }, { left: 100, right: 100, top: 100, bottom: 100 })).toBe(false)
  })
})
