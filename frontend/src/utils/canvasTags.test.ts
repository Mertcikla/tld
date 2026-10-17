import { describe, expect, it } from 'vitest'
import { countCanvasTags, deriveCanvasTagState } from './canvasTags'

const nodes = [
  { id: 1, tags: ['api'] }, { id: 2, tags: ['storage'] },
  { id: 3, tags: ['api'] }, { id: 4, tags: [] },
]
const connectors = [
  { id: 10, source: 1, target: 2, tags: ['critical'] },
  { id: 11, source: 1, target: 3, tags: [] },
  { id: 12, source: 2, target: 4, tags: [] },
  { id: 13, source: 1, target: 2, tags: [] },
]

describe('canvas tag matching', () => {
  it('retains only a matching relationship and its context endpoints', () => {
    const state = deriveCanvasTagState(nodes, connectors, ['critical'])
    expect(state.visibleConnectorIds).toEqual(new Set([10]))
    expect(state.visibleNodeIds).toEqual(new Set([1, 2]))
    expect(state.contextNodeIds).toEqual(new Set([1, 2]))
    expect(state.matchedNodeIds.size).toBe(0)
    expect(state.matchedConnectorIds).toEqual(new Set([10]))
    expect(nodes[0].tags).toEqual(['api'])
  })

  it('preserves relationships between direct matches and combines selected tags with OR', () => {
    const state = deriveCanvasTagState(nodes, connectors, ['api', 'critical'])
    expect(state.visibleConnectorIds).toEqual(new Set([10, 11]))
    expect(state.matchedNodeIds).toEqual(new Set([1, 3]))
    expect(state.contextNodeIds).toEqual(new Set([2]))
    expect(state.visibleNodeIds.has(4)).toBe(false)
  })

  it('lets explicit hiding override matches without hiding endpoints for a hidden connector', () => {
    expect(deriveCanvasTagState(nodes, connectors, ['critical'], ['storage']).visibleConnectorIds.size).toBe(0)
    const hiddenEdge = deriveCanvasTagState(nodes, connectors, [], ['critical'])
    expect(hiddenEdge.visibleConnectorIds).toEqual(new Set([11, 12, 13]))
    expect(hiddenEdge.visibleNodeIds).toEqual(new Set([1, 2, 3, 4]))
    expect(deriveCanvasTagState(nodes, connectors, [], ['api']).visibleConnectorIds).toEqual(new Set([12]))
  })

  it('restores all resources on reset and ignores dangling endpoints', () => {
    const state = deriveCanvasTagState(nodes, [...connectors, { id: 99, source: 1, target: 99 }])
    expect(state.visibleConnectorIds).toEqual(new Set([10, 11, 12, 13]))
    expect(state.visibleNodeIds).toEqual(new Set([1, 2, 3, 4]))
    expect(state.contextNodeIds.size).toBe(0)
  })
})

describe('canvas tag counts', () => {
  it('counts each authored resource once and excludes reserved tags', () => {
    const group = 'group:12345678-1234-4234-a234-123456789012'
    expect(countCanvasTags([{ tags: ['api', 'api', group, '$ignored'] }], [
      { tags: ['api', 'critical', 'critical', group, '$ignored'] }, {},
    ])).toEqual({
      tagCounts: { api: 2, critical: 1 }, tagElementCounts: { api: 1 }, tagConnectorCounts: { api: 1, critical: 1 },
    })
  })
})
