import type { SubSource } from './subElementSource'
import { displaySymbols, symbolContainsLine, symbolForHandle, expandLineHandles, firstHandleLine, type SubSymbol } from './subElements'

export interface SourceDisplayRow { diff?: import('../api/client').RepositorySourceDiffLine; sourceIndex: number; symbol?: SubSymbol; collapsed?: number }

const codeRows = new WeakMap<string[], SourceDisplayRow[]>()
export function sourceCodeRows(lines: string[]) {
  let rows = codeRows.get(lines)
  if (!rows) { rows = lines.map((_, sourceIndex) => ({ sourceIndex })); codeRows.set(lines, rows) }
  return rows
}

type FoldedRows = { rows: SourceDisplayRow[]; used: Set<SubSymbol> }
const folds = new WeakMap<SubSource, { lines: string[]; symbols: SubSymbol[]; start: number; end: number; variants: Map<string, FoldedRows> }>()

/** Keep original source coordinates separate from the folded display rows. */
export function connectedSourceRows(source: SubSource, connected: readonly string[], outline: boolean, expandedFolds: readonly number[] = []) {
  let cached = folds.get(source)
  if (!cached || cached.lines !== source.lines || cached.symbols !== source.symbols || cached.start !== source.startLine || cached.end !== source.endLine) {
    cached = { lines: source.lines, symbols: source.symbols, start: source.startLine, end: source.endLine, variants: new Map() }
    folds.set(source, cached)
  }
  const key = JSON.stringify([outline, connected, expandedFolds])
  const previous = cached.variants.get(key)
  if (previous) return previous
  const symbols = [...displaySymbols(source.symbols)]
  const used = new Set<SubSymbol>()
  const looseLines = new Set<number>()
  for (const id of connected.flatMap(expandLineHandles)) {
    const named = symbolForHandle(id, source.symbols)
    let member = id.match(/-member-(.*)$/)?.[1]
    try { if (member) member = decodeURIComponent(member) } catch { /* Stale handles keep their fallback port. */ }
    const line = firstHandleLine(id)
    const selected = named
      ? symbols.find(symbol => symbol === named) ?? symbols.find(symbol => symbol.startLine >= named.startLine && symbol.endLine <= named.endLine)
      : member ? symbols.find(symbol => symbol.name === member || symbol.qualifiedName === member)
        : Number.isFinite(line) ? [...symbols].reverse().find(symbol => symbolContainsLine(symbol, line)) : undefined
    if (selected) used.add(selected)
    else if (Number.isFinite(line)) looseLines.add(line)
  }
  const rows: SourceDisplayRow[] = []
  const expanded = new Set(expandedFolds)
  const appendFold = (sourceIndex: number, count: number, hiddenSymbols: SubSymbol[] = []) => {
    if (!expanded.has(sourceIndex)) { rows.push({ sourceIndex, collapsed: count }); return }
    if (outline) for (const symbol of hiddenSymbols) {
      rows.push({ sourceIndex: symbol.startLine - source.startLine, symbol })
      used.add(symbol)
    }
    else {
      for (let offset = 0; offset < count; offset++) rows.push({ sourceIndex: sourceIndex + offset })
      for (const symbol of symbols) if (symbol.startLine - source.startLine >= sourceIndex && symbol.startLine - source.startLine < sourceIndex + count) used.add(symbol)
    }
  }
  if (outline) {
    // Nested connected members get their own signature instead of their enclosing class.
    for (const line of looseLines) {
      if (line < source.startLine || line > source.endLine) continue
      const symbol = { name: `Line ${line}`, startLine: line, endLine: line }
      symbols.push(symbol); used.add(symbol)
    }
    symbols.sort((a, b) => a.startLine - b.startLine || b.endLine - a.endLine)
    let omitted: SubSymbol[] = []
    for (const symbol of symbols) {
      if (!used.has(symbol)) { omitted.push(symbol); continue }
      if (omitted.length) { appendFold(symbol.startLine - source.startLine, omitted.length, omitted); omitted = [] }
      rows.push({ sourceIndex: symbol.startLine - source.startLine, symbol })
    }
    if (omitted.length) appendFold(source.lines.length - 1, omitted.length, omitted)
  } else {
    const keep = new Uint8Array(source.lines.length)
    for (const symbol of used) {
      keep.fill(1, Math.max(0, (symbol.attachmentStartLine ?? symbol.startLine) - source.startLine), Math.min(keep.length, symbol.endLine - source.startLine + 1))
    }
    // A connected class keeps its own code, while unused members inside it fold away.
    for (const symbol of symbols) {
      if (used.has(symbol) || [...used].some(active => active.startLine >= symbol.startLine && active.endLine <= symbol.endLine)) continue
      keep.fill(0, Math.max(0, (symbol.attachmentStartLine ?? symbol.startLine) - source.startLine), Math.min(keep.length, symbol.endLine - source.startLine + 1))
    }
    for (const line of looseLines) {
      if (line >= source.startLine && line <= source.endLine) keep.fill(1, Math.max(0, line - source.startLine - 2), Math.min(keep.length, line - source.startLine + 3))
    }
    for (let index = 0; index < keep.length; index++) {
      if (keep[index]) rows.push({ sourceIndex: index })
      else {
        const start = index
        while (index + 1 < keep.length && !keep[index + 1]) index++
        appendFold(start, index - start + 1)
      }
    }
  }
  const result = { rows, used }
  cached.variants.set(key, result)
  if (cached.variants.size > 8) cached.variants.delete(cached.variants.keys().next().value!)
  return result
}


const diffRows = new WeakMap<SubSource, { lines: unknown; rows: SourceDisplayRow[] }>()
export function sourceDiffRows(source: SubSource): SourceDisplayRow[] {
  const cached = diffRows.get(source)
  if (cached?.lines === source.diff?.lines) return cached?.rows ?? []
  const rows = (source.diff?.lines ?? []).map(diff => ({ diff,
    sourceIndex: (source.diff?.side === 'base' ? diff.oldLine : diff.newLine) - source.startLine,
  }))
  diffRows.set(source, { lines: source.diff?.lines, rows })
  return rows
}
