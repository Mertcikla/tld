import { describe, expect, it } from 'vitest'
import { buildSymbolPresentation, symbolStems } from './subElementSymbols'
import type { SubSource } from './subElementSource'
import { lineHandle, symbolHandle } from './subElements'
import { expandCodeTabs, measureSubText, SUB_CODE_TEXT_X, SUB_SYMBOL_PADDING } from './subElementText'

const outer = { name: 'User', type: 'class', startLine: 2, endLine: 16 }
const save = { name: 'save', qualifiedName: 'User.save', type: 'method', startLine: 8, endLine: 12 }
const find = { name: 'find', type: 'function', startLine: 19, endLine: 24 }
const source: SubSource = { status: 'ready', startLine: 1, endLine: 30, lines: Array.from({ length: 30 }, (_, i) => `${i >= 7 && i <= 11 ? '    ' : '  '}return user;`), tokens: [], symbols: [outer, save, find] }
const base = { kind: 'code', source, path: 'users.ts', detail: 'code' as const, width: 600, height: 360 }

describe('symbol selection areas and semantic ports', () => {
  it('uses the full body without empty overflow bands and keeps the first row connected', () => {
    const id = symbolHandle('right', outer)
    const model = buildSymbolPresentation({ ...base, detail: 'outline', height: 135, connected: [id] })
    expect(model.viewportTop).toBe(84)
    expect(model.viewportBottom).toBe(135)
    expect(model.above.areas).toHaveLength(0)
    expect(model.below.areas).toHaveLength(0)
    expect(model.areas[0].contentTop).toBe(4)
    expect(model.areas.every(area => area.visible)).toBe(true)
    expect(symbolStems(model, 600, 135)[0]).toMatchObject({ targetY: 99, sourceY: 99 })
  })

  it('aggregates anchors beneath an active overflow band while keeping raw line connections', () => {
    const model = buildSymbolPresentation({ ...base, detail: 'outline', height: 135, scrollTop: 32, connected: [symbolHandle('right', outer)] })
    expect(model.viewportTop).toBe(100)
    expect(model.above.areas.map(area => area.symbol)).toEqual([outer, find])
    expect(model.above.representative?.visible).toBe(true)
    const lines = buildSymbolPresentation({ kind: 'code', detail: 'code', width: 400, height: 240, connected: [lineHandle('right', 50)] })
    expect(lines.below.areas).toHaveLength(0)
    expect(lines.below.connectionCount).toBe(1)
    expect(lines.viewportBottom).toBe(224)
    expect(lines.below.representative?.visible).toBe(true)
  })

  it('uses the application accent for all symbols and endpoints', () => {
    const model = buildSymbolPresentation(base)
    expect(model.areas.every(area => area.color === 'var(--accent)')).toBe(true)
    expect([...model.endpoints.values()].every(endpoint => endpoint.color === 'var(--accent)')).toBe(true)
  })

  it('pads nested symbol ranges and anchors both sides to their start line', () => {
    const left = symbolHandle('left', save), right = symbolHandle('right', save)
    const model = buildSymbolPresentation({ ...base, connected: [left] })
    const area = model.areas.find(candidate => candidate.symbol.name === 'save')!
    expect(area.depth).toBe(1)
    expect(area.x).toBeGreaterThan(model.areas[0].x)
    expect(area.height).toBe(5 * 18 + 4)
    expect(area.width).toBeGreaterThan(source.lines[7].trimStart().length * 7.2)
    expect(model.endpoints.get(left)).toMatchObject({ y: 221, side: 'left', visible: true, color: area.color })
    expect(model.endpoints.get(right)).toMatchObject({ y: 221, side: 'right', visible: true, color: area.color })
  })

  it('aggregates off-screen connections without losing their IDs or counts', () => {
    const above = symbolHandle('left', outer), below = symbolHandle('right', find)
    const raw = lineHandle('left', 28)
    const model = buildSymbolPresentation({ ...base, scrollTop: 30, connected: [above, above, below, raw] })
    expect(model.above.connectionCount).toBe(2)
    expect(model.below.connectionCount).toBe(2)
    expect(model.endpoints.get(above)).toMatchObject({ y: 92, side: 'left', overflow: 'above', visible: true })
    expect(model.endpoints.get(below)).toMatchObject({ y: 352, side: 'right', overflow: 'below' })
    expect(model.endpoints.get(raw)).toMatchObject({ y: 352, side: 'left', overflow: 'below' })
    expect(model.below.endpoints.filter(endpoint => endpoint.visible)).toHaveLength(2)
    expect(model.endpoints.size).toBeGreaterThanOrEqual(3)
  })

  it('routes nested symbols to their enclosing collapsed row', () => {
    const id = symbolHandle('right', save)
    const model = buildSymbolPresentation({ ...base, detail: 'outline', height: 164, connected: [id] })
    expect(model.areas.map(area => area.symbol.name)).toEqual(['User', 'find'])
    expect(model.endpoints.get(id)).toMatchObject({ y: 99, area: { symbol: outer } })
    expect(buildSymbolPresentation({ ...base, detail: 'overview' }).endpoints.size).toBe(0)
  })

  it('keeps hidden connections attached but excludes them from overflow counts', () => {
    const id = symbolHandle('right', find)
    const model = buildSymbolPresentation({ ...base, connected: [id], connectionCounts: { [id]: 0 } })
    expect(model.endpoints.has(id)).toBe(true)
    expect(model.below.connectionCount).toBe(0)
    expect(model.below.representative).toBeUndefined()
  })

  it('renders a Go type specification once while keeping wrapper and line connections', () => {
    const spec = { name: 'User', type: 'type_spec', startLine: 2, endLine: 16 }
    const wrapper = { ...spec, name: 'User struct { id string }', type: 'type_declaration' }
    const doc = { ...source, symbols: [wrapper, spec] }
    const ids = [symbolHandle('right', wrapper), lineHandle('right', 2)]
    const model = buildSymbolPresentation({ ...base, source: doc, connected: ids })
    expect(model.areas.map(area => area.symbol)).toEqual([spec])
    expect([...model.endpoints.values()].filter(endpoint => endpoint.visible)).toHaveLength(2)
    expect(model.endpoints.get(symbolHandle('right', spec))?.connected).toBe(true)
    for (const id of ids) expect(model.endpoints.get(id)?.area?.symbol).toEqual(spec)
    expect(symbolStems(model, 600, 360)).toHaveLength(2)
    expect(buildSymbolPresentation({ ...base, source: doc, detail: 'outline', connected: ids }).areas.map(area => area.symbol)).toEqual([spec])
  })

  it('moves stems with horizontal scroll and removes them when their region scrolls beyond the viewport', () => {
    const model = buildSymbolPresentation(base)
    const original = symbolStems(model, 600, 360)[0]
    const scrolled = symbolStems(model, 600, 360, 0, 50)[0]
    expect(scrolled.sourceX).toBeCloseTo(original.sourceX - 50)
    expect(symbolStems(model, 600, 360, 0, 1000)).toEqual([])
    const id = symbolHandle('right', outer)
    const above = buildSymbolPresentation({ ...base, scrollTop: 120, connected: [id] })
    expect(above.above.representative?.visible).toBe(true)
    expect(symbolStems(above, 600, 360, 120).some(stem => stem.area.symbol === outer)).toBe(false)
  })

  it('joins connected line ports to their symbol region without duplicate stems', () => {
    const id = lineHandle('right', 4)
    const model = buildSymbolPresentation({ ...base, connected: [id, id] })
    const stem = symbolStems(model, 600, 360).find(candidate => candidate.endpoint.id === id)
    expect(stem?.area.symbol).toBe(outer)
    expect(stem?.targetY).toBe(149)
    expect(stem?.sourceY).toBe(stem?.targetY)
  })

  it('keeps symbol and member stems level with their handles while scrolling', () => {
    const id = symbolHandle('right', save)
    for (const scrollTop of [0, 50, 160]) {
      const model = buildSymbolPresentation({ ...base, scrollTop, connected: [id] })
      for (const stem of symbolStems(model, 600, 360, scrollTop)) expect(stem.sourceY).toBe(stem.targetY)
    }

  })

  it('encloses long indented code without limiting its boundary to the card width', () => {
    const lines = ['\tfunction save() {', `\t\treturn "${'x'.repeat(150)}";`, '\t}']
    const symbol = { name: 'save', type: 'function', startLine: 1, endLine: 3 }
    const doc = { ...source, startLine: 1, endLine: 3, lines, symbols: [symbol] }
    const model = buildSymbolPresentation({ ...base, width: 300, source: doc })
    const area = model.areas[0]
    expect(area.width).toBeGreaterThan(300)
    expect(area.x).toBeCloseTo(SUB_CODE_TEXT_X + measureSubText('    ') - SUB_SYMBOL_PADDING)
    for (const line of lines) expect(area.x + area.width).toBeGreaterThanOrEqual(SUB_CODE_TEXT_X + measureSubText(expandCodeTabs(line)) + SUB_SYMBOL_PADDING)
    expect(model.contentWidth).toBeGreaterThan(area.x + area.width)
    expect(buildSymbolPresentation({ ...base, width: 900, source: doc }).areas[0].width).toBe(area.width)
    // The boundary is outside the viewport until the user scrolls to its right edge.
    expect(symbolStems(model, 300, 360).filter(stem => stem.endpoint.side === 'right')).toHaveLength(0)
    const scrollLeft = area.x + area.width - 250
    expect(symbolStems(model, 300, 360, 0, scrollLeft)[0].sourceX).toBeCloseTo(250)
  })

  it('retains horizontal scroll space for long collapsed signatures', () => {
    const name = 'LongSymbol'.repeat(20)
    const lines = [...source.lines]
    lines[outer.startLine - source.startLine] = `class ${name} {`
    const doc = { ...source, lines, symbols: [{ ...outer, name }] }
    const outline = buildSymbolPresentation({ ...base, source: doc, detail: 'outline', width: 400, height: 240 })
    expect(outline.contentWidth).toBeGreaterThan(measureSubText(doc.symbols[0].name))
  })
})


it('keeps symbol bounds and anchors in sync with font size and the line-number gutter', () => {
  const id = symbolHandle('right', outer)
  const small = buildSymbolPresentation({ ...base, connected: [id] })
  const large = buildSymbolPresentation({ ...base, connected: [id], fontSize: 24 })
  expect(large.rowHeight).toBe(36)
  expect(large.areas[0].width - 16).toBeCloseTo((small.areas[0].width - 16) * 2)
  expect(large.endpoints.get(id)?.y).toBe(140)
  const hidden = buildSymbolPresentation({ ...base, connected: [id], fontSize: 24, lineNumbers: false })
  expect(hidden.textX).toBe(14)
  expect(hidden.areas[0].x).toBeLessThan(large.areas[0].x)
  expect(hidden.endpoints.get(id)?.y).toBe(large.endpoints.get(id)?.y)
  expect(buildSymbolPresentation({ ...base }).areas[0].width).toBe(small.areas[0].width)
})

describe('connected parts layout', () => {
  it('shows one fallback signature per source line while retaining every connection', () => {
    const doc: SubSource = { status: 'ready', startLine: 1, endLine: 3, tokens: [], symbols: [],
      lines: ['export default function RepositoryChangeCanvas() {', 'const view = useMemo(() => scene, [scene])', '}'] }
    const outgoing = lineHandle('right', 1), incoming = lineHandle('left', 1)
    const second = lineHandle('left', 2)
    const connected = [...Array<string>(8).fill(outgoing), incoming, second, second]
    const model = buildSymbolPresentation({ ...base, source: doc, detail: 'outline', connectedOnly: true, connected })
    expect(model.rows.filter(row => !row.collapsed).map(row => row.sourceIndex)).toEqual([0, 1])
    expect(model.areas.map(area => area.signature?.text)).toEqual([
      'export default function RepositoryChangeCanvas()', 'const view = useMemo(() => scene, [scene])',
    ])
    expect(model.endpoints.get(outgoing)?.area).toBe(model.areas[0])
    expect(model.endpoints.get(incoming)?.area).toBe(model.areas[0])
    expect(model.endpoints.get(second)?.area).toBe(model.areas[1])
    expect(model.endpoints.get(outgoing)?.y).toBe(model.endpoints.get(incoming)?.y)
    expect(model.areas.every(area => area.connected)).toBe(true)
    const overflow = buildSymbolPresentation({ ...base, source: doc, height: 90, detail: 'outline', connectedOnly: true, connected })
    expect(overflow.below.connectionCount).toBe(11)
  })

  it('exposes connected nested methods in compact layout instead of their unused class', () => {
    const id = symbolHandle('right', save)
    const model = buildSymbolPresentation({ ...base, detail: 'outline', connectedOnly: true, connected: [id] })
    expect(model.areas.map(area => area.symbol.name)).toEqual(['save'])
    expect(model.rows.filter(row => row.collapsed).map(row => row.collapsed)).toEqual([1, 1])
    expect(model.endpoints.get(id)?.area?.symbol).toBe(save)
    expect(model.areas[0].signature?.text).toBeTruthy()
  })

  it('folds unused code while retaining original source indices and line ports', () => {
    const id = lineHandle('right', 10)
    const model = buildSymbolPresentation({ ...base, connectedOnly: true, connected: [id] })
    expect(model.rows.filter(row => !row.collapsed).map(row => row.sourceIndex)).toEqual([7, 8, 9, 10, 11])
    expect(model.rowCount).toBe(7)
    expect(model.areas.map(area => area.symbol.name)).toEqual(['save'])
    expect(model.endpoints.get(id)).toMatchObject({ y: 149, visible: true, area: { symbol: save } })
    expect(symbolStems(model, 600, 360).find(stem => stem.endpoint.id === id)?.targetY).toBe(149)
    expect(buildSymbolPresentation(base).rowCount).toBe(source.lines.length)
    expect(source.lines).toHaveLength(30)
  })

  it('folds unused class members even when their enclosing class is connected', () => {
    const model = buildSymbolPresentation({ ...base, connectedOnly: true, connected: [symbolHandle('left', outer)] })
    expect(model.rows.some(row => !row.collapsed && row.sourceIndex === save.startLine - 1)).toBe(false)
    expect(model.rows.some(row => !row.collapsed && row.sourceIndex === outer.startLine - 1)).toBe(true)
    expect(model.areas.map(area => area.symbol.name)).toEqual(['User'])
  })

  it('keeps selected class members and folds unused long lines out of horizontal sizing', () => {
    const lines = [...source.lines]; lines[find.startLine - 1] = 'x'.repeat(2000)
    const doc = { ...source, lines }
    const full = buildSymbolPresentation({ ...base, source: doc })
    const folded = buildSymbolPresentation({ ...base, source: doc, connectedOnly: true, connected: [symbolHandle('right', save)] })
    expect(folded.contentWidth).toBeLessThan(full.contentWidth)
    expect(buildSymbolPresentation({ ...base, source: doc }).contentWidth).toBe(full.contentWidth)
    const both = buildSymbolPresentation({ ...base, connectedOnly: true, connected: [symbolHandle('right', outer), symbolHandle('left', save)] })
    expect(both.rows.some(row => !row.collapsed && row.sourceIndex === save.startLine - 1)).toBe(true)
  })

  it.each(['outline', 'code'] as const)('collapses everything unused in %s, retaining hidden and stale handle IDs', detail => {
    const hidden = symbolHandle('right', find), stale = 'left-symbol-deleted'
    const model = buildSymbolPresentation({ ...base, detail, connectedOnly: true, connected: [hidden, stale], connectionCounts: { [hidden]: 0 } })
    expect(model.areas).toHaveLength(0)
    expect(model.rows).toHaveLength(1)
    expect(model.rows[0].collapsed).toBeGreaterThan(0)
    expect(model.endpoints.has(hidden)).toBe(true)
    expect(model.endpoints.has(stale)).toBe(true)
  })

  it.each(['outline', 'code'] as const)('supports direct line connections outside symbols in %s with nonzero file offsets', detail => {
    const doc = { ...source, symbols: [], startLine: 101, endLine: 130 }
    const id = lineHandle('left', 120)
    const model = buildSymbolPresentation({ ...base, source: doc, detail, connectedOnly: true, connected: [id] })
    expect(model.rows.some(row => !row.collapsed && row.sourceIndex === 19)).toBe(true)
    const endpoint = model.endpoints.get(id)!
    expect(endpoint.overflow).toBeUndefined()
    expect([...model.endpoints.values()].some(candidate => candidate.visible && candidate.y === endpoint.y && candidate.side === endpoint.side)).toBe(true)
    expect(model.rowCount).toBeLessThan(doc.lines.length)
  })
})

describe('intermediate symbol summaries', () => {
  it('shows connected nested symbol names without signatures or source bodies', () => {
    const id = symbolHandle('right', save)
    const summary = buildSymbolPresentation({ ...base, detail: 'summary', width: 320, height: 180, connectedOnly: true, connected: [id] })
    const outline = buildSymbolPresentation({ ...base, detail: 'outline', connectedOnly: true, connected: [id] })
    expect(summary.areas.map(area => area.signature?.text)).toEqual(['save'])
    expect(summary.areas[0].width).toBeLessThan(outline.areas[0].width)
    expect(summary.rowHeight).toBeLessThan(outline.rowHeight)
    expect(summary.rows.filter(row => row.collapsed)).toHaveLength(2)
    expect(summary.endpoints.get(id)?.area?.symbol).toBe(save)
    expect(summary.endpoints.get(id)?.overflow).toBeUndefined()
    expect(symbolStems(summary, 320, 180).some(stem => stem.endpoint.id === id)).toBe(true)
  })

  it('keeps saved line aliases attached to the compact symbol row', () => {
    const id = lineHandle('left', 10)
    const summary = buildSymbolPresentation({ ...base, detail: 'summary', connectedOnly: true, connected: [id] })
    expect(summary.endpoints.get(id)?.area?.symbol).toBe(save)
    expect(summary.endpoints.get(id)?.y).toBe(summary.areas[0].y)
  })
})

it('preserves semantic anchors in the text-free structure stage', () => {
  const id = symbolHandle('right', save)
  const model = buildSymbolPresentation({ ...base, detail: 'structure', connectedOnly: true, connected: [id] })
  expect(model.areas.map(area => area.symbol.name)).toEqual(['save'])
  expect(model.endpoints.get(id)?.area?.symbol).toBe(save)
  expect(model.endpoints.get(id)?.overflow).toBeUndefined()
  expect(symbolStems(model, 600, 360).some(stem => stem.endpoint.id === id)).toBe(true)
})

it('uses fractional text metrics in Explore while preserving the editor layout', () => {
  const doc = { ...source, endLine: 1234567 }
  const fontSize = 10.34
  const continuous = buildSymbolPresentation({ ...base, source: doc, fontSize, fractionalMetrics: true })
  const editor = buildSymbolPresentation({ ...base, source: doc, fontSize })
  expect(continuous.rowHeight).toBeCloseTo(15.51)
  expect(editor.rowHeight).toBe(16)
  const measured = measureSubText(String(doc.endLine), fontSize)
  expect(continuous.textX).toBeCloseTo(14 + Math.max(SUB_CODE_TEXT_X - 14, measured + 12))
  expect(editor.textX).toBe(14 + Math.max(SUB_CODE_TEXT_X - 14, Math.ceil(measured) + 12))
})


it('keeps added and removed Git rows visible and maps Head anchors past deletions', () => {
  const doc: SubSource = { status: 'ready', startLine: 1, endLine: 3, lines: ['same', 'new', 'last'], tokens: [], symbols: [],
    diff: { side: 'head', baseTokens: [], headTokens: [], lines: [
      { change: 'unchanged', oldLine: 1, newLine: 1, text: 'same' },
      { change: 'removed', oldLine: 2, newLine: 0, text: 'old' },
      { change: 'added', oldLine: 0, newLine: 2, text: 'new' },
      { change: 'unchanged', oldLine: 3, newLine: 3, text: 'last' },
    ] } }
  const model = buildSymbolPresentation({ ...base, source: doc, detail: 'code', connectedOnly: true, connected: [lineHandle('left', 3)] })
  expect(model.rows.map(row => row.diff?.change)).toEqual(['unchanged', 'removed', 'added', 'unchanged'])
  expect(model.rows.map(row => row.sourceIndex)).toEqual([0, -1, 1, 2])
  expect(model.endpoints.get(lineHandle('left', 3))?.y).toBe(84 + 2 + 3.5 * model.rowHeight)
  const ordinary = buildSymbolPresentation({ ...base, source: { ...doc, diff: undefined }, detail: 'code' })
  expect(ordinary.rows).toHaveLength(3)
  expect(ordinary.rows.every(row => !row.diff)).toBe(true)
})

it.each(['summary', 'outline', 'code'] as const)('keeps every symbol of one merged file connector visible in %s', detail => {
  const handle = 'right-line-8,19'
  const model = buildSymbolPresentation({ ...base, detail, connectedOnly: true, connected: [handle], height: 600 })
  expect(model.areas.filter(area => area.connected).map(area => area.symbol.name)).toEqual(['save', 'find'])
  expect(model.endpoints.has(handle)).toBe(true)
  expect(model.endpoints.get(handle)?.y).toBe(model.endpoints.get('right-line-8')?.y)
  expect(model.endpoints.get('right-line-19')?.connected).toBe(true)
  const hidden = buildSymbolPresentation({ ...base, detail, connectedOnly: true, connected: [handle], connectionCounts: { [handle]: 0 } })
  expect(hidden.areas.filter(area => area.connected)).toHaveLength(0)
})
