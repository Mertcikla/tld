import type { Connector } from '../../types'
import { saveBlobAs, saveDataUrlAs, type DialogFilter } from '../../lib/desktop'
import type { Node as RFNode } from 'reactflow'
import { DEFAULT_SOURCE_HANDLE_SIDE, DEFAULT_TARGET_HANDLE_SIDE, ensureVisualHandleId, getHandleFlowPosition } from '../../utils/edgeDistribution'

// ── Domain-model conversion ────────────────────────────────────────────────

/** Pass-through: the API already returns Connector in the canonical shape. */
export function connectorToConnector(connector: Connector): Connector {
  return connector
}

// ── Handle geometry ────────────────────────────────────────────────────────

export function findClosestHandles(
  sourceNode: RFNode,
  targetNode: RFNode,
): { sourceHandle: string; targetHandle: string } {
  const w1 = sourceNode.width ?? 180
  const h1 = sourceNode.height ?? 80
  const w2 = targetNode.width ?? 180
  const h2 = targetNode.height ?? 80
  const sp = sourceNode.position
  const tp = targetNode.position

  const srcHandles: [string, number, number][] = [
    ['top', sp.x + w1 / 2, sp.y],
    ['bottom', sp.x + w1 / 2, sp.y + h1],
    ['left', sp.x, sp.y + h1 / 2],
    ['right', sp.x + w1, sp.y + h1 / 2],
  ]
  const tgtHandles: [string, number, number][] = [
    ['top', tp.x + w2 / 2, tp.y],
    ['bottom', tp.x + w2 / 2, tp.y + h2],
    ['left', tp.x, tp.y + h2 / 2],
    ['right', tp.x + w2, tp.y + h2 / 2],
  ]

  let minDist = Infinity
  let bestSrc = 'right'
  let bestTgt = 'left'
  for (const [sid, sx, sy] of srcHandles) {
    for (const [tid, tx, ty] of tgtHandles) {
      const d = Math.hypot(tx - sx, ty - sy)
      if (d < minDist) { minDist = d; bestSrc = sid; bestTgt = tid }
    }
  }
  return { sourceHandle: bestSrc, targetHandle: bestTgt }
}

/** Shared by the ghost and confirmation, keeping the source slot the user chose. */
export function resolvePendingConnectorHandles(
  sourceNode: RFNode | undefined,
  targetNode: Pick<RFNode, 'position' | 'positionAbsolute' | 'width' | 'height'>,
  chosenSourceHandle: string | null,
): { sourceHandle: string; targetHandle: string } {
  const targetPosition = targetNode.positionAbsolute ?? targetNode.position
  const target = { ...targetNode, id: 'pending-target', data: {}, position: targetPosition, width: targetNode.width ?? 180, height: targetNode.height ?? 85 }
  const source = sourceNode ? {
    ...sourceNode, position: sourceNode.positionAbsolute ?? sourceNode.position,
    width: sourceNode.width ?? 180, height: sourceNode.height ?? 85,
  } : undefined
  const sourceHandle = ensureVisualHandleId(
    chosenSourceHandle ?? (source ? findClosestHandles(source, target).sourceHandle : null),
    DEFAULT_SOURCE_HANDLE_SIDE,
  )!
  if (!source) return { sourceHandle, targetHandle: ensureVisualHandleId(null, DEFAULT_TARGET_HANDLE_SIDE)! }
  const sourcePoint = getHandleFlowPosition(source.position.x, source.position.y, source.width, source.height, sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE)
  let targetHandle = 'left-2'
  let nearest = Infinity
  for (const side of ['top', 'bottom', 'left', 'right'] as const) {
    const point = getHandleFlowPosition(target.position.x, target.position.y, target.width, target.height, side, side)
    const distance = Math.hypot(point.x - sourcePoint.x, point.y - sourcePoint.y)
    if (distance < nearest) {
      nearest = distance
      targetHandle = `${side}-2`
    }
  }
  return { sourceHandle, targetHandle }
}

export function findClosestHandleToPoint(
  sourceNode: RFNode,
  tx: number,
  ty: number,
): { sourceHandle: string; targetHandle: string } {
  const w = sourceNode.width ?? 180
  const h = sourceNode.height ?? 80
  const sp = sourceNode.position
  const handles: [string, number, number][] = [
    ['top', sp.x + w / 2, sp.y],
    ['bottom', sp.x + w / 2, sp.y + h],
    ['left', sp.x, sp.y + h / 2],
    ['right', sp.x + w, sp.y + h / 2],
  ]
  let minDist = Infinity
  let bestSrc = 'right'
  for (const [sid, sx, sy] of handles) {
    const d = Math.hypot(tx - sx, ty - sy)
    if (d < minDist) { minDist = d; bestSrc = sid }
  }
  const opp: Record<string, string> = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' }
  return { sourceHandle: bestSrc, targetHandle: opp[bestSrc] ?? 'left' }
}

// ── Export helpers ─────────────────────────────────────────────────────────

export function sanitizeExportFilename(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return 'view-export'
  return trimmed.replace(/[\\/:*?"<>|]+/g, '-').replace(/\s+/g, ' ')
}

export const exportFilters: Record<string, DialogFilter[]> = {
  mermaid: [{ displayName: 'Markdown Files (*.md)', pattern: '*.md' }],
  svg: [{ displayName: 'SVG Files (*.svg)', pattern: '*.svg' }],
  png: [{ displayName: 'PNG Files (*.png)', pattern: '*.png' }],
  markdown: [{ displayName: 'Markdown Files (*.md)', pattern: '*.md' }],
}

export function triggerDownload(dataUrl: string, filename: string, format?: string) {
  return saveDataUrlAs(dataUrl, filename, format ? exportFilters[format] : undefined)
}

export function triggerBlobDownload(blob: Blob, filename: string, format?: string) {
  return saveBlobAs(blob, filename, format ? exportFilters[format] : undefined)
}
