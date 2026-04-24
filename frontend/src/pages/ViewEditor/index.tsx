import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CoreUISlots } from '../../slots'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { parseNumericId } from '../../utils/ids'
import { useSafeFitView } from '../../hooks/useSafeFitView'
import { SafeBackground } from '../../components/SafeBackground'
import ReactFlow, {
  BackgroundVariant,
  ConnectionMode,
  Controls,
  PanOnScrollMode,
  ReactFlowProvider,
  useReactFlow,
} from 'reactflow'
import type { Edge as RFEdge, EdgeMarker as RFEdgeMarker, Node as RFNode, NodeChange } from 'reactflow'
import 'reactflow/dist/style.css'
import { toPng, toSvg } from 'html-to-image'
import {
  Box,
  Button,
  Flex,
  IconButton,
  Spinner,
  Text,
  Tooltip,
  VStack,
  useBreakpointValue,
  useDisclosure,
  useToast,
} from '@chakra-ui/react'
import {
  NavigationIcon,
  LibraryIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
} from '../../components/Icons'
import { api } from '../../api/client'
import type {
  ViewTreeNode,
  PlacedElement,
  LibraryElement as WorkspaceElement,
  Connector,
  ViewConnector,
  Tag,
} from '../../types'
import ElementNode from '../../components/ElementNode'
import ElementPanel from '../../components/ElementPanel'
import CodePreviewPanel from '../../components/CodePreviewPanel'
import ConnectorPanel from '../../components/ConnectorPanel'
import ElementLibrary from '../../components/ElementLibrary'
import ViewExplorer from '../../components/ViewExplorer'
import { useSetHeader } from '../../components/HeaderContext'
import ViewPanel from '../../components/ViewPanel'
import InlineElementAdder from '../../components/InlineElementAdder'
import ExportModal, { type ExportOptions } from '../../components/ExportModal'
import ImportModal from '../../components/ImportModal'
import ViewEditorOnboarding from '../../components/ViewEditorOnboarding'
import DrawingCanvas, { type DrawingCanvasHandle } from '../../components/DrawingCanvas'
import ViewFloatingMenu from '../../components/ViewFloatingMenu'
import ViewDrawMenu from '../../components/ViewDrawMenu'
import ViewHeaderButton from '../../components/ViewHeaderButton'
import ViewBezierConnector from '../../components/ViewBezierConnector'
import ViewContextNeighborElement from '../../components/ContextNeighborElement'
import ContextBoundaryElement from '../../components/ContextBoundaryElement'
import ContextStraightConnector from '../../components/ContextStraightConnector'
import ProxyConnectorEdge from '../../components/ProxyConnectorEdge'
import ProxyConnectorPanel from '../../components/ProxyConnectorPanel'
import { useViewContextNeighbours } from './hooks/useViewContextNeighbours'
import type { ParsedImport } from '../../pkg/importer/mermaid'
import { vscodeBridge } from '../../lib/vscodeBridge'
import type { ExtensionToWebviewMessage } from '../../types/vscode-messages'

import { ViewEditorContext } from './context'
import { useViewData } from './hooks/useViewData'
import { useDrawingEngine } from './hooks/useDrawingEngine'
import { applyNodeChangesWithStructuralSharing, useCanvasInteractions } from './hooks/useCanvasInteractions'
import { connectorToConnector, findClosestHandles, sanitizeExportFilename, triggerDownload } from './utils'
import { pickUnusedColor } from '../../components/ViewExplorer/utils'

import { EmptyCanvasState } from './components/EmptyCanvasState'
import { EditorOverlays } from './components/EditorOverlays'
import { ConnectorContextMenu, CanvasContextMenu } from './components/EditorMenus'
import { overrideViewContentInSnapshot } from '../../crossBranch/graph'
import { useCrossBranchContextSettings } from '../../crossBranch/settings'
import { removeConnectorGraphSnapshot, upsertConnectorGraphSnapshot, useWorkspaceGraphSnapshot } from '../../crossBranch/store'
import type { ProxyConnectorDetails } from '../../crossBranch/types'
import { useDemoRevealViewport, type ViewEditorDemoOptions } from '../../demo/viewEditor'

const nodeTypes = {
  elementNode: ElementNode,
  contextNeighborNode: ViewContextNeighborElement,
  ContextBoundaryElement: ContextBoundaryElement,
}
const edgeTypes = { default: ViewBezierConnector, contextStraightConnector: ContextStraightConnector, proxyConnectorEdge: ProxyConnectorEdge }
const EMPTY_LINKS: ViewConnector[] = []
const VIEW_EDITOR_MIN_ZOOM_FLOOR = 0.12
const VIEW_EDITOR_EMPTY_EXTENT_RATIO = 0.75
const VIEW_EDITOR_PAN_MARGIN_RATIO = 0.25
const VIEW_EDITOR_PAN_MARGIN_MIN = 180
const VIEW_EDITOR_PAN_MARGIN_MAX = 720
const SNAP_GRID: [number, number] = [30, 30]

function alphaColor(color: string, opacity: number): string {
  if (opacity >= 1) return color
  return `color-mix(in srgb, ${color} ${Math.round(opacity * 100)}%, transparent)`
}

function fadeMarker(marker: string | RFEdgeMarker | undefined, opacity: number) {
  if (!marker || typeof marker === 'string') return marker
  return {
    ...marker,
    color: alphaColor(marker.color ?? 'var(--accent)', opacity),
  }
}

function areTranslateExtentsEqual(
  left: [[number, number], [number, number]] | undefined,
  right: [[number, number], [number, number]] | undefined,
) {
  if (left === right) return true
  if (!left || !right) return !left && !right

  return left[0][0] === right[0][0] &&
    left[0][1] === right[0][1] &&
    left[1][0] === right[1][0] &&
    left[1][1] === right[1][1]
}



// ─────────────────────────────────────────────────────────────────────────────

interface Props extends CoreUISlots {
  demoOptions?: ViewEditorDemoOptions
}

function ViewEditorInner({
  demoOptions,
  canvasOverlaySlot,
  toolbarSlot,
  shareSlot,
  elementPanelAfterContentSlot,
  connectorPanelAfterContentSlot,
  rightSlot: _rightSlot,
  mobileMenuSlot: _mobileMenuSlot,
  userControlsSlot: _userControlsSlot,
}: Props) {
  const { id: viewIdParam } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const viewId = parseNumericId(viewIdParam)
  const navigate = useNavigate()
  const navigateRef = useRef(navigate)
  navigateRef.current = navigate

  const toast = useToast()
  const canEdit = true
  const isOwner = true
  const isFreePlan = false

  const setHeader = useSetHeader()
  const isMobileLayout = useBreakpointValue({ base: true, md: false }) ?? false

  const elementPanel = useDisclosure()
  const connectorPanel = useDisclosure()
  const proxyConnectorPanel = useDisclosure()
  const viewDetails = useDisclosure()
  const exportModal = useDisclosure()
  const importModal = useDisclosure()
  const codePreview = useDisclosure()

  // ── Stable disclosure refs ──────────────────────────────────────────────
  const openElementPanelRef = useRef(elementPanel.onOpen)
  openElementPanelRef.current = elementPanel.onOpen
  const closeElementPanelRef = useRef(elementPanel.onClose)
  closeElementPanelRef.current = elementPanel.onClose

  const openConnectorPanelRef = useRef(connectorPanel.onOpen)
  openConnectorPanelRef.current = connectorPanel.onOpen
  const closeConnectorPanelRef = useRef(connectorPanel.onClose)
  closeConnectorPanelRef.current = connectorPanel.onClose

  const openProxyConnectorPanelRef = useRef(proxyConnectorPanel.onOpen)
  openProxyConnectorPanelRef.current = proxyConnectorPanel.onOpen
  const closeProxyConnectorPanelRef = useRef(proxyConnectorPanel.onClose)
  closeProxyConnectorPanelRef.current = proxyConnectorPanel.onClose

  const openViewDetailsRef = useRef(viewDetails.onOpen)
  openViewDetailsRef.current = viewDetails.onOpen

  const openCodePreviewRef = useRef(codePreview.onOpen)
  openCodePreviewRef.current = codePreview.onOpen

  const openExportModalRef = useRef(exportModal.onOpen)
  openExportModalRef.current = exportModal.onOpen
  const closeExportModalRef = useRef(exportModal.onClose)
  closeExportModalRef.current = exportModal.onClose

  const openImportModalRef = useRef(importModal.onOpen)
  openImportModalRef.current = importModal.onOpen
  const closeImportModalRef = useRef(importModal.onClose)
  closeImportModalRef.current = importModal.onClose


  const [selectedElement, setSelectedElement] = useState<WorkspaceElement | null>(null)
  const [selectedEdge, setSelectedEdge] = useState<Connector | null>(null)
  const [selectedEdgeId, setSelectedEdgeId] = useState<number | null>(null)
  const [selectedProxyConnectorDetails, setSelectedProxyConnectorDetails] = useState<ProxyConnectorDetails | null>(null)
  const [previewElement, setPreviewElement] = useState<PlacedElement | null>(null)
  const [libraryOpen, setLibraryOpen] = useState(() => {
    if (typeof window === 'undefined') return false
    const stored = localStorage.getItem('diag:libraryOpen')
    return stored !== null ? stored === 'true' : window.innerWidth >= 768
  })
  const [isExplorerOpen, setIsExplorerOpen] = useState(() => {
    if (typeof window === 'undefined') return false
    const stored = localStorage.getItem('diag:explorerOpen')
    return stored !== null ? stored === 'true' : window.innerWidth >= 768
  })

  useEffect(() => { localStorage.setItem('diag:libraryOpen', String(libraryOpen)) }, [libraryOpen])
  useEffect(() => { localStorage.setItem('diag:explorerOpen', String(isExplorerOpen)) }, [isExplorerOpen])
  const [extrasOpen, setExtrasOpen] = useState(false)
  const [isImporting, setIsImporting] = useState(false)
  const [isExporting, setIsExporting] = useState(false)
  const [snapToGrid, setSnapToGrid] = useState(() => {
    if (typeof window === 'undefined') return false
    const stored = localStorage.getItem('diag:snapToGrid')
    return stored === 'true'
  })
  useEffect(() => { localStorage.setItem('diag:snapToGrid', String(snapToGrid)) }, [snapToGrid])
  const [, setHoveredZoom] = useState<{ elementId: number | null; type: 'in' | 'out' | null } | null>(null)
  const hoveredZoomRef = useRef<{ elementId: number | null; type: 'in' | 'out' | null } | null>(null)
  const hoverPanLockedUntilRef = useRef(0)

  const [activeTags, setActiveTags] = useState<string[]>([])
  const activeTagsRef = useRef<string[]>([])
  activeTagsRef.current = activeTags
  const [tagColors, setTagColors] = useState<Record<string, Tag>>({})

  useEffect(() => {
    // api.workspace.orgs.tagColors.list().then(setTagColors).catch(() => { /* skip */ })
  }, [])

  const [layers, setLayers] = useState<import('../../types').ViewLayer[]>([])
  const [hiddenLayerTags, setHiddenLayerTags] = useState<string[]>([])
  const hiddenLayerTagsRef = useRef<string[]>([])
  hiddenLayerTagsRef.current = hiddenLayerTags
  const [hoveredLayerTags, setHoveredLayerTags] = useState<string[] | null>(null)
  const [hoveredLayerColor, setHoveredLayerColor] = useState<string | null>(null)
  const handleHoverLayer = useCallback((tags: string[] | null, color?: string | null) => {
    setHoveredLayerTags(tags)
    setHoveredLayerColor(tags ? (color ?? null) : null)
  }, [])

  useEffect(() => {
    if (viewId === null) return
    api.workspace.views.layers.list(viewId).then(setLayers).catch(() => { /* skip */ })
  }, [viewId])

  const handleCreateLayer = useCallback(async (name: string, tags: string[], color: string) => {
    if (viewId === null) return
    try {
      const layer = await api.workspace.views.layers.create(viewId, { name, tags, color })
      setLayers(prev => [...prev, layer])
    } catch (e) {
      toast({ status: 'error', title: 'Failed to create layer', description: String(e) })
    }
  }, [viewId, toast])

  const handleCreateTag = useCallback(async (tag: string, color?: string, description?: string) => {
    const name = tag.trim()
    if (!name) return

    const nextColor = color ?? tagColors[name]?.color ?? pickUnusedColor(Object.values(tagColors).map(t => t.color))
    const nextDescription = description ?? tagColors[name]?.description ?? null

    setTagColors((prev) => ({ ...prev, [name]: { name, color: nextColor, description: nextDescription } }))
  }, [tagColors])

  const handleUpdateLayer = useCallback(async (layer: import('../../types').ViewLayer) => {
    if (viewId === null) return
    try {
      const updated = await api.workspace.views.layers.update(viewId, layer.id, layer)
      setLayers(prev => prev.map(l => l.id === updated.id ? updated : l))
    } catch (e) {
      toast({ status: 'error', title: 'Failed to update layer', description: String(e) })
    }
  }, [viewId, toast])

  const handleDeleteLayer = useCallback(async (layerId: number) => {
    if (viewId === null) return
    try {
      await api.workspace.views.layers.delete(viewId, layerId)
      setLayers(prev => prev.filter(l => l.id !== layerId))
    } catch (e) {
      toast({ status: 'error', title: 'Failed to delete layer', description: String(e) })
    }
  }, [viewId, toast])

  const containerRef = useRef<HTMLDivElement | null>(null)
  const drawingCanvasRef = useRef<DrawingCanvasHandle | null>(null)

  const { safeFitView } = useSafeFitView(containerRef)
  const { screenToFlowPosition, fitView, setViewport } = useReactFlow()
  const screenToFlowPositionRef = useRef(screenToFlowPosition)
  screenToFlowPositionRef.current = screenToFlowPosition
  const needsFitView = useRef(true)
  const rfReadyRef = useRef(false)
  const interactionSourceIdRef = useRef<number | null>(null)

  const nodeTypesMemo = useMemo(() => nodeTypes, [])
  const edgeTypesMemo = useMemo(() => edgeTypes, [])
  const workspaceGraphSnapshot = useWorkspaceGraphSnapshot(true)
  const { settings: crossBranchSettings, setEnabled: setCrossBranchEnabled } = useCrossBranchContextSettings('editor')

  const previewViewElementsRef = useRef<PlacedElement[]>([])

  const handleSetActiveTags = useCallback((tags: string[]) => {
    setActiveTags(tags)
  }, [])

  const handleSetHiddenLayerTags = useCallback((tags: string[]) => {
    setHiddenLayerTags(tags)
  }, [])

  // stableOnConnectTo is wired after canvasInteractions is declared
  const stableOnConnectToRef = useRef<(targetElementId: number) => Promise<void>>(async () => { })
  const stableOnStartHandleReconnectRef = useRef<(args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number }) => void>(() => { })

  // ── Drawing engine ────────────────────────────────────────────────────────
  const drawing = useDrawingEngine(viewId)
  const {
    drawingMode, setDrawingMode, drawingVisible, setDrawingVisible,
    drawingPaths, setDrawingPaths: _setDrawingPaths, drawingTool, setDrawingTool,
    drawingColor, setDrawingColor, drawingWidth, setDrawingWidth,
    setTextEditorState, drawingHistoryRef, drawingRedoStackRef,
    handleUndo, handleRedo, onPathComplete, onPathDelete, onPathUpdate,
  } = drawing

  // ── Data ──────────────────────────────────────────────────────────────────
  const stableOnZoomInRef = useRef<(id: number) => Promise<void>>(async () => { })
  const stableOnZoomOutRef = useRef<(id: number) => Promise<void>>(async () => { })
  const stableOnNavigateToViewRef = useRef<(id: number) => void>(() => { })
  const stableOnRemoveElementRef = useRef<(id: number) => Promise<void>>(async () => { })

  const data = useViewData({
    viewId,
    interactionSourceId: interactionSourceIdRef.current,
    clickConnectMode: null, // wired after canvasInteractions
    selectedEdgeId,
    activeTags,
    hiddenLayerTags,
    hoveredLayerTags,
    hoveredLayerColor,
    tagColors,
    stableOnZoomIn: useCallback(async (id: number) => { await stableOnZoomInRef.current(id) }, []),
    stableOnZoomOut: useCallback(async (id: number) => { await stableOnZoomOutRef.current(id) }, []),
    stableOnNavigateToView: useCallback((id: number) => { stableOnNavigateToViewRef.current(id) }, []),
    stableOnSelect: useCallback((obj: PlacedElement) => {
      setSelectedEdge(null)
      setSelectedEdgeId(null)
      setSelectedProxyConnectorDetails(null)
      closeProxyConnectorPanelRef.current()
      closeConnectorPanelRef.current()
      setSelectedElement({
        id: obj.element_id, name: obj.name, description: obj.description, kind: obj.kind,
        technology: obj.technology, url: obj.url, logo_url: obj.logo_url,
        technology_connectors: obj.technology_connectors, tags: obj.tags, repo: obj.repo,
        branch: obj.branch, file_path: obj.file_path, language: obj.language,
        created_at: '', updated_at: '', has_view: false, view_label: null,
      })
      openElementPanelRef.current()
    }, []),
    stableOnOpenCodePreview: useCallback((elementId: number) => {
      const obj = previewViewElementsRef.current.find((o) => o.element_id === elementId)
      if (obj) {
        setPreviewElement(obj)
        openCodePreviewRef.current()
      }
    }, []),
    stableOnInteractionStart: useCallback((elementId: number) => {
      if (!canEdit) return
      interactionSourceIdRef.current = interactionSourceIdRef.current === elementId ? null : elementId
    }, [canEdit]),
    stableOnConnectTo: useCallback(async (targetElementId: number) => {
      await stableOnConnectToRef.current(targetElementId)
    }, []),
    stableOnStartHandleReconnect: useCallback((args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number }) => {
      stableOnStartHandleReconnectRef.current(args)
    }, []),
    stableOnRemoveElement: useCallback(async (id: number) => { await stableOnRemoveElementRef.current(id) }, []),
    stableOnHoverZoom: useCallback((elementId: number, type: 'in' | 'out' | null) => {
      const next = type ? { elementId, type } : null
      hoveredZoomRef.current = next
      setHoveredZoom(next)
    }, []),
    hoveredZoomRef,
  })

  const {
    view, setView, viewElements, setViewElements, connectors, setConnectors,
    rfNodes, setRfNodes, rfEdges, setRfEdges,
    linksMap, setLinksMap, parentLinksMap, setParentLinksMap,
    treeData, allElements, libraryRefresh,
    existingElementIds,
    viewElementsRef, linksMapRef, parentLinksMapRef, incomingLinksRef,
    treeDataRef, rfNodesRef, rfEdgesRef, viewIdRef,
    refreshGrid, refreshElements,
    handleElementDeleted, handleElementPermanentlyDeleted, handleElementSaved,
    setAllElements: _setAllElements,
  } = data

  const tagCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    viewElements.forEach(p => {
      (p.tags ?? []).forEach(t => { counts[t] = (counts[t] ?? 0) + 1 })
    })
    return counts
  }, [viewElements])

  const layerElementCounts = useMemo(() => {
    const counts: Record<number, number> = {}
    for (const layer of layers) {
      let count = 0
      viewElements.forEach(p => {
        if ((p.tags ?? []).some(t => layer.tags.includes(t))) count++
      })
      counts[layer.id] = count
    }
    return counts
  }, [viewElements, layers])

  const toggleLayerVisibility = useCallback((layer: import('../../types').ViewLayer) => {
    if (layer.tags.length === 0) return
    const prev = hiddenLayerTagsRef.current
    const allHidden = layer.tags.every(t => prev.includes(t))
    const next = allHidden
      ? prev.filter(t => !layer.tags.includes(t))
      : Array.from(new Set([...prev, ...layer.tags]))
    handleSetHiddenLayerTags(next)
  }, [handleSetHiddenLayerTags])

  const toggleTagVisibility = useCallback((tag: string) => {
    const prev = hiddenLayerTagsRef.current
    const next = prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag]
    handleSetHiddenLayerTags(next)
  }, [handleSetHiddenLayerTags])

  // ── VS Code Integration ───────────────────────────────────────────────────
  const hasSentLoadedRef = useRef<number | null>(null)
  useEffect(() => {
    if (view && viewElements.length > 0 && hasSentLoadedRef.current !== view.id) {
      hasSentLoadedRef.current = view.id
      vscodeBridge.postMessage({
        type: 'diagram-loaded',
        diagramId: view.id,
        elements: allElements,
      })
    }
  }, [view, viewElements, allElements])

  useEffect(() => {
    const unsub = vscodeBridge.onMessage(async (msg: ExtensionToWebviewMessage) => {
      if (msg.type === 'focus-element') {
        fitView({ nodes: [{ id: String(msg.elementId) }], duration: 800, padding: 100 })
      } else if (msg.type === 'element-placed') {
        if (viewId === null) return
        try {
          await api.workspace.views.placements.add(viewId, msg.elementId, msg.x, msg.y)
          void refreshElements()
        } catch (e) {
          console.error('Failed to place element from VS Code:', e)
        }
      }
    })
    return unsub
  }, [fitView, viewId, refreshElements])

  const existingElements = useMemo(() => {
    return viewElements.map(obj => ({
      id: obj.element_id,
      name: obj.name,
      kind: obj.kind,
      description: obj.description,
      technology: obj.technology,
      url: obj.url,
      logo_url: obj.logo_url,
      technology_connectors: obj.technology_connectors,
      tags: obj.tags,
      repo: obj.repo,
      branch: obj.branch,
      file_path: obj.file_path,
      language: obj.language,
      created_at: '',
      updated_at: '',
      has_view: false,
      view_label: null,
    } as WorkspaceElement))
  }, [viewElements])

  const availableTags = useMemo(() => {
    const tags = new Set<string>()
    allElements.forEach((o) => o.tags?.forEach((t: string) => tags.add(t)))
    Object.keys(tagColors).forEach((t) => tags.add(t))
    return Array.from(tags).sort((a, b) => a.localeCompare(b))
  }, [allElements, tagColors])

  const effectiveWorkspaceSnapshot = useMemo(() => {
    if (viewId == null) return workspaceGraphSnapshot
    return overrideViewContentInSnapshot(workspaceGraphSnapshot, viewId, viewElements, connectors)
  }, [workspaceGraphSnapshot, viewId, viewElements, connectors])

  const placementSummaryByElementId = useMemo(() => {
    const summary: Record<number, string> = {}
    if (!effectiveWorkspaceSnapshot) return summary
    for (const [elementId, placements] of Object.entries(effectiveWorkspaceSnapshot.placementsByElementId)) {
      const names = Array.from(new Set(placements.map((placement) => placement.viewName))).slice(0, 2)
      if (names.length > 0) {
        summary[Number(elementId)] = names.length > 1 ? `${names.join(' · ')}…` : names[0]
      }
    }
    return summary
  }, [effectiveWorkspaceSnapshot])

  useEffect(() => {
    const requestedElementId = parseNumericId(searchParams.get('element'))
    if (requestedElementId == null) return
    const match = viewElements.find((element) => element.element_id === requestedElementId)
    if (!match) return
    setSelectedEdge(null)
    setSelectedEdgeId(null)
    setSelectedProxyConnectorDetails(null)
    closeConnectorPanelRef.current()
    closeProxyConnectorPanelRef.current()
    setSelectedElement({
      id: match.element_id,
      name: match.name,
      description: match.description,
      kind: match.kind,
      technology: match.technology,
      url: match.url,
      logo_url: match.logo_url,
      technology_connectors: match.technology_connectors,
      tags: match.tags,
      repo: match.repo,
      branch: match.branch,
      file_path: match.file_path,
      language: match.language,
      created_at: '',
      updated_at: '',
      has_view: match.has_view,
      view_label: match.view_label,
    })
    openElementPanelRef.current()
    const next = new URLSearchParams(searchParams)
    next.delete('element')
    setSearchParams(next, { replace: true })
  }, [searchParams, setSearchParams, viewElements])

  previewViewElementsRef.current = viewElements

  const handleUpdateTags = useCallback(async (elementId: number, tags: string[]) => {
    if (!canEdit) return
    const obj = selectedElement?.id === elementId ? selectedElement : allElements.find(o => o.id === elementId)
    if (!obj) return
    try {
      const saved = await api.elements.update(elementId, {
        name: obj.name,
        description: obj.description ?? '',
        kind: obj.kind ?? '',
        technology: obj.technology ?? '',
        url: obj.url ?? '',
        logo_url: obj.logo_url ?? '',
        technology_connectors: obj.technology_connectors ?? [],
        tags,
        repo: obj.repo,
        branch: obj.branch,
        file_path: obj.file_path,
        language: obj.language,
      })
      handleElementSaved(saved)
      if (selectedElement?.id === elementId) {
        setSelectedElement(saved)
      }
    } catch (err) {
      console.error('Failed to update tags:', err)
    }
  }, [canEdit, selectedElement, allElements, handleElementSaved, setSelectedElement])

  // ── Canvas interactions ────────────────────────────────────────────────────
  const canvas = useCanvasInteractions({
    viewId, canEdit,
    drawingMode, isMobileLayout,
    rfNodesRef, rfEdgesRef, viewElementsRef, viewIdRef,
    incomingLinksRef,
    treeDataRef,
    navigateRef,
    containerRef,
    interactionSourceIdRef,
    hoveredZoomRef, hoverPanLockedUntilRef,
    setViewElements, setConnectors,
    setRfNodes, setRfEdges,
    setLinksMap, setParentLinksMap,
    setHoveredZoom,
    refreshGrid, refreshElements,
    stableOnConnectTo: async (targetElementId: number) => {
      // Inline this is the real implementation, also stored in stableOnConnectToRef
      const sourceId = interactionSourceIdRef.current
      const cid = viewIdRef.current
      if (sourceId === null || cid === null) return
      interactionSourceIdRef.current = null
      const sourceNode = rfNodesRef.current.find((n) => n.id === String(sourceId))
      const targetNode = rfNodesRef.current.find((n) => n.id === String(targetElementId))
      let finalSourceHandle = 'right'; let finalTargetHandle = 'left'
      if (sourceNode && targetNode) {
        const h = findClosestHandles(sourceNode, targetNode)
        finalSourceHandle = h.sourceHandle; finalTargetHandle = h.targetHandle
      }
      try {
        const newConnector = await api.workspace.connectors.create(cid, {
          source_element_id: sourceId, target_element_id: targetElementId,
          source_handle: finalSourceHandle, target_handle: finalTargetHandle, direction: 'forward',
        })
        const connector = connectorToConnector(newConnector)
        upsertConnectorGraphSnapshot(connector)
        setConnectors((prev) => [...prev, connector])
      } catch { /* intentionally empty */ }
    },
    existingElementIds, linksMapRef, parentLinksMapRef,
    openElementPanel: useCallback(() => openElementPanelRef.current(), []),
    closeElementPanel: useCallback(() => closeElementPanelRef.current(), []),
    openConnectorPanel: useCallback(() => openConnectorPanelRef.current(), []),
    closeConnectorPanel: useCallback(() => closeConnectorPanelRef.current(), []),
    selectedElement, selectedEdgeId, connectors,
    layers,
    setSelectedElement,
    setSelectedEdge, setSelectedEdgeId,
    setSelectedProxyConnectorDetails,
    openProxyConnectorPanel: useCallback(() => openProxyConnectorPanelRef.current(), []),
    closeProxyConnectorPanel: useCallback(() => closeProxyConnectorPanelRef.current(), []),
    handleElementDeleted, handleElementPermanentlyDeleted,
    handleConnectorDeleted: useCallback((edgeId: number) => {
      if (viewId != null) removeConnectorGraphSnapshot(viewId, edgeId)
      setConnectors((prev) => prev.filter((connector) => connector.id !== edgeId))
    }, [setConnectors, viewId]),
    handleUpdateTags,
    drawingCanvasRef,
    snapToGrid,
  })

  // Wire stable placeholders to the real implementations from canvas hook
  useEffect(() => {
    stableOnZoomInRef.current = canvas.stableOnZoomIn
    stableOnZoomOutRef.current = canvas.stableOnZoomOut
    stableOnNavigateToViewRef.current = canvas.stableOnNavigateToView
    stableOnRemoveElementRef.current = canvas.stableOnRemoveElement
    stableOnConnectToRef.current = canvas.stableOnConnectTo
    stableOnStartHandleReconnectRef.current = canvas.stableOnStartHandleReconnect
  }, [canvas.stableOnZoomIn, canvas.stableOnZoomOut, canvas.stableOnNavigateToView, canvas.stableOnRemoveElement, canvas.stableOnConnectTo, canvas.stableOnStartHandleReconnect])
  const viewName = view?.name ?? null

  const [expandedAncestorGroups, setExpandedAncestorGroups] = useState<Set<string>>(new Set())
  const stableOnToggleAncestorGroup = useCallback((anchorId: string) => {
    setExpandedAncestorGroups((prev) => {
      const next = new Set(prev)
      if (next.has(anchorId)) next.delete(anchorId)
      else next.add(anchorId)
      return next
    })
  }, [])

  const { contextNodes, contextConnectors } = useViewContextNeighbours({
    snapshot: effectiveWorkspaceSnapshot,
    settings: crossBranchSettings,
    viewId,
    viewElements,
    rfNodes,
    stableOnNavigateToView: canvas.stableOnNavigateToView,
    onSelectProxyDetails: useCallback((details: ProxyConnectorDetails) => {
      setSelectedElement(null)
      setSelectedEdge(null)
      setSelectedEdgeId(null)
      closeConnectorPanelRef.current()
      closeElementPanelRef.current()
      setSelectedProxyConnectorDetails(details)
      openProxyConnectorPanelRef.current()
    }, []),
    expandedAncestorGroups,
    onToggleAncestorGroup: stableOnToggleAncestorGroup,
  })

  // Keep context nodes in state so React Flow can store measured dimensions.
  // When computed positions change (e.g. main node drag), preserve the previously
  // measured width/height so nodes don't flash hidden while being re-measured.
  const [liveContextNodes, setLiveContextNodes] = useState<RFNode[]>([])
  const contextNodeIdsRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    contextNodeIdsRef.current = new Set(contextNodes.map((n) => n.id))
    setLiveContextNodes((prev) => {
      const prevById = new Map(prev.map((p) => [p.id, p]))
      return contextNodes.map((n) => {
        const existing = prevById.get(n.id)
        if (existing?.width != null && existing?.height != null) {
          return { ...n, width: existing.width, height: existing.height }
        }
        return n
      })
    })
  }, [contextNodes])

  const fadedNodeCacheRef = useRef<WeakMap<RFNode, RFNode>>(new WeakMap())
  const fadedEdgeCacheRef = useRef<WeakMap<RFEdge, RFEdge>>(new WeakMap())

  const flowNodes = useMemo(() => {
    const allNodes = liveContextNodes.length === 0
      ? rfNodes
      : rfNodes.length === 0
        ? liveContextNodes
        : [...liveContextNodes, ...rfNodes]

    let hasNodeSel = false
    const selectedNodeIds = new Set<string>()
    for (const n of allNodes) {
      if (n.selected) { selectedNodeIds.add(n.id); hasNodeSel = true }
    }

    const allEdges = contextConnectors.length === 0
      ? rfEdges
      : rfEdges.length === 0
        ? contextConnectors
        : [...contextConnectors, ...rfEdges]

    const selectedEdgeEndPoints = new Set<string>()
    let hasEdgeSel = false
    for (const e of allEdges) {
      if (e.selected) {
        selectedEdgeEndPoints.add(e.source)
        selectedEdgeEndPoints.add(e.target)
        hasEdgeSel = true
      }
    }

    const neighborNodeIds = new Set<string>()
    if (hasNodeSel) {
      for (const e of allEdges) {
        if (selectedNodeIds.has(e.source)) neighborNodeIds.add(e.target)
        if (selectedNodeIds.has(e.target)) neighborNodeIds.add(e.source)
      }
    }

    if (!hasNodeSel && !hasEdgeSel) return allNodes

    const cache = fadedNodeCacheRef.current
    return allNodes.map((n) => {
      const isHighlighted = selectedNodeIds.has(n.id) || selectedEdgeEndPoints.has(n.id) || neighborNodeIds.has(n.id)
      if (isHighlighted) return n
      const cached = cache.get(n)
      if (cached) return cached
      const faded: RFNode = {
        ...n,
        style: { ...n.style, opacity: (Number(n.style?.opacity ?? 1)) * 0.2 },
      }
      cache.set(n, faded)
      return faded
    })
  }, [liveContextNodes, rfNodes, contextConnectors, rfEdges])

  const flowEdges = useMemo(() => {
    const allEdges = contextConnectors.length === 0
      ? rfEdges
      : rfEdges.length === 0
        ? contextConnectors
        : [...contextConnectors, ...rfEdges]
    const allNodes = liveContextNodes.length === 0
      ? rfNodes
      : rfNodes.length === 0
        ? liveContextNodes
        : [...liveContextNodes, ...rfNodes]

    const selectedNodeIds = new Set<string>()
    let hasNodeSel = false
    for (const n of allNodes) {
      if (n.selected) { selectedNodeIds.add(n.id); hasNodeSel = true }
    }
    let hasEdgeSel = false
    for (const e of allEdges) { if (e.selected) { hasEdgeSel = true; break } }

    if (!hasNodeSel && !hasEdgeSel) return allEdges

    const cache = fadedEdgeCacheRef.current
    return allEdges.map((e) => {
      const isHighlighted = e.selected || selectedNodeIds.has(e.source) || selectedNodeIds.has(e.target)
      if (isHighlighted) return e
      const cached = cache.get(e)
      if (cached) return cached
      const multiplier = 0.2
      const faded: RFEdge = {
        ...e,
        style: { ...e.style, opacity: (Number(e.style?.opacity ?? 0.8)) * multiplier },
        labelStyle: e.labelStyle ? { ...e.labelStyle, opacity: (Number(e.labelStyle.opacity ?? 1)) * multiplier } : undefined,
        labelBgStyle: e.labelBgStyle ? { ...e.labelBgStyle, fillOpacity: (Number(e.labelBgStyle.fillOpacity ?? 0.95)) * multiplier } : undefined,
        markerEnd: fadeMarker(e.markerEnd, multiplier),
        markerStart: fadeMarker(e.markerStart, multiplier),
      }
      cache.set(e, faded)
      return faded
    })
  }, [contextConnectors, rfEdges, liveContextNodes, rfNodes])

  // Route onNodesChange: context node changes (dimensions, selection) go to
  // liveContextNodes state; main node changes go to the canvas handler.
  const { onNodesChange: canvasOnNodesChange } = canvas
  const onNodesChange = useCallback((changes: NodeChange[]) => {
    const ctxChanges = changes.filter((c) => 'id' in c && contextNodeIdsRef.current.has((c as { id: string }).id))
    const mainChanges = changes.filter((c) => !('id' in c) || !contextNodeIdsRef.current.has((c as { id: string }).id))
    if (ctxChanges.length > 0) {
      setLiveContextNodes((nds) => applyNodeChangesWithStructuralSharing(ctxChanges, nds))
    }
    if (mainChanges.length > 0) {
      canvasOnNodesChange(mainChanges)
    }
  }, [canvasOnNodesChange])

  const {
    canvasMenu, setCanvasMenu,
    addingElementAt, setAddingElementAt,
    connectGhostPos, clickConnectMode, clickConnectCursorPos,
    setPendingConnectionSource,
    reconnectPicking, setReconnectPicking, reconnectPickingRef,
    connectorLongPressMenu, setConnectorLongPressMenu,
    lastMousePosRef,
    showAddingElementAt,
    onEdgesChange, onNodeDragStart, onNodeDrag, onNodeDragStop,
    onConnect, onConnectStart, onConnectEnd,
    onReconnect, onReconnectStart, onReconnectEnd,
    onEdgeClick, onEdgeContextMenu, onPaneClick, onPaneContextMenu, onPaneMouseMove,
    onMoveStart, onMove, onMoveEnd,
    onTouchStart, onTouchMove, onTouchEnd,
    onContainerPointerDown, onContainerPointerMove, onContainerPointerUp,
    onDragOver, onDrop, onWheelCapture,
    handleConfirmNewElement, handleConfirmExistingElement, handleConfirmConnectExistingElement,
  } = canvas

  // ── FitView ────────────────────────────────────────────────────────────────
  const fitViewRef = useRef(safeFitView)
  fitViewRef.current = safeFitView
  const [computedMinZoom, setComputedMinZoom] = useState(VIEW_EDITOR_MIN_ZOOM_FLOOR)
  const [computedTranslateExtent, setComputedTranslateExtent] = useState<[[number, number], [number, number]] | undefined>(undefined)
  const {
    clampedRevealProgress,
    applyDemoRevealViewport,
    disableImportExport,
    hideFlowControls,
  } = useDemoRevealViewport({
    demoOptions,
    containerRef,
    rfNodesRef,
    rfReadyRef,
    needsFitViewRef: needsFitView,
    computedMinZoom,
    setViewport,
    resetKey: viewId,
  })

  const maybeFitView = useCallback(() => {
    if (!rfReadyRef.current || !needsFitView.current) return
    const nodes = rfNodesRef.current
    if (nodes.length === 0) return
    if (!nodes.every((n) => typeof n.width === 'number' && n.width > 0 && typeof n.height === 'number' && n.height > 0)) return

    if (clampedRevealProgress !== null) {
      const ok = applyDemoRevealViewport()
      if (ok && clampedRevealProgress >= 0.999) needsFitView.current = false
      else if (!ok) setTimeout(() => { if (needsFitView.current) maybeFitView() }, 50)
      return
    }

    const ok = safeFitView({ duration: 0 })
    if (ok) needsFitView.current = false
    else setTimeout(() => { if (needsFitView.current) maybeFitView() }, 50)
  }, [applyDemoRevealViewport, clampedRevealProgress, safeFitView, rfNodesRef])

  const onRFInit = useCallback(() => { rfReadyRef.current = true; maybeFitView() }, [maybeFitView])

  useEffect(() => { maybeFitView() }, [rfNodes, maybeFitView])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver(() => { if (needsFitView.current) maybeFitView() })
    observer.observe(el)
    return () => observer.disconnect()
  }, [maybeFitView])

  useEffect(() => { setRfNodes([]); setRfEdges([]); needsFitView.current = true }, [viewId, setRfEdges, setRfNodes])

  // ── Dynamic viewport bounds ────────────────────────────────────────────────
  useEffect(() => {
    const vw = window.innerWidth; const vh = window.innerHeight
    const emptyExtent: [[number, number], [number, number]] = [
      [-vw * VIEW_EDITOR_EMPTY_EXTENT_RATIO, -vh * VIEW_EDITOR_EMPTY_EXTENT_RATIO],
      [vw * VIEW_EDITOR_EMPTY_EXTENT_RATIO, vh * VIEW_EDITOR_EMPTY_EXTENT_RATIO],
    ]
    const boundsNodes = liveContextNodes.length === 0
      ? rfNodes
      : rfNodes.length === 0
        ? liveContextNodes
        : [...liveContextNodes, ...rfNodes]
    if (boundsNodes.length === 0 && drawingPaths.length === 0) {
      setComputedMinZoom((prev) => prev === VIEW_EDITOR_MIN_ZOOM_FLOOR ? prev : VIEW_EDITOR_MIN_ZOOM_FLOOR)
      setComputedTranslateExtent((prev) => areTranslateExtentsEqual(prev, emptyExtent) ? prev : emptyExtent)
      return
    }
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const n of boundsNodes) {
      minX = Math.min(minX, n.position.x); minY = Math.min(minY, n.position.y)
      maxX = Math.max(maxX, n.position.x + (n.width ?? 180)); maxY = Math.max(maxY, n.position.y + (n.height ?? 80))
    }
    for (const p of drawingPaths) {
      for (const pt of p.points) { minX = Math.min(minX, pt.x); minY = Math.min(minY, pt.y); maxX = Math.max(maxX, pt.x); maxY = Math.max(maxY, pt.y) }
    }
    if (!isFinite(minX)) {
      setComputedMinZoom((prev) => prev === VIEW_EDITOR_MIN_ZOOM_FLOOR ? prev : VIEW_EDITOR_MIN_ZOOM_FLOOR)
      setComputedTranslateExtent((prev) => areTranslateExtentsEqual(prev, emptyExtent) ? prev : emptyExtent)
      return
    }
    const bboxW = maxX - minX; const bboxH = maxY - minY
    let minZoom = Math.sqrt((0.12 * vw * vh) / Math.max(1, bboxW * bboxH))
    if (!isFinite(minZoom) || isNaN(minZoom) || minZoom <= 0) minZoom = VIEW_EDITOR_MIN_ZOOM_FLOOR
    const nextMinZoom = Math.max(VIEW_EDITOR_MIN_ZOOM_FLOOR, Math.min(minZoom, 1))
    setComputedMinZoom((prev) => prev === nextMinZoom ? prev : nextMinZoom)
    // Extent must be ≥ viewport at minZoom (else pan locks). Center on content bbox.
    // Keep only modest content-proportional slack so the canvas stays discoverable.
    const vwFlowMax = vw / nextMinZoom; const vhFlowMax = vh / nextMinZoom
    const slackX = Math.min(Math.max(bboxW * VIEW_EDITOR_PAN_MARGIN_RATIO, VIEW_EDITOR_PAN_MARGIN_MIN), VIEW_EDITOR_PAN_MARGIN_MAX)
    const slackY = Math.min(Math.max(bboxH * VIEW_EDITOR_PAN_MARGIN_RATIO, VIEW_EDITOR_PAN_MARGIN_MIN), VIEW_EDITOR_PAN_MARGIN_MAX)
    const spanX = Math.max(bboxW + 2 * slackX, vwFlowMax)
    const spanY = Math.max(bboxH + 2 * slackY, vhFlowMax)
    const cx = (minX + maxX) / 2; const cy = (minY + maxY) / 2
    const nextTranslateExtent: [[number, number], [number, number]] = [[cx - spanX / 2, cy - spanY / 2], [cx + spanX / 2, cy + spanY / 2]]
    setComputedTranslateExtent((prev) => areTranslateExtentsEqual(prev, nextTranslateExtent) ? prev : nextTranslateExtent)
  }, [rfNodes, liveContextNodes, drawingPaths])

  // ── Keyboard shortcuts for drawing ────────────────────────────────────────
  useEffect(() => {
    if (!drawingMode) return
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable) return

      const key = e.key.toLowerCase()
      const isCmd = e.metaKey || e.ctrlKey

      if (isCmd && key === 'z') {
        e.preventDefault()
        if (e.shiftKey) handleRedo()
        else handleUndo()
        return
      }

      if (key === 'p') { setDrawingTool('pencil'); return }
      if (key === 'e') { setDrawingTool('eraser'); return }
      if (key === 't') { setDrawingTool('text'); return }
      if (key === 'v') { setDrawingTool('select'); return }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [drawingMode, handleUndo, handleRedo, setDrawingTool])

  // ── Overscroll prevention ──────────────────────────────────────────────────
  useEffect(() => {
    const html = document.documentElement
    const prev = html.style.overscrollBehaviorX
    html.style.overscrollBehaviorX = 'none'
    return () => { html.style.overscrollBehaviorX = prev }
  }, [])

  // ── Header ─────────────────────────────────────────────────────────────────
  useEffect(() => {
    setHeader({
      node: <ViewHeaderButton name={viewName ?? undefined} onOpen={openViewDetailsRef.current} />,
    })
  }, [viewName, setHeader])

  useEffect(() => () => setHeader(null), [setHeader])

  // ── Share ──────────────────────────────────────────────────────────────────
  const onShare = useCallback(() => {}, [])

  const handleExplorerHoverZoom = useCallback((elementId: number | null, type: 'in' | 'out' | null) => {
    setHoveredZoom(type && elementId ? { elementId, type } : null)
  }, [])
  const handleToggleExplorer = useCallback(() => setIsExplorerOpen((v) => !v), [])
  const handleCloseLibrary = useCallback(() => setLibraryOpen(false), [])
  const handleCreateNewLibraryRef = useRef<() => void>(() => {})
  const handleCreateNewLibrary = useCallback(() => handleCreateNewLibraryRef.current(), [])
  const handleFocusModeChange = useCallback((v: boolean) => setCrossBranchEnabled(!v), [setCrossBranchEnabled])
  const handleOpenExport = useCallback(() => exportModal.onOpen(), [exportModal])
  const handleConnectorSave = useCallback((updated: Connector) => {
    upsertConnectorGraphSnapshot(updated)
    setConnectors((prev) => prev.map((c) => (c.id === updated.id ? updated : c)))
  }, [setConnectors])
  const handleConnectorDeleteInPanel = useCallback((edgeId: number) => {
    if (viewId != null) removeConnectorGraphSnapshot(viewId, edgeId)
    setConnectors((prev) => prev.filter((c) => c.id !== edgeId))
  }, [setConnectors, viewId])
  const handleViewSave = useCallback((updated: ViewTreeNode) => setView(updated), [setView])

  // ── Library helpers ────────────────────────────────────────────────────────
  // Assigned below; referenced by memoized callbacks (e.g. ElementLibrary onCreateNew).
  const handleAddElementAtCenter = useCallback((forceCenter = false) => {
    if (!canEdit) return
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    let cx = rect.left + rect.width / 2; let cy = rect.top + rect.height * 0.4
    if (!forceCenter && lastMousePosRef.current) {
      const { clientX, clientY } = lastMousePosRef.current
      if (clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom) {
        cx = clientX; cy = clientY
      }
    }
    showAddingElementAt(cx, cy, true)
  }, [canEdit, showAddingElementAt, lastMousePosRef])
  handleCreateNewLibraryRef.current = () => handleAddElementAtCenter(true)

  const handleTapAdd = useCallback(async (obj: WorkspaceElement) => {
    if (!canEdit || !viewId || existingElementIds.has(obj.id)) return
    const pos = screenToFlowPositionRef.current({ x: window.innerWidth / 2, y: window.innerHeight / 2 })
    try { await api.workspace.views.placements.add(viewId, obj.id, pos.x - 100, pos.y - 40); await refreshElements() } catch { /* intentionally empty */ }
  }, [canEdit, viewId, existingElementIds, refreshElements])

  const handleTouchDrop = useCallback(async (obj: WorkspaceElement, clientX: number, clientY: number) => {
    if (!canEdit || !viewId || existingElementIds.has(obj.id)) return
    const container = containerRef.current; if (!container) return
    const bounds = container.getBoundingClientRect()
    if (clientX < bounds.left || clientX > bounds.right || clientY < bounds.top || clientY > bounds.bottom) return
    const pos = screenToFlowPositionRef.current({ x: clientX, y: clientY })
    try { await api.workspace.views.placements.add(viewId, obj.id, pos.x - 100, pos.y - 40); await refreshElements() } catch { /* intentionally empty */ }
  }, [canEdit, viewId, existingElementIds, refreshElements])

  const handleFindElement = useCallback((elementId: number) => {
    const node = rfNodesRef.current.find((n) => (n.data as PlacedElement).element_id === elementId)
    if (node) {
      fitViewRef.current({ nodes: [node], duration: 800, padding: 0.8 })
    }
  }, [rfNodesRef])

  // ── Export / Import ────────────────────────────────────────────────────────
  const handleExportView = useCallback(async (options: ExportOptions) => {
    const flowRoot = containerRef.current?.querySelector('.react-flow') as HTMLElement | null
    if (!flowRoot) { toast({ status: 'error', title: 'Export failed', description: 'Could not find the view canvas.' }); return }
    const baseName = sanitizeExportFilename(options.filename || viewName || 'view-export')
    const downloadName = `${baseName}.${options.format}`
    const filterNode = (node: HTMLElement) => {
      const cn = node.className
      if (typeof cn !== 'string') return true
      return !cn.includes('react-flow__controls') && !cn.includes('react-flow__panel')
    }
    try {
      setIsExporting(true)
      if (options.format === 'mermaid') {
        let code = 'architecture-beta\n'
        for (const obj of viewElements) {
          const safeId = `obj_${obj.element_id}`
          const shape = obj.kind === 'database' ? 'database' : obj.kind === 'person' ? 'person' : 'server'
          code += `  service ${safeId}(${shape})[${obj.name}]\n`
        }
        code += '\n'
        for (const connector of connectors) { code += `  obj_${connector.source_element_id}:R -- L:obj_${connector.target_element_id}\n` }
        triggerDownload(URL.createObjectURL(new Blob([code], { type: 'text/plain;charset=utf-8' })), downloadName)
      } else if (options.format === 'svg') {
        triggerDownload(await toSvg(flowRoot, { cacheBust: true, filter: filterNode }), downloadName)
      } else {
        triggerDownload(await toPng(flowRoot, { cacheBust: true, pixelRatio: options.scale, filter: filterNode }), downloadName)
      }
      closeExportModalRef.current()
      toast({ status: 'success', title: 'Export complete', description: `Saved ${downloadName}` })
    } catch {
      toast({ status: 'error', title: 'Export failed', description: 'Please try again.' })
    } finally { setIsExporting(false) }
  }, [viewName, viewElements, connectors, toast])

  const handleImportView = useCallback(async (parsed: ParsedImport) => {
    const currentViewId = viewIdRef.current
    if (!currentViewId) return
    setIsImporting(true)
    try {
      const res = await api.import.resources('', { elements: parsed.elements, connectors: parsed.connectors })
      closeImportModalRef.current()
      toast({ status: 'success', title: 'Import complete', description: `Created ${parsed.elements.length} elements and ${parsed.connectors.length} connectors.`, duration: 5000, isClosable: true })
      if (res.view_id && res.view_id !== currentViewId) navigate(`/views/${res.view_id}`)
      else window.location.reload()
    } catch (e) {
      toast({ status: 'error', title: 'Import failed', description: e instanceof Error ? e.message : 'Unknown error' })
    } finally { setIsImporting(false) }
  }, [navigate, toast, viewIdRef])

  // ─────────────────────────────────────────────────────────────────────────────
  // Render states
  // ─────────────────────────────────────────────────────────────────────────────
  if (view === undefined) {
    return <Flex h={{ base: '100vh', sm: 'calc(100vh - var(--editor-top-offset, 48px))' }} align="center" justify="center"><Spinner size="xl" /></Flex>
  }
  if (view === null) {
    return <Flex h={{ base: '100vh', sm: 'calc(100vh - var(--editor-top-offset, 48px))' }} align="center" justify="center"><Text>View not found.</Text></Flex>
  }

  return (
    <ViewEditorContext.Provider value={{
      viewId, canEdit, isOwner, isFreePlan, snapToGrid, setSnapToGrid,
      selectedElement, selectedConnector: selectedEdge
    }}>
      <Box h={{ base: '100vh', sm: 'calc(100vh - var(--editor-top-offset, 48px))' }} display="flex" flexDir="column">
        <Flex flex={1} overflow="hidden">
          <Box
            ref={containerRef}
            flex={1}
            position="relative"
            onDrop={onDrop}
            onDragOver={onDragOver}
            onPointerDown={onContainerPointerDown}
            onPointerMove={onContainerPointerMove}
            onPointerUp={onContainerPointerUp}
            onPointerCancel={onContainerPointerUp}
            sx={{ overscrollBehaviorX: 'none' }}
          >
            {/* Library toggle */}
            {!isMobileLayout && (
              <Tooltip label={libraryOpen ? 'Close element library' : 'Open element library'} placement="right" openDelay={300}>
                <IconButton
                  aria-label={libraryOpen ? 'Close element library' : 'Open element library'}
                  icon={libraryOpen ? <ChevronLeftIcon size={16} strokeWidth={3.5} /> : <ChevronRightIcon size={16} strokeWidth={3.5} />}
                  size="md" position="absolute" top="50%"
                  left={libraryOpen ? '328px' : 3}
                  transition="left 0.2s cubic-bezier(0.25, 0.46, 0.45, 0.94), transform 0.15s ease"
                  zIndex={1200} border='1px solid rgba(255, 255, 255, 0.08)'
                  variant="clay" colorScheme="gray" bg="var(--bg-panel)"
                  color={libraryOpen ? 'white' : 'gray.300'}
                  _hover={{ bg: 'var(--bg-card-solid)', transform: 'translateY(-50%) scale(1.1)', color: 'white' }}
                  onClick={() => setLibraryOpen((v) => !v)}
                  transform="translateY(-50%)"
                />
              </Tooltip>
            )}

            {/* Explorer toggle */}
            {!isMobileLayout && !elementPanel.isOpen && !connectorPanel.isOpen && !viewDetails.isOpen && (
              <Tooltip label={isExplorerOpen ? 'Close view explorer' : 'Open view explorer'} placement="left" openDelay={300}>
                <IconButton
                  aria-label={isExplorerOpen ? 'Close view explorer' : 'Open view explorer'}
                  icon={isExplorerOpen ? <ChevronRightIcon size={16} strokeWidth={3.5} /> : <ChevronLeftIcon size={16} strokeWidth={3.5} />}
                  size="md" position="absolute" top="50%"
                  right={isExplorerOpen ? '328px' : 3}
                  transition="right 0.2s cubic-bezier(0.25, 0.46, 0.45, 0.94), transform 0.15s ease"
                  zIndex={5} border="1px solid rgba(255, 255, 255, 0.08)"
                  variant="clay" colorScheme="gray" bg="var(--bg-panel)"
                  color={isExplorerOpen ? 'white' : 'gray.300'}
                  _hover={{ bg: 'var(--bg-card-solid)', transform: 'translateY(-50%) scale(1.1)', color: 'white' }}
                  onClick={() => setIsExplorerOpen((v) => !v)}
                  transform="translateY(-50%)"
                />
              </Tooltip>
            )}

            {/* Mobile toggles */}
            {isMobileLayout && !isExplorerOpen && !libraryOpen && !elementPanel.isOpen && !connectorPanel.isOpen && !viewDetails.isOpen && (
              <VStack position="absolute" left={3} top="50%" transform="translateY(-50%)" spacing={2} zIndex={5}>
                <IconButton aria-label="Open view navigation" icon={<NavigationIcon />}
                  size="md" variant="clay" colorScheme="gray" bg="var(--bg-panel)" color="gray.300"
                  border="1px solid rgba(255,255,255,0.08)"
                  _hover={{ bg: 'var(--bg-card-solid)', transform: 'scale(1.1)', color: 'white' }}
                  transition="all 0.15s ease"
                  onClick={() => { setIsExplorerOpen(true); setLibraryOpen(false) }}
                />
                <IconButton aria-label="Open element library" icon={<LibraryIcon />}
                  size="md" variant="clay" colorScheme="gray" bg="var(--bg-panel)" color="gray.300"
                  border="1px solid rgba(255,255,255,0.08)"
                  _hover={{ bg: 'var(--bg-card-solid)', transform: 'scale(1.1)', color: 'white' }}
                  transition="all 0.15s ease"
                  onClick={() => { setLibraryOpen(true); setIsExplorerOpen(false) }}
                />
              </VStack>
            )}

            <Box
              position="relative"
              w="full"
              h="full"
              onWheelCapture={onWheelCapture}
              onTouchStart={onTouchStart}
              onTouchMove={onTouchMove}
              onTouchEnd={onTouchEnd}
              sx={{
                '.react-flow__edgelabel-renderer': {
                  zIndex: 1002,
                },
              }}
            >
              <ReactFlow
                nodes={flowNodes} edges={flowEdges}
                onInit={onRFInit}
                onNodesChange={onNodesChange} onEdgesChange={onEdgesChange}
                onConnect={onConnect} onConnectStart={onConnectStart} onConnectEnd={onConnectEnd}
                onNodeDragStart={onNodeDragStart} onNodeDrag={onNodeDrag} onNodeDragStop={onNodeDragStop}
                onEdgeClick={onEdgeClick} onEdgeContextMenu={onEdgeContextMenu}
                onPaneContextMenu={onPaneContextMenu} onPaneClick={onPaneClick}
                onPaneMouseMove={onPaneMouseMove}
                onMoveStart={onMoveStart} onMove={onMove} onMoveEnd={onMoveEnd}
                translateExtent={computedTranslateExtent} nodeExtent={computedTranslateExtent} minZoom={computedMinZoom} maxZoom={4}
                onReconnect={onReconnect} onReconnectStart={onReconnectStart} onReconnectEnd={onReconnectEnd}
                nodeTypes={nodeTypesMemo} edgeTypes={edgeTypesMemo}
                nodesDraggable={canEdit} connectionMode={ConnectionMode.Loose} connectionRadius={25}
                edgesUpdatable={canEdit} reconnectRadius={0}
                snapToGrid={snapToGrid}
                snapGrid={SNAP_GRID}
                deleteKeyCode={null}
                onlyRenderVisibleElements
                autoPanOnNodeDrag={false}
                panOnDrag={!drawingMode}
                panOnScroll={!isMobileLayout} panOnScrollSpeed={1.2} panOnScrollMode={PanOnScrollMode.Free}
                zoomOnScroll={false} zoomOnPinch
              >
                <SafeBackground variant={BackgroundVariant.Dots} gap={16} color="#2D3748" size={1} />
                {!hideFlowControls && (
                  <Controls position="bottom-right" className="glass" style={{ overflow: 'hidden', margin: '1rem' }} />
                )}
              </ReactFlow>
              {canvasOverlaySlot && (
                <Box position="absolute" inset={0} pointerEvents="none" zIndex={10}>
                  {canvasOverlaySlot}
                </Box>
              )}
            </Box>

            <ViewExplorer
              treeNodes={treeData}
              linksMap={linksMap} viewElements={viewElements}
              onNavigate={canvas.stableOnNavigateToView}
              onHoverZoom={handleExplorerHoverZoom}
              isOpen={isExplorerOpen} onToggle={handleToggleExplorer}
              isMobile={isMobileLayout}
              activeTags={activeTags}
              setActiveTags={handleSetActiveTags}
              hiddenLayerTags={hiddenLayerTags}
              setHiddenLayerTags={handleSetHiddenLayerTags}
              availableTags={availableTags}
              layers={layers}
              onHoverLayer={handleHoverLayer}
              onCreateLayer={handleCreateLayer}
              onUpdateLayer={handleUpdateLayer}
              onDeleteLayer={handleDeleteLayer}
              tagColors={tagColors}
              selectedElement={selectedElement}
              onUpdateTags={handleUpdateTags}
              onCreateTag={handleCreateTag}
              suppressed={elementPanel.isOpen || connectorPanel.isOpen || viewDetails.isOpen}
            />

            <EditorOverlays
              connectGhostPos={connectGhostPos}
              clickConnectMode={clickConnectMode}
              clickConnectCursorPos={clickConnectCursorPos}
              handleReconnectDrag={canvas.handleReconnectDrag}
              rfNodes={flowNodes}
            />

            <ViewDrawMenu
              drawingMode={drawingMode} drawingTool={drawingTool} setDrawingTool={setDrawingTool}
              drawingColor={drawingColor} setDrawingColor={setDrawingColor}
              drawingWidth={drawingWidth} setDrawingWidth={setDrawingWidth}
              onUndo={handleUndo} onRedo={handleRedo}
              canUndo={drawingHistoryRef.current.length > 0} canRedo={drawingRedoStackRef.current.length > 0}
              setDrawingMode={setDrawingMode}
            />

            {/* Inline text editor ... */}
            {/* ... */}

            <DrawingCanvas
              ref={drawingCanvasRef}
              paths={drawingPaths}
              isDrawing={drawingMode} isVisible={drawingVisible}
              strokeColor={drawingColor} strokeWidth={drawingWidth} mode={drawingTool}
              onPathComplete={onPathComplete} onPathDelete={onPathDelete} onPathUpdate={onPathUpdate}
              onTextPositionSelected={(canvasX, canvasY, flowX, flowY) => setTextEditorState({ canvasX, canvasY, flowX, flowY })}
            />



            <ConnectorContextMenu
              menu={connectorLongPressMenu}
              onEdit={(edgeId) => { const connector = connectors.find((e) => e.id === edgeId); if (connector) { setSelectedEdge(connector); connectorPanel.onOpen() }; setConnectorLongPressMenu(null) }}
              onMoveSource={(edgeId) => { const picking = { edgeId, endpoint: 'source' as const }; reconnectPickingRef.current = picking; setReconnectPicking(picking); setConnectorLongPressMenu(null) }}
              onMoveTarget={(edgeId) => { const picking = { edgeId, endpoint: 'target' as const }; reconnectPickingRef.current = picking; setConnectorLongPressMenu(null) }}
              onDelete={async (edgeId) => {
                setConnectorLongPressMenu(null)
                if (!viewId) return
                try {
                  await api.workspace.connectors.delete('', edgeId)
                  removeConnectorGraphSnapshot(viewId, edgeId)
                  setConnectors((prev) => prev.filter((connector) => connector.id !== edgeId))
                } catch { /* intentionally empty */ }
              }}
            />

            {/* Reconnect picking banner */}
            {reconnectPicking && (
              <Box position="absolute" top="14px" left="50%" transform="translateX(-50%)" zIndex={2000}
                bg="var(--bg-dots)" border="1px" borderColor="gray.900" px={4} py={2} rounded="xl" shadow="xl"
                display="flex" alignItems="center" gap={3} onClick={(e) => e.stopPropagation()}>
                <Text fontSize="sm" fontWeight="semibold">Tap a node to set as new {reconnectPicking.endpoint}</Text>
                <Button size="xs" variant="clay" onClick={() => { reconnectPickingRef.current = null; setReconnectPicking(null) }}>Cancel</Button>
              </Box>
            )}

            <CanvasContextMenu
              menu={canvasMenu}
              onAddElement={(x, y) => {
                const rect = containerRef.current?.getBoundingClientRect()
                if (rect) showAddingElementAt(x + rect.left, y + rect.top)
                setCanvasMenu(null)
              }}
            />

            {/* Inline element adder */}
            {addingElementAt && (
              <InlineElementAdder
                x={addingElementAt.x} y={addingElementAt.y} expandResults={addingElementAt.expandResults}
                allElements={allElements} existingElementIds={existingElementIds}
                allowCreate={addingElementAt.mode === 'add'}
                title={addingElementAt.mode === 'connect' ? 'Connect To Off-View Element' : undefined}
                placeholder={addingElementAt.mode === 'connect' ? 'Search workspace elements...' : undefined}
                getSecondaryLabel={addingElementAt.mode === 'connect'
                  ? (obj) => placementSummaryByElementId[obj.id] ?? obj.technology ?? null
                  : undefined}
                onConfirmNew={handleConfirmNewElement}
                onConfirmExisting={addingElementAt.mode === 'connect' ? handleConfirmConnectExistingElement : handleConfirmExistingElement}
                onCancel={() => { setAddingElementAt(null); setPendingConnectionSource(null) }}
              />
            )}

            <EmptyCanvasState isMobile={isMobileLayout} hasNodes={rfNodes.length > 0} />

            <ViewFloatingMenu
              handleAddElementAtCenter={handleAddElementAtCenter}
              drawingMode={drawingMode} setDrawingMode={setDrawingMode}
              hasDrawingPaths={drawingPaths.length > 0} drawingVisible={drawingVisible} setDrawingVisible={setDrawingVisible}
              extrasOpen={extrasOpen} setExtrasOpen={setExtrasOpen}
              focusMode={!crossBranchSettings.enabled}
              onFocusModeChange={handleFocusModeChange}
              disableImportExport={disableImportExport}
              onImport={importModal.onOpen} onExport={handleOpenExport} onShare={onShare}
              allTags={availableTags}
              layers={layers}
              tagColors={tagColors}
              hiddenTags={hiddenLayerTags}
              toggleTagVisibility={toggleTagVisibility}
              toggleLayerVisibility={toggleLayerVisibility}
              tagCounts={tagCounts}
              layerElementCounts={layerElementCounts}
              setHighlightedTags={setHoveredLayerTags}
              setHighlightColor={setHoveredLayerColor}
              shareSlot={shareSlot}
              toolbarSlot={toolbarSlot}
            />
          </Box>
        </Flex>

        <ElementLibrary
          existingElementIds={existingElementIds}
          existingElements={existingElements}
          onCreateNew={handleCreateNewLibrary} refresh={libraryRefresh}
          isOpen={libraryOpen} onClose={handleCloseLibrary}
          onTapAdd={canEdit ? handleTapAdd : undefined}
          onFindElement={handleFindElement}
          onTouchDrop={canEdit ? handleTouchDrop : undefined}
        />

        <ElementPanel
          isOpen={elementPanel.isOpen} onClose={elementPanel.onClose} element={selectedElement}
          onSave={handleElementSaved} autoSave
          onDelete={handleElementDeleted} onPermanentDelete={handleElementPermanentlyDeleted}
          orgId={''}
          links={selectedElement ? (linksMap[selectedElement.id] || EMPTY_LINKS) : EMPTY_LINKS}
          parentLinks={selectedElement ? (parentLinksMap[selectedElement.id] || EMPTY_LINKS) : EMPTY_LINKS}
          hasBackdrop={isMobileLayout}
          availableTags={availableTags}
          elementPanelAfterContentSlot={elementPanelAfterContentSlot}
        />

        <CodePreviewPanel isOpen={codePreview.isOpen} onClose={codePreview.onClose} element={previewElement} hasBackdrop={isMobileLayout} />

        <ConnectorPanel
          isOpen={connectorPanel.isOpen} onClose={connectorPanel.onClose} connector={selectedEdge}
          orgId={''}
          onSave={handleConnectorSave} autoSave
          onDelete={handleConnectorDeleteInPanel}
          hasBackdrop={isMobileLayout}
          connectorPanelAfterContentSlot={connectorPanelAfterContentSlot}
        />
        <ProxyConnectorPanel
          isOpen={proxyConnectorPanel.isOpen}
          onClose={proxyConnectorPanel.onClose}
          details={selectedProxyConnectorDetails}
          hasBackdrop={isMobileLayout}
        />

        <ViewPanel
          isOpen={viewDetails.isOpen} onClose={viewDetails.onClose}
          view={view as ViewTreeNode}
          onSave={handleViewSave} hasBackdrop={isMobileLayout}
        />

        <ExportModal
          isOpen={exportModal.isOpen} onClose={exportModal.onClose}
          defaultFilename={sanitizeExportFilename((view as ViewTreeNode).name)}
          onExport={handleExportView} isExporting={isExporting}
        />
        <ImportModal
          isOpen={importModal.isOpen} onClose={importModal.onClose}
          onImport={handleImportView} isImporting={isImporting}
        />
        {!demoOptions?.disableOnboarding && <ViewEditorOnboarding hasElements={rfNodes.length > 0} />}
      </Box>
    </ViewEditorContext.Provider>
  )
}

export default function ViewEditor(props: Props) {
  return (
    <ReactFlowProvider>
      <ViewEditorInner {...props} />
    </ReactFlowProvider>
  )
}
