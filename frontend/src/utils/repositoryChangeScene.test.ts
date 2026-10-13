import { describe, expect, it } from 'vitest'
import type { RepositoryImpactOverlay, RepositoryImpactProvenance, RepositoryImpactScene } from '../api/client'
import type { PlacedElement, ViewTreeNode } from '../types'
import { repositoryChangeScene } from './repositoryChangeScene'
import { applyChangeOverlays } from '../components/ZUI/changeOverlay'
import { computeLayout } from '../components/ZUI/layout'

const view = (id: number, name = 'Map', children: ViewTreeNode[] = []): ViewTreeNode => ({ id, name, description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children })
const placement = (id: number, path: string): PlacedElement => ({ id, element_id: id, view_id: 1, position_x: 42, position_y: 84, name: path, kind: 'component', description: 'Existing description', technology: null, url: null, logo_url: null, technology_connectors: [], tags: ['original'], repo: '/repo', file_path: path, has_view: false, view_label: null })
const overlay = (change: 'added' | 'removed' | 'modified' | 'unchanged', path: string, distance: number, extra: Partial<RepositoryImpactOverlay> = {}): RepositoryImpactOverlay => ({ change, path, symbols: [], symbolDetails: [], distance, reason: 'direct', ...extra })
const connector = (id: number, source: number, target: number) => ({ id, view_id: 1, source_element_id: source, target_element_id: target, label: null, description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: null, target_handle: null, created_at: '', updated_at: '' })

const provenance = (origin: 'authored' | 'mapped' | 'raw'): RepositoryImpactProvenance => ({
  key: '', viewId: 1, resourceId: 0, name: '',
  origin, grounding: 'file', role: 'direct', paths: [],
  distance: 0,
})

const scene: RepositoryImpactScene = {
  repositoryId: 'repo',
  comparisonKey: 'before..after',
  version: '1',
  schemaVersion: '2',
  fromGitRevision: 'before',
  toGitRevision: 'after',
  tree: [view(1, 'Map')],
  views: {
    1: { placements: [placement(1, 'a.go'), placement(2, 'b.go'), placement(3, 'c.go')], connectors: [connector(11, 1, 2), connector(12, 1, 3)] },
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
    const rendered = repositoryChangeScene(scene)
    expect(rendered.data.tree.map((tree) => tree.id)).toEqual([1])
    expect(rendered.overlays[1]).toMatchObject({ change: 'modified', linesAdded: 3, linesRemoved: 2 })
    expect(rendered.overlays[2]?.change).toBe('unchanged')
    expect(rendered.overlays[3]?.change).toBe('unchanged')
  })

  it('tags direct changes and blast-radius context with change-state tags', () => {
    const rendered = repositoryChangeScene(scene)
    const annotated = applyChangeOverlays(computeLayout(rendered.data), rendered.overlays)
    const changed = annotated.groups[0].nodes.find((node) => node.elementId === 1)!
    const context = annotated.groups[0].nodes.find((node) => node.elementId === 2)!
    // Modified reads through the +a/−r lines badge, not a tag.
    expect(changed.tags).not.toContain('modified')
    expect(changed.changeOverlay?.change).toBe('modified')
    expect(context.tags).toContain('unchanged')
    expect(context.changeOverlay?.change).toBe('unchanged')
    expect(rendered.data.views[1].placements.find((element) => element.element_id === 1)!.tags).not.toContain('modified')
  })

  it('preserves backend-selected containers and dependency neighbours', () => {
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
    const rendered = repositoryChangeScene(nested)
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
      placements: { 'placement|1|-1': provenance('raw') },
    }
    const rendered = repositoryChangeScene(withTransient)
    const added = rendered.data.views[1].placements.find((element) => element.element_id === -1)!
    expect(added.tags).toContain('added')
    expect(added.tags).toContain('augmented')
    expect(rendered.overlays[-1].change).toBe('added')
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
    const rendered = repositoryChangeScene(nested)
    expect(rendered.data.views[52]?.placements.map((element) => element.element_id)).toEqual([8])
    expect(rendered.data.navigations).toContainEqual(expect.objectContaining({ from_view_id: 51, to_view_id: 52 }))
    const layout = computeLayout(rendered.data)
    const parent = layout.groups.flatMap((group) => group.nodes).find((node) => node.elementId === 7)!
    expect(parent.linkedDiagramId).toBe(52)
    expect(parent.children.map((child) => child.elementId)).toEqual([8])
  })

  it('standard mode keeps containers that reach retained child views so the canvas never blanks', () => {
    const linked = (id: number, from: number, to: number, element: number) => ({ id, element_id: element, from_view_id: from, to_view_id: to, to_view_name: `view-${to}`, relation_type: 'child' })
    // Only the leaf view is authored; the ancestors hold overlay-less
    // containers. Dropping those would orphan the nesting: the root would
    // render zero nodes and the canvas would go blank.
    const hierarchy: RepositoryImpactScene = {
      ...scene,
      tree: [view(1, 'Workspace', [view(2, 'Mid', [view(3, 'Leaf')])])],
      views: {
        1: { placements: [{ ...placement(100, ''), file_path: null, has_view: true, name: 'mid' }], connectors: [] },
        2: { placements: [{ ...placement(200, ''), file_path: null, has_view: true, name: 'leaf' }], connectors: [] },
        3: { placements: [placement(1, 'a.go')], connectors: [] },
      },
      navigations: [linked(1, 1, 2, 100), linked(2, 2, 3, 200)],
      fallbackViewId: 0,
      overlays: { 1: overlay('modified', 'a.go', 0) },
      authoredViewIds: [3],
    }
    const rendered = repositoryChangeScene(hierarchy)
    expect(rendered.data.views[1]?.placements.map((element) => element.element_id)).toEqual([100])
    expect(rendered.data.views[2]?.placements.map((element) => element.element_id)).toEqual([200])
    const nodes = applyChangeOverlays(computeLayout(rendered.data), rendered.overlays).groups.flatMap((group) => group.nodes)
    expect(nodes.length).toBeGreaterThan(0)
    expect(rendered.overlays[100]).toBeUndefined()
  })
})

it('passes canonical scenes through without a second semantic pruning policy', () => {
  const canonical: RepositoryImpactScene = {
    ...scene, schemaVersion: '2',
    views: { 1: { placements: [placement(1, 'a.go'), placement(2, '')], connectors: [connector(11, 1, 2)] } },
    tree: [view(1, 'Architecture')],
    authoredViewIds: [1],
  }
  const rendered = repositoryChangeScene(canonical)
  // Plain selection is owned by the backend; the adapter must preserve its
  // final coverage, conceptual context and declared relationships exactly.
  expect(rendered.data.views[1].placements.map((element) => element.element_id)).toEqual([1, 2])
  expect(rendered.data.views[1].connectors?.map((edge) => edge.id)).toEqual([11])
  expect(rendered.data.tree).toEqual(canonical.tree)
})

it('uses recorded provenance rather than IDs or map-generated tags', () => {
  const canonical: RepositoryImpactScene = {
    ...scene,
    views: { 1: { placements: [
      { ...placement(-1, 'concept'), tags: ['map-generated'] },
      placement(2, 'mapped.go'), placement(3, 'raw.go'),
    ], connectors: [] } },
    placements: {
      'placement|1|-1': provenance('authored'),
      'placement|1|2': provenance('mapped'),
      'placement|1|3': provenance('raw'),
    },
  }
  const rendered = repositoryChangeScene(canonical)
  expect(rendered.data.views[1].placements.map((element) => element.tags)).toEqual([
    ['authored'], ['original', 'generated', 'unchanged'], ['original', 'augmented', 'unchanged'],
  ])
  expect(canonical.views[1].placements[0].tags).toEqual(['map-generated'])
})
