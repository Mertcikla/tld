import { expect, it, vi } from 'vitest'
import { readSourcePreviewSize, writeSourcePreviewSize, subscribeSourcePreviewSizes } from './sourcePreviewSize'

it('stores preview size as a browser preference and restores an unset preference on undo', () => {
  const storage = new Map<string, string>()
  vi.stubGlobal('localStorage', { getItem: (key: string) => storage.get(key), setItem: (key: string, value: string) => storage.set(key, value), removeItem: (key: string) => storage.delete(key) })
  try {
    const listener = vi.fn(), stop = subscribeSourcePreviewSizes(listener)
    expect(readSourcePreviewSize(901, 902)).toBeNull()
    writeSourcePreviewSize(901, 902, { width: 600, height: 400 })
    expect(readSourcePreviewSize(901, 902)).toEqual({ width: 600, height: 400 })
    expect(JSON.parse(storage.get('tld:source-preview-size:901:902')!)).toEqual({ width: 600, height: 400 })
    writeSourcePreviewSize(901, 902, null)
    expect(readSourcePreviewSize(901, 902)).toBeNull()
    expect(listener).toHaveBeenCalledTimes(2)
    stop()
  } finally { vi.unstubAllGlobals() }
})
