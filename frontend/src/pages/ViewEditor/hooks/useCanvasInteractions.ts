import { useCallback, useEffect, useRef, useState } from 'react'
import type { DrawingCanvasHandle } from '../../../components/DrawingCanvas'
import {
  applyEdgeChanges,
  applyNodeChanges,
  reconnectEdge,
  type Connection,
  type Edge as RFEdge,
  type EdgeChange,
  type Node as RFNode,
  type NodeChange,
  type NodeDragHandler,
  type OnConnect,
  type OnConnectStartParams,
  useReactFlow,
} from 'reactflow'
import { api } from '../../../api/client'
import type {
  Connector,
  PlacedElement,
  ViewTreeNode,
  LibraryElement,
  ViewLayer,
  ViewConnector,
  IncomingViewConnector,
} from '../../../types'
import { parseNumericId } from '../../../utils/ids'
import { connectorToConnector, findClosestHandles, findClosestHandleToPoint } from '../utils'
import { removePlacementGraphSnapshot, upsertConnectorGraphSnapshot, upsertPlacementGraphSnapshot } from '../../../crossBranch/store'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  HANDLE_SLOT_CENTER_INDEX,
  ensureVisualHandleId,
  getLogicalHandleId,
} from '../../../utils/edgeDistribution'
import { useStore } from '../../../store/useStore'
import { isMouseWheelGesture } from '../../../utils/wheel'

const SNAP_RADIUS = 75
const CONNECTOR_DRAG_UPDATE_INTERVAL_MS = 25
export const PENDING_ELEMENT_NODE_ID = 'pending-element'
const PENDING_ELEMENT_OFFSET_X = 100
const PENDING_ELEMENT_OFFSET_Y = 40

type HandleTarget = {
  nodeId?: string
  handleId: string
  x: number
  y: number
}

export type ConnectorDropTarget = {
  droppedNode: RFNode | null
  droppedHandleId: string | null
}

export type ConnectorDragTarget = {
  nodeId?: string
  isHandle: boolean
}

function resolveElementIdFromNode(node: RFNode | null | undefined) {
  if (!node) return null
  const numericId = parseNumericId(node.id)
  if (numericId !== null) return numericId

  const nodeData = node.data as { element_id?: unknown } | undefined
  return typeof nodeData?.element_id === 'number' ? nodeData.element_id : null
}

function isDomElement(target: unknown): target is Element {
  return typeof globalThis.Element !== 'undefined' && target instanceof globalThis.Element
}

function findNodeFromEventTarget(target: EventTarget | null, nodes: RFNode[]) {
  if (!isDomElement(target)) return null
  const nodeId = target.closest('.react-flow__node')?.getAttribute('data-id')
  if (!nodeId) return null
  return nodes.find((node) => node.id === nodeId) ?? null
}

function findHandleIdFromEventTarget(target: EventTarget | null) {
  if (!isDomElement(target)) return null
  const handle = target.closest('.react-flow__handle')
  return handle?.getAttribute('data-handleid') || handle?.id || null
}

function elementsFromClientPoint(clientX: number, clientY: number) {
  if (typeof document === 'undefined') return null
  if (document.elementsFromPoint) return document.elementsFromPoint(clientX, clientY)
  const element = document.elementFromPoint?.(clientX, clientY)
  return element ? [element] : []
}

export function getConnectorDropTargetAtPoint(clientX: number, clientY: number, nodes: RFNode[]): ConnectorDropTarget | null {
  const targets = elementsFromClientPoint(clientX, clientY)
  if (!targets) return null

  for (const target of targets) {
    const droppedNode = findNodeFromEventTarget(target, nodes)
    const droppedHandleId = findHandleIdFromEventTarget(target)
    if (droppedNode || droppedHandleId) {
      return { droppedNode, droppedHandleId }
    }
  }

  return { droppedNode: null, droppedHandleId: null }
}

export function resolveConnectorDropTarget(eventTarget: EventTarget | null, clientX: number, clientY: number, nodes: RFNode[]): ConnectorDropTarget {
  return getConnectorDropTargetAtPoint(clientX, clientY, nodes) ?? {
    droppedNode: findNodeFromEventTarget(eventTarget, nodes),
    droppedHandleId: findHandleIdFromEventTarget(eventTarget),
  }
}

export function shouldDisplayConnectorDragPlaceholder(target: ConnectorDragTarget | null) {
  if (!target) return true
  if (target.isHandle) return false
  return !target.nodeId || target.nodeId === PENDING_ELEMENT_NODE_ID
}

function getConnectorDragTargetAtPoint(clientX: number, clientY: number): ConnectorDragTarget | null {
  const targets = document.elementsFromPoint?.(clientX, clientY) ?? [document.elementFromPoint(clientX, clientY)]
  let sawPendingNode = false

  for (const target of targets) {
    if (!isDomElement(target)) continue
    const nodeId = target.closest('.react-flow__node')?.getAttribute('data-id') || undefined
    const isHandle = !!target.closest('.react-flow__handle')
    if (isHandle && nodeId !== PENDING_ELEMENT_NODE_ID) return { nodeId, isHandle: true }
    if (nodeId === PENDING_ELEMENT_NODE_ID) {
      sawPendingNode = true
      continue
    }
    if (nodeId) return { nodeId, isHandle: false }
  }

  return sawPendingNode ? { nodeId: PENDING_ELEMENT_NODE_ID, isHandle: false } : null
}

function collectHandleTargets(excludeNodeId?: string): HandleTarget[] {
  const handles = document.querySelectorAll('.react-flow__handle')
  const targets: HandleTarget[] = []

  for (const handle of handles) {
    if (!isDomElement(handle)) continue
    const nodeId = handle.closest('.react-flow__node')?.getAttribute('data-id') || undefined
    if (excludeNodeId && nodeId === excludeNodeId) continue

    const rect = handle.getBoundingClientRect()
    targets.push({
      nodeId,
      handleId: handle.getAttribute('data-handleid') || handle.id,
      x: rect.left + rect.width / 2,
      y: rect.top + rect.height / 2,
    })
  }

  return targets
}

function findNearestHandleTargetInCache(targets: HandleTarget[], clientX: number, clientY: number) {
  let hoveredHandleId: string | undefined
  let hoveredNodeId: string | undefined
  let snapPos = { x: clientX, y: clientY }
  let nearestDistance = Infinity

  for (const target of targets) {
    const dist = Math.hypot(clientX - target.x, clientY - target.y)
    if (dist < 36 && dist < nearestDistance) {
      nearestDistance = dist
      snapPos = { x: target.x, y: target.y }
      hoveredHandleId = target.handleId
      hoveredNodeId = target.nodeId
    }
  }

  return {
    nearHandle: hoveredHandleId !== undefined,
    snapPos,
    hoveredHandleId,
    hoveredNodeId,
  }
}

function flattenViewTree(nodes: ViewTreeNode[]): ViewTreeNode[] {
  const out: ViewTreeNode[] = []
  const walk = (items: ViewTreeNode[]) => {
    items.forEach((item) => {
      out.push(item)
      walk(item.children ?? [])
    })
  }
  walk(nodes)
  return out
}

export function applyNodeChangesWithStructuralSharing(changes: NodeChange[], nodes: RFNode[]) {
  if (changes.length === 0) return nodes

  const canFastPath = changes.every((change) => change.type === 'position' && 'id' in change)
  if (!canFastPath) return applyNodeChanges(changes, nodes)

  const changesById = new Map(changes.map((change) => [change.id, change]))
  let didChange = false

  const nextNodes = nodes.map((node) => {
    const change = changesById.get(node.id)
    if (!change || change.type !== 'position') return node

    const position = change.position ?? node.position
    const positionAbsolute = change.positionAbsolute ?? node.positionAbsolute
    const dragging = change.dragging ?? node.dragging

    if (
      node.position.x === position.x &&
      node.position.y === position.y &&
      node.positionAbsolute?.x === positionAbsolute?.x &&
      node.positionAbsolute?.y === positionAbsolute?.y &&
      node.dragging === dragging
    ) {
      return node
    }

    didChange = true
    return {
      ...node,
      position,
      positionAbsolute,
      dragging,
    }
  })

  return didChange ? nextNodes : nodes
}

export function getConnectorDeletionTarget(
  selectedConnector: Connector | null,
) {
  return selectedConnector?.id ?? null
}

interface CanvasInteractionOptions {
  viewId: number | null
  canEdit: boolean
  interactionSourceId: number | null
  setInteractionSourceId: React.Dispatch<React.SetStateAction<number | null>>
  clickConnectMode: ClickConnectModeState | null
  setClickConnectMode: React.Dispatch<React.SetStateAction<ClickConnectModeState | null>>
  clickConnectCursorPos: ClickConnectCursorPosition | null
  setClickConnectCursorPos: React.Dispatch<React.SetStateAction<ClickConnectCursorPosition | null>>
  
  drawingMode: boolean
  isMobileLayout: boolean
  rfNodesRef: React.MutableRefObject<RFNode[]>
  interactionNodesRef: React.MutableRefObject<RFNode[]>
  rfEdgesRef: React.MutableRefObject<RFEdge[]>
  viewElementsRef: React.MutableRefObject<PlacedElement[]>
  viewIdRef: React.MutableRefObject<number | null>
  incomingLinksRef: React.MutableRefObject<IncomingViewConnector[]>
  treeDataRef: React.MutableRefObject<ViewTreeNode[]>
  navigateRef: React.MutableRefObject<(path: string) => void>
  containerRef: React.MutableRefObject<HTMLDivElement | null>
  interactionSourceIdRef: React.MutableRefObject<number | null>
  multiConnectionSourceIdsRef: React.MutableRefObject<number[] | null>
  hoveredZoomRef: React.MutableRefObject<{ elementId: number | null; type: 'in' | 'out' | null } | null>
  hoverPanLockedUntilRef: React.MutableRefObject<number>
  setViewElements: React.Dispatch<React.SetStateAction<PlacedElement[]>>
  setConnectors: React.Dispatch<React.SetStateAction<Connector[]>>
  setRfNodes: React.Dispatch<React.SetStateAction<RFNode[]>>
  setRfEdges: React.Dispatch<React.SetStateAction<RFEdge[]>>
  setLinksMap: React.Dispatch<React.SetStateAction<Record<number, ViewConnector[]>>>
  setParentLinksMap: React.Dispatch<React.SetStateAction<Record<number, ViewConnector[]>>>
  setHoveredZoom: (val: { elementId: number | null; type: 'in' | 'out' | null } | null) => void
  refreshGrid: () => Promise<void>
  refreshElements: () => Promise<void>
  stableOnConnectTo: (targetElementId: number) => Promise<void>
  existingElementIds: Set<number>
  linksMapRef: React.MutableRefObject<Record<number, ViewConnector[]>>
  parentLinksMapRef: React.MutableRefObject<Record<number, ViewConnector[]>>
  openElementPanel: () => void
  closeElementPanel: () => void
  openConnectorPanel: () => void
  closeConnectorPanel: () => void
  selectedElement: LibraryElement | null
  selectedConnector: Connector | null
  connectors: Connector[]
  layers: ViewLayer[]
  setSelectedElement: React.Dispatch<React.SetStateAction<LibraryElement | null>>
  setSelectedEdge: (e: Connector | null) => void
  setSelectedProxyConnectorDetails: React.Dispatch<React.SetStateAction<import('../../../crossBranch/types').ProxyConnectorDetails | null>>
  openProxyConnectorPanel: () => void
  closeProxyConnectorPanel: () => void
  handleElementDeleted: (id: number) => void
  handleElementPermanentlyDeleted: (id: number) => void
  handleConnectorDeleted: (id: number) => void
  onPlacementMoved?: (before: PlacedElement, after: PlacedElement) => void
  onPlacementsMoved?: (before: PlacedElement[], after: PlacedElement[]) => void
  onElementPositionPreview?: (elementId: number, x: number, y: number) => void
  onPlacementRemoved?: (placement: PlacedElement) => void
  onConnectorUpdated?: (before: Connector, after: Connector) => void
  onConnectorDeleted?: (connector: Connector) => void
  onSelectionRemoveFromView?: () => Promise<void>
  onUnsupportedMutation?: () => void
  handleUpdateTags: (elementId: number, tags: string[]) => Promise<void>
  drawingCanvasRef: React.MutableRefObject<DrawingCanvasHandle | null>
  snapToGrid?: boolean
  onMoveStateChange?: (isMoving: boolean) => void
  libraryOpen?: boolean
  openLibrary?: () => void
  toggleLibrary?: () => void
  toggleExplorer?: () => void
  toggleMarkdown?: () => void
  onFitView?: () => void
  setSnapToGrid?: (snap: boolean) => void
}

export type PendingElementState = {
  id: string
  position: { x: number; y: number }
  dragging?: boolean
  preview?: boolean
  mode: 'add' | 'connect'
  sourceElementIds: number[]
  sourceHandle: string | null
}

export function pendingElementPositionFromFlowPoint(flowX: number, flowY: number) {
  return {
    x: flowX - PENDING_ELEMENT_OFFSET_X,
    y: flowY - PENDING_ELEMENT_OFFSET_Y,
  }
}

export function applyPendingElementNodeChanges(
  pendingElement: PendingElementState | null,
  changes: NodeChange[],
): PendingElementState | null {
  if (!pendingElement) return null

  let next = pendingElement
  for (const change of changes) {
    if (!('id' in change) || change.id !== pendingElement.id) continue
    if (change.type === 'remove') return null
    if (change.type !== 'position') continue

    const position = change.position ?? next.position
    const dragging = change.dragging ?? next.dragging
    if (
      next.position.x === position.x &&
      next.position.y === position.y &&
      next.dragging === dragging
    ) {
      continue
    }
    next = { ...next, position, dragging }
  }

  return next
}

export function getDraggedElementNodes(primaryNode: RFNode, draggedNodes: RFNode[], fallbackNodes: RFNode[]) {
  const sourceNodes = draggedNodes.length > 0
    ? draggedNodes
    : primaryNode.selected
      ? fallbackNodes.filter((candidate) => candidate.selected)
      : [primaryNode]

  const result = new Map<string, RFNode>()
  sourceNodes.forEach((candidate) => {
    if (candidate.type !== 'elementNode') return
    if (parseNumericId(candidate.id) === null) return
    result.set(candidate.id, candidate)
  })
  return Array.from(result.values())
}

type HandleReconnectDragState = {
  edgeId: string
  endpoint: 'source' | 'target'
  fixedNodeId: string
  fixedHandle: string
  movingHandle: string
  cursorPos: { x: number; y: number }
  hoveredNodeId?: string
  hoveredHandleId?: string
}

type InteractionStartOptions = {
  sourceHandle?: string
  clientX?: number
  clientY?: number
}

export type ClickConnectModeState = {
  sourceNodeId: string
  sourceHandle: string
  targetHandle?: string
}

export type ClickConnectCursorPosition = { x: number; y: number }

export function useCanvasInteractions({
  viewId,
  canEdit,
  interactionSourceId,
  setInteractionSourceId,
  clickConnectMode,
  setClickConnectMode,
  clickConnectCursorPos,
  setClickConnectCursorPos,
  
  drawingMode: _drawingMode,
  isMobileLayout: _isMobileLayout,
  rfNodesRef,
  interactionNodesRef,
  rfEdgesRef: _rfEdgesRef,
  viewElementsRef,
  viewIdRef,
  incomingLinksRef,
  treeDataRef,
  navigateRef,
  containerRef,
  interactionSourceIdRef,
  multiConnectionSourceIdsRef,
  hoveredZoomRef,
  hoverPanLockedUntilRef,
  setViewElements: _setViewElements,
  setConnectors: _setConnectors,
  setRfNodes,
  setRfEdges,
  setLinksMap,
  setParentLinksMap: _setParentLinksMap,
  setHoveredZoom,
  refreshGrid,
  refreshElements,
  stableOnConnectTo,
  existingElementIds,
  linksMapRef,
  parentLinksMapRef,
  openElementPanel: _openElementPanel,
  closeElementPanel: closeElementPanel,
  openConnectorPanel: openConnectorPanel,
  closeConnectorPanel: closeConnectorPanel,
  selectedElement,
  selectedConnector,
  connectors,
  layers,
  setSelectedElement,
  setSelectedEdge,
  setSelectedProxyConnectorDetails,
  openProxyConnectorPanel,
  closeProxyConnectorPanel,
  handleElementDeleted,
  handleElementPermanentlyDeleted,
  handleConnectorDeleted,
  onPlacementMoved,
  onPlacementsMoved,
  onElementPositionPreview,
  onPlacementRemoved,
  onConnectorUpdated,
  onConnectorDeleted,
  onSelectionRemoveFromView,
  onUnsupportedMutation,
  handleUpdateTags,
  drawingCanvasRef,
  snapToGrid,
  onMoveStateChange,
  libraryOpen,
  openLibrary,
  toggleLibrary,
  toggleExplorer,
  toggleMarkdown,
  onFitView,
  setSnapToGrid: setGlobalSnapToGrid,
}: CanvasInteractionOptions) {
  const { screenToFlowPosition, setViewport, getViewport, zoomIn, zoomOut } = useReactFlow()
  const updateElementPosition = useStore((state) => state.updateElementPosition)
  const removeElementPlacement = useStore((state) => state.removeElementPlacement)
  const upsertConnector = useStore((state) => state.upsertConnector)
  const replaceConnector = useStore((state) => state.replaceConnector)
  const screenToFlowPositionRef = useRef(screenToFlowPosition)
  screenToFlowPositionRef.current = screenToFlowPosition
  const getInteractionNodes = useCallback(() => {
    return interactionNodesRef.current.length > 0 ? interactionNodesRef.current : rfNodesRef.current
  }, [interactionNodesRef, rfNodesRef])

  const [canvasMenu, setCanvasMenu] = useState<{ x: number; y: number; flowX: number; flowY: number } | null>(null)
  const [pendingElement, setPendingElement] = useState<PendingElementState | null>(null)
  const [reconnectPicking, setReconnectPicking] = useState<{ edgeId: number; endpoint: 'source' | 'target' } | null>(null)
  const [handleReconnectDrag, setHandleReconnectDrag] = useState<HandleReconnectDragState | null>(null)
  const [connectorLongPressMenu, setConnectorLongPressMenu] = useState<{ edgeId: number; x: number; y: number } | null>(null)
  const isMovingRef = useRef(false)

  interactionSourceIdRef.current = interactionSourceId

  const reconnectPickingRef = useRef<{ edgeId: number; endpoint: 'source' | 'target' } | null>(null)
  const handleReconnectDragRef = useRef<HandleReconnectDragState | null>(null)
  const handleReconnectListenersRef = useRef<{ move: (event: PointerEvent) => void; up: (event: PointerEvent) => void } | null>(null)
  const connectorDragPreviewListenersRef = useRef<{
    move: (event: PointerEvent) => void
    touchMove: (event: TouchEvent) => void
  } | null>(null)
  const connectingSourceRef = useRef<string | null>(null)
  const connectWasValidRef = useRef(false)
  const connectorDragLastUpdateRef = useRef(0)
  const isReconnectingRef = useRef(false)
  const suppressNextConnectorClickRef = useRef(false)
  const suppressNextPaneClickRef = useRef(false)
  const longPressCanvasRef = useRef<{ timer: ReturnType<typeof setTimeout>; clientX: number; clientY: number } | null>(null)
  const pendingConnectionSourceRef = useRef<number | null>(null)
  const pendingConnectionSourceHandleRef = useRef<string | null>(null)
  const clickConnectModeRef = useRef(clickConnectMode)
  clickConnectModeRef.current = clickConnectMode

  const setSyncedInteractionSourceId = useCallback((next: number | null) => {
    interactionSourceIdRef.current = next
    setInteractionSourceId(next)
  }, [interactionSourceIdRef, setInteractionSourceId])

  const setSyncedClickConnectMode = useCallback((next: React.SetStateAction<ClickConnectModeState | null>) => {
    const resolved = typeof next === 'function'
      ? next(clickConnectModeRef.current)
      : next
    clickConnectModeRef.current = resolved
    setClickConnectMode(resolved)
  }, [setClickConnectMode])

  const clearConnectionSourceRefs = useCallback(() => {
    interactionSourceIdRef.current = null
    multiConnectionSourceIdsRef.current = null
    pendingConnectionSourceRef.current = null
    pendingConnectionSourceHandleRef.current = null
  }, [interactionSourceIdRef, multiConnectionSourceIdsRef])

  const activeConnectionSourceIds = useCallback((fallbackSourceId: number | null, targetElementId: number) => {
    const sourceIds = multiConnectionSourceIdsRef.current?.length
      ? multiConnectionSourceIdsRef.current
      : fallbackSourceId !== null
        ? [fallbackSourceId]
        : []
    return Array.from(new Set(sourceIds)).filter((sourceId) => sourceId !== targetElementId)
  }, [multiConnectionSourceIdsRef])

  const clearPendingConnectionRefs = useCallback(() => {
    pendingConnectionSourceRef.current = null
    pendingConnectionSourceHandleRef.current = null
    multiConnectionSourceIdsRef.current = null
  }, [multiConnectionSourceIdsRef])

  const cancelPendingElement = useCallback(() => {
    setPendingElement(null)
    clearPendingConnectionRefs()
  }, [clearPendingConnectionRefs])
  const lastMousePosRef = useRef<{ clientX: number; clientY: number } | null>(null)

  const touchStateRef = useRef<{
    touches: Map<number, { x: number; y: number }>
    initialDistance: number
    isPinching: boolean
    lastMultiTouchWheelTime: number
  }>({ touches: new Map(), initialDistance: 0, isPinching: false, lastMultiTouchWheelTime: 0 })

  const hoverPanTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const syncHandleReconnectDrag = useCallback((next: HandleReconnectDragState | null) => {
    handleReconnectDragRef.current = next
    setHandleReconnectDrag(next)
  }, [])

  const clearHandleReconnectListeners = useCallback(() => {
    const listeners = handleReconnectListenersRef.current
    if (!listeners) return
    document.removeEventListener('pointermove', listeners.move)
    document.removeEventListener('pointerup', listeners.up)
    document.removeEventListener('pointercancel', listeners.up)
    handleReconnectListenersRef.current = null
  }, [])

  const clearConnectorDragPreviewListeners = useCallback(() => {
    const listeners = connectorDragPreviewListenersRef.current
    if (!listeners) return
    document.removeEventListener('pointermove', listeners.move)
    document.removeEventListener('touchmove', listeners.touchMove)
    connectorDragPreviewListenersRef.current = null
  }, [])

  const clearConnectorDragPreview = useCallback(() => {
    clearConnectorDragPreviewListeners()
    setPendingElement((current) => current?.preview ? null : current)
  }, [clearConnectorDragPreviewListeners])

  const stopHandleReconnectDrag = useCallback(() => {
    clearHandleReconnectListeners()
    handleReconnectDragRef.current = null
    setHandleReconnectDrag(null)
    isReconnectingRef.current = false
  }, [clearHandleReconnectListeners])

  const finalizeConnectorCreate = useCallback(async (connector: Connector) => {
    upsertConnectorGraphSnapshot(connector)
    upsertConnector(connector)
    await refreshElements()
  }, [refreshElements, upsertConnector])

  // ── Ref-forwarded callbacks ────────────────────────────────────────────────
  const openConnectorPanelRef = useRef(openConnectorPanel)
  openConnectorPanelRef.current = openConnectorPanel

  const resolvePickerMode = useCallback((flowX: number, flowY: number, preferredMode: 'add' | 'connect', forceConnect = false) => {
    if (preferredMode !== 'connect') return preferredMode
    if (forceConnect) return 'connect'

    const boundaryNode = getInteractionNodes().find((node) => node.type === 'ContextBoundaryElement')
    if (!boundaryNode) return 'add'

    const boundaryData = boundaryNode.data as { width?: number; height?: number } | undefined
    const width = boundaryData?.width ?? boundaryNode.width
    const height = boundaryData?.height ?? boundaryNode.height
    if (width == null || height == null) return 'add'

    const withinBoundary =
      flowX >= boundaryNode.position.x &&
      flowX <= boundaryNode.position.x + width &&
      flowY >= boundaryNode.position.y &&
      flowY <= boundaryNode.position.y + height

    return withinBoundary ? 'add' : 'connect'
  }, [getInteractionNodes])

  // ── Pending element node ───────────────────────────────────────────────────
  const showAddingElementAt = useCallback((clientX: number, clientY: number, _expandResults = false, mode: 'add' | 'connect' = 'add', forceConnect = false) => {
    const flowPos = screenToFlowPositionRef.current({ x: clientX, y: clientY })
    let { x: flowX, y: flowY } = flowPos
    if (snapToGrid) {
      flowX = Math.round(flowX / 10) * 10
      flowY = Math.round(flowY / 10) * 10
    }
    const sourceIds = Array.from(new Set(
      multiConnectionSourceIdsRef.current?.length
        ? multiConnectionSourceIdsRef.current
        : pendingConnectionSourceRef.current !== null
          ? [pendingConnectionSourceRef.current]
          : [],
    ))
    const sourceHandle = pendingConnectionSourceHandleRef.current
    setPendingElement({
      id: PENDING_ELEMENT_NODE_ID,
      position: pendingElementPositionFromFlowPoint(flowX, flowY),
      mode: resolvePickerMode(flowX, flowY, mode, forceConnect),
      sourceElementIds: sourceIds,
      sourceHandle,
    })
    pendingConnectionSourceRef.current = null
    multiConnectionSourceIdsRef.current = null
  }, [multiConnectionSourceIdsRef, snapToGrid, resolvePickerMode])

  const updateConnectorDragPreview = useCallback((clientX: number, clientY: number, sourceNodeId: string, forceConnect = false) => {
    if (!shouldDisplayConnectorDragPlaceholder(getConnectorDragTargetAtPoint(clientX, clientY))) {
      setPendingElement((current) => current?.preview ? null : current)
      return
    }

    const interactionNodes = getInteractionNodes()
    const sourceElementId = resolveElementIdFromNode(interactionNodes.find((node) => node.id === sourceNodeId)) ?? parseNumericId(sourceNodeId)
    if (sourceElementId === null) {
      setPendingElement((current) => current?.preview ? null : current)
      return
    }

    const flowPos = screenToFlowPositionRef.current({ x: clientX, y: clientY })
    let { x: flowX, y: flowY } = flowPos
    if (snapToGrid) {
      flowX = Math.round(flowX / 10) * 10
      flowY = Math.round(flowY / 10) * 10
    }
    const position = pendingElementPositionFromFlowPoint(flowX, flowY)
    const mode = resolvePickerMode(flowX, flowY, 'connect', forceConnect)
    const sourceHandle = pendingConnectionSourceHandleRef.current

    setPendingElement((current) => {
      if (
        current?.preview &&
        current.position.x === position.x &&
        current.position.y === position.y &&
        current.mode === mode &&
        current.sourceHandle === sourceHandle &&
        current.sourceElementIds.length === 1 &&
        current.sourceElementIds[0] === sourceElementId
      ) {
        return current
      }
      return {
        id: PENDING_ELEMENT_NODE_ID,
        position,
        preview: true,
        mode,
        sourceElementIds: [sourceElementId],
        sourceHandle,
      }
    })
  }, [getInteractionNodes, resolvePickerMode, snapToGrid])

  // ── Pending element confirmation handlers ──────────────────────────────────
  const handleConfirmNewElement = useCallback(async (name: string) => {
    if (!canEdit || viewId === null || !pendingElement) return
    const activePendingElement = pendingElement
    const targetPoint = {
      x: activePendingElement.position.x + PENDING_ELEMENT_OFFSET_X,
      y: activePendingElement.position.y + PENDING_ELEMENT_OFFSET_Y,
    }
    setPendingElement(null)
    clearPendingConnectionRefs()
    try {
      const obj = await api.elements.create({ name, kind: '' })
      await api.workspace.views.placements.add(viewId, obj.id, activePendingElement.position.x, activePendingElement.position.y)
      await refreshElements()
      const placed = viewElementsRef.current.find((element) => element.element_id === obj.id)
      if (placed) upsertPlacementGraphSnapshot(viewId, placed)
      const sourceIds = activePendingElement.sourceElementIds
        .filter((nextSourceId) => nextSourceId !== obj.id)
      for (const nextSourceId of sourceIds) {
        const sourceNode = rfNodesRef.current.find((n) => n.id === String(nextSourceId))
        const { sourceHandle, targetHandle } = sourceNode
          ? findClosestHandleToPoint(sourceNode, targetPoint.x, targetPoint.y)
          : { sourceHandle: 'right', targetHandle: 'left' }
        const newConnector = await api.workspace.connectors.create(viewId, {
          source_element_id: nextSourceId, target_element_id: obj.id,
          source_handle: activePendingElement.sourceHandle ?? sourceHandle, target_handle: targetHandle, direction: 'forward',
        })
        const connector = connectorToConnector(newConnector)
        await finalizeConnectorCreate(connector)
      }
      onUnsupportedMutation?.()
    } catch { /* intentionally empty */ }
  }, [canEdit, clearPendingConnectionRefs, finalizeConnectorCreate, onUnsupportedMutation, pendingElement, refreshElements, rfNodesRef, viewId, viewElementsRef])

  const handleConfirmExistingElement = useCallback(async (obj: LibraryElement) => {
    if (!canEdit || viewId === null || !pendingElement) return
    const activePendingElement = pendingElement
    const targetPoint = {
      x: activePendingElement.position.x + PENDING_ELEMENT_OFFSET_X,
      y: activePendingElement.position.y + PENDING_ELEMENT_OFFSET_Y,
    }
    const sourceIds = activePendingElement.sourceElementIds.filter((sourceId) => sourceId !== obj.id)
    setPendingElement(null)
    clearPendingConnectionRefs()
    try {
      if (!existingElementIds.has(obj.id)) {
        await api.workspace.views.placements.add(viewId, obj.id, activePendingElement.position.x, activePendingElement.position.y)
        await refreshElements()
        const placed = viewElementsRef.current.find((element) => element.element_id === obj.id)
        if (placed) upsertPlacementGraphSnapshot(viewId, placed)
      }
      for (const nextSourceId of sourceIds) {
        const sourceNode = rfNodesRef.current.find((n) => n.id === String(nextSourceId))
        const targetNode = rfNodesRef.current.find((n) => n.id === String(obj.id))
        const { sourceHandle, targetHandle } = sourceNode && targetNode
          ? findClosestHandles(sourceNode, targetNode)
          : sourceNode
            ? findClosestHandleToPoint(sourceNode, targetPoint.x, targetPoint.y)
            : { sourceHandle: 'right', targetHandle: 'left' }
        const newConnector = await api.workspace.connectors.create(viewId, {
          source_element_id: nextSourceId, target_element_id: obj.id,
          source_handle: activePendingElement.sourceHandle ?? sourceHandle, target_handle: targetHandle, direction: 'forward',
        })
        const connector = connectorToConnector(newConnector)
        await finalizeConnectorCreate(connector)
      }
      if (!existingElementIds.has(obj.id) || sourceIds.length > 0) onUnsupportedMutation?.()
    } catch { /* intentionally empty */ }
  }, [canEdit, clearPendingConnectionRefs, existingElementIds, finalizeConnectorCreate, onUnsupportedMutation, pendingElement, refreshElements, rfNodesRef, viewId, viewElementsRef])

  // ── Zoom-in / zoom-out stable callbacks ───────────────────────────────────
  const stableOnZoomIn = useCallback(async (elementId: number) => {
    const childLinks = linksMapRef.current[elementId] || []
    if (childLinks.length > 0) {
      setSelectedElement(null)
      setSelectedEdge(null)
      closeElementPanel()
      closeConnectorPanel()
      navigateRef.current(`/views/${childLinks[0].to_view_id}`)
      return
    }

    const obj = viewElementsRef.current.find((o) => o.element_id === elementId)
    if (obj?.has_view) {
      // Find the existing view in the tree
      const findInTree = (nodes: ViewTreeNode[]): ViewTreeNode | null => {
        for (const node of nodes) {
          if (node.owner_element_id !== null && Number(node.owner_element_id) === Number(elementId)) return node
          const found = findInTree(node.children)
          if (found) return found
        }
        return null
      }
      const existingView = findInTree(treeDataRef.current)
      if (existingView) {
        setSelectedElement(null)
        setSelectedEdge(null)
        closeElementPanel()
        closeConnectorPanel()
        navigateRef.current(`/views/${existingView.id}`)
        return
      }
    }

    if (!canEdit) return
    const cid = viewIdRef.current
    if (cid === null) return
    try {
      const newView = await api.workspace.views.create({ name: `${obj?.name ?? 'Element'}`, parent_view_id: elementId })
      setLinksMap((prev) => ({
        ...prev,
        [elementId]: [...(prev[elementId] || []),
        { id: 0, element_id: elementId, from_view_id: cid, to_view_id: newView.id, to_view_name: newView.name, relation_type: 'child' as const }],
      }))
      setSelectedElement(null)
      setSelectedEdge(null)
      closeElementPanel()
      closeConnectorPanel()
      navigateRef.current(`/views/${newView.id}`)
    } catch { /* intentionally empty */ }
  }, [canEdit, linksMapRef, viewIdRef, viewElementsRef, navigateRef, setLinksMap, treeDataRef, setSelectedElement, setSelectedEdge, closeElementPanel, closeConnectorPanel])

  const stableOnZoomOut = useCallback(async (elementId: number) => {
    const parentLinks = parentLinksMapRef.current[elementId] || []
    // If the clicked element has no direct parent link, fall back to the current
    // view's parent stored under the view's owner element ID (which may differ
    // from the clicked element's ID for elements like functions/classes that
    // don't own a view themselves).
    const anyParentLink = parentLinks[0] ?? Object.values(parentLinksMapRef.current).flat()[0]
    if (anyParentLink) {
      setSelectedElement(null)
      setSelectedEdge(null)
      closeElementPanel()
      closeConnectorPanel()
      navigateRef.current(`/views/${anyParentLink.from_view_id}`)
      return
    }

    // Final fallback: use current view's parent_view_id if available
    const findInTreeById = (nodes: ViewTreeNode[], id: number): ViewTreeNode | null => {
      for (const node of nodes) {
        if (node.id === id) return node
        const found = findInTreeById(node.children, id)
        if (found) return found
      }
      return null
    }
    const currentView = findInTreeById(treeDataRef.current, viewIdRef.current || -1)
    if (currentView?.parent_view_id) {
      setSelectedElement(null)
      setSelectedEdge(null)
      closeElementPanel()
      closeConnectorPanel()
      navigateRef.current(`/views/${currentView.parent_view_id}`)
    }
  }, [parentLinksMapRef, navigateRef, treeDataRef, viewIdRef, setSelectedElement, setSelectedEdge, closeElementPanel, closeConnectorPanel])

  const stableOnNavigateToView = useCallback((id: number) => {
    setSelectedElement(null)
    setSelectedEdge(null)
    closeElementPanel()
    closeConnectorPanel()
    navigateRef.current(`/views/${id}`)
  }, [navigateRef, setSelectedElement, setSelectedEdge, closeElementPanel, closeConnectorPanel])

  const stableOnHoverZoom = useCallback((elementId: number, type: 'in' | 'out' | null) => {
    const prev = hoveredZoomRef.current
    const next = type ? { elementId, type } : null
    hoveredZoomRef.current = next
    setHoveredZoom(next)
    setRfNodes((nodes) =>
      nodes.map((n) => {
        const wasHovered = prev && prev.elementId !== null && n.id === String(prev.elementId) ? prev.type : null
        const isHovered = n.id === String(elementId) ? type : null
        if (wasHovered === isHovered) return n
        return { ...n, data: { ...n.data, isZoomHovered: isHovered } }
      }),
    )
  }, [hoveredZoomRef, setHoveredZoom, setRfNodes])

  const stableOnRemoveElement = useCallback(async (elementId: number) => {
    if (!canEdit || viewId === null) return
    const removedPlacement = viewElementsRef.current.find((element) => element.element_id === elementId) ?? null
    try {
      await api.workspace.views.placements.remove(viewId, elementId)
      removePlacementGraphSnapshot(viewId, elementId)
      removeElementPlacement(elementId)
      if (removedPlacement) onPlacementRemoved?.(removedPlacement)
      handleElementDeleted(elementId)
      setSyncedInteractionSourceId(null)
      pendingConnectionSourceHandleRef.current = null
    } catch { /* intentionally empty */ }
  }, [canEdit, viewId, viewElementsRef, removeElementPlacement, onPlacementRemoved, handleElementDeleted, setSyncedInteractionSourceId])

  const connectClickModeToHandle = useCallback(async (targetElementId: number, targetHandle: string) => {
    if (!canEdit) return
    const cid = viewIdRef.current
    const sourceElementId = interactionSourceIdRef.current
    const sourceIds = activeConnectionSourceIds(sourceElementId, targetElementId)
    if (cid === null || sourceIds.length === 0) return

    const sourceHandle = pendingConnectionSourceHandleRef.current ??
      (clickConnectModeRef.current?.sourceHandle
        ? getLogicalHandleId(clickConnectModeRef.current.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
        : DEFAULT_SOURCE_HANDLE_SIDE)
    const logicalTargetHandle = getLogicalHandleId(targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE

    setSyncedInteractionSourceId(null)
    clearPendingConnectionRefs()
    setSyncedClickConnectMode(null)
    setClickConnectCursorPos(null)

    try {
      for (const nextSourceId of sourceIds) {
        const newConnector = await api.workspace.connectors.create(cid, {
          source_element_id: nextSourceId,
          target_element_id: targetElementId,
          source_handle: sourceHandle,
          target_handle: logicalTargetHandle,
          direction: 'forward',
        })
        const connector = connectorToConnector(newConnector)
        await finalizeConnectorCreate(connector)
      }
      onUnsupportedMutation?.()
    } catch { /* intentionally empty */ }
  }, [activeConnectionSourceIds, canEdit, clearPendingConnectionRefs, finalizeConnectorCreate, interactionSourceIdRef, onUnsupportedMutation, setClickConnectCursorPos, setSyncedClickConnectMode, setSyncedInteractionSourceId, viewIdRef])

  const stableOnInteractionStart = useCallback((elementId: number, options?: InteractionStartOptions) => {
    if (!canEdit) return
    const sourceHandle = options?.sourceHandle

    if (sourceHandle) {
      const cursorPos = options?.clientX !== undefined && options.clientY !== undefined
        ? { x: options.clientX, y: options.clientY }
        : lastMousePosRef.current
          ? { x: lastMousePosRef.current.clientX, y: lastMousePosRef.current.clientY }
          : null
      if (!cursorPos) return

      const activeSourceId = interactionSourceIdRef.current
      if (activeSourceId !== null && activeSourceId !== elementId) {
        void connectClickModeToHandle(elementId, sourceHandle)
        return
      }
      if (activeSourceId === elementId) {
        pendingConnectionSourceHandleRef.current = null
        multiConnectionSourceIdsRef.current = null
        setSyncedInteractionSourceId(null)
        setSyncedClickConnectMode(null)
        setClickConnectCursorPos(null)
        return
      }

      pendingConnectionSourceHandleRef.current = getLogicalHandleId(sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
      multiConnectionSourceIdsRef.current = null
      setSyncedInteractionSourceId(elementId)
      pendingConnectionSourceRef.current = null
      setPendingElement(null)
      setSyncedClickConnectMode({
        sourceNodeId: String(elementId),
        sourceHandle: ensureVisualHandleId(sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? sourceHandle,
      })
      setClickConnectCursorPos(cursorPos)
      return
    }

    const isCancelling = interactionSourceIdRef.current === elementId
    const nextSourceId = isCancelling ? null : elementId

    if (isCancelling) pendingConnectionSourceHandleRef.current = null
    multiConnectionSourceIdsRef.current = null
    setSyncedInteractionSourceId(nextSourceId)

    if (isCancelling) {
      setSyncedClickConnectMode(null)
      setClickConnectCursorPos(null)
    }
  }, [canEdit, connectClickModeToHandle, interactionSourceIdRef, multiConnectionSourceIdsRef, setClickConnectCursorPos, setSyncedClickConnectMode, setSyncedInteractionSourceId])

  const stableOnConnectToAndReset = useCallback(async (targetElementId: number) => {
    await stableOnConnectTo(targetElementId)
    setSyncedInteractionSourceId(null)
    clearPendingConnectionRefs()
    setSyncedClickConnectMode(null)
    setClickConnectCursorPos(null)
  }, [clearPendingConnectionRefs, setClickConnectCursorPos, setSyncedClickConnectMode, setSyncedInteractionSourceId, stableOnConnectTo])

  // ── Node/connector changes ─────────────────────────────────────────────────────
  const onNodesChange = useCallback((changes: NodeChange[]) => {
    setPendingElement((current) => applyPendingElementNodeChanges(current, changes))
    const mainNodeChanges = changes.filter((change) => !('id' in change) || change.id !== PENDING_ELEMENT_NODE_ID)
    if (mainNodeChanges.length === 0) return

    const elementOnlySelectionChanges = mainNodeChanges.map((change) => {
      if (change.type !== 'select') return change
      const node = rfNodesRef.current.find((candidate) => candidate.id === change.id)
      return node?.type === 'elementNode' ? change : { ...change, selected: false }
    })
    if (!canEdit) {
      const nonMutating = elementOnlySelectionChanges.filter((c) => c.type !== 'position')
      if (nonMutating.length === 0) return
      setRfNodes((nds) => applyNodeChangesWithStructuralSharing(nonMutating, nds))
      return
    }
    setRfNodes((nds) => applyNodeChangesWithStructuralSharing(elementOnlySelectionChanges, nds))
  }, [canEdit, rfNodesRef, setRfNodes])

  const onEdgesChange = useCallback((changes: EdgeChange[]) => {
    setRfEdges((eds) => applyEdgeChanges(changes, eds))
  }, [setRfEdges])

  const onNodeDragStart: NodeDragHandler = useCallback((_e, node, nodes) => {
    if (!canEdit || viewId === null) return
    const selectedElementNodes = getDraggedElementNodes(node, nodes, rfNodesRef.current)
    selectedElementNodes.forEach((candidate) => {
      const elementId = parseNumericId(candidate.id)
      if (elementId !== null) dragStartPositionsRef.current[candidate.id] = { x: candidate.position.x, y: candidate.position.y }
    })
  }, [canEdit, rfNodesRef, viewId])

  const onNodeDrag: NodeDragHandler = useCallback((_e, node, nodes) => {
    // React Flow already updates rfNodes via onNodesChange while dragging.
    // Mirroring into viewElements here forces every derived edge/node to rebuild
    // on each pointer frame, so persist to app state only on drag stop.
    if (!canEdit || !onElementPositionPreview) return
    const selectedElementNodes = getDraggedElementNodes(node, nodes, rfNodesRef.current)
    selectedElementNodes.forEach((candidate) => {
      const elementId = parseNumericId(candidate.id)
      if (elementId !== null) onElementPositionPreview(elementId, candidate.position.x, candidate.position.y)
    })
  }, [canEdit, onElementPositionPreview, rfNodesRef])

  const positionTimers = useRef<Record<string, ReturnType<typeof setTimeout>>>({})
  const dragStartPositionsRef = useRef<Record<string, { x: number; y: number }>>({})
  const onNodeDragStop: NodeDragHandler = useCallback((_e, node, nodes) => {
    if (!canEdit || viewId === null) return
    const selectedElementNodes = getDraggedElementNodes(node, nodes, rfNodesRef.current)

    const beforePlacements: PlacedElement[] = []
    const afterPlacements: PlacedElement[] = []

    selectedElementNodes.forEach((candidate) => {
      const elementId = parseNumericId(candidate.id)
      if (elementId === null) return

      const currentObj = viewElementsRef.current.find((o) => o.element_id === elementId)
      const startPos = dragStartPositionsRef.current[candidate.id] ?? (currentObj ? { x: currentObj.position_x, y: currentObj.position_y } : null)
      delete dragStartPositionsRef.current[candidate.id]
      if (!currentObj || !startPos) return
      if (Math.abs(startPos.x - candidate.position.x) < 2 && Math.abs(startPos.y - candidate.position.y) < 2) return

      beforePlacements.push({ ...currentObj, position_x: startPos.x, position_y: startPos.y })
      afterPlacements.push({ ...currentObj, position_x: candidate.position.x, position_y: candidate.position.y })
      updateElementPosition(elementId, candidate.position.x, candidate.position.y)
    })

    if (afterPlacements.length === 0) return

    afterPlacements.forEach((placement) => {
      clearTimeout(positionTimers.current[String(placement.element_id)])
    })
    const timerKey = node.id
    positionTimers.current[timerKey] = setTimeout(() => {
      Promise.all(afterPlacements.map((placement) =>
        api.workspace.views.placements.updatePosition(viewId, placement.element_id, placement.position_x, placement.position_y)
      ))
        .then(() => {
          if (beforePlacements.length === 1 && afterPlacements.length === 1) {
            onPlacementMoved?.(beforePlacements[0], afterPlacements[0])
          } else {
            onPlacementsMoved?.(beforePlacements, afterPlacements)
          }
        })
        .catch(() => { /* intentionally empty */ })
    }, 400)
  }, [canEdit, rfNodesRef, updateElementPosition, viewId, viewElementsRef, onPlacementMoved, onPlacementsMoved])

  // ── Connections ────────────────────────────────────────────────────────────
  const onConnect: OnConnect = useCallback(async (params: Connection) => {
    if (!canEdit || isReconnectingRef.current) return
    clearConnectorDragPreview()
    connectWasValidRef.current = true
    if (viewId === null || !params.source || !params.target) {
      clearPendingConnectionRefs()
      return
    }
    const interactionNodes = getInteractionNodes()
    const sourceId = resolveElementIdFromNode(interactionNodes.find((node) => node.id === params.source)) ?? parseNumericId(params.source)
    const targetId = resolveElementIdFromNode(interactionNodes.find((node) => node.id === params.target)) ?? parseNumericId(params.target)
    if (sourceId === null || targetId === null) {
      clearPendingConnectionRefs()
      return
    }
    try {
      const sourceHandle = getLogicalHandleId(params.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE)
      const targetHandle = getLogicalHandleId(params.targetHandle, DEFAULT_TARGET_HANDLE_SIDE)
      const newConnector = await api.workspace.connectors.create(viewId, {
        source_element_id: sourceId, target_element_id: targetId,
        source_handle: sourceHandle, target_handle: targetHandle,
        direction: 'forward', style: 'bezier',
      })
      const connector = connectorToConnector(newConnector)
      await finalizeConnectorCreate(connector)
      onUnsupportedMutation?.()
    } catch { /* intentionally empty */ }
    clearPendingConnectionRefs()
  }, [canEdit, clearConnectorDragPreview, clearPendingConnectionRefs, finalizeConnectorCreate, getInteractionNodes, onUnsupportedMutation, viewId])

  const onConnectStart = useCallback((event: React.MouseEvent | React.TouchEvent, { nodeId, handleId }: OnConnectStartParams) => {
    if (!canEdit || isReconnectingRef.current) return
    clearConnectorDragPreviewListeners()
    setPendingElement(null)
    connectingSourceRef.current = nodeId
    pendingConnectionSourceHandleRef.current = getLogicalHandleId(handleId, DEFAULT_SOURCE_HANDLE_SIDE)
    const sourceElementId = nodeId
      ? resolveElementIdFromNode(getInteractionNodes().find((node) => node.id === nodeId)) ?? parseNumericId(nodeId)
      : null
    pendingConnectionSourceRef.current = sourceElementId
    connectWasValidRef.current = false
    if (!nodeId) return

    connectorDragLastUpdateRef.current = 0
    const updateFromPoint = (clientX: number, clientY: number, forceConnect = false) => {
      const now = performance.now()
      if (now - connectorDragLastUpdateRef.current < CONNECTOR_DRAG_UPDATE_INTERVAL_MS) return
      connectorDragLastUpdateRef.current = now
      updateConnectorDragPreview(clientX, clientY, nodeId, forceConnect)
    }

    if ('clientX' in event) {
      updateConnectorDragPreview(event.clientX, event.clientY, nodeId, event.shiftKey)
    } else {
      const touch = event.touches[0]
      if (touch) updateConnectorDragPreview(touch.clientX, touch.clientY, nodeId)
    }

    const move = (moveEvent: PointerEvent) => {
      updateFromPoint(moveEvent.clientX, moveEvent.clientY, moveEvent.shiftKey)
    }
    const touchMove = (moveEvent: TouchEvent) => {
      const touch = moveEvent.touches[0]
      if (touch) updateFromPoint(touch.clientX, touch.clientY)
    }
    connectorDragPreviewListenersRef.current = { move, touchMove }
    document.addEventListener('pointermove', move)
    document.addEventListener('touchmove', touchMove, { passive: true })
  }, [canEdit, clearConnectorDragPreviewListeners, getInteractionNodes, updateConnectorDragPreview])

  const onConnectEnd = useCallback((event: MouseEvent | TouchEvent | PointerEvent) => {
    if (!canEdit || isReconnectingRef.current) return
    const sourceId = connectingSourceRef.current
    connectingSourceRef.current = null
    clearConnectorDragPreview()
    if (!sourceId || connectWasValidRef.current) {
      connectWasValidRef.current = false
      clearPendingConnectionRefs()
      return
    }
    connectWasValidRef.current = false
    const interactionNodes = getInteractionNodes()
    const point = 'changedTouches' in event
      ? event.changedTouches[0] ?? event.touches[0]
      : event
    if (!point) {
      clearPendingConnectionRefs()
      return
    }
    const { clientX, clientY } = point
    const { droppedNode, droppedHandleId } = resolveConnectorDropTarget(event.target, clientX, clientY, interactionNodes)
    const flowPos = screenToFlowPositionRef.current({ x: clientX, y: clientY })
    if (droppedNode?.id === sourceId) {
      clearPendingConnectionRefs()
      return
    }
    const releaseNode = droppedNode?.id === PENDING_ELEMENT_NODE_ID ? null : droppedNode
    const nearNode = releaseNode ?? interactionNodes.find((node) => {
      if (node.id === sourceId) return false
      if (node.id === PENDING_ELEMENT_NODE_ID) return false
      const cx = node.position.x + (node.width ?? 180) / 2
      const cy = node.position.y + (node.height ?? 80) / 2
      return Math.hypot(flowPos.x - cx, flowPos.y - cy) < SNAP_RADIUS
    })
    const cid = viewIdRef.current
    if (cid === null) {
      clearPendingConnectionRefs()
      return
    }
    const sourceElementId = resolveElementIdFromNode(interactionNodes.find((node) => node.id === sourceId)) ?? parseNumericId(sourceId)
    if (sourceElementId === null) {
      clearPendingConnectionRefs()
      return
    }
    if (nearNode) {
      const targetElementId = resolveElementIdFromNode(nearNode)
      if (targetElementId === null) {
        clearPendingConnectionRefs()
        return
      }
      const sourceNode = interactionNodes.find((n) => n.id === sourceId)
      const closestHandles = sourceNode
        ? findClosestHandles(sourceNode, nearNode)
        : { sourceHandle: 'right', targetHandle: 'left' }
      const sourceHandle = pendingConnectionSourceHandleRef.current ?? closestHandles.sourceHandle
      const targetHandle = nearNode === releaseNode && droppedHandleId
        ? getLogicalHandleId(droppedHandleId, DEFAULT_TARGET_HANDLE_SIDE) ?? closestHandles.targetHandle
        : closestHandles.targetHandle
      api.workspace.connectors.create(cid, {
        source_element_id: sourceElementId, target_element_id: targetElementId,
        source_handle: sourceHandle, target_handle: targetHandle, direction: 'forward',
      }).then((connector) => {
        const next = connectorToConnector(connector)
        void finalizeConnectorCreate(next)
        onUnsupportedMutation?.()
      }).catch(() => { /* intentionally empty */ })
      clearPendingConnectionRefs()
    } else {
      pendingConnectionSourceRef.current = sourceElementId
      suppressNextPaneClickRef.current = true
      showAddingElementAt(clientX, clientY, true, 'connect', 'shiftKey' in event && event.shiftKey)
    }
  }, [canEdit, clearConnectorDragPreview, clearPendingConnectionRefs, finalizeConnectorCreate, getInteractionNodes, onUnsupportedMutation, showAddingElementAt, viewIdRef])

  // ── Reconnect ──────────────────────────────────────────────────────────────
  const performReconnect = useCallback(async (oldConnector: RFEdge, newConnection: Connection) => {
    if (!canEdit || viewId === null || !newConnection.source || !newConnection.target) return
    const edgeId = parseNumericId(oldConnector.id)
    const sourceId = parseNumericId(newConnection.source)
    const targetId = parseNumericId(newConnection.target)
    if (edgeId === null || sourceId === null || targetId === null) return
    setRfEdges((eds) => reconnectEdge(oldConnector, newConnection, eds))
    try {
      const existingData = oldConnector.data as Connector
      const sourceHandle = getLogicalHandleId(newConnection.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE)
      const targetHandle = getLogicalHandleId(newConnection.targetHandle, DEFAULT_TARGET_HANDLE_SIDE)
      const updated = await api.workspace.connectors.update(viewId, edgeId, {
        source_element_id: sourceId, target_element_id: targetId,
        source_handle: sourceHandle ?? undefined,
        target_handle: targetHandle ?? undefined,
        label: existingData?.label ?? undefined, description: existingData?.description ?? undefined,
        direction: existingData?.direction ?? undefined,
        style: existingData?.style === 'default' ? 'bezier' : (existingData?.style ?? 'bezier'),
        url: existingData?.url ?? undefined, relationship: existingData?.relationship ?? undefined,
      })
      const connector = connectorToConnector(updated)
      upsertConnectorGraphSnapshot(connector)
      replaceConnector(connector)
      if (existingData) onConnectorUpdated?.(existingData, connector)
    } catch { /* intentionally empty */ }
  }, [canEdit, replaceConnector, viewId, setRfEdges, onConnectorUpdated])
  const onReconnect = useCallback(async (oldConnector: RFEdge, newConnection: Connection) => {
    await performReconnect(oldConnector, newConnection)
  }, [performReconnect])
  const onReconnectStart = useCallback(() => { isReconnectingRef.current = true }, [])
  const onReconnectEnd = useCallback(() => { isReconnectingRef.current = false }, [])

  const stableOnStartHandleReconnect = useCallback((args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number }) => {
    if (!canEdit) return
    const edge = _rfEdgesRef.current.find((candidate) => candidate.id === args.edgeId)
    if (!edge) return

    const fixedNodeId = args.endpoint === 'source' ? edge.target : edge.source
    const fixedHandle = ensureVisualHandleId(
      args.endpoint === 'source' ? edge.targetHandle : edge.sourceHandle,
      args.endpoint === 'source' ? DEFAULT_TARGET_HANDLE_SIDE : DEFAULT_SOURCE_HANDLE_SIDE,
    ) ?? (args.endpoint === 'source'
      ? `${DEFAULT_TARGET_HANDLE_SIDE}-${HANDLE_SLOT_CENTER_INDEX}`
      : `${DEFAULT_SOURCE_HANDLE_SIDE}-${HANDLE_SLOT_CENTER_INDEX}`)
    const movingHandle = ensureVisualHandleId(
      args.handleId,
      args.endpoint === 'source' ? DEFAULT_SOURCE_HANDLE_SIDE : DEFAULT_TARGET_HANDLE_SIDE,
    ) ?? args.handleId

    setSyncedInteractionSourceId(null)
    setSyncedClickConnectMode(null)
    setClickConnectCursorPos(null)
    clearHandleReconnectListeners()
    isReconnectingRef.current = true
    const handleTargets = collectHandleTargets(fixedNodeId)
    connectorDragLastUpdateRef.current = 0
    syncHandleReconnectDrag({
      edgeId: args.edgeId,
      endpoint: args.endpoint,
      fixedNodeId,
      fixedHandle,
      movingHandle,
      cursorPos: { x: args.clientX, y: args.clientY },
    })

    const move = (event: PointerEvent) => {
      const now = performance.now()
      if (now - connectorDragLastUpdateRef.current < CONNECTOR_DRAG_UPDATE_INTERVAL_MS) return
      connectorDragLastUpdateRef.current = now
      const hit = findNearestHandleTargetInCache(handleTargets, event.clientX, event.clientY)
      const current = handleReconnectDragRef.current
      if (!current) return
      syncHandleReconnectDrag({
        ...current,
        cursorPos: hit.snapPos,
        hoveredNodeId: hit.hoveredNodeId,
        hoveredHandleId: hit.hoveredHandleId,
        movingHandle: hit.hoveredHandleId ?? current.movingHandle,
      })
    }

    const up = async (event: PointerEvent) => {
      const current = handleReconnectDragRef.current
      clearHandleReconnectListeners()
      handleReconnectDragRef.current = null
      setHandleReconnectDrag(null)
      isReconnectingRef.current = false
      suppressNextPaneClickRef.current = true
      if (!current) return

      const oldConnector = _rfEdgesRef.current.find((candidate) => candidate.id === current.edgeId)
      if (!oldConnector) return

      let newConnection: Connection | null = null

      if (current.hoveredNodeId && current.hoveredHandleId) {
        newConnection = current.endpoint === 'source'
          ? {
            source: current.hoveredNodeId,
            sourceHandle: current.hoveredHandleId,
            target: current.fixedNodeId,
            targetHandle: current.fixedHandle,
          }
          : {
            source: current.fixedNodeId,
            sourceHandle: current.fixedHandle,
            target: current.hoveredNodeId,
            targetHandle: current.hoveredHandleId,
          }
      } else {
        const flowPos = screenToFlowPositionRef.current({ x: event.clientX, y: event.clientY })
        const nearNode = rfNodesRef.current.find((node) => {
          if (node.id === current.fixedNodeId) return false
          const cx = node.position.x + (node.width ?? 180) / 2
          const cy = node.position.y + (node.height ?? 80) / 2
          return Math.hypot(flowPos.x - cx, flowPos.y - cy) < SNAP_RADIUS
        })

        if (nearNode) {
          const fixedNode = rfNodesRef.current.find((node) => node.id === current.fixedNodeId)
          if (!fixedNode) return

          if (current.endpoint === 'source') {
            const { sourceHandle, targetHandle } = findClosestHandles(nearNode, fixedNode)
            newConnection = {
              source: nearNode.id,
              sourceHandle: ensureVisualHandleId(sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? sourceHandle,
              target: fixedNode.id,
              targetHandle: ensureVisualHandleId(targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? targetHandle,
            }
          } else {
            const { sourceHandle, targetHandle } = findClosestHandles(fixedNode, nearNode)
            newConnection = {
              source: fixedNode.id,
              sourceHandle: ensureVisualHandleId(sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? sourceHandle,
              target: nearNode.id,
              targetHandle: ensureVisualHandleId(targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? targetHandle,
            }
          }
        }
      }

      if (!newConnection) return
      await performReconnect(oldConnector, newConnection)
    }

    handleReconnectListenersRef.current = { move, up }
    document.addEventListener('pointermove', move)
    document.addEventListener('pointerup', up)
    document.addEventListener('pointercancel', up)
  }, [canEdit, clearHandleReconnectListeners, performReconnect, rfNodesRef, _rfEdgesRef, setClickConnectCursorPos, setSyncedClickConnectMode, setSyncedInteractionSourceId, syncHandleReconnectDrag])

  const stableOnReconnectPick = useCallback(async (targetElementId: number) => {
    const picking = reconnectPickingRef.current
    if (!canEdit || !picking) return false

    const oldConnector = _rfEdgesRef.current.find((candidate) => candidate.id === String(picking.edgeId))
    const pickedNode = rfNodesRef.current.find((node) => node.id === String(targetElementId))
    if (!oldConnector || !pickedNode) return false

    const fixedNodeId = picking.endpoint === 'source' ? oldConnector.target : oldConnector.source
    if (fixedNodeId === pickedNode.id) return false
    const fixedNode = rfNodesRef.current.find((node) => node.id === fixedNodeId)
    if (!fixedNode) return false

    const closest = picking.endpoint === 'source'
      ? findClosestHandles(pickedNode, fixedNode)
      : findClosestHandles(fixedNode, pickedNode)
    const newConnection: Connection = picking.endpoint === 'source'
      ? {
        source: pickedNode.id,
        sourceHandle: ensureVisualHandleId(closest.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? closest.sourceHandle,
        target: fixedNode.id,
        targetHandle: ensureVisualHandleId(closest.targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? closest.targetHandle,
      }
      : {
        source: fixedNode.id,
        sourceHandle: ensureVisualHandleId(closest.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? closest.sourceHandle,
        target: pickedNode.id,
        targetHandle: ensureVisualHandleId(closest.targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? closest.targetHandle,
      }

    reconnectPickingRef.current = null
    setReconnectPicking(null)
    setConnectorLongPressMenu(null)
    await performReconnect(oldConnector, newConnection)
    return true
  }, [canEdit, _rfEdgesRef, performReconnect, rfNodesRef])

  // ── Click-connect ghost cursor tracking ────────────────────────────────────
  useEffect(() => {
    if (!clickConnectMode) {
      setClickConnectCursorPos(null)
      return
    }
    const handleTargets = collectHandleTargets(clickConnectMode.sourceNodeId)
    const sourceHandleTargets = collectHandleTargets().filter((target) => target.nodeId === clickConnectMode.sourceNodeId)
    connectorDragLastUpdateRef.current = 0
    const listener = (e: MouseEvent) => {
      const now = performance.now()
      if (now - connectorDragLastUpdateRef.current < CONNECTOR_DRAG_UPDATE_INTERVAL_MS) return
      connectorDragLastUpdateRef.current = now
      const hit = findNearestHandleTargetInCache(handleTargets, e.clientX, e.clientY)
      setClickConnectCursorPos(hit.snapPos)
      setSyncedClickConnectMode((prev) => {
        if (!prev) return null
        let bestHandle = prev.sourceHandle
        let minDist = Infinity
        for (const target of sourceHandleTargets) {
          const dist = Math.hypot(e.clientX - target.x, e.clientY - target.y)
          if (dist < minDist) {
            minDist = dist
            bestHandle = target.handleId
          }
        }
        if (prev.sourceHandle !== bestHandle || prev.targetHandle !== hit.hoveredHandleId) {
          return { ...prev, sourceHandle: bestHandle, targetHandle: hit.hoveredHandleId }
        }
        return prev
      })
    }
    document.addEventListener('mousemove', listener)
    return () => { document.removeEventListener('mousemove', listener) }
  }, [clickConnectMode, setClickConnectCursorPos, setSyncedClickConnectMode])

  useEffect(() => {
    if (interactionSourceId === null) setSyncedClickConnectMode(null)
  }, [interactionSourceId, setSyncedClickConnectMode])

  useEffect(() => () => {
    clearConnectorDragPreviewListeners()
    stopHandleReconnectDrag()
  }, [clearConnectorDragPreviewListeners, stopHandleReconnectDrag])

  // ── Connector interactions ─────────────────────────────────────────────────────
  const onEdgeContextMenu = useCallback((e: React.MouseEvent, rfConnector: RFEdge) => {
    if ((rfConnector.data as { isProxy?: boolean } | undefined)?.isProxy) return
    e.preventDefault()
    suppressNextConnectorClickRef.current = true
    const edgeId = parseNumericId(rfConnector.id)
    if (edgeId === null) return
    const rect = containerRef.current?.getBoundingClientRect()
    setConnectorLongPressMenu({ edgeId, x: e.clientX - (rect?.left ?? 0), y: e.clientY - (rect?.top ?? 0) })
  }, [containerRef])

  const onEdgeClick = useCallback((_: React.MouseEvent, rfConnector: RFEdge) => {
    if (suppressNextConnectorClickRef.current) { suppressNextConnectorClickRef.current = false; return }
    if ((rfConnector.data as { isProxy?: boolean; details?: import('../../../crossBranch/types').ProxyConnectorDetails } | undefined)?.isProxy) {
      setSelectedElement(null)
      closeElementPanel()
      setSelectedEdge(null)
      closeConnectorPanel()
      setSelectedProxyConnectorDetails((rfConnector.data as { details?: import('../../../crossBranch/types').ProxyConnectorDetails }).details ?? null)
      openProxyConnectorPanel()
      return
    }
    const clickedId = parseNumericId(rfConnector.id)
    if (clickedId === null) return
    const connector = connectors.find((e) => e.id === clickedId)
    if (!connector) return
    setSelectedElement(null)
    closeElementPanel()
    setSelectedProxyConnectorDetails(null)
    setSelectedEdge(connector)
    openConnectorPanelRef.current()
  }, [closeConnectorPanel, closeElementPanel, connectors, openProxyConnectorPanel, setSelectedEdge, setSelectedElement, setSelectedProxyConnectorDetails])

  // ── Pane interactions ─────────────────────────────────────────────────────
  const onPaneClick = useCallback((e: React.MouseEvent) => {
    if (suppressNextPaneClickRef.current) { suppressNextPaneClickRef.current = false; return }
    reconnectPickingRef.current = null
    setReconnectPicking(null)
    setSelectedElement(null)
    setSelectedEdge(null)
    setSelectedProxyConnectorDetails(null)
    setConnectorLongPressMenu(null)
    setCanvasMenu(null)
    closeElementPanel()
    closeConnectorPanel()
    closeProxyConnectorPanel()
    const sourceId = interactionSourceIdRef.current
    if (sourceId !== null) {
      const interactionNodes = getInteractionNodes()
      const flowPos = screenToFlowPositionRef.current({ x: e.clientX, y: e.clientY })
      const nearNode = interactionNodes.find((node) => {
        if (node.id === String(sourceId)) return false
        const cx = node.position.x + (node.width ?? 180) / 2
        const cy = node.position.y + (node.height ?? 80) / 2
        return Math.hypot(flowPos.x - cx, flowPos.y - cy) < SNAP_RADIUS
      })
      if (nearNode) {
        const targetId = resolveElementIdFromNode(nearNode)
        if (targetId === null) return
        stableOnConnectToAndReset(targetId)
      } else {
        setSyncedInteractionSourceId(null)
        pendingConnectionSourceRef.current = sourceId
        pendingConnectionSourceHandleRef.current = clickConnectModeRef.current?.sourceHandle
          ? getLogicalHandleId(clickConnectModeRef.current.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
          : pendingConnectionSourceHandleRef.current
        setSyncedClickConnectMode(null)
        setClickConnectCursorPos(null)
        showAddingElementAt(e.clientX, e.clientY, true, 'connect', e.shiftKey)
      }
      return
    }
    setSyncedInteractionSourceId(null)
    cancelPendingElement()
  }, [stableOnConnectToAndReset, showAddingElementAt, closeElementPanel, closeConnectorPanel, closeProxyConnectorPanel, getInteractionNodes, interactionSourceIdRef, cancelPendingElement, setClickConnectCursorPos, setSelectedElement, setSelectedEdge, setSelectedProxyConnectorDetails, setSyncedClickConnectMode, setSyncedInteractionSourceId])

  const onPaneContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    setConnectorLongPressMenu(null)
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    const flowPos = screenToFlowPositionRef.current({ x: e.clientX, y: e.clientY })
    const px = e.clientX - rect.left; const py = e.clientY - rect.top
    const x = Math.max(75, Math.min(px, rect.width - 75))
    const y = Math.max(190, Math.min(py, rect.height - 10))
    setCanvasMenu({ x, y, flowX: flowPos.x, flowY: flowPos.y })
  }, [containerRef])

  const onPaneMouseMove = useCallback((e: React.MouseEvent) => {
    lastMousePosRef.current = { clientX: e.clientX, clientY: e.clientY }
  }, [])

  const onMoveStart = useCallback(() => {
    if (isMovingRef.current) return
    isMovingRef.current = true
    setCanvasMenu(null)
    setConnectorLongPressMenu(null)
    onMoveStateChange?.(true)
  }, [onMoveStateChange])

  const onMove = useCallback((_: unknown, viewport: { x: number; y: number; zoom: number }) => {
    drawingCanvasRef.current?.notifyViewportChange(viewport)
  }, [drawingCanvasRef])

  const onMoveEnd = useCallback(() => {
    if (!isMovingRef.current) return
    isMovingRef.current = false
    onMoveStateChange?.(false)
  }, [onMoveStateChange])

  // ── Touch & long-press ────────────────────────────────────────────────────
  function getTouchDistance(touches: Map<number, { x: number; y: number }>): number {
    const points = Array.from(touches.values())
    if (points.length < 2) return 0
    const [p1, p2] = points
    return Math.hypot(p2.x - p1.x, p2.y - p1.y)
  }

  const onTouchStart = useCallback((e: React.TouchEvent) => {
    if (e.touches.length !== 2) { touchStateRef.current.touches.clear(); touchStateRef.current.isPinching = false; return }
    touchStateRef.current.touches.clear()
    for (let i = 0; i < 2; i++) {
      const t = e.touches[i]
      touchStateRef.current.touches.set(t.identifier, { x: t.clientX, y: t.clientY })
    }
    touchStateRef.current.initialDistance = getTouchDistance(touchStateRef.current.touches)
    touchStateRef.current.isPinching = false
  }, [])

  const onTouchMove = useCallback((e: React.TouchEvent) => {
    if (e.touches.length !== 2) return
    touchStateRef.current.touches.clear()
    for (let i = 0; i < 2; i++) {
      const t = e.touches[i]
      touchStateRef.current.touches.set(t.identifier, { x: t.clientX, y: t.clientY })
    }
    if (Math.abs(getTouchDistance(touchStateRef.current.touches) - touchStateRef.current.initialDistance) > 8) {
      touchStateRef.current.isPinching = true
    }
  }, [])

  const onTouchEnd = useCallback((e: React.TouchEvent) => {
    if (e.touches.length < 2) { touchStateRef.current.touches.clear(); touchStateRef.current.isPinching = false }
  }, [])

  const onContainerPointerDown = useCallback((e: React.PointerEvent) => {
    if (e.pointerType !== 'touch') return
    const target = e.target
    if (target instanceof globalThis.Element) {
      if (target.closest('.react-flow__node') || target.closest('.react-flow__connector')) return
    }
    const { clientX, clientY } = e
    longPressCanvasRef.current = {
      clientX, clientY,
      timer: setTimeout(() => {
        longPressCanvasRef.current = null
        const rect = containerRef.current?.getBoundingClientRect()
        if (!rect) return
        const flowPos = screenToFlowPositionRef.current({ x: clientX, y: clientY })
        const px = clientX - rect.left; const py = clientY - rect.top
        setCanvasMenu({
          x: Math.max(75, Math.min(px, rect.width - 75)),
          y: Math.max(190, Math.min(py, rect.height - 10)),
          flowX: flowPos.x, flowY: flowPos.y,
        })
      }, 600),
    }
  }, [containerRef])

  const onContainerPointerMove = useCallback((e: React.PointerEvent) => {
    lastMousePosRef.current = { clientX: e.clientX, clientY: e.clientY }
    if (!longPressCanvasRef.current) return
    const dx = e.clientX - longPressCanvasRef.current.clientX
    const dy = e.clientY - longPressCanvasRef.current.clientY
    if (Math.hypot(dx, dy) > 10) { clearTimeout(longPressCanvasRef.current.timer); longPressCanvasRef.current = null }
  }, [])

  const onContainerPointerUp = useCallback(() => {
    if (longPressCanvasRef.current) { clearTimeout(longPressCanvasRef.current.timer); longPressCanvasRef.current = null }
  }, [])

  // ── Hover pan ─────────────────────────────────────────────────────────────
  useEffect(() => {
    if (hoverPanTimeoutRef.current) { clearTimeout(hoverPanTimeoutRef.current); hoverPanTimeoutRef.current = null }
    const elementId = hoveredZoomRef.current?.elementId
    if (!elementId) return
    hoverPanTimeoutRef.current = setTimeout(() => {
      hoverPanTimeoutRef.current = null
      if (Date.now() < hoverPanLockedUntilRef.current) return
      const node = rfNodesRef.current.find((n) => n.id === String(elementId))
      if (!node) return
      const container = containerRef.current
      if (!container) return
      const { width: cw, height: ch } = container.getBoundingClientRect()
      const { x: vx, y: vy, zoom } = getViewport()
      const nodeW = (node.width ?? 200) * zoom; const nodeH = (node.height ?? 90) * zoom
      const sx = node.position.x * zoom + vx; const sy = node.position.y * zoom + vy
      const pad = 80
      let dx = 0; let dy = 0
      if (sx < pad) dx = pad - sx
      else if (sx + nodeW > cw - pad) dx = (cw - pad) - (sx + nodeW)
      if (sy < pad) dy = pad - sy
      else if (sy + nodeH > ch - pad) dy = (ch - pad) - (sy + nodeH)
      if (dx === 0 && dy === 0) return
      hoverPanLockedUntilRef.current = Date.now() + 900
      setViewport({ x: vx + dx, y: vy + dy, zoom }, { duration: 450 })
    }, 320)
  }, [hoveredZoomRef, hoverPanLockedUntilRef, rfNodesRef, containerRef, getViewport, setViewport])

  // ── Keyboard navigation (WASD, e, c, delete) ──────────────────────────────
  useEffect(() => {
    const handler = async (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      const isInput = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' ||
        target?.tagName === 'SELECT' || target?.isContentEditable
      if (isInput) return
      if (e.key === 'Escape') {
        e.preventDefault()
        if (selectedElement || selectedConnector) {
          setSelectedElement(null)
          setSelectedEdge(null)
          closeElementPanel()
          closeConnectorPanel()
          closeProxyConnectorPanel()
        }
        reconnectPickingRef.current = null
        clearConnectionSourceRefs()
        clickConnectModeRef.current = null
        setReconnectPicking(null)
        setConnectorLongPressMenu(null)
        setPendingElement(null)
        setSyncedClickConnectMode(null)
        setClickConnectCursorPos(null)
        setSyncedInteractionSourceId(null)
        return
      }
      const key = e.key.toLowerCase()
      if (!['w', 'a', 's', 'd', 'm', 'c', 'e', 'backspace', 'delete', 'r', 'f', 'g', '+', '=', '-', '/'].includes(key)) return
      if (e.ctrlKey || e.altKey || (e.metaKey && key !== 'z')) return
      if (key === 'c' && e.shiftKey) return

      if (key === 'backspace' || key === 'delete' || key === 'r') {
        if (!canEdit) return
        e.preventDefault()
        const selectedElementNodes = rfNodesRef.current.filter((node) => node.selected && node.type === 'elementNode' && parseNumericId(node.id) !== null)
        const selectedElementIds = selectedElementNodes
          .map((node) => parseNumericId(node.id))
          .filter((id): id is number => id !== null)
        if (selectedElementIds.length > 1) {
          if (key === 'r' && e.shiftKey) {
            await Promise.all(selectedElementIds.map((elementId) =>
              api.elements.delete('', elementId).then(() => {
                handleElementPermanentlyDeleted(elementId)
              }).catch(() => { /* intentionally empty */ })
            ))
            setSelectedElement(null)
            setSelectedEdge(null)
            closeElementPanel()
            closeConnectorPanel()
          } else if (onSelectionRemoveFromView) {
            await onSelectionRemoveFromView()
          }
          return
        }
        if (selectedElement) {
          if (key === 'r' && e.shiftKey) {
            api.elements.delete('', selectedElement.id).then(() => {
              handleElementPermanentlyDeleted(selectedElement.id)
              setSelectedElement(null)
              closeElementPanel()
            }).catch(() => { /* intentionally empty */ })
          } else {
            stableOnRemoveElement(selectedElement.id)
            setSelectedElement(null)
            closeElementPanel()
          }
        } else {
          const connectorId = getConnectorDeletionTarget(selectedConnector)
          if (connectorId === null) {
            if (key !== 'r' && onSelectionRemoveFromView) {
              if (selectedElementNodes.length > 0) await onSelectionRemoveFromView()
            }
            return
          }
          const deletedConnector = selectedConnector?.id === connectorId
            ? selectedConnector
            : connectors.find((connector) => connector.id === connectorId) ?? null
          api.workspace.connectors.delete('', connectorId).then(() => {
            if (deletedConnector) onConnectorDeleted?.(deletedConnector)
            handleConnectorDeleted(connectorId)
            setSelectedEdge(null)
            closeConnectorPanel()
          }).catch(() => { /* intentionally empty */ })
        }
        return
      }
      e.preventDefault()
      if (key === 'c') {
        if (!canEdit) return
        const rect = containerRef.current?.getBoundingClientRect()
        if (!rect) return
        let cx = rect.left + rect.width / 2
        let cy = rect.top + rect.height * 0.4
        if (lastMousePosRef.current) {
          const { clientX, clientY } = lastMousePosRef.current
          if (clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom) {
            cx = clientX; cy = clientY
          }
        }
        showAddingElementAt(cx, cy, true)
        return
      }
      if (key === 'e') {
        if (!canEdit) return
        const selectedElementIds = rfNodesRef.current
          .filter((node) => node.selected && node.type === 'elementNode')
          .map((node) => parseNumericId(node.id))
          .filter((id): id is number => id !== null)
        const sourceElementIds = selectedElementIds.length > 1
          ? selectedElementIds
          : selectedElement
            ? [selectedElement.id]
            : []
        if (sourceElementIds.length === 0) return
        const primarySourceId = sourceElementIds[0]
        const node = rfNodesRef.current.find((n) => n.id === String(primarySourceId))
        if (!node) return
        const cursor = lastMousePosRef.current
        const flowCursor = cursor
          ? screenToFlowPositionRef.current({ x: cursor.clientX, y: cursor.clientY })
          : { x: node.position.x + (node.width ?? 180), y: node.position.y + (node.height ?? 80) / 2 }
        const { sourceHandle } = findClosestHandleToPoint(node, flowCursor.x, flowCursor.y)
        const nextClickConnectMode = {
          sourceNodeId: node.id,
          sourceHandle: ensureVisualHandleId(sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? sourceHandle,
        }
        suppressNextPaneClickRef.current = false
        multiConnectionSourceIdsRef.current = sourceElementIds.length > 1 ? sourceElementIds : null
        clickConnectModeRef.current = nextClickConnectMode
        interactionSourceIdRef.current = primarySourceId
        setSyncedClickConnectMode(nextClickConnectMode)
        setSyncedInteractionSourceId(primarySourceId)
        if (cursor) setClickConnectCursorPos({ x: cursor.clientX, y: cursor.clientY })
        return
      }

      if (key === 'f') {
        e.preventDefault()
        onFitView?.()
        return
      }

      if (key === 'g') {
        e.preventDefault()
        setGlobalSnapToGrid?.(!snapToGrid)
        return
      }

      if (key === '/') {
        e.preventDefault()
        // Toggle the library panel if it's not already open
        if (!libraryOpen && openLibrary) {
          openLibrary()
        }
        // Focus search in open panel (might need a tiny timeout to wait for panel to mount/open)
        setTimeout(() => {
          const searchInput = document.querySelector<HTMLInputElement>('.panel-search-input')
          searchInput?.focus()
        }, 10)
        return
      }

      if (key === '+' || key === '=') {
        e.preventDefault()
        zoomIn()
        return
      }

      if (key === '-') {
        e.preventDefault()
        zoomOut()
        return
      }

      if (key === 'a') {
        e.preventDefault()
        toggleLibrary?.()
        return
      }

      if (key === 'd') {
        e.preventDefault()
        toggleExplorer?.()
        return
      }

      if (key === 'm') {
        e.preventDefault()
        toggleMarkdown?.()
        return
      }
      const cid = viewIdRef.current
      if (!cid) return
      const incoming = incomingLinksRef.current
      const tree = flattenViewTree(treeDataRef.current)
      const nav = navigateRef.current
      const links = linksMapRef.current
      const treeNode = tree.find((n) => n.id === cid)

      // Parents: parent_view_id + incoming links
      const parentIds = new Set<number>()
      if (treeNode?.parent_view_id) parentIds.add(treeNode.parent_view_id)
      incoming.forEach(l => parentIds.add(l.from_view_id))
      const allParents = Array.from(parentIds).sort((a, b) => a - b)

      // Children: direct children in tree + linksMap
      const childIds = new Set<number>()
      tree.filter(n => n.parent_view_id === cid).forEach(n => childIds.add(n.id))
      Object.values(links).flat().forEach(l => childIds.add(l.to_view_id))
      const allChildren = Array.from(childIds).sort((a, b) => a - b)

      // Siblings: views with same parent or at same level
      const siblings = tree
        .filter((n) => n.parent_view_id === treeNode?.parent_view_id && n.id !== cid)
        .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
      const allSiblings = [treeNode, ...siblings].filter(Boolean) as ViewTreeNode[]
      allSiblings.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())

      if (e.shiftKey) {
        if (!canEdit) return
        if (key === 'w') {
          const primaryObj = incoming[0]; if (!primaryObj) return
          try {
            const newDiag = await api.workspace.views.create({ name: `${primaryObj.element_name}` })
            await api.workspace.views.placements.add(newDiag.id, primaryObj.element_id, 200, 200)
            await refreshGrid()
            nav(`/views/${newDiag.id}`)
          } catch { /* intentionally empty */ }
        } else if (key === 's') {
          const firstObj = viewElementsRef.current[0]; if (!firstObj) return
          try {
            const newDiag = await api.workspace.views.create({ name: `${firstObj.name}`, parent_view_id: firstObj.element_id })
            setLinksMap((prev) => ({ ...prev, [firstObj.element_id]: [...(prev[firstObj.element_id] || []), { id: 0, element_id: firstObj.element_id, from_view_id: cid, to_view_id: newDiag.id, to_view_name: newDiag.name, relation_type: 'child' as const }] }))
            await refreshGrid()
            nav(`/views/${newDiag.id}`)
          } catch { /* intentionally empty */ }
        } else {
          const primaryObj = incoming[0]; if (!primaryObj) return
          try {
            const newDiag = await api.workspace.views.create({ name: `${primaryObj.element_name} - Layer`, parent_view_id: primaryObj.element_id })
            await api.workspace.views.placements.add(newDiag.id, primaryObj.element_id, 200, 200)
            await refreshGrid()
            nav(`/views/${newDiag.id}`)
          } catch { /* intentionally empty */ }
        }
      } else {
        if (key === 'w') {
          if (allParents.length > 0) nav(`/views/${allParents[0]}`)
        } else if (key === 's') {
          if (allChildren.length > 0) nav(`/views/${allChildren[0]}`)
        }
      }
    }
    window.addEventListener('keydown', handler, { capture: true })
    return () => window.removeEventListener('keydown', handler, { capture: true })
  }, [canEdit, refreshGrid, selectedElement, selectedConnector, connectors, viewId, stableOnRemoveElement, handleConnectorDeleted, handleElementPermanentlyDeleted, onConnectorDeleted, onSelectionRemoveFromView, closeElementPanel, closeConnectorPanel, closeProxyConnectorPanel, clickConnectMode, setClickConnectCursorPos, setSyncedClickConnectMode, setSyncedInteractionSourceId, viewIdRef, incomingLinksRef, treeDataRef, navigateRef, rfNodesRef, viewElementsRef, setLinksMap, showAddingElementAt, setSelectedElement, setSelectedEdge, containerRef, linksMapRef, interactionSourceIdRef, multiConnectionSourceIdsRef, clearConnectionSourceRefs, onFitView, setGlobalSnapToGrid, snapToGrid, libraryOpen, openLibrary, toggleLibrary, toggleExplorer, toggleMarkdown, zoomIn, zoomOut])

  // ── DnD handlers ──────────────────────────────────────────────────────────
  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault(); e.dataTransfer.dropEffect = 'move'
  }, [])

  const onDrop = useCallback(async (e: React.DragEvent) => {
    if (!canEdit || !viewId) return
    e.preventDefault()

    // 1. Check for View Element drop (existing functionality)
    const rawObj = e.dataTransfer.getData('application/diag-element')
    if (rawObj) {
      const obj: { id: number } = JSON.parse(rawObj)
      if (existingElementIds.has(obj.id)) return
      const pos = screenToFlowPositionRef.current({ x: e.clientX, y: e.clientY })
      try {
        await api.workspace.views.placements.add(viewId, obj.id, pos.x - 100, pos.y - 40)
        onUnsupportedMutation?.()
        await refreshElements()
        const placed = viewElementsRef.current.find((element) => element.element_id === obj.id)
        if (placed) upsertPlacementGraphSnapshot(viewId, placed)
      } catch { /* ignored */ }
      return
    }

    // 2. Check for Tag or Layer drop
    const tagName = e.dataTransfer.getData('application/diag-tag')
    const layerIdStr = e.dataTransfer.getData('application/diag-layer')

    if (tagName || layerIdStr) {
      const flowPos = screenToFlowPositionRef.current({ x: e.clientX, y: e.clientY })
      // Find node under drop position
      const nodeUnderDrop = rfNodesRef.current.find((node) => {
        const x = node.position.x
        const y = node.position.y
        const w = node.width ?? 180
        const h = node.height ?? 80
        return flowPos.x >= x && flowPos.x <= x + w && flowPos.y >= y && flowPos.y <= y + h
      })

      if (nodeUnderDrop) {
        const elementId = parseNumericId(nodeUnderDrop.id)
        if (elementId === null) return

        // Get existing element tags
        const element = viewElementsRef.current.find(o => o.element_id === elementId)
        if (!element) return

        const nextTags = [...(element.tags || [])]

        if (tagName) {
          if (!nextTags.includes(tagName)) nextTags.push(tagName)
        } else if (layerIdStr) {
          const layerId = Number(layerIdStr)
          const layer = layers.find(l => l.id === layerId)
          if (layer) {
            // Merge logic: "if tag A exists and group(A&B) is added remove tag A let group take its place"
            // Since tags are just flat strings, we just ensure all group tags are present.
            layer.tags.forEach(t => {
              if (!nextTags.includes(t)) nextTags.push(t)
            })
          }
        }

        if (nextTags.length !== (element.tags?.length ?? 0)) {
          await handleUpdateTags(elementId, nextTags)
        }
      }
    }
  }, [canEdit, viewId, existingElementIds, onUnsupportedMutation, refreshElements, rfNodesRef, viewElementsRef, layers, handleUpdateTags])

  const onWheelCapture = useCallback((e: React.WheelEvent) => {
    if (touchStateRef.current.touches.size === 2) return
    if (e.deltaX !== 0) touchStateRef.current.lastMultiTouchWheelTime = Date.now()
    const isRecentMultiTouch = Date.now() - touchStateRef.current.lastMultiTouchWheelTime < 1000
    const isMouseWheel = isMouseWheelGesture(e)
    if (isMouseWheel && !isRecentMultiTouch) {
      e.preventDefault()
      e.stopPropagation()
      if (e.deltaY > 0) zoomOut()
      else zoomIn()
    }
  }, [zoomIn, zoomOut])

  return {
    // State
    canvasMenu,
    setCanvasMenu,
    pendingElement,
    clickConnectMode,
    clickConnectCursorPos,
    handleReconnectDrag,
    interactionSourceId,
    setInteractionSourceId: setSyncedInteractionSourceId,
    reconnectPicking,
    setReconnectPicking,
    reconnectPickingRef,
    connectorLongPressMenu,
    setConnectorLongPressMenu,
    // Refs
    screenToFlowPositionRef,
    lastMousePosRef,
    touchStateRef,
    // Stable callbacks passed to node data
    stableOnZoomIn,
    stableOnZoomOut,
    stableOnNavigateToView,
    stableOnHoverZoom,
    stableOnRemoveElement,
    stableOnConnectTo: stableOnConnectToAndReset,
    stableOnInteractionStart,
    stableOnStartHandleReconnect,
    stableOnReconnectPick,
    showAddingElementAt,
    cancelPendingElement,
    // RF event handlers
    onNodesChange,
    onEdgesChange,
    onNodeDragStart,
    onNodeDrag,
    onNodeDragStop,
    onConnect,
    onConnectStart,
    onConnectEnd,
    onReconnect,
    onReconnectStart,
    onReconnectEnd,
    onEdgeClick,
    onEdgeContextMenu,
    onPaneClick,
    onPaneContextMenu,
    onPaneMouseMove,
    onMoveStart,
    onMove,
    onMoveEnd,
    // Container event handlers
    onTouchStart,
    onTouchMove,
    onTouchEnd,
    onContainerPointerDown,
    onContainerPointerMove,
    onContainerPointerUp,
    onDragOver,
    onDrop,
    onWheelCapture,
    // Library / adder callbacks
    handleConfirmNewElement,
    handleConfirmExistingElement,
  }
}
