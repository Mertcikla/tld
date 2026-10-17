import { connectorAlreadyHasDraggedTags } from '../connectorTagDrop'
import type { WorkspaceGraphSnapshot } from '../../../crossBranch/types'
import type { CSSProperties } from 'react'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { MarkerType } from 'reactflow'
import { api } from '../../../api/client'
import { deriveCanvasTagState } from '../../../utils/canvasTags'
import type {
  ViewTreeNode,
  PlacedElement,
  LibraryElement,
  Connector,
  Tag,
} from '../../../types'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getLogicalHandleId,
  getVisualHandleIdForGroup,
  getVisualHandleSlot,
} from '../../../utils/edgeDistribution'
import { buildViewContentLinks, useStore } from '../../../store/useStore'
import {
  Z_CONNECTOR,
  Z_CONNECTOR_LABEL,
  Z_ELEMENT,
  Z_ELEMENT_INTERACTION_SOURCE,
  Z_ELEMENT_LAYER_HIGHLIGHT,
} from '../../../utils/zOrder'

interface ViewDataOptions {
  viewId: number | null
  interactionSourceId: number | null
  clickConnectMode: { sourceNodeId: string; sourceHandle: string; targetHandle?: string } | null
  isConnectorCreatePreviewActive?: boolean
  selectedConnector: Connector | null
  suppressSelectedConnectorHandleHighlight?: boolean
  activeTags: string[]
  draggedConnectorTags?: readonly string[] | null
  hiddenLayerTags: string[]
  hoveredLayerTags: string[] | null
  hoveredLayerColor: string | null
  tagColors: Record<string, Tag>
  workspaceSnapshot?: WorkspaceGraphSnapshot | null
  // Node-level callbacks (stable refs from parent)
  stableOnZoomIn: (elementId: number) => Promise<void>
  stableOnZoomOut: (elementId: number) => Promise<void>
  stableOnNavigateToView: (id: number) => void
  stableOnSelect: (obj: PlacedElement) => void
  stableOnOpenCodePreview: (elementId: number) => void
  stableOnInteractionStart: (elementId: number, options?: { sourceHandle?: string; clientX?: number; clientY?: number }) => void
  stableOnConnectTo: (targetElementId: number) => Promise<void>
  stableOnStartHandleReconnect: (args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number; pointerId: number }) => void
  stableOnRemoveElement: (elementId: number) => Promise<void>
  stableOnHoverZoom: (elementId: number, type: 'in' | 'out' | null) => void
  hoveredZoomRef: React.MutableRefObject<{ elementId: number | null; type: 'in' | 'out' | null } | null>
}

function alphaColor(color: string, opacity: number): string {
  if (opacity >= 1) return color
  return `color-mix(in srgb, ${color} ${Math.round(opacity * 100)}%, transparent)`
}

// Stable style refs so unchanged nodes keep identical style references across renders,
// letting structural-sharing fast-path bail out without rebuilding the node.
const HIDDEN_STYLE: CSSProperties = { opacity: 0.1, pointerEvents: 'none' }
const CONTEXT_STYLE: CSSProperties = { opacity: 0.55 }
const SOFT_FOCUS_STYLE: CSSProperties = { opacity: 0.2 }
const EMPTY_ARRAY: readonly never[] = Object.freeze([])
const EMPTY_NODE_CONNECTION_META = Object.freeze({
  key: '',
  connectedHandleIds: EMPTY_ARRAY as readonly string[],
  tagHiddenHandleIds: EMPTY_ARRAY as readonly string[],
  selectedHandleIds: EMPTY_ARRAY as readonly string[],
  reconnectCandidates: EMPTY_ARRAY as readonly NodeReconnectCandidate[],
  isConnectorHighlighted: false,
})

type NodeReconnectCandidate = {
  handleId: string
  edgeId: string
  endpoint: 'source' | 'target'
  selected: boolean
}

type NodeConnectionMeta = {
  key: string
  connectedHandleIds: readonly string[]
  tagHiddenHandleIds: readonly string[]
  selectedHandleIds: readonly string[]
  reconnectCandidates: readonly NodeReconnectCandidate[]
  isConnectorHighlighted: boolean
}

type ConnectorLayout = {
  connector: Connector
  sourceHandle: string
  targetHandle: string
  sourceGroupIndex: number
  sourceGroupCount: number
  targetGroupIndex: number
  targetGroupCount: number
  sourceHandleSide: string
  targetHandleSide: string
  sourceHandleSlot: number
  targetHandleSlot: number
}

function findViewInTree(nodes: ViewTreeNode[], viewId: number): ViewTreeNode | null {
  for (const node of nodes) {
    if (node.id === viewId) return node
    const found = findViewInTree(node.children ?? [], viewId)
    if (found) return found
  }
  return null
}

function buildConnectorLayouts(connectors: Connector[], elementMap: Map<number, PlacedElement>): ConnectorLayout[] {
  const filtered = connectors.filter((connector) =>
    elementMap.has(connector.source_element_id) && elementMap.has(connector.target_element_id),
  )

  const handleUsage: Record<string, { id: string; type: 'source' | 'target'; otherNodeCoord: number }[]> = {}
  filtered.forEach((connector) => {
    const srcNode = elementMap.get(connector.source_element_id)
    const tgtNode = elementMap.get(connector.target_element_id)
    if (!srcNode || !tgtNode) return

    const sourceSide = getLogicalHandleId(connector.source_handle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
    const targetSide = getLogicalHandleId(connector.target_handle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE

    const srcKey = `${connector.source_element_id}-${sourceSide}`
    handleUsage[srcKey] ??= []
    const srcCoord = (sourceSide === 'left' || sourceSide === 'right') ? (tgtNode.position_y ?? 0) : (tgtNode.position_x ?? 0)
    handleUsage[srcKey].push({ id: String(connector.id), type: 'source', otherNodeCoord: srcCoord })

    const tgtKey = `${connector.target_element_id}-${targetSide}`
    handleUsage[tgtKey] ??= []
    const tgtCoord = (targetSide === 'left' || targetSide === 'right') ? (srcNode.position_y ?? 0) : (srcNode.position_x ?? 0)
    handleUsage[tgtKey].push({ id: String(connector.id), type: 'target', otherNodeCoord: tgtCoord })
  })

  Object.values(handleUsage).forEach((usages) => {
    usages.sort((a, b) => a.otherNodeCoord - b.otherNodeCoord)
  })

  return filtered.map((connector) => {
    const edgeId = String(connector.id)
    const sourceSide = getLogicalHandleId(connector.source_handle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
    const targetSide = getLogicalHandleId(connector.target_handle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE
    const srcGroup = handleUsage[`${connector.source_element_id}-${sourceSide}`] ?? []
    const tgtGroup = handleUsage[`${connector.target_element_id}-${targetSide}`] ?? []
    const sourceGroupIndex = srcGroup.findIndex((usage) => usage.id === edgeId && usage.type === 'source')
    const targetGroupIndex = tgtGroup.findIndex((usage) => usage.id === edgeId && usage.type === 'target')
    const sourceGroupCount = Math.max(srcGroup.length, 1)
    const targetGroupCount = Math.max(tgtGroup.length, 1)
    const sourceHandleSlot = getVisualHandleSlot(sourceGroupIndex, sourceGroupCount)
    const targetHandleSlot = getVisualHandleSlot(targetGroupIndex, targetGroupCount)

    return {
      connector,
      sourceHandle: getVisualHandleIdForGroup(sourceSide, sourceGroupIndex, sourceGroupCount),
      targetHandle: getVisualHandleIdForGroup(targetSide, targetGroupIndex, targetGroupCount),
      sourceGroupIndex,
      sourceGroupCount: srcGroup.length,
      targetGroupIndex,
      targetGroupCount: tgtGroup.length,
      sourceHandleSide: sourceSide,
      targetHandleSide: targetSide,
      sourceHandleSlot,
      targetHandleSlot,
    }
  })
}

export function useViewData({
  viewId,
  interactionSourceId,
  clickConnectMode,
  isConnectorCreatePreviewActive = false,
  selectedConnector,
  suppressSelectedConnectorHandleHighlight = false,
  activeTags,
  draggedConnectorTags,
  hiddenLayerTags,
  hoveredLayerTags: hoveredLayerTagsInput,
  hoveredLayerColor,
  tagColors,
  workspaceSnapshot,
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
  const hoveredLayerTags = draggedConnectorTags?.length ? null : hoveredLayerTagsInput
  const selectedEdgeId = selectedConnector?.id ?? null
  const queryClient = useQueryClient()
  const view = useStore((state) => state.view)
  const setView = useStore((state) => state.setView)
  const viewElements = useStore((state) => state.viewElements)
  const setViewElements = useStore((state) => state.setViewElements)
  const connectors = useStore((state) => state.connectors)
  const setConnectors = useStore((state) => state.setConnectors)
  const rfNodes = useStore((state) => state.nodes)
  const setRfNodes = useStore((state) => state.setNodes)
  const rfEdges = useStore((state) => state.edges)
  const setRfEdges = useStore((state) => state.setEdges)
  const linksMap = useStore((state) => state.linksMap)
  const setLinksMap = useStore((state) => state.setLinksMap)
  const parentLinksMap = useStore((state) => state.parentLinksMap)
  const setParentLinksMap = useStore((state) => state.setParentLinksMap)
  const incomingLinks = useStore((state) => state.incomingLinks)
  const treeData = useStore((state) => state.treeData)
  const allElements = useStore((state) => state.allElements)
  const hydrateViewContent = useStore((state) => state.hydrateViewContent)
  const resetCanvas = useStore((state) => state.resetCanvas)
  const removeElementPlacement = useStore((state) => state.removeElementPlacement)
  const removeElementEverywhere = useStore((state) => state.removeElementEverywhere)
  const mergeSavedElement = useStore((state) => state.mergeSavedElement)

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
    if (viewId === null) return
    const tree = await queryClient.fetchQuery({
      queryKey: ['workspace', 'views', viewId, 'editor-tree'],
      queryFn: () => api.workspace.views.treeAround(viewId, { ancestorLevels: 2, descendantLevels: 2 }),
      staleTime: 0,
    }).catch(() => null)
    if (tree) {
      const links = buildViewContentLinks(tree, viewId, viewElementsRef.current)
      useStore.getState().setTreeData(tree)
      useStore.getState().setLinksMap(links.linksMap)
      useStore.getState().setParentLinksMap(links.parentLinksMap)
      useStore.getState().setIncomingLinks(links.incomingLinks)
    }
  }, [queryClient, viewId])

  const fetchViewEditorContent = useCallback(async (targetViewId: number) => {
    const [content, tree] = await Promise.all([
      api.workspace.views.content(targetViewId),
      api.workspace.views.treeAround(targetViewId, { ancestorLevels: 2, descendantLevels: 2 }),
    ])
    const viewElements = content.placements || []
    const connectors = content.connectors || []
    const view = content.view ?? findViewInTree(tree, targetViewId)
    if (!view) throw new Error('View not found')
    return {
      view,
      viewElements,
      connectors,
      treeData: tree,
      ...buildViewContentLinks(tree, targetViewId, viewElements),
    }
  }, [])

  // ── Fetch view content ──────────────────────────────────────────────────
  const viewContentQuery = useQuery({
    queryKey: ['workspace', 'views', viewId, 'editor-content'],
    enabled: viewId !== null,
    queryFn: async () => {
      if (viewId === null) throw new Error('Missing view id')
      return fetchViewEditorContent(viewId)
    },
  })

  useEffect(() => {
    if (viewId === null) return
    if (viewContentQuery.data) hydrateViewContent(viewContentQuery.data)
  }, [hydrateViewContent, viewContentQuery.data, viewId])

  useEffect(() => {
    if (viewContentQuery.isError) {
      console.error('DIAGRAM EDITOR LOAD ERROR:', viewContentQuery.error)
      setView(null)
    }
  }, [setView, viewContentQuery.error, viewContentQuery.isError])

  // ── Clear canvas on navigation ─────────────────────────────────────────────
  useEffect(() => {
    resetCanvas()
  }, [resetCanvas, viewId])

  // ── Refresh elements ────────────────────────────────────────────────────────
  const refreshElements = useCallback(async () => {
    if (viewId === null) return
    const fresh = await queryClient.fetchQuery({
      queryKey: ['workspace', 'views', viewId, 'editor-content'],
      queryFn: () => fetchViewEditorContent(viewId),
      staleTime: 0,
    }).catch(() => null)
    if (fresh) hydrateViewContent(fresh)
  }, [fetchViewEditorContent, hydrateViewContent, queryClient, viewId])

  // ── Element mutation helpers ───────────────────────────────────────────────
  const handleElementDeleted = useCallback((deletedId: number) => {
    removeElementPlacement(deletedId)
  }, [removeElementPlacement])

  const handleElementPermanentlyDeleted = useCallback((deletedId: number) => {
    removeElementEverywhere(deletedId)
  }, [removeElementEverywhere])

  const handleElementSaved = useCallback((saved: LibraryElement) => {
    mergeSavedElement(saved)
  }, [mergeSavedElement])

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

  // Stable-ref fallback parent links: flatten only when the underlying map changes so
  // nodes without their own parent link entry can still pass the data-equality fast path.
  const viewParentLinks = useMemo(
    () => Object.values(parentLinksMap).flat(),
    [parentLinksMap],
  )

  const parentViewId = useMemo(() => {
    const findInTreeById = (nodes: ViewTreeNode[], id: number): ViewTreeNode | null => {
      for (const node of nodes) {
        if (node.id === id) return node
        const found = findInTreeById(node.children, id)
        if (found) return found
      }
      return null
    }
    const currentView = findInTreeById(treeData, viewId || -1)
    return currentView?.parent_view_id
  }, [treeData, viewId])

  const elementMap = useMemo(() => {
    const next = new Map<number, PlacedElement>()
    for (const element of viewElements) next.set(element.element_id, element)
    return next
  }, [viewElements])

  const connectorLayouts = useMemo(
    () => buildConnectorLayouts(connectors, elementMap),
    [connectors, elementMap],
  )

  const tagGraph = useMemo(() => {
    const nodes = new Map(Object.entries(workspaceSnapshot?.placementsByElementId ?? {}).map(([id, placements]) => [
      Number(id), { id: Number(id), tags: placements[0]?.element.tags },
    ]))
    for (const node of viewElements) nodes.set(node.element_id, { id: node.element_id, tags: node.tags })
    const edges = [
      ...Object.entries(workspaceSnapshot?.connectorsByViewId ?? {}).flatMap(([id, edges]) => Number(id) === viewId ? [] : edges),
      ...connectors,
    ]
    return {
      nodes: [...nodes.values()],
      connectors: edges.map((edge) => ({ id: edge.id, source: edge.source_element_id, target: edge.target_element_id, tags: edge.tags })),
    }
  }, [viewElements, connectors, workspaceSnapshot, viewId])
  const tagState = useMemo(() => deriveCanvasTagState(tagGraph.nodes, tagGraph.connectors, activeTags, hiddenLayerTags), [tagGraph, activeTags, hiddenLayerTags])
  const hoverTagState = useMemo(() => deriveCanvasTagState(tagGraph.nodes, tagGraph.connectors, hoveredLayerTags ?? [], hiddenLayerTags), [tagGraph, hoveredLayerTags, hiddenLayerTags])

  const connectionMetaCacheRef = useRef<Map<number, NodeConnectionMeta>>(new Map())
  const nodeConnectionMetaByElementId = useMemo(() => {
    const drafts = new Map<number, {
      connected: Set<string>
      visible: Set<string>
      selected: Set<string>
      reconnect: NodeReconnectCandidate[]
      highlighted: boolean
    }>()

    const draftFor = (elementId: number) => {
      let draft = drafts.get(elementId)
      if (!draft) {
        draft = { connected: new Set(), visible: new Set(), selected: new Set(), reconnect: [], highlighted: false }
        drafts.set(elementId, draft)
      }
      return draft
    }

    const selectedId = selectedEdgeId === null ? null : String(selectedEdgeId)
    for (const layout of connectorLayouts) {
      const connector = layout.connector
      const edgeId = String(connector.id)
      const isSelected = selectedId === edgeId
      const sourceDraft = draftFor(connector.source_element_id)
      const targetDraft = draftFor(connector.target_element_id)

      sourceDraft.connected.add(layout.sourceHandle)
      targetDraft.connected.add(layout.targetHandle)
      // Keep all anchors mounted so tag dimming never changes connector routes.
      const isDimmedByTags = connectorAlreadyHasDraggedTags(connector, draggedConnectorTags) || !tagState.visibleConnectorIds.has(connector.id) ||
        (hoveredLayerTags !== null && !hoverTagState.visibleConnectorIds.has(connector.id))
      if (isDimmedByTags) continue
      sourceDraft.visible.add(layout.sourceHandle)
      targetDraft.visible.add(layout.targetHandle)
      sourceDraft.reconnect.push({ handleId: layout.sourceHandle, edgeId, endpoint: 'source', selected: isSelected })
      targetDraft.reconnect.push({ handleId: layout.targetHandle, edgeId, endpoint: 'target', selected: isSelected })

      if (isSelected) {
        if (!suppressSelectedConnectorHandleHighlight) {
          sourceDraft.selected.add(layout.sourceHandle)
          targetDraft.selected.add(layout.targetHandle)
        }
        sourceDraft.highlighted = true
        targetDraft.highlighted = true
      }
    }

    const prev = connectionMetaCacheRef.current
    const next = new Map<number, NodeConnectionMeta>()
    for (const [elementId, draft] of drafts) {
      const connectedHandleIds = Array.from(draft.connected).sort()
      const tagHiddenHandleIds = connectedHandleIds.filter((handleId) => !draft.visible.has(handleId))
      const selectedHandleIds = Array.from(draft.selected).sort()
      const reconnectCandidates = draft.reconnect.sort((left, right) => {
        if (left.handleId !== right.handleId) return left.handleId.localeCompare(right.handleId)
        if (left.selected !== right.selected) return left.selected ? -1 : 1
        return left.edgeId.localeCompare(right.edgeId)
      })
      const reconnectKey = reconnectCandidates
        .map((candidate) => `${candidate.handleId}:${candidate.edgeId}:${candidate.endpoint}:${candidate.selected ? 1 : 0}`)
        .join(',')
      const key = [
        connectedHandleIds.join('|'),
        tagHiddenHandleIds.join('|'),
        selectedHandleIds.join('|'),
        reconnectKey,
        draft.highlighted ? 1 : 0,
      ].join('::')
      const existing = prev.get(elementId)
      next.set(elementId, existing?.key === key
        ? existing
        : {
          key,
          connectedHandleIds,
          tagHiddenHandleIds,
          selectedHandleIds,
          reconnectCandidates,
          isConnectorHighlighted: draft.highlighted,
        })
    }
    connectionMetaCacheRef.current = next
    return next
  }, [connectorLayouts, selectedEdgeId, suppressSelectedConnectorHandleHighlight, tagState, hoverTagState, hoveredLayerTags, draggedConnectorTags])

  // ── Derive RF nodes ────────────────────────────────────────────────────────
  useEffect(() => {
    setRfNodes((prevNodes) => {

      const prevNodeMap = new Map(prevNodes.map((n) => [n.id, n]))
      const isClickConnectMode = clickConnectMode !== null
      const isCreateConnectMode = isConnectorCreatePreviewActive || isClickConnectMode || interactionSourceId !== null
      return viewElements.map((obj) => {
        const nodeId = String(obj.element_id)
        const existing = prevNodeMap.get(nodeId)
        const isInactive = !tagState.visibleNodeIds.has(obj.element_id)
        const isLayerHighlighted = hoveredLayerTags !== null && hoverTagState.matchedNodeIds.has(obj.element_id)
        const isSoftFocused = hoveredLayerTags !== null && !hoverTagState.visibleNodeIds.has(obj.element_id)
        const isContext = tagState.contextNodeIds.has(obj.element_id) || (hoveredLayerTags !== null && hoverTagState.contextNodeIds.has(obj.element_id))
        const newZIndex = isLayerHighlighted
          ? Z_ELEMENT_LAYER_HIGHLIGHT
          : interactionSourceId === obj.element_id
            ? Z_ELEMENT_INTERACTION_SOURCE
            : Z_ELEMENT
        const newStyle = isInactive
          ? HIDDEN_STYLE
          : isSoftFocused
            ? SOFT_FOCUS_STYLE
            : isContext ? CONTEXT_STYLE : undefined
        const layerHighlightColor = isLayerHighlighted ? (hoveredLayerColor ?? undefined) : undefined
        const position = existing?.dragging ? existing.position : { x: obj.position_x ?? 0, y: obj.position_y ?? 0 }
        const isZoomHovered = hoveredZoomRef.current?.elementId === obj.element_id ? hoveredZoomRef.current.type : null
        const links = linksMap[obj.element_id] || EMPTY_ARRAY
        const parentLinks = parentLinksMap[obj.element_id] || viewParentLinks
        const connectionMeta = nodeConnectionMetaByElementId.get(obj.element_id) ?? EMPTY_NODE_CONNECTION_META

        // Structural sharing: if every input that would produce the same output matches the
        // previous node, return the previous reference so React Flow skips this node's work.
        if (
          existing &&
          existing.style === newStyle &&
          existing.zIndex === newZIndex &&
          existing.position.x === position.x &&
          existing.position.y === position.y &&
          existing.data &&
          existing.data.element_id === obj.element_id &&
          existing.data.tags === obj.tags &&
          existing.data.name === obj.name &&
          existing.data.position_x === obj.position_x &&
          existing.data.position_y === obj.position_y &&
          existing.data.description === obj.description &&
          existing.data.kind === obj.kind &&
          existing.data.technology === obj.technology &&
          existing.data.url === obj.url &&
          existing.data.logo_url === obj.logo_url &&
          existing.data.repo === obj.repo &&
          existing.data.branch === obj.branch &&
          existing.data.file_path === obj.file_path &&
          existing.data.technology_connectors === obj.technology_connectors &&
          existing.data.links === links &&
          existing.data.parentLinks === parentLinks &&
          existing.data.parentViewId === parentViewId &&
          existing.data.interactionSourceId === interactionSourceId &&
          existing.data.isClickConnectMode === isClickConnectMode &&
          existing.data.isCreateConnectMode === isCreateConnectMode &&
          existing.data.tagColors === tagColors &&
          existing.data.layerHighlightColor === layerHighlightColor &&
          existing.data.forceShowTagPopup === isLayerHighlighted &&
          existing.data.isZoomHovered === isZoomHovered &&
          existing.data.connectedHandleIds === connectionMeta.connectedHandleIds &&
          existing.data.tagHiddenHandleIds === connectionMeta.tagHiddenHandleIds &&
          existing.data.selectedHandleIds === connectionMeta.selectedHandleIds &&
          existing.data.reconnectCandidates === connectionMeta.reconnectCandidates &&
          existing.data.isConnectorHighlighted === connectionMeta.isConnectorHighlighted
        ) {
          return existing
        }

        return {
          id: nodeId,
          type: 'elementNode',
          position,
          width: existing?.width,
          height: existing?.height,
          selected: existing?.selected,
          dragging: existing?.dragging,
          zIndex: newZIndex,
          style: newStyle,
          data: {
            ...obj,
            links,
            parentLinks,
            parentViewId,
            onZoomIn: stableOnZoomIn,
            onZoomOut: stableOnZoomOut,
            onNavigateToDiagram: stableOnNavigateToView,
            onSelect: stableOnSelect,
            onOpenCodePreview: stableOnOpenCodePreview,
            onInteractionStart: stableOnInteractionStart,
            onConnectTo: stableOnConnectTo,
            onStartHandleReconnect: stableOnStartHandleReconnect,
            onRemove: stableOnRemoveElement,
            onHoverZoom: stableOnHoverZoom,
            isZoomHovered,
            interactionSourceId,
            isClickConnectMode,
            isCreateConnectMode,
            tagColors,
            layerHighlightColor,
            forceShowTagPopup: isLayerHighlighted,
            connectedHandleIds: connectionMeta.connectedHandleIds,
            tagHiddenHandleIds: connectionMeta.tagHiddenHandleIds,
            selectedHandleIds: connectionMeta.selectedHandleIds,
            reconnectCandidates: connectionMeta.reconnectCandidates,
            isConnectorHighlighted: connectionMeta.isConnectorHighlighted,
          },
        }
      })
    })
  }, [
    viewElements, linksMap, parentLinksMap, viewParentLinks, parentViewId,
    interactionSourceId, clickConnectMode, isConnectorCreatePreviewActive,
    stableOnZoomIn, stableOnZoomOut, stableOnNavigateToView, stableOnSelect,
    stableOnInteractionStart, stableOnConnectTo, stableOnStartHandleReconnect, stableOnRemoveElement, stableOnHoverZoom,
    stableOnOpenCodePreview, hoveredZoomRef, activeTags, hiddenLayerTags, hoveredLayerTags, hoveredLayerColor, tagColors,
    nodeConnectionMetaByElementId, setRfNodes, tagState, hoverTagState,
  ])

  // ── Derive RF connectors ────────────────────────────────────────────────────────
  useEffect(() => {
    setRfEdges((prevConnectors) => {
      const prevEdgeMap = new Map(prevConnectors.map((e) => [e.id, e]))

      return connectorLayouts.map((layout) => {
        const e = layout.connector
        const edgeId = String(e.id)
        const existing = prevEdgeMap.get(edgeId)
        const dir = e.direction ?? 'forward'

        const isInactive = connectorAlreadyHasDraggedTags(e, draggedConnectorTags) || !tagState.visibleConnectorIds.has(e.id)
        const isSoftFocused = hoveredLayerTags !== null && !hoverTagState.visibleConnectorIds.has(e.id)
        const isTagHighlighted = !isInactive && hoveredLayerTags !== null && hoverTagState.matchedConnectorIds.has(e.id)
        const stroke = isTagHighlighted ? (hoveredLayerColor || 'var(--accent)') : 'var(--accent)'
        const strokeWidth = isTagHighlighted ? 3 : 2
        const edgeOpacity = isInactive ? 0.1 : isSoftFocused ? 0.2 : 0.8
        const markerOpacity = isInactive ? 0.1 : isSoftFocused ? 0.2 : 1
        const newZIndex = Z_CONNECTOR
        const pointerEvents = (isInactive || isSoftFocused) ? 'none' : 'auto'
        const labelBgOpacity = isInactive ? 0.1 : isSoftFocused ? 0.2 : 0.95

        // Structural sharing: when all user-visible outputs match prev exactly, reuse prev ref.
        // We match on the underlying connector ref plus every computed visibility/layout value.
        if (
          existing &&
          existing.data &&
          (existing.data as Connector & { __src?: unknown }).__src === e &&
          existing.sourceHandle === layout.sourceHandle &&
          existing.targetHandle === layout.targetHandle &&
          existing.zIndex === newZIndex &&
          (existing.style as CSSProperties | undefined)?.stroke === stroke &&
          (existing.style as CSSProperties | undefined)?.strokeWidth === strokeWidth &&
          (existing.style as CSSProperties | undefined)?.opacity === edgeOpacity &&
          (existing.style as CSSProperties | undefined)?.pointerEvents === pointerEvents &&
          (existing.labelStyle as CSSProperties | undefined)?.opacity === markerOpacity &&
          (existing.labelBgStyle as CSSProperties | undefined)?.fillOpacity === labelBgOpacity &&
          (existing.data as { sourceGroupIndex?: number }).sourceGroupIndex === layout.sourceGroupIndex &&
          (existing.data as { targetGroupIndex?: number }).targetGroupIndex === layout.targetGroupIndex &&
          (existing.data as { sourceGroupCount?: number }).sourceGroupCount === layout.sourceGroupCount &&
          (existing.data as { targetGroupCount?: number }).targetGroupCount === layout.targetGroupCount
        ) {
          return existing
        }

        const arrowMarker = { type: MarkerType.ArrowClosed, width: 14, height: 14 }

        return {
          id: edgeId,
          source: String(e.source_element_id),
          target: String(e.target_element_id),
          sourceHandle: layout.sourceHandle,
          targetHandle: layout.targetHandle,
          type: e.style === 'bezier' ? 'default' : (e.style || 'default'),
          label: e.label ?? '',
          data: {
            ...e,
            __src: e,
            sourceGroupIndex: layout.sourceGroupIndex,
            sourceGroupCount: layout.sourceGroupCount,
            targetGroupIndex: layout.targetGroupIndex,
            targetGroupCount: layout.targetGroupCount,
            sourceHandleSide: layout.sourceHandleSide,
            targetHandleSide: layout.targetHandleSide,
            sourceHandleSlot: layout.sourceHandleSlot,
            targetHandleSlot: layout.targetHandleSlot,
            labelZIndex: Z_CONNECTOR_LABEL,
          },

          style: { stroke, strokeWidth, opacity: edgeOpacity, pointerEvents },
          labelStyle: { fontSize: 11, fill: stroke, opacity: markerOpacity },
          labelBgStyle: { fill: 'var(--chakra-colors-gray-900)', fillOpacity: labelBgOpacity },
          markerEnd: (dir === 'forward' || dir === 'both') ? { ...arrowMarker, color: alphaColor(stroke, markerOpacity) } : undefined,
          markerStart: (dir === 'backward' || dir === 'both') ? { ...arrowMarker, color: alphaColor(stroke, markerOpacity) } : undefined,
          selected: existing?.selected,
          zIndex: newZIndex,
        }
      })
    })
  }, [connectorLayouts, activeTags, hiddenLayerTags, hoveredLayerTags, hoveredLayerColor, elementMap, setRfEdges, tagState, hoverTagState, draggedConnectorTags])

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
  }
}
