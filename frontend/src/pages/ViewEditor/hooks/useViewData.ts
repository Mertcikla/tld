import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { MarkerType, type Edge as RFEdge, type Node as RFNode } from 'reactflow'
import { api } from '../../../api/client'
import type {
  ViewTreeNode,
  PlacedElement,
  LibraryElement,
  Connector,
  IncomingViewConnector,
  ViewConnector,
  Tag,
} from '../../../types'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getLogicalHandleId,
  getVisualHandleIdForGroup,
  getVisualHandleSlot,
} from '../../../utils/edgeDistribution'

interface ViewDataOptions {
  viewId: number | null
  interactionSourceId: number | null
  clickConnectMode: { sourceNodeId: string; sourceHandle: string; targetHandle?: string } | null
  selectedEdgeId: number | null
  activeTags: string[]
  hiddenLayerTags: string[]
  hoveredLayerTags: string[] | null
  hoveredLayerColor: string | null
  tagColors: Record<string, Tag>
  // Node-level callbacks (stable refs from parent)
  stableOnZoomIn: (elementId: number) => Promise<void>
  stableOnZoomOut: (elementId: number) => Promise<void>
  stableOnNavigateToView: (id: number) => void
  stableOnSelect: (obj: PlacedElement) => void
  stableOnOpenCodePreview: (elementId: number) => void
  stableOnInteractionStart: (elementId: number) => void
  stableOnConnectTo: (targetElementId: number) => Promise<void>
  stableOnStartHandleReconnect: (args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number }) => void
  stableOnRemoveElement: (elementId: number) => Promise<void>
  stableOnHoverZoom: (elementId: number, type: 'in' | 'out' | null) => void
  hoveredZoomRef: React.MutableRefObject<{ elementId: number | null; type: 'in' | 'out' | null } | null>
}

function alphaColor(color: string, opacity: number): string {
  if (opacity >= 1) return color
  return `color-mix(in srgb, ${color} ${Math.round(opacity * 100)}%, transparent)`
}

export function useViewData({
  viewId,
  interactionSourceId,
  clickConnectMode,
  selectedEdgeId,
  activeTags,
  hiddenLayerTags,
  hoveredLayerTags,
  hoveredLayerColor,
  tagColors,
  stableOnZoomIn,
  stableOnZoomOut,
  stableOnNavigateToView,
  stableOnSelect,
  stableOnOpenCodePreview,
  stableOnInteractionStart,
  stableOnConnectTo,
  stableOnStartHandleReconnect,
  stableOnRemoveElement,
  stableOnHoverZoom,
  hoveredZoomRef,
}: ViewDataOptions) {
  const [view, setView] = useState<ViewTreeNode | null | undefined>(undefined)
  const [viewElements, setViewElements] = useState<PlacedElement[]>([])
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [rfNodes, setRfNodes] = useState<RFNode[]>([])
  const [rfEdges, setRfEdges] = useState<RFEdge[]>([])
  const [linksMap, setLinksMap] = useState<Record<number, ViewConnector[]>>({})
  const [parentLinksMap, setParentLinksMap] = useState<Record<number, ViewConnector[]>>({})
  const [incomingLinks, setIncomingLinks] = useState<IncomingViewConnector[]>([])
  const [treeData, setTreeData] = useState<ViewTreeNode[]>([])
  const [allElements, setAllElements] = useState<LibraryElement[]>([])
  const [libraryRefresh, setLibraryRefresh] = useState(0)

  // Mutable refs for stable callbacks
  const viewElementsRef = useRef(viewElements)
  viewElementsRef.current = viewElements
  const linksMapRef = useRef(linksMap)
  linksMapRef.current = linksMap
  const parentLinksMapRef = useRef(parentLinksMap)
  parentLinksMapRef.current = parentLinksMap
  const incomingLinksRef = useRef(incomingLinks)
  incomingLinksRef.current = incomingLinks
  const treeDataRef = useRef(treeData)
  treeDataRef.current = treeData
  const rfNodesRef = useRef(rfNodes)
  rfNodesRef.current = rfNodes
  const rfEdgesRef = useRef(rfEdges)
  rfEdgesRef.current = rfEdges
  const viewIdRef = useRef(viewId)
  viewIdRef.current = viewId

  // ── Fetch tree ─────────────────────────────────────────────────────────────
  const refreshGrid = useCallback(async () => {
    const tree = await api.workspace.views.tree().catch(() => null)
    if (tree) setTreeData(tree)
  }, [])

  // ── Fetch view content ──────────────────────────────────────────────────
  useEffect(() => {
    if (viewId === null) return
    let active = true

    const load = async () => {
      try {
        const [diag, content, tree] = await Promise.all([
          api.workspace.views.get(viewId),
          api.workspace.views.content(viewId),
          api.workspace.views.tree(),
        ])
        if (!active) return

        const safeObjs = content.placements || []
        const safeConnectors = content.connectors || []

        const linksObj: Record<number, ViewConnector[]> = {}
        const parentLinksObj: Record<number, ViewConnector[]> = {}

        // Helper: recursively find nodes in tree that are owned by elements on canvas (zoom-in)
        // OR the parent view of the current view (zoom-out)
        const findViewByOwner = (nodes: ViewTreeNode[], elementId: number): ViewTreeNode | null => {
          for (const node of nodes) {
            if (node.owner_element_id !== null && Number(node.owner_element_id) === Number(elementId)) return node
            const found = findViewByOwner(node.children, elementId)
            if (found) return found
          }
          return null
        }

        const findViewPath = (nodes: ViewTreeNode[], targetId: number, path: ViewTreeNode[] = []): ViewTreeNode[] | null => {
          for (const node of nodes) {
            if (node.id === targetId) return [...path, node]
            const found = findViewPath(node.children, targetId, [...path, node])
            if (found) return found
          }
          return null
        }

        const viewPath = findViewPath(tree, viewId)
        const parentView = viewPath && viewPath.length > 1 ? viewPath[viewPath.length - 2] : null
        const currentViewInTree = viewPath ? viewPath[viewPath.length - 1] : null

        const incoming: IncomingViewConnector[] = []
        if (parentView && currentViewInTree?.owner_element_id) {
          incoming.push({
            id: 0,
            element_id: currentViewInTree.owner_element_id,
            element_name: 'Parent', // Optional: could find name in parentView.placements
            from_view_id: parentView.id,
            from_view_name: parentView.name,
            to_view_id: viewId,
          })
        }

        for (const obj of safeObjs) {
          // Child Link: if there exists a view owned by this element
          const childView = findViewByOwner(tree, obj.element_id)
          if (childView) {
            linksObj[obj.element_id] = [{
              id: 0,
              element_id: obj.element_id,
              from_view_id: viewId,
              to_view_id: childView.id,
              to_view_name: childView.name,
              relation_type: 'child',
            }]
          }

          // Parent Link: all elements in a view can 'zoom out' to its structural parent
          if (parentView) {
            parentLinksObj[obj.element_id] = [{
              id: 0,
              element_id: obj.element_id,
              from_view_id: parentView.id, // we go TO the parentView, coming FROM the parentView context?
              to_view_id: parentView.id,
              to_view_name: parentView.name,
              relation_type: 'parent',
            }]
          }
        }

        setLinksMap(linksObj)
        setParentLinksMap(parentLinksObj)
        setConnectors(safeConnectors)
        setViewElements(safeObjs)
        setIncomingLinks(incoming)
        setView(diag)
        setTreeData(tree)
      } catch (err) {
        console.error('DIAGRAM EDITOR LOAD ERROR:', err)
        if (active) setView(null)
      }
    }

    load()
    return () => { active = false }
  }, [viewId])

  // ── Clear canvas on navigation ─────────────────────────────────────────────
  useEffect(() => {
    setRfNodes([])
    setRfEdges([])
  }, [viewId])

  // ── Keep all-org elements for inline adder ──────────────────────────────────
  useEffect(() => {
    api.elements.list().then(setAllElements).catch(() => { /* intentionally empty */ })
  }, [libraryRefresh])

  // ── Refresh elements ────────────────────────────────────────────────────────
  const refreshElements = useCallback(async () => {
    if (viewId === null) return
    const fresh = await api.workspace.views.content(viewId).catch(() => null)
    if (fresh) setViewElements(fresh.placements)
  }, [viewId])

  // ── CRDT-aware element mutation helpers ────────────────────────────────────
  const handleElementDeleted = useCallback((deletedId: number) => {
    setViewElements((prev) => prev.filter((o) => o.element_id !== deletedId))
  }, [])

  const handleElementPermanentlyDeleted = useCallback((deletedId: number) => {
    setViewElements((prev) => prev.filter((o) => o.element_id !== deletedId))
    setLibraryRefresh((n) => n + 1)
  }, [])

  const handleElementSaved = useCallback((saved: LibraryElement) => {
    setLibraryRefresh((n) => n + 1)
    setViewElements((prev) =>
      prev.map((o) =>
        o.element_id === saved.id
          ? {
            ...o,
            name: saved.name,
            description: saved.description,
            kind: saved.kind,
            technology: saved.technology,
            url: saved.url,
            logo_url: saved.logo_url,
            technology_connectors: saved.technology_connectors,
            tags: saved.tags,
          }
          : o,
      ),
    )
  }, [])

  // ── Stable element ID set ───────────────────────────────────────────────────
  const existingElementIdsRef = useRef<Set<number>>(new Set())
  const existingElementIds = useMemo(() => {
    const nextIds = new Set(viewElements.map((o) => o.element_id))
    const prevIds = existingElementIdsRef.current
    if (nextIds.size === prevIds.size) {
      let changed = false
      for (const id of nextIds) { if (!prevIds.has(id)) { changed = true; break } }
      if (!changed) return prevIds
    }
    existingElementIdsRef.current = nextIds
    return nextIds
  }, [viewElements])

  // ── Derive RF nodes ────────────────────────────────────────────────────────
  useEffect(() => {
    setRfNodes((prevNodes) => {
      // The structural parent of the current view is stored under the view's
      // owner element ID which differs from the IDs of elements placed inside
      // the view. Pre-compute it so every node in the view shows the correct
      // zoom-out state even when their own element_id has no direct parent link.
      const viewParentLinks = Object.values(parentLinksMap).flat()
      const findInTreeById = (nodes: ViewTreeNode[], id: number): ViewTreeNode | null => {
        for (const node of nodes) {
          if (node.id === id) return node
          const found = findInTreeById(node.children, id)
          if (found) return found
        }
        return null
      }
      const currentView = findInTreeById(treeData, viewId || -1)
      const parentViewId = currentView?.parent_view_id
      
      return viewElements.map((obj) => {
        const existing = prevNodes.find((n) => n.id === String(obj.element_id))
        const objTags = obj.tags || []
        const isHiddenByLayer = hiddenLayerTags.length > 0 && objTags.some((t) => hiddenLayerTags.includes(t))
        const isInactive = isHiddenByLayer || (activeTags.length > 0 && !objTags.some((t) => activeTags.includes(t)))
        const isLayerHighlighted = hoveredLayerTags !== null && objTags.some((t) => hoveredLayerTags.includes(t))
        const isSoftFocused = hoveredLayerTags !== null && !isLayerHighlighted

        return {
          id: String(obj.element_id),
          type: 'elementNode',
          position: existing?.dragging ? existing.position : { x: obj.position_x ?? 0, y: obj.position_y ?? 0 },
          width: existing?.width,
          height: existing?.height,
          selected: existing?.selected,
          dragging: existing?.dragging,
          zIndex: isLayerHighlighted ? 10 : interactionSourceId === obj.element_id ? 1000 : 0,
          style: isInactive
            ? { opacity: 0.1, pointerEvents: 'none' }
            : isSoftFocused
              ? { opacity: 0.2 }
              : undefined,
          data: {
            ...obj,
            links: linksMap[obj.element_id] || [],
            parentLinks: parentLinksMap[obj.element_id] || viewParentLinks,
            parentViewId,
            onZoomIn: stableOnZoomIn,
            onZoomOut: stableOnZoomOut,
            onNavigateToView: stableOnNavigateToView,
            onSelect: stableOnSelect,
            onOpenCodePreview: stableOnOpenCodePreview,
            onInteractionStart: stableOnInteractionStart,
            onConnectTo: stableOnConnectTo,
            onStartHandleReconnect: stableOnStartHandleReconnect,
            onRemove: stableOnRemoveElement,
            onHoverZoom: stableOnHoverZoom,
            isZoomHovered: hoveredZoomRef.current?.elementId === obj.element_id ? hoveredZoomRef.current.type : null,
            interactionSourceId,
            isClickConnectMode: clickConnectMode !== null,
            tagColors,
            layerHighlightColor: isLayerHighlighted ? (hoveredLayerColor ?? undefined) : undefined,
            forceShowTagPopup: isLayerHighlighted,
          },
        }
      })
    })
  }, [
    viewElements, viewId, linksMap, parentLinksMap, treeData,
    interactionSourceId, clickConnectMode,
    stableOnZoomIn, stableOnZoomOut, stableOnNavigateToView, stableOnSelect,
    stableOnInteractionStart, stableOnConnectTo, stableOnStartHandleReconnect, stableOnRemoveElement, stableOnHoverZoom,
    stableOnOpenCodePreview, hoveredZoomRef, activeTags, hiddenLayerTags, hoveredLayerTags, hoveredLayerColor, tagColors,
  ])

  // ── Derive RF connectors ────────────────────────────────────────────────────────
  useEffect(() => {
    const visibleElementIds = new Set(viewElements.map((element) => element.element_id))
    const filtered = connectors.filter((connector) => visibleElementIds.has(connector.source_element_id) && visibleElementIds.has(connector.target_element_id))

    const handleUsage: Record<string, { id: string; type: 'source' | 'target'; otherNodeCoord: number }[]> = {}
    filtered.forEach((c) => {
      const srcNode = viewElements.find((o) => o.element_id === c.source_element_id)
      const tgtNode = viewElements.find((o) => o.element_id === c.target_element_id)
      if (!srcNode || !tgtNode) return

      const sourceSide = getLogicalHandleId(c.source_handle, DEFAULT_SOURCE_HANDLE_SIDE)
      const targetSide = getLogicalHandleId(c.target_handle, DEFAULT_TARGET_HANDLE_SIDE)

      const srcKey = `${c.source_element_id}-${sourceSide}`
      handleUsage[srcKey] ??= []
      const srcCoord = (sourceSide === 'left' || sourceSide === 'right') ? (tgtNode.position_y ?? 0) : (tgtNode.position_x ?? 0)
      handleUsage[srcKey].push({ id: String(c.id), type: 'source', otherNodeCoord: srcCoord })

      const tgtKey = `${c.target_element_id}-${targetSide}`
      handleUsage[tgtKey] ??= []
      const tgtCoord = (targetSide === 'left' || targetSide === 'right') ? (srcNode.position_y ?? 0) : (srcNode.position_x ?? 0)
      handleUsage[tgtKey].push({ id: String(c.id), type: 'target', otherNodeCoord: tgtCoord })
    })

    Object.values(handleUsage).forEach((usages) => {
      usages.sort((a, b) => a.otherNodeCoord - b.otherNodeCoord)
    })


    setRfEdges((prevConnectors) =>
      filtered.map((e) => {
        const existing = prevConnectors.find((re) => re.id === String(e.id))
        const dir = e.direction ?? 'forward'
        const arrowMarker = { type: MarkerType.ArrowClosed, width: 14, height: 14 }

        const sourceObj = viewElements.find((o) => o.element_id === e.source_element_id)
        const targetObj = viewElements.find((o) => o.element_id === e.target_element_id)
        const srcTags = sourceObj?.tags || []
        const tgtTags = targetObj?.tags || []
        const isInactiveByLayer = hiddenLayerTags.length > 0 && (
          srcTags.some((t) => hiddenLayerTags.includes(t)) ||
          tgtTags.some((t) => hiddenLayerTags.includes(t))
        )
        const isInactiveByFilter = activeTags.length > 0 && (
          !srcTags.some((t) => activeTags.includes(t)) ||
          !tgtTags.some((t) => activeTags.includes(t))
        )
        const isInactive = isInactiveByLayer || isInactiveByFilter
        const isSoftFocused = hoveredLayerTags !== null && (
          !sourceObj?.tags?.some((t) => hoveredLayerTags.includes(t)) ||
          !targetObj?.tags?.some((t) => hoveredLayerTags.includes(t))
        )
        const edgeOpacity = isInactive ? 0.1 : isSoftFocused ? 0.2 : 0.8
        const markerOpacity = isInactive ? 0.1 : isSoftFocused ? 0.2 : 1
        const sourceSide = getLogicalHandleId(e.source_handle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
        const targetSide = getLogicalHandleId(e.target_handle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE

        const srcKey = `${e.source_element_id}-${sourceSide}`
        const tgtKey = `${e.target_element_id}-${targetSide}`
        const srcGroup = handleUsage[srcKey] ?? []
        const tgtGroup = handleUsage[tgtKey] ?? []
        const sourceGroupIndex = srcGroup.findIndex((u) => u.id === String(e.id) && u.type === 'source')
        const targetGroupIndex = tgtGroup.findIndex((u) => u.id === String(e.id) && u.type === 'target')
        const sourceHandleSlot = getVisualHandleSlot(sourceGroupIndex, Math.max(srcGroup.length, 1))
        const targetHandleSlot = getVisualHandleSlot(targetGroupIndex, Math.max(tgtGroup.length, 1))

        return {
          id: String(e.id),
          source: String(e.source_element_id),
          target: String(e.target_element_id),
          sourceHandle: getVisualHandleIdForGroup(sourceSide, sourceGroupIndex, Math.max(srcGroup.length, 1)),
          targetHandle: getVisualHandleIdForGroup(targetSide, targetGroupIndex, Math.max(tgtGroup.length, 1)),
          type: e.style === 'bezier' ? 'default' : (e.style || 'default'),
          label: e.label ?? '',
          data: {
            ...e,
            sourceGroupIndex,
            sourceGroupCount: srcGroup.length,
            targetGroupIndex,
            targetGroupCount: tgtGroup.length,
            sourceHandleSide: sourceSide,
            targetHandleSide: targetSide,
            sourceHandleSlot,
            targetHandleSlot,
          },

          style: { stroke: 'var(--accent)', strokeWidth: 2, opacity: edgeOpacity, pointerEvents: (isInactive || isSoftFocused) ? 'none' : 'auto' },
          labelStyle: { fontSize: 11, fill: 'var(--accent)', opacity: markerOpacity },
          labelBgStyle: { fill: 'var(--chakra-colors-gray-900)', fillOpacity: isInactive ? 0.1 : isSoftFocused ? 0.2 : 0.95 },
          markerEnd: (dir === 'forward' || dir === 'both') ? { ...arrowMarker, color: alphaColor('var(--accent)', markerOpacity) } : undefined,
          markerStart: (dir === 'backward' || dir === 'both') ? { ...arrowMarker, color: alphaColor('var(--accent)', markerOpacity) } : undefined,
          selected: existing?.selected,
          zIndex: selectedEdgeId !== null && existing?.id === String(selectedEdgeId) ? 1000 : 100,
        }
      }),
    )
  }, [connectors, selectedEdgeId, activeTags, hiddenLayerTags, hoveredLayerTags, viewElements])


  // ── Boost z-index of selected connector ────────────────────────────────────────
  useEffect(() => {
    setRfEdges((prev) =>
      prev.map((e) => ({ ...e, zIndex: selectedEdgeId !== null && e.id === String(selectedEdgeId) ? 1000 : 100 })),
    )
  }, [selectedEdgeId])

  return {
    // State
    view,
    setView,
    viewElements,
    setViewElements,
    connectors,
    setConnectors,
    rfNodes,
    setRfNodes,
    rfEdges,
    setRfEdges,
    linksMap,
    setLinksMap,
    parentLinksMap,
    setParentLinksMap,
    incomingLinks,
    treeData,
    allElements,
    libraryRefresh,
    setLibraryRefresh,
    existingElementIds,
    // Stable refs
    viewElementsRef,
    linksMapRef,
    parentLinksMapRef,
    incomingLinksRef,
    treeDataRef,
    rfNodesRef,
    rfEdgesRef,
    viewIdRef,
    // Actions
    refreshGrid,
    refreshElements,
    handleElementDeleted,
    handleElementPermanentlyDeleted,
    handleElementSaved,
    setAllElements,
  }
}
