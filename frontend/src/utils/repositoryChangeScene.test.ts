import { describe, expect, it } from 'vitest'
import type { RepositoryImpactOverlay, RepositoryImpactScene } from '../api/client'
import type { PlacedElement, ViewTreeNode } from '../types'
import { repositoryChangeScene, REPOSITORY_CHANGE_TAG } from './repositoryChangeScene'
import { applyChangeOverlays, CHANGE_OVERLAY_TAG } from '../components/ZUI/changeOverlay'
import { computeLayout } from '../components/ZUI/layout'

const view = (id: number, name = 'Map', children: ViewTreeNode[] = []): ViewTreeNode => ({ id, name, description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children })
const placement = (id: number, path: string): PlacedElement => ({ id, element_id: id, view_id: 1, position_x: 42, position_y: 84, name: path, kind: 'component', description: 'Existing description', technology: null, url: null, logo_url: null, technology_connectors: [], tags: ['original'], repo: '/repo', file_path: path, has_view: false, view_label: null })
const overlay = (change: 'added' | 'removed' | 'modified' | 'unchanged', path: string, distance: number, extra: Partial<RepositoryImpactOverlay> = {}): RepositoryImpactOverlay => ({ change, path, symbols: [], symbolDetails: [], distance, reason: 'direct', ...extra })
const connector = (id: number, source: number, target: number) => ({ id, view_id: 1, source_element_id: source, target_element_id: target, label: null, description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: null, target_handle: null, created_at: '', updated_at: '' })

const scene: RepositoryImpactScene = {
  repositoryId: 'repo',
  comparisonKey: 'before..after',
  version: '1',
  schemaVersion: '1',
  fromGitRevision: 'before',
  toGitRevision: 'after',
  tree: [view(1, 'Map', [view(9, 'repo impact · Live changes')])],
  views: {
    1: { placements: [placement(1, 'a.go'), placement(2, 'b.go'), placement(3, 'c.go')], connectors: [connector(11, 1, 2), connector(12, 1, 3)] },
    9: { placements: [placement(5, 'a.go')], connectors: [] },
  },
  navigations: [],
  fallbackViewId: -1,
  overlays: {
    1: overlay('modified', 'a.go', 0, { linesAdded: 3, linesRemoved: 2 }),
    2: overlay('unchanged', 'b.go', 1),
    3: overlay('unchanged', 'c.go', 2),
  },
  authoredViewIds: [],
}

describe('repositoryChangeScene', () => {
  it('retains the backend scene views and annotates every overlaid node', () => {
    const rendered = repositoryChangeScene(scene, {})
    expect(rendered.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(rendered.overlays[1]).toMatchObject({ change: 'modified', linesAdded: 3, linesRemoved: 2 })
    expect(rendered.overlays[2]?.change).toBe('unchanged')
    expect(rendered.overlays[3]?.change).toBe('unchanged')
  })

  it('tags direct changes and leaves blast-radius context unhighlighted', () => {
    const rendered = repositoryChangeScene(scene, {})
    const annotated = applyChangeOverlays(computeLayout(rendered.data), rendered.overlays)
    const changed = annotated.groups[0].nodes.find((node) => node.elementId === 1)!
    const context = annotated.groups[0].nodes.find((node) => node.elementId === 2)!
    expect(changed.tags).toContain(CHANGE_OVERLAY_TAG)
    expect(context.tags).not.toContain(CHANGE_OVERLAY_TAG)
    expect(context.changeOverlay?.change).toBe('unchanged')
    expect(rendered.data.views[1].placements.find((element) => element.element_id === 2)!.tags).not.toContain(REPOSITORY_CHANGE_TAG)
  })

  it('plain mode hides non-impacted elements and their connectors while keeping neighbours', () => {
    const rendered = repositoryChangeScene(scene, { plain: true })
    expect(rendered.data.views[1].placements.map((element) => element.file_path)).toEqual(['a.go', 'b.go', 'c.go'])
    expect(rendered.data.views[1].connectors?.map((item) => item.id)).toEqual([11, 12])
    expect(rendered.overlays[1]?.change).toBe('modified')
    expect(rendered.overlays[2]?.change).toBe('unchanged')
    expect(rendered.overlays[3]?.change).toBe('unchanged')
  })

  it('plain mode keeps the container elements that reach impacted nested views', () => {
    const linked = (id: number, from: number, to: number, element: number) => ({ id, element_id: element, from_view_id: from, to_view_id: to, to_view_name: `view-${to}`, relation_type: 'child' })
    const nested: RepositoryImpactScene = {
      ...scene,
      tree: [view(10, 'Workspace', [view(11, 'repo map', [view(12, 'Group')])])],
      views: {
        10: { placements: [{ ...placement(100, ''), file_path: null, has_view: true, name: 'repo map' }], connectors: [] },
        11: { placements: [{ ...placement(200, ''), file_path: null, has_view: true, name: 'Group' }], connectors: [] },
        12: { placements: [placement(1, 'a.go'), placement(3, 'c.go')], connectors: [] },
      },
      navigations: [linked(1, 10, 11, 100), linked(2, 11, 12, 200)],
      fallbackViewId: -1,
      overlays: { 1: overlay('modified', 'a.go', 0), 3: overlay('unchanged', 'c.go', 2) },
    }
    const rendered = repositoryChangeScene(nested, { plain: true })
    expect(rendered.data.tree[0].children?.[0].children?.[0].id).toBe(12)
    expect(rendered.data.views[10].placements.map((element) => element.element_id)).toEqual([100])
    expect(rendered.data.views[11].placements.map((element) => element.element_id)).toEqual([200])
    expect(rendered.data.views[12].placements.map((element) => element.file_path)).toEqual(['a.go', 'c.go'])
    expect(computeLayout(rendered.data).groups[0].nodes[0].elementId).toBe(100)
  })

  it('keeps backend-assembled transient placements and annotates them', () => {
    const transient = placement(-1, 'new.go')
    const withTransient: RepositoryImpactScene = {
      ...scene,
      views: { ...scene.views, 1: { placements: [...scene.views[1].placements, transient], connectors: [] } },
      overlays: { ...scene.overlays, [-1]: overlay('added', 'new.go', 0) },
    }
    const rendered = repositoryChangeScene(withTransient, {})
    const added = rendered.data.views[1].placements.find((element) => element.element_id === -1)!
    expect(added.tags).toContain(REPOSITORY_CHANGE_TAG)
    expect(rendered.overlays[-1].change).toBe('added')
  })

  it('pins authored views', () => {
    const rendered = repositoryChangeScene(scene, {})
    expect(rendered.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(rendered.overlays[1]?.change).toBe('modified')
  })

  it('marks provenance: transients augmented, authored views authored, the rest generated', () => {
    const withOrphan: RepositoryImpactScene = {
      ...scene,
      tree: [view(10, 'Workspace', [view(11, 'repo map'), view(51, 'My services'), view(-1, 'Changes')])],
      views: {
        10: { placements: [placement(100, 'top.go')], connectors: [] },
        11: { placements: [placement(1, 'a.go')], connectors: [] },
        51: { placements: [placement(7, 'svc/auth')], connectors: [] },
        '-1': { placements: [placement(-1, 'new.go')], connectors: [] },
      },
      overlays: {
        1: overlay('modified', 'a.go', 0),
        7: overlay('modified', 'svc/auth', 0),
        [-1]: overlay('added', 'new.go', 0),
      },
      authoredViewIds: [51],
    }
    const grounded = repositoryChangeScene(withOrphan, {})
    expect(grounded.provenance[7]).toBe('authored')
    expect(grounded.provenance[-1]).toBe('augmented')
    const degraded = repositoryChangeScene({ ...withOrphan, authoredViewIds: [] }, {})
    expect(degraded.provenance[1]).toBe('generated')
  })

  it('falls back to mapped when no authored view matched', () => {
    const rendered = repositoryChangeScene(scene, {})
    expect(rendered.data.tree.map((tree) => tree.id)).toEqual([1])
  })

  it('keeps the fallback Changes view so unlinked files stay visible', () => {
    const withOrphan: RepositoryImpactScene = {
      ...scene,
      tree: [view(10, 'Workspace', [view(11, 'repo map'), view(51, 'My services'), view(-1, 'Changes')])],
      views: {
        10: { placements: [placement(100, 'top.go')], connectors: [] },
        11: { placements: [placement(1, 'a.go')], connectors: [] },
        51: { placements: [placement(7, 'svc/auth')], connectors: [] },
        '-1': { placements: [placement(-1, 'new.go')], connectors: [] },
      },
      overlays: {
        1: overlay('modified', 'a.go', 0),
        7: overlay('modified', 'svc/auth', 0),
        [-1]: overlay('added', 'new.go', 0),
      },
      authoredViewIds: [51],
    }
    const rendered = repositoryChangeScene(withOrphan, {})
    expect(rendered.data.tree.map((tree) => tree.id)).toEqual([10])
    expect(rendered.data.tree[0].children?.map((child) => child.id)).toEqual([51, -1])
    expect(rendered.data.views[11]).toBeUndefined()
    expect(rendered.data.views[51].placements.map((element) => element.element_id)).toEqual([7])
    expect(rendered.data.views[-1].placements.map((element) => element.element_id)).toEqual([-1])
    expect(rendered.overlays[7]?.change).toBe('modified')
    expect(rendered.overlays[-1]?.change).toBe('added')
    expect(rendered.overlays[1]).toBeUndefined()
  })

  it('rescues transients from mapped views while pruning mapped context', () => {
    const transient1 = placement(-5, 'new.go')
    const transient2 = placement(-6, 'new2.go')
    const rescued: RepositoryImpactScene = {
      ...scene,
      tree: [view(10, 'Workspace', [view(11, 'repo map'), view(51, 'My services')])],
      views: {
        10: { placements: [], connectors: [] },
        11: {
          placements: [placement(2, 'b.go'), transient1, transient2, placement(4, 'd.go')],
          connectors: [connector(21, -5, -6), connector(22, -5, 2), connector(23, -5, 4)],
        },
        51: { placements: [placement(7, 'svc/auth')], connectors: [] },
      },
      overlays: {
        2: overlay('unchanged', 'b.go', 1),
        7: overlay('modified', 'svc/auth', 0),
        [-5]: overlay('added', 'new.go', 0),
        [-6]: overlay('added', 'new2.go', 0),
      },
      authoredViewIds: [51],
    }
    const rendered = repositoryChangeScene(rescued, {})
    expect(rendered.data.tree[0].children?.map((child) => child.id)).toEqual([11, 51])
    // Overlaid members stay (element 2 has context); the unoverlaid member
    // and its connector drop out.
    expect(rendered.data.views[11].placements.map((element) => element.element_id)).toEqual([2, -5, -6])
    expect(rendered.data.views[11].connectors?.map((item) => item.id)).toEqual([21, 22])
    expect(rendered.overlays[-5]?.change).toBe('added')
    expect(rendered.overlays[2]?.change).toBe('unchanged')
  })

  it('pins authored views and drops untouched mapped views', () => {
    const authored: RepositoryImpactScene = {
      ...scene,
      tree: [view(10, 'Workspace', [view(11, 'repo map'), view(51, 'My services')])],
      views: {
        10: { placements: [placement(100, 'top.go')], connectors: [] },
        11: { placements: [placement(1, 'a.go')], connectors: [] },
        51: { placements: [placement(7, 'svc/auth')], connectors: [] },
      },
      overlays: {
        1: overlay('modified', 'a.go', 0),
        7: overlay('modified', 'svc/auth', 0),
      },
      authoredViewIds: [51],
    }
    const rendered = repositoryChangeScene(authored, {})
    expect(rendered.data.tree[0].children?.map((child) => child.id)).toEqual([51])
    expect(rendered.data.views[11]).toBeUndefined()
    expect(rendered.overlays[7]?.change).toBe('modified')
    expect(rendered.overlays[1]).toBeUndefined()
  })

  it('retains drill-down child views of impacted elements with full output', () => {
    const linked = (id: number, from: number, to: number, element: number) => ({ id, element_id: element, from_view_id: from, to_view_id: to, to_view_name: `view-${to}`, relation_type: 'child' })
    // View 51 is authored and holds the impacted element 7, which owns the
    // non-authored child view 52. View 52 holds no impacted placements, so the
    // plain membership filter would drop it and element 7 would render flat.
    const nested: RepositoryImpactScene = {
      ...scene,
      tree: [view(51, 'My services', [view(52, 'Detail')])],
      views: {
        51: { placements: [{ ...placement(7, 'svc/auth'), has_view: true }], connectors: [] },
        52: { placements: [{ ...placement(8, 'internal'), file_path: null }], connectors: [] },
      },
      navigations: [linked(1, 51, 52, 7)],
      fallbackViewId: -1,
      overlays: { 7: overlay('modified', 'svc/auth', 0) },
      authoredViewIds: [51],
    }
    const rendered = repositoryChangeScene(nested, {})
    expect(rendered.data.views[52]?.placements.map((element) => element.element_id)).toEqual([8])
    expect(rendered.data.navigations).toContainEqual(expect.objectContaining({ from_view_id: 51, to_view_id: 52 }))
    const layout = computeLayout(rendered.data)
    const parent = layout.groups.flatMap((group) => group.nodes).find((node) => node.elementId === 7)!
    expect(parent.linkedDiagramId).toBe(52)
    expect(parent.children.map((child) => child.elementId)).toEqual([8])
  })
})
