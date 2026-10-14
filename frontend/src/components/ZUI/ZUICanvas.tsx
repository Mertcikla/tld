import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import { useBreakpointValue } from '@chakra-ui/react'
import { useTouchOnlyCanvasInput } from '../../hooks/useCanvasInputMode'
import { shouldEnableCanvasWheelPan } from '../../utils/canvasInputMode'
import type { ExploreData, ViewLayer } from '../../types'
import type { CrossBranchContextSettings } from '../../crossBranch/types'
import { buildWorkspaceGraphSnapshot } from '../../crossBranch/graph'
import { withUnplacedElements } from '../../crossBranch/unplacedElements'
import { computeLayout } from './layout'
import { filterLayoutByTags } from './tagVisibility'
import { getCameraRebase, screenToWorldX, screenToWorldY } from './layoutEngine'
import { useZUIInteraction } from './useZUIInteraction'
import type { DiagramGroupLayout, HoveredItem, ZUIObjectSelection, ZUITestInteraction, ZUIViewState } from './types'
import { getPathAt } from './camera'
import type { ZUIHitTestNodeResult } from './hitTest'
import { useZUICamera } from './useZUICamera'
import { useZUIProxyConnectors } from './useZUIProxyConnectors'
import { useZUIRenderLoop } from './useZUIRenderLoop'
import { ZUIBreadcrumb, ZUIHoverPopover } from './ZUIOverlays'
import { getHoverScreenRect } from './hoverFeedback'
import { applyChangeOverlays } from './changeOverlay'
import type { ZUIChangeOverlay } from './types'

declare global {
  interface Window {
    __TLD_ZUI_TEST_STATE__?: {
      viewState: ZUIViewState
      groups: DiagramGroupLayout[]
      lastInteraction?: ZUITestInteraction
    }
  }
}

export interface ZUICanvasHandle {
  fitView(): void
  focusDiagram(viewId: number): boolean
  focusElement(viewId: number, elementId: number): boolean
  setCameraFrame(frame: ZUICameraFrame): boolean
}

export interface ZUICameraFrame {
  profile: 'detail-to-overview'
  progress: number
}

interface Props {
  preserveCameraOnUpdate?: boolean
  changeOverlays?: Record<number, ZUIChangeOverlay>
  data: ExploreData
  onReady?: () => void
  onSelectObject?: (selection: ZUIObjectSelection | null) => void
  onZoom?: () => void
  onPan?: () => void
  initialCameraFrame?: ZUICameraFrame
  highlightedTags?: string[]
  highlightColor?: string
  hiddenTags?: string[]
  selectedTags?: string[]
  tagColors?: Record<string, { color?: string } | string>
  groupLayers?: ViewLayer[]
  crossBranchSettings: CrossBranchContextSettings
  hoverLocked?: boolean
}

export const ZUICanvas = forwardRef<ZUICanvasHandle, Props>(function ZUICanvas({
  preserveCameraOnUpdate = false,
  changeOverlays,
  data,
  onReady,
  onSelectObject,
  onZoom,
  onPan,
  initialCameraFrame,
  highlightedTags,
  highlightColor,
  hiddenTags,
  selectedTags,
  tagColors,
  groupLayers = [],
  crossBranchSettings,
  hoverLocked = false,
}, ref) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [initialized, setInitialized] = useState(false)
  const [containerSize, setContainerSize] = useState({ w: 0, h: 0 })
  const [debugStateReady, setDebugStateReady] = useState(false)
  const isMobileLayout = useBreakpointValue({ base: true, md: false }) ?? false
  const touchOnlyCanvasInput = useTouchOnlyCanvasInput()
  const suppressWheelPan = !shouldEnableCanvasWheelPan({ isMobileLayout, touchOnlyInput: touchOnlyCanvasInput })
  const debugViewport = useMemo(() => typeof window !== 'undefined' && window.location.href.includes('debugZuiCamera'), [])
  const debugTestState = useMemo(() => typeof window !== 'undefined' && window.location.href.includes('debugZuiTest'), [])

  const layout = useMemo(() => {
    const base = computeLayout(withUnplacedElements(data))
    return changeOverlays ? applyChangeOverlays(base, changeOverlays) : base
  }, [data, changeOverlays])
  const visibleLayout = useMemo(() => filterLayoutByTags(layout, selectedTags, hiddenTags), [layout, selectedTags, hiddenTags])
  const fittedLayoutRef = useRef(layout)
  const workspaceSnapshot = useMemo(() => buildWorkspaceGraphSnapshot(data), [data])

  const proxyResolverRef = useRef<((worldX: number, worldY: number, view: ZUIViewState, canvasW: number) => HoveredItem | null) | null>(null)
  const resolveHoveredProxyItem = useCallback((worldX: number, worldY: number, view: ZUIViewState, canvasW: number) => {
    return proxyResolverRef.current?.(worldX, worldY, view, canvasW) ?? null
  }, [])

  const cameraActionsRef = useRef<ReturnType<typeof useZUICamera> | null>(null)
  const handleNodeClick = useCallback((hit: ZUIHitTestNodeResult) => {
    cameraActionsRef.current?.revealNode(hit)
  }, [])
  const handleZoom = useCallback(() => {
    cameraActionsRef.current?.cancelTransition()
    onZoom?.()
  }, [onZoom])
  const handlePan = useCallback(() => {
    cameraActionsRef.current?.cancelTransition()
    onPan?.()
  }, [onPan])

  const { viewState, viewStateRef, setViewState, fitView, maxZoom, hoveredItem, setHoveredItem, setHoverLocked, setHoverPopoverElement } = useZUIInteraction(
    canvasRef,
    undefined,
    visibleLayout.groups,
    layout.bbox,
    handleZoom,
    handlePan,
    suppressWheelPan,
    resolveHoveredProxyItem,
    hiddenTags,
    containerSize.w,
    hoverLocked,
    onSelectObject,
    handleNodeClick,
  )

  useEffect(() => {
    setHoveredItem(null, true)
  }, [visibleLayout, setHoveredItem])

  const viewportBounds = useMemo(() => {
    const zoom = Math.max(0.0001, viewState.zoom)
    const stableView = { ...viewState, zoom }
    const minX = screenToWorldX(0, stableView)
    const minY = screenToWorldY(0, stableView)
    const maxX = screenToWorldX(containerSize.w, stableView)
    const maxY = screenToWorldY(containerSize.h, stableView)
    return {
      minX,
      minY,
      maxX,
      maxY,
      centerX: (minX + maxX) / 2,
      centerY: (minY + maxY) / 2,
    }
  }, [containerSize.h, containerSize.w, viewState])

  const proxyState = useZUIProxyConnectors(
    visibleLayout.groups,
    workspaceSnapshot,
    viewState,
    containerSize.w,
    viewportBounds,
    crossBranchSettings,
    hiddenTags,
  )
  proxyResolverRef.current = proxyState.resolveHoveredProxyItem

  const camera = useZUICamera({
    containerRef,
    layoutGroups: layout.groups,
    layoutBBox: layout.bbox,
    maxZoom,
    isMobileLayout,
    initialCameraFrame,
    fitView,
    setViewState,
    viewStateRef,
    setHoveredItem,
  })
  cameraActionsRef.current = camera
  const {
    fitInitialView,
    fitCurrentView,
    focusDiagram,
    focusElement,
    setCameraFrame,
    zoomToPathItem,
  } = camera

  useZUIRenderLoop({
    canvasRef,
    containerRef,
    initialized,
    setInitialized,
    setContainerSize,
    fitInitialView,
    onReady,
    layout: visibleLayout,
    viewState,
    viewStateRef,
    crossBranchSettings,
    proxyState,
    highlightedTags,
    highlightColor,
    hiddenTags,
    tagColors,
    groupLayers,
    hoveredItem,
  })

  useEffect(() => {
    if (!debugTestState || !initialized) {
      setDebugStateReady(false)
      return
    }
    let cancelled = false
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        if (!cancelled) setDebugStateReady(true)
      })
    })
    return () => { cancelled = true }
  }, [debugTestState, initialized])

  if (debugTestState && debugStateReady && typeof window !== 'undefined') {
    window.__TLD_ZUI_TEST_STATE__ = {
      viewState,
      groups: visibleLayout.groups,
      lastInteraction: window.__TLD_ZUI_TEST_STATE__?.lastInteraction,
    }
  }

  useEffect(() => {
    if (!debugTestState || !debugStateReady || typeof window === 'undefined') return
    window.__TLD_ZUI_TEST_STATE__ = {
      viewState,
      groups: visibleLayout.groups,
      lastInteraction: window.__TLD_ZUI_TEST_STATE__?.lastInteraction,
    }
  }, [debugStateReady, debugTestState, visibleLayout.groups, viewState])

  useEffect(() => {
    if (typeof window === 'undefined') return
    return () => {
      delete window.__TLD_ZUI_TEST_STATE__
    }
  }, [])

  useEffect(() => {
    if (!initialized) return
    if (fittedLayoutRef.current === layout) return
    fittedLayoutRef.current = layout
    if (preserveCameraOnUpdate) return
    const el = containerRef.current
    if (!el) return
    const w = el.offsetWidth
    const h = el.offsetHeight
    if (w > 0 && h > 0) {
      setContainerSize({ w, h })
      fitInitialView(w, h)
    }
  }, [fitInitialView, initialized, layout, preserveCameraOnUpdate])

  useEffect(() => {
    if (!debugViewport) return
    const cameraRebase = getCameraRebase(viewState, containerSize.w, containerSize.h)
    console.debug('[ZUICanvas] viewport', {
      x: viewState.x,
      y: viewState.y,
      zoom: viewState.zoom,
      width: containerSize.w,
      height: containerSize.h,
      minX: viewportBounds.minX,
      minY: viewportBounds.minY,
      maxX: viewportBounds.maxX,
      maxY: viewportBounds.maxY,
      centerX: viewportBounds.centerX,
      centerY: viewportBounds.centerY,
      renderX: cameraRebase.view.x,
      renderY: cameraRebase.view.y,
      renderOriginX: cameraRebase.originX,
      renderOriginY: cameraRebase.originY,
    })
  }, [containerSize.h, containerSize.w, debugViewport, viewState, viewportBounds])

  const hoveredScreenRect = useMemo(() => getHoverScreenRect(hoveredItem, viewState), [hoveredItem, viewState])

  const isHoveredItemFullyVisible = useMemo(() => {
    if (!hoveredScreenRect || containerSize.w === 0) return false
    return (
      hoveredScreenRect.sx >= 2 &&
      hoveredScreenRect.sy >= 2 &&
      hoveredScreenRect.sx + hoveredScreenRect.sw <= containerSize.w - 2 &&
      hoveredScreenRect.sy + hoveredScreenRect.sh <= containerSize.h - 2
    )
  }, [hoveredScreenRect, containerSize])


  const [breadcrumbView, setBreadcrumbView] = useState(viewState)
  const breadcrumbTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    if (breadcrumbTimerRef.current) clearTimeout(breadcrumbTimerRef.current)
    breadcrumbTimerRef.current = setTimeout(() => setBreadcrumbView(viewState), 80)
    return () => { if (breadcrumbTimerRef.current) clearTimeout(breadcrumbTimerRef.current) }
  }, [viewState])

  const currentPath = useMemo(() => {
    return getPathAt(breadcrumbView, visibleLayout.groups, containerSize.w, containerSize.h)
  }, [breadcrumbView, visibleLayout.groups, containerSize])


  useImperativeHandle(
    ref,
    () => ({
      fitView: fitCurrentView,
      focusDiagram,
      focusElement,
      setCameraFrame,
    }),
    [fitCurrentView, focusDiagram, focusElement, setCameraFrame],
  )

  return (
    <div
      ref={containerRef}
      style={{ width: '100%', height: '100%', overflow: 'hidden', position: 'relative' }}
    >
      <canvas
        ref={canvasRef}
        style={{
          display: 'block',
          width: '100%',
          height: '100%',
          opacity: initialized ? 1 : 0,
          transition: 'opacity 0.2s ease-in',
          touchAction: 'none',
        }}
      />

      <ZUIBreadcrumb
        initialized={initialized}
        isMobileLayout={isMobileLayout}
        currentPath={currentPath}
        onZoomToPathItem={zoomToPathItem}
      />

      <ZUIHoverPopover
        hoveredItem={hoveredItem}
        hoveredScreenRect={hoveredScreenRect}
        isHoveredItemFullyVisible={isHoveredItemFullyVisible}
        onHoverLock={setHoverLocked}
        onPopoverElement={setHoverPopoverElement}
      />
    </div>
  )
})
