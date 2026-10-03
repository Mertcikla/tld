import { describe, expect, it } from 'vitest'
import type { RepositoryImpact } from '../api/client'
import type { ExploreData, PlacedElement, ViewTreeNode } from '../types'
import { repositoryChangeOverlay, REPOSITORY_CHANGE_TAG } from './repositoryChangeOverlay'
import { applyChangeOverlays } from '../components/ZUI/changeOverlay'
import { computeLayout } from '../components/ZUI/layout'

const view = (id: number, name = 'Map'): ViewTreeNode => ({ id, name, description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children: [] })
const placement = (id: number, path: string, repo = '/repo'): PlacedElement => ({ id, element_id: id, view_id: 1, position_x: 42, position_y: 84, name: path, kind: 'component', description: 'Existing description', technology: null, url: null, logo_url: null, technology_connectors: [], tags: ['original'], repo, file_path: path, has_view: false, view_label: null })
const node = (path: string, change: 'added' | 'removed' | 'modified' | 'unchanged', elementId = 0) => ({ key: `file|${path}`, path, name: path, change, context: change === 'unchanged', elementId, x: 0, y: 0, symbols: { added: [], removed: [], modified: [] } })
const impact: RepositoryImpact = { repositoryId: 'repo', comparisonKey: 'live', viewId: 9, version: 'v1', radius: 1, maxRadius: 1, nodes: [node('a.go', 'modified'), node('b.go', 'unchanged', 2), node('new.go', 'added'), node('gone.go', 'removed')], edges: [], diff: { fromSnapshotId: 'base', toSnapshotId: 'head', fromGitRevision: 'old', toGitRevision: 'new', sources: [{ path: 'a.go', change: 'modified', fromHash: 'a', toHash: 'b', linesAdded: 3, linesRemoved: 2 }], facts: { added: 0, removed: 0, modified: 1 }, edgeFacts: { added: 0, removed: 0, modified: 0 } } }
const workspace: ExploreData = { tree: [view(1), view(2, 'Other repo'), view(9, 'repo impact · Live changes')], views: { 1: { placements: [placement(1, 'a.go'), placement(2, 'b.go')], connectors: [] }, 2: { placements: [placement(3, 'a.go', '/other')], connectors: [] }, 9: { placements: [placement(4, 'a.go')], connectors: [] } }, navigations: [] }

describe('repository change overlays', () => {
  it('highlights existing placements without changing coordinates or workspace data', () => {
    const original = JSON.stringify(workspace)
    const scene = repositoryChangeOverlay(workspace, impact, '/repo/')
    expect(scene.data.tree.map((tree) => tree.id)).toEqual([1, -1])
    expect(scene.data.views[1].placements[0]).toMatchObject({ element_id: 1, position_x: 42, position_y: 84, tags: ['original', REPOSITORY_CHANGE_TAG] })
    expect(scene.overlays[1]).toMatchObject({ change: 'modified', linesAdded: 3, linesRemoved: 2 })
    expect(scene.overlays[2].change).toBe('unchanged')
    expect(JSON.stringify(workspace)).toBe(original)
  })
  it('uses transient IDs for unmapped additions and removals and annotates ZUI nodes', () => {
    const scene = repositoryChangeOverlay(workspace, impact, '/repo')
    expect(scene.data.views[-1].placements.map((element) => element.element_id)).toEqual([-1, -2])
    expect(scene.overlays[-1].change).toBe('added')
    expect(scene.overlays[-2].change).toBe('removed')
    const base = computeLayout(scene.data)
    const annotated = applyChangeOverlays(base, scene.overlays)
    expect(annotated.groups[0].nodes[0].changeOverlay?.linesAdded).toBe(3)
    expect(base.groups[0].nodes[0].changeOverlay).toBeUndefined()
    expect(annotated.groups[0].nodes[0].description).toBe('Existing description')
  })
  it('clears annotations on a clean comparison while retaining the existing map', () => {
    const scene = repositoryChangeOverlay(workspace, { ...impact, nodes: [], edges: [], diff: { ...impact.diff, sources: [] } }, '/repo')
    expect(scene.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(scene.overlays).toEqual({})
    expect(scene.data.views[1].placements[0].tags).toEqual(['original'])
  })
})
