import type { Connector, PlacedElement } from '../../types'
import {
  compactTldNumber,
  encodeTldStringList,
  encodeTldTechnologyLinks,
  escapeTldMetadataValue,
  finiteTldNumber,
} from '../mermaid/metadata'

export interface MermaidWorkspaceView {
  placements: readonly PlacedElement[]
  connectors: readonly Connector[]
}

type MetadataEntry = [key: string, encodedValue: string]

function asTrimmedString(value: string | null | undefined) {
  const trimmed = value?.trim()
  return trimmed || ''
}

function escapeMermaidLabel(value: string) {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\r?\n/g, ' ')
}

function sanitizeMermaidId(value: string) {
  const sanitized = value.replace(/[^A-Za-z0-9_]/g, '_')
  return /^[A-Za-z_]/.test(sanitized) ? sanitized : `node_${sanitized}`
}

function metadataComment(kind: 'element' | 'connector', subject: string, entries: MetadataEntry[]) {
  const pairs = entries.map(([key, value]) => `${key}=${value}`).join(' ')
  return pairs ? `%% ${kind} ${subject} ${pairs}` : `%% ${kind} ${subject}`
}

function stringEntry(key: string, value: string | null | undefined): MetadataEntry | null {
  const trimmed = asTrimmedString(value)
  return trimmed ? [key, escapeTldMetadataValue(trimmed)] : null
}

function pushStringEntry(entries: MetadataEntry[], key: string, value: string | null | undefined) {
  const entry = stringEntry(key, value)
  if (entry) entries.push(entry)
}

function elementMetadataEntries(element: PlacedElement): MetadataEntry[] {
  const entries: MetadataEntry[] = [
    ['x', compactTldNumber(finiteTldNumber(element.position_x) ?? 0)],
    ['y', compactTldNumber(finiteTldNumber(element.position_y) ?? 0)],
  ]
  const kind = asTrimmedString(element.kind)
  if (kind && kind !== 'system') entries.push(['kind', escapeTldMetadataValue(kind)])
  pushStringEntry(entries, 'desc', element.description)
  pushStringEntry(entries, 'tech', element.technology)
  pushStringEntry(entries, 'url', element.url)
  pushStringEntry(entries, 'logo', element.logo_url)
  const tags = (element.tags ?? []).filter((tag) => tag.trim())
  if (tags.length > 0) entries.push(['tags', encodeTldStringList(tags)])
  const technologyLinks = (element.technology_connectors ?? []).filter((link) => link.label.trim())
  if (technologyLinks.length > 0) entries.push(['techLinks', encodeTldTechnologyLinks(technologyLinks)])
  pushStringEntry(entries, 'repo', element.repo)
  pushStringEntry(entries, 'branch', element.branch)
  pushStringEntry(entries, 'file', element.file_path)
  pushStringEntry(entries, 'lang', element.language)
  if (element.bypass_noise_gate) entries.push(['bypass', '1'])
  if (element.has_view) entries.push(['hasView', '1'])
  pushStringEntry(entries, 'viewLabel', element.view_label)
  return entries
}

function connectorMetadataEntries(connector: Connector): MetadataEntry[] {
  const entries: MetadataEntry[] = []
  pushStringEntry(entries, 'desc', connector.description)
  pushStringEntry(entries, 'rel', connector.relationship)
  const direction = asTrimmedString(connector.direction)
  if (direction && direction !== 'forward') entries.push(['dir', escapeTldMetadataValue(direction)])
  const style = asTrimmedString(connector.style)
  if (style && style !== 'bezier') entries.push(['style', escapeTldMetadataValue(style)])
  pushStringEntry(entries, 'url', connector.url)
  const sourceHandle = asTrimmedString(connector.source_handle)
  if (sourceHandle && sourceHandle !== 'right') entries.push(['sourceHandle', escapeTldMetadataValue(sourceHandle)])
  const targetHandle = asTrimmedString(connector.target_handle)
  if (targetHandle && targetHandle !== 'left') entries.push(['targetHandle', escapeTldMetadataValue(targetHandle)])
  return entries
}

export class MermaidExporter {
  constructor(private readonly view: MermaidWorkspaceView) {}

  toMermaid() {
    const elementIds = new Set(this.view.placements.map((element) => element.element_id))
    const sortedElements = [...this.view.placements].sort((a, b) => a.element_id - b.element_id)
    const sortedConnectors = this.view.connectors
      .filter((connector) => elementIds.has(connector.source_element_id) && elementIds.has(connector.target_element_id))
      .sort((a, b) => a.id - b.id)

    const lines = ['flowchart LR', '%% tld/v1']
    for (const element of sortedElements) {
      const nodeId = sanitizeMermaidId(`node_${element.element_id}`)
      lines.push(`  ${nodeId}["${escapeMermaidLabel(element.name)}"]`)
      lines.push(metadataComment('element', nodeId, elementMetadataEntries(element)))
    }
    if (sortedElements.length > 0 && sortedConnectors.length > 0) lines.push('')
    for (const connector of sortedConnectors) {
      const sourceId = sanitizeMermaidId(`node_${connector.source_element_id}`)
      const targetId = sanitizeMermaidId(`node_${connector.target_element_id}`)
      const label = connector.label?.trim()
      lines.push(label
        ? `  ${sourceId} -- "${escapeMermaidLabel(label)}" --> ${targetId}`
        : `  ${sourceId} --> ${targetId}`)
      lines.push(metadataComment('connector', `${sourceId}->${targetId}`, connectorMetadataEntries(connector)))
    }

    return `${lines.join('\n')}\n`
  }
}

export function serializeViewToMermaid(viewElements: readonly PlacedElement[], connectors: readonly Connector[]) {
  return new MermaidExporter({ placements: viewElements, connectors }).toMermaid()
}
