import { describe, expect, it } from 'vitest'
import { ensureVisualHandleId, getHandleSlotOffsetFromId } from './edgeDistribution'
import { isSubElement, isSubHandle, lineHandle, memberHandle, subDetail, subEditorSize, subElementKind, subHandleY, subSize, symbolHandle } from './subElements'

it('enables source layouts for repo files and symbols without changing their authored kind', () => {
  const element = { kind: 'service', repo: 'owner/repo', file_path: 'src/api.ts#function_declaration:find' }
  expect(subElementKind(element)).toBe('code')
  expect(element.kind).toBe('service')
  expect(subElementKind({ repository_id: 'indexed', file_path: 'schema.sql#L2-L5' })).toBe('code')
  expect(subElementKind({ ...element, file_path: 'src/' })).toBeNull()
  expect(subElementKind({ ...element, file_path: null })).toBeNull()
  expect(subElementKind({ file_path: 'users.ts' })).toBeNull()
  expect(subElementKind({ ...element, kind: 'class' })).toBe('code')
  expect(subElementKind({ kind: 'class' })).toBeNull()
})

describe('sub-element geometry and semantic handles', () => {
  const detail = { width: 600, height: 420 }
  it('preserves existing card dimensions and bounds optional sizes', () => {
    expect(subSize('service', detail)).toEqual({ width: 180, height: 85 })
    expect(subSize('code', { ...detail, width: NaN, height: -1 })).toEqual({ width: 520, height: 360 })
    expect(subSize('class', detail)).toEqual({ width: 180, height: 85 })
    expect(isSubElement('component')).toBe(false)
  })
  it('retains legacy member handle IDs without editable member records', () => {
    const handle = memberHandle('right', 'save/user')
    expect(isSubHandle(handle)).toBe(true)
    expect(ensureVisualHandleId(handle, 'left')).toBe(handle)
    expect(getHandleSlotOffsetFromId(handle)).toBe(0)
    expect(subHandleY(handle, detail)).toBe(96)
  })
  it('anchors lines relative to the source range and clamps missing endpoints', () => {
    expect(subHandleY(lineHandle('left', 12), null, 12)).toBe(96)
    expect(subHandleY(lineHandle('right', 15), null, 12)).toBe(168)
    expect(subHandleY(lineHandle('right', 1000), null, 12, 360)).toBe(352)
  })
  it('switches detail in discrete steps', () => {
    expect([[280, 160], [400, 240], [600, 360]].map(([width, height]) => subDetail(width, height))).toEqual(['overview', 'outline', 'code'])
    expect(subDetail(1000, 150)).toBe('overview')
    expect(subDetail(600, 240)).toBe('outline')
  })
})

it('keeps symbol connections attached when source lines move', () => {
  const symbol = { name: 'find', qualifiedName: 'User.find', type: 'method', startLine: 12, endLine: 15 }
  const handle = symbolHandle('right', symbol)
  expect(ensureVisualHandleId(handle, 'left')).toBe(handle)
  expect(subHandleY(handle, null, 12, 360, [symbol])).toBe(96)
  expect(subHandleY(handle, null, 12, 360, [{ ...symbol, startLine: 13, endLine: 16 }])).toBe(120)
})

it('keeps saved dimensions stable as an element changes detail with screen size', () => {
  const config = { width: 720, height: 600 }
  const size = subSize('code', config)
  expect([0.3, 0.5, 1].map(zoom => subDetail(size.width * zoom, size.height * zoom))).toEqual(['overview', 'outline', 'code'])
  expect(size).toEqual(config)
})

it('keeps cards fixed and preserves preview sizes independently of layout', () => {
  const config = { width: 900, height: 700 }
  expect(subEditorSize('code', config, 'overview')).toEqual({ width: 180, height: 85 })
  expect(subEditorSize('code', config, 'outline')).toEqual(config)
  expect(subEditorSize('code', config, 'code')).toEqual(config)
  expect(config).toEqual({ width: 900, height: 700 })
})

it('folds nested symbols and anchors their endpoints to the enclosing row', async () => {
  const { collapsedSymbols, outlineHandleY } = await import('./subElements')
  const outer = { name: 'User', type: 'class', startLine: 2, endLine: 15 }
  const method = { name: 'save', qualifiedName: 'User.save', type: 'method', startLine: 8, endLine: 12 }
  const fn = { name: 'find', type: 'function', startLine: 17, endLine: 20 }
  const source = { status: 'ready', startLine: 1, endLine: 20, symbols: [method, fn, outer, outer] }
  const rows = collapsedSymbols(source, 'users.ts')
  expect(rows).toEqual([outer, fn])
  expect(outlineHandleY(symbolHandle('right', method), rows, source.symbols, 164)).toBe(96)
  expect(outlineHandleY(lineHandle('left', 18), rows, source.symbols, 164)).toBe(120)
  expect(collapsedSymbols({ ...source, symbols: [] }, 'users.sql')).toEqual([{ name: 'users.sql', startLine: 1, endLine: 20 }])
})
