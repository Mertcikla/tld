import { useCallback, useEffect, useRef } from 'react'
import type { MutableRefObject, RefObject } from 'react'
import type { ZUICameraFrame } from './ZUICanvas'
import type { BBox, DiagramGroupLayout, HoveredItem, ZUIViewState } from './types'
import type { ZUIHitTestNodeResult } from './hitTest'
import { focusTargetForNode, viewportForNodeRevealTarget, findDiagramFocusTarget, findElementFocusTarget, viewportForDiagramFocusTarget, viewportForElementFocusTarget } from './focus'
import { clamp01, easeOutQuart, findFirstExpandableNode, fitWorldRect, type PathItem } from './camera'
import { rawCameraView } from './layoutEngine'

export interface ZUICameraController {
  animateToViewport: (to: ZUIViewState) => void
  fitInitialView: (w: number, h: number) => void
  fitCurrentView: () => void
  focusDiagram: (viewId: number) => boolean
  focusElement: (viewId: number, elementId: number) => boolean
  setCameraFrame: (frame: ZUICameraFrame) => boolean
  zoomToPathItem: (item: PathItem) => void
  revealNode: (hit: ZUIHitTestNodeResult) => boolean
  cancelTransition: () => void
}

interface UseZUICameraArgs {
  containerRef: RefObject<HTMLDivElement | null>
  layoutGroups: DiagramGroupLayout[]
  layoutBBox: BBox
  maxZoom: number
  isMobileLayout: boolean
  initialCameraFrame?: ZUICameraFrame
  fitView: (
    canvasW: number,
    canvasH: number,
    bbox: { minX: number; minY: number; maxX: number; maxY: number },
    padding?: number,
  ) => void
  setViewState: React.Dispatch<React.SetStateAction<ZUIViewState>>
  viewStateRef: MutableRefObject<ZUIViewState>
  setHoveredItem: (item: HoveredItem | null, force?: boolean) => void
}

export function useZUICamera({
  containerRef,
  layoutGroups,
  layoutBBox,
  maxZoom,
  isMobileLayout,
  initialCameraFrame,
  fitView,
  setViewState,
  viewStateRef,
  setHoveredItem,
}: UseZUICameraArgs): ZUICameraController {
  const cameraTransitionRef = useRef<number | null>(null)

  const cancelTransition = useCallback(() => {
    if (cameraTransitionRef.current !== null) {
      cancelAnimationFrame(cameraTransitionRef.current)
      cameraTransitionRef.current = null
    }
  }, [])

  const animateToViewport = useCallback((to: ZUIViewState) => {
    cancelTransition()

    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      setViewState(to)
      return
    }
    const from = rawCameraView(viewStateRef.current)
    const duration = 520
    const startedAt = performance.now()

    const step = (now: number) => {
      const t = Math.min(1, (now - startedAt) / duration)
      const eased = easeOutQuart(t)
      setViewState({
        x: from.x + (to.x - from.x) * eased,
        y: from.y + (to.y - from.y) * eased,
        zoom: from.zoom + (to.zoom - from.zoom) * eased,
      })

      if (t < 1) {
        cameraTransitionRef.current = requestAnimationFrame(step)
      } else {
        cameraTransitionRef.current = null
        setViewState(to)
      }
    }

    cameraTransitionRef.current = requestAnimationFrame(step)
  }, [cancelTransition, setViewState, viewStateRef])

  const focusDiagram = useCallback((viewId: number) => {
    const el = containerRef.current
    const target = findDiagramFocusTarget(layoutGroups, viewId)
    if (!el || !target) return false

    const canvasW = el.offsetWidth
    const canvasH = el.offsetHeight
    if (canvasW === 0 || canvasH === 0) return false

    const to = viewportForDiagramFocusTarget(target, canvasW, canvasH, maxZoom, isMobileLayout)
    if (!to) return false

    setHoveredItem(null, true)
    animateToViewport(to)
    return true
  }, [animateToViewport, containerRef, isMobileLayout, layoutGroups, maxZoom, setHoveredItem])

  const focusElement = useCallback((viewId: number, elementId: number) => {
    const el = containerRef.current
    const target = findElementFocusTarget(layoutGroups, viewId, elementId)
    if (!el || !target) return false

    const canvasW = el.offsetWidth
    const canvasH = el.offsetHeight
    if (canvasW === 0 || canvasH === 0) return false

    const to = viewportForElementFocusTarget(target, canvasW, canvasH, maxZoom, isMobileLayout)
    if (!to) return false

    setHoveredItem(null, true)
    animateToViewport(to)
    return true
  }, [animateToViewport, containerRef, isMobileLayout, layoutGroups, maxZoom, setHoveredItem])

  const revealNode = useCallback((hit: ZUIHitTestNodeResult) => {
    const el = containerRef.current
    if (!el || el.offsetWidth === 0 || el.offsetHeight === 0) return false
    const absX = hit.absX - (hit.node.worldW * hit.cumulativeScale - hit.absW) / 2
    const absY = hit.absY - (hit.node.worldH * hit.cumulativeScale - hit.absH) / 2
    const target = focusTargetForNode(hit.node, absX, absY, hit.cumulativeScale)
    const to = hit.node.children.length > 0
      ? viewportForNodeRevealTarget(target, el.offsetWidth, el.offsetHeight, maxZoom, isMobileLayout)
      : viewportForElementFocusTarget(target, el.offsetWidth, el.offsetHeight, maxZoom, isMobileLayout)
    if (!to || (hit.node.children.length === 0 && to.zoom <= viewStateRef.current.zoom)) return false
    setHoveredItem(null, true)
    animateToViewport(to)
    return true
  }, [animateToViewport, containerRef, isMobileLayout, maxZoom, setHoveredItem, viewStateRef])

  const setCameraFrame = useCallback((frame: ZUICameraFrame) => {
    if (frame.profile !== 'detail-to-overview') return false

    const el = containerRef.current
    if (!el) return false

    const canvasW = el.offsetWidth
    const canvasH = el.offsetHeight
    if (canvasW === 0 || canvasH === 0) return false

    const detailTarget = (frame.detailViewId !== undefined
      ? findDiagramFocusTarget(layoutGroups, frame.detailViewId)
      : null) ?? findFirstExpandableNode(layoutGroups)
    const overviewTarget = (frame.overviewViewId !== undefined
      ? findDiagramFocusTarget(layoutGroups, frame.overviewViewId)
      : null) ?? findDiagramFocusTarget(layoutGroups, layoutGroups[0]?.diagramId ?? -1)
    if (!detailTarget || !overviewTarget) return false

    const detail = fitWorldRect(
      {
        x: detailTarget.absX,
        y: detailTarget.absY,
        w: detailTarget.absW,
        h: detailTarget.absH,
      },
      canvasW,
      canvasH,
      maxZoom,
      0.28,
    )

    const overview = fitWorldRect(
      {
        x: overviewTarget.absX,
        y: overviewTarget.absY,
        w: overviewTarget.absW,
        h: overviewTarget.absH,
      },
      canvasW,
      canvasH,
      maxZoom,
      0.18,
    )

    if (!detail || !overview) return false

    cancelTransition()
    setHoveredItem(null, true)
    const t = easeOutQuart(clamp01(frame.progress))
    setViewState({
      x: detail.x + (overview.x - detail.x) * t,
      y: detail.y + (overview.y - detail.y) * t,
      zoom: detail.zoom + (overview.zoom - detail.zoom) * t,
    })
    return true
  }, [cancelTransition, containerRef, layoutGroups, maxZoom, setHoveredItem, setViewState])

  const fitInitialView = useCallback((w: number, h: number) => {
    if (initialCameraFrame && setCameraFrame(initialCameraFrame)) return
    fitView(w, h, layoutBBox)
  }, [fitView, initialCameraFrame, layoutBBox, setCameraFrame])

  const fitCurrentView = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    setHoveredItem(null, true)
    fitView(el.offsetWidth, el.offsetHeight, layoutBBox)
  }, [containerRef, fitView, layoutBBox, setHoveredItem])

  const zoomToPathItem = useCallback((item: PathItem) => {
    const el = containerRef.current
    if (!el) return
    const canvasW = el.offsetWidth
    const canvasH = el.offsetHeight
    if (canvasW === 0 || canvasH === 0) return

    cancelTransition()
    setHoveredItem(null, true)

    const next = fitWorldRect(
      { x: item.absX, y: item.absY, w: item.absW, h: item.absH },
      canvasW,
      canvasH,
      maxZoom,
      0.15,
    )
    if (next) setViewState(next)
  }, [cancelTransition, containerRef, maxZoom, setHoveredItem, setViewState])

  useEffect(() => cancelTransition, [cancelTransition])

  return {
    animateToViewport,
    fitInitialView,
    fitCurrentView,
    focusDiagram,
    focusElement,
    setCameraFrame,
    zoomToPathItem,
    revealNode,
    cancelTransition,
  }
}
