import { parseDocument, stringify } from 'yaml'
import type { Codec } from 'libbzip2-wasm'
import type { ExploreData, PlacedElement } from '../types'
import { allSteps, type FlowCatalog, type FlowDiagram, type StoryFlow } from './model'
import { exportCompleteFlowYaml, FLOW_YAML_MAX_BYTES, importFlowYaml, parseFlowYaml, validatePortableFlow, type PortableFlowDocument } from './yaml'

export const FLOW_SNAPSHOT_TOKEN = 'flow-snapshot'
export const WEB_FLOW_SNAPSHOT_URL = `https://tldiagram.com/app/explore/shared/${FLOW_SNAPSHOT_TOKEN}`
const PREFIX = '#flow=v1.'
const MAX_ENCODED_LENGTH = 2 * FLOW_YAML_MAX_BYTES
const MAX_LINKED_FLOWS = 100
interface SnapshotEntry { view: string; flow: PortableFlowDocument }
export interface FlowSnapshot {
  data: ExploreData
  catalog: FlowCatalog
  diagrams: Record<string, FlowDiagram>
}

/** Fetch only missing linked diagrams when sharing from an editor with a single view loaded. */
export async function loadFlowSnapshotDiagrams(flow: StoryFlow, diagram: FlowDiagram, catalog: FlowCatalog | undefined, diagrams: Record<string, FlowDiagram> | undefined, loadDiagram?: (viewId: number) => Promise<FlowDiagram>): Promise<Record<string, FlowDiagram>> {
  const result: Record<string, FlowDiagram> = { ...diagrams, [flow.viewId]: diagram }
  const visited = new Set<string>()
  const pending = [flow]
  while (pending.length) {
    const current = pending.shift()!
    if (visited.has(current.id)) continue
    if (visited.size >= MAX_LINKED_FLOWS) throw new Error('A snapshot can include at most 100 linked flows.')
    visited.add(current.id)
    if (!result[current.viewId] && loadDiagram) result[current.viewId] = await loadDiagram(current.viewId)
    for (const step of allSteps(current.steps)) {
      if (!step.goToFlowId || visited.has(step.goToFlowId)) continue
      const target = catalog?.flows.find(candidate => candidate.id === step.goToFlowId)
      if (!target) throw new Error('A linked flow is unavailable. Reload flows before sharing.')
      pending.push(target)
    }
  }
  return result
}

let codecInstance: Promise<{ codec: Codec }> | undefined
async function bzip(input: Uint8Array, compress: boolean): Promise<Uint8Array> {
  const instance = codecInstance ??= import('libbzip2-wasm').then(({ default: createCodec }) =>
    new Promise<{ codec: Codec }>((resolve, reject) => {
      // Wrap the legacy Emscripten thenable so Promise resolution cannot recurse into it.
      createCodec({ onAbort: reject }).then(codec => resolve({ codec }))
    }))
  try {
    const { codec } = await instance
    const result = compress ? codec.compressBZ2(input, 9, 30) : codec.decompressBZ2(input, 0)
    try {
      if (result.error) throw new Error(compress ? 'Could not compress flow.' : 'The flow snapshot is damaged or too large.')
      const output = result.output
      try {
        if (!compress && output.length > FLOW_YAML_MAX_BYTES) throw new Error('Flow snapshots must be at most 1 MiB of YAML.')
        return output.slice()
      } finally { codec._free(output.byteOffset) }
    } finally { result.delete() }
  } catch (error) {
    // The native wrapper retains its output allocation on errors; release that heap.
    if (codecInstance === instance) codecInstance = undefined
    throw error
  }
}

/** The first entry is the selected flow; capture the transitive closure of linked flows. */
export function exportFlowSnapshotYaml(flow: StoryFlow, diagram: FlowDiagram, catalog: FlowCatalog = { flows: [], views: [] }, diagrams: Record<string, FlowDiagram> = {}): string {
  // Draft changes to the selected flow also apply to links back to that flow.
  catalog = { ...catalog, flows: [flow, ...catalog.flows.filter(candidate => candidate.id !== flow.id)] }
  const entries: SnapshotEntry[] = []
  const visited = new Set<string>()
  const visit = (current: StoryFlow, currentDiagram: FlowDiagram) => {
    if (visited.has(current.id)) return
    if (entries.length >= MAX_LINKED_FLOWS) throw new Error('A snapshot can include at most 100 linked flows.')
    visited.add(current.id)
    entries.push({ view: catalog.views.find(view => view.id === current.viewId)?.name ?? 'Snapshot', flow: parseFlowYaml(exportCompleteFlowYaml(current, currentDiagram, catalog)) })
    for (const step of allSteps(current.steps)) {
      if (!step.goToFlowId || visited.has(step.goToFlowId)) continue
      const target = catalog.flows.find(candidate => candidate.id === step.goToFlowId)
      const targetDiagram = target?.viewId === flow.viewId ? diagram : target && diagrams[target.viewId]
      if (!target || !targetDiagram) throw new Error('Open Explore to share this flow with all of its linked diagrams.')
      visit(target, targetDiagram)
    }
  }
  visit(flow, diagram)
  const source = entries.length === 1 && !allSteps(flow.steps).some(step => step.goToFlowId)
    ? exportCompleteFlowYaml(flow, diagram, catalog)
    : stringify({ format: 'tld-flow-snapshot', version: 1, flows: entries }, { lineWidth: 0 })
  if (new TextEncoder().encode(source).length > FLOW_YAML_MAX_BYTES) throw new Error('Flow snapshots must be at most 1 MiB of YAML.')
  return source
}

/** Resolve display-only resources to fresh, snapshot-local IDs. Never import into a workspace. */
export function parseFlowSnapshotYaml(source: string): FlowSnapshot {
  if (new TextEncoder().encode(source).length > FLOW_YAML_MAX_BYTES) throw new Error('Flow snapshots must be at most 1 MiB of YAML.')
  const document = parseDocument(source, { version: '1.2', uniqueKeys: true, strict: true })
  if (document.errors.length || document.warnings.length || document.directives?.yaml.version !== '1.2') throw new Error('Invalid flow snapshot YAML.')
  const value = document.toJS({ maxAliasCount: 0 })
  let entries: SnapshotEntry[]
  if (value?.format === 'tld-flow') entries = [{ view: value.name, flow: validatePortableFlow(value) }]
  else {
    if (!value || value.format !== 'tld-flow-snapshot' || value.version !== 1 || !Array.isArray(value.flows) || !value.flows.length || value.flows.length > MAX_LINKED_FLOWS || Object.keys(value).some(key => !['format', 'version', 'flows'].includes(key))) throw new Error('Unsupported flow snapshot.')
    entries = value.flows.map((entry: SnapshotEntry) => {
      if (!entry || typeof entry.view !== 'string' || !entry.view.trim() || entry.view.length > 10000 || Object.keys(entry).some(key => !['view', 'flow'].includes(key))) throw new Error('Invalid snapshot diagram.')
      return { view: entry.view, flow: validatePortableFlow(entry.flow) }
    })
  }
  const viewNames = [...new Set(entries.map(entry => entry.view))]
  const data: ExploreData = { tree: [], views: {}, navigations: [] }
  let nextElementId = 1, nextConnectorId = 1
  for (const [index, name] of viewNames.entries()) {
    const id = index + 1
    const elements = new Map<string, PlacedElement>()
    const connectors = new Map<string, FlowDiagram['connectors'][number]>()
    for (const entry of entries.filter(entry => entry.view === name)) {
      if (!entry.flow.resources) throw new Error('The snapshot is missing its elements and connectors.')
      for (const element of entry.flow.resources.elements) {
        if (elements.has(element.name)) continue
        const elementId = nextElementId++
        elements.set(element.name, {
          ...element, id: elementId, element_id: elementId, view_id: id,
          kind: element.kind ?? null, description: element.description ?? null, technology: element.technology ?? null,
          url: element.url ?? null, logo_url: element.logo_url ?? null, tags: element.tags ?? [], technology_connectors: element.technology_connectors ?? [],
          position_x: element.position_x ?? ((elements.size % 4) * 280), position_y: element.position_y ?? (Math.floor(elements.size / 4) * 180),
          has_view: false, view_label: null,
        })
      }
    }
    for (const entry of entries.filter(entry => entry.view === name)) {
      for (const connector of entry.flow.resources!.connectors) {
        const key = JSON.stringify([connector.source, connector.target, connector.label ?? ''])
        if (connectors.has(key)) continue
        connectors.set(key, {
          ...connector, id: nextConnectorId++, view_id: id,
          source_element_id: elements.get(connector.source)!.element_id, target_element_id: elements.get(connector.target)!.element_id,
          label: connector.label ?? null, description: connector.description ?? null, relationship: connector.relationship ?? null,
          direction: connector.direction ?? 'forward', style: connector.style ?? 'solid', url: connector.url ?? null,
          source_handle: connector.source_handle ?? null, target_handle: connector.target_handle ?? null, created_at: '', updated_at: '',
        })
      }
    }
    data.tree.push({ id, name, description: null, level_label: null, level: 0, depth: 0, parent_view_id: null, children: [], created_at: '', updated_at: '' })
    data.views[id] = { placements: [...elements.values()], connectors: [...connectors.values()] }
  }
  const diagrams = Object.fromEntries(Object.entries(data.views).map(([id, view]) => [id, { elements: view.placements, connectors: view.connectors }]))
  const catalog: FlowCatalog = {
    views: data.tree.map(view => ({ id: view.id, name: view.name })),
    flows: entries.map((entry, index) => ({ id: `snapshot-flow-${index}`, name: entry.flow.name, viewId: viewNames.indexOf(entry.view) + 1, description: '', revision: 0, steps: [] })),
  }
  if (new Set(catalog.flows.map(flow => JSON.stringify([flow.viewId, flow.name]))).size !== entries.length) throw new Error('Duplicate flow in snapshot.')
  catalog.flows = entries.map((entry, index) => {
    const stub = catalog.flows[index]
    return { ...importFlowYaml(stringify(entry.flow), stub.viewId, diagrams[stub.viewId], {}, catalog), id: stub.id }
  })
  return { data, catalog, diagrams }
}

export async function encodeFlowSnapshot(source: string): Promise<string> {
  if (new TextEncoder().encode(source).length > FLOW_YAML_MAX_BYTES) throw new Error('Flow snapshots must be at most 1 MiB of YAML.')
  const bytes = await bzip(new TextEncoder().encode(source), true)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return PREFIX + btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export async function decodeFlowSnapshot(hash: string): Promise<FlowSnapshot> {
  if (!hash.startsWith(PREFIX)) throw new Error('This flow snapshot link is missing data or uses an unsupported version.')
  const encoded = hash.slice(PREFIX.length)
  if (!encoded || encoded.length > MAX_ENCODED_LENGTH || !/^[A-Za-z0-9_-]+$/.test(encoded) || encoded.length % 4 === 1) throw new Error('Invalid flow snapshot link.')
  const binary = atob(encoded.replace(/-/g, '+').replace(/_/g, '/'))
  const output = await bzip(Uint8Array.from(binary, byte => byte.charCodeAt(0)), false)
  return parseFlowSnapshotYaml(new TextDecoder('utf-8', { fatal: true }).decode(output))
}

export async function createFlowSnapshotLink(baseUrl: string, source: string): Promise<string> {
  let url = new URL(baseUrl)
  if (url.protocol !== 'http:' && url.protocol !== 'https:') url = new URL(WEB_FLOW_SNAPSHOT_URL)
  url.search = ''
  url.hash = await encodeFlowSnapshot(source)
  return url.href
}
