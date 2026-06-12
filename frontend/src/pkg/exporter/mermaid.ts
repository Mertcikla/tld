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

export interface MermaidExportOptions {
  includeTldMetadata?: boolean
  viewId?: number | null
}

type MetadataEntry = [key: string, encodedValue: string]

function asTrimmedString(value: string | null | undefined) {
  const trimmed = value?.trim()
  return trimmed || ''
}

function escapeMermaidLabel(value: string) {
  return value
    .replace(/\r?\n/g, ' ')
    .replace(/&/g, '&amp;')
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '&quot;')
}

function sanitizeMermaidId(value: string) {
  const sanitized = value.replace(/[^A-Za-z0-9_]/g, '_')
  return /^[A-Za-z_]/.test(sanitized) ? sanitized : `node_${sanitized}`
}

function metadataComment(kind: 'tld-element' | 'tld-connector', subject: string | null, entries: MetadataEntry[]) {
  const parts = [`%% ${kind}`]
  if (subject) parts.push(subject)
  parts.push(...entries.map(([key, value]) => `${key}=${value}`))
  return parts.join(' ')
}

function inferViewId(view: MermaidWorkspaceView, explicitViewId: number | null | undefined) {
  if (explicitViewId !== undefined) return explicitViewId
  const ids = new Set<number>()
  for (const placement of view.placements) ids.add(placement.view_id)
  for (const connector of view.connectors) ids.add(connector.view_id)
  return ids.size === 1 ? [...ids][0] : null
}

function stringEntry(key: string, value: string | null | undefined): MetadataEntry | null {
  const trimmed = asTrimmedString(value)
  return trimmed ? [key, escapeTldMetadataValue(trimmed)] : null
}

function pushStringEntry(entries: MetadataEntry[], key: string, value: string | null | undefined) {
  const entry = stringEntry(key, value)
  if (entry) entries.push(entry)
}

function elementMetadataEntries(element: PlacedElement, ref: string): MetadataEntry[] {
  const entries: MetadataEntry[] = [
    ['ref', escapeTldMetadataValue(ref)],
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

function connectorMetadataEntries(connector: Connector, ref: string, sourceRef: string, targetRef: string): MetadataEntry[] {
  const entries: MetadataEntry[] = [
    ['ref', escapeTldMetadataValue(ref)],
    ['source', escapeTldMetadataValue(sourceRef)],
    ['target', escapeTldMetadataValue(targetRef)],
  ]
  pushStringEntry(entries, 'label', connector.label)
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
  constructor(
    private readonly view: MermaidWorkspaceView,
    private readonly options: MermaidExportOptions = {},
  ) {}

  toMermaid() {
    const includeTldMetadata = this.options.includeTldMetadata ?? true
    const elementIds = new Set(this.view.placements.map((element) => element.element_id))
    const sortedElements = [...this.view.placements].sort((a, b) => a.element_id - b.element_id)
    const sortedConnectors = this.view.connectors
      .filter((connector) => elementIds.has(connector.source_element_id) && elementIds.has(connector.target_element_id))
      .sort((a, b) => a.id - b.id)

    const viewId = inferViewId(this.view, this.options.viewId)
    const marker = viewId !== null ? `%% tld/v1 view=${viewId}` : '%% tld/v1'
    const lines = includeTldMetadata ? ['flowchart LR', marker] : ['flowchart LR']
    for (const element of sortedElements) {
      const nodeId = sanitizeMermaidId(`node_${element.element_id}`)
      lines.push(`  ${nodeId}["${escapeMermaidLabel(element.name)}"]`)
      if (includeTldMetadata) lines.push(metadataComment('tld-element', null, elementMetadataEntries(element, `node_${element.element_id}`)))
    }
    if (sortedElements.length > 0 && sortedConnectors.length > 0) lines.push('')
    for (const connector of sortedConnectors) {
      const sourceId = sanitizeMermaidId(`node_${connector.source_element_id}`)
      const targetId = sanitizeMermaidId(`node_${connector.target_element_id}`)
      const label = connector.label?.trim()
      lines.push(label
        ? `  ${sourceId} -- "${escapeMermaidLabel(label)}" --> ${targetId}`
        : `  ${sourceId} --> ${targetId}`)
      const metadataEntries = connectorMetadataEntries(
        connector,
        String(connector.id),
        `node_${connector.source_element_id}`,
        `node_${connector.target_element_id}`,
      )
      if (includeTldMetadata) lines.push(metadataComment('tld-connector', null, metadataEntries))
    }

    return `${lines.join('\n')}\n`
  }

  toMarkdownBlock() {
    return `\`\`\`mermaid\n${this.toMermaid()}\`\`\`\n`
  }
}

export function serializeViewToMermaid(
  viewElements: readonly PlacedElement[],
  connectors: readonly Connector[],
  options?: MermaidExportOptions,
) {
  return new MermaidExporter({ placements: viewElements, connectors }, options).toMermaid()
}

export function serializeViewToMermaidMarkdownBlock(
  viewElements: readonly PlacedElement[],
  connectors: readonly Connector[],
  options?: MermaidExportOptions,
) {
  return new MermaidExporter({ placements: viewElements, connectors }, options).toMarkdownBlock()
}
