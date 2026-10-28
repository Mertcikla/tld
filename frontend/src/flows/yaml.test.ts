import { FLOW_MAX_STEPS, FLOW_MAX_PATHS, FLOW_MAX_BRANCH_DEPTH } from './model'
import { describe, expect, it } from 'vitest'
import { stringify } from 'yaml'
import type { Connector } from '../types'
import type { FlowDiagram } from './model'
import { exportFlowYaml, FLOW_YAML_MAX_BYTES, importFlowYaml, parseFlowYaml, validatePortableFlow, type PortableFlowDocument } from './yaml'

const example = `format: tld-flow
version: 1
name: Checkout
description: Follow an order from submission to confirmation.
steps:
  - id: intro
    title: Start checkout
  - id: submit
    title: Submit and store the order
    elements: [Checkout API, Order database]
    connectors:
      - source: Customer
        target: Checkout API
        label: Submits orders
      - source: Checkout API
        target: Order database
        label: Stores orders
  - id: process
    title: Process order
    elements: [Checkout API]
  - id: confirm
    title: Confirm the order
    connectors:
      - source: Customer
        target: Checkout API
        label: Submits orders
`
function diagram(offset = 0): FlowDiagram {
  return {
    elements: ['Customer', 'Checkout API', 'Order database'].map((name, i) => ({ element_id: offset + i + 1, name })),
    connectors: [
      { id: offset + 10, source_element_id: offset + 1, target_element_id: offset + 2, label: 'Submits orders', direction: 'both' },
      { id: offset + 11, source_element_id: offset + 2, target_element_id: offset + 3, label: 'Stores orders', direction: 'forward' },
    ] as Connector[],
  }
}
function simple(): PortableFlowDocument { return { format: 'tld-flow', version: 1, name: 'Story', steps: [{}] } }
const reference = { source: 'Customer', target: 'Checkout API' }

describe('portable flow YAML', () => {
  it('round-trips empty, multiple and mixed highlights across database IDs', () => {
    const original = importFlowYaml(example, 7, diagram())
    original.id = 'saved-flow-id'; original.revision = 22
    const yaml = exportFlowYaml(original, diagram())
    const imported = importFlowYaml(yaml, 987, diagram(1000))
    expect(imported).toMatchObject({ id: '', revision: 0, viewId: 987, name: original.name, description: original.description })
    expect(imported.steps.map(step => step.id)).toEqual(original.steps.map(step => step.id))
    expect(imported.steps[0]).toMatchObject({ elementIds: [], connectorIds: [] })
    expect(imported.steps[1]).toMatchObject({ elementIds: [1002, 1003], connectorIds: [1010, 1011] })
    expect(imported.steps[3].connectorIds).toEqual([1010])
    expect(yaml).not.toMatch(/saved-flow-id|viewId|view_id|revision|connectorId|connector_id|elementId|element_id|kind:|paths:/)
    expect(parseFlowYaml(yaml)).not.toHaveProperty('elements')
    expect(parseFlowYaml(yaml)).not.toHaveProperty('connectors')
    expect(exportFlowYaml(imported, diagram(1000))).toBe(yaml)
  })

  it('accepts empty stories, empty steps and omitted highlights; generates IDs only when omitted', () => {
    const value = simple()
    const first = importFlowYaml(stringify(value), 1, diagram())
    const second = importFlowYaml(stringify(value), 1, diagram())
    expect(first.steps[0].id).not.toBe(second.steps[0].id)
    expect(first.steps[0]).toMatchObject({ title: '', description: '', elementIds: [], connectorIds: [] })
    value.steps = [{ id: 'kept', elements: [], connectors: [] }]
    expect(importFlowYaml(stringify(value), 1, diagram()).steps[0].id).toBe('kept')
    value.steps = []
    expect(importFlowYaml(stringify(value), 1, diagram()).steps).toEqual([])
  })

  it('round-trips literal names, YAML-looking values, multiline text and Unicode', () => {
    const target = diagram()
    const flow = importFlowYaml(example, 1, target)
    target.elements[0].name = 'A / B'; target.elements[1].name = 'A + B'; target.elements[2].name = '日本語'
    target.connectors[0].label = ''; target.connectors[1].label = ''
    flow.name = 'true'; flow.steps[0].title = 'null'
    flow.steps[0].description = 'First: "yes"\n# not a comment\nTürkçe → 日本語\n'
    const exported = exportFlowYaml(flow, target)
    expect(parseFlowYaml(exported).steps[1].connectors?.[0]).toEqual({ source: 'A / B', target: 'A + B' })
    expect(exported).toContain('日本語')
    expect(importFlowYaml(exported, 1, target)).toEqual(flow)
  })

  it('refuses missing, ambiguous and reversed endpoints without mutating the target', () => {
    const target = diagram(), original = JSON.stringify(target)
    expect(() => importFlowYaml(example, 1, { ...target, elements: target.elements.slice(1) })).toThrow(/not in the target/)
    expect(() => importFlowYaml(example, 1, { ...target, elements: [...target.elements, { element_id: 50, name: 'Customer' }] })).toThrow(/ambiguous element/)
    expect(() => importFlowYaml(example, 1, { ...target, connectors: [...target.connectors, { ...target.connectors[0], id: 99 }] })).toThrow(/ambiguous connection/)
    expect(() => importFlowYaml(example, 1, { ...target, connectors: target.connectors.map(c => ({ ...c, source_element_id: c.target_element_id, target_element_id: c.source_element_id })) })).toThrow(/connection is not/)
    expect(JSON.stringify(target)).toBe(original)
  })

  it('supports indexed host bindings while enforcing placement and endpoints', () => {
    const target = diagram()
    target.elements[0].name = 'Buyer'; target.connectors[0].label = 'Posts basket'
    const bindings = { elements: { Customer: 1 }, connectors: { submit: { 0: 10 }, confirm: { 0: 10 } } }
    expect(importFlowYaml(example, 1, target, bindings).steps[1].connectorIds).toEqual([10, 11])
    expect(() => importFlowYaml(example, 1, target, { ...bindings, elements: { Customer: 99 } })).toThrow(/not in the target/)
    expect(() => importFlowYaml(example, 1, target, { ...bindings, connectors: { submit: { 0: 11 } } })).toThrow(/connection is not/)
    expect(() => importFlowYaml(example, 1, target, { elements: { typo: 1 } })).toThrow(/unknown reference/)
    expect(() => importFlowYaml(example, 1, target, { elements: { Customer: 0 } })).toThrow(/positive local/)
    expect(() => importFlowYaml(example, 1, target, { connectors: { typo: { 0: 10 } } })).toThrow(/unknown step/)
    expect(() => importFlowYaml(example, 1, target, { connectors: { submit: { 9: 10 } } })).toThrow(/unknown highlight/)
  })

  it('refuses lossy exports with missing, duplicate or ambiguous references', () => {
    const flow = importFlowYaml(example, 1, diagram())
    expect(() => exportFlowYaml(flow, { elements: [], connectors: diagram().connectors })).toThrow(/unavailable/)
    expect(() => exportFlowYaml(flow, { elements: diagram().elements, connectors: [] })).toThrow(/unavailable/)
    expect(() => exportFlowYaml(flow, { ...diagram(), elements: [...diagram().elements, { element_id: 99, name: 'Customer' }] })).toThrow(/ambiguous/)
    expect(() => exportFlowYaml(flow, { ...diagram(), connectors: [...diagram().connectors, { ...diagram().connectors[0], id: 99 }] })).toThrow(/distinct labels/)
    flow.steps[1].connectorIds.push(flow.steps[1].connectorIds[0])
    expect(() => exportFlowYaml(flow, diagram())).toThrow(/duplicate/)
  })

  it('allows an omitted label only for unique ordered endpoints and never ignores a supplied label', () => {
    const value: PortableFlowDocument = { ...simple(), steps: [{ connectors: [reference] }] }
    expect(importFlowYaml(stringify(value), 1, diagram()).steps[0].connectorIds[0]).toBe(10)
    const parallel = { ...diagram(), connectors: [...diagram().connectors, { ...diagram().connectors[0], id: 99, label: 'Alternative' }] }
    expect(() => importFlowYaml(stringify(value), 1, parallel)).toThrow(/ambiguous connection/)
    value.steps = [{ connectors: [{ ...reference, label: 'Alternative' }] }]
    expect(importFlowYaml(stringify(value), 1, parallel).steps[0].connectorIds[0]).toBe(99)
    value.steps = [{ connectors: [{ ...reference, label: 'Does not exist' }] }]
    expect(() => importFlowYaml(stringify(value), 1, diagram())).toThrow(/connection is not/)
  })

  it('distinguishes an explicit empty label from omission', () => {
    const target = diagram()
    target.connectors[0].label = null
    target.connectors.push({ ...target.connectors[0], id: 99, label: 'Named connection' })
    const value: PortableFlowDocument = { ...simple(), steps: [{ connectors: [{ ...reference, label: '' }] }] }
    const flow = importFlowYaml(stringify(value), 1, target)
    expect(flow.steps[0].connectorIds).toEqual([10])
    const exported = exportFlowYaml(flow, target)
    expect(parseFlowYaml(exported).steps[0].connectors?.[0]).toEqual({ ...reference, label: '' })
    expect(importFlowYaml(exported, 1, target)).toEqual(flow)
    expect(() => importFlowYaml(stringify(value), 1, diagram())).toThrow(/connection is not/)
  })

  it('rejects two references resolving to one highlight, including through bindings', () => {
    const value: PortableFlowDocument = { ...simple(), steps: [{ connectors: [reference, { ...reference, label: 'Submits orders' }] }] }
    expect(() => importFlowYaml(stringify(value), 1, diagram())).toThrow(/duplicate resolved/)
    value.steps = [{ elements: ['Customer', 'Buyer'] }]
    expect(() => importFlowYaml(stringify(value), 1, diagram(), { elements: { Buyer: 1 } })).toThrow(/duplicate resolved/)
  })

  it.each([
    'format: tld-flow\nversion: 1\nversion: 1',
    '---\nformat: tld-flow\n---\nformat: tld-flow',
    'format: !custom tld-flow',
    '%YAML 1.1\n---\nformat: tld-flow',
    'format: tld-flow\nversion: 1\nname: &name Flow\nsteps: [{title: *name}]',
  ])('rejects unsupported or unsafe YAML syntax', source => {
    expect(() => parseFlowYaml(source)).toThrow()
  })

  it('rejects kinds, paths, definition tables, wrong types, duplicates and malformed highlights', () => {
    for (const step of [
      { kind: 'message' }, { paths: [] }, { overview: true }, { metadata: {} }, { connectors: [{ ...reference, reverse: true }] }, { source: 'Customer' }, { elements: [''] },
      { elements: ['Customer', 'Customer'] }, { connectors: [reference, { ...reference, reverse: true }] },
      { connectors: [{ source: 'Customer', target: ' ' }] }, { connectors: [{ ...reference, label: 1 }] },
      { connectors: [{ ...reference, reverse: 'true' }] }, { connectors: ['label'] },
      { metadata: { ' ': 'value' } }, { metadata: { count: 1 } }, { metadata: [] },
    ]) expect(() => validatePortableFlow({ ...simple(), steps: [step] })).toThrow()
    for (const value of [
      { ...simple(), version: 2 }, { ...simple(), view_id: 1 }, { ...simple(), name: true },
      { ...simple(), elements: {} }, { ...simple(), connectors: {} },
      { ...simple(), steps: [{ id: 'same' }, { id: 'same' }] },
    ]) expect(() => validatePortableFlow(value)).toThrow()
  })

  it('enforces document size, collection limits and UTF-8 field limits', () => {
    expect(() => parseFlowYaml('#'.repeat(FLOW_YAML_MAX_BYTES + 1))).toThrow(/1 MiB/)
    expect(() => validatePortableFlow({ ...simple(), steps: Array.from({ length: FLOW_MAX_STEPS + 1 }, () => ({})) })).toThrow(/1000/)
    expect(() => validatePortableFlow({ ...simple(), steps: [{ elements: Array.from({ length: 501 }, (_, i) => String(i)) }] })).toThrow(/500/)
    expect(() => validatePortableFlow({ ...simple(), name: '界'.repeat(100) })).toThrow(/UTF-8 bytes/)
  })

  it('accepts generous step and path bounds and rejects one beyond each', () => {
    expect(validatePortableFlow({ ...simple(), steps: Array.from({ length: FLOW_MAX_STEPS }, () => ({})) }).steps).toHaveLength(FLOW_MAX_STEPS)
    const paths = Array.from({ length: FLOW_MAX_PATHS }, (_, i) => ({ name: `Path ${i}`, steps: [] }))
    expect(validatePortableFlow({ ...simple(), steps: [{ branches: { mode: 'parallel', paths } }] }).steps[0].branches?.paths).toHaveLength(FLOW_MAX_PATHS)
    expect(() => validatePortableFlow({ ...simple(), steps: [{ branches: { mode: 'parallel', paths: [...paths, { name: 'Too many', steps: [] }] } }] })).toThrow(/26/)
  })
})

describe('portable flow control extensions', () => {
  const linked = { id: 'linked', viewId: 2, revision: 1, name: 'Payment', description: '', steps: [] }
  const catalog = { flows: [linked], views: [{ id: 1, name: 'Checkout' }, { id: 2, name: 'Payments' }] }
  const doc: PortableFlowDocument = { ...simple(), steps: [
    { id: 'intro', title: 'Start' },
    { id: 'branch', branches: { mode: 'alternate', paths: [
      { id: 'success', name: 'Success', steps: [{ id: 'go', goToFlow: { name: 'Payment', view: 'Payments' } }] },
      { id: 'failure', name: 'Failure', steps: [{ id: 'notify', branches: { mode: 'parallel', paths: [
        { name: 'Customer', steps: [{ connectors: [reference] }] },
        { name: 'Database', steps: [{ elements: ['Order database'] }] },
      ] } }] },
    ] } },
    { id: 'outro' },
  ] }
  it('round-trips nested paths, cross-diagram links without database IDs', () => {
    const flow = importFlowYaml(stringify(doc), 1, diagram(), {}, catalog)
    expect(flow.steps[1].branches?.paths[0].steps[0].goToFlowId).toBe('linked')
    const yaml = exportFlowYaml(flow, diagram(), catalog)
    expect(yaml).not.toContain('goToFlowId')
    const targetCatalog = { flows: [{ ...linked, id: 'target-link', viewId: 202 }], views: [{ id: 202, name: 'Payments' }] }
    const restored = importFlowYaml(yaml, 101, diagram(1000), {}, targetCatalog)
    expect(restored.steps[1].branches?.paths[0].steps[0].goToFlowId).toBe('target-link')
    expect(restored.steps[1].branches?.paths[1].steps[0].branches?.paths[1].steps[0].elementIds).toEqual([1003])
    expect(exportFlowYaml(restored, diagram(1000), targetCatalog)).toBe(yaml)
  })
  it('resolves same-diagram links by name and refuses missing or ambiguous targets', () => {
    const source = stringify({ ...simple(), steps: [{ goToFlow: { name: 'Payment' } }] })
    expect(importFlowYaml(source, 2, diagram(), {}, catalog).steps[0].goToFlowId).toBe('linked')
    expect(() => importFlowYaml(source, 1, diagram(), {}, catalog)).toThrow(/linked flow/)
    expect(() => importFlowYaml(source, 2, diagram(), {}, { ...catalog, flows: [linked, { ...linked, id: 'duplicate' }] })).toThrow(/ambiguous/)
    expect(() => importFlowYaml(stringify(doc), 1, diagram(), {}, { ...catalog, views: [...catalog.views, { id: 3, name: 'Payments' }] })).toThrow(/ambiguous/)
    const flow = importFlowYaml(stringify(doc), 1, diagram(), {}, catalog)
    expect(() => exportFlowYaml(flow, diagram())).toThrow(/linked flow/)
  })
  it('rejects conflicting controls, invalid branches, duplicate nested IDs and excessive nesting/counts', () => {
    const branches = { mode: 'alternate', paths: [{ id: 'a', name: 'A', steps: [] }, { id: 'b', name: 'B', steps: [] }] }
    for (const step of [
      { overview: 'true' }, { overview: true, branches }, { branches, goToFlow: { name: 'Payment' } },
      { branches: { ...branches, mode: 'condition' } }, { branches: { mode: 'parallel', paths: [] } },
      { id: 'a', branches }, { goToFlow: { name: '' } }, { goToFlow: { name: 'Payment', id: 2 } },
      { branches: { mode: 'parallel', paths: [{ name: 'A', steps: [{ id: 'duplicate' }] }, { name: 'B', steps: [{ id: 'duplicate' }] }] } },
    ]) expect(() => validatePortableFlow({ ...simple(), steps: [step] })).toThrow()
    let nested: PortableFlowDocument['steps'] = [{}]
    for (let i = 0; i < FLOW_MAX_BRANCH_DEPTH + 1; i++) nested = [{ branches: { mode: 'alternate', paths: [{ name: 'A', steps: nested }, { name: 'B', steps: [] }] } }]
    expect(() => validatePortableFlow({ ...simple(), steps: nested })).toThrow(/eight/)
    expect(() => validatePortableFlow({ ...simple(), steps: [{ branches: { mode: 'parallel', paths: [{ name: 'A', steps: Array.from({ length: FLOW_MAX_STEPS / 2 }, () => ({})) }, { name: 'B', steps: Array.from({ length: FLOW_MAX_STEPS / 2 }, () => ({})) }] } }] })).toThrow(/1000 total/)
  })
})
