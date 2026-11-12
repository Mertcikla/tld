import { afterEach, expect, it, vi } from 'vitest'
import { readSourcePreviewAppearance, subscribeSourcePreviewAppearance, writeSourcePreviewAppearance } from './sourcePreviewAppearance'

afterEach(() => vi.unstubAllGlobals())

it('persists per-element appearance and notifies mounted previews', () => {
  const values = new Map<string, string>()
  vi.stubGlobal('localStorage', { getItem: (key: string) => values.get(key), setItem: (key: string, value: string) => values.set(key, value) })
  const listener = vi.fn(), unsubscribe = subscribeSourcePreviewAppearance(listener)
  expect(readSourcePreviewAppearance(5000, 1)).toEqual({ fontSize: 10 })
  writeSourcePreviewAppearance(5000, 1, { fontSize: 13, lineNumbers: false })
  expect(readSourcePreviewAppearance(5000, 1)).toEqual({ fontSize: 13, lineNumbers: false })
  expect(JSON.parse(values.get('tld:source-preview-appearance:5000:1')!)).toEqual({ fontSize: 13, lineNumbers: false })
  expect(readSourcePreviewAppearance(5000, 2)).toEqual({ fontSize: 10 })
  expect(listener).toHaveBeenCalledTimes(1)
  unsubscribe()
})

it('bounds font sizes and remains usable when browser storage fails', () => {
  vi.stubGlobal('localStorage', { getItem() { throw Error('Unavailable') }, setItem() { throw Error('Unavailable') } })
  expect(readSourcePreviewAppearance(5001, 1)).toEqual({ fontSize: 10 })
  for (const [input, expected] of [[100, 14], [-1, 8], [NaN, 10]]) {
    writeSourcePreviewAppearance(5001, 1, { fontSize: input })
    expect(readSourcePreviewAppearance(5001, 1).fontSize).toBe(expected)
  }
})

it('persists and restores the connected-parts preference', () => {
  const values = new Map([['tld:source-preview-appearance:5002:1', '{"fontSize":10,"connectedOnly":true}']])
  vi.stubGlobal('localStorage', { getItem: (key: string) => values.get(key), setItem: (key: string, value: string) => values.set(key, value) })
  expect(readSourcePreviewAppearance(5002, 1).connectedOnly).toBe(true)
  writeSourcePreviewAppearance(5002, 1, { fontSize: 10, connectedOnly: false })
  expect(JSON.parse(values.get('tld:source-preview-appearance:5002:1')!).connectedOnly).toBe(false)
})
