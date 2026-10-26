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
    metadata:
      owner: Commerce
      ticket: ORD-42
  - id: process
    title: Process order
    elements: [Checkout API]
  - id: confirm
    title: Confirm the order
    connectors:
      - source: Customer
        target: Checkout API
        label: Submits orders
        reverse: true
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
  it('round-trips empty, multiple and mixed highlights, metadata and reverse directions across database IDs', () => {
    const original = importFlowYaml(example, 7, diagram())
    original.id = 'saved-flow-id'; original.revision = 22
    const yaml = exportFlowYaml(original, diagram())
    const imported = importFlowYaml(yaml, 987, diagram(1000))
    expect(imported).toMatchObject({ id: '', revision: 0, viewId: 987, name: original.name, description: original.description })
    expect(imported.steps.map(step => step.id)).toEqual(original.steps.map(step => step.id))
    expect(imported.steps[0]).toMatchObject({ elementIds: [], connectors: [] })
    expect(imported.steps[1]).toMatchObject({ elementIds: [1002, 1003], connectors: [{ id: 1010, reverse: false }, { id: 1011, reverse: false }], metadata: { owner: 'Commerce', ticket: 'ORD-42' } })
    expect(imported.steps[3].connectors).toEqual([{ id: 1010, reverse: true }])
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
    expect(first.steps[0]).toMatchObject({ title: '', description: '', elementIds: [], connectors: [], metadata: {} })
    value.steps = [{ id: 'kept', elements: [], connectors: [], metadata: {} }]
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
    flow.steps[0].metadata = { count: '001', enabled: 'true', note: 'a\nb' }
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
    expect(importFlowYaml(example, 1, target, bindings).steps[1].connectors.map(c => c.id)).toEqual([10, 11])
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
    flow.steps[1].connectors.push(flow.steps[1].connectors[0])
    expect(() => exportFlowYaml(flow, diagram())).toThrow(/duplicate/)
  })

  it('allows an omitted label only for unique ordered endpoints and never ignores a supplied label', () => {
    const value: PortableFlowDocument = { ...simple(), steps: [{ connectors: [reference] }] }
    expect(importFlowYaml(stringify(value), 1, diagram()).steps[0].connectors[0].id).toBe(10)
    const parallel = { ...diagram(), connectors: [...diagram().connectors, { ...diagram().connectors[0], id: 99, label: 'Alternative' }] }
    expect(() => importFlowYaml(stringify(value), 1, parallel)).toThrow(/ambiguous connection/)
    value.steps = [{ connectors: [{ ...reference, label: 'Alternative' }] }]
    expect(importFlowYaml(stringify(value), 1, parallel).steps[0].connectors[0].id).toBe(99)
    value.steps = [{ connectors: [{ ...reference, label: 'Does not exist' }] }]
    expect(() => importFlowYaml(stringify(value), 1, diagram())).toThrow(/connection is not/)
  })

  it('distinguishes an explicit empty label from omission', () => {
    const target = diagram()
    target.connectors[0].label = null
    target.connectors.push({ ...target.connectors[0], id: 99, label: 'Named connection' })
    const value: PortableFlowDocument = { ...simple(), steps: [{ connectors: [{ ...reference, label: '', reverse: true }] }] }
    const flow = importFlowYaml(stringify(value), 1, target)
    expect(flow.steps[0].connectors).toEqual([{ id: 10, reverse: true }])
    const exported = exportFlowYaml(flow, target)
    expect(parseFlowYaml(exported).steps[0].connectors?.[0]).toEqual({ ...reference, label: '', reverse: true })
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
      { kind: 'message' }, { paths: [] }, { source: 'Customer' }, { elements: [''] },
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
    expect(() => validatePortableFlow({ ...simple(), steps: Array.from({ length: 501 }, () => ({})) })).toThrow(/500/)
    expect(() => validatePortableFlow({ ...simple(), steps: [{ elements: Array.from({ length: 501 }, (_, i) => String(i)) }] })).toThrow(/500/)
    expect(() => validatePortableFlow({ ...simple(), steps: [{ metadata: Object.fromEntries(Array.from({ length: 51 }, (_, i) => [String(i), 'v'])) }] })).toThrow(/50 metadata/)
    expect(() => validatePortableFlow({ ...simple(), name: '界'.repeat(100) })).toThrow(/UTF-8 bytes/)
  })
})
