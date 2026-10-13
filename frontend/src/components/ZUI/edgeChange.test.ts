import { describe, expect, it } from 'vitest'
import { connectorChangeFromTags, EDGE_CHANGE_META } from './edgeChange'

describe('connectorChangeFromTags', () => {
  it('reads the scene change tag', () => {
    expect(connectorChangeFromTags(['change:added'])).toBe('added')
    expect(connectorChangeFromTags(['other', 'change:removed'])).toBe('removed')
    expect(connectorChangeFromTags(['change:modified'])).toBe('modified')
  })

  it('claims nothing without a change tag', () => {
    expect(connectorChangeFromTags([])).toBeUndefined()
    expect(connectorChangeFromTags(['external'])).toBeUndefined()
    expect(connectorChangeFromTags(undefined)).toBeUndefined()
  })

  it('covers every change kind in metadata', () => {
    for (const kind of ['added', 'removed', 'modified'] as const) {
      expect(EDGE_CHANGE_META[kind].label).toBeTruthy()
      expect(EDGE_CHANGE_META[kind].color).toMatch(/^#/)
    }
    expect(EDGE_CHANGE_META.removed.dash.length).toBeGreaterThan(0)
    expect(EDGE_CHANGE_META.added.dash).toHaveLength(0)
  })
})
