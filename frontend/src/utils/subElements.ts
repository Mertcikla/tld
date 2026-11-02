import type { SourcePreviewSize } from './sourcePreviewSize'
import { sourceLinkKind } from './sourceLinks'

export const SUB_HEADER = 84
export const SUB_ROW = 24
export const SUB_ELLIPSIS = 16
export const SUB_CONTENT_TOP = SUB_HEADER
export type SubDetail = 'overview' | 'outline' | 'code'
export const isSubElement = (kind?: string | null) => kind === 'code'
export function subElementKind(element?: { kind?: string | null; repo?: string | null; repository_id?: string | null; file_path?: string | null } | null) {
  return (element?.repo || element?.repository_id) && sourceLinkKind(element.file_path) === 'file' ? 'code' : null
}
export const subDetail = (width: number, height: number): SubDetail => width < 320 || height < 180 ? 'overview' : width < 440 || height < 280 ? 'outline' : 'code'
export function subSize(kind?: string | null, value?: SourcePreviewSize | null) {
  if (!isSubElement(kind)) return { width: 180, height: 85 }
  return {
    width: Math.max(280, Math.min(2400, Number.isFinite(value?.width) && value!.width > 0 ? value!.width : 520)),
    height: Math.max(160, Math.min(2400, Number.isFinite(value?.height) && value!.height > 0 ? value!.height : 360)),
  }
}
export function subEditorSize(kind: string | null | undefined, value: SourcePreviewSize | null | undefined, detail: SubDetail) {
  if (detail === 'overview') return { width: 180, height: 85 }
  if (detail === 'outline' && !value) return { width: 400, height: 240 }
  return subSize(kind, value)
}

// Large cards should not force connector control points across nearby cards.
export const subRouteDimension = (kind: string | null | undefined, size: number, gap: number) => isSubElement(kind) ? Math.min(size, Math.max(24, gap)) : size
export function subRowHeight() { return SUB_ROW }
export const memberHandle = (side: 'left' | 'right', id: string) => `${side}-member-${encodeURIComponent(id)}`
export const lineHandle = (side: 'left' | 'right', line: number) => `${side}-line-${line}`
export interface SubSymbol { name: string; type?: string; qualifiedName?: string; startLine: number; endLine: number }
const symbolKey = (symbol: SubSymbol) => `${symbol.type ?? ''}:${symbol.qualifiedName || symbol.name}`
export const symbolHandle = (side: 'left' | 'right', symbol: SubSymbol) => `${side}-symbol-${encodeURIComponent(symbolKey(symbol))}`
export function symbolForHandle(id: string, symbols: SubSymbol[]) {
  if (!id.includes('-symbol-')) return undefined
  return symbols.find(s => symbolHandle(id.startsWith('left') ? 'left' : 'right', s) === id)
}
export const isSubHandle = (id?: string | null) => /^(left|right)-(member|line|symbol)-/.test(id ?? '')
export function subHandleY(id: string, _value: SourcePreviewSize | null | undefined, startLine = 1, height = 360, symbols: SubSymbol[] = [], scrollTop = 0) {
  let index = -1
  const symbol = symbolForHandle(id, symbols)
  if (symbol) index = symbol.startLine - startLine
  const line = id.match(/^(?:left|right)-line-(\d+)$/)
  if (line) index = Number(line[1]) - startLine
  // Deleted members and out-of-range lines keep a visible fallback endpoint.
  const rowHeight = SUB_ROW
  return Math.max(SUB_HEADER + SUB_ELLIPSIS / 2, Math.min(height - SUB_ELLIPSIS / 2, SUB_CONTENT_TOP + (Math.max(0, index) + 0.5) * rowHeight - scrollTop))
}
export const isDetailedSubElement = (kind: string | null | undefined, detail?: SubDetail, width = 520, height = 360, zoom = 1) => isSubElement(kind) && (detail ?? subDetail(width * zoom, height * zoom)) !== 'overview'

const collapsedSymbolCache = new WeakMap<SubSymbol[], SubSymbol[]>()
const displaySymbolCache = new WeakMap<SubSymbol[], SubSymbol[]>()

// Go exposes both a declaration wrapper and its named type specifications.
// Render the named symbols once; retain the original symbols for saved handle lookup.
export function displaySymbols(symbols: SubSymbol[]): SubSymbol[] {
  const cached = displaySymbolCache.get(symbols)
  if (cached) return cached
  const specifications = symbols.filter(symbol => symbol.type === 'type_spec' || symbol.type === 'type_alias')
  const seen = new Set<string>()
  const result = symbols.filter(symbol => {
    if (symbol.type === 'type_declaration' && specifications.some(spec => spec.startLine >= symbol.startLine && spec.endLine <= symbol.endLine)) return false
    const key = `${symbol.qualifiedName || symbol.name}:${symbol.startLine}:${symbol.endLine}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).sort((a, b) => a.startLine - b.startLine || b.endLine - a.endLine)
  displaySymbolCache.set(symbols, result)
  return result
}

// Source snapshots replace their symbol arrays, so folding is cached between canvas frames.
// Keep one row per outer symbol; nested bodies remain folded into their owner.
export function collapsedSymbols(source: { symbols: SubSymbol[]; startLine: number; endLine: number; status: string } | undefined, path = ''): SubSymbol[] {
  if (!source || source.status !== 'ready') return []
  let rows = collapsedSymbolCache.get(source.symbols)
  if (!rows) {
    const sorted = displaySymbols(source.symbols)
    rows = []
    for (const symbol of sorted) {
      const previous = rows[rows.length - 1]
      if (previous && symbol.startLine >= previous.startLine && symbol.endLine <= previous.endLine) continue
      rows.push(symbol)
    }
    collapsedSymbolCache.set(source.symbols, rows)
  }
  return rows.length ? rows : [{ name: path.split('#')[0].split(/[\\/]/).pop() || 'Source', startLine: source.startLine, endLine: source.endLine }]
}

export function outlineHandleY(id: string, rows: SubSymbol[], allSymbols: SubSymbol[], height: number, scrollTop = 0) {
  const symbol = symbolForHandle(id, allSymbols) ?? symbolForHandle(id, rows)
  const line = symbol ? symbol.startLine : Number(id.match(/-line-(\d+)$/)?.[1])
  const index = rows.findIndex(row => line >= row.startLine && line <= row.endLine)
  return Math.max(SUB_HEADER + SUB_ELLIPSIS / 2, Math.min(height - SUB_ELLIPSIS / 2, SUB_CONTENT_TOP + (Math.max(0, index) + 0.5) * SUB_ROW - scrollTop))
}
