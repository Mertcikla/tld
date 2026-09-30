import type { Connector, PlacedElement, TechnologyConnector } from '../../types'

export const VIEW_SELECTION_CLIPBOARD_MIME = 'application/x-tldiagram-view-selection+json'

const VIEW_SELECTION_CLIPBOARD_KIND = 'tldiagram.view-selection'
const VIEW_SELECTION_CLIPBOARD_VERSION = 1

export interface ViewSelectionClipboardElement {
  elementId: number
  x: number
  y: number
  name: string
  description: string | null
  kind: string | null
  technology: string | null
  url: string | null
  logo_url: string | null
  technology_connectors: TechnologyConnector[]
  tags: string[]
  repo: string | null
  branch: string | null
  file_path: string | null
  language: string | null
  bypass_noise_gate: boolean
}

export interface ViewSelectionClipboardConnector {
  source_element_id: number
  target_element_id: number
  label: string | null
  description: string | null
  relationship: string | null
  direction: string
  style: string
  url: string | null
  source_handle: string | null
  target_handle: string | null
  tags: string[]
}

export interface ViewSelectionClipboardPayload {
  kind: typeof VIEW_SELECTION_CLIPBOARD_KIND
  version: typeof VIEW_SELECTION_CLIPBOARD_VERSION
  sourceViewId: number
  elements: ViewSelectionClipboardElement[]
  connectors: ViewSelectionClipboardConnector[]
}

export interface ViewSelectionPastePlacement {
  sourceElementId: number
  elementId: number
  x: number
  y: number
}

export interface ViewSelectionPasteConnector extends ViewSelectionClipboardConnector {
  sourceElementId: number
  targetElementId: number
}

export function buildViewSelectionClipboardPayload(
  sourceViewId: number,
  placements: PlacedElement[],
  connectors: Connector[],
  selectedElementIds: number[],
): ViewSelectionClipboardPayload | null {
  const selectedIds = new Set(selectedElementIds)
  const elements = placements
    .filter((placement) => selectedIds.has(placement.element_id))
    .map((placement): ViewSelectionClipboardElement => ({
      elementId: placement.element_id,
      x: placement.position_x,
      y: placement.position_y,
      name: placement.name,
      description: placement.description,
      kind: placement.kind,
      technology: placement.technology,
      url: placement.url,
      logo_url: placement.logo_url,
      technology_connectors: placement.technology_connectors ?? [],
      tags: placement.tags ?? [],
      repo: placement.repo ?? null,
      branch: placement.branch ?? null,
      file_path: placement.file_path ?? null,
      language: placement.language ?? null,
      bypass_noise_gate: placement.bypass_noise_gate ?? false,
    }))

  if (elements.length === 0) return null

  const payloadElementIds = new Set(elements.map((element) => element.elementId))
  return {
    kind: VIEW_SELECTION_CLIPBOARD_KIND,
    version: VIEW_SELECTION_CLIPBOARD_VERSION,
    sourceViewId,
    elements,
    connectors: connectors
      .filter((connector) =>
        connector.view_id === sourceViewId &&
        payloadElementIds.has(connector.source_element_id) &&
        payloadElementIds.has(connector.target_element_id)
      )
      .map((connector): ViewSelectionClipboardConnector => ({
        source_element_id: connector.source_element_id,
        target_element_id: connector.target_element_id,
        label: connector.label,
        description: connector.description,
        relationship: connector.relationship,
        direction: connector.direction,
        style: connector.style,
        url: connector.url,
        source_handle: connector.source_handle,
        target_handle: connector.target_handle,
        tags: connector.tags ?? [],
      })),
  }
}

export function serializeViewSelectionClipboardPayload(payload: ViewSelectionClipboardPayload): string {
  return JSON.stringify(payload)
}

export function parseViewSelectionClipboardPayload(raw: string | null | undefined): ViewSelectionClipboardPayload | null {
  if (!raw?.trim()) return null

  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }

  if (!isRecord(parsed)) return null
  if (parsed.kind !== VIEW_SELECTION_CLIPBOARD_KIND || parsed.version !== VIEW_SELECTION_CLIPBOARD_VERSION) return null
  if (!isPositiveInteger(parsed.sourceViewId)) return null
  if (!Array.isArray(parsed.elements) || !Array.isArray(parsed.connectors)) return null

  const elements = parsed.elements.map(parseClipboardElement)
  if (elements.some((element) => element === null)) return null
  const validElements = elements as ViewSelectionClipboardElement[]
  if (validElements.length === 0) return null

  const elementIds = new Set(validElements.map((element) => element.elementId))
  if (elementIds.size !== validElements.length) return null

  const connectors = parsed.connectors.map((connector) => parseClipboardConnector(connector, elementIds))
  if (connectors.some((connector) => connector === null)) return null

  return {
    kind: VIEW_SELECTION_CLIPBOARD_KIND,
    version: VIEW_SELECTION_CLIPBOARD_VERSION,
    sourceViewId: parsed.sourceViewId,
    elements: validElements,
    connectors: connectors as ViewSelectionClipboardConnector[],
  }
}

export function findViewSelectionPasteConflicts(
  payload: ViewSelectionClipboardPayload,
  existingElementIds: ReadonlySet<number>,
): number[] {
  return payload.elements
    .map((element) => element.elementId)
    .filter((elementId) => existingElementIds.has(elementId))
}

export function findViewSelectionMissingElementIds(
  payload: ViewSelectionClipboardPayload,
  existingElementIds: ReadonlySet<number>,
): number[] {
  return payload.elements
    .map((element) => element.elementId)
    .filter((elementId) => !existingElementIds.has(elementId))
}

export function mapViewSelectionElementIds(
  payload: ViewSelectionClipboardPayload,
  duplicatedElementIdsBySourceId: ReadonlyMap<number, number>,
): Map<number, number> {
  const targetElementIdsBySourceId = new Map<number, number>()
  payload.elements.forEach((element) => {
    targetElementIdsBySourceId.set(
      element.elementId,
      duplicatedElementIdsBySourceId.get(element.elementId) ?? element.elementId,
    )
  })
  return targetElementIdsBySourceId
}

export function planViewSelectionPastePlacements(
  payload: ViewSelectionClipboardPayload,
  targetElementIdsBySourceId: ReadonlyMap<number, number>,
  pasteCenter: { x: number; y: number },
): ViewSelectionPastePlacement[] {
  const sourceCenter = sourceSelectionCenter(payload.elements)
  return payload.elements.map((element) => ({
    sourceElementId: element.elementId,
    elementId: targetElementIdsBySourceId.get(element.elementId) ?? element.elementId,
    x: pasteCenter.x + element.x - sourceCenter.x,
    y: pasteCenter.y + element.y - sourceCenter.y,
  }))
}

export function planViewSelectionPasteConnectors(
  payload: ViewSelectionClipboardPayload,
  targetElementIdsBySourceId: ReadonlyMap<number, number>,
): ViewSelectionPasteConnector[] {
  return payload.connectors.map((connector) => ({
    ...connector,
    sourceElementId: targetElementIdsBySourceId.get(connector.source_element_id) ?? connector.source_element_id,
    targetElementId: targetElementIdsBySourceId.get(connector.target_element_id) ?? connector.target_element_id,
  }))
}

function sourceSelectionCenter(elements: ViewSelectionClipboardElement[]) {
  const left = Math.min(...elements.map((element) => element.x))
  const right = Math.max(...elements.map((element) => element.x))
  const top = Math.min(...elements.map((element) => element.y))
  const bottom = Math.max(...elements.map((element) => element.y))

  return {
    x: left + (right - left) / 2,
    y: top + (bottom - top) / 2,
  }
}

function parseClipboardElement(value: unknown): ViewSelectionClipboardElement | null {
  if (!isRecord(value)) return null
  if (!isPositiveInteger(value.elementId) || !isFiniteNumber(value.x) || !isFiniteNumber(value.y)) return null
  if (typeof value.name !== 'string') return null

  const description = readNullableString(value.description)
  const kind = readNullableString(value.kind)
  const technology = readNullableString(value.technology)
  const url = readNullableString(value.url)
  const logoUrl = readNullableString(value.logo_url)
  const repo = readNullableString(value.repo)
  const branch = readNullableString(value.branch)
  const filePath = readNullableString(value.file_path)
  const language = readNullableString(value.language)
  if (
    description === undefined ||
    kind === undefined ||
    technology === undefined ||
    url === undefined ||
    logoUrl === undefined ||
    repo === undefined ||
    branch === undefined ||
    filePath === undefined ||
    language === undefined
  ) {
    return null
  }

  const technologyConnectors = readTechnologyConnectors(value.technology_connectors ?? [])
  const tags = readStringArray(value.tags ?? [])
  if (!technologyConnectors || !tags) return null

  return {
    elementId: value.elementId,
    x: value.x,
    y: value.y,
    name: value.name,
    description,
    kind,
    technology,
    url,
    logo_url: logoUrl,
    technology_connectors: technologyConnectors,
    tags,
    repo,
    branch,
    file_path: filePath,
    language,
    bypass_noise_gate: typeof value.bypass_noise_gate === 'boolean' ? value.bypass_noise_gate : false,
  }
}

function parseClipboardConnector(value: unknown, elementIds: ReadonlySet<number>): ViewSelectionClipboardConnector | null {
  if (!isRecord(value)) return null
  if (!isPositiveInteger(value.source_element_id) || !isPositiveInteger(value.target_element_id)) return null
  if (!elementIds.has(value.source_element_id) || !elementIds.has(value.target_element_id)) return null
  if (typeof value.direction !== 'string' || typeof value.style !== 'string') return null

  const label = readNullableString(value.label)
  const description = readNullableString(value.description)
  const relationship = readNullableString(value.relationship)
  const url = readNullableString(value.url)
  const sourceHandle = readNullableString(value.source_handle)
  const targetHandle = readNullableString(value.target_handle)
  if (
    label === undefined ||
    description === undefined ||
    relationship === undefined ||
    url === undefined ||
    sourceHandle === undefined ||
    targetHandle === undefined
  ) {
    return null
  }

  const tags = readStringArray(value.tags ?? [])
  if (!tags) return null

  return {
    source_element_id: value.source_element_id,
    target_element_id: value.target_element_id,
    label,
    description,
    relationship,
    direction: value.direction,
    style: value.style,
    url,
    source_handle: sourceHandle,
    target_handle: targetHandle,
    tags,
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isPositiveInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0
}

function readNullableString(value: unknown): string | null | undefined {
  if (value === null || value === undefined) return null
  return typeof value === 'string' ? value : undefined
}

function readStringArray(value: unknown): string[] | null {
  if (!Array.isArray(value)) return null
  return value.every((item) => typeof item === 'string') ? [...value] : null
}

function readTechnologyConnectors(value: unknown): TechnologyConnector[] | null {
  if (!Array.isArray(value)) return null
  const connectors: TechnologyConnector[] = []
  for (const item of value) {
    if (!isRecord(item)) return null
    if (item.type !== 'catalog' && item.type !== 'custom') return null
    if (typeof item.label !== 'string') return null
    const connector: TechnologyConnector = { type: item.type, label: item.label }
    if (item.slug !== undefined && item.slug !== null) {
      if (typeof item.slug !== 'string') return null
      connector.slug = item.slug
    }
    if (item.is_primary_icon !== undefined && item.is_primary_icon !== null) {
      if (typeof item.is_primary_icon !== 'boolean') return null
      connector.is_primary_icon = item.is_primary_icon
    }
    if (item.isPrimaryIcon !== undefined && item.isPrimaryIcon !== null) {
      if (typeof item.isPrimaryIcon !== 'boolean') return null
      connector.isPrimaryIcon = item.isPrimaryIcon
    }
    connectors.push(connector)
  }
  return connectors
}
