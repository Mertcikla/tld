import { describe, expect, it } from 'vitest'
import type { ViewLayer } from '../types'
import { createElementGroupTag, elementGroupTagForLayer, isElementGroupTag } from './elementGroups'

const layer = (tags: string[]): ViewLayer => ({
  id: 1,
  diagram_id: 1,
  name: 'Payments',
  tags,
  color: '#4299E1',
})

describe('element group tags', () => {
  it('creates a unique UUID-backed marker and recognizes only that format', () => {
    const first = createElementGroupTag()
    const second = createElementGroupTag()

    expect(first).toMatch(/^group:[0-9a-f-]{36}$/i)
    expect(second).not.toBe(first)
    expect(isElementGroupTag(first)).toBe(true)
    expect(isElementGroupTag('group:payments')).toBe(false)
  })

  it('treats a layer as an element group only when it contains one marker tag', () => {
    const marker = createElementGroupTag()

    expect(elementGroupTagForLayer(layer([marker]))).toBe(marker)
    expect(elementGroupTagForLayer(layer([marker, 'payments']))).toBeNull()
    expect(elementGroupTagForLayer(layer(['payments']))).toBeNull()
  })
})
