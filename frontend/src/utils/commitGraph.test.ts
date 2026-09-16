import { describe, expect, it } from 'vitest'
import type { ImpactCommit } from '../api/client'
import { layoutCommitGraph, parseCommitRefs } from './commitGraph'

function commit(sha: string, parents: string[] = [], refs: string[] = []): ImpactCommit {
  return {
    sha,
    short_sha: sha.slice(0, 7),
    subject: sha,
    author: 'Test',
    date: '2024-01-01',
    parents,
    refs,
    author_email: 'test@example.com',
    body: '',
  }
}

describe('parseCommitRefs', () => {
  it('buckets head, branches, remotes and tags', () => {
    expect(parseCommitRefs(['HEAD -> main', 'origin/main', 'tag: v1.2'])).toEqual({
      isHead: true,
      branches: ['main'],
      remotes: ['origin/main'],
      tags: ['v1.2'],
    })
  })

  it('handles bare HEAD and plain branches', () => {
    expect(parseCommitRefs(['HEAD', 'feature'])).toEqual({
      isHead: true,
      branches: ['feature'],
      remotes: [],
      tags: [],
    })
  })

  it('ignores blanks', () => {
    expect(parseCommitRefs(['', '  '])).toEqual({ isHead: false, branches: [], remotes: [], tags: [] })
  })
})

describe('layoutCommitGraph', () => {
  it('keeps linear history on a single lane', () => {
    const layout = layoutCommitGraph([commit('c3', ['c2']), commit('c2', ['c1']), commit('c1')])
    expect(layout.rows.map((row) => row.lane)).toEqual([0, 0, 0])
    expect(layout.laneCount).toBe(1)
    expect(layout.edges).toEqual([
      { fromRow: 0, fromLane: 0, toRow: 1, toLane: 0, railLane: 0 },
      { fromRow: 1, fromLane: 0, toRow: 2, toLane: 0, railLane: 0 },
    ])
  })

  it('forks a second lane and joins it back on merge', () => {
    // newest first: merge <- feature + main
    const layout = layoutCommitGraph([
      commit('merge', ['main', 'feature']),
      commit('feature', ['base']),
      commit('main', ['base']),
      commit('base'),
    ])
    expect(layout.rows[0].lane).toBe(0)
    expect(layout.rows[0].isMerge).toBe(true)
    // First-parent rails remain reserved through the shared parent.
    expect(layout.rows[1].lane).toBe(1)
    expect(layout.rows[2].lane).toBe(0)
    expect(layout.rows[3].lane).toBe(0)
    expect(layout.laneCount).toBe(2)
    // Main stays on lane 0; the feature joins from its reserved rail.
    const join = layout.edges.find((edge) => edge.fromRow === 2)
    expect(join).toMatchObject({ fromLane: 0, toRow: 3, toLane: 0, railLane: 0 })
    expect(layout.edges.find((edge) => edge.fromRow === 1)).toMatchObject({ fromLane: 1, toLane: 0, railLane: 1 })
  })

  it('emits a stub when the parent is outside the list', () => {
    const layout = layoutCommitGraph([commit('c2', ['c1'])])
    expect(layout.edges).toEqual([{ fromRow: 0, fromLane: 0, toRow: null, toLane: 0, railLane: 0 }])
  })

  it('handles root commits without edges', () => {
    const layout = layoutCommitGraph([commit('c1')])
    expect(layout.edges).toEqual([])
    expect(layout.laneCount).toBe(1)
  })
})
