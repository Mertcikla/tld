export interface SourcePreviewSize { width: number; height: number }
const sizes = new Map<string, SourcePreviewSize | null>()
const listeners = new Set<() => void>()
let revision = 0
const keyFor = (viewId: number, elementId: number) => `tld:source-preview-size:${viewId}:${elementId}`
export const subscribeSourcePreviewSizes = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }
export const sourcePreviewSizeRevision = () => revision

export function readSourcePreviewSize(viewId: number, elementId: number): SourcePreviewSize | null {
  const key = keyFor(viewId, elementId)
  if (sizes.has(key)) return sizes.get(key)!
  let size: SourcePreviewSize | null = null
  try {
    const value = JSON.parse(localStorage.getItem(key) || 'null')
    if (value && Number.isFinite(value.width) && Number.isFinite(value.height) && value.width >= 280 && value.width <= 2400 && value.height >= 160 && value.height <= 2400) size = { width: value.width, height: value.height }
  } catch { /* Preview preferences are optional in embedded viewers. */ }
  sizes.set(key, size)
  return size
}

export function writeSourcePreviewSize(viewId: number, elementId: number, size: SourcePreviewSize | null) {
  const key = keyFor(viewId, elementId)
  sizes.set(key, size)
  try { if (size) localStorage.setItem(key, JSON.stringify(size)); else localStorage.removeItem(key) } catch { /* Keep resizing usable without browser storage. */ }
  revision++
  listeners.forEach(listener => listener())
}
