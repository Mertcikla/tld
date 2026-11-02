import { readSourcePreviewAppearance, type SourcePreviewAppearance } from '../../utils/sourcePreviewAppearance'
import { expandCodeTabs, expandCodeTokens, subCodeFont } from '../../utils/subElementText'
import type { LayoutNode } from './types'
import { isSubElement, isSubHandle, SUB_HEADER, SUB_ELLIPSIS, SUB_CONTENT_TOP, subDetail } from '../../utils/subElements'
import { buildSymbolPresentation, symbolStems, type SymbolPresentation } from '../../utils/subElementSymbols'
import { peekSubSource, requestSubSource, type SubSource } from '../../utils/subElementSource'
import { elementTechnologyText } from '../../utils/elementIcon'

const scrollRows = new Map<string, number>()
const scrollListeners = new Set<() => void>()
export const subscribeSubScroll = (fn: () => void) => { scrollListeners.add(fn); return () => { scrollListeners.delete(fn) } }
const scrollKey = (node: LayoutNode, zoom: number) => `${node.id}:${node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom)}`
const firstRow = (node: LayoutNode, zoom: number) => scrollRows.get(scrollKey(node, zoom)) ?? 0
const presentations = new WeakMap<LayoutNode, { detail: string; first: number; symbols?: unknown; lines?: unknown; tokens?: unknown; status?: string; source?: SubSource; model: SymbolPresentation; appearance: SourcePreviewAppearance }>()

export function subNodeSymbols(node: LayoutNode, zoom: number): SymbolPresentation {
  const source = node.sourceLink ? peekSubSource(node.sourceLink) : undefined
  const detail = node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom), first = firstRow(node, zoom)
  const appearance = readSourcePreviewAppearance(node.diagramId, node.elementId)
  const lineNumbers = appearance.lineNumbers ?? detail === 'code'
  const cached = presentations.get(node)
  if (cached && cached.appearance === appearance && cached.detail === detail && cached.first === first && cached.source === source && cached.symbols === source?.symbols && cached.lines === source?.lines && cached.tokens === source?.tokens && cached.status === source?.status) return cached.model
  const connected = node.subConnectedHandles ?? node.edgesOut.flatMap(edge => edge.sourceHandle ? [edge.sourceHandle] : [])
  const base = buildSymbolPresentation({ kind: node.subKind ?? node.type, source, path: node.sourceLink?.file_path ?? '', detail, width: node.worldW, height: node.worldH, connected, fontSize: appearance.fontSize, lineNumbers })
  const model = first ? buildSymbolPresentation({ kind: node.subKind ?? node.type, source, path: node.sourceLink?.file_path ?? '', detail, width: node.worldW, height: node.worldH, connected, scrollTop: first * base.rowHeight, fontSize: appearance.fontSize, lineNumbers }) : base
  presentations.set(node, { appearance, detail, first, source, symbols: source?.symbols, lines: source?.lines, tokens: source?.tokens, status: source?.status, model })
  return model
}

export function scrollSubElement(node: LayoutNode, delta: number, zoom: number): boolean {
  if ((node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom)) === 'overview') return false
  const model = subNodeSymbols(node, zoom)
  const max = Math.max(0, model.rowCount - Math.floor((node.worldH - SUB_HEADER) / model.rowHeight))
  if (max === 0) return false
  const next = Math.max(0, Math.min(max, firstRow(node, zoom) + Math.sign(delta) * Math.max(1, Math.round(Math.abs(delta) / model.rowHeight))))
  scrollRows.set(scrollKey(node, zoom), next)
  if (scrollRows.size > 128) scrollRows.delete(scrollRows.keys().next().value!)
  scrollListeners.forEach(fn => fn())
  return true
}

export function subNodeHandleAnchor(node: LayoutNode, handle: string | null, zoom: number): { x: number; y: number; side: 'left' | 'right' } | null {
  if (!isSubElement(node.subKind ?? node.type) || !isSubHandle(handle) || (node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom)) === 'overview') return null
  let endpoint = subNodeSymbols(node, zoom).endpoints.get(handle!)
  if (!endpoint) {
    const source = node.sourceLink ? peekSubSource(node.sourceLink) : undefined
    endpoint = buildSymbolPresentation({ kind: node.subKind ?? node.type, source, path: node.sourceLink?.file_path ?? '', detail: node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom), width: node.worldW, height: node.worldH,
      ...readSourcePreviewAppearance(node.diagramId, node.elementId), connected: [handle!], scrollTop: firstRow(node, zoom) * subNodeSymbols(node, zoom).rowHeight }).endpoints.get(handle!)
  }
  return endpoint ? { x: endpoint.side === 'left' ? 0 : node.worldW, y: endpoint.y, side: endpoint.side } : null
}
export function subNodeHandleY(node: LayoutNode, handle: string | null, zoom: number) { return subNodeHandleAnchor(node, handle, zoom)?.y ?? null }

export function subSymbolAtPoint(node: LayoutNode, zoom: number, x: number, y: number): string | undefined {
  if (!isSubElement(node.subKind ?? node.type) || (node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom)) === 'overview') return
  const model = subNodeSymbols(node, zoom)
  if (y < SUB_HEADER) return
  if (y < model.viewportTop) return model.above.representative?.area?.key
  if (y > model.viewportBottom) return model.below.representative?.area?.key
  const offset = firstRow(node, zoom) * model.rowHeight
  return [...model.areas].reverse().find(area => x >= area.x && x <= area.x + area.width && y >= SUB_HEADER + area.contentTop - offset && y <= SUB_HEADER + area.contentTop + area.height - offset)?.key
}

function roundedStep(ctx: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number) {
  if (y1 === y2) { ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke(); return }
  const mid = (x1 + x2) / 2, sx = Math.sign(x2 - x1), sy = Math.sign(y2 - y1)
  const r = Math.min(8, Math.abs(y2 - y1) / 2, Math.abs(x2 - x1) / 2)
  ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(mid - sx * r, y1)
  ctx.quadraticCurveTo(mid, y1, mid, y1 + sy * r); ctx.lineTo(mid, y2 - sy * r)
  ctx.quadraticCurveTo(mid, y2, mid + sx * r, y2); ctx.lineTo(x2, y2); ctx.stroke()
}

/** Cached text/tokens and visible symbol regions only; animation redraws while active. */
export function drawSubElement(ctx: CanvasRenderingContext2D, node: LayoutNode, zoom: number, alpha: number, accent: string, lowDetail: boolean, activeHandle?: string, _groupColor?: string, icon?: HTMLImageElement | null, headerBg = '#2d3748', symbolAccent = accent) {
  const w = node.worldW, h = node.worldH, detail = node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom)
  const source = (node.subKind ?? node.type) === 'code' && node.sourceLink
    ? peekSubSource(node.sourceLink) ?? (!lowDetail && detail !== 'overview' && w * zoom >= 100 ? requestSubSource(node.sourceLink) : undefined) : undefined
  const model = subNodeSymbols(node, zoom)
  const pulse = activeHandle ? 0.75 + Math.sin(performance.now() / 220) * 0.2 : 0.5
  const active = (left: string, right: string) => activeHandle === left || activeHandle === right || (!!activeHandle && model.endpoints.get(activeHandle)?.area?.key === right)
  ctx.save(); ctx.globalAlpha = alpha
  ctx.fillStyle = '#20252d'; ctx.strokeStyle = accent; ctx.lineWidth = 1 / zoom
  ctx.beginPath(); ctx.roundRect(0, 0, w, h, 10); ctx.fill(); ctx.stroke(); ctx.clip()
  ctx.fillStyle = headerBg; ctx.fillRect(0.5, 0.5, w - 1, SUB_HEADER - 0.5)
  if (w * zoom < 50) { ctx.restore(); return }
  if (icon) {
    const scale = 24 / Math.max(icon.naturalWidth, icon.naturalHeight)
    const iconWidth = icon.naturalWidth * scale, iconHeight = icon.naturalHeight * scale
    ctx.drawImage(icon, (w - iconWidth) / 2, 6 + (24 - iconHeight) / 2, iconWidth, iconHeight)
  }
  const technology = node.technologyText === undefined ? elementTechnologyText(node.technology, [], !!node.logoUrl) : node.technologyText
  const metadata = source?.status !== 'ready' ? source?.error || (source ? 'Loading source…' : 'Zoom in to load source') : technology
  const textWidth = w - 96, nameY = (node.logoUrl ? 45 : 32) + (metadata ? 0 : 6)
  ctx.textBaseline = 'middle'; ctx.textAlign = 'center'; ctx.font = '600 18px system-ui'; ctx.fillStyle = '#f7fafc'
  ctx.fillText(node.label, w / 2, nameY, textWidth)
  ctx.font = '9px system-ui'; ctx.fillStyle = '#0bc5ea'
  ctx.fillText(node.type.toUpperCase(), w / 2, nameY + 15, textWidth)
  ctx.font = '10px system-ui'; ctx.fillStyle = '#718096'
  if (metadata) ctx.fillText(`[${metadata}]`, w / 2, nameY + 27, textWidth)
  if (detail === 'overview' || lowDetail) { ctx.restore(); return }
  const first = Math.min(firstRow(node, zoom), Math.max(0, model.rowCount - 1))
  const offset = first * model.rowHeight
  const rows = Math.min(model.rowCount - first, Math.ceil((h - SUB_HEADER) / model.rowHeight))
  ctx.save(); ctx.textAlign = 'left'; ctx.beginPath(); ctx.rect(0, model.viewportTop, w, model.viewportBottom - model.viewportTop); ctx.clip()
  for (const area of model.areas) {
    const y = SUB_HEADER + area.contentTop - offset
    if (y + area.height < model.viewportTop || y > model.viewportBottom) continue
    const isActive = active(area.leftId, area.rightId), color = symbolAccent
    if (!isActive && !area.connected) continue
    ctx.fillStyle = color; ctx.strokeStyle = color; ctx.lineWidth = 1 / zoom
    ctx.beginPath(); ctx.roundRect(area.x, y, area.width, area.height, Math.min(8, area.height / 2, model.rowHeight / 2 - model.highlightInset))
    ctx.globalAlpha = alpha * (isActive ? 0.18 : 0.10); ctx.fill()
    ctx.globalAlpha = alpha * (isActive ? pulse : 0.52); ctx.stroke()
  }
  ctx.globalAlpha = alpha
  for (let row = 0; row < rows; row++) {
    const index = first + row, cy = SUB_CONTENT_TOP + model.contentInset + (row + 0.5) * model.rowHeight
    ctx.fillStyle = '#e1e6ed'; ctx.font = subCodeFont(model.fontSize)
    if (model.lineNumbers) { ctx.fillStyle = '#7f8b9c'; ctx.fillText(String(detail === 'outline' ? model.areas[index]?.symbol.startLine ?? '' : (source?.startLine ?? 1) + index), 12, cy) }
    if (detail === 'outline') {
      const area = model.areas[index]
      if (area?.signature) {
        let x = model.textX
        for (const token of area.signature.tokens) {
          ctx.fillStyle = token.color; ctx.fillText(token.text, x, cy)
          x += ctx.measureText(token.text).width
          if (x > w) break
        }
      }
    } else {
      let tx = model.textX
      if (source?.tokens[index]?.length) for (const token of expandCodeTokens(source.tokens[index])) {
        ctx.fillStyle = token.color; ctx.fillText(token.text, tx, cy)
        tx += ctx.measureText(token.text).width
        if (tx > w) break
      } else { ctx.fillStyle = '#abb2bf'; ctx.fillText(expandCodeTabs(source?.lines[index] ?? ''), tx, cy) }
    }
  }
  ctx.restore()
  for (const { endpoint, area, sourceX, sourceY, targetX, targetY } of symbolStems(model, w, h, offset)) {
    if (!endpoint.connected && !active(area.leftId, area.rightId)) continue
    ctx.strokeStyle = symbolAccent; ctx.lineWidth = 1.4 / zoom; ctx.globalAlpha = alpha * (active(area.leftId, area.rightId) ? pulse : 0.55)
    if (active(area.leftId, area.rightId)) { ctx.setLineDash([6 / zoom, 4 / zoom]); ctx.lineDashOffset = -performance.now() / 50 / zoom }
    roundedStep(ctx, sourceX, sourceY, targetX, targetY)
    ctx.setLineDash([])
  }
  ctx.globalAlpha = alpha
  for (const [direction, overflow] of [['above', model.above], ['below', model.below]] as const) {
    const y = direction === 'above' ? SUB_HEADER : h - SUB_ELLIPSIS
    const count = overflow.areas.length
    const label = [count ? `${count} ${count === 1 ? 'symbol' : 'symbols'}` : '', overflow.connectionCount ? `${overflow.connectionCount} connections` : ''].filter(Boolean).join(' · ')
    if (!label) continue
    ctx.fillStyle = '#20252d'; ctx.fillRect(0, y, w, SUB_ELLIPSIS)
    ctx.strokeStyle = '#455263'; ctx.lineWidth = 1 / zoom; ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke()
    ctx.font = '9px ui-monospace, monospace'; ctx.textAlign = 'center'; ctx.fillStyle = symbolAccent
    ctx.fillText(`⋯ ${label} ⋯`, w / 2, overflow.y, w - 50)
  }
  for (const endpoint of model.endpoints.values()) if (endpoint.visible && (endpoint.connected || (endpoint.area && active(endpoint.area.leftId, endpoint.area.rightId)))) {
    ctx.fillStyle = symbolAccent; ctx.beginPath(); ctx.arc(endpoint.side === 'left' ? 0 : w, endpoint.y, 4, 0, Math.PI * 2); ctx.fill()
  }
  ctx.restore()
}
