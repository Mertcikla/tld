import { api } from '../../api/client'
import type { ParsedImport, MermaidDirection } from '../../pkg/importer/mermaid'
import type {
  Connector,
  LibraryElement as WorkspaceElement,
  PlacedElement,
  TechnologyConnector,
} from '../../types'

type CreateElementPayload = Parameters<typeof api.elements.create>[0]
type CreateViewPayload = Parameters<typeof api.workspace.views.create>[0]
type CreateConnectorPayload = Parameters<typeof api.workspace.connectors.create>[1]

export interface MermaidImportClient {
  getElement(id: number): ReturnType<typeof api.elements.get>
  createElement(data: CreateElementPayload): ReturnType<typeof api.elements.create>
  deleteElement(id: number): Promise<void>
  createView(data: CreateViewPayload): ReturnType<typeof api.workspace.views.create>
  deleteView(viewId: number): Promise<void>
  addPlacement(viewId: number, elementId: number, x: number, y: number): ReturnType<typeof api.workspace.views.placements.add>
  removePlacement(viewId: number, elementId: number): Promise<void>
  createConnector(viewId: number, data: CreateConnectorPayload): ReturnType<typeof api.workspace.connectors.create>
  deleteConnector(connectorId: number): Promise<void>
}

export interface MermaidLocalImportSummary {
  resolvedElementCount: number
  createdElementCount: number
  resolvedConnectorCount: number
  createdConnectorCount: number
  importedElementIds: Set<number>
}

export interface ImportMermaidIntoViewOptions {
  parsed: ParsedImport
  currentViewId: number
  center: { x: number; y: number }
  allElements: readonly WorkspaceElement[]
  viewElements: readonly PlacedElement[]
  connectors: readonly Connector[]
  onConnectorCreated?: (connector: Connector) => void
  client?: MermaidImportClient
}

function mermaidConnectorHandles(direction: MermaidDirection) {
  if (direction === 'RL') return { source_handle: 'left', target_handle: 'right' }
  if (direction === 'TB' || direction === 'TD') return { source_handle: 'bottom', target_handle: 'top' }
  if (direction === 'BT') return { source_handle: 'top', target_handle: 'bottom' }
  return { source_handle: 'right', target_handle: 'left' }
}

type MermaidConnectorHandles = ReturnType<typeof mermaidConnectorHandles>

export function mermaidRefElementId(ref: string) {
  const match = /^node_(\d+)$/.exec(ref)
  if (!match) return null
  const id = Number(match[1])
  return Number.isSafeInteger(id) && id > 0 ? id : null
}

function countLabel(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`
}

export function mermaidLocalImportDescription(summary: MermaidLocalImportSummary) {
  return [
    `Resolved ${countLabel(summary.resolvedElementCount, 'element')} and ${countLabel(summary.resolvedConnectorCount, 'connector')}.`,
    `Created ${countLabel(summary.createdElementCount, 'element')} and ${countLabel(summary.createdConnectorCount, 'connector')}.`,
  ].join(' ')
}

function normalizedConnectorText(value: string | null | undefined) {
  return value?.trim() ?? ''
}

function normalizedConnectorDirection(value: string | null | undefined) {
  return normalizedConnectorText(value) || 'forward'
}

function normalizedConnectorStyle(value: string | null | undefined) {
  return normalizedConnectorText(value) || 'bezier'
}

function mermaidParsedConnectorKey(
  connector: ParsedImport['connectors'][number],
  sourceElementId: number,
  targetElementId: number,
  handles: MermaidConnectorHandles,
) {
  return JSON.stringify([
    sourceElementId,
    targetElementId,
    normalizedConnectorText(connector.label),
    normalizedConnectorText(connector.description),
    normalizedConnectorText(connector.relationship),
    normalizedConnectorDirection(connector.direction),
    normalizedConnectorStyle(connector.style),
    normalizedConnectorText(connector.url),
    normalizedConnectorText(connector.sourceHandle) || handles.source_handle,
    normalizedConnectorText(connector.targetHandle) || handles.target_handle,
  ])
}

function mermaidExistingConnectorKey(connector: Connector, handles: MermaidConnectorHandles) {
  return JSON.stringify([
    connector.source_element_id,
    connector.target_element_id,
    normalizedConnectorText(connector.label),
    normalizedConnectorText(connector.description),
    normalizedConnectorText(connector.relationship),
    normalizedConnectorDirection(connector.direction),
    normalizedConnectorStyle(connector.style),
    normalizedConnectorText(connector.url),
    normalizedConnectorText(connector.source_handle) || handles.source_handle,
    normalizedConnectorText(connector.target_handle) || handles.target_handle,
  ])
}

function finitePlanNumber(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function mermaidImportMetadataPosition(element: ParsedImport['elements'][number]) {
  const placement = element.placements?.[0]
  const x = finitePlanNumber(placement?.positionX)
  const y = finitePlanNumber(placement?.positionY)
  return x === null || y === null ? null : { x, y }
}

function mermaidImportTechnologyConnectors(element: ParsedImport['elements'][number]): TechnologyConnector[] {
  return (element.technologyLinks ?? [])
    .filter((link) => link.label.trim())
    .map((link): TechnologyConnector => ({
      type: link.type === 'catalog' ? 'catalog' : 'custom',
      slug: link.slug || undefined,
      label: link.label,
      is_primary_icon: link.isPrimaryIcon,
    }))
}

export function mermaidImportReviewWarnings(parsed: ParsedImport, allElements: readonly WorkspaceElement[]) {
  const existingById = new Map(allElements.map((element) => [element.id, element]))
  const mismatches: string[] = []
  for (const element of parsed.elements) {
    const existingId = mermaidRefElementId(element.ref)
    if (existingId === null) continue
    const existing = existingById.get(existingId)
    if (!existing) continue
    const importedName = element.name?.trim() ?? ''
    const existingName = existing.name?.trim() ?? ''
    if (!importedName || !existingName || importedName === existingName) continue
    mismatches.push(`${element.ref}: "${importedName}" will reuse "${existingName}"`)
  }
  if (mismatches.length === 0) return []
  const samples = mismatches.slice(0, 3)
  const suffix = mismatches.length > samples.length ? `, and ${mismatches.length - samples.length} more` : ''
  return [`${countLabel(mismatches.length, 'Mermaid node')} will reuse existing workspace elements with different names: ${samples.join('; ')}${suffix}.`]
}

function layoutMermaidRefs(parsed: ParsedImport, refs: string[], center: { x: number; y: number }) {
  const refSet = new Set(refs)
  const outgoing = new Map<string, string[]>()
  const indegree = new Map<string, number>()
  const rank = new Map<string, number>()
  refs.forEach((ref) => {
    outgoing.set(ref, [])
    indegree.set(ref, 0)
    rank.set(ref, 0)
  })

  parsed.connectors.forEach((connector) => {
    const source = connector.sourceElementRef
    const target = connector.targetElementRef
    if (!refSet.has(source) || !refSet.has(target)) return
    outgoing.get(source)?.push(target)
    indegree.set(target, (indegree.get(target) ?? 0) + 1)
  })

  const queue = refs.filter((ref) => (indegree.get(ref) ?? 0) === 0)
  let cursor = 0
  while (cursor < queue.length) {
    const ref = queue[cursor++]
    for (const target of outgoing.get(ref) ?? []) {
      rank.set(target, Math.max(rank.get(target) ?? 0, (rank.get(ref) ?? 0) + 1))
      const nextIndegree = (indegree.get(target) ?? 0) - 1
      indegree.set(target, nextIndegree)
      if (nextIndegree === 0) queue.push(target)
    }
  }

  const groups = new Map<number, string[]>()
  refs.forEach((ref, index) => {
    const refRank = cursor === refs.length ? (rank.get(ref) ?? 0) : Math.floor(index / 4)
    const group = groups.get(refRank) ?? []
    group.push(ref)
    groups.set(refRank, group)
  })

  const horizontal = parsed.direction === 'LR' || parsed.direction === 'RL'
  const reverse = parsed.direction === 'RL' || parsed.direction === 'BT'
  const rankSpacing = 280
  const itemSpacing = 150
  const rankCount = groups.size || 1
  const positions = new Map<string, { x: number; y: number }>()

  Array.from(groups.entries()).sort(([a], [b]) => a - b).forEach(([groupRank, group]) => {
    const rankOffset = (groupRank - (rankCount - 1) / 2) * rankSpacing * (reverse ? -1 : 1)
    group.forEach((ref, index) => {
      const itemOffset = (index - (group.length - 1) / 2) * itemSpacing
      positions.set(ref, horizontal
        ? { x: center.x + rankOffset, y: center.y + itemOffset }
        : { x: center.x + itemOffset, y: center.y + rankOffset })
    })
  })

  return positions
}

export function layoutMermaidImport(parsed: ParsedImport, center: { x: number; y: number }) {
  const refs = parsed.elements.map((element) => element.ref).filter(Boolean)
  const metadataPositions = new Map<string, { x: number; y: number }>()
  parsed.elements.forEach((element) => {
    const position = mermaidImportMetadataPosition(element)
    if (position) metadataPositions.set(element.ref, position)
  })
  if (metadataPositions.size === 0) return layoutMermaidRefs(parsed, refs, center)

  const values = Array.from(metadataPositions.values())
  const left = Math.min(...values.map((position) => position.x))
  const right = Math.max(...values.map((position) => position.x))
  const top = Math.min(...values.map((position) => position.y))
  const bottom = Math.max(...values.map((position) => position.y))
  const metadataCenter = {
    x: left + (right - left) / 2,
    y: top + (bottom - top) / 2,
  }
  const positions = new Map<string, { x: number; y: number }>()
  metadataPositions.forEach((position, ref) => {
    positions.set(ref, {
      x: center.x + position.x - metadataCenter.x,
      y: center.y + position.y - metadataCenter.y,
    })
  })

  const missingRefs = refs.filter((ref) => !metadataPositions.has(ref))
  if (missingRefs.length === 0) return positions

  const horizontal = parsed.direction === 'LR' || parsed.direction === 'RL'
  const reverse = parsed.direction === 'RL' || parsed.direction === 'BT'
  const metadataWidth = right - left
  const metadataHeight = bottom - top
  const offset = horizontal
    ? Math.max(360, metadataWidth / 2 + 320)
    : Math.max(240, metadataHeight / 2 + 220)
  const fallbackCenter = horizontal
    ? { x: center.x + offset * (reverse ? -1 : 1), y: center.y }
    : { x: center.x, y: center.y + offset * (reverse ? -1 : 1) }
  layoutMermaidRefs(parsed, missingRefs, fallbackCenter).forEach((position, ref) => {
    positions.set(ref, position)
  })

  return positions
}

const defaultMermaidImportClient: MermaidImportClient = {
  getElement: (id) => api.elements.get(id),
  createElement: (data) => api.elements.create(data),
  deleteElement: (id) => api.elements.delete('', id),
  createView: (data) => api.workspace.views.create(data),
  deleteView: (viewId) => api.workspace.views.delete('', viewId),
  addPlacement: (viewId, elementId, x, y) => api.workspace.views.placements.add(viewId, elementId, x, y),
  removePlacement: (viewId, elementId) => api.workspace.views.placements.remove(viewId, elementId),
  createConnector: (viewId, data) => api.workspace.connectors.create(viewId, data),
  deleteConnector: (connectorId) => api.workspace.connectors.delete('', connectorId),
}

type MermaidImportRollbackAction = () => Promise<void>

async function rollbackMermaidImport(actions: MermaidImportRollbackAction[]) {
  const failures: unknown[] = []
  for (let index = actions.length - 1; index >= 0; index -= 1) {
    try {
      await actions[index]()
    } catch (error) {
      failures.push(error)
    }
  }
  return failures
}

async function resolveExistingMermaidElement(
  ref: string,
  existingElementsById: Map<number, WorkspaceElement>,
  client: MermaidImportClient,
) {
  const existingElementId = mermaidRefElementId(ref)
  if (existingElementId === null) return null

  const cached = existingElementsById.get(existingElementId)
  if (cached) return cached

  try {
    const fetched = await client.getElement(existingElementId)
    existingElementsById.set(fetched.id, fetched)
    return fetched
  } catch {
    return null
  }
}

export async function importMermaidIntoView({
  parsed,
  currentViewId,
  center,
  allElements,
  viewElements,
  connectors,
  onConnectorCreated,
  client = defaultMermaidImportClient,
}: ImportMermaidIntoViewOptions): Promise<MermaidLocalImportSummary> {
  const positions = layoutMermaidImport(parsed, center)
  const existingElementsById = new Map(allElements.map((element) => [element.id, element]))
  const elementsByRef = new Map<string, WorkspaceElement>()
  const rollbackActions: MermaidImportRollbackAction[] = []
  let resolvedElementCount = 0
  let createdElementCount = 0

  try {
    for (const element of parsed.elements) {
      const existingElement = await resolveExistingMermaidElement(element.ref, existingElementsById, client)
      if (existingElement) {
        elementsByRef.set(element.ref, existingElement)
        resolvedElementCount += 1
        continue
      }

      const created = await client.createElement({
        name: element.name,
        kind: element.kind ?? 'system',
        description: element.description ?? '',
        technology: element.technology ?? '',
        url: element.url ?? '',
        logo_url: element.logoUrl ?? undefined,
        technology_connectors: mermaidImportTechnologyConnectors(element),
        tags: element.tags ?? [],
        repo: element.repo ?? undefined,
        branch: element.branch ?? undefined,
        file_path: element.filePath ?? undefined,
        language: element.language ?? undefined,
        bypass_noise_gate: element.bypassNoiseGate ?? false,
      })
      rollbackActions.push(() => client.deleteElement(created.id))
      if (element.hasView) {
        const createdView = await client.createView({
          name: element.name || created.name,
          label: element.viewLabel ?? undefined,
          parent_view_id: created.id,
        })
        rollbackActions.push(() => client.deleteView(createdView.id))
      }
      elementsByRef.set(element.ref, created)
      createdElementCount += 1
    }

    const placedElementIds = new Set(viewElements.map((element) => element.element_id))
    for (const element of parsed.elements) {
      const resolved = elementsByRef.get(element.ref)
      if (!resolved || placedElementIds.has(resolved.id)) continue
      placedElementIds.add(resolved.id)
      const position = positions.get(element.ref) ?? center
      await client.addPlacement(currentViewId, resolved.id, position.x, position.y)
      rollbackActions.push(() => client.removePlacement(currentViewId, resolved.id))
    }

    const handles = mermaidConnectorHandles(parsed.direction)
    const existingConnectorKeys = new Set(connectors.map((connector) => mermaidExistingConnectorKey(connector, handles)))
    let resolvedConnectorCount = 0
    const materializedConnectors: Connector[] = []
    for (const connector of parsed.connectors) {
      const source = elementsByRef.get(connector.sourceElementRef)
      const target = elementsByRef.get(connector.targetElementRef)
      if (!source || !target) continue

      const connectorKey = mermaidParsedConnectorKey(connector, source.id, target.id, handles)
      if (existingConnectorKeys.has(connectorKey)) {
        resolvedConnectorCount += 1
        existingConnectorKeys.add(connectorKey)
        continue
      }

      const createdConnector = await client.createConnector(currentViewId, {
        source_element_id: source.id,
        target_element_id: target.id,
        label: connector.label ?? '',
        description: connector.description ?? '',
        relationship: connector.relationship ?? '',
        direction: connector.direction ?? 'forward',
        style: connector.style ?? 'bezier',
        url: connector.url ?? '',
        source_handle: connector.sourceHandle ?? handles.source_handle,
        target_handle: connector.targetHandle ?? handles.target_handle,
      })
      rollbackActions.push(() => client.deleteConnector(createdConnector.id))
      materializedConnectors.push(createdConnector)
      existingConnectorKeys.add(connectorKey)
    }
    const summary = {
      resolvedElementCount,
      createdElementCount,
      resolvedConnectorCount,
      createdConnectorCount: materializedConnectors.length,
      importedElementIds: new Set(Array.from(elementsByRef.values(), (element) => element.id)),
    }
    materializedConnectors.forEach((connector) => {
      try {
        onConnectorCreated?.(connector)
      } catch {
        /* Local notification failure should not roll back the persisted import. */
      }
    })
    return summary
  } catch (error) {
    const rollbackFailures = await rollbackMermaidImport(rollbackActions)
    if (rollbackFailures.length > 0) {
      throw new Error(`${error instanceof Error ? error.message : String(error)} Rollback also failed for ${countLabel(rollbackFailures.length, 'operation')}.`)
    }
    throw error
  }
}
