import { describe, expect, it } from 'vitest'
import { stringify } from 'yaml'
import { prepareCompleteFlowImport } from './completeImport'
import { exportCompleteFlowYaml, exportFlowYaml, importFlowYaml, parseFlowYaml, validatePortableFlow, type PortableFlowDocument } from './yaml'
import type { Connector } from '../types'
import type { FlowDiagram, StoryFlow } from './model'

const diagram = (offset = 0): FlowDiagram => ({
  elements: [
    { element_id: offset + 1, name: 'API', kind: 'service', description: 'Accepts requests', technology: 'Go', tags: ['backend'], position_x: -20, position_y: 100,
      technology_connectors: [{ type: 'catalog', label: 'Go', slug: 'go', is_primary_icon: true }] },
    { element_id: offset + 2, name: 'DB' },
    { element_id: offset + 3, name: 'Unused' },
  ],
  connectors: [{ id: offset + 10, source_element_id: offset + 1, target_element_id: offset + 2, label: 'Reads', direction: 'both', style: 'smoothstep', description: 'Queries', tags: ['data'] }] as Connector[],
})
const flow: StoryFlow = { id: 'local-id', revision: 10, viewId: 1, name: 'Checkout', description: 'Story description', steps: [
  { id: 'branch', title: '', description: '', elementIds: [], connectorIds: [], branches: { mode: 'alternate', paths: [
    { id: 'work', name: 'Work', steps: [{ id: 'read', title: 'Read', description: '', elementIds: [], connectorIds: [10] }] },
    { id: 'skip', name: 'Skip', steps: [] },
  ] } },
] }
const complete = () => exportCompleteFlowYaml(flow, diagram())

describe('complete flow YAML', () => {
  it('bundles nested highlights and connector endpoints with display data, without unrelated objects or database IDs', () => {
    const source = complete()
    const resources = parseFlowYaml(source).resources!
    expect(resources.elements.map(element => element.name)).toEqual(['API', 'DB'])
    expect(resources.elements[0]).toMatchObject({ kind: 'service', technology: 'Go', description: 'Accepts requests', tags: ['backend'], position_x: -20, position_y: 100 })
    expect(resources.elements[0].technology_connectors?.[0]).toMatchObject({ slug: 'go', is_primary_icon: true })
    expect(resources.connectors).toEqual([expect.objectContaining({ source: 'API', target: 'DB', label: 'Reads', direction: 'both', description: 'Queries', tags: ['data'] })])
    expect(source).not.toMatch(/element_id|view_id|local-id|revision|Unused/)
    expect(parseFlowYaml(exportFlowYaml(flow, diagram()))).not.toHaveProperty('resources')
  })

  it('plans fresh resources in a new owner under root and rebinds the draft to the returned view', () => {
    const plan = prepareCompleteFlowImport(complete(), 7)
    expect(plan.batch.elements[0]).toMatchObject({ name: 'Checkout', description: 'Story description', hasView: true, placements: [{ parentRef: 'root' }] })
    expect(plan.batch.elements.slice(1).map(element => element.placements[0].parentRef)).toEqual(['flow-owner', 'flow-owner'])
    expect(plan.batch.elements[1]).toMatchObject({ kind: 'service', technology: 'Go', technologyLinks: [{ slug: 'go', isPrimaryIcon: true }], placements: [{ positionX: -20, positionY: 100 }] })
    expect(plan.batch.elements.every(element => element.id === undefined && element.viewId === undefined)).toBe(true)
    expect(plan.batch.connectors[0]).toMatchObject({ viewRef: 'flow-owner', sourceElementRef: 'element-0', targetElementRef: 'element-1', direction: 'both' })
    const draft = plan.resolve(99, diagram(1000))
    expect(draft).toMatchObject({ id: '', revision: 0, viewId: 99, name: 'Checkout' })
    expect(draft.steps[0].branches!.paths[0].steps[0].connectorIds).toEqual([1010])
    expect(importFlowYaml(complete(), 7, diagram(1000)).viewId).toBe(7)
  })

  it('rejects incomplete bundles and missing linked flows before producing a write plan', () => {
    const doc = parseFlowYaml(complete())
    doc.resources!.elements.pop()
    expect(() => prepareCompleteFlowImport(stringify(doc), 7)).toThrow(/endpoints/)
    doc.resources = { elements: [{ name: 'API' }, { name: 'DB' }], connectors: [] }
    expect(() => prepareCompleteFlowImport(stringify(doc), 7)).toThrow(/connection is not/)
    const linked: PortableFlowDocument = { ...doc, steps: [{ goToFlow: { name: 'Missing' } }] }
    expect(() => prepareCompleteFlowImport(stringify(linked), 7)).toThrow(/linked flow/)
    expect(() => prepareCompleteFlowImport(exportFlowYaml(flow, diagram()), 7)).toThrow(/does not include/)
  })

  it('rejects duplicate resource names, identities, invalid coordinates, and unknown endpoints', () => {
    const doc = parseFlowYaml(complete())
    const check = (resources: unknown) => validatePortableFlow({ ...doc, resources })
    expect(() => check({ ...doc.resources, elements: [{ name: 'A' }, { name: 'A' }] })).toThrow(/duplicate/)
    expect(() => check({ ...doc.resources, elements: [{ name: 'A', id: 42 }] })).toThrow(/unknown field/)
    expect(() => check({ ...doc.resources, elements: [{ name: 'A', position_x: Infinity, position_y: 0 }] })).toThrow(/finite number/)
    expect(() => check({ ...doc.resources, elements: [{ name: 'A', position_x: 1 }] })).toThrow(/both position/)
    expect(() => check({ ...doc.resources, connectors: [...doc.resources!.connectors, ...doc.resources!.connectors] })).toThrow(/duplicate connection/)
  })

  it('supports empty complete flows and resolves existing linked flows in the original import context', () => {
    expect(parseFlowYaml(exportCompleteFlowYaml({ ...flow, steps: [] }, diagram())).resources).toEqual({ elements: [], connectors: [] })
    const doc = parseFlowYaml(complete())
    doc.steps = [{ goToFlow: { name: 'Existing' } }]
    const linked = { ...flow, id: 'linked', viewId: 7, name: 'Existing', steps: [] }
    const plan = prepareCompleteFlowImport(stringify(doc), 7, { flows: [linked], views: [] })
    expect(plan.resolve(99, diagram()).steps[0].goToFlowId).toBe('linked')
  })
})
