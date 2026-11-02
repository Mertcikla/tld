import { describe, expect, it } from 'vitest'
import { buildSymbolPresentation, symbolGroupColor, symbolStems } from './subElementSymbols'
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
    expect(model.areas[0].contentTop).toBe(2)
    expect(model.areas.every(area => area.visible)).toBe(true)
    expect(symbolStems(model, 600, 135)[0]).toMatchObject({ targetY: 96, sourceY: 96 })
  })

  it('aggregates anchors beneath an active overflow band while keeping raw line connections', () => {
    const model = buildSymbolPresentation({ ...base, detail: 'outline', height: 135, scrollTop: 24, connected: [symbolHandle('right', outer)] })
    expect(model.viewportTop).toBe(100)
    expect(model.above.areas.map(area => area.symbol)).toEqual([outer, find])
    expect(model.above.representative?.visible).toBe(true)
    const lines = buildSymbolPresentation({ kind: 'code', detail: 'code', width: 400, height: 240, connected: [lineHandle('right', 50)] })
    expect(lines.below.areas).toHaveLength(0)
    expect(lines.below.connectionCount).toBe(1)
    expect(lines.viewportBottom).toBe(224)
    expect(lines.below.representative?.visible).toBe(true)
  })

  it('uses stable group colors and respects an explicit group tag color', () => {
    expect(symbolGroupColor('save')).toBe(symbolGroupColor('save'))
    expect(symbolGroupColor('save', '#123456')).toBe('#123456')
    expect(buildSymbolPresentation({ ...base, groupColor: '#123456' }).areas.every(area => area.color === '#123456')).toBe(true)
  })

  it('pads nested symbol ranges and anchors both sides to their start line', () => {
    const left = symbolHandle('left', save), right = symbolHandle('right', save)
    const model = buildSymbolPresentation({ ...base, connected: [left] })
    const area = model.areas.find(candidate => candidate.symbol.name === 'save')!
    expect(area.depth).toBe(1)
    expect(area.x).toBeGreaterThan(model.areas[0].x)
    expect(area.height).toBe(5 * 24 - 4)
    expect(area.width).toBeGreaterThan(source.lines[7].trimStart().length * 7.2)
    expect(model.endpoints.get(left)).toMatchObject({ y: 264, side: 'left', visible: true, color: area.color })
    expect(model.endpoints.get(right)).toMatchObject({ y: 264, side: 'right', visible: true, color: area.color })
  })

  it('aggregates off-screen connections without losing their IDs or counts', () => {
    const above = symbolHandle('left', outer), below = symbolHandle('right', find)
    const raw = lineHandle('left', 28)
    const model = buildSymbolPresentation({ ...base, scrollTop: 120, connected: [above, above, below, raw] })
    expect(model.above.connectionCount).toBe(2)
    expect(model.below.connectionCount).toBe(2)
    expect(model.endpoints.get(above)).toMatchObject({ y: 92, side: 'right', overflow: 'above', visible: true })
    for (const id of [below, raw]) expect(model.endpoints.get(id)).toMatchObject({ y: 352, side: 'right', overflow: 'below' })
    expect(model.below.endpoints.filter(endpoint => endpoint.visible)).toHaveLength(1)
    expect(model.endpoints.size).toBeGreaterThanOrEqual(3)
  })

  it('routes nested symbols to their enclosing collapsed row', () => {
    const id = symbolHandle('right', save)
    const model = buildSymbolPresentation({ ...base, detail: 'outline', height: 164, connected: [id] })
    expect(model.areas.map(area => area.symbol.name)).toEqual(['User', 'find'])
    expect(model.endpoints.get(id)).toMatchObject({ y: 96, area: { symbol: outer } })
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
    expect([...model.endpoints.values()].filter(endpoint => endpoint.visible)).toHaveLength(1)
    expect(model.endpoints.get(symbolHandle('right', spec))?.connected).toBe(true)
    for (const id of ids) expect(model.endpoints.get(id)?.area?.symbol).toEqual(spec)
    expect(symbolStems(model, 600, 360)).toHaveLength(1)
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
    expect(stem?.targetY).toBe(168)
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
    expect(symbolStems(model, 300, 360)).toHaveLength(0)
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
