import { afterEach, expect, it, vi } from 'vitest'
import { parseNumericId } from './ids'
afterEach(() => vi.unstubAllGlobals())
it('accepts plan-local aliases only in a planning editor', () => {
  vi.stubGlobal('window', { location: { pathname: '/views/1', search: '' } })
  expect(parseNumericId('-42')).toBeNull()
  vi.stubGlobal('window', { location: { pathname: '/views/1', search: '?plan=draft' } })
  expect(parseNumericId('-42')).toBe(-42)
  expect(parseNumericId('0')).toBeNull()
  expect(parseNumericId('12')).toBe(12)
})
