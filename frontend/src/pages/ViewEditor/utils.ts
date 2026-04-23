import type { Connector } from '../../types'
import type { Node as RFNode } from 'reactflow'

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

export function triggerDownload(dataUrl: string, filename: string) {
  const link = document.createElement('a')
  link.href = dataUrl
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}
