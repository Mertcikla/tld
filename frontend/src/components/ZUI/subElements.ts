import { sourceDetailZoom } from './sourceDetailScale'
import { CHANGE_RING_COLORS } from './changeOverlay'
import { readSourcePreviewAppearance, type SourcePreviewAppearance } from '../../utils/sourcePreviewAppearance'
import { expandCodeTabs, expandCodeTokens, subCodeFont } from '../../utils/subElementText'
import type { LayoutNode } from './types'
import { isSymbolDetail, isSubElement, isSubHandle, SUB_HEADER, SUB_ELLIPSIS, subDetail } from '../../utils/subElements'
import { buildSymbolPresentation, symbolStems, symbolPresentationRowHeight, type SymbolPresentation } from '../../utils/subElementSymbols'
import { peekSubSource, requestSubSource, type SubSource } from '../../utils/subElementSource'
import { fitElementName, ELEMENT_NAME_FONT_FAMILY } from '../../utils/elementName'
import { elementTechnologyText } from '../../utils/elementIcon'

// Reference detail scale: level 1 = zoom 0.1, level 100 = the default
// 180×85 leaf cap (281 / 85). Keep this fixed while tuning individual stages.
const COLLAPSED_SYMBOLS_START_LEVEL = 40
const collapsedSymbolsStartZoom = sourceDetailZoom(COLLAPSED_SYMBOLS_START_LEVEL)

export const nodeDetail = (node: LayoutNode, zoom: number) => {
  if (!node.subDetail && node.worldW * zoom < 180 * collapsedSymbolsStartZoom && node.worldH * zoom < 85 * collapsedSymbolsStartZoom) return 'overview'
  const stage = node.subDetail ?? subDetail(node.worldW * zoom, node.worldH * zoom)
  // Compact cards already show a title; retain it when source rows first appear.
  const detail = node.worldW < 280 && stage === 'structure' ? 'summary' : stage
  return node.subSource && !node.subSource.lines.length && detail === 'code' ? 'outline' : detail
}

const scrollRows = new Map<string, number>()
const scrollColumns = new Map<string, number>()
const scrollListeners = new Set<() => void>()
export const subscribeSubScroll = (fn: () => void) => { scrollListeners.add(fn); return () => { scrollListeners.delete(fn) } }
const scrollKey = (node: LayoutNode, zoom: number) => {
  const detail = nodeDetail(node, zoom)
  // Structure reveals summary text in place, including after scrolling.
  const layout = detail === 'structure' ? 'summary' : detail
  return `${node.id}:${node.sourceLink?.comparisonKey ?? ''}:${layout}:${node.subConnectedOnly ?? readSourcePreviewAppearance(node.diagramId, node.elementId).connectedOnly ?? false}:${node.subConnectedHandles?.join('|') ?? ''}`
}
const firstRow = (node: LayoutNode, zoom: number) => {
  const saved = scrollRows.get(scrollKey(node, zoom))
  if (saved !== undefined) return saved
  const source = node.subSource ?? (node.sourceLink ? peekSubSource(node.sourceLink) : undefined)
  if (nodeDetail(node, zoom) !== 'code' || !source?.diff) return 0
  return Math.max(0, source.diff.lines.findIndex(line => line.change === 'added' || line.change === 'removed') - 3)
}
const presentations = new WeakMap<LayoutNode, { height: number; headerHeight?: number; detail: string; first: number; fontSize: number; symbols?: unknown; lines?: unknown; tokens?: unknown; status?: string; source?: SubSource; model: SymbolPresentation; appearance: SourcePreviewAppearance }>()

export function subNodeSymbols(node: LayoutNode, zoom: number): SymbolPresentation {
  const source = node.subSource ?? (node.sourceLink ? peekSubSource(node.sourceLink) : undefined)
  const detail = nodeDetail(node, zoom), first = firstRow(node, zoom)
  const appearance = readSourcePreviewAppearance(node.diagramId, node.elementId)
  const fontSize = Math.min(Math.max(appearance.fontSize ?? 12, detail === 'summary' || detail === 'structure' ? 9 / zoom : 0), 14 / zoom)
  const lineNumbers = appearance.lineNumbers ?? detail === 'code'
  const cached = presentations.get(node)
  if (cached && cached.height === node.worldH && cached.headerHeight === node.subHeaderHeight && cached.appearance === appearance && cached.detail === detail && cached.fontSize === fontSize && cached.first === first && cached.source === source && cached.symbols === source?.symbols && cached.lines === source?.lines && cached.tokens === source?.tokens && cached.status === source?.status) return cached.model
  const connected = node.subConnectedHandles ?? node.edgesOut.flatMap(edge => edge.sourceHandle ? [edge.sourceHandle] : [])
  const model = buildSymbolPresentation({ kind: node.subKind ?? node.type, source, path: node.sourceLink?.file_path ?? '', detail, width: node.worldW, height: node.worldH, headerHeight: node.subHeaderHeight, connected, scrollTop: first * symbolPresentationRowHeight(detail, fontSize, true), fontSize, lineNumbers, fractionalMetrics: true, connectedOnly: node.subConnectedOnly ?? appearance.connectedOnly })

  presentations.set(node, { height: node.worldH, headerHeight: node.subHeaderHeight, appearance, detail, first, fontSize, source, symbols: source?.symbols, lines: source?.lines, tokens: source?.tokens, status: source?.status, model })
  return model
}

function scrollSubElementNative(node: LayoutNode, delta: number, zoom: number): boolean {
  if ((nodeDetail(node, zoom)) === 'overview') return false
  const model = subNodeSymbols(node, zoom)
  const max = Math.max(0, model.rowCount - Math.floor((node.worldH - model.headerHeight) / model.rowHeight))
  if (max === 0) return false
  const next = Math.max(0, Math.min(max, firstRow(node, zoom) + Math.sign(delta) * Math.max(1, Math.round(Math.abs(delta) / model.rowHeight))))
  if (next === firstRow(node, zoom)) return true
  scrollRows.set(scrollKey(node, zoom), next)
  if (scrollRows.size > 128) scrollRows.delete(scrollRows.keys().next().value!)
  scrollListeners.forEach(fn => fn())
  return true
}

function subNodeHandleAnchorNative(node: LayoutNode, handle: string | null, zoom: number): { x: number; y: number; side: 'left' | 'right' } | null {
  if (!isSubElement(node.subKind ?? node.type) || !isSubHandle(handle) || (nodeDetail(node, zoom)) === 'overview') return null
  const model = subNodeSymbols(node, zoom)
  let endpoint = model.endpoints.get(handle!)
  if (!endpoint) {
    const source = node.subSource ?? (node.sourceLink ? peekSubSource(node.sourceLink) : undefined)
    endpoint = buildSymbolPresentation({ kind: node.subKind ?? node.type, source, path: node.sourceLink?.file_path ?? '', detail: nodeDetail(node, zoom), width: node.worldW, height: node.worldH, headerHeight: node.subHeaderHeight,
      ...readSourcePreviewAppearance(node.diagramId, node.elementId), connectedOnly: node.subConnectedOnly ?? readSourcePreviewAppearance(node.diagramId, node.elementId).connectedOnly,
      fontSize: model.fontSize, lineNumbers: model.lineNumbers, fractionalMetrics: true, connected: [handle!], scrollTop: firstRow(node, zoom) * model.rowHeight }).endpoints.get(handle!)
  }
  return endpoint ? { x: endpoint.side === 'left' ? 0 : node.worldW, y: endpoint.y, side: endpoint.side } : null
}
export function subNodeHandleY(node: LayoutNode, handle: string | null, zoom: number) { return subNodeHandleAnchor(node, handle, zoom)?.y ?? null }

function subSymbolAtPointNative(node: LayoutNode, zoom: number, x: number, y: number): string | undefined {
  if (!isSubElement(node.subKind ?? node.type) || (nodeDetail(node, zoom)) === 'overview') return
  const model = subNodeSymbols(node, zoom)
  if (y < model.headerHeight) return
  if (y < model.viewportTop) return model.above.representative?.area?.key
  if (y > model.viewportBottom) return model.below.representative?.area?.key
  const offset = firstRow(node, zoom) * model.rowHeight
  x += scrollColumns.get(scrollKey(node, zoom)) ?? 0
  return [...model.areas].reverse().find(area => x >= area.x && x <= area.x + area.width && y >= model.headerHeight + area.contentTop - offset && y <= model.headerHeight + area.contentTop + area.height - offset)?.key
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
function drawSubElementNative(ctx: CanvasRenderingContext2D, node: LayoutNode, zoom: number, alpha: number, accent: string, lowDetail: boolean, activeHandle?: string, _groupColor?: string, icon?: HTMLImageElement | null, headerBg = '#2d3748', symbolAccent = accent, bodyBg = '#232732', headerScale = 1) {
  const w = node.worldW, h = node.worldH, detail = nodeDetail(node, zoom)
  const source = node.subSource ?? ((node.subKind ?? node.type) === 'code' && node.sourceLink
    ? peekSubSource(node.sourceLink) ?? (!lowDetail && detail !== 'overview' && w * zoom >= 100 ? requestSubSource(node.sourceLink) : undefined) : undefined)
  const model = subNodeSymbols(node, zoom)
  const diffMode = detail === 'code' && !!source?.diff
  const pulse = activeHandle ? 0.75 + Math.sin(performance.now() / 220) * 0.2 : 0.5
  const active = (left: string, right: string) => activeHandle === left || activeHandle === right || (!!activeHandle && model.endpoints.get(activeHandle)?.area?.key === right)
  ctx.save(); ctx.globalAlpha = alpha
  const drawBorder = () => {
    ctx.strokeStyle = accent; ctx.lineWidth = 1.5 / zoom
    ctx.beginPath(); ctx.roundRect(0, 0, w, h, 10); ctx.stroke()
  }
  ctx.fillStyle = bodyBg
  ctx.beginPath(); ctx.roundRect(0, 0, w, h, 10); ctx.fill(); ctx.save(); ctx.clip()
  ctx.fillStyle = headerBg; ctx.fillRect(0.5, 0.5, w - 1, model.headerHeight - 0.5)
  if (w * zoom < 50) { ctx.restore(); drawBorder(); ctx.restore(); return }
  // Match the editor header's centered icon and stacked name/kind/metadata.
  const unit = Math.min(headerScale, 20 / (18 * zoom))
  const headerTop = (model.headerHeight - SUB_HEADER * unit) / 2
  const textWidth = Math.max(1, w - 96 * unit)
  if (icon) {
    const scale = 28 * unit / Math.max(icon.naturalWidth, icon.naturalHeight)
    const iconWidth = icon.naturalWidth * scale, iconHeight = icon.naturalHeight * scale
    ctx.drawImage(icon, (w - iconWidth) / 2, detail === 'structure' ? (model.headerHeight - iconHeight) / 2 : headerTop + 4 * unit + (28 * unit - iconHeight) / 2, iconWidth, iconHeight)
  }
  if (detail !== 'structure') {
    const technology = node.technologyText === undefined ? elementTechnologyText(node.technology, [], !!node.logoUrl) : node.technologyText
    const metadata = source?.status !== 'ready' ? source?.error || (source ? 'Loading source…' : 'Zoom in to load source') : technology
    const nameHeight = 18 * 1.15 * unit, kindHeight = 9 * 1.1 * unit, metadataHeight = metadata ? 10 * 1.1 * unit : 0
    const stackHeight = nameHeight + kindHeight + metadataHeight
    const stackCenter = headerTop + (icon ? 55.5 : 41.5) * unit
    const nameY = stackCenter - stackHeight / 2 + nameHeight / 2
    ctx.textBaseline = 'middle'; ctx.textAlign = 'center'; ctx.fillStyle = '#f7fafc'
    const name = fitElementName({ name: node.label, maxWidth: textWidth, maxLines: 1, fontSizes: [18 * unit], measureKey: 'zui-code',
      measure: (text, size) => { ctx.font = `600 ${size}px ${ELEMENT_NAME_FONT_FAMILY}`; return ctx.measureText(text).width } })
    ctx.font = `600 ${name.fontSize}px ${ELEMENT_NAME_FONT_FAMILY}`
    ctx.fillText(name.lines[0], w / 2, nameY)
    ctx.font = `${9 * unit}px ${ELEMENT_NAME_FONT_FAMILY}`
    ctx.fillStyle = node.type === 'code' ? '#0bc5ea' : node.type === 'class' ? '#38b2ac' : '#a0aec0'
    ctx.fillText(diffMode ? 'GIT DIFF · − BASE / + HEAD' : node.type.toUpperCase(), w / 2, nameY + (nameHeight + kindHeight) / 2, textWidth)
    ctx.font = `${10 * unit}px ${ELEMENT_NAME_FONT_FAMILY}`; ctx.fillStyle = '#718096'
    if (metadata) ctx.fillText(`[${metadata}]`, w / 2, stackCenter + stackHeight / 2 - metadataHeight / 2, textWidth)
  }
  if (detail === 'overview' || lowDetail) { ctx.restore(); drawBorder(); ctx.restore(); return }
  const position = Math.min(firstRow(node, zoom), Math.max(0, model.rowCount - 1))
  const first = Math.floor(position)
  const offset = position * model.rowHeight
  const rows = Math.min(model.rowCount - first, Math.ceil((h - model.headerHeight) / model.rowHeight) + 1)
  const scrollLeft = scrollColumns.get(scrollKey(node, zoom)) ?? 0
  ctx.save(); ctx.textAlign = 'left'; ctx.beginPath(); ctx.rect(0, model.viewportTop, w, model.viewportBottom - model.viewportTop); ctx.clip()
  if (scrollLeft) ctx.translate(-scrollLeft, 0)
  for (const area of model.areas) {
    const y = model.headerHeight + area.contentTop - offset
    if (y + area.height < model.viewportTop || y > model.viewportBottom) continue
    const change = node.changeOverlay?.symbolDetails?.find(change =>
      change.change !== 'unchanged' && change.name === area.symbol.name &&
      (!change.anchor || change.anchor.startLine + 1 === area.symbol.startLine))
    const isActive = active(area.leftId, area.rightId), color = change ? CHANGE_RING_COLORS[change.change] : symbolAccent
    if (!change && !isActive && !area.connected && detail !== 'structure') continue
    ctx.fillStyle = color; ctx.strokeStyle = color; ctx.lineWidth = 1 / zoom
    ctx.beginPath(); ctx.roundRect(area.x, y, area.width, area.height, Math.min(8, area.height / 2, model.rowHeight / 2 - model.highlightInset))
    if (!diffMode) { ctx.globalAlpha = alpha * (isActive ? 0.18 : 0.10); ctx.fill() }
    ctx.globalAlpha = alpha * (isActive ? pulse : diffMode ? 0.3 : 0.52); ctx.stroke()
  }
  ctx.globalAlpha = alpha
  for (let row = 0; detail !== 'structure' && row < rows; row++) {
    const index = first + row, displayRow = model.rows[index], sourceIndex = displayRow.sourceIndex, cy = model.headerHeight + model.contentInset + (row + first - position + 0.5) * model.rowHeight
    ctx.fillStyle = '#e1e6ed'; ctx.font = subCodeFont(model.fontSize)
    if (displayRow.diff) {
      const diff = displayRow.diff
      const added = diff.change === 'added', removed = diff.change === 'removed'
      if (added || removed) {
        ctx.strokeStyle = added ? 'rgba(46,160,67,0.30)' : 'rgba(248,81,73,0.30)'
        ctx.lineWidth = 1 / zoom
        const inset = ctx.lineWidth / 2
        ctx.strokeRect(inset, cy - model.rowHeight / 2 + inset, w - ctx.lineWidth, model.rowHeight - ctx.lineWidth)
      }
      ctx.fillStyle = added ? '#7ee787' : removed ? '#ffa198' : '#7f8b9c'
      ctx.textAlign = 'right'
      if (diff.oldLine) ctx.fillText(String(diff.oldLine), (model.textX - 28) / 2, cy)
      if (diff.newLine) ctx.fillText(String(diff.newLine), model.textX - 28, cy)
      ctx.textAlign = 'left'
      ctx.fillText(added ? '+' : removed ? '−' : '', model.textX - 16, cy)
      const tokens = removed ? source?.diff?.baseTokens[diff.oldLine - 1] : source?.diff?.headTokens[diff.newLine - 1]
      let x = model.textX
      for (const token of tokens?.length ? expandCodeTokens(tokens) : [{ text: expandCodeTabs(diff.text), color: added ? '#7ee787' : removed ? '#ffa198' : '#e1e6ed' }]) {
        ctx.fillStyle = token.color; ctx.fillText(token.text, x, cy); x += ctx.measureText(token.text).width
      }
      continue
    }
    if (displayRow.collapsed) { ctx.fillStyle = '#7f8b9c'; ctx.fillText(`⋯ ${displayRow.collapsed} ${isSymbolDetail(detail) ? 'symbols' : 'lines'} collapsed`, model.textX, cy); continue }
    const rowArea = isSymbolDetail(detail) ? model.areas.find(area => area.start === index) : undefined
    if (model.lineNumbers) { ctx.fillStyle = '#7f8b9c'; ctx.fillText(String(isSymbolDetail(detail) ? rowArea?.symbol.startLine ?? '' : (source?.startLine ?? 1) + sourceIndex), 12, cy) }
    if (isSymbolDetail(detail)) {
      const area = rowArea
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
      if (source?.tokens[sourceIndex]?.length) for (const token of expandCodeTokens(source.tokens[sourceIndex])) {
        ctx.fillStyle = token.color; ctx.fillText(token.text, tx, cy)
        tx += ctx.measureText(token.text).width
        if (tx > w) break
      } else { ctx.fillStyle = '#abb2bf'; ctx.fillText(expandCodeTabs(source?.lines[sourceIndex] ?? ''), tx, cy) }
    }
  }
  ctx.restore()
  for (const { endpoint, area, sourceX, sourceY, targetX, targetY } of symbolStems(model, w, h, offset, scrollLeft)) {
    if (!endpoint.connected && !active(area.leftId, area.rightId)) continue
    ctx.strokeStyle = symbolAccent; ctx.lineWidth = 1.4 / zoom; ctx.globalAlpha = alpha * (diffMode ? 0.3 : active(area.leftId, area.rightId) ? pulse : 0.55)
    if (active(area.leftId, area.rightId)) { ctx.setLineDash([6 / zoom, 4 / zoom]); ctx.lineDashOffset = -performance.now() / 50 / zoom }
    roundedStep(ctx, sourceX, sourceY, targetX, targetY)
    ctx.setLineDash([])
  }
  ctx.globalAlpha = alpha
  for (const [direction, overflow] of [['above', model.above], ['below', model.below]] as const) {
    const y = direction === 'above' ? model.headerHeight : h - SUB_ELLIPSIS
    const count = diffMode ? direction === 'above' ? first : Math.max(0, model.rowCount - first - rows) : overflow.areas.length
    const unit = diffMode ? count === 1 ? 'line' : 'lines' : count === 1 ? 'symbol' : 'symbols'
    const symbolLabel = count ? `${count} ${unit} ${direction}` : ''
    const connectionLabel = !diffMode && overflow.connectionCount ? `${overflow.connectionCount} ${overflow.connectionCount === 1 ? 'connection' : 'connections'}` : ''
    if (!symbolLabel && !connectionLabel) continue
    ctx.fillStyle = bodyBg; ctx.fillRect(0, y, w, SUB_ELLIPSIS)
    ctx.strokeStyle = 'rgba(255,255,255,0.08)'; ctx.lineWidth = 1 / zoom; ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke()
    if (detail === 'structure') continue
    ctx.font = `${Math.min(9, 11 / zoom)}px system-ui`; ctx.textAlign = 'left'
    const separator = symbolLabel && connectionLabel ? ' · ' : ''
    const arrowWidth = count ? 14 : 0
    const labelWidth = ctx.measureText(symbolLabel + separator + connectionLabel).width + arrowWidth
    let x = Math.max(24, (w - labelWidth) / 2)
    ctx.fillStyle = '#a0adbd'; ctx.strokeStyle = '#a0adbd'
    if (count) {
      const sign = direction === 'above' ? -1 : 1
      ctx.beginPath(); ctx.moveTo(x, overflow.y - sign * 2); ctx.lineTo(x + 3, overflow.y + sign); ctx.lineTo(x + 6, overflow.y - sign * 2); ctx.stroke()
      x += arrowWidth
    }
    ctx.fillText(symbolLabel + separator, x, overflow.y)
    x += ctx.measureText(symbolLabel + separator).width
    ctx.fillStyle = symbolAccent; ctx.fillText(connectionLabel, x, overflow.y, Math.max(0, w - x - 24))
  }
  // Ports straddle the border; only the card content should be clipped.
  ctx.restore()
  ctx.globalAlpha = alpha
  // Paint the complete outline above header/content fills, matching regular Explore cards.
  drawBorder()
  ctx.globalAlpha = alpha
  for (const endpoint of model.endpoints.values()) if (endpoint.visible && (endpoint.connected || (endpoint.area && active(endpoint.area.leftId, endpoint.area.rightId)))) {
    ctx.fillStyle = symbolAccent; ctx.beginPath(); ctx.arc(endpoint.side === 'left' ? 0 : w, endpoint.y, 4, 0, Math.PI * 2); ctx.fill()
  }
  ctx.restore()
}

// Source metrics use editor-sized coordinates even when Explore reserves only a
// regular card. Share this transform across drawing, ports, scrolling and picking.
const sourceCards = new WeakMap<LayoutNode, LayoutNode>()
export const sourceCardScale = (node: LayoutNode) => Math.min(1, node.worldW / 520)
function sourceCard(node: LayoutNode, zoom: number) {
  const scale = sourceCardScale(node)
  if (scale === 1) return node
  let card = sourceCards.get(node)
  if (!card) { card = { ...node }; sourceCards.set(node, card) }
  Object.assign(card, node, { worldW: node.worldW / scale, worldH: node.worldH / scale, subDetail: nodeDetail(node, zoom), subHeaderHeight: node.subHeaderHeight === undefined ? undefined : node.subHeaderHeight / scale })
  return card
}

export function scrollSubElement(node: LayoutNode, delta: number, zoom: number) {
  const scale = sourceCardScale(node)
  return scrollSubElementNative(sourceCard(node, zoom), delta / scale, zoom * scale)
}
export function subNodeHandleAnchor(node: LayoutNode, handle: string | null, zoom: number) {
  const scale = sourceCardScale(node)
  const anchor = subNodeHandleAnchorNative(sourceCard(node, zoom), handle, zoom * scale)
  return anchor ? { ...anchor, x: anchor.x * scale, y: anchor.y * scale } : null
}
export function subSymbolAtPoint(node: LayoutNode, zoom: number, x: number, y: number) {
  const scale = sourceCardScale(node)
  return subSymbolAtPointNative(sourceCard(node, zoom), zoom * scale, x / scale, y / scale)
}
export function drawSubElement(ctx: CanvasRenderingContext2D, node: LayoutNode, zoom: number, alpha: number, accent: string, lowDetail: boolean, activeHandle?: string, groupColor?: string, icon?: HTMLImageElement | null, headerBg?: string, symbolAccent?: string, bodyBg?: string) {
  const scale = sourceCardScale(node)
  if (scale !== 1) { ctx.save(); ctx.scale(scale, scale) }
  drawSubElementNative(ctx, sourceCard(node, zoom), zoom * scale, alpha, accent, lowDetail, activeHandle, groupColor, icon, headerBg, symbolAccent, bodyBg, 1 / scale)
  if (scale !== 1) ctx.restore()
}


/** Screen-space wheel or drag deltas; the source surface owns both axes. */
export function panSubElement(node: LayoutNode, deltaX: number, deltaY: number, zoom: number) {
  if (nodeDetail(node, zoom) === 'overview') return false
  const scale = sourceCardScale(node), nativeZoom = zoom * scale
  const card = sourceCard(node, zoom), model = subNodeSymbols(card, nativeZoom)
  const key = scrollKey(card, nativeZoom)
  const oldLeft = scrollColumns.get(key) ?? 0
  const left = Math.max(0, Math.min(Math.max(0, model.contentWidth - card.worldW), oldLeft + deltaX / nativeZoom))
  const oldTop = firstRow(card, nativeZoom)
  const top = Math.max(0, Math.min(Math.max(0, model.rowCount - (card.worldH - model.headerHeight) / model.rowHeight), oldTop + deltaY / nativeZoom / model.rowHeight))
  if (left !== oldLeft || top !== oldTop) {
    scrollColumns.set(key, left)
    scrollRows.set(key, top)
    if (scrollColumns.size > 128) scrollColumns.delete(scrollColumns.keys().next().value!)
    if (scrollRows.size > 128) scrollRows.delete(scrollRows.keys().next().value!)
    scrollListeners.forEach(fn => fn())
  }
  return true
}

/** Match the editor's overflow banners: reveal the nearest hidden symbol. */
export function revealSubOverflow(node: LayoutNode, zoom: number, y: number) {
  if (nodeDetail(node, zoom) === 'overview') return false
  const scale = sourceCardScale(node), card = sourceCard(node, zoom), nativeZoom = zoom * scale
  const model = subNodeSymbols(card, nativeZoom)
  y /= scale
  const direction = y >= model.headerHeight && y < model.headerHeight + SUB_ELLIPSIS ? 'above'
    : y >= card.worldH - SUB_ELLIPSIS && y <= card.worldH ? 'below' : null
  if (!direction) return false
  const overflow = model[direction]
  const area = direction === 'above' ? overflow.areas[overflow.areas.length - 1] : overflow.areas[0]
  const diff = nodeDetail(card, nativeZoom) === 'code' && !!(card.subSource ?? (card.sourceLink ? peekSubSource(card.sourceLink) : undefined))?.diff
  if (!area && !overflow.connectionCount && !diff) return false
  const visibleRows = Math.max(1, Math.floor((card.worldH - model.headerHeight) / model.rowHeight))
  const target = area ? area.start - Math.ceil(SUB_ELLIPSIS / model.rowHeight)
    : firstRow(card, nativeZoom) + (direction === 'above' ? -visibleRows : visibleRows - 1)
  const max = Math.max(0, model.rowCount - visibleRows)
  scrollRows.set(scrollKey(card, nativeZoom), Math.max(0, Math.min(max, target)))
  scrollListeners.forEach(fn => fn())
  return true
}
