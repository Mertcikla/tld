import { useCallback, useEffect, useRef } from 'react'
import type { Dispatch, MutableRefObject, RefObject, SetStateAction } from 'react'
import type { CrossBranchContextSettings } from '../../crossBranch/types'
import type { WorkspaceVersionFollowTarget, WorkspaceVersionPreview } from '../../context/WorkspaceVersionContext'
import type { ExploreDiffLens } from '../../utils/exploreDiffLens'
import type { ViewLayer } from '../../types'
import type { ZUIViewportBounds } from '../../crossBranch/resolve'
import {
  drawVisibleDirectProxyBadges,
  drawVisibleProxyConnectors,
  type VisibleNodeAnchor,
} from './proxy'
import {
  getCameraRebase,
  getExpandThresholds,
  updateScene,
  type ZUITransitionRebase,
} from './layoutEngine'
import {
  renderFrame,
  setHighlightColor as setRendererHighlightColor,
  setHighlightedTags as setRendererHighlightedTags,
  setHiddenTags as setRendererHiddenTags,
  setOnImageLoadCallback,
  setVersionDiff as setRendererVersionDiff,
  getThemeVars,
  type RenderContext,
} from './renderer'
import { buildSceneGraph, type SceneGraph } from './sceneGraph'
import type { ZUILayout, ZUIViewState } from './types'
import type { ZUIProxyConnectorState } from './useZUIProxyConnectors'

export interface ZUIRenderInvalidator {
  invalidate: () => void
}

export function isPanFrame(previousView: ZUIViewState | null, currentView: ZUIViewState): boolean {
  return previousView !== null && (
    previousView.x !== currentView.x ||
    previousView.y !== currentView.y
  )
}

function rebaseVisibleNodeAnchors(
  anchors: Map<string, VisibleNodeAnchor>,
  originX: number,
  originY: number,
  reusable: Map<string, VisibleNodeAnchor>,
): Map<string, VisibleNodeAnchor> {
  for (const [nodeId, anchor] of anchors) {
    const existing = reusable.get(nodeId)
    if (existing) {
      existing.nodeId = anchor.nodeId
      existing.elementId = anchor.elementId
      existing.label = anchor.label
      existing.worldX = anchor.worldX - originX
      existing.worldY = anchor.worldY - originY
      existing.worldW = anchor.worldW
      existing.worldH = anchor.worldH
      existing.pathDepth = anchor.pathDepth
      existing.renderAlpha = anchor.renderAlpha
      continue
    }

    reusable.set(nodeId, {
      ...anchor,
      worldX: anchor.worldX - originX,
      worldY: anchor.worldY - originY,
    })
  }

  for (const nodeId of reusable.keys()) {
    if (!anchors.has(nodeId)) reusable.delete(nodeId)
  }

  return reusable
}

interface UseZUIRenderLoopArgs {
  canvasRef: RefObject<HTMLCanvasElement | null>
  containerRef: RefObject<HTMLDivElement | null>
  initialized: boolean
  setInitialized: Dispatch<SetStateAction<boolean>>
  setContainerSize: Dispatch<SetStateAction<{ w: number; h: number }>>
  fitInitialView: (w: number, h: number) => void
  onReady?: () => void
  layout: ZUILayout
  viewState: ZUIViewState
  viewStateRef: MutableRefObject<ZUIViewState>
  crossBranchSettings: CrossBranchContextSettings
  proxyState: ZUIProxyConnectorState
  highlightedTags?: string[]
  highlightColor?: string
  hiddenTags?: string[]
  groupLayers: ViewLayer[]
  versionPreview?: WorkspaceVersionPreview | null
  versionFollowTarget?: WorkspaceVersionFollowTarget | null
  diffLens?: ExploreDiffLens | null
}

export function useZUIRenderLoop({
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
  groupLayers,
  versionPreview,
  versionFollowTarget,
  diffLens,
}: UseZUIRenderLoopArgs): ZUIRenderInvalidator {
  const needsRedrawRef = useRef(true)
  const labelBgRef = useRef('#171923')
  const accentRef = useRef('#63b3ed')
  const requestFrameRef = useRef<(() => void) | null>(null)
  const rafIdRef = useRef<number | null>(null)
  const lastViewRef = useRef<ZUIViewState | null>(null)
  const panLowDetailFramesRef = useRef(0)
  const rebasedProxyAnchorsRef = useRef(new Map<string, VisibleNodeAnchor>())
  const proxyStateRef = useRef(proxyState)
  proxyStateRef.current = proxyState
  const sceneGraphRef = useRef<SceneGraph | null>(null)
  const transitionRebaseRef = useRef<ZUITransitionRebase>({ preserveChildAlphaNodeIds: new Set() })

  const invalidate = useCallback(() => {
    needsRedrawRef.current = true
    requestFrameRef.current?.()
  }, [])

  const groupLayersRef = useRef(groupLayers)
  useEffect(() => {
    groupLayersRef.current = groupLayers
    invalidate()
  }, [groupLayers, invalidate])

  useEffect(() => {
    const update = () => {
      const styles = getComputedStyle(document.documentElement)
      labelBgRef.current = styles.getPropertyValue('--chakra-colors-gray-900').trim() || '#171923'
      accentRef.current = styles.getPropertyValue('--accent').trim() || '#63b3ed'
      invalidate()
    }
    update()
    const mo = new MutationObserver(update)
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] })
    return () => mo.disconnect()
  }, [invalidate])

  useEffect(() => {
    setOnImageLoadCallback(invalidate)
    return () => setOnImageLoadCallback(null)
  }, [invalidate])

  useEffect(invalidate, [invalidate, viewState, crossBranchSettings])

  useEffect(() => {
    sceneGraphRef.current = buildSceneGraph(layout)
    invalidate()
  }, [layout, invalidate])

  const fitInitialViewRef = useRef(fitInitialView)
  const onReadyRef = useRef(onReady)
  const setInitializedRef = useRef(setInitialized)
  const setContainerSizeRef = useRef(setContainerSize)
  const initializedRef = useRef(initialized)

  useEffect(() => {
    fitInitialViewRef.current = fitInitialView
    onReadyRef.current = onReady
    setInitializedRef.current = setInitialized
    setContainerSizeRef.current = setContainerSize
    initializedRef.current = initialized
  }, [fitInitialView, onReady, setInitialized, setContainerSize, initialized])

  useEffect(() => {
    const canvas = canvasRef.current
    const container = containerRef.current
    if (!canvas || !container) return

    function resize() {
      if (!canvas || !container) return
      const dpr = window.devicePixelRatio || 1
      const w = container.offsetWidth
      const h = container.offsetHeight
      if (w === 0 || h === 0) return

      const newWidth = Math.floor(w * dpr)
      const newHeight = Math.floor(h * dpr)

      if (canvas.width !== newWidth || canvas.height !== newHeight) {
        canvas.width = newWidth
        canvas.height = newHeight
        canvas.style.width = `${w}px`
        canvas.style.height = `${h}px`
        const ctx = canvas.getContext('2d')
        if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
        invalidate()
      }

      setContainerSizeRef.current({ w, h })

      if (!initializedRef.current && w > 0 && h > 0) {
        fitInitialViewRef.current(w, h)
        setInitializedRef.current(true)
        onReadyRef.current?.()
      }
    }

    const ro = new ResizeObserver(resize)
    ro.observe(container)
    resize()
    return () => ro.disconnect()
  }, [canvasRef, containerRef, invalidate])

  useEffect(() => {
    if (!initialized) return

    const canvas = canvasRef.current
    const container = containerRef.current
    if (!canvas || !container) return

    function requestFrame() {
      if (rafIdRef.current !== null) return
      rafIdRef.current = requestAnimationFrame(() => {
        rafIdRef.current = null
        frame()
      })
    }

    requestFrameRef.current = requestFrame

    function frame() {
      const ctx = canvas!.getContext('2d')
      if (!ctx) return

      const dpr = window.devicePixelRatio || 1
      const w = container!.offsetWidth
      const h = container!.offsetHeight
      if (w === 0 || h === 0) return

      const currentView = viewStateRef.current
      const previousView = lastViewRef.current
      const graph = sceneGraphRef.current
      if (!graph) return

      const changed =
        previousView === null ||
        previousView.x !== currentView.x ||
        previousView.y !== currentView.y ||
        previousView.zoom !== currentView.zoom ||
        needsRedrawRef.current

      if (!changed) return

      const thresholds = getExpandThresholds(w)

      const isPanning = isPanFrame(previousView, currentView)
      if (isPanning) {
        panLowDetailFramesRef.current = 2
      } else if (panLowDetailFramesRef.current > 0) {
        panLowDetailFramesRef.current -= 1
      }
      const lowDetail = panLowDetailFramesRef.current > 0

      transitionRebaseRef.current = updateScene(graph, currentView, w, h, thresholds)

      ctx.save()
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

      const theme = getThemeVars()
      const groupLayersByDiagram = new Map<number, ViewLayer[]>()
      for (const layer of groupLayersRef.current) {
        const diagramLayers = groupLayersByDiagram.get(layer.diagram_id) ?? []
        diagramLayers.push(layer)
        groupLayersByDiagram.set(layer.diagram_id, diagramLayers)
      }
      const renderCtx: RenderContext = {
        canvasBg: theme.canvasBg,
        nodeBg: theme.nodeBg,
        accent: theme.accent,
        labelBg: theme.labelBg,
        canvasW: w,
        canvasH: h,
        thresholds,
        lowDetail,
        groupLayersByDiagram,
      }

      const occupiedLabelRects = renderFrame(ctx, graph, renderCtx, currentView, transitionRebaseRef.current)

      const cameraRebase = getCameraRebase(currentView, w, h)
      const rebasedViewport: ZUIViewportBounds = {
        minX: (-cameraRebase.view.x) / cameraRebase.view.zoom,
        minY: (-cameraRebase.view.y) / cameraRebase.view.zoom,
        maxX: (w - cameraRebase.view.x) / cameraRebase.view.zoom,
        maxY: (h - cameraRebase.view.y) / cameraRebase.view.zoom,
        centerX: (w / 2 - cameraRebase.view.x) / cameraRebase.view.zoom,
        centerY: (h / 2 - cameraRebase.view.y) / cameraRebase.view.zoom,
      }
      const rebasedProxyAnchors = rebaseVisibleNodeAnchors(
        proxyStateRef.current.byNodeId,
        cameraRebase.originX,
        cameraRebase.originY,
        rebasedProxyAnchorsRef.current,
      )
      ctx.save()
      ctx.translate(cameraRebase.view.x, cameraRebase.view.y)
      ctx.scale(cameraRebase.view.zoom, cameraRebase.view.zoom)
      drawVisibleProxyConnectors(
        ctx,
        proxyStateRef.current.proxyConnectorRenderState,
        rebasedProxyAnchors,
        cameraRebase.view.zoom,
        labelBgRef.current,
        accentRef.current,
        occupiedLabelRects,
        !lowDetail,
        rebasedViewport,
      )
      if (!lowDetail) {
        drawVisibleDirectProxyBadges(
          ctx,
          proxyStateRef.current.hiddenProxyBadges,
          rebasedProxyAnchors,
          cameraRebase.view.zoom,
          labelBgRef.current,
          occupiedLabelRects,
          rebasedViewport,
        )
      }
      ctx.restore()
      ctx.restore()
      lastViewRef.current = currentView
      needsRedrawRef.current = false

      if (needsRedrawRef.current) {
        requestFrame()
      }
    }

    requestFrame()
    return () => {
      requestFrameRef.current = null
      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current)
        rafIdRef.current = null
      }
    }
  }, [canvasRef, containerRef, initialized, viewStateRef])

  useEffect(() => {
    if (initialized) invalidate()
  }, [initialized, invalidate, layout])

  useEffect(() => {
    setRendererHighlightedTags(new Set(highlightedTags ?? []))
    setRendererHighlightColor(highlightColor ?? '')
    invalidate()
  }, [highlightedTags, highlightColor, invalidate])

  useEffect(() => {
    setRendererHiddenTags(new Set(hiddenTags ?? []))
    invalidate()
  }, [hiddenTags, invalidate])

  useEffect(() => {
    if (diffLens) {
      setRendererVersionDiff(
        diffLens.elementChanges,
        diffLens.connectorChanges,
        diffLens.elementLineDeltas,
        diffLens.contextElementIds,
        diffLens.contextConnectorIds,
        true,
      )
      invalidate()
      return
    }
    const pulsedElementChanges = new Map<number, string>()
    const pulsedElementLineDeltas = new Map<number, { added: number; removed: number }>()
    if (versionFollowTarget?.resourceType === 'element' && versionFollowTarget.resourceId) {
      const change = versionFollowTarget.changeType ?? versionPreview?.elementChanges.get(versionFollowTarget.resourceId)
      if (change) pulsedElementChanges.set(versionFollowTarget.resourceId, change)
    }
    setRendererVersionDiff(
      pulsedElementChanges,
      versionPreview?.connectorChanges ?? new Map(),
      versionPreview?.elementLineDeltas ?? pulsedElementLineDeltas,
    )
    invalidate()
  }, [diffLens, invalidate, versionPreview, versionFollowTarget])

  useEffect(() => {
    return () => {
      setRendererHighlightedTags(new Set())
      setRendererHighlightColor('')
      setRendererHiddenTags(new Set())
      setRendererVersionDiff(new Map(), new Map())
    }
  }, [])

  return { invalidate }
}
