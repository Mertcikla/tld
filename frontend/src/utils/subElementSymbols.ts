import type { SubSource } from './subElementSource'
import { symbolSignature, type SymbolSignature } from './subElementSignatures'
import { expandCodeTabs, measureSubText, SUB_CODE_TEXT_X, SUB_SYMBOL_CONNECTOR_GAP, SUB_SYMBOL_PADDING } from './subElementText'
import { ACCENT_OPTIONS } from '../constants/colors'
import { collapsedSymbols, displaySymbols, isSubHandle, SUB_CONTENT_TOP, SUB_ELLIPSIS, SUB_HEADER, subRowHeight, symbolForHandle, symbolHandle, type SubDetail, type SubSymbol } from './subElements'

export interface SymbolArea {
  key: string; symbol: SubSymbol; color: string
  signature?: SymbolSignature
  leftId: string; rightId: string; start: number; end: number
  x: number; width: number; contentTop: number; height: number
  y: number; visible: boolean; depth: number
}
export interface SymbolEndpoint { id: string; y: number; side: 'left' | 'right'; color: string; visible: boolean; connected: boolean; area?: SymbolArea; overflow?: 'above' | 'below' }
export interface SymbolOverflow { areas: SymbolArea[]; endpoints: SymbolEndpoint[]; connectionCount: number; y: number; representative?: SymbolEndpoint; color: string }
export interface SymbolPresentation {
  areas: SymbolArea[]; endpoints: Map<string, SymbolEndpoint>
  above: SymbolOverflow; below: SymbolOverflow
  rowHeight: number; rowCount: number; contentWidth: number
  viewportTop: number; viewportBottom: number
}
const palette = ACCENT_OPTIONS.filter(color => color.name !== 'White' && color.name !== 'Black').map(color => color.value)
export function symbolGroupColor(key: string, groupColor?: string) {
  if (groupColor) return groupColor
  let hash = 2166136261
  for (let index = 0; index < key.length; index++) hash = Math.imul(hash ^ key.charCodeAt(index), 16777619)
  return palette[(hash >>> 0) % palette.length]
}
const documents = new WeakMap<SubSource, { symbols: SubSymbol[]; lines: string[]; entries: Array<{ symbol: SubSymbol; depth: number; textWidth: number; indent: number }>; contentWidth: number }>()
function documentSymbols(source: SubSource, path: string) {
  const cached = documents.get(source)
  if (cached?.symbols === source.symbols && cached.lines === source.lines) return cached
  const symbols = source.symbols.length ? displaySymbols(source.symbols) : collapsedSymbols(source, path)
  const lines = source.lines.map(line => {
    const text = expandCodeTabs(line)
    return { width: measureSubText(text), indent: text.trim() ? measureSubText(text.match(/^ */)?.[0] ?? '') : Infinity }
  })
  const stack: SubSymbol[] = []
  const entries = symbols.map(symbol => {
    while (stack.length && symbol.startLine > stack[stack.length - 1].endLine) stack.pop()
    const depth = stack.length
    stack.push(symbol)
    let textWidth = 0, indent = Infinity
    for (let line = symbol.startLine; line <= symbol.endLine; line++) {
      const metrics = lines[line - source.startLine]
      if (metrics) { textWidth = Math.max(textWidth, metrics.width); indent = Math.min(indent, metrics.indent) }
    }
    return { symbol, depth, textWidth, indent: Number.isFinite(indent) ? indent : 0 }
  })
  const document = { symbols: source.symbols, lines: source.lines, entries, contentWidth: SUB_CODE_TEXT_X + lines.reduce((width, line) => Math.max(width, line.width), 0) + SUB_SYMBOL_PADDING * 2 }
  documents.set(source, document)
  return document
}

export function buildSymbolPresentation({ kind, source, path = '', detail, width, height, scrollTop = 0, connected = [], connectionCounts, groupColor }: {
  kind: string | null; source?: SubSource; path?: string; detail: SubDetail
  width: number; height: number; scrollTop?: number; connected?: readonly string[]; connectionCounts?: Readonly<Record<string, number>>; groupColor?: string
}): SymbolPresentation {
  const outline = collapsedSymbols(source, path)
  const rowHeight = subRowHeight()
  const rowCount = detail === 'outline' ? outline.length : source?.lines.length ?? 0
  const readingCode = kind === 'code' && detail === 'code'
  const document = readingCode && source?.status === 'ready' ? documentSymbols(source, path) : undefined
  const entries = !source || source.status !== 'ready' ? [] : detail === 'outline' ? outline.map(symbol => ({ symbol, depth: 0, textWidth: measureSubText(symbolSignature(source, symbol).text), indent: 0 })) : document?.entries ?? []
  const areas: SymbolArea[] = entries.map(({ symbol, depth, textWidth, indent }, index) => {
    const start = detail === 'outline' ? index : symbol.startLine - (source?.startLine ?? 1)
    const end = detail === 'outline' ? index : symbol.endLine - (source?.startLine ?? 1)
    const leftId = symbolHandle('left', symbol)
    const rightId = symbolHandle('right', symbol)
    const x = readingCode ? SUB_CODE_TEXT_X + indent - SUB_SYMBOL_PADDING : 10
    const y = SUB_CONTENT_TOP + (start + 0.5) * rowHeight - scrollTop
    return { key: rightId, symbol, color: symbolGroupColor(rightId, groupColor), leftId, rightId, start, end, x,
      signature: kind === 'code' && detail === 'outline' && source ? symbolSignature(source, symbol) : undefined,
      width: Math.max(40, readingCode ? textWidth - indent + SUB_SYMBOL_PADDING * 2 : textWidth + 20),
      contentTop: start * rowHeight + 2,
      height: (end - start + 1) * rowHeight - 4,
      y, visible: true, depth }
  })
  const byHandle = new Map<string, SymbolArea>()
  for (const area of areas) { byHandle.set(area.leftId, area); byHandle.set(area.rightId, area) }
  const counts = new Map<string, number>()
  for (const id of connected) counts.set(id, (counts.get(id) ?? 0) + 1)
  const connections = [...counts.keys()].filter(isSubHandle).map(id => {
    let area = byHandle.get(id)
    const named = source ? symbolForHandle(id, source.symbols) : undefined
    const line = named?.startLine ?? Number(id.match(/-line-(\d+)$/)?.[1])
    if (!area && Number.isFinite(line)) area = [...areas].reverse().find(candidate => line >= candidate.symbol.startLine && line <= candidate.symbol.endLine)
    if (!area && named) area = areas.find(candidate => candidate.symbol.startLine >= named.startLine && candidate.symbol.endLine <= named.endLine)
    const index = detail === 'outline' || (named && area) ? area?.start ?? 0 : Number.isFinite(line) ? line - (source?.startLine ?? 1) : area?.start ?? 0
    return { id, area, index, count: connectionCounts?.[id] ?? counts.get(id) ?? 1 }
  })
  // Overflow bands overlay the content only when an anchor is outside the full viewport.
  // They never add padding or alter the document's scroll height.
  const positions = [...areas.map(area => area.y), ...connections.filter(connection => connection.count > 0).map(connection => SUB_CONTENT_TOP + (connection.index + 0.5) * rowHeight - scrollTop)]
  const top = SUB_CONTENT_TOP + (positions.some(y => y < SUB_CONTENT_TOP) ? SUB_ELLIPSIS : 0)
  const bottom = height - (positions.some(y => y > height) ? SUB_ELLIPSIS : 0)
  for (const area of areas) area.visible = area.y >= top && area.y <= bottom
  const above: SymbolOverflow = { areas: areas.filter(area => area.y < top), endpoints: [], connectionCount: 0, y: SUB_HEADER + SUB_ELLIPSIS / 2, color: '#a0adbd' }
  const below: SymbolOverflow = { areas: areas.filter(area => area.y > bottom), endpoints: [], connectionCount: 0, y: height - SUB_ELLIPSIS / 2, color: '#a0adbd' }
  const endpoints = new Map<string, SymbolEndpoint>()
  const addEndpoint = (id: string, area: SymbolArea | undefined, index: number, visible = false) => {
    const rawY = SUB_CONTENT_TOP + (index + 0.5) * rowHeight - scrollTop
    const overflow = rawY < top ? 'above' : rawY > bottom ? 'below' : undefined
    const endpoint: SymbolEndpoint = { id, area, y: overflow ? (overflow === 'above' ? above.y : below.y) : rawY,
      side: id.startsWith('left') ? 'left' : 'right', color: area?.color ?? '#a0adbd', visible: visible && !overflow, connected: false, overflow }
    endpoints.set(id, endpoint)
    return endpoint
  }
  if (detail !== 'overview') {
    for (const area of areas) if (area.visible) {
      addEndpoint(area.leftId, area, area.start, true)
      addEndpoint(area.rightId, area, area.start, true)
    }
    for (const { id, area, index, count } of connections) {
      const endpoint = addEndpoint(id, area, index, count > 0)
      endpoint.connected = count > 0
      if (endpoint.overflow) {
        const overflow = endpoint.overflow === 'above' ? above : below
        if (count > 0) { overflow.endpoints.push(endpoint); overflow.connectionCount += count }
      }
    }
    for (const overflow of [above, below]) {
      overflow.representative = overflow.endpoints.find(endpoint => endpoint.id.startsWith('right')) ?? overflow.endpoints[0]
      overflow.color = overflow.representative?.color ?? '#a0adbd'
      for (const side of ['left', 'right'] as const) {
        const representative = overflow.endpoints.find(endpoint => endpoint.side === side)
        if (representative) representative.visible = true
      }
    }
    // Saved line handles and declaration aliases can share a symbol's port.
    // Keep their routing IDs mounted, with a single visible port per position.
    const visiblePositions = new Set<string>()
    const connectedPositions = new Set([...endpoints.values()].filter(endpoint => endpoint.connected).map(endpoint => `${endpoint.side}:${endpoint.y}`))
    for (const endpoint of endpoints.values()) endpoint.connected = connectedPositions.has(`${endpoint.side}:${endpoint.y}`)
    for (const endpoint of endpoints.values()) if (endpoint.visible) {
      const position = `${endpoint.side}:${endpoint.y}`
      if (visiblePositions.has(position)) endpoint.visible = false
      else visiblePositions.add(position)
    }
  }
  const symbolContentWidth = kind === 'code' ? areas.reduce((end, area) => Math.max(end, area.x + area.width + SUB_SYMBOL_CONNECTOR_GAP), 0) : 0
  // Include long member labels and signatures in the horizontal scroll extent.
  const rowContentWidth = readingCode ? 0 : entries.reduce((end, { textWidth }) => Math.max(end, textWidth + 48), 0)
  return { areas, endpoints, above, below, rowHeight, rowCount, viewportTop: top, viewportBottom: bottom, contentWidth: Math.max(width - 2, document?.contentWidth ?? 0, symbolContentWidth, rowContentWidth) }
}

/** Inner stems only join visible regions to their own in-bounds ports. */
export function symbolStems(presentation: SymbolPresentation, width: number, _height: number, scrollTop = 0, scrollLeft = 0) {
  return [...presentation.endpoints.values()].flatMap(endpoint => {
    const area = endpoint.area
    if (!endpoint.visible || endpoint.overflow || !area) return []
    const left = area.x - scrollLeft, right = left + area.width
    const top = SUB_HEADER + area.contentTop - scrollTop, bottom = top + area.height
    if (right <= 0 || left >= width || bottom <= presentation.viewportTop || top >= presentation.viewportBottom) return []
    const sourceX = endpoint.side === 'right' ? right : left
    if (sourceX < 0 || sourceX > width) return []
    const inset = Math.min(8, area.height / 2)
    const sourceY = Math.max(Math.max(presentation.viewportTop, top + inset), Math.min(Math.min(presentation.viewportBottom, bottom - inset), endpoint.y))
    return [{ endpoint, area, sourceX, sourceY, targetX: endpoint.side === 'right' ? width : 0, targetY: endpoint.y }]
  })
}
