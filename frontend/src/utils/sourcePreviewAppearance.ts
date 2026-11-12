export interface SourcePreviewAppearance { fontSize: number; lineNumbers?: boolean; connectedOnly?: boolean }
const defaults: SourcePreviewAppearance = { fontSize: 10 }
const preferences = new Map<string, SourcePreviewAppearance>()
const listeners = new Set<() => void>()
let revision = 0
const keyFor = (viewId: number, elementId: number) => `tld:source-preview-appearance:${viewId}:${elementId}`
export const subscribeSourcePreviewAppearance = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }
export const sourcePreviewAppearanceRevision = () => revision
export function readSourcePreviewAppearance(viewId: number, elementId: number): SourcePreviewAppearance {
  const key = keyFor(viewId, elementId)
  if (preferences.has(key)) return preferences.get(key)!
  let result = defaults
  try {
    const value = JSON.parse(localStorage.getItem(key) || 'null')
    if (value) result = { fontSize: Number.isFinite(value.fontSize) ? Math.max(8, Math.min(14, Math.round(value.fontSize))) : 10,
      lineNumbers: typeof value.lineNumbers === 'boolean' ? value.lineNumbers : undefined,
      connectedOnly: typeof value.connectedOnly === 'boolean' ? value.connectedOnly : undefined }
  } catch { /* Browser preferences are optional. */ }
  preferences.set(key, result)
  return result
}
export function writeSourcePreviewAppearance(viewId: number, elementId: number, value: SourcePreviewAppearance) {
  const key = keyFor(viewId, elementId)
  const next = { ...value, fontSize: Number.isFinite(value.fontSize) ? Math.max(8, Math.min(14, Math.round(value.fontSize))) : 10 }
  preferences.set(key, next)
  try { localStorage.setItem(key, JSON.stringify(next)) } catch { /* Keep controls usable without storage. */ }
  revision++
  listeners.forEach(listener => listener())
}
