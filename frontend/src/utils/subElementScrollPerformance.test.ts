import { expect, it, vi } from 'vitest'
import { buildSymbolPresentation } from './subElementSymbols'
import type { SubSource } from './subElementSource'
import * as textMetrics from './subElementText'

it('reuses document measurements while scrolling a large Git diff', () => {
  const lines = Array.from({ length: 8000 }, (_, index) => `const value${index} = 'a representative source line';`)
  const source: SubSource = { status: 'ready', lines, tokens: [], startLine: 1, endLine: lines.length,
    symbols: Array.from({ length: 200 }, (_, index) => ({ name: `symbol${index}`, startLine: index * 40 + 1, endLine: (index + 1) * 40 })),
    diff: { side: 'head', baseTokens: [], headTokens: [], lines: lines.map((text, index) => ({ change: 'unchanged', oldLine: index + 1, newLine: index + 1, text })) },
  }
  const options = { kind: 'code', source, detail: 'code' as const, width: 520, height: 360,
    connected: Array.from({ length: 100 }, (_, index) => `left-line-${index * 50 + 1}`) }
  const first = buildSymbolPresentation(options)
  const measure = vi.spyOn(textMetrics, 'measureSubText')
  try {
    let last = first
    for (let frame = 1; frame <= 30; frame++) last = buildSymbolPresentation({ ...options, scrollTop: frame * 18 })
    expect(measure).not.toHaveBeenCalled()
    expect(last.rows).toBe(first.rows)
    expect(last.contentWidth).toBe(first.contentWidth)
    expect(last.endpoints.get('left-line-1')?.y).not.toBe(first.endpoints.get('left-line-1')?.y)
  } finally { measure.mockRestore() }
})


it('refreshes cached diff geometry when captured lines or font size change', () => {
  const source: SubSource = { status: 'ready', lines: ['old'], tokens: [], symbols: [], startLine: 1, endLine: 1,
    diff: { side: 'head', baseTokens: [], headTokens: [], lines: [{ change: 'added', oldLine: 0, newLine: 1, text: 'old' }] } }
  const options = { kind: 'code', source, detail: 'code' as const, width: 280, height: 160 }
  const first = buildSymbolPresentation(options)
  source.diff = { ...source.diff!, lines: [{ change: 'added', oldLine: 0, newLine: 1, text: 'a much longer replacement line '.repeat(20) }] }
  const changed = buildSymbolPresentation(options)
  expect(changed.rows[0].diff?.text).toContain('replacement line')
  expect(changed.contentWidth).toBeGreaterThan(first.contentWidth)
  const largerFont = buildSymbolPresentation({ ...options, fontSize: 24 })
  expect(largerFont.rowHeight).toBeGreaterThan(changed.rowHeight)
  expect(largerFont.contentWidth).toBeGreaterThan(changed.contentWidth)
  expect(buildSymbolPresentation({ ...options, scrollTop: 50 }).contentWidth).toBe(changed.contentWidth)
})
