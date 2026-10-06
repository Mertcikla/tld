import { describe, expect, it } from 'vitest'
import type { RepositoryImpact } from '../api/client'
import type { ExploreData, PlacedElement, ViewTreeNode } from '../types'
import { repositoryChangeOverlay, REPOSITORY_CHANGE_TAG } from './repositoryChangeOverlay'
import { applyChangeOverlays, CHANGE_OVERLAY_TAG } from '../components/ZUI/changeOverlay'
import { computeLayout } from '../components/ZUI/layout'

const view = (id: number, name = 'Map'): ViewTreeNode => ({ id, name, description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children: [] })
const placement = (id: number, path: string, repo = '/repo'): PlacedElement => ({ id, element_id: id, view_id: 1, position_x: 42, position_y: 84, name: path, kind: 'component', description: 'Existing description', technology: null, url: null, logo_url: null, technology_connectors: [], tags: ['original'], repo, file_path: path, has_view: false, view_label: null })
const node = (path: string, change: 'added' | 'removed' | 'modified' | 'unchanged', elementId = 0) => ({ key: `file|${path}`, path, name: path, change, context: change === 'unchanged', elementId, x: 0, y: 0, symbols: { added: [], removed: [], modified: [] } })
const impact: RepositoryImpact = { repositoryId: 'repo', comparisonKey: 'live', viewId: 9, version: 'v1', radius: 1, maxRadius: 1, nodes: [node('a.go', 'modified'), node('b.go', 'unchanged', 2), node('new.go', 'added'), node('gone.go', 'removed')], edges: [], diff: { fromSnapshotId: 'base', toSnapshotId: 'head', fromGitRevision: 'old', toGitRevision: 'new', sources: [{ path: 'a.go', change: 'modified', fromHash: 'a', toHash: 'b', linesAdded: 3, linesRemoved: 2 }], facts: { added: 0, removed: 0, modified: 1 }, edgeFacts: { added: 0, removed: 0, modified: 0 } } }
const workspace: ExploreData = { tree: [view(1), view(2, 'Other repo'), view(9, 'repo impact · Live changes')], views: { 1: { placements: [placement(1, 'a.go'), placement(2, 'b.go')], connectors: [] }, 2: { placements: [placement(3, 'a.go', '/other')], connectors: [] }, 9: { placements: [placement(4, 'a.go')], connectors: [] } }, navigations: [] }

describe('repository change overlays', () => {
  it('retains remote-linked placements by repository ID, including clean comparisons', () => {
    const remoteWorkspace: ExploreData = { ...workspace, tree: [view(1), view(2)], views: {
      1: { placements: [{ ...placement(1, 'a.go', 'https://github.com/owner/repo'), repository_id: 'repo' }], connectors: [] },
      2: { placements: [{ ...placement(3, 'a.go', '/repo'), repository_id: 'other' }], connectors: [] },
    } }
    const changed = { ...impact, nodes: [node('a.go', 'modified')] }
    const scene = repositoryChangeOverlay(remoteWorkspace, changed, '/repo')
    expect(scene.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(scene.data.views[1].placements[0]).toMatchObject({ element_id: 1, position_x: 42, position_y: 84 })
    expect(scene.overlays[1]?.change).toBe('modified')
    const clean = repositoryChangeOverlay(remoteWorkspace, { ...changed, nodes: [] }, '/repo')
    expect(clean.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(clean.overlays).toEqual({})
  })
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
  it('does not highlight blast-radius context nodes as direct impact', () => {
    const scene = repositoryChangeOverlay(workspace, impact, '/repo')
    const annotated = applyChangeOverlays(computeLayout(scene.data), scene.overlays)
    const changed = annotated.groups[0].nodes.find((node) => node.elementId === 1)!
    const context = annotated.groups[0].nodes.find((node) => node.elementId === 2)!
    expect(changed.tags).toContain(CHANGE_OVERLAY_TAG)
    expect(context.tags).not.toContain(CHANGE_OVERLAY_TAG)
    expect(context.changeOverlay?.change).toBe('unchanged')
    expect(scene.data.views[1].placements.find((element) => element.element_id === 2)!.tags).not.toContain(REPOSITORY_CHANGE_TAG)
  })
  it('places added files in the pipeline-selected base view at the computed position', () => {
    const target: RepositoryImpact = { ...impact, viewId: 1, nodes: [node('a.go', 'modified'), { ...node('new.go', 'added'), x: 300, y: 120 }] }
    const scene = repositoryChangeOverlay(workspace, target, '/repo')
    expect(scene.data.tree.map((tree) => tree.id)).toEqual([1])
    const added = scene.data.views[1].placements.find((element) => element.file_path === 'new.go')!
    expect(added).toMatchObject({ view_id: 1, position_x: 300, position_y: 120 })
    expect(scene.overlays[added.element_id].change).toBe('added')
  })
  it('re-attaches transient connectors to the closest handles', () => {
    const target: RepositoryImpact = {
      ...impact,
      viewId: 1,
      nodes: [{ ...node('new.go', 'added'), x: 0, y: 0 }, { ...node('new2.go', 'added'), x: 400, y: 0 }],
      edges: [{ fromKey: 'file|new.go', toKey: 'file|new2.go', change: 'added', weight: 1 }],
    }
    const scene = repositoryChangeOverlay(workspace, target, '/repo')
    const connector = scene.data.views[1].connectors.find((item) => item.label === '1 dependencies')!
    expect(connector).toMatchObject({ source_handle: 'right', target_handle: 'left' })
  })
  it('falls back to a transient view when the selected view is unavailable', () => {
    const target: RepositoryImpact = { ...impact, viewId: 42, nodes: [...impact.nodes, { ...node('new.go', 'added'), x: 300, y: 120 }] }
    const scene = repositoryChangeOverlay(workspace, target, '/repo')
    expect(scene.data.tree.map((tree) => tree.id)).toEqual([1, -1])
    expect(scene.data.views[-1].placements.find((element) => element.file_path === 'new.go')).toMatchObject({ view_id: -1, position_x: 0, position_y: 0 })
  })
  it('clears annotations on a clean comparison while retaining the existing map', () => {
    const scene = repositoryChangeOverlay(workspace, { ...impact, nodes: [], edges: [], diff: { ...impact.diff, sources: [] } }, '/repo')
    expect(scene.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(scene.overlays).toEqual({})
    expect(scene.data.views[1].placements[0].tags).toEqual(['original'])
  })
  it('plain mode hides non-impacted elements and their connectors while keeping neighbours', () => {
    const connector = (id: number, source: number, target: number) => ({ id, view_id: 1, source_element_id: source, target_element_id: target, label: null, description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: null, target_handle: null, created_at: '', updated_at: '' })
    const plainWorkspace: ExploreData = {
      ...workspace,
      views: {
        1: { placements: [placement(1, 'a.go'), placement(2, 'b.go'), placement(3, 'c.go')], connectors: [connector(11, 1, 2), connector(12, 1, 3)] },
        9: { placements: [placement(5, 'a.go')], connectors: [] },
      },
    }
    const scene = repositoryChangeOverlay(plainWorkspace, impact, '/repo', { plain: true })
    expect(scene.data.views[1].placements.map((element) => element.file_path)).toEqual(['a.go', 'b.go'])
    expect(scene.data.views[1].connectors?.map((item) => item.id)).toEqual([11])
    expect(scene.overlays[1]?.change).toBe('modified')
    expect(scene.overlays[2]?.change).toBe('unchanged')
    expect(scene.overlays[3]).toBeUndefined()
  })
  it('plain mode keeps the container elements that reach impacted nested views', () => {
    const linked = (id: number, from: number, to: number, element: number) => ({ id, element_id: element, from_view_id: from, to_view_id: to, to_view_name: `view-${to}`, relation_type: 'child' })
    const nestedTree: ViewTreeNode[] = [{
      ...view(10, 'Workspace'),
      children: [{ ...view(11, 'repo map'), parent_view_id: 10, children: [{ ...view(12, 'Group'), parent_view_id: 11 }] }],
    }]
    const nested: ExploreData = {
      tree: nestedTree,
      views: {
        10: { placements: [{ ...placement(100, ''), file_path: null, has_view: true, name: 'repo map' }], connectors: [] },
        11: { placements: [{ ...placement(200, ''), file_path: null, has_view: true, name: 'Group' }], connectors: [] },
        12: { placements: [placement(1, 'a.go'), placement(3, 'c.go')], connectors: [] },
      },
      navigations: [linked(1, 10, 11, 100), linked(2, 11, 12, 200)],
    }
    const scene = repositoryChangeOverlay(nested, impact, '/repo', { plain: true })
    expect(scene.data.tree[0].children?.[0].children?.[0].id).toBe(12)
    expect(scene.data.views[10].placements.map((element) => element.element_id)).toEqual([100])
    expect(scene.data.views[11].placements.map((element) => element.element_id)).toEqual([200])
    expect(scene.data.views[12].placements.map((element) => element.file_path)).toEqual(['a.go'])
    expect(computeLayout(scene.data).groups[0].nodes[0].elementId).toBe(100)
  })
})
