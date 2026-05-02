// src/components/ZUI/useZUIInteraction.ts

import { useCallback, useEffect, useRef, useState, useMemo } from 'react'
import type { BBox, DiagramGroupLayout, LayoutNode, ZUIViewState, HoveredItem } from './types'
import { getExpandThresholds } from './renderer'

export function constrainViewState(view: ZUIViewState, canvasW: number, canvasH: number, bbox: BBox): ZUIViewState {
  const padding = Math.min(600, canvasW * 0.45, canvasH * 0.45)
  const minX = padding - bbox.maxX * view.zoom
  const maxX = canvasW - padding - bbox.minX * view.zoom
  const minY = padding - bbox.maxY * view.zoom
  const maxY = canvasH - padding - bbox.minY * view.zoom

  let { x, y } = view
  if (maxX >= minX) x = Math.max(minX, Math.min(maxX, x))
  else x = (minX + maxX) / 2

  if (maxY >= minY) y = Math.max(minY, Math.min(maxY, y))
  else y = (minY + maxY) / 2

  return { ...view, x, y }
}

interface DeepestNodeResult {
  node: LayoutNode
  absX: number
  absY: number
  absW: number
  absH: number
  cumulativeScale: number
}

interface NodeSpatialIndex {
  cellSize: number
  cells: Map<string, LayoutNode[]>
}

const NODE_INDEX_CELL_SIZE = 320
const nodeSpatialIndexCache = new WeakMap<LayoutNode[], NodeSpatialIndex>()

function getNodeSpatialIndex(nodes: LayoutNode[]): NodeSpatialIndex {
  const cached = nodeSpatialIndexCache.get(nodes)
  if (cached) return cached

  const index: NodeSpatialIndex = { cellSize: NODE_INDEX_CELL_SIZE, cells: new Map() }
  for (const node of nodes) {
    const startX = Math.floor(node.worldX / index.cellSize)
    const endX = Math.floor((node.worldX + node.worldW) / index.cellSize)
    const startY = Math.floor(node.worldY / index.cellSize)
    const endY = Math.floor((node.worldY + node.worldH) / index.cellSize)

    for (let cx = startX; cx <= endX; cx++) {
      for (let cy = startY; cy <= endY; cy++) {
        const key = cellKey(cx, cy)
        let bucket = index.cells.get(key)
        if (!bucket) {
          bucket = []
          index.cells.set(key, bucket)
        }
        bucket.push(node)
      }
    }
  }

  nodeSpatialIndexCache.set(nodes, index)
  return index
}

function getNodesAtPoint(nodes: LayoutNode[], worldX: number, worldY: number): LayoutNode[] {
  const index = getNodeSpatialIndex(nodes)
  return index.cells.get(cellKey(Math.floor(worldX / index.cellSize), Math.floor(worldY / index.cellSize))) ?? []
}

function warmNodeSpatialIndexes(nodes: LayoutNode[]): void {
  getNodeSpatialIndex(nodes)
  for (const node of nodes) {
    if (node.children.length > 0) warmNodeSpatialIndexes(node.children)
  }
}

function findDeepestAt(worldX: number, worldY: number, groups: DiagramGroupLayout[], view: ZUIViewState, thresholds: { start: number, end: number }): DeepestNodeResult | null {
  for (const group of groups) {
    if (worldX >= group.worldX && worldX <= group.worldX + group.worldW &&
      worldY >= group.worldY && worldY <= group.worldY + group.worldH) {
      // Root nodes in the group have absolute world coordinates already
      return findDeepestInNodes(worldX, worldY, group.nodes, 0, 0, 1, 0, 0, view, thresholds)
    }
  }
  return null
}

function findDeepestInNodes(
  worldX: number,
  worldY: number,
  nodes: LayoutNode[],
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
  view: ZUIViewState,
  thresholds: { start: number, end: number }
): DeepestNodeResult | null {
  const candidates = getNodesAtPoint(nodes, worldX, worldY)
  for (const node of candidates) {
    if (worldX >= node.worldX && worldX <= node.worldX + node.worldW &&
      worldY >= node.worldY && worldY <= node.worldY + node.worldH) {

      // Screen width of this node at current zoom level
      const worldW = node.worldW * parentAbsScale
      const screenW = worldW * view.zoom

      // Visibility check: if node is too small to be drawn, skip it
      if (screenW < 2) continue

      // Absolute world position of THIS node
      const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
      const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
      const absW = worldW
      const absH = node.worldH * parentAbsScale

      const childX = (worldX - node.worldX) / node.childScale + node.childOffsetX
      const childY = (worldY - node.worldY) / node.childScale + node.childOffsetY

      const hasChildren = node.children && node.children.length > 0
      const t = hasChildren ? Math.max(0, Math.min(1, (screenW - thresholds.start) / (thresholds.end - thresholds.start))) : 0

      // If children are significantly visible, descend
      if (t > 0.05) {
        const deeper = findDeepestInNodes(
          childX,
          childY,
          node.children,
          absX,
          absY,
          parentAbsScale * node.childScale,
          node.childOffsetX,
          node.childOffsetY,
          view,
          thresholds
        )
        if (deeper) return deeper
      }

      // If the node has fully transitioned to its children, the parent itself is no longer hoverable
      if (t > 0.95) return null

      return { node, absX, absY, absW, absH, cumulativeScale: parentAbsScale }
    }
  }
  return null
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

type IndexedEdge =
  | {
    kind: 'edge'
    x1: number
    y1: number
    x2: number
    y2: number
    midX: number
    midY: number
    sourceLabel: string
    targetLabel: string
    label: string
    diagramId: number
    sourceObjId: number
    targetObjId: number
  }
  | {
    kind: 'portal'
    x1: number
    y1: number
    x2: number
    y2: number
    midX: number
    midY: number
    sourceLabel: string
    targetLabel: string
    diagramId: number
    targetDiagId?: number
  }

interface EdgeSpatialIndex {
  cellSize: number
  cells: Map<string, IndexedEdge[]>
}

const EDGE_INDEX_CELL_SIZE = 360

function cellKey(cx: number, cy: number): string {
  return `${cx},${cy}`
}

function addEdgeToSpatialIndex(index: EdgeSpatialIndex, edge: IndexedEdge): void {
  const minX = Math.min(edge.x1, edge.x2)
  const maxX = Math.max(edge.x1, edge.x2)
  const minY = Math.min(edge.y1, edge.y2)
  const maxY = Math.max(edge.y1, edge.y2)
  const startX = Math.floor(minX / index.cellSize)
  const endX = Math.floor(maxX / index.cellSize)
  const startY = Math.floor(minY / index.cellSize)
  const endY = Math.floor(maxY / index.cellSize)

  for (let cx = startX; cx <= endX; cx++) {
    for (let cy = startY; cy <= endY; cy++) {
      const key = cellKey(cx, cy)
      let bucket = index.cells.get(key)
      if (!bucket) {
        bucket = []
        index.cells.set(key, bucket)
      }
      bucket.push(edge)
    }
  }
}

function buildEdgeSpatialIndex(groups: DiagramGroupLayout[]): EdgeSpatialIndex {
  const index: EdgeSpatialIndex = { cellSize: EDGE_INDEX_CELL_SIZE, cells: new Map() }

  for (const group of groups) {
    const nodeMap = new Map<string, LayoutNode>()
    for (const node of group.nodes) {
      nodeMap.set(node.id, node)
    }

    for (const edge of group.edges) {
      const source = nodeMap.get(edge.sourceId)
      const target = nodeMap.get(edge.targetId)
      if (!source || !target) continue

      const x1 = source.worldX + source.worldW / 2
      const y1 = source.worldY + source.worldH / 2
      const x2 = target.worldX + target.worldW / 2
      const y2 = target.worldY + target.worldH / 2
      addEdgeToSpatialIndex(index, {
        kind: 'edge',
        x1,
        y1,
        x2,
        y2,
        midX: (x1 + x2) / 2,
        midY: (y1 + y2) / 2,
        sourceLabel: source.label,
        targetLabel: target.label,
        label: edge.label || 'Connection',
        diagramId: group.diagramId,
        sourceObjId: source.elementId,
        targetObjId: target.elementId,
      })
    }

    for (const node of group.nodes) {
      if (!node.isPortal) continue
      const x1 = group.worldX + group.diagramX + group.diagramW / 2
      const y1 = group.worldY + group.diagramY + group.diagramH
      const x2 = node.worldX + node.worldW / 2
      const y2 = node.worldY
      addEdgeToSpatialIndex(index, {
        kind: 'portal',
        x1,
        y1,
        x2,
        y2,
        midX: (x1 + x2) / 2,
        midY: (y1 + y2) / 2,
        sourceLabel: group.label,
        targetLabel: node.label,
        diagramId: group.diagramId,
        targetDiagId: node.linkedDiagramId,
      })
    }
  }

  return index
}

function findHoveredEdge(
  worldX: number,
  worldY: number,
  index: EdgeSpatialIndex,
  view: ZUIViewState
): HoveredItem | null {
  const threshold = 18 / view.zoom // 18 screen pixels converted to world distance
  const startX = Math.floor((worldX - threshold) / index.cellSize)
  const endX = Math.floor((worldX + threshold) / index.cellSize)
  const startY = Math.floor((worldY - threshold) / index.cellSize)
  const endY = Math.floor((worldY + threshold) / index.cellSize)
  const thresholdSquared = threshold * threshold
  let bestEdge: IndexedEdge | null = null
  let bestDistSquared = thresholdSquared

  for (let cx = startX; cx <= endX; cx++) {
    for (let cy = startY; cy <= endY; cy++) {
      const bucket = index.cells.get(cellKey(cx, cy))
      if (!bucket) continue

      for (const edge of bucket) {
        const dx = edge.x2 - edge.x1
        const dy = edge.y2 - edge.y1
        const l2 = dx * dx + dy * dy
        if (l2 === 0) continue

        let t = ((worldX - edge.x1) * dx + (worldY - edge.y1) * dy) / l2
        t = Math.max(0, Math.min(1, t))

        const nearestX = edge.x1 + t * dx
        const nearestY = edge.y1 + t * dy
        const distSquared = (worldX - nearestX) ** 2 + (worldY - nearestY) ** 2
        if (distSquared < bestDistSquared) {
          bestDistSquared = distSquared
          bestEdge = edge
        }
      }
    }
  }

  if (!bestEdge) return null
  if (bestEdge.kind === 'portal') {
    return {
      type: 'edge',
      data: {
        sourceId: bestEdge.sourceLabel,
        targetId: bestEdge.targetLabel,
        label: '',
        diagramId: bestEdge.diagramId,
        targetDiagId: bestEdge.targetDiagId,
        isPortalConn: true
      },
      absX: bestEdge.midX,
      absY: bestEdge.midY
    }
  }

  return {
    type: 'edge',
    data: {
      sourceId: bestEdge.sourceLabel,
      targetId: bestEdge.targetLabel,
      label: bestEdge.label,
      diagramId: bestEdge.diagramId,
      sourceObjId: bestEdge.sourceObjId,
      targetObjId: bestEdge.targetObjId
    },
    absX: bestEdge.midX,
    absY: bestEdge.midY
  }
}

export function calculateMaxZoom(groups: DiagramGroupLayout[], canvasW: number): number {
  if (canvasW <= 0) return 40
  const thresholds = getExpandThresholds(canvasW)
  let maxPossibleZoom = 40

  function visitNodes(nodes: LayoutNode[], cumulativeScale: number) {
    for (const node of nodes) {
      if (!node.children || node.children.length === 0) {
        // This is a leaf node. We want it to be able to fill 'thresholds.end' of the canvas.
        const neededZoom = thresholds.end / (node.worldW * cumulativeScale)
        if (neededZoom > maxPossibleZoom) {
          maxPossibleZoom = neededZoom
        }
      } else {
        visitNodes(node.children, cumulativeScale * node.childScale)
      }
    }
  }

  for (const group of groups) {
    visitNodes(group.nodes, 1)
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
function zoomAround(
  view: ZUIViewState,
  focalX: number,
  focalY: number,
  factor: number,
  maxZoom: number,
): ZUIViewState {
  const newZoom = clampZoom(view.zoom * factor, view.zoom, maxZoom)
  const scale = newZoom / view.zoom
  return {
    zoom: newZoom,
    x: focalX - (focalX - view.x) * scale,
    y: focalY - (focalY - view.y) * scale,
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
  /** Set to true to prevent clearing hoveredItem (e.g. when mouse is over a popover). */
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
): ZUIInteraction {
  const [viewState, setViewStateInternal] = useState<ZUIViewState>(initialView)
  const [hoveredItem, setHoveredItemInternal] = useState<HoveredItem | null>(null)
  const hoverLockedRef = useRef(false)
  const hoverTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

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
        if (!hoverLockedRef.current) {
          setHoveredItemInternal(null)
        }
      }, 100)
    } else {
      setHoveredItemInternal(item)
    }
  }, [])

  const setHoverLocked = useCallback((locked: boolean) => {
    hoverLockedRef.current = locked
    if (locked && hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current)
      hoverTimeoutRef.current = null
    }
    if (!locked) {
      // If we unlock and there is no item currently being "detected" by mouse,
      // it should ideally clear soon. The next mouse move will handle it.
    }
  }, [])

  // ── Refs for stable event handlers ──────────────────────────────
  const viewStateRef = useRef<ZUIViewState>(initialView)
  const groupsRef = useRef<DiagramGroupLayout[]>(groups)
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
      warmNodeSpatialIndexes(group.nodes)
    }
    bboxRef.current = bbox
    onZoomRef.current = onZoom
    onPanRef.current = onPan
  }, [groups, bbox, onZoom, onPan])

  const [lastCanvasW, setLastCanvasW] = useState(0)

  const dynamicMaxZoom = useMemo(() => {
    return calculateMaxZoom(groups, lastCanvasW || 1200) // Fallback width for initial calc
  }, [groups, lastCanvasW])

  const maxZoomRef = useRef(40)
  useEffect(() => {
    maxZoomRef.current = dynamicMaxZoom
  }, [dynamicMaxZoom])

  const setViewState = useCallback(
    (update: React.SetStateAction<ZUIViewState>) => {
      setViewStateInternal((prev) => {
        const next = typeof update === 'function' ? (update as (p: ZUIViewState) => ZUIViewState)(prev) : update
        const box = bboxRef.current
        if (!box || !canvasRef.current) {
          viewStateRef.current = next
          return next
        }
        const el = canvasRef.current
        const w = el.clientWidth || el.width / (window.devicePixelRatio || 1)
        const h = el.clientHeight || el.height / (window.devicePixelRatio || 1)

        if (w !== lastCanvasW && w > 0) {
          setLastCanvasW(w)
        }

        if (w === 0 || h === 0) {
          viewStateRef.current = next
          return next
        }
        const constrained = constrainViewState(next, w, h, box)
        viewStateRef.current = constrained
        return constrained
      })
    },
    [canvasRef, lastCanvasW],
  )

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
      if (canvasW !== lastCanvasW && canvasW > 0) {
        setLastCanvasW(canvasW)
      }
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
    [setViewState, lastCanvasW],
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
      const isMouseWheel = e.deltaMode !== 0 || (e.deltaX === 0 && Number.isInteger(e.deltaY) && Math.abs(e.deltaY) >= 20)

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
      const isNotchedWheel = !isRecentMultiTouch && e.deltaX === 0 && Number.isInteger(e.deltaY) && Math.abs(e.deltaY) >= 20
      const isRealMouseWheel = e.deltaMode !== 0 || isNotchedWheel

      if (isPinch || isRealMouseWheel) {
        const focalX = e.clientX - rect.left
        const focalY = e.clientY - rect.top

        // Use standard factors for zoom
        let factor = 1 - e.deltaY * (isRealMouseWheel ? 0.002 : 0.01)
        factor = Math.max(0.85, Math.min(1.15, factor))

        setViewState((prev) => {
          const worldX = (focalX - prev.x) / prev.zoom
          const worldY = (focalY - prev.y) / prev.zoom
          const thresholds = getExpandThresholds(rect.width)
          const deepest = findDeepestAt(worldX, worldY, groupsRef.current, prev, thresholds)

          let currentMaxZ = maxZoomRef.current

          if (deepest && (!deepest.node.children || deepest.node.children.length === 0)) {
            currentMaxZ = thresholds.end / (deepest.node.worldW * deepest.cumulativeScale)
          }

          return zoomAround(prev, focalX, focalY, factor, currentMaxZ)
        })
        onZoomRef.current?.()
      } else if (!isMobile) {
        // Trackpad panning - disabled on mobile to avoid interference with pinch-to-zoom
        setViewState((prev) => ({ ...prev, x: prev.x - e.deltaX, y: prev.y - e.deltaY }))
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
      if (hoverLockedRef.current) return

      const rect = el!.getBoundingClientRect()
      const screenX = e.clientX - rect.left
      const screenY = e.clientY - rect.top

      if (dragging.current) {
        const dx = e.clientX - lastMouse.current.x
        const dy = e.clientY - lastMouse.current.y
        lastMouse.current.x = e.clientX
        lastMouse.current.y = e.clientY
        setViewState((prev) => ({ ...prev, x: prev.x + dx, y: prev.y + dy }))
        onPanRef.current?.()
        return
      }

      // Hover detection
      const view = viewStateRef.current
      const worldX = (screenX - view.x) / view.zoom
      const worldY = (screenY - view.y) / view.zoom
      const thresholds = getExpandThresholds(rect.width)

      const deepest = findDeepestAt(worldX, worldY, groupsRef.current, view, thresholds)
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
        const worldX = (focalX - prev.x) / prev.zoom
        const worldY = (focalY - prev.y) / prev.zoom
        const thresholds = getExpandThresholds(rect.width)
        const deepest = findDeepestAt(worldX, worldY, groupsRef.current, prev, thresholds)

        let currentMaxZ = maxZoomRef.current

        if (deepest && (!deepest.node.children || deepest.node.children.length === 0)) {
          currentMaxZ = thresholds.end / (deepest.node.worldW * deepest.cumulativeScale)
        }

        return zoomAround(prev, focalX, focalY, 2, currentMaxZ)
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
        setViewState((prev) => ({ ...prev, x: prev.x + dx, y: prev.y + dy }))
        onPanRef.current?.()
      } else if (e.touches.length >= 2) {
        const dist = pinchDist(e.touches)
        const mid = pinchMid(e.touches)
        if (lastPinchDist.current !== null && lastPinchDist.current > 0) {
          const factor = dist / lastPinchDist.current
          const dx = mid.x - lastPinchMid.current.x
          const dy = mid.y - lastPinchMid.current.y

          if (isFinite(factor) && factor > 0) {
            setViewState((prev) => {
              const rect = el!.getBoundingClientRect()
              const worldX = (mid.x - prev.x) / prev.zoom
              const worldY = (mid.y - prev.y) / prev.zoom
              const thresholds = getExpandThresholds(rect.width)
              const deepest = findDeepestAt(worldX, worldY, groupsRef.current, prev, thresholds)

              let currentMaxZ = maxZoomRef.current

              if (deepest && (!deepest.node.children || deepest.node.children.length === 0)) {
                currentMaxZ = thresholds.end / (deepest.node.worldW * deepest.cumulativeScale)
              }

              const zoomed = zoomAround(prev, mid.x, mid.y, factor, currentMaxZ)
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
  }, [canvasRef, setViewState, setHoveredItem, isMobile, resolveHoveredProxyItem]) // groupsRef handles groups updates without re-binding!

  return { viewState, viewStateRef, setViewState, fitView, maxZoom: dynamicMaxZoom, hoveredItem, setHoveredItem, setHoverLocked }
}
