import { useLayoutEffect, useSyncExternalStore } from 'react'

interface LabelRect {
  left: number
  top: number
  right: number
  bottom: number
}

interface LabelEntry {
  id: string
  preferredX: number
  preferredY: number
  width: number
  height: number
  dx: number
  dy: number
  alongLineOnly?: boolean
  position: { x: number; y: number }
}

const entries = new Map<string, LabelEntry>()
const fallbackPositions = new Map<string, { x: number; y: number }>()
const listeners = new Set<() => void>()
let measureCtx: CanvasRenderingContext2D | null = null

function getMeasureContext() {
  if (measureCtx) return measureCtx
  if (typeof document === 'undefined') return null
  measureCtx = document.createElement('canvas').getContext('2d')
  return measureCtx
}

export function measureEdgeLabel(text: string, font: string) {
  const ctx = getMeasureContext()
  if (!ctx) return Math.max(8, text.length * 7)
  ctx.font = font
  return ctx.measureText(text).width
}

function emit() {
  listeners.forEach((listener) => listener())
}

function rectsOverlap(a: LabelRect, b: LabelRect) {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
}

function buildRect(centerX: number, centerY: number, width: number, height: number, gap = 6): LabelRect {
  return {
    left: centerX - width / 2 - gap,
    top: centerY - height / 2 - gap / 2,
    right: centerX + width / 2 + gap,
    bottom: centerY + height / 2 + gap / 2,
  }
}

function pickPosition(entry: LabelEntry, occupiedRects: LabelRect[]) {
  const step = entry.height + 8
  const length = Math.hypot(entry.dx, entry.dy) || 1
  const tangentX = entry.dx / length
  const tangentY = entry.dy / length
  const normalX = -entry.dy / length
  const normalY = entry.dx / length

  const candidateOffsets = entry.alongLineOnly
    ? [
        { x: 0, y: 0 },
        { x: tangentX * step, y: tangentY * step },
        { x: -tangentX * step, y: -tangentY * step },
        { x: tangentX * step * 2, y: tangentY * step * 2 },
        { x: -tangentX * step * 2, y: -tangentY * step * 2 },
        { x: tangentX * step * 3, y: tangentY * step * 3 },
        { x: -tangentX * step * 3, y: -tangentY * step * 3 },
      ]
    : [
        { x: 0, y: 0 },
        { x: normalX * step, y: normalY * step },
        { x: -normalX * step, y: -normalY * step },
        { x: normalX * step * 2, y: normalY * step * 2 },
        { x: -normalX * step * 2, y: -normalY * step * 2 },
        { x: tangentX * step, y: tangentY * step },
        { x: -tangentX * step, y: -tangentY * step },
      ]

  for (const offset of candidateOffsets) {
    const x = entry.preferredX + offset.x
    const y = entry.preferredY + offset.y
    const rect = buildRect(x, y, entry.width, entry.height)
    if (occupiedRects.some((existing) => rectsOverlap(rect, existing))) continue
    occupiedRects.push(rect)
    return { x, y }
  }

  occupiedRects.push(buildRect(entry.preferredX, entry.preferredY, entry.width, entry.height))
  return { x: entry.preferredX, y: entry.preferredY }
}

function recomputeLayout() {
  const occupiedRects: LabelRect[] = []
  let changed = false
  const sortedEntries = Array.from(entries.values()).sort((left, right) => left.id.localeCompare(right.id))

  for (const entry of sortedEntries) {
    const nextPosition = pickPosition(entry, occupiedRects)
    if (entry.position.x !== nextPosition.x || entry.position.y !== nextPosition.y) {
      entry.position = nextPosition
      changed = true
    }
  }

  if (changed) emit()
}

function upsertEntry(entry: Omit<LabelEntry, 'position'>) {
  const existing = entries.get(entry.id)
  entries.set(entry.id, {
    ...entry,
    position: existing?.position ?? { x: entry.preferredX, y: entry.preferredY },
  })
  recomputeLayout()
}

function removeEntry(id: string) {
  if (!entries.delete(id)) return
  fallbackPositions.delete(id)
  recomputeLayout()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function getPosition(id: string, fallbackX: number, fallbackY: number) {
  const entry = entries.get(id)
  if (entry) return entry.position
  // Cache the fallback so useSyncExternalStore gets a stable reference and
  // doesn't detect a spurious change on every call before the entry is added.
  let cached = fallbackPositions.get(id)
  if (!cached || cached.x !== fallbackX || cached.y !== fallbackY) {
    cached = { x: fallbackX, y: fallbackY }
    fallbackPositions.set(id, cached)
  }
  return cached
}

interface UseEdgeLabelLayoutArgs {
  id: string
  preferredX: number
  preferredY: number
  width: number
  height: number
  dx: number
  dy: number
  alongLineOnly?: boolean
}

export function useEdgeLabelLayout({
  id,
  preferredX,
  preferredY,
  width,
  height,
  dx,
  dy,
  alongLineOnly,
}: UseEdgeLabelLayoutArgs) {
  useLayoutEffect(() => {
    upsertEntry({
      id,
      preferredX,
      preferredY,
      width,
      height,
      dx,
      dy,
      alongLineOnly,
    })

    return () => removeEntry(id)
  }, [id, preferredX, preferredY, width, height, dx, dy, alongLineOnly])

  return useSyncExternalStore(
    subscribe,
    () => getPosition(id, preferredX, preferredY),
    () => ({ x: preferredX, y: preferredY }),
  )
}
