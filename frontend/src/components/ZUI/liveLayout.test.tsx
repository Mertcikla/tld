import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { followViewport } from './followViewport'
import { interpolateLayout, useAnimatedLayout } from './useAnimatedLayout'
import { buildSceneGraph } from './sceneGraph'
import { updateScene } from './layoutEngine'
import { hitTestZUIRenderedNode } from './hitTest'
import type { LayoutNode, ZUILayout } from './types'

function node(id: number, x: number): LayoutNode {
  return { id: `d1-o${id}`, elementId: id, diagramId: 1, label: `File ${id}`, type: 'component', logoUrl: null, description: null,
    technology: null, tags: [], ancestorElementIds: [], pathElementIds: [], worldX: x, worldY: 200, worldW: 100, worldH: 60,
    children: [], childScale: 1, childOffsetX: 0, childOffsetY: 0, edgesOut: [] }
}
function layout(nodes: LayoutNode[]): ZUILayout {
  return { groups: [{ diagramId: 1, label: 'Changes', description: null, level: 0, levelLabel: null,
    worldX: -10000, worldY: 0, worldW: 40000, worldH: 600, diagramW: 40000, diagramH: 600, diagramX: 0, diagramY: 0, nodes, edges: [] }],
    bbox: { minX: -10000, minY: 0, maxX: 30000, maxY: 600 } }
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('live layout transitions', () => {
  it('interpolates stable nodes and fades additions and removals without mutating inputs', () => {
    const before = layout([node(1, 0), node(2, 500)])
    const after = layout([node(1, 200), node(3, 800)])
    const middle = interpolateLayout(before, after, 0.5)
    const [moved, added, removed] = middle.groups[0].nodes
    expect(moved.worldX).toBe(100)
    expect(added.transitionOpacity).toBe(0.5)
    expect(removed.transitionOpacity).toBe(0.5)
    expect(removed.transitionLeaving).toBe(true)
    expect(before.groups[0].nodes[0].worldX).toBe(0)
    expect(after.groups[0].nodes[0].worldX).toBe(200)
    expect(interpolateLayout(before, after, 1)).toBe(after)
    const graph = buildSceneGraph(middle)
    updateScene(graph, { x: 0, y: 0, zoom: 1 }, 1000, 600, { start: 200, end: 600 })
    expect(graph.nodeById.get(removed.id)?.state.parentAlpha).toBe(0.5)
    expect(hitTestZUIRenderedNode(550, 230, middle.groups, { x: 0, y: 0, zoom: 1 }, { start: 200, end: 600 })).toBeNull()
  })

  it('retargets an interrupted animation from the currently displayed layout', () => {
    let now = 0, id = 0
    const frames = new Map<number, FrameRequestCallback>()
    vi.spyOn(performance, 'now').mockImplementation(() => now)
    vi.stubGlobal('window', { matchMedia: () => ({ matches: false }) })
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++id, callback); return id })
    vi.stubGlobal('cancelAnimationFrame', (frame: number) => frames.delete(frame))
    let displayed!: ZUILayout
    const Harness = ({ target }: { target: ZUILayout }) => { displayed = useAnimatedLayout(target, true); return null }
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(<Harness target={layout([node(1, 0)])} />) })
    act(() => renderer.update(<Harness target={layout([node(1, 200)])} />))
    act(() => { now = 180; const [frame, callback] = [...frames.entries()][0]; frames.delete(frame); callback(now) })
    const halfway = displayed.groups[0].nodes[0].worldX
    expect(halfway).toBeGreaterThan(0)
    expect(halfway).toBeLessThan(200)
    act(() => renderer.update(<Harness target={layout([node(1, 400)])} />))
    expect(displayed.groups[0].nodes[0].worldX).toBe(halfway)
    act(() => { const [frame, callback] = [...frames.entries()][0]; frames.delete(frame); callback(now) })
    expect(displayed.groups[0].nodes[0].worldX).toBe(halfway)
    act(() => renderer.unmount())
    expect(frames.size).toBe(0)
  })

  it('respects reduced motion', () => {
    vi.stubGlobal('window', { matchMedia: () => ({ matches: true }) })
    vi.stubGlobal('requestAnimationFrame', vi.fn())
    let displayed!: ZUILayout
    const Harness = ({ target }: { target: ZUILayout }) => { displayed = useAnimatedLayout(target, true); return null }
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(<Harness target={layout([node(1, 0)])} />) })
    const target = layout([node(1, 300)])
    act(() => renderer.update(<Harness target={target} />))
    expect(displayed).toBe(target)
    expect(requestAnimationFrame).not.toHaveBeenCalled()
    act(() => renderer.unmount())
  })
})

describe('follow viewport', () => {
  const camera = { x: 0, y: 0, zoom: 1 }
  const refs = [{ viewId: 1, elementId: 1 }]
  it('keeps comfortably visible changes stationary', () => {
    expect(followViewport(layout([node(1, 200)]).groups, refs, camera, 800, 600, 8)).toBeNull()
  })
  it('pans offscreen changes into view without unnecessary zoom changes, including rebased cameras', () => {
    const next = followViewport(layout([node(1, 1600)]).groups, refs, { x: 400, y: 300, originX: 400, originY: 300, zoom: 1 }, 800, 600, 8)
    expect(next?.zoom).toBe(1)
    expect(next!.x + 1650).toBe(400)
  })
  it('follows the nearest cluster instead of zooming out to a repository-wide change', () => {
    const next = followViewport(layout([node(1, 1600), node(2, 20000)]).groups, [...refs, { viewId: 1, elementId: 2 }], camera, 800, 600, 8)
    expect(next?.zoom).toBe(1)
    expect(next!.x + 1650).toBe(400)
  })
  it('ignores filtered targets and can use the previous position of a removed placement', () => {
    expect(followViewport([], refs, camera, 800, 600, 8)).toBeNull()
    expect(followViewport([], refs, camera, 800, 600, 8, layout([node(1, 1600)]).groups)?.zoom).toBe(1)
  })
})
