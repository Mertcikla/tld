export type ClientPoint = {
  clientX: number
  clientY: number
}

export type SafariGestureEventLike = {
  scale?: unknown
  clientX?: unknown
  clientY?: unknown
}

export function safariGestureScale(event: SafariGestureEventLike): number | null {
  return typeof event.scale === 'number' && Number.isFinite(event.scale) && event.scale > 0
    ? event.scale
    : null
}

export function safariGestureFactor(event: SafariGestureEventLike, previousScale: number | null): { scale: number; factor: number } | null {
  const scale = safariGestureScale(event)
  if (scale === null) return null

  const baseline = previousScale !== null && Number.isFinite(previousScale) && previousScale > 0
    ? previousScale
    : scale
  const factor = scale / baseline
  return Number.isFinite(factor) && factor > 0 ? { scale, factor } : null
}

export function safariGestureClientPoint(event: SafariGestureEventLike, fallback: ClientPoint): ClientPoint {
  return {
    clientX: typeof event.clientX === 'number' && Number.isFinite(event.clientX) ? event.clientX : fallback.clientX,
    clientY: typeof event.clientY === 'number' && Number.isFinite(event.clientY) ? event.clientY : fallback.clientY,
  }
}
