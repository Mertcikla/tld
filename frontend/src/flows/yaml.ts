import { parseDocument, stringify } from 'yaml'
import { FLOW_MAX_STEPS, FLOW_MAX_PATHS, FLOW_MAX_BRANCH_DEPTH, allSteps, type FlowCatalog, type FlowDiagram, type FlowStep, type StoryFlow } from './model'

export const FLOW_YAML_MAX_BYTES = 1024 * 1024
export const FLOW_YAML_SCHEMA_URL = 'https://raw.githubusercontent.com/Mertcikla/tld/refs/heads/main/schemas/flow.schema.json'

export interface PortableConnectorHighlight { source: string; target: string; label?: string }
export interface PortableFlowStep {
  id?: string
  title?: string
  description?: string
  elements?: string[]
  connectors?: PortableConnectorHighlight[]
  branches?: { mode: 'alternate' | 'parallel'; paths: { id?: string; name: string; steps: PortableFlowStep[] }[] }
  goToFlow?: { name: string; view?: string }
}
export interface PortableFlowElement {
  name: string
  kind?: string
  description?: string
  technology?: string
  url?: string
  logo_url?: string
  technology_connectors?: { type: 'catalog' | 'custom'; slug?: string; label: string; is_primary_icon?: boolean }[]
  tags?: string[]
  position_x?: number
  position_y?: number
}
export interface PortableFlowConnector extends PortableConnectorHighlight {
  description?: string
  relationship?: string
  direction?: string
  style?: string
  url?: string
  source_handle?: string
  target_handle?: string
  tags?: string[]
}
export interface PortableFlowResources {
  elements: PortableFlowElement[]
  connectors: PortableFlowConnector[]
}
export interface PortableFlowDocument {
  format: 'tld-flow'
  version: 1
  name: string
  description?: string
  steps: PortableFlowStep[]
  resources?: PortableFlowResources
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

/** Resource bundles contain display data only, never local database identity. */
function validateResources(value: unknown): PortableFlowResources {
  const resources = record(value, 'resources')
  keys(resources, ['elements', 'connectors'], 'resources')
  const strings = (entry: Record<string, unknown>, fields: string[], path: string) => Object.fromEntries(fields
    .filter(field => entry[field] !== undefined)
    .map(field => [field, text(entry[field], `${path}.${field}`, 10000)]))
  const tags = (entry: Record<string, unknown>, path: string) => entry.tags === undefined ? {} : {
    tags: list(entry.tags, `${path}.tags`, 100).map((tag, index) => text(tag, `${path}.tags[${index}]`, 200, true)),
  }
  const elements = list(resources.elements, 'resources.elements', 5000).map((value, index): PortableFlowElement => {
    const path = `resources.elements[${index}]`, element = record(value, path)
    const fields = ['kind', 'description', 'technology', 'url', 'logo_url']
    keys(element, ['name', ...fields, 'tags', 'technology_connectors', 'position_x', 'position_y'], path)
    const result: PortableFlowElement = { name: text(element.name, `${path}.name`, 10000, true), ...strings(element, fields, path), ...tags(element, path) }
    for (const field of ['position_x', 'position_y'] as const) {
      if (element[field] !== undefined) {
        if (typeof element[field] !== 'number' || !Number.isFinite(element[field])) fail(`${path}.${field}`, 'expected a finite number')
        result[field] = element[field]
      }
    }
    if ((result.position_x === undefined) !== (result.position_y === undefined)) fail(path, 'supply both position coordinates')
    if (element.technology_connectors !== undefined) result.technology_connectors = list(element.technology_connectors, `${path}.technology_connectors`, 100).map((value, index) => {
      const location = `${path}.technology_connectors[${index}]`, link = record(value, location)
      keys(link, ['type', 'slug', 'label', 'is_primary_icon'], location)
      if (link.type !== 'catalog' && link.type !== 'custom') fail(location, 'expected catalog or custom technology type')
      if (link.is_primary_icon !== undefined && typeof link.is_primary_icon !== 'boolean') fail(location, 'expected a boolean for is_primary_icon')
      return { type: link.type, label: text(link.label, `${location}.label`, 10000),
        ...(link.slug !== undefined ? { slug: text(link.slug, `${location}.slug`, 10000) } : {}),
        ...(link.is_primary_icon !== undefined ? { is_primary_icon: link.is_primary_icon as boolean } : {}),
      }
    })
    return result
  })
  if (new Set(elements.map(element => element.name)).size !== elements.length) fail('resources.elements', 'duplicate element name')
  const connectors = list(resources.connectors, 'resources.connectors', 5000).map((value, index): PortableFlowConnector => {
    const path = `resources.connectors[${index}]`, connector = record(value, path)
    const fields = ['label', 'description', 'relationship', 'direction', 'style', 'url', 'source_handle', 'target_handle']
    keys(connector, ['source', 'target', ...fields, 'tags'], path)
    const result = { source: text(connector.source, `${path}.source`, 10000, true), target: text(connector.target, `${path}.target`, 10000, true), ...strings(connector, fields, path), ...tags(connector, path) }
    if (![result.source, result.target].every(name => elements.some(element => element.name === name))) fail(path, 'connector endpoints must be included in resources.elements')
    return result
  })
  const references = connectors.map(connector => JSON.stringify([connector.source, connector.target, connector.label ?? '']))
  if (new Set(references).size !== references.length) fail('resources.connectors', 'duplicate connection')
  return { elements, connectors }
}

/** Strict, bounded validation shared by parsing and exporting. No document data executes. */
export function validatePortableFlow(value: unknown): PortableFlowDocument {
  const doc = record(value, 'flow')
  keys(doc, ['format', 'version', 'name', 'description', 'steps', 'resources'], 'flow')
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
      keys(step, ['id', 'title', 'description', 'elements', 'connectors', 'branches', 'goToFlow'], path)
      const result: PortableFlowStep = {}
      const id = optionalId(step.id, `${path}.id`)
      if (id !== undefined) result.id = id
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
      if (result.branches && result.goToFlow) fail(path, 'choose only one of branches or goToFlow')
      if (step.title !== undefined) result.title = text(step.title, `${path}.title`, 500)
      if (step.description !== undefined) result.description = text(step.description, `${path}.description`, 10000)
      if (step.elements !== undefined) {
        result.elements = list(step.elements, `${path}.elements`).map((name, index) => text(name, `${path}.elements[${index}]`, 10000, true))
        if (new Set(result.elements).size !== result.elements.length) fail(path, 'duplicate element highlight')
      }
      if (step.connectors !== undefined) {
        result.connectors = list(step.connectors, `${path}.connectors`).map((value, index) => {
          const location = `${path}.connectors[${index}]`, connector = record(value, location)
          keys(connector, ['source', 'target', 'label'], location)
          return {
            source: text(connector.source, `${location}.source`, 10000, true),
            target: text(connector.target, `${location}.target`, 10000, true),
            ...(connector.label !== undefined ? { label: text(connector.label, `${location}.label`, 10000) } : {}),
          }
        })
        const references = result.connectors.map(c => JSON.stringify([c.source, c.target, c.label ?? '']))
        if (new Set(references).size !== references.length) fail(path, 'duplicate connector highlight')
      }
      return result
    })
  }
  return { format: 'tld-flow', version: 1, name, ...(description !== undefined ? { description } : {}), steps: validateSteps(doc.steps, 'steps', 0), ...(doc.resources !== undefined ? { resources: validateResources(doc.resources) } : {}) }
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
    const connectorIds = (step.connectors ?? []).map((highlight, index) => {
      const location = `${path}.connectors[${index}]`
      const source = elementId(highlight.source, `${location}.source`), target = elementId(highlight.target, `${location}.target`)
      const bound = owns(boundConnectors, index) ? boundConnectors[index] : undefined
      const matches = diagram.connectors.filter(c => c.source_element_id === source && c.target_element_id === target &&
        (bound !== undefined ? c.id === bound : highlight.label === undefined || (c.label ?? '') === highlight.label))
      if (matches.length !== 1) fail(location, matches.length ? 'ambiguous connection; specify a unique label or an explicit binding' : 'connection is not in the target diagram (source, target, and any supplied label must match)')
      return matches[0].id
    })
    if (new Set(elementIds).size !== elementIds.length || new Set(connectorIds).size !== connectorIds.length) fail(path, 'duplicate resolved highlight')
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
    return { id: step.id ?? crypto.randomUUID(), title: step.title ?? '', description: step.description ?? '', elementIds, connectorIds,
      ...(goToFlowId ? { goToFlowId } : {}),
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
      ...(step.connectorIds.length ? { connectors: step.connectorIds.map(id => {
        const connector = diagram.connectors.find(c => c.id === id)
        if (!connector) fail('export', `connection ${id} is unavailable; relink the step before exporting`)
        const label = connector.label ?? ''
        const between = diagram.connectors.filter(c => c.source_element_id === connector.source_element_id && c.target_element_id === connector.target_element_id)
        if (between.filter(c => (c.label ?? '') === label).length !== 1) fail('export', 'connections with the same endpoints need distinct labels before exporting')
        return { source: elementName(connector.source_element_id), target: elementName(connector.target_element_id), ...(label || between.length > 1 ? { label } : {}) }
      }) } : {}),
      ...(step.goToFlowId ? { goToFlow: flowReference(step.goToFlowId) } : {}),
      ...(step.branches ? { branches: { mode: step.branches.mode as 'alternate' | 'parallel', paths: step.branches.paths.map(path => ({ id: path.id, name: path.name, steps: convert(path.steps, depth + 1) })) } } : {}),
    }))
  }
  const doc = validatePortableFlow({ format: 'tld-flow', version: 1, name: flow.name, ...(flow.description ? { description: flow.description } : {}), steps: convert(flow.steps, 0) })
  const source = `# yaml-language-server: $schema=${FLOW_YAML_SCHEMA_URL}\n${stringify(doc, { lineWidth: 0 })}`
  if (byteLength(source) > FLOW_YAML_MAX_BYTES) fail('export', 'YAML must be at most 1 MiB')
  return source
}

/** Opt-in export of the resources referenced anywhere in the flow, including connector endpoints. */
export function exportCompleteFlowYaml(flow: StoryFlow, diagram: FlowDiagram, catalog: FlowCatalog = { flows: [], views: [] }): string {
  const doc = parseFlowYaml(exportFlowYaml(flow, diagram, catalog))
  const steps = allSteps(flow.steps)
  const connectorIds = new Set(steps.flatMap(step => step.connectorIds))
  const connectors = diagram.connectors.filter(connector => connectorIds.has(connector.id))
  const elementIds = new Set([...steps.flatMap(step => step.elementIds), ...connectors.flatMap(connector => [connector.source_element_id, connector.target_element_id])])
  const name = (id: number) => diagram.elements.find(element => element.element_id === id)!.name
  const resources: PortableFlowResources = {
    elements: diagram.elements.filter(element => elementIds.has(element.element_id)).map(element => ({
      name: element.name,
      ...Object.fromEntries(['kind', 'description', 'technology', 'url', 'logo_url', 'tags', 'position_x', 'position_y'].flatMap(field => {
        const value = element[field as keyof typeof element]
        return value === undefined || value === null ? [] : [[field, value]]
      })),
      ...(element.technology_connectors ? { technology_connectors: element.technology_connectors.map(link => ({ type: link.type, label: link.label, ...(link.slug !== undefined ? { slug: link.slug } : {}), is_primary_icon: link.is_primary_icon ?? link.isPrimaryIcon ?? false })) } : {}),
    })),
    connectors: connectors.map(connector => ({
      source: name(connector.source_element_id), target: name(connector.target_element_id), label: connector.label ?? '',
      ...Object.fromEntries(['description', 'relationship', 'direction', 'style', 'url', 'source_handle', 'target_handle', 'tags'].flatMap(field => {
        const value = connector[field as keyof typeof connector]
        return value === undefined || value === null ? [] : [[field, value]]
      })),
    })),
  }
  const source = `# yaml-language-server: $schema=${FLOW_YAML_SCHEMA_URL}\n${stringify(validatePortableFlow({ ...doc, resources }), { lineWidth: 0 })}`
  if (byteLength(source) > FLOW_YAML_MAX_BYTES) fail('export', 'YAML must be at most 1 MiB')
  return source
}
