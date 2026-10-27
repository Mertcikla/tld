import { parseDocument, stringify } from 'yaml'
import { FLOW_MAX_STEPS, FLOW_MAX_PATHS, FLOW_MAX_BRANCH_DEPTH, type FlowCatalog, type FlowDiagram, type FlowStep, type StoryFlow } from './model'

export const FLOW_YAML_MAX_BYTES = 1024 * 1024
export const FLOW_YAML_SCHEMA_URL = 'https://raw.githubusercontent.com/Mertcikla/tld/refs/heads/main/schemas/flow.schema.json'

export interface PortableConnectorHighlight { source: string; target: string; label?: string; reverse?: boolean }
export interface PortableFlowStep {
  id?: string
  title?: string
  description?: string
  elements?: string[]
  connectors?: PortableConnectorHighlight[]
  metadata?: Record<string, string>
  overview?: boolean
  branches?: { mode: 'alternate' | 'parallel'; paths: { id?: string; name: string; steps: PortableFlowStep[] }[] }
  goToFlow?: { name: string; view?: string }
}
export interface PortableFlowDocument {
  format: 'tld-flow'
  version: 1
  name: string
  description?: string
  steps: PortableFlowStep[]
}
/** Local-only overrides for duplicate or renamed objects. */
export interface FlowYamlBindings {
  /** Keys are element names used in steps. */
  elements?: Record<string, number>
  /** Step ID → zero-based highlight index → local connector ID. */
  connectors?: Record<string, Record<number, number>>
}

const byteLength = (value: string) => new TextEncoder().encode(value).length
const owns = (value: object, key: string | number) => Object.prototype.hasOwnProperty.call(value, key)
function fail(path: string, message: string): never { throw new Error(`${path}: ${message}`) }
function record(value: unknown, path: string): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) fail(path, 'expected a mapping')
  return value as Record<string, unknown>
}
function keys(value: Record<string, unknown>, allowed: string[], path: string) {
  for (const key of Object.keys(value)) if (!allowed.includes(key)) fail(`${path}.${key}`, 'unknown field')
}
function text(value: unknown, path: string, max: number, nonempty = false): string {
  if (typeof value !== 'string' || (nonempty && !value.trim())) fail(path, nonempty ? 'expected a nonempty string' : 'expected a string')
  if (byteLength(value) > max) fail(path, `must be at most ${max} UTF-8 bytes`)
  return value
}
function list(value: unknown, path: string, max = 500): unknown[] {
  if (!Array.isArray(value)) fail(path, 'expected a sequence')
  if (value.length > max) fail(path, `must contain at most ${max} entries`)
  return value
}

/** Strict, bounded validation shared by parsing and exporting. No document data executes. */
export function validatePortableFlow(value: unknown): PortableFlowDocument {
  const doc = record(value, 'flow')
  keys(doc, ['format', 'version', 'name', 'description', 'steps'], 'flow')
  if (doc.format !== 'tld-flow') fail('flow.format', 'expected tld-flow')
  if (doc.version !== 1) fail('flow.version', 'only version 1 is supported')
  const name = text(doc.name, 'flow.name', 200, true)
  const description = doc.description === undefined ? undefined : text(doc.description, 'flow.description', 10000)
  const ids = new Set<string>()
  let count = 0
  function optionalId(value: unknown, path: string): string | undefined {
    if (value === undefined) return undefined
    const id = text(value, path, 100, true)
    if (ids.has(id)) fail(path, 'duplicate step id or path id')
    ids.add(id)
    return id
  }
  function validateSteps(value: unknown, location: string, depth: number): PortableFlowStep[] {
    if (depth > FLOW_MAX_BRANCH_DEPTH) fail(location, 'branches may be nested at most eight levels')
    return list(value, location, FLOW_MAX_STEPS).map((value, index): PortableFlowStep => {
      const path = `${location}[${index}]`, step = record(value, path)
      if (++count > FLOW_MAX_STEPS) fail(path, `at most ${FLOW_MAX_STEPS} total steps are allowed`)
      keys(step, ['id', 'title', 'description', 'elements', 'connectors', 'metadata', 'overview', 'branches', 'goToFlow'], path)
      const result: PortableFlowStep = {}
      const id = optionalId(step.id, `${path}.id`)
      if (id !== undefined) result.id = id
      if (step.overview !== undefined) {
        if (typeof step.overview !== 'boolean') fail(`${path}.overview`, 'expected a boolean')
        result.overview = step.overview
      }
      if (step.goToFlow !== undefined) {
        const target = record(step.goToFlow, `${path}.goToFlow`)
        keys(target, ['name', 'view'], `${path}.goToFlow`)
        result.goToFlow = { name: text(target.name, `${path}.goToFlow.name`, 200, true), ...(target.view !== undefined ? { view: text(target.view, `${path}.goToFlow.view`, 10000, true) } : {}) }
      }
      if (step.branches !== undefined) {
        const branches = record(step.branches, `${path}.branches`)
        keys(branches, ['mode', 'paths'], `${path}.branches`)
        if (branches.mode !== 'alternate' && branches.mode !== 'parallel') fail(path, 'branches mode must be alternate or parallel')
        const paths = list(branches.paths, `${path}.branches.paths`, FLOW_MAX_PATHS)
        if (paths.length < 2 || paths.length > FLOW_MAX_PATHS) fail(path, `branches require two to ${FLOW_MAX_PATHS} paths`)
        result.branches = { mode: branches.mode, paths: paths.map((value, index) => {
          const location = `${path}.branches.paths[${index}]`, branch = record(value, location)
          keys(branch, ['id', 'name', 'steps'], location)
          const id = optionalId(branch.id, `${location}.id`)
          return { ...(id !== undefined ? { id } : {}), name: text(branch.name, `${location}.name`, 200, true), steps: validateSteps(branch.steps, `${location}.steps`, depth + 1) }
        }) }
      }
      if ([result.overview, result.branches, result.goToFlow].filter(Boolean).length > 1) fail(path, 'choose only one of overview, branches, or goToFlow')
      if (step.title !== undefined) result.title = text(step.title, `${path}.title`, 500)
      if (step.description !== undefined) result.description = text(step.description, `${path}.description`, 10000)
      if (step.elements !== undefined) {
        result.elements = list(step.elements, `${path}.elements`).map((name, index) => text(name, `${path}.elements[${index}]`, 10000, true))
        if (new Set(result.elements).size !== result.elements.length) fail(path, 'duplicate element highlight')
      }
      if (step.connectors !== undefined) {
        result.connectors = list(step.connectors, `${path}.connectors`).map((value, index) => {
          const location = `${path}.connectors[${index}]`, connector = record(value, location)
          keys(connector, ['source', 'target', 'label', 'reverse'], location)
          if (connector.reverse !== undefined && typeof connector.reverse !== 'boolean') fail(`${location}.reverse`, 'expected a boolean')
          return {
            source: text(connector.source, `${location}.source`, 10000, true),
            target: text(connector.target, `${location}.target`, 10000, true),
            ...(connector.label !== undefined ? { label: text(connector.label, `${location}.label`, 10000) } : {}),
            ...(connector.reverse !== undefined ? { reverse: connector.reverse as boolean } : {}),
          }
        })
        const references = result.connectors.map(c => JSON.stringify([c.source, c.target, c.label]))
        if (new Set(references).size !== references.length) fail(path, 'duplicate connector highlight')
      }
      if (step.metadata !== undefined) {
        const entries = Object.entries(record(step.metadata, `${path}.metadata`))
        if (entries.length > 50) fail(path, 'at most 50 metadata entries are allowed')
        result.metadata = Object.fromEntries(entries.map(([key, value]) => [text(key, `${path}.metadata key`, 100, true), text(value, `${path}.metadata.${key}`, 10000)]))
      }
      return result
    })
  }
  return { format: 'tld-flow', version: 1, name, ...(description !== undefined ? { description } : {}), steps: validateSteps(doc.steps, 'steps', 0) }
}

export function parseFlowYaml(source: string): PortableFlowDocument {
  if (byteLength(source) > FLOW_YAML_MAX_BYTES) fail('flow', 'YAML must be at most 1 MiB')
  const document = parseDocument(source, { version: '1.2', uniqueKeys: true, strict: true })
  if (document.errors.length || document.warnings.length) throw new Error([...document.errors, ...document.warnings].map(error => error.message).join('\n'))
  if (document.directives?.yaml.version !== '1.2') fail('flow', 'only YAML 1.2 is supported')
  return validatePortableFlow(document.toJS({ maxAliasCount: 0 }))
}

/** Resolve a document into a new unsaved flow; never overwrite a saved flow. */
export function importFlowYaml(source: string, viewId: number, diagram: FlowDiagram, bindings: FlowYamlBindings = {}, catalog: FlowCatalog = { flows: [], views: [] }): StoryFlow {
  if (!Number.isSafeInteger(viewId) || viewId <= 0) fail('view', 'choose a valid target diagram')
  const doc = parseFlowYaml(source)
  const flatten = (steps: PortableFlowStep[]): PortableFlowStep[] => steps.flatMap(step => [step, ...(step.branches?.paths ?? []).flatMap(path => flatten(path.steps))])
  const all = flatten(doc.steps)
  const names = new Set(all.flatMap(step => [...step.elements ?? [], ...(step.connectors ?? []).flatMap(c => [c.source, c.target])]))
  const validId = (id: number) => Number.isSafeInteger(id) && id > 0
  for (const [name, id] of Object.entries(bindings.elements ?? {})) {
    if (!names.has(name)) fail('bindings.elements', `unknown reference ${JSON.stringify(name)}`)
    if (!validId(id)) fail('bindings.elements', 'expected a positive local resource ID')
  }
  for (const [stepId, indexes] of Object.entries(bindings.connectors ?? {})) {
    const step = all.find(step => step.id === stepId)
    if (!step) fail('bindings.connectors', `unknown step ${JSON.stringify(stepId)}`)
    for (const [index, id] of Object.entries(indexes)) {
      if (!/^(0|[1-9][0-9]*)$/.test(index) || !step.connectors?.[Number(index)]) fail('bindings.connectors', 'unknown highlight index')
      if (!validId(id)) fail('bindings.connectors', 'expected a positive local resource ID')
    }
  }
  function elementId(name: string, location: string): number {
    const bound = owns(bindings.elements ?? {}, name) ? bindings.elements![name] : undefined
    const matches = diagram.elements.filter(candidate => bound !== undefined ? candidate.element_id === bound : candidate.name === name)
    if (matches.length !== 1) fail(location, matches.length ? `ambiguous element ${JSON.stringify(name)}; provide an explicit binding` : `element ${JSON.stringify(name)} is not in the target diagram`)
    return matches[0].element_id
  }
  const convert = (steps: PortableFlowStep[], location: string): FlowStep[] => steps.map((step, index): FlowStep => {
    const path = `${location}[${index}]`
    const elementIds = (step.elements ?? []).map(name => elementId(name, `${path}.elements`))
    const boundConnectors = step.id !== undefined && owns(bindings.connectors ?? {}, step.id) ? bindings.connectors![step.id] : {}
    const connectors = (step.connectors ?? []).map((highlight, index) => {
      const location = `${path}.connectors[${index}]`
      const source = elementId(highlight.source, `${location}.source`), target = elementId(highlight.target, `${location}.target`)
      const bound = owns(boundConnectors, index) ? boundConnectors[index] : undefined
      const matches = diagram.connectors.filter(c => c.source_element_id === source && c.target_element_id === target &&
        (bound !== undefined ? c.id === bound : highlight.label === undefined || (c.label ?? '') === highlight.label))
      if (matches.length !== 1) fail(location, matches.length ? 'ambiguous connection; specify a unique label or an explicit binding' : 'connection is not in the target diagram (source, target, and any supplied label must match)')
      return { id: matches[0].id, reverse: highlight.reverse ?? false }
    })
    if (new Set(elementIds).size !== elementIds.length || new Set(connectors.map(c => c.id)).size !== connectors.length) fail(path, 'duplicate resolved highlight')
    let goToFlowId: string | undefined
    if (step.goToFlow) {
      let targetViewId = viewId
      if (step.goToFlow.view !== undefined) {
        const views = catalog.views.filter(view => view.name === step.goToFlow!.view)
        if (views.length !== 1) fail(path, 'linked diagram is missing or ambiguous')
        targetViewId = views[0].id
      }
      const targets = catalog.flows.filter(flow => flow.viewId === targetViewId && flow.name === step.goToFlow!.name)
      if (targets.length !== 1) fail(path, 'linked flow is missing or ambiguous')
      goToFlowId = targets[0].id
    }
    return { id: step.id ?? crypto.randomUUID(), title: step.title ?? '', description: step.description ?? '', elementIds, connectors, metadata: step.metadata ?? {},
      ...(step.overview ? { overview: true } : {}), ...(goToFlowId ? { goToFlowId } : {}),
      ...(step.branches ? { branches: { mode: step.branches.mode, paths: step.branches.paths.map((branch, index) => ({ id: branch.id ?? crypto.randomUUID(), name: branch.name, steps: convert(branch.steps, `${path}.branches.paths[${index}].steps`) })) } } : {}),
    }
  })
  return { id: '', viewId, revision: 0, name: doc.name, description: doc.description ?? '', steps: convert(doc.steps, 'steps') }
}

/** Export only names and labels; database identity stays in the selected diagram. */
export function exportFlowYaml(flow: StoryFlow, diagram: FlowDiagram, catalog: FlowCatalog = { flows: [], views: [] }): string {
  function elementName(id: number): string {
    const element = diagram.elements.find(element => element.element_id === id)
    if (!element) fail('export', `element ${id} is unavailable; relink the step before exporting`)
    if (diagram.elements.filter(candidate => candidate.name === element.name).length !== 1) fail('export', `element name ${JSON.stringify(element.name)} is ambiguous; give it a unique name before exporting`)
    return element.name
  }
  function flowReference(id: string): { name: string; view?: string } {
    const target = catalog.flows.find(flow => flow.id === id)
    if (!target) fail('export', 'linked flow is unavailable')
    if (catalog.flows.filter(flow => flow.viewId === target.viewId && flow.name === target.name).length !== 1) fail('export', 'linked flow name is ambiguous')
    if (target.viewId === flow.viewId) return { name: target.name }
    const view = catalog.views.find(view => view.id === target.viewId)
    if (!view || catalog.views.filter(candidate => candidate.name === view.name).length !== 1) fail('export', 'linked diagram name is missing or ambiguous')
    return { name: target.name, view: view.name }
  }
  const convert = (steps: FlowStep[], depth: number): PortableFlowStep[] => {
    if (depth > FLOW_MAX_BRANCH_DEPTH) fail('export', 'branches may be nested at most eight levels')
    return steps.map(step => ({
      id: step.id,
      ...(step.title ? { title: step.title } : {}),
      ...(step.description ? { description: step.description } : {}),
      ...(step.elementIds.length ? { elements: step.elementIds.map(elementName) } : {}),
      ...(step.connectors.length ? { connectors: step.connectors.map(highlight => {
        const connector = diagram.connectors.find(c => c.id === highlight.id)
        if (!connector) fail('export', `connection ${highlight.id} is unavailable; relink the step before exporting`)
        const label = connector.label ?? ''
        const between = diagram.connectors.filter(c => c.source_element_id === connector.source_element_id && c.target_element_id === connector.target_element_id)
        if (between.filter(c => (c.label ?? '') === label).length !== 1) fail('export', 'connections with the same endpoints need distinct labels before exporting')
        return { source: elementName(connector.source_element_id), target: elementName(connector.target_element_id), ...(label || between.length > 1 ? { label } : {}), ...(highlight.reverse ? { reverse: true } : {}) }
      }) } : {}),
      ...(Object.keys(step.metadata).length ? { metadata: step.metadata } : {}),
      ...(step.overview ? { overview: true } : {}),
      ...(step.goToFlowId ? { goToFlow: flowReference(step.goToFlowId) } : {}),
      ...(step.branches ? { branches: { mode: step.branches.mode as 'alternate' | 'parallel', paths: step.branches.paths.map(path => ({ id: path.id, name: path.name, steps: convert(path.steps, depth + 1) })) } } : {}),
    }))
  }
  const doc = validatePortableFlow({ format: 'tld-flow', version: 1, name: flow.name, ...(flow.description ? { description: flow.description } : {}), steps: convert(flow.steps, 0) })
  const source = `# yaml-language-server: $schema=${FLOW_YAML_SCHEMA_URL}\n${stringify(doc, { lineWidth: 0 })}`
  if (byteLength(source) > FLOW_YAML_MAX_BYTES) fail('export', 'YAML must be at most 1 MiB')
  return source
}
