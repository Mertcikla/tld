import { describe, expect, it } from 'vitest'
import type { RepositoryImpactOverlay, RepositoryImpactScene } from '../api/client'
import type { PlacedElement, ViewTreeNode } from '../types'
import { repositoryChangeScene, REPOSITORY_CHANGE_TAG } from './repositoryChangeScene'
import { applyChangeOverlays, CHANGE_OVERLAY_TAG } from '../components/ZUI/changeOverlay'
import { computeLayout } from '../components/ZUI/layout'

const view = (id: number, name = 'Map', children: ViewTreeNode[] = []): ViewTreeNode => ({ id, name, description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children })
const placement = (id: number, path: string): PlacedElement => ({ id, element_id: id, view_id: 1, position_x: 42, position_y: 84, name: path, kind: 'component', description: 'Existing description', technology: null, url: null, logo_url: null, technology_connectors: [], tags: ['original'], repo: '/repo', file_path: path, has_view: false, view_label: null })
const overlay = (change: 'added' | 'removed' | 'modified' | 'unchanged', path: string, distance: number, extra: Partial<RepositoryImpactOverlay> = {}): RepositoryImpactOverlay => ({ change, path, symbols: [], distance, ...extra })
const connector = (id: number, source: number, target: number) => ({ id, view_id: 1, source_element_id: source, target_element_id: target, label: null, description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: null, target_handle: null, created_at: '', updated_at: '' })

const scene: RepositoryImpactScene = {
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
}

describe('repositoryChangeScene', () => {
  it('retains the backend scene views and annotates only nodes within the blast radius', () => {
    const view1 = repositoryChangeScene(scene, { radius: 0 })
    expect(view1.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(view1.overlays[1]).toMatchObject({ change: 'modified', linesAdded: 3, linesRemoved: 2 })
    expect(view1.overlays[2]).toBeUndefined()
    expect(view1.overlays[3]).toBeUndefined()

    const view2 = repositoryChangeScene(scene, { radius: 1 })
    expect(view2.overlays[2]?.change).toBe('unchanged')
    expect(view2.overlays[3]).toBeUndefined()
  })

  it('tags direct changes and leaves blast-radius context unhighlighted', () => {
    const rendered = repositoryChangeScene(scene, { radius: 1 })
    const annotated = applyChangeOverlays(computeLayout(rendered.data), rendered.overlays)
    const changed = annotated.groups[0].nodes.find((node) => node.elementId === 1)!
    const context = annotated.groups[0].nodes.find((node) => node.elementId === 2)!
    expect(changed.tags).toContain(CHANGE_OVERLAY_TAG)
    expect(context.tags).not.toContain(CHANGE_OVERLAY_TAG)
    expect(context.changeOverlay?.change).toBe('unchanged')
    expect(rendered.data.views[1].placements.find((element) => element.element_id === 2)!.tags).not.toContain(REPOSITORY_CHANGE_TAG)
  })

  it('plain mode hides non-impacted elements and their connectors while keeping neighbours', () => {
    const rendered = repositoryChangeScene(scene, { radius: 1, plain: true })
    expect(rendered.data.views[1].placements.map((element) => element.file_path)).toEqual(['a.go', 'b.go'])
    expect(rendered.data.views[1].connectors?.map((item) => item.id)).toEqual([11])
    expect(rendered.overlays[1]?.change).toBe('modified')
    expect(rendered.overlays[2]?.change).toBe('unchanged')
    expect(rendered.overlays[3]).toBeUndefined()
  })

  it('plain mode keeps the container elements that reach impacted nested views', () => {
    const linked = (id: number, from: number, to: number, element: number) => ({ id, element_id: element, from_view_id: from, to_view_id: to, to_view_name: `view-${to}`, relation_type: 'child' })
    const nested: RepositoryImpactScene = {
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
    const rendered = repositoryChangeScene(nested, { radius: 0, plain: true })
    expect(rendered.data.tree[0].children?.[0].children?.[0].id).toBe(12)
    expect(rendered.data.views[10].placements.map((element) => element.element_id)).toEqual([100])
    expect(rendered.data.views[11].placements.map((element) => element.element_id)).toEqual([200])
    expect(rendered.data.views[12].placements.map((element) => element.file_path)).toEqual(['a.go'])
    expect(computeLayout(rendered.data).groups[0].nodes[0].elementId).toBe(100)
  })

  it('keeps backend-assembled transient placements and annotates them', () => {
    const transient = placement(-1, 'new.go')
    const withTransient: RepositoryImpactScene = {
      ...scene,
      views: { ...scene.views, 1: { placements: [...scene.views[1].placements, transient], connectors: [] } },
      overlays: { ...scene.overlays, [-1]: overlay('added', 'new.go', 0) },
    }
    const rendered = repositoryChangeScene(withTransient, { radius: 0 })
    const added = rendered.data.views[1].placements.find((element) => element.element_id === -1)!
    expect(added.tags).toContain(REPOSITORY_CHANGE_TAG)
    expect(rendered.overlays[-1].change).toBe('added')
  })
})
