// src/components/ZUI/ZUICanvas.tsx

import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState, useCallback } from 'react'
import {
  Box,
  Text,
  Icon,
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverBody,
  PopoverHeader,
  PopoverArrow,
  Button,
  VStack,
  HStack,
  Badge,
  Divider,
  Portal,
  Image as ChakraImage,
  useBreakpointValue,
} from '@chakra-ui/react'
import { Link as RouterLink } from 'react-router-dom'
import { ExternalLinkIcon } from '@chakra-ui/icons'
import type { ExploreData } from '../../types'
import { computeLayout } from './layout'
import { renderFrame, getExpandThresholds, setOnImageLoadCallback, setHighlightedTags as setRendererHighlightedTags, setHiddenTags as setRendererHiddenTags, setHighlightColor as setRendererHighlightColor } from './renderer'
import { useZUIInteraction } from './useZUIInteraction'
import type { DiagramGroupLayout, ZUIViewState } from './types'
import { buildWorkspaceGraphSnapshot } from '../../crossBranch/graph'
import type { CrossBranchContextSettings } from '../../crossBranch/types'
import type { ZUIResolvedConnector } from '../../crossBranch/resolve'
import {
  buildVisibleProxyConnectors,
  collectVisibleNodeAnchors,
  drawVisibleDirectProxyBadges,
  drawVisibleProxyConnectors,
  findHoveredProxyConnector,
} from './proxy'

export interface ZUICanvasHandle {
  fitView(): void
  focusDiagram(viewId: number): boolean
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
  crossBranchSettings: CrossBranchContextSettings
  hoverLocked?: boolean
}

interface PathItem {
  id: string
  label: string
  type: 'group' | 'node'
  isCircular?: boolean
  // Absolute world coordinates for zooming
  absX: number
  absY: number
  absW: number
  absH: number
}

function getPathAt(
  view: ZUIViewState,
  groups: DiagramGroupLayout[],
  canvasW: number,
  canvasH: number,
): PathItem[] {
  if (canvasW === 0 || canvasH === 0) return []

  // World center of the screen
  const worldCenterX = (canvasW / 2 - view.x) / view.zoom
  const worldCenterY = (canvasH / 2 - view.y) / view.zoom
  const thresholds = getExpandThresholds(canvasW)

  for (const group of groups) {
    if (
      worldCenterX >= group.worldX &&
      worldCenterX <= group.worldX + group.worldW &&
      worldCenterY >= group.worldY &&
      worldCenterY <= group.worldY + group.worldH
    ) {
      const path: PathItem[] = [
        {
          id: `g-${group.diagramId}`,
          label: group.label,
          type: 'group',
          absX: group.worldX,
          absY: group.worldY,
          absW: group.worldW,
          absH: group.worldH,
        },
      ]

      let currentNodes = group.nodes
      let currentX = worldCenterX
      let currentY = worldCenterY

      // Track cumulative transform for children
      // We start at 0 because root-level nodes already have absolute worldX/Y
      let parentAbsX = 0
      let parentAbsY = 0
      let parentAbsScale = 1
      let parentChildOffsetX = 0
      let parentChildOffsetY = 0

      while (true) {
        let found = false
        for (const node of currentNodes) {
          if (
            currentX >= node.worldX &&
            currentX <= node.worldX + node.worldW &&
            currentY >= node.worldY &&
            currentY <= node.worldY + node.worldH
          ) {
            // Absolute position of this node
            const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
            const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
            const absW = node.worldW * parentAbsScale
            const absH = node.worldH * parentAbsScale

            // Screen width of this node to check expansion
            const screenW = absW * view.zoom

            path.push({
              id: node.id,
              label: node.label,
              type: 'node',
              isCircular: node.isCircular,
              absX,
              absY,
              absW,
              absH,
            })

            // Only descend if the node is "expanded" enough on screen
            // and has children. This makes the breadcrumb track the visual focus.
            const isExpanded = screenW > thresholds.start * 1.1

            if (isExpanded && node.children && node.children.length > 0) {
              // Update parent context for next level
              parentAbsX = absX
              parentAbsY = absY
              parentAbsScale = parentAbsScale * node.childScale
              parentChildOffsetX = node.childOffsetX
              parentChildOffsetY = node.childOffsetY

              // Transform current center into child-local space
              currentX = (currentX - node.worldX) / node.childScale + node.childOffsetX
              currentY = (currentY - node.worldY) / node.childScale + node.childOffsetY
              currentNodes = node.children
              found = true
              break
            } else {
              // Stop breadcrumb at the deepest visible/centered node
              found = false
              break
            }
          }
        }
        if (!found) break
      }
      return path
    }
  }
  return []
}

function findDiagramFocusTarget(groups: DiagramGroupLayout[], viewId: number): PathItem | null {
  for (const group of groups) {
    if (group.diagramId === viewId) {
      return {
        id: `g-${group.diagramId}`,
        label: group.label,
        type: 'group',
        absX: group.worldX,
        absY: group.worldY,
        absW: group.worldW,
        absH: group.worldH,
      }
    }

    const found = findLinkedDiagramInNodes(viewId, group.nodes, 0, 0, 1, 0, 0)
    if (found) return found
  }
  return null
}

function findLinkedDiagramInNodes(
  viewId: number,
  nodes: DiagramGroupLayout['nodes'],
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
): PathItem | null {
  for (const node of nodes) {
    const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
    const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
    const absW = node.worldW * parentAbsScale
    const absH = node.worldH * parentAbsScale

    if (node.linkedDiagramId === viewId) {
      return {
        id: node.id,
        label: node.linkedDiagramLabel || node.label,
        type: 'node',
        isCircular: node.isCircular,
        absX,
        absY,
        absW,
        absH,
      }
    }

    if (node.children.length > 0) {
      const found = findLinkedDiagramInNodes(
        viewId,
        node.children,
        absX,
        absY,
        parentAbsScale * node.childScale,
        node.childOffsetX,
        node.childOffsetY,
      )
      if (found) return found
    }
  }
  return null
}

function easeOutQuart(t: number): number {
  return 1 - Math.pow(1 - t, 4)
}

function clamp01(value: number): number {
  return Math.max(0, Math.min(1, value))
}

function fitWorldRect(
  rect: { x: number; y: number; w: number; h: number },
  canvasW: number,
  canvasH: number,
  maxZoom: number,
  padding: number,
): ZUIViewState | null {
  const bboxW = Math.max(1, rect.w)
  const bboxH = Math.max(1, rect.h)
  const zoom = Math.min(
    (canvasW * (1 - padding * 2)) / bboxW,
    (canvasH * (1 - padding * 2)) / bboxH,
    maxZoom,
  )
  if (!Number.isFinite(zoom) || zoom <= 0) return null

  return {
    x: (canvasW - bboxW * zoom) / 2 - rect.x * zoom,
    y: (canvasH - bboxH * zoom) / 2 - rect.y * zoom,
    zoom,
  }
}

function findFirstExpandableNode(groups: DiagramGroupLayout[]): PathItem | null {
  for (const group of groups) {
    const found = findFirstExpandableNodeInTree(group.nodes, 0, 0, 1, 0, 0)
    if (found) return found
  }
  return null
}

function findFirstExpandableNodeInTree(
  nodes: DiagramGroupLayout['nodes'],
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
): PathItem | null {
  for (const node of nodes) {
    const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
    const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
    const absW = node.worldW * parentAbsScale
    const absH = node.worldH * parentAbsScale

    if (node.children.length > 0) {
      return {
        id: node.id,
        label: node.linkedDiagramLabel || node.label,
        type: 'node',
        isCircular: node.isCircular,
        absX,
        absY,
        absW,
        absH,
      }
    }

    const found = findFirstExpandableNodeInTree(
      node.children,
      absX,
      absY,
      parentAbsScale * node.childScale,
      node.childOffsetX,
      node.childOffsetY,
    )
    if (found) return found
  }
  return null
}

export const ZUICanvas = forwardRef<ZUICanvasHandle, Props>(function ZUICanvas({ data, onReady, onZoom, onPan, initialCameraFrame, highlightedTags, highlightColor, hiddenTags, crossBranchSettings, hoverLocked = false }, ref) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const cameraTransitionRef = useRef<number | null>(null)
  const [initialized, setInitialized] = useState(false)
  const [containerSize, setContainerSize] = useState({ w: 0, h: 0 })
  const isMobileLayout = useBreakpointValue({ base: true, md: false }) ?? false

  // ── Layout ──────────────────────────────────────────────────────
  const layout = useMemo(() => computeLayout(data), [data])
  const workspaceSnapshot = useMemo(() => buildWorkspaceGraphSnapshot(data), [data])
  // Holds the most-recently resolved connector topology so hover detection can
  // use it without re-running the expensive O(connectors) resolution on every mousemove.
  const proxyConnectorsRef = useRef<ZUIResolvedConnector[]>([])

  const resolveHoveredProxyItem = useCallback((worldX: number, worldY: number, view: ZUIViewState, canvasW: number) => {
    const freshAnchors = collectVisibleNodeAnchors(layout.groups, view, canvasW, hiddenTags)
    return findHoveredProxyConnector(worldX, worldY, proxyConnectorsRef.current, freshAnchors.byNodeId, view)
  }, [hiddenTags, layout.groups])

  // ── Interaction ─────────────────────────────────────────────────
  const { viewState, viewStateRef, setViewState, fitView, maxZoom, hoveredItem, setHoveredItem, setHoverLocked } = useZUIInteraction(
    canvasRef,
    undefined,
    layout.groups,
    layout.bbox,
    onZoom,
    onPan,
    isMobileLayout,
    resolveHoveredProxyItem,
  )

  // Anchor positions recompute every render (fast tree traversal, view-dependent).
  const anchors = useMemo(() =>
    collectVisibleNodeAnchors(layout.groups, viewState, containerSize.w || 1, hiddenTags),
    [layout.groups, viewState, containerSize.w, hiddenTags],
  )

  // A stable string key encoding which element→nodeId pairs are currently visible.
  // This only changes when nodes cross zoom-expansion thresholds not on every pan pixel.
  const visibleElementSig = useMemo(() =>
    Array.from(anchors.visibleAnchors.entries())
      .sort(([a], [b]) => a - b)
      .map(([id, anchor]) => `${id}:${anchor.nodeId}`)
      .join(','),
    [anchors.visibleAnchors],
  )

  // Connector topology: expensive O(connectors) resolution only when visibility set changes.
  const proxyConnectors = useMemo(() => {
    const resolved = buildVisibleProxyConnectors(workspaceSnapshot, anchors.visibleAnchors, crossBranchSettings)
    proxyConnectorsRef.current = resolved.connectors
    return resolved
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceSnapshot, visibleElementSig, crossBranchSettings])

  const visibleProxyState = useMemo(() => ({
    ...anchors,
    proxyConnectors: proxyConnectors.connectors,
    hiddenProxyBadges: proxyConnectors.hiddenBadges,
  }), [anchors, proxyConnectors])

  const visibleProxyStateRef = useRef(visibleProxyState)
  visibleProxyStateRef.current = visibleProxyState

  const labelBgRef = useRef('#171923')
  useEffect(() => {
    const update = () => {
      labelBgRef.current = getComputedStyle(document.documentElement).getPropertyValue('--chakra-colors-gray-900').trim() || '#171923'
      needsRedrawRef.current = true
    }
    update()
    const mo = new MutationObserver(update)
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] })
    return () => mo.disconnect()
  }, [])

  // ── Hierarchy Breadcrumb ────────────────────────────────────────
  const hoveredScreenRect = useMemo(() => {
    if (!hoveredItem) return null
    let absX, absY, absW, absH
    if (hoveredItem.type === 'node') {
      ({ absX, absY, absW, absH } = hoveredItem)
    } else if (hoveredItem.type === 'edge') {
      ({ absX, absY } = hoveredItem)
      absW = 0
      absH = 0
    } else {
      const g = hoveredItem.data
      // Target the label area (centered above diagram)
      absW = 200 / viewState.zoom
      absH = 50 / viewState.zoom
      absX = g.worldX + g.diagramX + g.diagramW / 2 - absW / 2
      absY = g.worldY + g.diagramY - absH
    }

    const sx = absX * viewState.zoom + viewState.x
    const sy = absY * viewState.zoom + viewState.y
    const sw = absW * viewState.zoom
    const sh = absH * viewState.zoom

    return { sx, sy, sw, sh }
  }, [hoveredItem, viewState])

  const isHoveredItemFullyVisible = useMemo(() => {
    if (!hoveredScreenRect || containerSize.w === 0) return false
    // A target is "fully visible" if its screen-space rect is entirely within viewport.
    return (
      hoveredScreenRect.sx >= 2 &&
      hoveredScreenRect.sy >= 2 &&
      hoveredScreenRect.sx + hoveredScreenRect.sw <= containerSize.w - 2 &&
      hoveredScreenRect.sy + hoveredScreenRect.sh <= containerSize.h - 2
    )
  }, [hoveredScreenRect, containerSize])

  // Debounce breadcrumb computation so getPathAt doesn't run on every scroll tick
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

  const zoomToPathItem = useCallback((item: PathItem) => {
    if (containerSize.w === 0 || containerSize.h === 0) return
    if (cameraTransitionRef.current !== null) {
      cancelAnimationFrame(cameraTransitionRef.current)
      cameraTransitionRef.current = null
    }
    setHoveredItem(null, true) // Clear popover immediately on breadcrumb jump

    // Use a comfortable padding for the focused item
    const padding = 0.15
    const bboxW = item.absW
    const bboxH = item.absH

    const zoom = Math.min(
      (containerSize.w * (1 - padding * 2)) / bboxW,
      (containerSize.h * (1 - padding * 2)) / bboxH,
      maxZoom
    )

    // Center the item exactly
    const x = (containerSize.w - bboxW * zoom) / 2 - item.absX * zoom
    const y = (containerSize.h - bboxH * zoom) / 2 - item.absY * zoom

    setViewState({ x, y, zoom })
  }, [containerSize, maxZoom, setViewState, setHoveredItem])

  const focusDiagram = useCallback((viewId: number) => {
    const el = containerRef.current
    const target = findDiagramFocusTarget(layout.groups, viewId)
    if (!el || !target) return false

    const canvasW = el.offsetWidth
    const canvasH = el.offsetHeight
    if (canvasW === 0 || canvasH === 0) return false

    setHoveredItem(null, true)

    const padding = isMobileLayout ? 0.18 : 0.16
    const bboxW = Math.max(1, target.absW)
    const bboxH = Math.max(1, target.absH)
    const zoom = Math.min(
      (canvasW * (1 - padding * 2)) / bboxW,
      (canvasH * (1 - padding * 2)) / bboxH,
      maxZoom,
    )

    const x = (canvasW - bboxW * zoom) / 2 - target.absX * zoom
    const y = (canvasH - bboxH * zoom) / 2 - target.absY * zoom

    if (cameraTransitionRef.current !== null) {
      cancelAnimationFrame(cameraTransitionRef.current)
      cameraTransitionRef.current = null
    }

    const from = viewStateRef.current
    const to = { x, y, zoom }
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
    return true
  }, [isMobileLayout, layout.groups, maxZoom, setHoveredItem, setViewState, viewStateRef])

  const setCameraFrame = useCallback((frame: ZUICameraFrame) => {
    if (frame.profile !== 'detail-to-overview') return false

    const el = containerRef.current
    if (!el) return false

    const canvasW = el.offsetWidth
    const canvasH = el.offsetHeight
    if (canvasW === 0 || canvasH === 0) return false

    const detailTarget = findFirstExpandableNode(layout.groups)
    const overviewTarget = layout.groups[0]
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
        x: overviewTarget.worldX,
        y: overviewTarget.worldY,
        w: overviewTarget.worldW,
        h: overviewTarget.worldH,
      },
      canvasW,
      canvasH,
      maxZoom,
      0.18,
    )

    if (!detail || !overview) return false

    if (cameraTransitionRef.current !== null) {
      cancelAnimationFrame(cameraTransitionRef.current)
      cameraTransitionRef.current = null
    }

    setHoveredItem(null, true)
    const t = easeOutQuart(clamp01(frame.progress))
    setViewState({
      x: detail.x + (overview.x - detail.x) * t,
      y: detail.y + (overview.y - detail.y) * t,
      zoom: detail.zoom + (overview.zoom - detail.zoom) * t,
    })
    return true
  }, [layout.groups, maxZoom, setHoveredItem, setViewState])

  const fitInitialView = useCallback((w: number, h: number) => {
    if (initialCameraFrame && setCameraFrame(initialCameraFrame)) return
    fitView(w, h, layout.bbox)
  }, [fitView, initialCameraFrame, layout.bbox, setCameraFrame])

  useEffect(() => {
    return () => {
      if (cameraTransitionRef.current !== null) {
        cancelAnimationFrame(cameraTransitionRef.current)
      }
    }
  }, [])

  // ── Fit view on mount and when layout changes ────────────────────
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const { offsetWidth: w, offsetHeight: h } = el

    // Only set as initialized if we have valid dimensions
    if (w > 0 && h > 0) {
      setContainerSize({ w, h })
      fitInitialView(w, h)
      if (!initialized) {
        setInitialized(true)
        onReady?.()
      }
    }
  }, [initialized, onReady, fitInitialView])

  // ── Expose fitView to parent ─────────────────────────────────────
  useImperativeHandle(
    ref,
    () => ({
      fitView() {
        const el = containerRef.current
        if (!el) return
        setHoveredItem(null, true) // Clear popover immediately on fitView
        fitView(el.offsetWidth, el.offsetHeight, layout.bbox)
      },
      focusDiagram,
      setCameraFrame,
    }),
    [fitView, focusDiagram, layout.bbox, setCameraFrame, setHoveredItem],
  )

  // ── RAF render loop ──────────────────────────────────────────────
  // viewStateRef comes from useZUIInteraction updated synchronously on input events,
  // so the RAF loop sees new state immediately without waiting for React to re-render.
  const needsRedrawRef = useRef(true) // Force first frame

  useEffect(() => {
    setOnImageLoadCallback(() => {
      needsRedrawRef.current = true
    })
    return () => setOnImageLoadCallback(null)
  }, [])

  useEffect(() => {
    needsRedrawRef.current = true
  }, [viewState])

  useEffect(() => {
    needsRedrawRef.current = true
  }, [crossBranchSettings])

  // ── HiDPI canvas resize ──────────────────────────────────────────
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

      setContainerSize({ w, h })
      canvas.width = w * dpr
      canvas.height = h * dpr
      canvas.style.width = `${w}px`
      canvas.style.height = `${h}px`
      const ctx = canvas.getContext('2d')
      if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

      needsRedrawRef.current = true // Canvas was cleared!

      // Trigger initialization if it hasn't happened yet
      if (!initialized && w > 0 && h > 0) {
        fitInitialView(w, h)
        setInitialized(true)
        onReady?.()
      }
    }

    const ro = new ResizeObserver(resize)
    ro.observe(container)
    resize()
    return () => ro.disconnect()
  }, [initialized, fitInitialView, onReady])

  useEffect(() => {
    if (!initialized) return // Don't start loop until initialized

    const canvas = canvasRef.current
    const container = containerRef.current
    if (!canvas || !container) return

    let rafId: number
    let lastView = { x: NaN, y: NaN, zoom: NaN } // Force first draw

    function frame() {
      const ctx = canvas!.getContext('2d')
      if (!ctx) { rafId = requestAnimationFrame(frame); return }

      const dpr = window.devicePixelRatio || 1
      const w = container!.offsetWidth
      const h = container!.offsetHeight
      if (w === 0 || h === 0) { rafId = requestAnimationFrame(frame); return }

      const currentView = viewStateRef.current

      // Only redraw when view changed (saves GPU on idle)
      const changed =
        lastView.x !== currentView.x ||
        lastView.y !== currentView.y ||
        lastView.zoom !== currentView.zoom ||
        needsRedrawRef.current

      if (changed) {
        ctx.save()
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
        const occupiedLabelRects = renderFrame(ctx, layout.groups, currentView, w, h)
        ctx.save()
        ctx.translate(currentView.x, currentView.y)
        ctx.scale(currentView.zoom, currentView.zoom)
        drawVisibleProxyConnectors(
          ctx,
          visibleProxyStateRef.current.proxyConnectors,
          visibleProxyStateRef.current.byNodeId,
          currentView.zoom,
          labelBgRef.current,
          occupiedLabelRects,
        )
        drawVisibleDirectProxyBadges(
          ctx,
          visibleProxyStateRef.current.hiddenProxyBadges,
          visibleProxyStateRef.current.byNodeId,
          currentView.zoom,
          labelBgRef.current,
          occupiedLabelRects,
        )
        ctx.restore()
        ctx.restore()
        lastView = currentView
        needsRedrawRef.current = false
      }

      rafId = requestAnimationFrame(frame)
    }

    rafId = requestAnimationFrame(frame)
    return () => cancelAnimationFrame(rafId)
  }, [initialized, layout, viewStateRef])

  // Force draw when layout changes (though NaN in RAF loop should cover it)
  useEffect(() => {
    if (initialized) needsRedrawRef.current = true
  }, [layout, initialized])

  // Sync highlighted tags + color with renderer module
  useEffect(() => {
    setRendererHighlightedTags(new Set(highlightedTags ?? []))
    setRendererHighlightColor(highlightColor ?? '')
    needsRedrawRef.current = true
  }, [highlightedTags, highlightColor])

  // Sync hidden tags with renderer module
  useEffect(() => {
    setRendererHiddenTags(new Set(hiddenTags ?? []))
    needsRedrawRef.current = true
  }, [hiddenTags])

  useEffect(() => {
    setHoverLocked(hoverLocked)
  }, [hoverLocked, setHoverLocked])

  // Clear renderer state on unmount
  useEffect(() => {
    return () => {
      setRendererHighlightedTags(new Set())
      setRendererHighlightColor('')
      setRendererHiddenTags(new Set())
    }
  }, [])

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

      {/* Breadcrumb Overlay */}
      {initialized && currentPath.length > 0 && (
        <Box
          position="absolute"
          top={isMobileLayout ? "66px" : 4}
          left={4}
          zIndex={10}
          className="glass"
          borderRadius="lg"
          px={3}
          py={1.5}
          pointerEvents="auto"
        >
          <Breadcrumb
            spacing="8px"
            separator={<Text color="whiteAlpha.400" fontSize="xs">/</Text>}
          >
            {currentPath.map((item, idx) => (
              <BreadcrumbItem key={item.id} isCurrentPage={idx === currentPath.length - 1}>
                <BreadcrumbLink
                  onClick={() => zoomToPathItem(item)}
                  color={idx === currentPath.length - 1 ? "var(--accent)" : "gray.400"}
                  fontSize="xs"
                  fontWeight={idx === currentPath.length - 1 ? "600" : "normal"}
                  _hover={{ color: "var(--accent)", textDecoration: "none" }}
                  display="flex"
                  alignItems="center"
                  gap={1.5}
                >
                  {item.type === 'group' && (
                    <Icon viewBox="0 0 24 24" boxSize={3} fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                      <polyline points="9 22 9 12 15 12 15 22" />
                    </Icon>
                  )}
                  {item.isCircular && (
                    <Icon viewBox="0 0 24 24" boxSize={3.5} fill="none" stroke="currentColor" strokeWidth="3.5">
                      <path d="M20 4l-4 4 4 4" />
                      <path d="M16 8h-4a8 8 0 1 0 8 8" />
                    </Icon>
                  )}
                  {item.label}
                </BreadcrumbLink>
              </BreadcrumbItem>
            ))}
          </Breadcrumb>
          {currentPath[currentPath.length - 1]?.isCircular && (
            <Text mt={1.5} color="var(--accent)" fontSize="2xs" fontWeight="500" letterSpacing="wide">
              CIRCULAR LINK - CLICK BREADCRUMB TO JUMP BACK
            </Text>
          )}
        </Box>
      )}

      {/* Hover metadata card */}
      <Popover
        isOpen={isHoveredItemFullyVisible}
        placement="right-start"
        closeOnBlur={false}
        gutter={12}
        isLazy
      >
        <PopoverTrigger>
          <Box
            position="absolute"
            left={hoveredScreenRect?.sx ?? 0}
            top={hoveredScreenRect?.sy ?? 0}
            width={hoveredScreenRect?.sw ?? 0}
            height={hoveredScreenRect?.sh ?? 0}
            pointerEvents="none"
          />
        </PopoverTrigger>
        <Portal>
          <PopoverContent
            bg="gray.900"
            borderColor="whiteAlpha.300"
            boxShadow="2xl"
            width="280px"
            _focus={{ boxShadow: 'none' }}
            pointerEvents="auto"
            onMouseEnter={() => setHoverLocked(true)}
            onMouseLeave={() => setHoverLocked(false)}
          >
            <PopoverArrow bg="gray.900" />
            {hoveredItem?.type === 'node' && (
              <>
                <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
                  <HStack spacing={3}>
                    {hoveredItem.data.logoUrl && (
                      <Box flexShrink={0}>
                        <ChakraImage src={hoveredItem.data.logoUrl} boxSize="24px" objectFit="contain" />
                      </Box>
                    )}
                    <VStack align="start" spacing={0} flex={1} overflow="hidden">
                      <Text fontWeight="600" fontSize="sm" isTruncated width="100%" color="white">
                        {hoveredItem.data.label}
                      </Text>
                      <Badge colorScheme={hoveredItem.data.isPortal ? "purple" : "blue"} variant="subtle" fontSize="2xs">
                        {hoveredItem.data.isPortal ? "Portal" : hoveredItem.data.type}
                      </Badge>
                    </VStack>
                  </HStack>
                </PopoverHeader>
                <PopoverBody px={4} py={3}>
                  <VStack align="start" spacing={3}>
                    {hoveredItem.data.technology && (
                      <Box>
                        <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">TECHNOLOGY</Text>
                        <Text fontSize="xs" color="gray.200">{hoveredItem.data.technology}</Text>
                      </Box>
                    )}
                    {hoveredItem.data.description && (
                      <Box>
                        <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">DESCRIPTION</Text>
                        <Text fontSize="xs" color="gray.200" noOfLines={4}>{hoveredItem.data.description}</Text>
                      </Box>
                    )}
                    {hoveredItem.data.linkedDiagramId && (
                      <Box>
                        <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">LINKS TO</Text>
                        <Text fontSize="xs" color="teal.300" fontWeight="500">
                          ⊞ {hoveredItem.data.linkedDiagramLabel}
                        </Text>
                      </Box>
                    )}
                    <Divider borderColor="whiteAlpha.200" />
                    <Button
                      as={RouterLink}
                      to={hoveredItem.data.isPortal
                        ? `/views/${hoveredItem.data.linkedDiagramId}`
                        : `/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.elementId}`
                      }
                      size="xs"
                      colorScheme="teal"
                      variant="solid"
                      width="full"
                      rightIcon={<ExternalLinkIcon />}
                      onClick={(e) => e.stopPropagation()}
                    >
                      {hoveredItem.data.isPortal ? 'Open Diagram' : 'Open in Editor'}
                    </Button>
                  </VStack>
                </PopoverBody>
              </>
            )}
            {hoveredItem?.type === 'edge' && hoveredItem.data.isProxy && hoveredItem.data.details && (
              <>
                <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
                  <VStack align="start" spacing={0}>
                    <Text fontWeight="600" fontSize="sm" color="white">
                      Cross-Branch Connector
                    </Text>
                    <Badge colorScheme="blue" variant="subtle" fontSize="2xs">
                      {hoveredItem.data.details.count} connector{hoveredItem.data.details.count === 1 ? '' : 's'}
                    </Badge>
                  </VStack>
                </PopoverHeader>
                <PopoverBody px={4} py={3}>
                  <VStack align="start" spacing={3}>
                    <VStack align="start" spacing={1}>
                      <Text color="gray.400" fontSize="2xs" fontWeight="600" letterSpacing="wider">BETWEEN</Text>
                      <Text fontSize="xs" color="gray.200">
                        {hoveredItem.data.details.sourceAnchorName} &rarr; {hoveredItem.data.details.targetAnchorName}
                      </Text>
                      <Text fontSize="xs" color="gray.400">{hoveredItem.data.details.label}</Text>
                    </VStack>
                    <VStack align="start" spacing={1} width="full">
                      <Text color="gray.400" fontSize="2xs" fontWeight="600" letterSpacing="wider">UNDERLYING PATHS</Text>
                      {hoveredItem.data.details.connectors.slice(0, 4).map((leaf, index) => (
                        <Text key={`${leaf.connector.id}-${index}`} fontSize="xs" color="gray.200">
                          {leaf.source.actualElementName} &rarr; {leaf.target.actualElementName}
                        </Text>
                      ))}
                      {hoveredItem.data.details.connectors.length > 4 && (
                        <Text fontSize="xs" color="gray.500">
                          +{hoveredItem.data.details.connectors.length - 4} more
                        </Text>
                      )}
                    </VStack>
                    <Divider borderColor="whiteAlpha.200" />
                    <VStack align="stretch" spacing={2} width="full">
                      {hoveredItem.data.details.ownerViewIds.map((ownerViewId, index) => (
                        <Button
                          key={`${ownerViewId}-${index}`}
                          as={RouterLink}
                          to={`/views/${ownerViewId}`}
                          size="xs"
                          colorScheme="gray"
                          variant="solid"
                          width="full"
                          justifyContent="space-between"
                          rightIcon={<ExternalLinkIcon />}
                          onClick={(e) => e.stopPropagation()}
                        >
                          {hoveredItem.data.details!.ownerViewNames[index] ?? `Open View ${ownerViewId}`}
                        </Button>
                      ))}
                    </VStack>
                    <Divider borderColor="whiteAlpha.200" />
                    <HStack width="full" spacing={2}>
                      <Button
                        as={RouterLink}
                        to={`/views/${hoveredItem.data.details!.connectors[0]?.source.anchorViewId ?? hoveredItem.data.diagramId}?element=${hoveredItem.data.sourceObjId}`}
                        size="xs"
                        colorScheme="gray"
                        variant="solid"
                        flex={1}
                        rightIcon={<ExternalLinkIcon />}
                        onClick={(e) => e.stopPropagation()}
                      >
                        Open Source
                      </Button>
                      <Button
                        as={RouterLink}
                        to={`/views/${hoveredItem.data.details!.connectors[0]?.target.anchorViewId ?? hoveredItem.data.diagramId}?element=${hoveredItem.data.targetObjId}`}
                        size="xs"
                        colorScheme="teal"
                        variant="solid"
                        flex={1}
                        rightIcon={<ExternalLinkIcon />}
                        onClick={(e) => e.stopPropagation()}
                      >
                        Open Target
                      </Button>
                    </HStack>
                  </VStack>
                </PopoverBody>
              </>
            )}
            {hoveredItem?.type === 'edge' && !hoveredItem.data.isProxy && (
              <>
                <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
                  <VStack align="start" spacing={0}>
                    <Text fontWeight="600" fontSize="sm" color="white">
                      {hoveredItem.data.label}
                    </Text>
                    <Badge colorScheme={hoveredItem.data.isPortalConn ? "purple" : "orange"} variant="subtle" fontSize="2xs">
                      {hoveredItem.data.isPortalConn ? "Portal Connection" : "Connection"}
                    </Badge>
                  </VStack>
                </PopoverHeader>
                <PopoverBody px={4} py={3}>
                  <VStack align="start" spacing={3}>
                    <VStack align="start" spacing={1}>
                      <Text color="gray.400" fontSize="2xs" fontWeight="600" letterSpacing="wider">BETWEEN</Text>
                      <Text fontSize="xs" color="gray.200">{hoveredItem.data.sourceId} & {hoveredItem.data.targetId}</Text>
                    </VStack>
                    <Divider borderColor="whiteAlpha.200" />

                    {hoveredItem.data.isPortalConn ? (
                      <>
                        <Button
                          as={RouterLink}
                          to={`/views/${hoveredItem.data.diagramId}`}
                          size="xs"
                          colorScheme="gray"
                          variant="solid"
                          width="full"
                          rightIcon={<ExternalLinkIcon />}
                          onClick={(e) => e.stopPropagation()}
                        >
                          Open {hoveredItem.data.sourceId}
                        </Button>
                        <Button
                          as={RouterLink}
                          to={`/views/${hoveredItem.data.targetDiagId}`}
                          size="xs"
                          colorScheme="teal"
                          variant="solid"
                          width="full"
                          rightIcon={<ExternalLinkIcon />}
                          onClick={(e) => e.stopPropagation()}
                        >
                          Open {hoveredItem.data.targetId}
                        </Button>
                      </>
                    ) : (
                      <>
                        <Button
                          as={RouterLink}
                          to={`/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.sourceObjId}`}
                          size="xs"
                          colorScheme="gray"
                          variant="solid"
                          width="full"
                          rightIcon={<ExternalLinkIcon />}
                          onClick={(e) => e.stopPropagation()}
                        >
                          Go to {hoveredItem.data.sourceId}
                        </Button>
                        <Button
                          as={RouterLink}
                          to={`/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.targetObjId}`}
                          size="xs"
                          colorScheme="teal"
                          variant="solid"
                          width="full"
                          rightIcon={<ExternalLinkIcon />}
                          onClick={(e) => e.stopPropagation()}
                        >
                          Go to {hoveredItem.data.targetId}
                        </Button>
                      </>
                    )}
                  </VStack>
                </PopoverBody>
              </>
            )}
            {hoveredItem?.type === 'group' && (
              <>
                <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
                  <VStack align="start" spacing={0}>
                    <Text fontWeight="600" fontSize="sm" color="white">
                      {hoveredItem.data.label}
                    </Text>
                    <Badge colorScheme="purple" variant="subtle" fontSize="2xs">
                      Diagram Group
                    </Badge>
                  </VStack>
                </PopoverHeader>
                <PopoverBody px={4} py={3}>
                  <VStack align="start" spacing={3}>
                    {hoveredItem.data.description && (
                      <Box>
                        <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">DESCRIPTION</Text>
                        <Text fontSize="xs" color="gray.200" noOfLines={4}>{hoveredItem.data.description}</Text>
                      </Box>
                    )}
                    <Text fontSize="xs" color="gray.300">
                      Root level diagram containing {hoveredItem.data.nodes.length} elements and {hoveredItem.data.edges.length} connections.
                    </Text>
                    <Divider borderColor="whiteAlpha.200" />
                    <Button
                      as={RouterLink}
                      to={`/views/${hoveredItem.data.diagramId}`}
                      size="xs"
                      colorScheme="teal"
                      variant="solid"
                      width="full"
                      rightIcon={<ExternalLinkIcon />}
                      onClick={(e) => e.stopPropagation()}
                    >
                      Open Diagram
                    </Button>
                  </VStack>
                </PopoverBody>
              </>
            )}
          </PopoverContent>
        </Portal>
      </Popover>
    </div>
  )
})
