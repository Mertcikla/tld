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

const SNAP_RADIUS = 75
const CONNECTOR_DRAG_UPDATE_INTERVAL_MS = 25

type HandleTarget = {
  nodeId?: string
  handleId: string
  x: number
  y: number
}

function collectHandleTargets(excludeNodeId?: string): HandleTarget[] {
  const handles = document.querySelectorAll('.react-flow__handle')
  const targets: HandleTarget[] = []

  for (const handle of handles) {
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

interface CanvasInteractionOptions {
  viewId: number | null
  canEdit: boolean
  
  drawingMode: boolean
  isMobileLayout: boolean
  rfNodesRef: React.MutableRefObject<RFNode[]>
  rfEdgesRef: React.MutableRefObject<RFEdge[]>
  viewElementsRef: React.MutableRefObject<PlacedElement[]>
  viewIdRef: React.MutableRefObject<number | null>
  incomingLinksRef: React.MutableRefObject<IncomingViewConnector[]>
  treeDataRef: React.MutableRefObject<ViewTreeNode[]>
  navigateRef: React.MutableRefObject<(path: string) => void>
  containerRef: React.MutableRefObject<HTMLDivElement | null>
  interactionSourceIdRef: React.MutableRefObject<number | null>
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
  selectedEdgeId: number | null
  connectors: Connector[]
  layers: ViewLayer[]
  setSelectedElement: React.Dispatch<React.SetStateAction<LibraryElement | null>>
  setSelectedEdge: (e: Connector | null) => void
  setSelectedEdgeId: (id: number | null) => void
  setSelectedProxyConnectorDetails: React.Dispatch<React.SetStateAction<import('../../../crossBranch/types').ProxyConnectorDetails | null>>
  openProxyConnectorPanel: () => void
  closeProxyConnectorPanel: () => void
  handleElementDeleted: (id: number) => void
  handleElementPermanentlyDeleted: (id: number) => void
  handleConnectorDeleted: (id: number) => void
  handleUpdateTags: (elementId: number, tags: string[]) => Promise<void>
  drawingCanvasRef: React.MutableRefObject<DrawingCanvasHandle | null>
  snapToGrid?: boolean
  onMoveStateChange?: (isMoving: boolean) => void
}

type PickerState = {
  x: number
  y: number
  flowX: number
  flowY: number
  expandResults?: boolean
  mode: 'add' | 'connect'
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

export function useCanvasInteractions({
  viewId,
  canEdit,
  
  drawingMode: _drawingMode,
  isMobileLayout: _isMobileLayout,
  rfNodesRef,
  rfEdgesRef: _rfEdgesRef,
  viewElementsRef,
  viewIdRef,
  incomingLinksRef,
  treeDataRef,
  navigateRef,
  containerRef,
  interactionSourceIdRef,
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
  selectedEdgeId,
  connectors,
  layers,
  setSelectedElement,
  setSelectedEdge,
  setSelectedEdgeId,
  setSelectedProxyConnectorDetails,
  openProxyConnectorPanel,
  closeProxyConnectorPanel,
  handleElementDeleted,
  handleElementPermanentlyDeleted,
  handleConnectorDeleted,
  handleUpdateTags,
  drawingCanvasRef,
  snapToGrid,
  onMoveStateChange,
}: CanvasInteractionOptions) {
  const { screenToFlowPosition, setViewport, getViewport, zoomIn, zoomOut } = useReactFlow()
  const updateElementPosition = useStore((state) => state.updateElementPosition)
  const removeElementPlacement = useStore((state) => state.removeElementPlacement)
  const upsertConnector = useStore((state) => state.upsertConnector)
  const replaceConnector = useStore((state) => state.replaceConnector)
  const screenToFlowPositionRef = useRef(screenToFlowPosition)
  screenToFlowPositionRef.current = screenToFlowPosition

  const [canvasMenu, setCanvasMenu] = useState<{ x: number; y: number; flowX: number; flowY: number } | null>(null)
  const [addingElementAt, setAddingElementAt] = useState<PickerState | null>(null)
  const [connectGhostPos, setConnectGhostPos] = useState<{ x: number; y: number } | null>(null)
  const [clickConnectMode, setClickConnectMode] = useState<{ sourceNodeId: string; sourceHandle: string; targetHandle?: string } | null>(null)
  const [clickConnectCursorPos, setClickConnectCursorPos] = useState<{ x: number; y: number } | null>(null)
  const [interactionSourceId, setInteractionSourceId] = useState<number | null>(null)
  const [pendingConnectionSource, setPendingConnectionSource] = useState<number | null>(null)
  const [reconnectPicking, setReconnectPicking] = useState<{ edgeId: number; endpoint: 'source' | 'target' } | null>(null)
  const [handleReconnectDrag, setHandleReconnectDrag] = useState<HandleReconnectDragState | null>(null)
  const [connectorLongPressMenu, setConnectorLongPressMenu] = useState<{ edgeId: number; x: number; y: number } | null>(null)
  const isMovingRef = useRef(false)

  interactionSourceIdRef.current = interactionSourceId

  const reconnectPickingRef = useRef<{ edgeId: number; endpoint: 'source' | 'target' } | null>(null)
  const handleReconnectDragRef = useRef<HandleReconnectDragState | null>(null)
  const handleReconnectListenersRef = useRef<{ move: (event: PointerEvent) => void; up: (event: PointerEvent) => void } | null>(null)
  const connectingSourceRef = useRef<string | null>(null)
  const connectWasValidRef = useRef(false)
  const connectGhostListenerRef = useRef<((e: MouseEvent) => void) | null>(null)
  const connectorDragLastUpdateRef = useRef(0)
  const isReconnectingRef = useRef(false)
  const suppressNextConnectorClickRef = useRef(false)
  const suppressNextPaneClickRef = useRef(false)
  const longPressCanvasRef = useRef<{ timer: ReturnType<typeof setTimeout>; clientX: number; clientY: number } | null>(null)
  const pendingConnectionSourceRef = useRef(pendingConnectionSource)
  pendingConnectionSourceRef.current = pendingConnectionSource
  const clickConnectModeRef = useRef(clickConnectMode)
  clickConnectModeRef.current = clickConnectMode
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

  const stopHandleReconnectDrag = useCallback(() => {
    clearHandleReconnectListeners()
    handleReconnectDragRef.current = null
    setHandleReconnectDrag(null)
    isReconnectingRef.current = false
  }, [clearHandleReconnectListeners])

  // ── Ref-forwarded callbacks ────────────────────────────────────────────────
  const openConnectorPanelRef = useRef(openConnectorPanel)
  openConnectorPanelRef.current = openConnectorPanel

  const resolvePickerMode = useCallback((flowX: number, flowY: number, preferredMode: 'add' | 'connect', forceConnect = false) => {
    if (preferredMode !== 'connect') return preferredMode
    if (forceConnect) return 'connect'

    const boundaryNode = rfNodesRef.current.find((node) => node.type === 'ContextBoundaryElement')
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
  }, [rfNodesRef])

  // ── showAddingElementAt ─────────────────────────────────────────────────────
  const showAddingElementAt = useCallback((clientX: number, clientY: number, expandResults = false, mode: 'add' | 'connect' = 'add', forceConnect = false) => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    const flowPos = screenToFlowPositionRef.current({ x: clientX, y: clientY })
    let { x: flowX, y: flowY } = flowPos
    if (snapToGrid) {
      flowX = Math.round(flowX / 10) * 10
      flowY = Math.round(flowY / 10) * 10
    }
    const px = clientX - rect.left
    const py = clientY - rect.top
    const x = expandResults
      ? Math.max(100, Math.min(px, rect.width - 450))
      : Math.max(120, Math.min(px, rect.width - 120))
    const y = Math.max(40, Math.min(py, rect.height - 250))
    setAddingElementAt({ x, y, flowX, flowY, expandResults, mode: resolvePickerMode(flowX, flowY, mode, forceConnect) })
  }, [containerRef, snapToGrid, resolvePickerMode])

  // ── Inline element adder handlers ───────────────────────────────────────────
  const handleConfirmNewElement = useCallback(async (name: string) => {
    if (!canEdit || viewId === null || !addingElementAt || addingElementAt.mode !== 'add') return
    const { flowX, flowY } = addingElementAt
    const sourceId = pendingConnectionSourceRef.current
    setAddingElementAt(null)
    setPendingConnectionSource(null)
    try {
      const obj = await api.elements.create({ name, kind: '' })
      await api.workspace.views.placements.add(viewId, obj.id, flowX - 100, flowY - 40)
      await refreshElements()
      const placed = viewElementsRef.current.find((element) => element.element_id === obj.id)
      if (placed) upsertPlacementGraphSnapshot(viewId, placed)
      if (sourceId !== null && sourceId !== obj.id) {
        const sourceNode = rfNodesRef.current.find((n) => n.id === String(sourceId))
        const { sourceHandle, targetHandle } = sourceNode
          ? findClosestHandleToPoint(sourceNode, flowX, flowY)
          : { sourceHandle: 'right', targetHandle: 'left' }
        const newConnector = await api.workspace.connectors.create(viewId, {
          source_element_id: sourceId, target_element_id: obj.id,
          source_handle: sourceHandle, target_handle: targetHandle, direction: 'forward',
        })
        const connector = connectorToConnector(newConnector)
        upsertConnectorGraphSnapshot(connector)
        upsertConnector(connector)
      }
    } catch { /* intentionally empty */ }
  }, [canEdit, viewId, addingElementAt, refreshElements, rfNodesRef, upsertConnector, viewElementsRef])

  const handleConfirmExistingElement = useCallback(async (obj: LibraryElement) => {
    if (!canEdit || viewId === null || !addingElementAt || addingElementAt.mode !== 'add') return
    const { flowX, flowY } = addingElementAt
    const sourceId = pendingConnectionSourceRef.current
    setAddingElementAt(null)
    setPendingConnectionSource(null)
    try {
      if (!existingElementIds.has(obj.id)) {
        await api.workspace.views.placements.add(viewId, obj.id, flowX - 100, flowY - 40)
        await refreshElements()
        const placed = viewElementsRef.current.find((element) => element.element_id === obj.id)
        if (placed) upsertPlacementGraphSnapshot(viewId, placed)
      }
      if (sourceId !== null && sourceId !== obj.id) {
        const sourceNode = rfNodesRef.current.find((n) => n.id === String(sourceId))
        const targetNode = rfNodesRef.current.find((n) => n.id === String(obj.id))
        const { sourceHandle, targetHandle } = sourceNode && targetNode
          ? findClosestHandles(sourceNode, targetNode)
          : sourceNode
            ? findClosestHandleToPoint(sourceNode, flowX, flowY)
            : { sourceHandle: 'right', targetHandle: 'left' }
        const newConnector = await api.workspace.connectors.create(viewId, {
          source_element_id: sourceId, target_element_id: obj.id,
          source_handle: sourceHandle, target_handle: targetHandle, direction: 'forward',
        })
        const connector = connectorToConnector(newConnector)
        upsertConnectorGraphSnapshot(connector)
        upsertConnector(connector)
      }
    } catch { /* intentionally empty */ }
  }, [canEdit, viewId, addingElementAt, existingElementIds, refreshElements, rfNodesRef, upsertConnector, viewElementsRef])

  const handleConfirmConnectExistingElement = useCallback(async (obj: LibraryElement) => {
    if (!canEdit || viewId === null || !addingElementAt || addingElementAt.mode !== 'connect') return
    const { flowX, flowY } = addingElementAt
    const sourceId = pendingConnectionSourceRef.current
    setAddingElementAt(null)
    setPendingConnectionSource(null)
    if (sourceId == null || sourceId === obj.id) return
    try {
      const sourceNode = rfNodesRef.current.find((n) => n.id === String(sourceId))
      const { sourceHandle, targetHandle } = sourceNode
        ? findClosestHandleToPoint(sourceNode, flowX, flowY)
        : { sourceHandle: 'right', targetHandle: 'left' }
      const newConnector = await api.workspace.connectors.create(viewId, {
        source_element_id: sourceId,
        target_element_id: obj.id,
        source_handle: sourceHandle,
        target_handle: targetHandle,
        direction: 'forward',
      })
      const connector = connectorToConnector(newConnector)
      upsertConnectorGraphSnapshot(connector)
      upsertConnector(connector)
    } catch { /* intentionally empty */ }
  }, [addingElementAt, canEdit, rfNodesRef, upsertConnector, viewId])

  // ── Zoom-in / zoom-out stable callbacks ───────────────────────────────────
  const stableOnZoomIn = useCallback(async (elementId: number) => {
    const childLinks = linksMapRef.current[elementId] || []
    if (childLinks.length > 0) { navigateRef.current(`/views/${childLinks[0].to_view_id}`); return }

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
      navigateRef.current(`/views/${newView.id}`)
    } catch { /* intentionally empty */ }
  }, [canEdit, linksMapRef, viewIdRef, viewElementsRef, navigateRef, setLinksMap, treeDataRef])

  const stableOnZoomOut = useCallback(async (elementId: number) => {
    const parentLinks = parentLinksMapRef.current[elementId] || []
    // If the clicked element has no direct parent link, fall back to the current
    // view's parent stored under the view's owner element ID (which may differ
    // from the clicked element's ID for elements like functions/classes that
    // don't own a view themselves).
    const anyParentLink = parentLinks[0] ?? Object.values(parentLinksMapRef.current).flat()[0]
    if (anyParentLink) { navigateRef.current(`/views/${anyParentLink.from_view_id}`); return }

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
      navigateRef.current(`/views/${currentView.parent_view_id}`)
    }
  }, [parentLinksMapRef, navigateRef, treeDataRef, viewIdRef])

  const stableOnNavigateToView = useCallback((id: number) => {
    navigateRef.current(`/views/${id}`)
  }, [navigateRef])

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
    try {
      await api.workspace.views.placements.remove(viewId, elementId)
      removePlacementGraphSnapshot(viewId, elementId)
      removeElementPlacement(elementId)
      handleElementDeleted(elementId)
      setInteractionSourceId(null)
    } catch { /* intentionally empty */ }
  }, [canEdit, viewId, removeElementPlacement, handleElementDeleted])

  // ── Node/connector changes ─────────────────────────────────────────────────────
  const onNodesChange = useCallback((changes: NodeChange[]) => {
    if (!canEdit) {
      const nonMutating = changes.filter((c) => c.type !== 'position')
      if (nonMutating.length === 0) return
      setRfNodes((nds) => applyNodeChangesWithStructuralSharing(nonMutating, nds))
      return
    }
    setRfNodes((nds) => applyNodeChangesWithStructuralSharing(changes, nds))
  }, [canEdit, setRfNodes])

  const onEdgesChange = useCallback((changes: EdgeChange[]) => {
    setRfEdges((eds) => applyEdgeChanges(changes, eds))
  }, [setRfEdges])

  const onNodeDragStart: NodeDragHandler = useCallback((_e, node) => {
    if (!canEdit || viewId === null) return
    const elementId = parseNumericId(node.id)
    if (elementId === null) return
    dragStartPositionsRef.current[node.id] = { x: node.position.x, y: node.position.y }
  }, [canEdit, viewId])

  const onNodeDrag: NodeDragHandler = useCallback(() => {
    // React Flow already updates rfNodes via onNodesChange while dragging.
    // Mirroring into viewElements here forces every derived edge/node to rebuild
    // on each pointer frame, so persist to app state only on drag stop.
  }, [])

  const positionTimers = useRef<Record<string, ReturnType<typeof setTimeout>>>({})
  const dragStartPositionsRef = useRef<Record<string, { x: number; y: number }>>({})
  const onNodeDragStop: NodeDragHandler = useCallback((_e, node) => {
    if (!canEdit || viewId === null) return
    const elementId = parseNumericId(node.id)
    if (elementId === null) return

    // Skip update if position hasn't changed (prevents redundant calls on simple clicks)
    const currentObj = viewElementsRef.current.find((o) => o.element_id === elementId)
    const startPos = dragStartPositionsRef.current[node.id] ?? (currentObj ? { x: currentObj.position_x, y: currentObj.position_y } : null)
    if (startPos && Math.abs(startPos.x - node.position.x) < 2 && Math.abs(startPos.y - node.position.y) < 2) {
      delete dragStartPositionsRef.current[node.id]
      return
    }

    updateElementPosition(elementId, node.position.x, node.position.y)
    clearTimeout(positionTimers.current[node.id])
    positionTimers.current[node.id] = setTimeout(() => {
      api.workspace.views.placements
        .updatePosition(viewId, elementId, node.position.x, node.position.y)
        .catch(() => { /* intentionally empty */ })
    }, 400)
    delete dragStartPositionsRef.current[node.id]
  }, [canEdit, updateElementPosition, viewId, viewElementsRef])

  // ── Connections ────────────────────────────────────────────────────────────
  const onConnect: OnConnect = useCallback(async (params: Connection) => {
    if (!canEdit || isReconnectingRef.current) return
    connectWasValidRef.current = true
    if (viewId === null || !params.source || !params.target) return
    const sourceId = parseNumericId(params.source)
    const targetId = parseNumericId(params.target)
    if (sourceId === null || targetId === null) return
    try {
      const sourceHandle = getLogicalHandleId(params.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE)
      const targetHandle = getLogicalHandleId(params.targetHandle, DEFAULT_TARGET_HANDLE_SIDE)
      const newConnector = await api.workspace.connectors.create(viewId, {
        source_element_id: sourceId, target_element_id: targetId,
        source_handle: sourceHandle, target_handle: targetHandle,
        direction: 'forward', style: 'bezier',
      })
      const connector = connectorToConnector(newConnector)
      upsertConnectorGraphSnapshot(connector)
      upsertConnector(connector)
    } catch { /* intentionally empty */ }
  }, [canEdit, upsertConnector, viewId])

  const onConnectStart = useCallback((_: React.MouseEvent | React.TouchEvent, { nodeId }: OnConnectStartParams) => {
    if (!canEdit || isReconnectingRef.current) return
    connectingSourceRef.current = nodeId
    connectWasValidRef.current = false
    const handleTargets = collectHandleTargets(nodeId ?? undefined)
    connectorDragLastUpdateRef.current = 0
    const listener = (e: MouseEvent) => {
      const now = performance.now()
      if (now - connectorDragLastUpdateRef.current < CONNECTOR_DRAG_UPDATE_INTERVAL_MS) return
      connectorDragLastUpdateRef.current = now
      const hit = findNearestHandleTargetInCache(handleTargets, e.clientX, e.clientY)
      setConnectGhostPos(hit.nearHandle ? null : { x: e.clientX, y: e.clientY })
    }
    connectGhostListenerRef.current = listener
    document.addEventListener('mousemove', listener)
  }, [canEdit])

  const onConnectEnd = useCallback((event: MouseEvent | TouchEvent) => {
    if (connectGhostListenerRef.current) {
      document.removeEventListener('mousemove', connectGhostListenerRef.current)
      connectGhostListenerRef.current = null
    }
    setConnectGhostPos(null)
    if (!canEdit || isReconnectingRef.current) return
    const sourceId = connectingSourceRef.current
    connectingSourceRef.current = null
    if (!sourceId || connectWasValidRef.current) { connectWasValidRef.current = false; return }
    connectWasValidRef.current = false
    const target = event.target
    if (target instanceof globalThis.Element) {
      if (target.closest('.react-flow__handle') || target.closest('.react-flow__node')) return
    }
    const { clientX, clientY } = 'changedTouches' in event
      ? (event as TouchEvent).changedTouches[0]
      : (event as MouseEvent)
    const flowPos = screenToFlowPositionRef.current({ x: clientX, y: clientY })
    const nearNode = rfNodesRef.current.find((node) => {
      if (node.id === sourceId) return false
      const cx = node.position.x + (node.width ?? 180) / 2
      const cy = node.position.y + (node.height ?? 80) / 2
      return Math.hypot(flowPos.x - cx, flowPos.y - cy) < SNAP_RADIUS
    })
    const cid = viewIdRef.current
    if (cid === null) return
    const sourceElementId = parseNumericId(sourceId)
    if (sourceElementId === null) return
    if (nearNode) {
      const targetElementId = parseNumericId(nearNode.id)
      if (targetElementId === null) return
      const sourceNode = rfNodesRef.current.find((n) => n.id === sourceId)
      const { sourceHandle, targetHandle } = sourceNode
        ? findClosestHandles(sourceNode, nearNode)
        : { sourceHandle: 'right', targetHandle: 'left' }
      api.workspace.connectors.create(cid, {
        source_element_id: sourceElementId, target_element_id: targetElementId,
        source_handle: sourceHandle, target_handle: targetHandle, direction: 'forward',
      }).then((connector) => {
        const next = connectorToConnector(connector)
        upsertConnectorGraphSnapshot(next)
        upsertConnector(next)
      }).catch(() => { /* intentionally empty */ })
    } else {
      setPendingConnectionSource(sourceElementId)
      suppressNextPaneClickRef.current = true
      showAddingElementAt(clientX, clientY, true, 'connect', 'shiftKey' in event && event.shiftKey)
    }
  }, [canEdit, upsertConnector, showAddingElementAt, rfNodesRef, viewIdRef])

  // ── Reconnect ──────────────────────────────────────────────────────────────
  const performReconnect = useCallback(async (oldConnector: RFEdge, newConnection: Connection) => {
    if (!canEdit || viewId === null || !newConnection.source || !newConnection.target) return
    const edgeId = parseNumericId(oldConnector.id)
    const sourceId = parseNumericId(newConnection.source)
    const targetId = parseNumericId(newConnection.target)
    if (edgeId === null || sourceId === null || targetId === null) return
    setRfEdges((eds) => reconnectEdge(oldConnector, newConnection, eds))
    setSelectedEdgeId(null)
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
    } catch { /* intentionally empty */ }
  }, [canEdit, replaceConnector, viewId, setRfEdges, setSelectedEdgeId])
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

    setInteractionSourceId(null)
    setClickConnectMode(null)
    setClickConnectCursorPos(null)
    setConnectGhostPos(null)
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
  }, [canEdit, clearHandleReconnectListeners, performReconnect, rfNodesRef, _rfEdgesRef, syncHandleReconnectDrag])

  // ── Click-connect ghost cursor tracking ────────────────────────────────────
  useEffect(() => {
    if (!clickConnectMode) {
      setClickConnectCursorPos(null)
      setConnectGhostPos(null)
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
      setConnectGhostPos(hit.nearHandle ? null : { x: e.clientX, y: e.clientY })
      setClickConnectMode((prev) => {
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
    return () => { document.removeEventListener('mousemove', listener); setConnectGhostPos(null) }
  }, [clickConnectMode])

  useEffect(() => {
    if (interactionSourceId === null) setClickConnectMode(null)
  }, [interactionSourceId])

  useEffect(() => () => {
    stopHandleReconnectDrag()
  }, [stopHandleReconnectDrag])

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
      setSelectedEdgeId(null)
      closeConnectorPanel()
      setSelectedProxyConnectorDetails((rfConnector.data as { details?: import('../../../crossBranch/types').ProxyConnectorDetails }).details ?? null)
      openProxyConnectorPanel()
      return
    }
    const clickedId = parseNumericId(rfConnector.id)
    if (clickedId === null) return
    if (selectedEdgeId === clickedId) {
      const connector = connectors.find((e) => e.id === clickedId)
      if (connector) { setSelectedEdge(connector); openConnectorPanelRef.current() }
      setSelectedEdgeId(null)
    } else {
      setSelectedElement(null)
      closeElementPanel()
      setSelectedEdgeId(clickedId)
    }
  }, [closeConnectorPanel, closeElementPanel, connectors, openProxyConnectorPanel, selectedEdgeId, setSelectedEdge, setSelectedEdgeId, setSelectedElement, setSelectedProxyConnectorDetails])

  // ── Pane interactions ─────────────────────────────────────────────────────
  const onPaneClick = useCallback((e: React.MouseEvent) => {
    if (suppressNextPaneClickRef.current) { suppressNextPaneClickRef.current = false; return }
    reconnectPickingRef.current = null
    setReconnectPicking(null)
    setSelectedElement(null)
    setSelectedEdge(null)
    setSelectedEdgeId(null)
    setSelectedProxyConnectorDetails(null)
    setConnectorLongPressMenu(null)
    setCanvasMenu(null)
    closeElementPanel()
    closeConnectorPanel()
    closeProxyConnectorPanel()
    const sourceId = interactionSourceIdRef.current
    if (sourceId !== null) {
      const flowPos = screenToFlowPositionRef.current({ x: e.clientX, y: e.clientY })
      const nearNode = rfNodesRef.current.find((node) => {
        if (node.id === String(sourceId)) return false
        const cx = node.position.x + (node.width ?? 180) / 2
        const cy = node.position.y + (node.height ?? 80) / 2
        return Math.hypot(flowPos.x - cx, flowPos.y - cy) < SNAP_RADIUS
      })
      if (nearNode) {
        const targetId = parseNumericId(nearNode.id)
        if (targetId === null) return
        stableOnConnectTo(targetId)
      } else {
        setInteractionSourceId(null)
        setPendingConnectionSource(sourceId)
        showAddingElementAt(e.clientX, e.clientY, true, 'connect', e.shiftKey)
      }
      return
    }
    setInteractionSourceId(null)
    setPendingConnectionSource(null)
    setAddingElementAt(null)
  }, [stableOnConnectTo, showAddingElementAt, closeElementPanel, closeConnectorPanel, closeProxyConnectorPanel, rfNodesRef, interactionSourceIdRef, setSelectedElement, setSelectedEdge, setSelectedEdgeId, setSelectedProxyConnectorDetails])

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
    setAddingElementAt(null)
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
      if (e.key === 'Escape') { setInteractionSourceId(null); return }
      const key = e.key.toLowerCase()
      if (!['w', 'a', 's', 'd', 'c', 'e', 'backspace', 'delete', 'r'].includes(key)) return
      if (e.ctrlKey || e.altKey || e.metaKey) return
      if (key === 'c' && e.shiftKey) return

      if (key === 'backspace' || key === 'delete' || key === 'r') {
        if (!canEdit) return
        e.preventDefault()
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
        } else if (selectedEdgeId) {
          api.workspace.connectors.delete('', selectedEdgeId).then(() => {
            handleConnectorDeleted(selectedEdgeId)
            setSelectedEdgeId(null)
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
        if (!canEdit || !selectedElement) return
        const node = rfNodesRef.current.find((n) => n.id === String(selectedElement.id))
        if (!node) return
        const cursor = lastMousePosRef.current
        const flowCursor = cursor
          ? screenToFlowPositionRef.current({ x: cursor.clientX, y: cursor.clientY })
          : { x: node.position.x + (node.width ?? 180), y: node.position.y + (node.height ?? 80) / 2 }
        const { sourceHandle } = findClosestHandleToPoint(node, flowCursor.x, flowCursor.y)
        setClickConnectMode({
          sourceNodeId: node.id,
          sourceHandle: ensureVisualHandleId(sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? sourceHandle,
        })
        setInteractionSourceId(selectedElement.id)
        if (cursor) setClickConnectCursorPos({ x: cursor.clientX, y: cursor.clientY })
        return
      }
      const cid = viewIdRef.current
      if (!cid) return
      const incoming = incomingLinksRef.current
      const tree = treeDataRef.current
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
        } else {
          if (allSiblings.length < 2) return
          const idx = allSiblings.findIndex((n) => n.id === cid)
          if (idx === -1) return
          const next = key === 'd' ? (idx + 1) % allSiblings.length : (idx - 1 + allSiblings.length) % allSiblings.length
          nav(`/views/${allSiblings[next].id}`)
        }
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [canEdit, refreshGrid, selectedElement, selectedEdgeId, viewId, stableOnRemoveElement, handleConnectorDeleted, handleElementPermanentlyDeleted,  closeElementPanel, closeConnectorPanel, viewIdRef, incomingLinksRef, treeDataRef, navigateRef, rfNodesRef, viewElementsRef, setLinksMap, showAddingElementAt, setSelectedElement, setSelectedEdge, setSelectedEdgeId, containerRef, linksMapRef])

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
  }, [canEdit, viewId, existingElementIds, refreshElements, rfNodesRef, viewElementsRef, layers, handleUpdateTags])

  const onWheelCapture = useCallback((e: React.WheelEvent) => {
    if (touchStateRef.current.touches.size === 2) return
    if (e.deltaX !== 0) touchStateRef.current.lastMultiTouchWheelTime = Date.now()
    const isRecentMultiTouch = Date.now() - touchStateRef.current.lastMultiTouchWheelTime < 1000
    const isNotchedWheel = !e.ctrlKey && e.deltaX === 0 && Number.isInteger(e.deltaY) && Math.abs(e.deltaY) >= 20
    const isMouseWheel = e.deltaMode !== 0 || isNotchedWheel
    if (isMouseWheel && !isRecentMultiTouch) {
      e.stopPropagation()
      if (e.deltaY > 0) zoomOut()
      else zoomIn()
    }
  }, [zoomIn, zoomOut])

  return {
    // State
    canvasMenu,
    setCanvasMenu,
    addingElementAt,
    setAddingElementAt,
    connectGhostPos,
    clickConnectMode,
    clickConnectCursorPos,
    handleReconnectDrag,
    interactionSourceId,
    setInteractionSourceId,
    pendingConnectionSource,
    setPendingConnectionSource,
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
    stableOnConnectTo,
    stableOnStartHandleReconnect,
    showAddingElementAt,
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
    handleConfirmConnectExistingElement,
  }
}
