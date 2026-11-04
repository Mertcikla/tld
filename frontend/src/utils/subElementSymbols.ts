import { connectedSourceRows, sourceCodeRows, sourceDiffRows, type SourceDisplayRow } from './subElementFolding'
import type { SubSource } from './subElementSource'
import { symbolSignature, type SymbolSignature } from './subElementSignatures'
import { expandCodeTabs, measureSubText, subCodeLineHeight, SUB_CODE_TEXT_X, SUB_SYMBOL_CONNECTOR_GAP, SUB_SYMBOL_PADDING } from './subElementText'
import { isSymbolDetail, expandLineHandles, firstHandleLine, collapsedSymbols, displaySymbols, isSubHandle, SUB_HEADER, symbolContainsLine, symbolForHandle, symbolHandle, type SubDetail, type SubSymbol } from './subElements'


export interface SymbolArea {
  key: string; symbol: SubSymbol; color: string
  signature?: SymbolSignature
  leftId: string; rightId: string; start: number; end: number
  x: number; width: number; contentTop: number; height: number
  y: number; visible: boolean; depth: number; connected: boolean
}
export interface SymbolEndpoint { id: string; y: number; side: 'left' | 'right'; color: string; visible: boolean; connected: boolean; area?: SymbolArea; overflow?: 'above' | 'below' }
export interface SymbolOverflow { areas: SymbolArea[]; endpoints: SymbolEndpoint[]; connectionCount: number; y: number; representative?: SymbolEndpoint; color: string }
export interface SymbolPresentation {
  headerHeight: number
  rows: SourceDisplayRow[]
  areas: SymbolArea[]; endpoints: Map<string, SymbolEndpoint>
  above: SymbolOverflow; below: SymbolOverflow
  rowHeight: number; highlightInset: number; contentInset: number; rowCount: number; contentWidth: number; fontSize: number; lineNumbers: boolean; textX: number
  viewportTop: number; viewportBottom: number
}
type DocumentMetrics = { symbols: SubSymbol[]; lines: string[]; entries: Array<{ symbol: SubSymbol; depth: number; textWidth: number; indent: number }>; contentWidth: number }
const documents = new WeakMap<SubSource, Map<string, DocumentMetrics>>()
function documentSymbols(source: SubSource, path: string, fontSize: number, textX: number) {
  let variants = documents.get(source)
  if (!variants) { variants = new Map(); documents.set(source, variants) }
  const key = `${fontSize}:${textX}:${path}`
  const cached = variants.get(key)
  if (cached?.symbols === source.symbols && cached.lines === source.lines) return cached
  const symbols = source.symbols.length ? displaySymbols(source.symbols) : collapsedSymbols(source, path)
  const lines = source.lines.map(line => {
    const text = expandCodeTabs(line)
    return { width: measureSubText(text, fontSize), indent: text.trim() ? measureSubText(text.match(/^ */)?.[0] ?? '', fontSize) : Infinity }
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
  const document = { symbols: source.symbols, lines: source.lines, entries, contentWidth: textX + lines.reduce((width, line) => Math.max(width, line.width), 0) + SUB_SYMBOL_PADDING * 2 }
  variants.set(key, document)
  if (variants.size > 8) variants.delete(variants.keys().next().value!)
  return document
}

export interface SymbolPresentationOptions {
  kind: string | null; source?: SubSource; path?: string; detail: SubDetail
  headerHeight?: number
  expandedFolds?: readonly number[]
  connectedOnly?: boolean; fractionalMetrics?: boolean
  width: number; height: number; scrollTop?: number; connected?: readonly string[]; connectionCounts?: Readonly<Record<string, number>>; fontSize?: number; lineNumbers?: boolean
}
type SymbolGeometry = Pick<SymbolPresentation, 'rows' | 'rowHeight' | 'highlightInset' | 'contentInset' | 'rowCount' | 'contentWidth' | 'fontSize' | 'lineNumbers' | 'textX'> & {
  areas: SymbolArea[]; connections: Array<{ id: string; area?: SymbolArea; index: number; count: number }>; detail: SubDetail; height: number; headerHeight: number
}

export function symbolPresentationRowHeight(detail: SubDetail, fontSize: number, fractionalMetrics = false) {
  return (fractionalMetrics ? fontSize * 1.5 : subCodeLineHeight(fontSize)) + (isSymbolDetail(detail) ? detail === 'outline' ? 8 : 4 : 0)
}

function buildSymbolGeometry({ kind, source, path = '', detail, width, height, connected = [], connectionCounts, fontSize = 12, lineNumbers = detail === 'code', connectedOnly = false, fractionalMetrics = false, headerHeight = SUB_HEADER, expandedFolds }: SymbolPresentationOptions): SymbolGeometry {
  const outline = collapsedSymbols(source, path)
  const symbolDetail = isSymbolDetail(detail)
  const signature = (symbol: SubSymbol): SymbolSignature => (detail === 'summary' || detail === 'structure')
    ? { text: symbol.name, tokens: [{ text: symbol.name, color: '#61afef' }] }
    : symbolSignature(source!, symbol)
  const rowHeight = symbolPresentationRowHeight(detail, fontSize, fractionalMetrics)
  const contentInset = 2
  // Full-code bounds expand around line boxes; outline rows reserve their own gap.
  const highlightInset = symbolDetail ? 2 : -2
  const diffMode = detail === 'code' && !!source?.diff
  const diffMaxLine = source?.diff?.lines.reduce((max, line) => Math.max(max, line.oldLine, line.newLine), 1) ?? 1
  const lineNumberWidth = measureSubText(String(diffMode ? diffMaxLine : source?.endLine ?? 1), fontSize)
  const gutter = Math.max(SUB_CODE_TEXT_X - 14, (fractionalMetrics ? lineNumberWidth : Math.ceil(lineNumberWidth)) + 12)
  const textX = diffMode ? 32 + (lineNumberWidth + 10) * 2 : lineNumbers ? 14 + gutter : 14
  const folded = !diffMode && connectedOnly && source?.status === 'ready' && detail !== 'overview'
    ? connectedSourceRows(source, connected.filter(id => (connectionCounts?.[id] ?? 1) > 0), symbolDetail, expandedFolds) : undefined
  const rows: SourceDisplayRow[] = folded?.rows ?? (symbolDetail
    ? outline.map(symbol => ({ sourceIndex: symbol.startLine - (source?.startLine ?? 1), symbol }))
    : source ? diffMode ? sourceDiffRows(source) : sourceCodeRows(source.lines) : [])
  const rowCount = rows.length
  const displayIndex = (line: number) => {
    const index = line - (source?.startLine ?? 1)
    if (!folded && !diffMode) return index
    const exact = rows.findIndex(row => !row.collapsed && (!row.diff || row.sourceIndex >= 0) && row.sourceIndex === index)
    if (exact >= 0) return exact
    return Math.max(0, rows.findIndex(row => row.collapsed && index >= row.sourceIndex && index < row.sourceIndex + row.collapsed))
  }
  const readingCode = kind === 'code' && detail === 'code'
  const document = readingCode && source?.status === 'ready' ? documentSymbols(source, path, fontSize, textX) : undefined
  const entries = !source || source.status !== 'ready' ? [] : symbolDetail ? (folded ? rows.flatMap(row => row.symbol ? [row.symbol] : []) : outline).map(symbol => ({ symbol, depth: 0, textWidth: measureSubText(signature(symbol).text, fontSize), indent: 0 })) : (folded ? document?.entries.filter(entry => folded.used.has(entry.symbol)).map(entry => ({ ...entry })) : document?.entries) ?? []
  if (folded && readingCode && source) for (const entry of entries) {
    const visible = rows.filter(row => !row.collapsed && row.sourceIndex + source.startLine >= entry.symbol.startLine && row.sourceIndex + source.startLine <= entry.symbol.endLine)
      .map(row => expandCodeTabs(source.lines[row.sourceIndex]))
    // Do not let folded-out lines retain their old horizontal footprint.
    entry.textWidth = visible.reduce((end, line) => Math.max(end, measureSubText(line, fontSize)), 0)
    const indents = visible.filter(line => line.trim()).map(line => measureSubText(line.match(/^ */)?.[0] ?? '', fontSize))
    entry.indent = indents.length ? indents.reduce((min, indent) => Math.min(min, indent), Infinity) : 0
  }
  const areas: SymbolArea[] = entries.map(({ symbol, depth, textWidth, indent }, index) => {
    const start = symbolDetail ? (folded ? rows.findIndex(row => row.symbol === symbol) : index) : displayIndex(symbol.startLine)
    const end = symbolDetail ? start : displayIndex(symbol.endLine)
    const leftId = symbolHandle('left', symbol)
    const rightId = symbolHandle('right', symbol)
    const x = readingCode ? textX + indent - SUB_SYMBOL_PADDING : textX - 4
    const y = headerHeight + contentInset + (start + 0.5) * rowHeight
    // Fold banners own their full row. Keep neighboring outlines inside their
    // code rows instead of shrinking the banner and its label to make room.
    const topInset = rows[start - 1]?.collapsed ? Math.max(1, highlightInset) : highlightInset
    const bottomInset = rows[end + 1]?.collapsed ? Math.max(1, highlightInset) : highlightInset
    return { key: rightId, symbol, color: 'var(--accent)', leftId, rightId, start, end, x,
      signature: kind === 'code' && symbolDetail && source ? signature(symbol) : undefined,
      width: Math.max(40, readingCode ? textWidth - indent + SUB_SYMBOL_PADDING * 2 : textWidth + 20),
      contentTop: start * rowHeight + contentInset + topInset,
      height: (end - start + 1) * rowHeight - topInset - bottomInset,
      y, visible: true, depth, connected: false }
  })
  const byHandle = new Map<string, SymbolArea>()
  for (const area of areas) { byHandle.set(area.leftId, area); byHandle.set(area.rightId, area) }
  const counts = new Map<string, number>()
  for (const id of connected) counts.set(id, (counts.get(id) ?? 0) + 1)
  // Expand merged endpoints for visibility/stems, while retaining the original
  // bundle ID as a hidden routing alias to the first declaration.
  const activeCounts = new Map<string, number>()
  for (const [id, count] of counts) for (const handle of expandLineHandles(id)) {
    activeCounts.set(handle, (activeCounts.get(handle) ?? 0) + (connectionCounts?.[id] ?? count))
  }
  const connections = [...new Set([...activeCounts.keys(), ...counts.keys()])].filter(isSubHandle).map(id => {
    let area = byHandle.get(id)
    const named = source ? symbolForHandle(id, source.symbols) : undefined
    const line = named?.startLine ?? firstHandleLine(id)
    if (!area && Number.isFinite(line)) area = [...areas].reverse().find(candidate => symbolContainsLine(candidate.symbol, line))
    if (!area && named) area = areas.find(candidate => candidate.symbol.startLine >= named.startLine && candidate.symbol.endLine <= named.endLine)
    const index = symbolDetail || (area && (named || line < area.symbol.startLine)) ? area?.start ?? 0 : Number.isFinite(line) ? displayIndex(line) : area?.start ?? 0
    return { id, area, index, count: activeCounts.get(id) ?? 0 }
  })
  const symbolContentWidth = kind === 'code' ? areas.reduce((end, area) => Math.max(end, area.x + area.width + SUB_SYMBOL_CONNECTOR_GAP), 0) : 0
  // Include long member labels and signatures in the horizontal scroll extent.
  const rowContentWidth = readingCode ? 0 : entries.reduce((end, { textWidth }) => Math.max(end, textWidth + textX + 34), 0)
  const documentWidth = diffMode
    ? rows.reduce((end, row) => Math.max(end, textX + measureSubText(expandCodeTabs(row.diff?.text ?? ''), fontSize) + SUB_SYMBOL_PADDING * 2), 0)
    : folded && source && readingCode
    ? rows.reduce((end, row) => row.collapsed ? end : Math.max(end, textX + measureSubText(expandCodeTabs(source.lines[row.sourceIndex]), fontSize) + SUB_SYMBOL_PADDING * 2), 0)
    : document?.contentWidth ?? 0
  return { rows, areas, connections, detail, height, headerHeight, rowHeight, highlightInset, contentInset, rowCount, fontSize, lineNumbers, textX, contentWidth: Math.max(width - 2, documentWidth, symbolContentWidth, rowContentWidth) }
}

// Scroll frames only move cached geometry and recompute visible ports/overflow.
function positionSymbolGeometry(geometry: SymbolGeometry, scrollTop: number): SymbolPresentation {
  const { rows, detail, height, headerHeight, rowHeight, highlightInset, contentInset, rowCount, fontSize, lineNumbers, textX, contentWidth } = geometry
  const bannerHeight = foldBannerBounds(geometry, 0).height
  const areas = geometry.areas.map(area => ({ ...area, y: area.y - scrollTop }))
  const areaCopies = new Map(geometry.areas.map((area, index) => [area, areas[index]]))
  const connections = geometry.connections.map(connection => ({ ...connection, area: connection.area ? areaCopies.get(connection.area) : undefined }))
  // Overflow bands overlay the content only when an anchor is outside the full viewport.
  // They never add padding or alter the document's scroll height.
  const positions = [...areas.map(area => area.y), ...connections.filter(connection => connection.count > 0).map(connection => headerHeight + contentInset + (connection.index + 0.5) * rowHeight - scrollTop)]
  const top = headerHeight + (positions.some(y => y < headerHeight) ? bannerHeight : 0)
  const bottom = height - (positions.some(y => y > height) ? bannerHeight : 0)
  for (const area of areas) area.visible = area.y >= top && area.y <= bottom
  const above: SymbolOverflow = { areas: areas.filter(area => area.y < top), endpoints: [], connectionCount: 0, y: headerHeight + bannerHeight / 2, color: 'var(--accent)' }
  const below: SymbolOverflow = { areas: areas.filter(area => area.y > bottom), endpoints: [], connectionCount: 0, y: height - bannerHeight / 2, color: 'var(--accent)' }
  const endpoints = new Map<string, SymbolEndpoint>()
  const addEndpoint = (id: string, area: SymbolArea | undefined, index: number, visible = false) => {
    const rawY = headerHeight + contentInset + (index + 0.5) * rowHeight - scrollTop
    const overflow = rawY < top ? 'above' : rawY > bottom ? 'below' : undefined
    const endpoint: SymbolEndpoint = { id, area, y: overflow ? (overflow === 'above' ? above.y : below.y) : rawY,
      side: id.startsWith('left') ? 'left' : 'right', color: area?.color ?? 'var(--accent)', visible: visible && !overflow, connected: false, overflow }
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
      overflow.color = overflow.representative?.color ?? 'var(--accent)'
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
  for (const endpoint of endpoints.values()) if (endpoint.connected && endpoint.area) endpoint.area.connected = true
  return { headerHeight, rows, areas, endpoints, above, below, rowHeight, highlightInset, contentInset, rowCount, fontSize, lineNumbers, textX, viewportTop: top, viewportBottom: bottom, contentWidth }
}

type CachedSymbolGeometry = {
  lines: SubSource['lines']; symbols: SubSource['symbols']; tokens: SubSource['tokens']; diffLines?: unknown
  status: SubSource['status']; start: number; end: number; variants: Map<string, SymbolGeometry>
}
const symbolGeometries = new WeakMap<SubSource, CachedSymbolGeometry>()

export function buildSymbolPresentation(options: SymbolPresentationOptions): SymbolPresentation {
  const { source } = options
  let geometry: SymbolGeometry
  if (!source) geometry = buildSymbolGeometry(options)
  else {
    let cached = symbolGeometries.get(source)
    if (!cached || cached.lines !== source.lines || cached.symbols !== source.symbols || cached.tokens !== source.tokens || cached.diffLines !== source.diff?.lines || cached.status !== source.status || cached.start !== source.startLine || cached.end !== source.endLine) {
      cached = { lines: source.lines, symbols: source.symbols, tokens: source.tokens, diffLines: source.diff?.lines, status: source.status, start: source.startLine, end: source.endLine, variants: new Map() }
      symbolGeometries.set(source, cached)
    }
    const key = JSON.stringify([options.expandedFolds, options.headerHeight, options.kind, options.path, options.detail, options.width, options.height, options.fontSize, options.lineNumbers, options.connectedOnly, options.fractionalMetrics, options.connected, options.connectionCounts])
    const existing = cached.variants.get(key)
    geometry = existing ?? buildSymbolGeometry(options)
    if (!existing) {
      cached.variants.set(key, geometry)
      if (cached.variants.size > 8) cached.variants.delete(cached.variants.keys().next().value!)
    }
  }
  return positionSymbolGeometry(geometry, options.scrollTop ?? 0)
}


/** Inner stems only join visible regions to their own in-bounds ports. */
export function symbolStems(presentation: SymbolPresentation, width: number, _height: number, scrollTop = 0, scrollLeft = 0) {
  return [...presentation.endpoints.values()].flatMap(endpoint => {
    const area = endpoint.area
    if (!endpoint.visible || endpoint.overflow || !area) return []
    const left = area.x - scrollLeft, right = left + area.width
    const top = presentation.headerHeight + area.contentTop - scrollTop, bottom = top + area.height
    if (right <= 0 || left >= width || bottom <= presentation.viewportTop || top >= presentation.viewportBottom) return []
    const sourceX = endpoint.side === 'right' ? right : left
    if (sourceX < 0 || sourceX > width) return []
    const inset = Math.min(8, area.height / 2, presentation.rowHeight / 2 - presentation.highlightInset)
    const sourceY = Math.max(Math.max(presentation.viewportTop, top + inset), Math.min(Math.min(presentation.viewportBottom, bottom - inset), endpoint.y))
    return [{ endpoint, area, sourceX, sourceY, targetX: endpoint.side === 'right' ? width : 0, targetY: endpoint.y }]
  })
}


/** A full-height fold row keeps its label legible at every source font size. */
export function foldBannerBounds(presentation: Pick<SymbolPresentation, 'rowHeight' | 'contentInset'>, index: number) {
  // The first banner absorbs the body's initial padding without moving any code.
  const inset = index === 0 ? 0 : presentation.contentInset
  return { top: index * presentation.rowHeight + inset, height: presentation.rowHeight + presentation.contentInset - inset }
}
