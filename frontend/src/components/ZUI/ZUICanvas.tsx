import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import { useBreakpointValue } from '@chakra-ui/react'
import { useTouchOnlyCanvasInput } from '../../hooks/useCanvasInputMode'
import { shouldEnableCanvasWheelPan } from '../../utils/canvasInputMode'
import type { ExploreData } from '../../types'
import { api } from '../../api/client'
import type { CrossBranchContextSettings } from '../../crossBranch/types'
import { buildWorkspaceGraphSnapshot } from '../../crossBranch/graph'
import type { WorkspaceVersionFollowTarget, WorkspaceVersionPreview } from '../../context/WorkspaceVersionContext'
import { diffResourceKey, type ExploreDiffDetail, type ExploreDiffLens } from '../../utils/exploreDiffLens'
import { getSourceEditor } from '../../utils/sourceEditor'
import { toast } from '../../utils/toast'
import { computeLayout } from './layout'
import { getCameraRebase, screenToWorldX, screenToWorldY, worldToScreenX, worldToScreenY } from './layoutEngine'
import { useZUIInteraction } from './useZUIInteraction'
import type { DiagramGroupLayout, HoveredItem, ZUITestInteraction, ZUIViewState } from './types'
import { getPathAt } from './camera'
import { useZUICamera } from './useZUICamera'
import { useZUIProxyConnectors } from './useZUIProxyConnectors'
import { useZUIRenderLoop } from './useZUIRenderLoop'
import { ZUIBreadcrumb, ZUIHoverPopover } from './ZUIOverlays'

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
  data: ExploreData
  onReady?: () => void
  onZoom?: () => void
  onPan?: () => void
  initialCameraFrame?: ZUICameraFrame
  highlightedTags?: string[]
  highlightColor?: string
  hiddenTags?: string[]
  versionPreview?: WorkspaceVersionPreview | null
  versionFollowTarget?: WorkspaceVersionFollowTarget | null
  diffLens?: ExploreDiffLens | null
  crossBranchSettings: CrossBranchContextSettings
  hoverLocked?: boolean
}

export const ZUICanvas = forwardRef<ZUICanvasHandle, Props>(function ZUICanvas({
  data,
  onReady,
  onZoom,
  onPan,
  initialCameraFrame,
  highlightedTags,
  highlightColor,
  hiddenTags,
  versionPreview,
  versionFollowTarget,
  diffLens,
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

  const layout = useMemo(() => computeLayout(data), [data])
  const fittedLayoutRef = useRef(layout)
  const workspaceSnapshot = useMemo(() => buildWorkspaceGraphSnapshot(data), [data])

  const proxyResolverRef = useRef<((worldX: number, worldY: number, view: ZUIViewState, canvasW: number) => HoveredItem | null) | null>(null)
  const resolveHoveredProxyItem = useCallback((worldX: number, worldY: number, view: ZUIViewState, canvasW: number) => {
    return proxyResolverRef.current?.(worldX, worldY, view, canvasW) ?? null
  }, [])

  const { viewState, viewStateRef, setViewState, fitView, maxZoom, hoveredItem, setHoveredItem, setHoverLocked } = useZUIInteraction(
    canvasRef,
    undefined,
    layout.groups,
    layout.bbox,
    onZoom,
    onPan,
    suppressWheelPan,
    resolveHoveredProxyItem,
    hiddenTags,
    containerSize.w,
    hoverLocked,
  )

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
    layout.groups,
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
    layout,
    viewState,
    viewStateRef,
    crossBranchSettings,
    proxyState,
    highlightedTags,
    highlightColor,
    hiddenTags,
    versionPreview,
    versionFollowTarget,
    diffLens,
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
      groups: layout.groups,
      lastInteraction: window.__TLD_ZUI_TEST_STATE__?.lastInteraction,
    }
  }

  useEffect(() => {
    if (!debugTestState || !debugStateReady || typeof window === 'undefined') return
    window.__TLD_ZUI_TEST_STATE__ = {
      viewState,
      groups: layout.groups,
      lastInteraction: window.__TLD_ZUI_TEST_STATE__?.lastInteraction,
    }
  }, [debugStateReady, debugTestState, layout.groups, viewState])

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
    const el = containerRef.current
    if (!el) return
    const w = el.offsetWidth
    const h = el.offsetHeight
    if (w > 0 && h > 0) {
      setContainerSize({ w, h })
      fitInitialView(w, h)
    }
  }, [fitInitialView, initialized, layout])

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

  const hoveredScreenRect = useMemo(() => {
    if (!hoveredItem) return null
    let absX: number
    let absY: number
    let absW: number
    let absH: number
    if (hoveredItem.type === 'node') {
      ({ absX, absY, absW, absH } = hoveredItem)
    } else if (hoveredItem.type === 'edge') {
      ({ absX, absY } = hoveredItem)
      absW = 0
      absH = 0
    } else {
      const group = hoveredItem.data
      absW = 200 / viewState.zoom
      absH = 50 / viewState.zoom
      absX = group.worldX + group.diagramX + group.diagramW / 2 - absW / 2
      absY = group.worldY + group.diagramY - absH
    }

    return {
      sx: worldToScreenX(absX, viewState),
      sy: worldToScreenY(absY, viewState),
      sw: absW * viewState.zoom,
      sh: absH * viewState.zoom,
    }
  }, [hoveredItem, viewState])

  const isHoveredItemFullyVisible = useMemo(() => {
    if (!hoveredScreenRect || containerSize.w === 0) return false
    return (
      hoveredScreenRect.sx >= 2 &&
      hoveredScreenRect.sy >= 2 &&
      hoveredScreenRect.sx + hoveredScreenRect.sw <= containerSize.w - 2 &&
      hoveredScreenRect.sy + hoveredScreenRect.sh <= containerSize.h - 2
    )
  }, [hoveredScreenRect, containerSize])

  const hoveredDiffDetail = useMemo(() => {
    if (!hoveredItem || !diffLens) return null
    if (hoveredItem.type === 'node') {
      return diffLens.diffDetailsByResource.get(diffResourceKey('element', hoveredItem.data.elementId)) ?? null
    }
    if (hoveredItem.type === 'edge' && !hoveredItem.data.isProxy) {
      return hoveredItem.data.id
        ? diffLens.diffDetailsByResource.get(diffResourceKey('connector', hoveredItem.data.id)) ?? null
        : null
    }
    return null
  }, [diffLens, hoveredItem])

  const handleOpenSource = useCallback((detail: ExploreDiffDetail) => {
    if (!detail.sourcePath) return
    api.editor.open({
      editor: getSourceEditor(),
      file_path: detail.sourcePath,
      line: detail.line ?? null,
    }).catch((error: unknown) => {
      toast({
        title: 'Could not open source',
        description: error instanceof Error ? error.message : 'The source editor command failed.',
        status: 'error',
      })
    })
  }, [])

  const [breadcrumbView, setBreadcrumbView] = useState(viewState)
  const breadcrumbTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    if (breadcrumbTimerRef.current) clearTimeout(breadcrumbTimerRef.current)
    breadcrumbTimerRef.current = setTimeout(() => setBreadcrumbView(viewState), 80)
    return () => { if (breadcrumbTimerRef.current) clearTimeout(breadcrumbTimerRef.current) }
  }, [viewState])

  const currentPath = useMemo(() => {
    return getPathAt(breadcrumbView, layout.groups, containerSize.w, containerSize.h)
  }, [breadcrumbView, layout.groups, containerSize])

  useEffect(() => {
    if (!initialized || !versionFollowTarget?.viewId) return
    if (versionFollowTarget.resourceType === 'element' && versionFollowTarget.resourceId) {
      focusElement(versionFollowTarget.viewId, versionFollowTarget.resourceId)
      return
    }
    focusDiagram(versionFollowTarget.viewId)
  }, [focusDiagram, focusElement, initialized, versionFollowTarget?.resourceId, versionFollowTarget?.resourceType, versionFollowTarget?.token, versionFollowTarget?.viewId])

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
        hoveredDiffDetail={hoveredDiffDetail}
        onOpenSource={handleOpenSource}
        onHoverLock={setHoverLocked}
      />
    </div>
  )
})
