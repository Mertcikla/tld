import type { SubSource } from './subElementSource'
import { displaySymbols, symbolForHandle, type SubSymbol } from './subElements'

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
export function connectedSourceRows(source: SubSource, connected: readonly string[], outline: boolean) {
  let cached = folds.get(source)
  if (!cached || cached.lines !== source.lines || cached.symbols !== source.symbols || cached.start !== source.startLine || cached.end !== source.endLine) {
    cached = { lines: source.lines, symbols: source.symbols, start: source.startLine, end: source.endLine, variants: new Map() }
    folds.set(source, cached)
  }
  const key = JSON.stringify([outline, connected])
  const previous = cached.variants.get(key)
  if (previous) return previous
  const symbols = [...displaySymbols(source.symbols)]
  const used = new Set<SubSymbol>()
  const looseLines = new Set<number>()
  for (const id of connected) {
    const named = symbolForHandle(id, source.symbols)
    let member = id.match(/-member-(.*)$/)?.[1]
    try { if (member) member = decodeURIComponent(member) } catch { /* Stale handles keep their fallback port. */ }
    const line = Number(id.match(/-line-(\d+)$/)?.[1])
    const selected = named
      ? symbols.find(symbol => symbol === named) ?? symbols.find(symbol => symbol.startLine >= named.startLine && symbol.endLine <= named.endLine)
      : member ? symbols.find(symbol => symbol.name === member || symbol.qualifiedName === member)
        : Number.isFinite(line) ? [...symbols].reverse().find(symbol => line >= symbol.startLine && line <= symbol.endLine) : undefined
    if (selected) used.add(selected)
    else if (Number.isFinite(line)) looseLines.add(line)
  }
  const rows: SourceDisplayRow[] = []
  if (outline) {
    // Nested connected members get their own signature instead of their enclosing class.
    for (const line of looseLines) {
      if (line < source.startLine || line > source.endLine) continue
      const symbol = { name: `Line ${line}`, startLine: line, endLine: line }
      symbols.push(symbol); used.add(symbol)
    }
    symbols.sort((a, b) => a.startLine - b.startLine || b.endLine - a.endLine)
    let omitted = 0
    for (const symbol of symbols) {
      if (!used.has(symbol)) { omitted++; continue }
      if (omitted) { rows.push({ sourceIndex: symbol.startLine - source.startLine, collapsed: omitted }); omitted = 0 }
      rows.push({ sourceIndex: symbol.startLine - source.startLine, symbol })
    }
    if (omitted) rows.push({ sourceIndex: source.lines.length - 1, collapsed: omitted })
  } else {
    const keep = new Uint8Array(source.lines.length)
    for (const symbol of used) {
      keep.fill(1, Math.max(0, symbol.startLine - source.startLine), Math.min(keep.length, symbol.endLine - source.startLine + 1))
    }
    // A connected class keeps its own code, while unused members inside it fold away.
    for (const symbol of symbols) {
      if (used.has(symbol) || [...used].some(active => active.startLine >= symbol.startLine && active.endLine <= symbol.endLine)) continue
      keep.fill(0, Math.max(0, symbol.startLine - source.startLine), Math.min(keep.length, symbol.endLine - source.startLine + 1))
    }
    for (const line of looseLines) {
      if (line >= source.startLine && line <= source.endLine) keep.fill(1, Math.max(0, line - source.startLine - 2), Math.min(keep.length, line - source.startLine + 3))
    }
    for (let index = 0; index < keep.length; index++) {
      if (keep[index]) rows.push({ sourceIndex: index })
      else {
        const start = index
        while (index + 1 < keep.length && !keep[index + 1]) index++
        rows.push({ sourceIndex: start, collapsed: index - start + 1 })
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
