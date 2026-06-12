import type { PlanConnector, PlanElement, TechnologyLink } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'

interface TldElementMetadata {
  ref?: string
  x?: number
  y?: number
  kind?: string
  description?: string
  technology?: string
  url?: string
  logoUrl?: string
  tags?: string[]
  technologyLinks?: TechnologyLink[]
  repo?: string
  branch?: string
  filePath?: string
  language?: string
  bypassNoiseGate?: boolean
  hasView?: boolean
  viewLabel?: string
}

interface TldConnectorMetadata {
  ref?: string
  sourceRef?: string
  targetRef?: string
  label?: string
  description?: string
  relationship?: string
  direction?: string
  style?: string
  url?: string
  sourceHandle?: string
  targetHandle?: string
}

interface TldMetadata {
  elements: Map<string, TldElementMetadata>
  connectors: TldConnectorMetadata[]
}

interface MetadataImportResult {
  elements: PlanElement[]
  connectors: PlanConnector[]
}

export interface TldTechnologyLinkExport {
  type: 'catalog' | 'custom'
  slug?: string
  label: string
  is_primary_icon?: boolean
  isPrimaryIcon?: boolean
}

function splitEscaped(value: string, separator: string) {
  const parts: string[] = []
  let current = ''
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index]
    if (char === '\\' && index + 1 < value.length) {
      current += char + value[index + 1]
      index += 1
      continue
    }
    if (char === separator) {
      parts.push(current)
      current = ''
      continue
    }
    current += char
  }
  parts.push(current)
  return parts
}

export function escapeTldMetadataValue(value: string) {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/\n/g, '\\n')
    .replace(/\r/g, '\\r')
    .replace(/\t/g, '\\t')
    .replace(/=/g, '\\=')
    .replace(/,/g, '\\,')
    .replace(/\|/g, '\\|')
    .replace(/:/g, '\\:')
}

function unescapeTldMetadataValue(value: string) {
  let out = ''
  for (let index = 0; index < value.length; index += 1) {
    const char = value[index]
    if (char !== '\\') {
      out += char
      continue
    }
    index += 1
    if (index >= value.length) return null
    switch (value[index]) {
      case '\\':
      case '=':
      case ',':
      case '|':
      case ':':
        out += value[index]
        break
      case 'n':
        out += '\n'
        break
      case 'r':
        out += '\r'
        break
      case 't':
        out += '\t'
        break
      default:
        return null
    }
  }
  return out
}

export function encodeTldStringList(values: string[]) {
  return values.map(escapeTldMetadataValue).join(',')
}

function decodeTldStringList(value: string) {
  if (!value) return []
  const out: string[] = []
  for (const raw of splitEscaped(value, ',')) {
    const item = unescapeTldMetadataValue(raw)
    if (item === null) return null
    out.push(item)
  }
  return out
}

function technologyLinkIsPrimary(link: TldTechnologyLinkExport) {
  return Boolean(link.is_primary_icon ?? link.isPrimaryIcon)
}

export function encodeTldTechnologyLinks(values: TldTechnologyLinkExport[]) {
  return values.map((link) => [
    link.type,
    link.slug ?? '',
    link.label,
    technologyLinkIsPrimary(link) ? '1' : '0',
  ].map(escapeTldMetadataValue).join(':')).join('|')
}

function decodeTldTechnologyLinks(value: string): TechnologyLink[] | null {
  if (!value) return []
  const links: TechnologyLink[] = []
  for (const rawLink of splitEscaped(value, '|')) {
    const fields = splitEscaped(rawLink, ':')
    if (fields.length < 3 || fields.length > 4) return null
    const [rawType, rawSlug, rawLabel, rawPrimary = '0'] = fields
    const type = unescapeTldMetadataValue(rawType)
    const slug = unescapeTldMetadataValue(rawSlug)
    const label = unescapeTldMetadataValue(rawLabel)
    const primary = unescapeTldMetadataValue(rawPrimary)
    if ((type !== 'catalog' && type !== 'custom') || slug === null || label === null || primary === null) return null
    if (!label.trim()) return null
    links.push({
      type,
      slug: slug || undefined,
      label,
      isPrimaryIcon: primary === '1' || primary === 'true',
    } as TechnologyLink)
  }
  return links
}

export function compactTldNumber(value: number) {
  return Number.isInteger(value) ? String(value) : String(Math.round(value * 1000) / 1000)
}

export function finiteTldNumber(value: unknown) {
  const number = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(number) ? number : null
}

function decodeTldNumber(value: string) {
  const raw = unescapeTldMetadataValue(value)
  if (raw === null) return null
  return finiteTldNumber(raw)
}

function decodeTldBoolean(value: string) {
  const raw = unescapeTldMetadataValue(value)
  if (raw === null) return null
  if (raw === '1' || raw === 'true') return true
  if (raw === '0' || raw === 'false') return false
  return null
}

function parseMetadataPairs(text: string) {
  const pairs = new Map<string, string>()
  const matches = Array.from(text.matchAll(/(^|\s+)([A-Za-z][A-Za-z0-9_]*)=/g))
  for (let index = 0; index < matches.length; index += 1) {
    const match = matches[index]
    const key = match[2]
    const keyStart = match.index + match[1].length
    const valueStart = keyStart + key.length + 1
    const nextMatch = matches[index + 1]
    const valueEnd = nextMatch?.index ?? text.length
    pairs.set(key, text.slice(valueStart, valueEnd))
  }
  return pairs
}

function parseElementMetadata(pairs: Map<string, string>): TldElementMetadata {
  const metadata: TldElementMetadata = {}
  const stringKeys: Array<[string, keyof TldElementMetadata]> = [
    ['ref', 'ref'],
    ['kind', 'kind'],
    ['desc', 'description'],
    ['tech', 'technology'],
    ['url', 'url'],
    ['logo', 'logoUrl'],
    ['repo', 'repo'],
    ['branch', 'branch'],
    ['file', 'filePath'],
    ['lang', 'language'],
    ['viewLabel', 'viewLabel'],
  ]

  for (const [key, field] of stringKeys) {
    const value = pairs.get(key)
    if (value === undefined) continue
    const decoded = unescapeTldMetadataValue(value)
    if (decoded) metadata[field] = decoded as never
  }

  const x = pairs.get('x')
  const y = pairs.get('y')
  if (x !== undefined) {
    const decoded = decodeTldNumber(x)
    if (decoded !== null) metadata.x = decoded
  }
  if (y !== undefined) {
    const decoded = decodeTldNumber(y)
    if (decoded !== null) metadata.y = decoded
  }

  const tags = pairs.get('tags')
  if (tags !== undefined) {
    const decoded = decodeTldStringList(tags)
    if (decoded && decoded.length > 0) metadata.tags = decoded
  }

  const technologyLinks = pairs.get('techLinks')
  if (technologyLinks !== undefined) {
    const decoded = decodeTldTechnologyLinks(technologyLinks)
    if (decoded && decoded.length > 0) metadata.technologyLinks = decoded
  }

  const bypass = pairs.get('bypass')
  if (bypass !== undefined) {
    const decoded = decodeTldBoolean(bypass)
    if (decoded !== null) metadata.bypassNoiseGate = decoded
  }

  const hasView = pairs.get('hasView')
  if (hasView !== undefined) {
    const decoded = decodeTldBoolean(hasView)
    if (decoded !== null) metadata.hasView = decoded
  }

  return metadata
}

function parseConnectorMetadata(pairs: Map<string, string>): TldConnectorMetadata {
  const metadata: TldConnectorMetadata = {}
  const stringKeys: Array<[string, keyof TldConnectorMetadata]> = [
    ['ref', 'ref'],
    ['source', 'sourceRef'],
    ['target', 'targetRef'],
    ['label', 'label'],
    ['desc', 'description'],
    ['rel', 'relationship'],
    ['dir', 'direction'],
    ['style', 'style'],
    ['url', 'url'],
    ['sourceHandle', 'sourceHandle'],
    ['targetHandle', 'targetHandle'],
  ]

  for (const [key, field] of stringKeys) {
    const value = pairs.get(key)
    if (value === undefined) continue
    const decoded = unescapeTldMetadataValue(value)
    if (decoded) metadata[field] = decoded
  }

  return metadata
}

function mergeConnectorMetadata(
  base: TldConnectorMetadata | undefined,
  override: TldConnectorMetadata | undefined,
): TldConnectorMetadata | undefined {
  if (!base) return override
  if (!override) return base
  return { ...base, ...override }
}

function isExportedFlowchartConnectorLine(line: string) {
  return /^[A-Za-z_][A-Za-z0-9_]*\s+(?:--\s+"(?:\\.|[^"\\])*"\s+-->|-->)\s+[A-Za-z_][A-Za-z0-9_]*$/.test(line)
}

function exportedFlowchartElementRef(line: string) {
  return line.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*\["(?:\\.|[^"\\])*"\]$/)?.[1] ?? null
}

export function parseTldMetadata(source: string): TldMetadata | null {
  const lines = source.split(/\r?\n/)
  const metadata: TldMetadata = {
    elements: new Map(),
    connectors: [],
  }
  let markerSeen = false
  let lastElementRef: string | null = null
  let lastConnectorIndex = -1

  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('%%')) {
      if (markerSeen) {
        lastElementRef = exportedFlowchartElementRef(trimmed)
        if (isExportedFlowchartConnectorLine(trimmed)) {
          metadata.connectors.push({})
          lastConnectorIndex = metadata.connectors.length - 1
          lastElementRef = null
        }
      }
      continue
    }
    const body = trimmed.slice(2).trim()
    if (/^tld\/v1(?:\s|$)/.test(body)) {
      markerSeen = true
      lastElementRef = null
      lastConnectorIndex = -1
      continue
    }
    if (!markerSeen) continue

    const metadataLine = body.match(/^(tld-element|tld-connector)(?:\s+(.*))?$/)
    if (!metadataLine) continue
    const [, kind, metadataText = ''] = metadataLine
    const pairs = parseMetadataPairs(metadataText)
    if (pairs.size === 0) continue
    if (kind === 'tld-element') {
      if (!lastElementRef) continue
      metadata.elements.set(lastElementRef, parseElementMetadata(pairs))
    } else if (kind === 'tld-connector') {
      const item = parseConnectorMetadata(pairs)
      if (lastConnectorIndex >= 0) {
        metadata.connectors[lastConnectorIndex] = mergeConnectorMetadata(metadata.connectors[lastConnectorIndex], item) ?? {}
      } else {
        metadata.connectors.push(item)
      }
    }
  }

  return markerSeen ? metadata : null
}

export function stripMermaidCommentLines(source: string) {
  return source
    .split(/\r?\n/)
    .filter((line) => !line.trim().startsWith('%%'))
    .join('\n')
    .trim()
}

export function applyTldMetadata(result: MetadataImportResult, metadata: TldMetadata | null) {
  if (!metadata) return

  const explicitRefs = new Map<string, string>()
  for (const element of result.elements) {
    const item = metadata.elements.get(element.ref)
    if (!item) continue
    if (item.ref && item.ref !== element.ref) {
      explicitRefs.set(element.ref, item.ref)
      element.ref = item.ref
    }
    if (item.kind) element.kind = item.kind
    if (item.description) element.description = item.description
    if (item.technology) element.technology = item.technology
    if (item.url) element.url = item.url
    if (item.logoUrl) element.logoUrl = item.logoUrl
    if (item.tags) element.tags = item.tags
    if (item.technologyLinks) element.technologyLinks = item.technologyLinks
    if (item.repo) element.repo = item.repo
    if (item.branch) element.branch = item.branch
    if (item.filePath) element.filePath = item.filePath
    if (item.language) element.language = item.language
    if (item.bypassNoiseGate !== undefined) element.bypassNoiseGate = item.bypassNoiseGate
    if (item.hasView !== undefined) element.hasView = item.hasView
    if (item.viewLabel) element.viewLabel = item.viewLabel
    if (item.x !== undefined && item.y !== undefined) {
      const placement = element.placements?.[0] ?? { parentRef: 'root' }
      placement.parentRef = placement.parentRef || 'root'
      placement.positionX = item.x
      placement.positionY = item.y
      element.placements = [placement]
    }
  }

  if (explicitRefs.size > 0) {
    for (const connector of result.connectors) {
      connector.sourceElementRef = explicitRefs.get(connector.sourceElementRef) ?? connector.sourceElementRef
      connector.targetElementRef = explicitRefs.get(connector.targetElementRef) ?? connector.targetElementRef
      connector.viewRef = explicitRefs.get(connector.viewRef) ?? connector.viewRef
    }
  }

  result.connectors.forEach((connector, index) => {
    const item = metadata.connectors[index]
    if (!item) return
    if (item.ref) connector.ref = item.ref
    if (item.sourceRef) connector.sourceElementRef = item.sourceRef
    if (item.targetRef) connector.targetElementRef = item.targetRef
    if (item.label) connector.label = item.label
    if (item.description) connector.description = item.description
    if (item.relationship) connector.relationship = item.relationship
    if (item.direction) connector.direction = item.direction
    if (item.style) connector.style = item.style
    if (item.url) connector.url = item.url
    if (item.sourceHandle) connector.sourceHandle = item.sourceHandle
    if (item.targetHandle) connector.targetHandle = item.targetHandle
  })
}
