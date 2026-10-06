import { describe, expect, it } from 'vitest'
import type { RepositoryImpact } from '../api/client'
import { fitBlastRadius } from './impactScope'

const node = (path: string, distance: number) => ({
  key: `file|${path}`, path, name: path, change: 'modified' as const, distance, elementId: 0, x: 0, y: 0,
  symbols: { added: [], removed: [], modified: [] },
})

const diagram = (nodes: ReturnType<typeof node>[], maxRadius = 3): RepositoryImpact => ({
  repositoryId: 'repo', comparisonKey: 'pair', viewId: 0, version: 'v1', maxRadius,
  nodes, edges: [],
  diff: { fromSnapshotId: 'base', toSnapshotId: 'head', fromGitRevision: 'a', toGitRevision: 'b', sources: [], facts: { added: 0, removed: 0, modified: 0 }, edgeFacts: { added: 0, removed: 0, modified: 0 } },
})

describe('fitBlastRadius', () => {
  it('keeps the widest radius when the diagram fits the budget', () => {
    const impact = diagram([node('a.go', 0), node('b.go', 1)])
    expect(fitBlastRadius(impact, 3)).toBe(3)
  })

  it('narrows the radius until the node count fits', () => {
    const impact = diagram([node('a.go', 0), node('b.go', 1), node('c.go', 2), node('d.go', 2)])
    expect(fitBlastRadius(impact, 3, 2)).toBe(1)
    expect(fitBlastRadius(impact, 3, 1)).toBe(0)
  })

  it('never drops below the direct changes', () => {
    const impact = diagram([node('a.go', 0), node('b.go', 0)])
    expect(fitBlastRadius(impact, 3, 1)).toBe(0)
  })

  it('disables the budget when maxNodes is zero', () => {
    expect(fitBlastRadius(diagram([node('a.go', 0)]), 3, 0)).toBe(3)
  })
})
