import { describe, expect, it, vi } from 'vitest'
import { foldBannerBounds } from '../../utils/subElementSymbols'
import { drawSubElement, subNodeSymbols, scrollSubElement, subNodeHandleAnchor, subSymbolAtPoint, revealSubOverflow } from './subElements'
import * as subSources from '../../utils/subElementSource'
import { symbolHandle } from '../../utils/subElements'
import type { LayoutNode } from './types'

vi.mock('../../utils/subElementSource', () => ({
  peekSubSource: () => ({ status: 'ready', lines: ['class Checkout {}'], tokens: [], symbols: [{ name: 'Checkout', type: 'class', startLine: 1, endLine: 1 }], startLine: 1, endLine: 1 }),
  requestSubSource: vi.fn(),
}))

const node: LayoutNode = {
  id: 'code', elementId: 1, diagramId: 1, worldX: 0, worldY: 0, worldW: 520, worldH: 360,
  label: 'Checkout', type: 'class', subKind: 'code', subDetail: 'code',
  sourceLink: { repo: 'example/repo', file_path: 'checkout.ts' },
  logoUrl: null, description: null, technology: 'TypeScript', tags: [], ancestorElementIds: [], pathElementIds: [1],
  children: [], childScale: 1, childOffsetX: 0, childOffsetY: 0, edgesOut: [],
}

describe('Explore source cards', () => {
  it.each([1, 2, 8])('caps header and body text at zoom %s', zoom => {
    const text: { value: string; screenFont: number }[] = []
    // Capture actual canvas font assignments to cover both the header and code rows.
    const context = {
      font: '12px sans-serif',
      fillText(value: string) { text.push({ value, screenFont: Number(this.font.match(/([\d.]+)px/)![1]) * zoom }) },
      measureText(value: string) { return { width: value.length * Number(this.font.match(/([\d.]+)px/)![1]) * 0.6 } },
      save() {}, restore() {}, beginPath() {}, roundRect() {}, fill() {}, stroke() {}, clip() {}, fillRect() {}, rect() {},
    }
    drawSubElement(context as unknown as CanvasRenderingContext2D, node, zoom, 1, '#63b3ed', false)
    expect(text.find(item => item.value === 'Checkout')!.screenFont).toBeLessThanOrEqual(20)
    expect(text.find(item => item.value === 'CLASS')!.screenFont).toBeLessThanOrEqual(12)
    expect(text.find(item => item.value === 'class Checkout {}')!.screenFont).toBeLessThanOrEqual(14)
    expect(subNodeSymbols(node, zoom).fontSize * zoom).toBeLessThanOrEqual(14)
  })
})

it.each([0.1, 1, 4])('draws complete border ports outside the content clip at zoom %s', zoom => {
  let clipped = false
  const stack: boolean[] = []
  const ports: boolean[] = []
  const context = {
    font: '12px sans-serif',
    save() { stack.push(clipped) }, restore() { clipped = stack.pop()! }, clip() { clipped = true },
    arc(_x: number, _y: number, _radius: number, start: number, end: number) {
      expect(end - start).toBe(Math.PI * 2)
      ports.push(clipped)
    },
    measureText(value: string) { return { width: value.length * 6 } },
    fillText() {}, beginPath() {}, roundRect() {}, fill() {}, stroke() {}, fillRect() {}, rect() {}, moveTo() {}, lineTo() {}, setLineDash() {},
  }
  drawSubElement(context as unknown as CanvasRenderingContext2D, { ...node, subConnectedHandles: ['right-line-1'] }, zoom, 0.8, '#63b3ed', false)
  expect(ports.length).toBeGreaterThan(0)
  expect(ports.every(underClip => !underClip)).toBe(true)
  expect(stack).toHaveLength(0)
})

it.each([1, 4])('centers the editor-style icon and text stack at zoom %s', zoom => {
  const text: { value: string; x: number; y: number; align: string; color: string }[] = []
  const context = {
    font: '12px sans-serif', textAlign: '', fillStyle: '',
    fillText(value: string, x: number, y: number) { text.push({ value, x, y, align: this.textAlign, color: this.fillStyle }) },
    measureText(value: string) { return { width: value.length * 6 } }, drawImage: vi.fn(),
    save() {}, restore() {}, beginPath() {}, roundRect() {}, fill() {}, stroke() {}, clip() {}, fillRect() {}, rect() {},
  }
  const icon = { naturalWidth: 20, naturalHeight: 28 } as HTMLImageElement
  drawSubElement(context as unknown as CanvasRenderingContext2D, node, zoom, 1, '#63b3ed', false, undefined, undefined, icon)
  const title = text.find(row => row.value === 'Checkout')!
  const kind = text.find(row => row.value === 'CLASS')!
  const metadata = text.find(row => row.value === '[TypeScript]')!
  for (const row of [title, kind, metadata]) {
    expect(row.x).toBe(node.worldW / 2)
    expect(row.align).toBe('center')
  }
  expect(kind.color).toBe('#38b2ac')
  expect(title.y).toBeLessThan(kind.y)
  expect(kind.y).toBeLessThan(metadata.y)
  const [, x, y, width, height] = context.drawImage.mock.calls[0]
  expect(x + width / 2).toBe(node.worldW / 2)
  expect(y + height).toBeLessThan(title.y)
  expect(Math.max(width, height) * zoom).toBeLessThanOrEqual(32)
})

it('keeps summary labels readable before outline detail appears', () => {
  const model = subNodeSymbols({ ...node, subDetail: 'summary' }, 0.5)
  expect(model.areas[0].signature?.text).toBe('Checkout')
  expect(model.fontSize * 0.5).toBeGreaterThanOrEqual(9)
  expect(model.fontSize * 0.5).toBeLessThanOrEqual(14)
})

it('renders connected shapes and ports without any text in the structure stage', () => {
  const context = {
    font: '12px sans-serif', fillText: vi.fn(), roundRect: vi.fn(), arc: vi.fn(),
    measureText(value: string) { return { width: value.length * 6 } },
    save() {}, restore() {}, beginPath() {}, fill() {}, stroke() {}, clip() {}, fillRect() {}, rect() {}, moveTo() {}, lineTo() {}, setLineDash() {},
  }
  drawSubElement(context as unknown as CanvasRenderingContext2D, { ...node, subDetail: 'structure', subConnectedOnly: true, subConnectedHandles: ['right-line-1'] }, 0.25, 1, '#63b3ed', false)
  expect(context.fillText).not.toHaveBeenCalled()
  expect(context.roundRect.mock.calls.length).toBeGreaterThan(1)
  expect(context.arc).toHaveBeenCalled()
})

it('keeps lower-zoom summary names readable', () => {
  const model = subNodeSymbols({ ...node, subDetail: 'summary' }, 0.35)
  expect(model.fontSize * 0.35).toBeGreaterThanOrEqual(9)
})

it.each([0.25, 0.4, 1, 2])('matches structure and summary geometry at zoom %s', zoom => {
  const sourceNode = { ...node, subConnectedOnly: true, subConnectedHandles: ['right-line-1'] }
  const structure = subNodeSymbols({ ...sourceNode, subDetail: 'structure' }, zoom)
  const summary = subNodeSymbols({ ...sourceNode, subDetail: 'summary' }, zoom)
  expect(structure).toEqual(summary)
  // The larger layouts retain their own signature/code bounds and spacing.
  expect(subNodeSymbols({ ...sourceNode, subDetail: 'outline' }, zoom).rowHeight).not.toBe(summary.rowHeight)
  expect(subNodeSymbols({ ...sourceNode, subDetail: 'code' }, zoom).areas[0].width).not.toBe(summary.areas[0].width)
})

it('draws identical highlighted shapes and ports before and after summary text is revealed', () => {
  const context = () => ({
    font: '12px sans-serif', roundRect: vi.fn(), arc: vi.fn(), fillText: vi.fn(),
    measureText(value: string) { return { width: value.length * 6 } },
    save() {}, restore() {}, beginPath() {}, fill() {}, stroke() {}, clip() {}, fillRect() {}, rect() {}, moveTo() {}, lineTo() {}, setLineDash() {},
  })
  const sourceNode = { ...node, subConnectedOnly: true, subConnectedHandles: ['right-line-1'] }
  const structure = context(), summary = context()
  drawSubElement(structure as unknown as CanvasRenderingContext2D, { ...sourceNode, subDetail: 'structure' }, 0.4, 1, '#63b3ed', false)
  drawSubElement(summary as unknown as CanvasRenderingContext2D, { ...sourceNode, subDetail: 'summary' }, 0.4, 1, '#63b3ed', false)
  expect(structure.roundRect.mock.calls).toEqual(summary.roundRect.mock.calls)
  expect(structure.arc.mock.calls).toEqual(summary.arc.mock.calls)
  expect(structure.fillText).not.toHaveBeenCalled()
  expect(summary.fillText).toHaveBeenCalled()
})

it('shares scroll position only between structure and summary', () => {
  const symbols = Array.from({ length: 10 }, (_, index) => ({ name: `fn${index}`, type: 'function', startLine: index + 1, endLine: index + 1 }))
  const spy = vi.spyOn(subSources, 'peekSubSource').mockReturnValue({ status: 'ready', symbols, lines: symbols.map(symbol => `function ${symbol.name}() {}`), tokens: [], startLine: 1, endLine: 10 })
  try {
    const sourceNode = { ...node, id: 'shared-summary-scroll', worldH: 180, subConnectedOnly: true, subConnectedHandles: symbols.map(symbol => symbolHandle('right', symbol)) }
    expect(scrollSubElement({ ...sourceNode, subDetail: 'structure' }, 19, 1)).toBe(true)
    const structure = subNodeSymbols({ ...sourceNode, subDetail: 'structure' }, 1)
    const summary = subNodeSymbols({ ...sourceNode, subDetail: 'summary' }, 1)
    expect(structure).toEqual(summary)
    expect(summary.above.areas.length).toBeGreaterThan(0)
    expect(subNodeSymbols({ ...sourceNode, subDetail: 'outline' }, 1).above.areas).toHaveLength(0)
    expect(subNodeSymbols({ ...sourceNode, subDetail: 'code' }, 1).above.areas).toHaveLength(0)
  } finally { spy.mockRestore() }
})

it.each([
  { zoom: 0.05, lowDetail: false },
  { zoom: 0.5, lowDetail: false },
  { zoom: 1, lowDetail: true },
  { zoom: 4, lowDetail: false },
])('keeps a complete 1.5-pixel border above content at zoom $zoom (low detail: $lowDetail)', ({ zoom, lowDetail }) => {
  let clipped = false
  const stack: boolean[] = []
  const paints: { type: string; width?: number; clipped?: boolean }[] = []
  const context = {
    font: '12px sans-serif', lineWidth: 0,
    save() { stack.push(clipped) }, restore() { clipped = stack.pop()! }, clip() { clipped = true },
    fillRect() { paints.push({ type: 'content' }) },
    stroke() { paints.push({ type: 'border', width: this.lineWidth * zoom, clipped }) },
    measureText(value: string) { return { width: value.length * 6 } },
    fillText() {}, beginPath() {}, roundRect() {}, fill() {}, rect() {},
  }
  drawSubElement(context as unknown as CanvasRenderingContext2D, node, zoom, 1, '#63b3ed', lowDetail)
  expect(paints.at(-1)).toEqual({ type: 'border', width: 1.5, clipped: false })
  expect(paints.filter(paint => paint.type === 'border')).toHaveLength(1)
  expect(stack).toHaveLength(0)
})

it.each(['structure', 'summary'] as const)('keeps %s row positions continuous across a font-rounding boundary', subDetail => {
  const symbols = Array.from({ length: 30 }, (_, index) => ({ name: `fn${index}`, type: 'function', startLine: index + 1, endLine: index + 1 }))
  const sourceNode: LayoutNode = { ...node, id: `smooth-${subDetail}`, worldH: 5000, subDetail,
    subSource: { status: 'ready', symbols, lines: symbols.map(symbol => `function ${symbol.name}() {}`), tokens: [], startLine: 1, endLine: 30 } }
  const boundary = 27 / 49 // 9 / zoom * 1.5 crosses the old 24.5 rounding boundary.
  const before = subNodeSymbols(sourceNode, boundary - 0.000001)
  const after = subNodeSymbols(sourceNode, boundary + 0.000001)
  expect(Math.abs(after.rowHeight - before.rowHeight)).toBeLessThan(0.001)
  expect(Math.abs(after.areas[25].y - before.areas[25].y)).toBeLessThan(0.01)
  const handle = symbolHandle('right', symbols[25])
  expect(Math.abs(after.endpoints.get(handle)!.y - before.endpoints.get(handle)!.y)).toBeLessThan(0.01)
})

it.each([1.41, 1.45, 1.47, 1.52, 2.01])('keeps capped code rows at a smooth screen height at zoom %s', zoom => {
  const model = subNodeSymbols(node, zoom)
  expect(model.fontSize * zoom).toBeCloseTo(14)
  expect(model.rowHeight * zoom).toBeCloseTo(21)
})

it('preserves fractional metrics after scrolling', () => {
  const symbols = Array.from({ length: 30 }, (_, index) => ({ name: `fn${index}`, type: 'function', startLine: index + 1, endLine: index + 1 }))
  const sourceNode: LayoutNode = { ...node, id: 'smooth-scrolled-summary', subDetail: 'summary', worldH: 180,
    subSource: { status: 'ready', symbols, lines: symbols.map(symbol => `function ${symbol.name}() {}`), tokens: [], startLine: 1, endLine: 30 } }
  expect(scrollSubElement(sourceNode, 20, 0.55)).toBe(true)
  const model = subNodeSymbols(sourceNode, 0.55)
  expect(model.rowHeight).toBeCloseTo(model.fontSize * 1.5 + 4)
  expect(Number.isInteger(model.rowHeight)).toBe(false)
})


it('draws the repository diff with translucent line borders, no highlight fill, and both revisions’ line numbers', () => {
  const text: string[] = [], backgrounds: string[] = [], borders: string[] = []
  const context = {
    font: '12px sans-serif', fillStyle: '', strokeStyle: '',
    fillText(value: string) { text.push(value) },
    fillRect() { backgrounds.push(this.fillStyle) },
    strokeRect() { borders.push(this.strokeStyle) },
    measureText(value: string) { return { width: value.length * 7 } },
    save() {}, restore() {}, beginPath() {}, roundRect() {}, fill() {}, stroke() {}, clip() {}, rect() {},
  }
  const diffNode = { ...node, subSource: { status: 'ready' as const, startLine: 1, endLine: 1, lines: ['new'], tokens: [], symbols: [],
    diff: { side: 'head' as const, baseTokens: [], headTokens: [], lines: [
      { change: 'removed', oldLine: 1, newLine: 0, text: 'old' },
      { change: 'added', oldLine: 0, newLine: 1, text: 'new' },
    ] } } }
  drawSubElement(context as unknown as CanvasRenderingContext2D, diffNode, 1, 1, '#63b3ed', false)
  expect(text).toContain('GIT DIFF · − BASE / + HEAD')
  expect(text).toContain('old'); expect(text).toContain('new')
  expect(text).toContain('−'); expect(text).toContain('+')
  expect(text.filter(value => value === '1')).toHaveLength(2)
  expect(borders).toEqual(['rgba(248,81,73,0.30)', 'rgba(46,160,67,0.30)'])
  expect(backgrounds).toEqual(['#2d3748'])
})

it('keeps compact source ports and symbol picking aligned with the zoomed preview', () => {
  const compact = { ...node, worldW: 180, worldH: 85 }
  const scale = 180 / 520
  const expanded = { ...node, worldH: 85 / scale }
  const handle = symbolHandle('right', { name: 'Checkout', type: 'class', startLine: 1, endLine: 1 })
  const zoom = 3.4
  const expected = subNodeHandleAnchor(expanded, handle, zoom * scale)!
  const actual = subNodeHandleAnchor(compact, handle, zoom)!
  expect(actual.x).toBeCloseTo(expected.x * scale)
  expect(actual.y).toBeCloseTo(expected.y * scale)
  const area = subNodeSymbols(expanded, zoom * scale).areas[0]
  const x = (area.x + area.width / 2) * scale
  const y = (84 + area.contentTop + area.height / 2) * scale
  expect(subSymbolAtPoint(compact, zoom, x, y)).toBe(area.key)
  expect(subNodeHandleAnchor({ ...compact, subDetail: undefined }, handle, 1)).toBeNull()
})


it.each([1, 2, 3.3])('keeps fold banners readable at zoom %s and reveals the clicked range', zoom => {
  const sourceNode: LayoutNode = { ...node, id: `connected-fold-banner-${zoom}`, subConnectedOnly: true, subConnectedHandles: ['right-line-10'],
    subSource: { status: 'ready', startLine: 1, endLine: 30, lines: Array<string>(30).fill('source line'), symbols: [], tokens: [] },
  }
  const labels: number[] = []
  const text: string[] = [], bands: { x: number; width: number }[] = []
  const context = {
    font: '12px sans-serif', textAlign: '', fillStyle: '',
    fillText(value: string) { text.push(value); if (value.includes('hidden')) labels.push(Number(this.font.match(/([\d.]+)px/)![1]) * zoom) }, fillRect(x: number, _y: number, width: number) { bands.push({ x, width }) },
    measureText(value: string) { return { width: value.length * 6 } },
    save() {}, restore() {}, beginPath() {}, roundRect() {}, fill() {}, stroke() {}, clip() {}, rect() {}, moveTo() {}, lineTo() {}, arc() {}, setLineDash() {}, quadraticCurveTo() {},
  }
  drawSubElement(context as unknown as CanvasRenderingContext2D, sourceNode, zoom, 1, '#63b3ed', false)
  expect(text).toContain('7 lines hidden')
  expect(labels.every(size => size >= 11 && size <= 12.001)).toBe(true)
  expect(text.some(value => value.includes('collapsed'))).toBe(false)
  expect(bands.some(band => band.x === 0 && band.width === 520)).toBe(true)
  const before = subNodeSymbols(sourceNode, zoom)
  expect(revealSubOverflow(sourceNode, zoom, before.headerHeight + 0.5)).toBe(true)
  const after = subNodeSymbols(sourceNode, zoom)
  expect(after.rows.filter(row => row.collapsed)).toHaveLength(1)
  expect(after.rows.filter(row => !row.collapsed).map(row => row.sourceIndex)).toEqual(Array.from({ length: 12 }, (_, index) => index))
  expect(sourceNode.subConnectedOnly).toBe(true)
})


it('uses the fold banner height for painting and clicking both viewport banners', () => {
  const sourceNode: LayoutNode = { ...node, id: 'matching-viewport-banners', worldH: 160,
    subSource: { status: 'ready', startLine: 1, endLine: 30, lines: Array<string>(30).fill('source line'),
      symbols: [{ name: 'first', startLine: 1, endLine: 1 }, { name: 'last', startLine: 30, endLine: 30 }], tokens: [] },
    subConnectedHandles: ['right-line-1', 'right-line-30'],
  }
  scrollSubElement(sourceNode, 100, 1)
  const model = subNodeSymbols(sourceNode, 1), height = foldBannerBounds(model, 0).height
  const bands: { y: number; height: number }[] = []
  const context = {
    font: '12px sans-serif', fillText() {},
    fillRect(x: number, y: number, width: number, height: number) { if (x === 0 && width === sourceNode.worldW) bands.push({ y, height }) },
    measureText(value: string) { return { width: value.length * 6 } },
    save() {}, restore() {}, beginPath() {}, roundRect() {}, fill() {}, stroke() {}, clip() {}, rect() {}, moveTo() {}, lineTo() {}, arc() {}, setLineDash() {}, quadraticCurveTo() {},
  }
  drawSubElement(context as unknown as CanvasRenderingContext2D, sourceNode, 1, 1, '#63b3ed', false)
  expect(bands).toContainEqual({ y: model.headerHeight, height })
  expect(bands).toContainEqual({ y: sourceNode.worldH - height, height })
  expect(revealSubOverflow(sourceNode, 1, sourceNode.worldH - height + 0.25)).toBe(true)
  expect(subNodeSymbols(sourceNode, 1).areas[1].visible).toBe(true)
  expect(revealSubOverflow(sourceNode, 1, model.headerHeight + height - 0.25)).toBe(true)
  expect(subNodeSymbols(sourceNode, 1).areas[0].visible).toBe(true)
})
