import { describe, expect, it } from 'vitest'
import type { WatchDiff } from '../api/client'
import type { ExploreData } from '../types'
import {
  buildWatchDiffLocations,
  changedResourceCount,
  formatDiagramResourceSummary,
  formatStatLine,
  isWatchDiffChange,
  summarizeWatchDiffs,
  totalResourceCount,
} from './watchDiffSummary'

function diff(overrides: Partial<WatchDiff>): WatchDiff {
  return {
    id: 1,
    version_id: 1,
    owner_type: 'symbol',
    owner_key: 'go:main.go:function:Main',
    change_type: 'initialized',
    resource_type: 'element',
    resource_id: 101,
    ...overrides,
  }
}

describe('watch diff summary', () => {
  const data: ExploreData = {
    tree: [{
      id: 1,
      owner_element_id: null,
      name: 'Root',
      description: null,
      level_label: null,
      level: 1,
      depth: 0,
      created_at: '2024-01-01',
      updated_at: '2024-01-01',
      parent_view_id: null,
      children: [],
    }],
    navigations: [],
    views: {
      1: {
        placements: [{
          id: 1,
          view_id: 1,
          element_id: 101,
          position_x: 0,
          position_y: 0,
          name: 'Main',
          description: null,
          kind: 'service',
          technology: null,
          url: null,
          logo_url: null,
          technology_connectors: [],
          tags: [],
          has_view: false,
          view_label: null,
        }],
        connectors: [],
      },
    },
  }

  it('does not describe clean initial resources as changed', () => {
    const summary = summarizeWatchDiffs([
      diff({ id: 1, change_type: 'initialized', resource_type: 'element', resource_id: 101 }),
      diff({ id: 2, change_type: 'initialized', resource_type: 'element', resource_id: 102 }),
      diff({ id: 3, change_type: 'initialized', resource_type: 'connector', resource_id: 201 }),
    ])

    expect(changedResourceCount(summary.elements)).toBe(0)
    expect(totalResourceCount(summary.elements)).toBe(2)
    expect(formatDiagramResourceSummary(summary)).toBe('3 initialized resources')
    expect(formatStatLine('element', summary.elements)).toBe('2 elements initialized')
    expect(isWatchDiffChange('initialized')).toBe(false)
    expect(buildWatchDiffLocations(data, [
      diff({ id: 1, change_type: 'initialized', resource_type: 'element', resource_id: 101 }),
    ])).toEqual([])
  })

  it('keeps changed and initialized resources separate in labels', () => {
    const summary = summarizeWatchDiffs([
      diff({ id: 1, change_type: 'updated', resource_type: 'element', resource_id: 101 }),
      diff({ id: 2, change_type: 'deleted', resource_type: 'connector', resource_id: 201 }),
      diff({ id: 3, change_type: 'initialized', resource_type: 'element', resource_id: 102 }),
    ])

    expect(formatDiagramResourceSummary(summary)).toBe('2 changed, 1 initialized resources')
    expect(formatStatLine('connector', summary.connectors)).toBe('1 connector changed')
    expect(formatStatLine('element', summary.elements)).toBe('1 element changed, 1 element initialized')
  })
})
