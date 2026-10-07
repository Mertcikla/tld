import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
} from 'react'

/**
 * Snap thresholds shared by the resizable columns on the repositories page.
 *
 * `collapsed` — widths under this snap into the thin rail form.
 * `overlay`   — widths over this snap into the expanded, canvas-covering form.
 */
export const PANEL_COLLAPSE_SNAP = 200
export const PANEL_OVERLAY_SNAP = 650

export const PANEL_RAIL_WIDTH = 44

export type ResizableColumnZone = 'collapsed' | 'inline' | 'overlay'

export type ResizableColumnOptions = {
  /** localStorage key used to persist the chosen width. */
  storageKey: string
  defaultWidth: number
  /** Widths below this snap to the collapsed rail. */
  collapseBelow: number
  /** Widths above this snap to the overlay (canvas-covering) form. */
  overlayAbove?: number
  /** Rail width while collapsed. Defaults to {@link PANEL_RAIL_WIDTH}. */
  collapsedWidth?: number
  /** Which edge of the container the column is docked to. `end` = right side. */
  side?: 'start' | 'end'
  /** Hard upper bound, or a resolver given the measured container width. */
  maxWidth?: number | ((containerWidth: number) => number)
  /** Measured container used to resolve `maxWidth` and re-clamp on window resize. */
  containerRef?: RefObject<HTMLElement | null>
}

export type ResizableColumn = {
  /** Requested width in px. Sits below `collapseBelow` while collapsed. */
  width: number
  /** Width to actually render, i.e. the rail width while collapsed. */
  renderedWidth: number
  zone: ResizableColumnZone
  isCollapsed: boolean
  isOverlay: boolean
  isResizing: boolean
  /** Starts a drag-resize; intended for a resize handle's `onPointerDown`. */
  startResize: (event: ReactPointerEvent) => void
  /** Leaves the collapsed rail, restoring the default width. */
  expand: () => void
  /** Collapses into the rail form. */
  collapse: () => void
  /** Leaves the overlay form by restoring the default width. */
  dock: () => void
}

export function resizableColumnZone(
  width: number,
  collapseBelow: number,
  overlayAbove?: number,
): ResizableColumnZone {
  if (width < collapseBelow) return 'collapsed'
  if (overlayAbove != null && width > overlayAbove) return 'overlay'
  return 'inline'
}

function readStoredWidth(storageKey: string, fallback: number) {
  try {
    if (typeof window === 'undefined') return fallback
    const stored = Number.parseFloat(window.localStorage.getItem(storageKey) ?? '')
    return Number.isFinite(stored) && stored > 0 ? stored : fallback
  } catch {
    return fallback
  }
}

function lockDragCursor() {
  if (typeof document === 'undefined') return
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}

function releaseDragCursor() {
  if (typeof document === 'undefined') return
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}

/**
 * Drag-to-resize column with snap zones.
 *
 * Dragging under the collapse threshold parks the column on its rail, and
 * dragging past the overlay threshold switches it to the canvas-covering form.
 * There is no rest state in between, which is what gives the panel its snap.
 */
export function useResizableColumn({
  storageKey,
  defaultWidth,
  collapseBelow,
  overlayAbove,
  collapsedWidth = PANEL_RAIL_WIDTH,
  side = 'end',
  maxWidth,
  containerRef,
}: ResizableColumnOptions): ResizableColumn {
  const [width, setWidth] = useState(() => readStoredWidth(storageKey, defaultWidth))
  const [isResizing, setIsResizing] = useState(false)
  const resizeStateRef = useRef<{ startX: number; startWidth: number } | null>(null)

  const clamp = useCallback(
    (next: number) => {
      const containerWidth = containerRef?.current?.clientWidth ?? 0
      const limit =
        typeof maxWidth === 'function'
          ? containerWidth > 0
            ? maxWidth(containerWidth)
            : defaultWidth
          : maxWidth
      const upper =
        limit != null && limit > 0 ? Math.max(limit, collapsedWidth) : Number.POSITIVE_INFINITY
      return Math.min(upper, Math.max(collapsedWidth, next))
    },
    [collapsedWidth, containerRef, defaultWidth, maxWidth],
  )

  useEffect(() => {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.setItem(storageKey, String(width))
    } catch {
      /* storage is best-effort */
    }
  }, [storageKey, width])

  useEffect(() => {
    if (typeof window === 'undefined') return undefined
    const handleResize = () => setWidth((current) => clamp(current))
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [clamp])

  useEffect(() => {
    if (!isResizing) return undefined

    const handlePointerMove = (event: PointerEvent) => {
      const resizeState = resizeStateRef.current
      if (!resizeState) return
      event.preventDefault()
      const delta =
        side === 'end'
          ? resizeState.startX - event.clientX
          : event.clientX - resizeState.startX
      setWidth(clamp(resizeState.startWidth + delta))
    }

    const stopResizing = () => {
      resizeStateRef.current = null
      setIsResizing(false)
      releaseDragCursor()
    }

    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', stopResizing)
    window.addEventListener('pointercancel', stopResizing)
    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', stopResizing)
      window.removeEventListener('pointercancel', stopResizing)
    }
  }, [clamp, isResizing, side])

  const zone = useMemo(
    () => resizableColumnZone(width, collapseBelow, overlayAbove),
    [collapseBelow, overlayAbove, width],
  )
  const renderedWidth = zone === 'collapsed' ? collapsedWidth : width

  const startResize = useCallback(
    (event: ReactPointerEvent) => {
      if (event.pointerType === 'mouse' && event.button !== 0) return
      event.preventDefault()
      // Dragging out of the rail picks up from what is on screen, not from the
      // threshold, so the column does not jump as soon as the drag begins.
      resizeStateRef.current = {
        startX: event.clientX,
        startWidth: zone === 'collapsed' ? renderedWidth : width,
      }
      setIsResizing(true)
      lockDragCursor()
    },
    [renderedWidth, width, zone],
  )

  const expand = useCallback(() => {
    setWidth((current) => clamp(current < collapseBelow ? defaultWidth : current))
  }, [clamp, collapseBelow, defaultWidth])

  const collapse = useCallback(() => {
    setWidth(collapsedWidth)
  }, [collapsedWidth])

  const dock = useCallback(() => {
    setWidth((current) =>
      overlayAbove == null || current <= overlayAbove ? current : clamp(defaultWidth),
    )
  }, [clamp, defaultWidth, overlayAbove])

  return {
    width,
    renderedWidth,
    zone,
    isCollapsed: zone === 'collapsed',
    isOverlay: zone === 'overlay',
    isResizing,
    startResize,
    expand,
    collapse,
    dock,
  }
}