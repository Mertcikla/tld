// src/components/ZUI/useZUIInteraction.ts

import { useCallback, useEffect, useRef, useState, useMemo } from 'react'
import type { BBox, DiagramGroupLayout, LayoutNode, ZUIViewState, HoveredItem } from './types'
import { getExpandThresholds, screenToWorldX, screenToWorldY, viewOriginX, viewOriginY, worldToScreenX, worldToScreenY } from './layoutEngine'
import { hitTestZUIRenderedNode, warmZUIHitTestIndexes } from './hitTest'
import { buildEdgeSpatialIndex, findHoveredEdge, type EdgeSpatialIndex } from './edgeHover'
import { isMouseWheelGesture, isNotchedWheelGesture, wheelZoomFactor } from '../../utils/wheel'

export function constrainViewState(view: ZUIViewState, canvasW: number, canvasH: number, bbox: BBox): ZUIViewState {
  const padding = Math.min(600, canvasW * 0.45, canvasH * 0.45)
  const normalized = normalizeViewState(view, canvasW, canvasH)
  const halfVisibleX = Math.max(0, canvasW / 2 - padding) / normalized.zoom
  const halfVisibleY = Math.max(0, canvasH / 2 - padding) / normalized.zoom
  const minOriginX = bbox.minX - halfVisibleX
  const maxOriginX = bbox.maxX + halfVisibleX
  const minOriginY = bbox.minY - halfVisibleY
  const maxOriginY = bbox.maxY + halfVisibleY

  return {
    ...normalized,
    originX: maxOriginX >= minOriginX
      ? Math.max(minOriginX, Math.min(maxOriginX, viewOriginX(normalized)))
      : (minOriginX + maxOriginX) / 2,
    originY: maxOriginY >= minOriginY
      ? Math.max(minOriginY, Math.min(maxOriginY, viewOriginY(normalized)))
      : (minOriginY + maxOriginY) / 2,
  }
}

function normalizeViewState(view: ZUIViewState, canvasW: number, canvasH: number): ZUIViewState {
  const zoom = Math.max(0.0001, view.zoom)
  return {
    ...view,
    x: canvasW / 2,
    y: canvasH / 2,
    zoom,
    originX: screenToWorldX(canvasW / 2, { ...view, zoom }),
    originY: screenToWorldY(canvasH / 2, { ...view, zoom }),
  }
}

function findHoveredGroup(worldX: number, worldY: number, groups: DiagramGroupLayout[], view: ZUIViewState): DiagramGroupLayout | null {
  for (const group of groups) {
    // Check if mouse is near the diagram label (placed above the main diagram box)
    const labelCenterX = group.worldX + group.diagramX + group.diagramW / 2
    const labelTop = group.worldY + group.diagramY - 50 / view.zoom
    const labelBot = group.worldY + group.diagramY

    // Estimated width for the label hit-target
    const labelHalfW = 100 / view.zoom

    if (worldX >= labelCenterX - labelHalfW && worldX <= labelCenterX + labelHalfW &&
      worldY >= labelTop && worldY <= labelBot) {
      return group
    }
  }
  return null
}

export function calculateMaxZoom(groups: DiagramGroupLayout[], canvasW: number, view?: ZUIViewState, canvasH?: number): number {
  if (canvasW <= 0) return 40
  const cH = canvasH ?? canvasW
  const thresholds = getExpandThresholds(canvasW)
  let maxPossibleZoom = 40

  let hasVisibleExpandable = !view
  let anyVisible = !view
  let minVisibleAbsW = Infinity

  function visitNodes(nodes: LayoutNode[], cumulativeScale: number,
    parentAbsX: number, parentAbsY: number,
    parentChildOffsetX: number, parentChildOffsetY: number,
  ) {
    for (const node of nodes) {
      const absW = node.worldW * cumulativeScale

      if (!node.children || node.children.length === 0) {
        const neededZoom = thresholds.end / absW
        if (neededZoom > maxPossibleZoom) {
          maxPossibleZoom = neededZoom
        }
      } else {
        visitNodes(node.children, cumulativeScale * (node.childScale > 0 ? node.childScale : 1),
          parentAbsX + (node.worldX - parentChildOffsetX) * cumulativeScale,
          parentAbsY + (node.worldY - parentChildOffsetY) * cumulativeScale,
          node.childOffsetX, node.childOffsetY)
      }

      if (view) {
        const absX = parentAbsX + (node.worldX - parentChildOffsetX) * cumulativeScale
        const absY = parentAbsY + (node.worldY - parentChildOffsetY) * cumulativeScale
        const sx = worldToScreenX(absX, view)
        const sy = worldToScreenY(absY, view)
        const sw = absW * view.zoom
        const sh = node.worldH * cumulativeScale * view.zoom
        if (sx + sw > 0 && sy + sh > 0 && sx < canvasW && sy < cH) {
          anyVisible = true
          if (absW < minVisibleAbsW) minVisibleAbsW = absW
          if (node.children && node.children.length > 0 && sw < thresholds.end) {
            hasVisibleExpandable = true
          }
        }
      }
    }
  }

  for (const group of groups) {
    visitNodes(group.nodes, 1, 0, 0, 0, 0)
  }

  if (view) {
    if (!anyVisible) {
      return view.zoom
    }
    if (!hasVisibleExpandable && minVisibleAbsW < Infinity) {
      const capZoom = thresholds.end / minVisibleAbsW
      return Math.min(maxPossibleZoom, capZoom)
    }
  }

  return maxPossibleZoom
}

const MIN_ZOOM = 0.4
const ZUI_NATIVE_WHEEL_SELECTOR = '[data-zui-native-wheel="true"]'

function shouldIgnoreCapturedWheel(e: WheelEvent): boolean {
  const target = e.target
  return target instanceof Element && target.closest(ZUI_NATIVE_WHEEL_SELECTOR) !== null
}

function clampZoom(z: number, prevZ: number, maxZ: number): number {
  if (z > prevZ) {
    // Zooming IN: cap at maxZ (but don't force down if already above)
    return Math.min(z, Math.max(prevZ, maxZ))
  } else {
    // Zooming OUT: ignore maxZ (only cap at global MIN_ZOOM)
    return Math.max(z, MIN_ZOOM)
  }
}

/** Zoom toward/away from a screen-space focal point. */
export function zoomAround(
  view: ZUIViewState,
  focalX: number,
  focalY: number,
  factor: number,
  maxZoom: number,
): ZUIViewState {
  const newZoom = clampZoom(view.zoom * factor, view.zoom, maxZoom)
  const worldX = screenToWorldX(focalX, view)
  const worldY = screenToWorldY(focalY, view)
  const originX = viewOriginX(view)
  const originY = viewOriginY(view)
  return {
    originX,
    originY,
    zoom: newZoom,
    x: focalX - (worldX - originX) * newZoom,
    y: focalY - (worldY - originY) * newZoom,
  }
}

export interface ZUIInteraction {
  viewState: ZUIViewState
  /** Ref that is updated synchronously on every input event use this in RAF loops to avoid waiting for React renders. */
  viewStateRef: React.MutableRefObject<ZUIViewState>
  setViewState: React.Dispatch<React.SetStateAction<ZUIViewState>>
  /** Call with the canvas DOMRect + layout bbox to fit all content. */
  fitView: (
    canvasW: number,
    canvasH: number,
    bbox: { minX: number; minY: number; maxX: number; maxY: number },
    padding?: number,
  ) => void
  maxZoom: number
  hoveredItem: HoveredItem | null
  setHoveredItem: (item: HoveredItem | null, force?: boolean) => void
  /** Set to true to prevent clearing hoveredItem while the mouse is over the hover popover. */
  setHoverLocked: (locked: boolean) => void
}

export function useZUIInteraction(
  canvasRef: React.RefObject<HTMLCanvasElement | null>,
  initialView: ZUIViewState = { x: 0, y: 0, zoom: 0.3 },
  groups: DiagramGroupLayout[] = [],
  bbox?: BBox,
  onZoom?: () => void,
  onPan?: () => void,
  isMobile: boolean = false,
  resolveHoveredProxyItem?: (worldX: number, worldY: number, view: ZUIViewState, canvasW: number) => HoveredItem | null,
  hiddenTags: string[] = [],
  canvasWidth: number = 0,
  hoverLocked: boolean = false,
): ZUIInteraction {
  const [viewState, setViewStateInternal] = useState<ZUIViewState>(initialView)
  const [hoveredItem, setHoveredItemInternal] = useState<HoveredItem | null>(null)
  const popoverHoverLockedRef = useRef(false)
  const externalHoverLockedRef = useRef(hoverLocked)
  const hoverTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isHoverLocked = useCallback(() => {
    return externalHoverLockedRef.current || popoverHoverLockedRef.current
  }, [])

  const setHoveredItem = useCallback((item: HoveredItem | null, force = false) => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current)
      hoverTimeoutRef.current = null
    }

    if (item === null) {
      if (force) {
        setHoveredItemInternal(null)
        return
      }
      // Grace period before clearing hover to allow mouse to reach popover
      hoverTimeoutRef.current = setTimeout(() => {
        if (!isHoverLocked()) {
          setHoveredItemInternal(null)
        }
      }, 100)
    } else {
      setHoveredItemInternal(item)
    }
  }, [isHoverLocked])

  const setHoverLocked = useCallback((locked: boolean) => {
    popoverHoverLockedRef.current = locked
    if (locked && hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current)
      hoverTimeoutRef.current = null
    }
    if (!locked) {
      // If we unlock and there is no item currently being "detected" by mouse,
      // it should ideally clear soon. The next mouse move will handle it.
    }
  }, [])

  useEffect(() => {
    externalHoverLockedRef.current = hoverLocked
    if (hoverLocked && hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current)
      hoverTimeoutRef.current = null
    }
  }, [hoverLocked])

  // ── Refs for stable event handlers ──────────────────────────────
  const viewStateRef = useRef<ZUIViewState>(initialView)
  const groupsRef = useRef<DiagramGroupLayout[]>(groups)
  const hiddenTagsRef = useRef<ReadonlySet<string>>(new Set(hiddenTags))
  const edgeSpatialIndexRef = useRef<EdgeSpatialIndex | null>(null)
  if (edgeSpatialIndexRef.current === null) {
    edgeSpatialIndexRef.current = buildEdgeSpatialIndex(groups)
  }
  const bboxRef = useRef<BBox | undefined>(bbox)
  const onZoomRef = useRef(onZoom)
  const onPanRef = useRef(onPan)

  useEffect(() => {
    groupsRef.current = groups
    edgeSpatialIndexRef.current = buildEdgeSpatialIndex(groups)
    for (const group of groups) {
      warmZUIHitTestIndexes(group.nodes)
    }
    bboxRef.current = bbox
    onZoomRef.current = onZoom
    onPanRef.current = onPan
  }, [groups, bbox, onZoom, onPan])

  useEffect(() => {
    hiddenTagsRef.current = new Set(hiddenTags)
  }, [hiddenTags])

  const dynamicMaxZoom = useMemo(() => {
    return calculateMaxZoom(groups, canvasWidth || 1200, viewState) // Fallback width for initial calc
  }, [groups, canvasWidth, viewState])

  const maxZoomRef = useRef(40)
  const pendingViewStateRef = useRef<ZUIViewState | null>(null)
  const queuedViewStateRafRef = useRef<number | null>(null)
  useEffect(() => {
    maxZoomRef.current = dynamicMaxZoom
  }, [dynamicMaxZoom])

  const resolveViewState = useCallback((
    update: React.SetStateAction<ZUIViewState>,
    baseView: ZUIViewState,
  ): ZUIViewState => {
    const next = typeof update === 'function' ? (update as (p: ZUIViewState) => ZUIViewState)(baseView) : update
    const box = bboxRef.current
    if (!box || !canvasRef.current) {
      return next
    }

    const el = canvasRef.current
    const w = el.clientWidth || el.width / (window.devicePixelRatio || 1)
    const h = el.clientHeight || el.height / (window.devicePixelRatio || 1)

    if (w === 0 || h === 0) {
      return next
    }

    return constrainViewState(next, w, h, box)
  }, [canvasRef])

  const flushQueuedViewState = useCallback(() => {
    queuedViewStateRafRef.current = null
    const next = pendingViewStateRef.current
    pendingViewStateRef.current = null
    if (!next) return

    viewStateRef.current = next
    setViewStateInternal(next)
  }, [])

  const setViewState = useCallback(
    (update: React.SetStateAction<ZUIViewState>) => {
      if (queuedViewStateRafRef.current !== null) {
        cancelAnimationFrame(queuedViewStateRafRef.current)
        queuedViewStateRafRef.current = null
      }

      const baseView = pendingViewStateRef.current ?? viewStateRef.current
      const next = resolveViewState(update, baseView)
      pendingViewStateRef.current = null
      viewStateRef.current = next
      setViewStateInternal(next)
    },
    [resolveViewState],
  )

  const scheduleViewState = useCallback((update: React.SetStateAction<ZUIViewState>) => {
    const baseView = pendingViewStateRef.current ?? viewStateRef.current
    const next = resolveViewState(update, baseView)
    pendingViewStateRef.current = next
    viewStateRef.current = next

    if (queuedViewStateRafRef.current !== null) return
    queuedViewStateRafRef.current = requestAnimationFrame(flushQueuedViewState)
  }, [flushQueuedViewState, resolveViewState])

  useEffect(() => {
    return () => {
      if (queuedViewStateRafRef.current !== null) {
        cancelAnimationFrame(queuedViewStateRafRef.current)
      }
    }
  }, [])

  const dragging = useRef(false)
  const lastMouse = useRef({ x: 0, y: 0 })
  const lastPinchDist = useRef<number | null>(null)
  const lastPinchMid = useRef({ x: 0, y: 0 })

  const fitView = useCallback(
    (
      canvasW: number,
      canvasH: number,
      bbox: { minX: number; minY: number; maxX: number; maxY: number },
      padding = 0.1,
    ) => {
      const bboxW = bbox.maxX - bbox.minX
      const bboxH = bbox.maxY - bbox.minY
      if (bboxW <= 0 || bboxH <= 0) return

      const currentMaxZ = calculateMaxZoom(groupsRef.current, canvasW)
      const zoom = Math.max(MIN_ZOOM, Math.min(currentMaxZ,
        Math.min(
          (canvasW * (1 - padding * 2)) / bboxW,
          (canvasH * (1 - padding * 2)) / bboxH,
        ),
      ))
      const x = (canvasW - bboxW * zoom) / 2 - bbox.minX * zoom
      const y = (canvasH - bboxH * zoom) / 2 - bbox.minY * zoom
      setViewState({ x, y, zoom })
    },
    [setViewState],
  )

  const lastPanTimeRef = useRef(0)

  useEffect(() => {
    const el = canvasRef.current
    if (!el) return

    function onWheel(e: WheelEvent) {
      if (shouldIgnoreCapturedWheel(e)) return

      const rect = el!.getBoundingClientRect()
      const isInsideCanvas =
        e.clientX >= rect.left &&
        e.clientX <= rect.right &&
        e.clientY >= rect.top &&
        e.clientY <= rect.bottom

      if (!isInsideCanvas) return

      // Heuristic to distinguish between trackpad and physical mouse wheel:
      // 1. If ctrlKey is true, it's a pinch (trackpad) or Ctrl+Wheel. We always zoom.
      // 2. If deltaMode !== 0, it's a physical mouse wheel (DOM_DELTA_LINE/PAGE). We zoom.
      // 3. Only zoom on notched mouse wheel, not trackpad pan gestures.
      const isPinch = e.ctrlKey

      // We don't have isRecentMultiTouch yet, but we can check if it looks like a mouse wheel
      const isMouseWheel = isMouseWheelGesture(e)

      // On mobile, Safari synthesizes wheel events for pinches.
      // If it's not a pinch or a real mouse wheel, we ignore it to allow native gestures or prevent conflicts.
      if (isMobile && !isPinch && !isMouseWheel) return

      e.preventDefault()
      setHoveredItem(null, true) // Clear popover immediately on zoom/pan

      // Track multi-touch wheel events (deltaX !== 0 indicates two-finger contact on trackpad)
      if (e.deltaX !== 0) {
        lastPanTimeRef.current = Date.now()
      }

      // If we just finished a multi-touch gesture, suppress zoom for ~1000ms (trackpad momentum can last longer)
      const isRecentMultiTouch = Date.now() - lastPanTimeRef.current < 1000

      // Re-evaluate isMouseWheel with trackpad suppression for desktop
      const isNotchedWheel = !isRecentMultiTouch && isNotchedWheelGesture(e)
      const isRealMouseWheel = e.deltaMode !== 0 || isNotchedWheel

      if (isPinch || isRealMouseWheel) {
        const focalX = e.clientX - rect.left
        const focalY = e.clientY - rect.top

        const factor = wheelZoomFactor(e, isRealMouseWheel)

        scheduleViewState((prev) => {
          return zoomAround(prev, focalX, focalY, factor, maxZoomRef.current)
        })
        onZoomRef.current?.()
      } else if (!isMobile) {
        // Trackpad panning - disabled on mobile to avoid interference with pinch-to-zoom
        scheduleViewState((prev) => ({ ...prev, x: prev.x - e.deltaX, y: prev.y - e.deltaY }))
        onPanRef.current?.()
      }
    }

    function onMouseDown(e: MouseEvent) {
      if (e.button !== 0) return
      dragging.current = true
      lastMouse.current.x = e.clientX
      lastMouse.current.y = e.clientY
      el!.style.cursor = 'grabbing'
      setHoveredItem(null, true) // Hide popover immediately while dragging
    }

    function onMouseMove(e: MouseEvent) {
      const rect = el!.getBoundingClientRect()
      const screenX = e.clientX - rect.left
      const screenY = e.clientY - rect.top

      if (dragging.current) {
        const dx = e.clientX - lastMouse.current.x
        const dy = e.clientY - lastMouse.current.y
        lastMouse.current.x = e.clientX
        lastMouse.current.y = e.clientY
        scheduleViewState((prev) => ({ ...prev, x: prev.x + dx, y: prev.y + dy }))
        onPanRef.current?.()
        return
      }

      if (isHoverLocked()) return

      // Hover detection
      const view = viewStateRef.current
      const worldX = screenToWorldX(screenX, view)
      const worldY = screenToWorldY(screenY, view)
      const thresholds = getExpandThresholds(rect.width)

      const deepest = hitTestZUIRenderedNode(worldX, worldY, groupsRef.current, view, thresholds, hiddenTagsRef.current)
      if (deepest) {
        const { node, absX, absY, absW, absH } = deepest
        setHoveredItem({
          type: 'node',
          data: node,
          absX,
          absY,
          absW,
          absH
        })
      } else {
        const proxyEdge = resolveHoveredProxyItem?.(worldX, worldY, view, rect.width) ?? null
        if (proxyEdge) {
          setHoveredItem(proxyEdge)
          return
        }
        const edge = findHoveredEdge(worldX, worldY, edgeSpatialIndexRef.current!, view)
        if (edge) {
          setHoveredItem(edge)
        } else {
          const group = findHoveredGroup(worldX, worldY, groupsRef.current, view)
          if (group) {
            setHoveredItem({
              type: 'group',
              data: group
            })
          } else {
            setHoveredItem(null)
          }
        }
      }
    }

    function onMouseUp() {
      dragging.current = false
      if (el) el.style.cursor = 'grab'
    }

    function onMouseOut() {
      setHoveredItem(null)
    }

    function onDblClick(e: MouseEvent) {
      const rect = el!.getBoundingClientRect()
      const focalX = e.clientX - rect.left
      const focalY = e.clientY - rect.top
      setHoveredItem(null, true) // Clear popover immediately on double-click zoom

      setViewState((prev) => {
        return zoomAround(prev, focalX, focalY, 2, maxZoomRef.current)
      })
      onZoomRef.current?.()
    }

    // ── Touch pan + pinch ──────────────────────────────────────────
    function pinchDist(touches: TouchList): number {
      if (touches.length < 2) return 0
      const dx = touches[0].clientX - touches[1].clientX
      const dy = touches[0].clientY - touches[1].clientY
      return Math.sqrt(dx * dx + dy * dy)
    }

    function pinchMid(touches: TouchList): { x: number; y: number } {
      const rect = el!.getBoundingClientRect()
      if (touches.length < 2) {
        return { x: touches[0].clientX - rect.left, y: touches[0].clientY - rect.top }
      }
      return {
        x: (touches[0].clientX + touches[1].clientX) / 2 - rect.left,
        y: (touches[0].clientY + touches[1].clientY) / 2 - rect.top,
      }
    }

    function onTouchStart(e: TouchEvent) {
      e.preventDefault()
      if (e.touches.length === 1) {
        dragging.current = true
        lastMouse.current.x = e.touches[0].clientX
        lastMouse.current.y = e.touches[0].clientY
        lastPinchDist.current = null
      } else if (e.touches.length >= 2) {
        dragging.current = false
        const dist = pinchDist(e.touches)
        lastPinchDist.current = dist > 0 ? dist : null
        lastPinchMid.current = pinchMid(e.touches)
      }
    }

    function onTouchMove(e: TouchEvent) {
      e.preventDefault()
      setHoveredItem(null, true) // Clear popover immediately on touch movement
      if (e.touches.length === 1 && dragging.current) {
        const dx = e.touches[0].clientX - lastMouse.current.x
        const dy = e.touches[0].clientY - lastMouse.current.y
        lastMouse.current.x = e.touches[0].clientX
        lastMouse.current.y = e.touches[0].clientY
        scheduleViewState((prev) => ({ ...prev, x: prev.x + dx, y: prev.y + dy }))
        onPanRef.current?.()
      } else if (e.touches.length >= 2) {
        const dist = pinchDist(e.touches)
        const mid = pinchMid(e.touches)
        if (lastPinchDist.current !== null && lastPinchDist.current > 0) {
          const factor = dist / lastPinchDist.current
          const dx = mid.x - lastPinchMid.current.x
          const dy = mid.y - lastPinchMid.current.y

          if (isFinite(factor) && factor > 0) {
            scheduleViewState((prev) => {
              const zoomed = zoomAround(prev, mid.x, mid.y, factor, maxZoomRef.current)
              return { ...zoomed, x: zoomed.x + dx, y: zoomed.y + dy }
            })
            onZoomRef.current?.()
          }
        }
        lastPinchDist.current = dist > 0 ? dist : lastPinchDist.current
        lastPinchMid.current = mid
      }
    }
    function onTouchEnd(e: TouchEvent) {
      if (e.touches.length === 0) {
        dragging.current = false
        lastPinchDist.current = null
      } else if (e.touches.length === 1) {
        // Transition back to dragging with the single remaining finger
        dragging.current = true
        lastMouse.current.x = e.touches[0].clientX
        lastMouse.current.y = e.touches[0].clientY
        lastPinchDist.current = null
      } else {
        // Still have multiple fingers, reset baseline to avoid jumps
        const dist = pinchDist(e.touches)
        lastPinchDist.current = dist > 0 ? dist : null
        lastPinchMid.current = pinchMid(e.touches)
      }
    }

    el.style.cursor = 'grab'

    window.addEventListener('wheel', onWheel, { passive: false, capture: true })
    el.addEventListener('mousedown', onMouseDown)
    el.addEventListener('mouseleave', onMouseOut)
    el.addEventListener('mouseout', onMouseOut)
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
    el.addEventListener('dblclick', onDblClick)
    el.addEventListener('touchstart', onTouchStart, { passive: false })
    el.addEventListener('touchmove', onTouchMove, { passive: false })
    el.addEventListener('touchend', onTouchEnd)
    el.addEventListener('touchcancel', onTouchEnd)

    return () => {
      window.removeEventListener('wheel', onWheel, { capture: true })
      el.removeEventListener('mousedown', onMouseDown)
      el.removeEventListener('mouseleave', onMouseOut)
      el.removeEventListener('mouseout', onMouseOut)
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
      el.removeEventListener('dblclick', onDblClick)
      el.removeEventListener('touchstart', onTouchStart)
      el.removeEventListener('touchmove', onTouchMove)
      el.removeEventListener('touchend', onTouchEnd)
      el.removeEventListener('touchcancel', onTouchEnd)
    }
  }, [canvasRef, scheduleViewState, setViewState, setHoveredItem, isHoverLocked, isMobile, resolveHoveredProxyItem]) // groupsRef handles groups updates without re-binding!

  return { viewState, viewStateRef, setViewState, fitView, maxZoom: dynamicMaxZoom, hoveredItem, setHoveredItem, setHoverLocked }
}
