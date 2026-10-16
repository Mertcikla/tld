import { describe, expect, it } from 'vitest'
import type { RepositoryImpactScene } from '../api/client'
import type { Connector, PlacedElement, ViewTreeNode } from '../types'
import { CHANGE_HISTORY_FILES, materializedSceneChanges, mergeChangeHistory } from './repositoryChangeHistory'

function historyScene(version = 'v1'): RepositoryImpactScene {
  return {
    repositoryId: 'repo', comparisonKey: 'live', version, schemaVersion: '2', fromGitRevision: 'base', toGitRevision: 'head', fallbackViewId: 1, overlays: {}, authoredViewIds: [], navigations: [],
    tree: [{ id: 1, name: 'Application', parent_view_id: null, children: [] } as unknown as ViewTreeNode],
    views: { 1: { placements: [
      { element_id: 10, view_id: 1, name: 'Service', file_path: 'src/service.go', tags: [], position_x: 0, position_y: 0 } as unknown as PlacedElement,
      { element_id: 20, view_id: 1, name: 'Store', file_path: 'src/store.go', tags: [] } as unknown as PlacedElement,
    ], connectors: [{ id: 30, view_id: 1, source_element_id: 10, target_element_id: 20, label: 'reads', tags: [] } as unknown as Connector] } },
    placements: Object.fromEntries([10, 20].map((id) => [`placement|1|${id}`, { key: `placement|1|${id}`, viewId: 1, resourceId: id, name: id === 10 ? 'Service' : 'Store', origin: 'mapped', grounding: 'resolved', role: 'direct change', paths: [id === 10 ? 'src/service.go' : 'src/store.go'], distance: 0 }])),
    files: [{ path: 'src/service.go', baseHash: 'base', headHash: 'first' }, { path: 'src/store.go', baseHash: 'store', headHash: 'store' }] as unknown as RepositoryImpactScene['files'],
  }
}

describe('materialized change history', () => {
  it('seeds only annotated changes, never all existing resources', () => {
    const scene = historyScene()
    expect(materializedSceneChanges(null, scene)).toEqual([])
    scene.overlays[10] = { change: 'modified', path: 'src/service.go', reason: 'direct', distance: 0, symbols: [], symbolDetails: [] }
    expect(materializedSceneChanges(null, scene).map(({ key, action }) => [key, action])).toEqual([['element|10', 'modified']])
  })

  it('detects equal-sized body edits using hashes, without repeating cumulative changes', () => {
    const first = historyScene()
    const second = historyScene('v2')
    second.files![0].headHash = 'same-sized-body-edit'
    expect(materializedSceneChanges(first, second).map(({ key, action }) => [key, action])).toEqual([['element|10', 'modified']])
    const third = { ...second, version: 'v3', comparisonKey: 'another-snapshot' }
    expect(materializedSceneChanges(second, third)).toEqual([])
    expect(materializedSceneChanges(second, second)).toEqual([])
  })

  it('tracks element, view, and connector semantics, ignoring layout and timestamps', () => {
    const first = historyScene()
    const next = historyScene('v2')
    next.views[1].placements[0].position_x = 999
    next.tree[0].updated_at = 'different'
    next.views[1].connectors[0].updated_at = 'different'
    expect(materializedSceneChanges(first, next)).toEqual([])
    next.views[1].placements[0].name = 'API'
    next.tree[0].name = 'Backend'
    next.views[1].connectors[0].label = 'writes'
    expect(materializedSceneChanges(first, next).map(({ kind, action }) => [kind, action]).sort()).toEqual([['connector', 'modified'], ['element', 'modified'], ['view', 'modified']])
  })

  it('tracks file hashes through directory-scoped materialized elements', () => {
    const first = historyScene()
    first.views[1].placements[0].file_path = 'src'
    first.placements!['placement|1|10'].paths = ['src']
    const next = structuredClone(first)
    next.version = 'v2'
    next.files![1].headHash = 'directory-member-edited'
    const changes = materializedSceneChanges(first, next)
    expect(changes.filter((change) => change.kind === 'element')).toHaveLength(2)
    expect(changes.every((change) => change.path === 'src/store.go')).toBe(true)
  })

  it('records removals and their surviving parent context, including reversions', () => {
    const first = historyScene()
    const next = historyScene('v2')
    next.views[1].placements = next.views[1].placements.filter((item) => item.element_id !== 10)
    next.views[1].connectors = []
    next.files = next.files!.filter((file) => file.path !== 'src/service.go')
    const changes = materializedSceneChanges(first, next)
    expect(changes.find((entry) => entry.key === 'element|10')).toMatchObject({ action: 'removed', path: 'src/service.go', target: { viewId: 1 } })
    expect(changes.find((entry) => entry.kind === 'connector')?.action).toBe('removed')
    expect(changes.find((entry) => entry.kind === 'view')?.action).toBe('modified')
    expect(materializedSceneChanges(next, { ...first, version: 'v3' }).find((entry) => entry.key === 'element|10')?.action).toBe('added')
  })

  it('deduplicates an element placed in multiple views and retains cross-view connector endpoints', () => {
    const first = historyScene()
    first.views[2] = { placements: [first.views[1].placements[1]], connectors: [] }
    first.connectorTargets = { 'connector|1|30': { viewId: 2, elementId: 20 } }
    const next = structuredClone(first)
    next.version = 'v2'
    next.files![1].headHash = 'store-edit'
    next.views[1].connectors[0].label = 'updates'
    const changes = materializedSceneChanges(first, next)
    expect(changes.filter((entry) => entry.key === 'element|20')).toHaveLength(1)
    expect(changes.find((entry) => entry.kind === 'connector')?.target.elementIds).toEqual([{ viewId: 1, elementId: 10 }, { viewId: 2, elementId: 20 }])
  })

  it('coalesces repeated edits by file, keeps actual counts for large batches, and bounds file rows', () => {
    const first = historyScene()
    const next = historyScene('v2')
    next.files![0].headHash = 'edit'
    const changes = materializedSceneChanges(first, next)
    let history = mergeChangeHistory([], changes, 100)
    for (let n = 0; n < 100; n++) history = mergeChangeHistory(history, changes, 101 + n)
    expect(history).toHaveLength(1)
    expect(history[0]).toMatchObject({ updates: 101, updatedAt: 200, counts: { element: 1, view: 0, connector: 0 } })
    expect(history[0].changes).toHaveLength(1)
    expect(mergeChangeHistory(history, [], 999)).toBe(history)
    const large = Array.from({ length: 1000 }, (_, n) => ({ ...changes[0], key: `element|${n}` }))
    expect(mergeChangeHistory([], large, 201)[0].counts.element).toBe(1000)
    for (let n = 0; n < 100; n++) history = mergeChangeHistory(history, [{ ...changes[0], path: `${n}.go` }], 300 + n)
    expect(history).toHaveLength(CHANGE_HISTORY_FILES)
    expect(history[0].path).toBe('99.go')
  })

  it('starts a different repository from annotations instead of comparing unrelated maps', () => {
    const first = historyScene()
    const next = { ...historyScene('v2'), repositoryId: 'another-repo' }
    next.views[1].placements[0].name = 'Another service'
    expect(materializedSceneChanges(first, next)).toEqual([])
  })
})
