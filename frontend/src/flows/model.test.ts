import { describe, expect, it } from 'vitest'
import type { Node, Edge } from 'reactflow'
import { duplicateSteps, missingReference, newStep, playbackFrames, resolveFocus, stepLabel, type FlowDiagram, type StoryFlow } from './model'
import type { Connector } from '../types'
import { presentEditorFlow, presentZUIFlow } from './presentation'
import type { LayoutNode, ZUILayout } from '../components/ZUI/types'

const diagram: FlowDiagram = {
  elements: [{ element_id: 1, name: 'API' }, { element_id: 2, name: 'Database' }],
  connectors: [{ id: 10, source_element_id: 1, target_element_id: 2, label: 'Reads', direction: 'both' } as Connector],
}
const makeFlow = (steps: StoryFlow['steps']): StoryFlow => ({ id: 'flow', viewId: 4, revision: 1, name: 'Checkout', description: '', steps })

describe('flow stories', () => {
  it('uses current element names and connector labels when a step has no title', () => {
    const step = { ...newStep(diagram), elementIds: [1, 2], connectors: [{ id: 10, reverse: false }] }
    expect(stepLabel(step, diagram)).toBe('API, Database, Reads')
    expect(stepLabel({ ...step, title: '  ' }, diagram)).toBe('API, Database, Reads')
    expect(stepLabel({ ...step, title: 'Checkout' }, diagram)).toBe('Checkout')
    expect(stepLabel(step, { ...diagram, elements: [{ element_id: 1, name: 'Gateway' }, diagram.elements[1]] })).toBe('Gateway, Database, Reads')
    expect(step.title).toBe('')
  })

  it('uses endpoint names for unlabeled connections, including reversed highlights', () => {
    const unlabeled = { ...diagram, connectors: [{ ...diagram.connectors[0], label: null }] }
    const step = { ...newStep(diagram), connectors: [{ id: 10, reverse: false }] }
    expect(stepLabel(step, unlabeled)).toBe('API → Database')
    expect(stepLabel({ ...step, connectors: [{ id: 10, reverse: true }] }, unlabeled)).toBe('Database → API')
  })

  it('provides readable labels for text-only steps and unavailable highlights', () => {
    expect(stepLabel(newStep(diagram), diagram)).toBe('Text-only step')
    expect(stepLabel({ ...newStep(diagram), elementIds: [99], connectors: [{ id: 88, reverse: false }] }, diagram)).toBe('Element #99, Connection #88')
  })

  it('resolves reverse messages without changing the underlying connector', () => {
    const step = { ...newStep(diagram), connectors: [{ id: 10, reverse: true }] }
    const flow = makeFlow([step])
    const focus = resolveFocus(flow, playbackFrames(flow.steps)[0], diagram)
    expect(focus.elementIds).toEqual([1, 2])
    expect(focus.connectors).toEqual([{ id: 10, reverse: true }])
    expect(diagram.connectors[0].direction).toBe('both')
    expect(missingReference(step, { elements: diagram.elements, connectors: [] })).toBe(true)
    expect(missingReference(step, { elements: [], connectors: diagram.connectors })).toBe(true)
  })

  it('supports empty, element-only and mixed highlights without inheriting the previous step', () => {
    const empty = newStep(diagram)
    const elements = { ...newStep(diagram), elementIds: [1, 2] }
    const mixed = { ...elements, connectors: [{ id: 10, reverse: false }] }
    const flow = makeFlow([mixed, empty, elements])
    const frames = playbackFrames(flow.steps)
    expect(frames.map(frame => frame.steps)).toEqual([[mixed], [empty], [elements]])
    expect(resolveFocus(flow, frames[0], diagram)).toEqual({ viewId: 4, elementIds: [1, 2], connectors: mixed.connectors })
    expect(resolveFocus(flow, frames[1], diagram)).toEqual({ viewId: 4, elementIds: [], connectors: [] })
    expect(resolveFocus(flow, frames[2], diagram).connectors).toEqual([])
    expect(missingReference(empty, { elements: [], connectors: [] })).toBe(false)
    expect(missingReference(elements, { elements: [], connectors: [] })).toBe(true)
    const duplicated = duplicateSteps([mixed])
    expect(duplicated[0].id).not.toBe(mixed.id)
    duplicated[0].connectors[0].reverse = true
    duplicated[0].elementIds.push(3)
    duplicated[0].metadata.owner = 'Changed'
    expect(mixed.connectors[0].reverse).toBe(false)
    expect(mixed.elementIds).toEqual([1, 2])
    expect(mixed.metadata).toEqual({})
  })

  it('keeps editor geometry and directions intact on entry and exit', () => {
    const nodes: Node[] = [1, 2, 3].map(id => ({ id: String(id), position: { x: id * 100, y: 0 }, data: {} }))
    const edges: Edge<{ direction: string }>[] = [{ id: '10', source: '1', target: '2', label: 'Reads', data: { direction: 'both' } }]
    const focus = resolveFocus(makeFlow([{ ...newStep(diagram), connectors: [{ id: 10, reverse: true }] }]), undefined, diagram)
    const shown = presentEditorFlow(nodes, edges, focus)
    expect(shown.nodes[0].position).toBe(nodes[0].position)
    expect(shown.nodes[2].style?.opacity).toBe(0.18)
    expect(shown.edges[0].data?.direction).toBe('backward')
    expect(shown.edges[0].label).toBe('Reads')
    expect(presentEditorFlow(nodes, edges, { viewId: 4, elementIds: [], connectors: [] })).toEqual({ nodes, edges })
    expect(edges[0].data?.direction).toBe('both')
    expect(presentEditorFlow(nodes, edges, null)).toEqual({ nodes, edges })
  })

  it('highlights nested ZUI placements only in the flow home view', () => {
    const node = (id: number, viewId: number, children: LayoutNode[] = []): LayoutNode => ({
      id: `d${viewId}-o${id}`, elementId: id, diagramId: viewId, children, edgesOut: [],
      worldX: 0, worldY: 0, worldW: 100, worldH: 100, label: 'Node', type: 'service', logoUrl: null,
      description: null, technology: null, tags: [], ancestorElementIds: [], pathElementIds: [], childScale: 0.1, childOffsetX: 0, childOffsetY: 0,
    })
    const child = node(1, 4)
    child.edgesOut = [{ id: 10, targetId: 'd4-o2', label: 'Reads', direction: 'forward', sourceHandle: null, targetHandle: null, type: 'straight' }]
    const original: ZUILayout = { bbox: { minX: 0, minY: 0, maxX: 100, maxY: 100 }, groups: [{
      diagramId: 1, label: 'Root', description: null, level: 0, levelLabel: null, worldX: 0, worldY: 0, worldW: 100, worldH: 100,
      diagramX: 0, diagramY: 0, diagramW: 100, diagramH: 100, nodes: [node(9, 1, [child, node(2, 4)]), node(1, 1)], edges: [],
    }] }
    const focus = resolveFocus(makeFlow([{ ...newStep(diagram), connectors: [{ id: 10, reverse: true }] }]), undefined, diagram)
    const shown = presentZUIFlow(original, focus)
    expect(shown.groups[0].nodes[0].tagHoverState).toBe('match')
    expect(shown.groups[0].nodes[0].children[0].edgesOut[0].direction).toBe('backward')
    expect(shown.groups[0].nodes[1].tagHoverState).toBe('inactive')
    expect(child.edgesOut[0].direction).toBe('forward')
    expect(presentZUIFlow(original, null)).toBe(original)
  })
})

describe('flow control extensions', () => {
  const a = { ...newStep(diagram), id: 'a', elementIds: [1] }
  const b = { ...newStep(diagram), id: 'b', elementIds: [2] }
  const branch = { ...newStep(diagram), id: 'branch', branches: { mode: 'alternate', paths: [{ id: 'pa', name: 'A', steps: [a] }, { id: 'pb', name: 'B', steps: [b] }] } }
  it('follows only the selected alternate path and rejoins the parent sequence', () => {
    const after = { ...newStep(diagram), id: 'after' }
    expect(playbackFrames([branch, after]).map(frame => frame.id)).toEqual(['branch', 'a', 'after'])
    expect(playbackFrames([branch, after], { branch: 'pb' }).map(frame => frame.id)).toEqual(['branch', 'b', 'after'])
    expect(playbackFrames([branch], { branch: 'deleted' }).map(frame => frame.id)).toEqual(['branch', 'a'])
    expect(resolveFocus(makeFlow([branch]), playbackFrames([branch], { branch: 'pb' })[0], diagram).elementIds).toEqual([2])
  })
  it('previews first steps of parallel paths together, then follows the selected path', () => {
    const parallel = { ...branch, branches: { ...branch.branches, mode: 'parallel' } }
    const flow = makeFlow([parallel])
    const frames = playbackFrames(flow.steps, { branch: 'pb' })
    expect(resolveFocus(flow, frames[0], diagram).elementIds).toEqual([1, 2])
    expect(resolveFocus(flow, frames[1], diagram).elementIds).toEqual([2])
  })
  it('respects an alternate choice nested at the start of a parallel path', () => {
    const parent = { ...newStep(diagram), branches: { mode: 'parallel', paths: [{ id: 'lane', name: 'Lane', steps: [branch] }, { id: 'empty', name: 'Empty', steps: [] }] } }
    const flow = makeFlow([parent])
    expect(resolveFocus(flow, playbackFrames(flow.steps, { branch: 'pb' })[0], diagram).elementIds).toEqual([2])
  })
  it('derives introduction and conclusion overviews from all nested steps', () => {
    const overview = { ...newStep(diagram), overview: true }
    const flow = makeFlow([overview, branch])
    expect(resolveFocus(flow, playbackFrames(flow.steps)[0], diagram).elementIds).toEqual([1, 2])
    expect(resolveFocus(flow, undefined, diagram).elementIds).toEqual([1, 2])
    expect(resolveFocus(flow, { id: 'empty', steps: [newStep(diagram)] }, diagram).elementIds).toEqual([])
    const [copy] = duplicateSteps([branch])
    expect(copy.branches!.paths[0].id).not.toBe('pa')
    expect(copy.branches!.paths[0].steps[0].id).not.toBe('a')
    copy.branches!.paths[0].steps[0].elementIds.push(2)
    expect(a.elementIds).toEqual([1])
  })
})
