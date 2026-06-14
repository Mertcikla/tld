import { memo, useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import type { CoreUISlots } from '../../slots'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { parseNumericId } from '../../utils/ids'
import { useSafeFitView } from '../../hooks/useSafeFitView'
import { SafeBackground } from '../../components/SafeBackground'
import ReactFlow, {
  BackgroundVariant,
  ConnectionMode,
  MarkerType,
  PanOnScrollMode,
  ReactFlowProvider,
  useReactFlow,
} from 'reactflow'
import type { ConnectionLineType, Edge as RFEdge, EdgeMarker as RFEdgeMarker, Node as RFNode, NodeChange } from 'reactflow'
import 'reactflow/dist/style.css'
import { toPng, toSvg } from 'html-to-image'
import {
  Box,
  Button,
  CloseButton,
  Flex,
  IconButton,
  Input,
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
import { api, type MermaidImportSummary, type ParsedImport } from '../../api/client'
import type {
  ViewTreeNode,
  PlacedElement,
  LibraryElement as WorkspaceElement,
  Connector,
  ElementReactionSummary,
  ThreadResolveEvent,
  ViewMarkdownDocument,
  ViewComment,
  ViewConnector,
  ViewThread,
  VisibilityOverride,
  Tag,
} from '../../types'
import ElementNode from '../../components/ElementNode'
import ElementPanel from '../../components/ElementPanel'
import ElementPanelCollaboration from '../../components/ElementPanelCollaboration'
import MergeDialog from '../../components/MergeDialog'
import ConfirmDialog from '../../components/ConfirmDialog'
import CodePreviewPanel from '../../components/CodePreviewPanel'
import ConnectorPanel from '../../components/ConnectorPanel'
import ConnectorPanelCollaboration from '../../components/ConnectorPanelCollaboration'
import ElementLibrary from '../../components/ElementLibrary'
import ViewExplorer from '../../components/ViewExplorer'
import ViewMarkdownPanel from '../../components/ViewMarkdownPanel'
import ViewPanel from '../../components/ViewPanel'
import { useSetHeader } from '../../components/HeaderContext'
import { useTouchOnlyCanvasInput } from '../../hooks/useCanvasInputMode'
import { shouldEnableCanvasWheelPan } from '../../utils/canvasInputMode'
import { usePlatform } from '../../platform/context'
import type {
  RealtimeCursor,
  RealtimeUserPresence,
  ViewRealtimeConnection,
  ViewRealtimeHandlers,
} from '../../platform/types'
import ExportModal, { type ExportOptions } from '../../components/ExportModal'
import ImportModal from '../../components/ImportModal'
import { KbdHint } from '../../components/PanelUI'
import ViewHeaderButton from '../../components/ViewHeaderButton'
import ViewEditorOnboarding from '../../components/ViewEditorOnboarding'
import DrawingCanvas, { type DrawingCanvasHandle, type DrawingPath } from '../../components/DrawingCanvas'
import ViewFloatingMenu from '../../components/ViewFloatingMenu'
import ViewDrawMenu from '../../components/ViewDrawMenu'
import ViewBezierConnector, { ViewConnectorConnectionLine } from '../../components/ViewBezierConnector'
import ViewContextNeighborElement from '../../components/ContextNeighborElement'
import ContextBoundaryElement from '../../components/ContextBoundaryElement'
import ContextStraightConnector from '../../components/ContextStraightConnector'
import ProxyConnectorEdge from '../../components/ProxyConnectorEdge'
import ProxyConnectorPanel from '../../components/ProxyConnectorPanel'
import {
  clampContextNodeAxisPosition,
  reconcileMeasuredContextNodes,
  type ContextNodePositionOverride,
  type ContextSide,
  useViewContextNeighbours,
} from './hooks/useViewContextNeighbours'
import { canonicalNodePairKey } from './pairKey'
import { vscodeBridge } from '../../lib/vscodeBridge'
import { openTextFile } from '../../lib/desktop'
import type { ExtensionToWebviewMessage } from '../../types/vscode-messages'

import { ViewEditorContext } from './context'
import { useViewData } from './hooks/useViewData'
import { useDrawingEngine } from './hooks/useDrawingEngine'
import { connectorStyleForCreate, connectorStyleForPreview, useConnectorStyle } from '../../context/ConnectorStyleContext'
import {
  PENDING_ELEMENT_NODE_ID,
  applyNodeChangesWithStructuralSharing,
  useCanvasInteractions,
  type ClickConnectCursorPosition,
  type ClickConnectModeState,
} from './hooks/useCanvasInteractions'
import { useViewEditHistory } from './hooks/useViewEditHistory'
import { useOverlapDetection } from './hooks/useOverlapDetection'
import { removeCollisions } from '../../utils/layout'
import { connectorToConnector, findClosestHandles, sanitizeExportFilename, triggerBlobDownload, triggerDownload } from './utils'
import { DEFAULT_SOURCE_HANDLE_SIDE, DEFAULT_TARGET_HANDLE_SIDE, getCenterVisualHandleId, getLogicalHandleId, getOppositeHandleSide } from '../../utils/edgeDistribution'
import { pickUnusedColor } from '../../components/ViewExplorer/utils'

import { EmptyCanvasState } from './components/EmptyCanvasState'
import { EditorOverlays } from './components/EditorOverlays'
import { ConnectorContextMenu, CanvasContextMenu } from './components/EditorMenus'
import { TEMPORARY_CONNECTOR_EDGE_STYLE, TEMPORARY_CONNECTOR_PATH_STYLE } from './temporaryConnectorStyle'
import {
  VIEW_SELECTION_CLIPBOARD_MIME,
  buildViewSelectionClipboardPayload,
  findViewSelectionPasteConflicts,
  mapViewSelectionElementIds,
  parseViewSelectionClipboardPayload,
  planViewSelectionPasteConnectors,
  planViewSelectionPastePlacements,
  serializeViewSelectionClipboardPayload,
  type ViewSelectionClipboardPayload,
} from './clipboard'
import SelectionBulkBar from './components/SelectionBulkBar'
import { overrideViewContentInSnapshot } from '../../crossBranch/graph'
import { useCrossBranchContextSettings } from '../../crossBranch/settings'
import { removeConnectorGraphSnapshot, removePlacementGraphSnapshot, upsertConnectorGraphSnapshot, upsertPlacementGraphSnapshot, useWorkspaceGraphSnapshot } from '../../crossBranch/store'
import type { ProxyConnectorDetails } from '../../crossBranch/types'
import { useDemoRevealViewport, type ViewEditorDemoOptions } from '../../demo/viewEditor'
import { buildElementLibraryItems, useStore, placedElementToLibraryElement, resolveElementForUpdate } from '../../store/useStore'
import { useWorkspaceVersionPreview } from '../../context/WorkspaceVersionContext'
import { useExperimental } from '../../context/ExperimentalContext'
import {
  elementSelectionRects,
  planSelectionAlignment,
  planSelectionDistribution,
  selectedElementIds,
  selectionBounds,
  visibleElementSelectionRects,
  type SelectionAlign,
  type SelectionDistribute,
  type SelectionNodeUpdate,
} from './selection'
import { deriveViewNoiseGateEnabled } from './noiseGate'

const nodeTypes = {
  elementNode: ElementNode,
  contextNeighborNode: ViewContextNeighborElement,
  ContextBoundaryElement: ContextBoundaryElement,
}
const edgeTypes = {
  default: ViewBezierConnector,
  straight: ViewBezierConnector,
  step: ViewBezierConnector,
  smoothstep: ViewBezierConnector,
  contextStraightConnector: ContextStraightConnector,
  proxyConnectorEdge: ProxyConnectorEdge,
}
const EMPTY_LINKS: ViewConnector[] = []
const EMPTY_TAG_COLORS: Record<string, Tag> = {}
const noop = () => { }
const noopAsync = async () => { }
const VIEW_EDITOR_MIN_ZOOM_FLOOR = 0.12
const VIEW_EDITOR_INITIAL_FIT_PADDING = 0.25
const VIEW_EDITOR_FOCUS_FIT_PADDING = 0.35
const VIEW_EDITOR_EMPTY_EXTENT_RATIO = 0.75
const VIEW_EDITOR_PAN_MARGIN_RATIO = 0.25
const VIEW_EDITOR_PAN_MARGIN_MIN = 180
const VIEW_EDITOR_PAN_MARGIN_MAX = 720
const VIEW_EDITOR_MAX_ZOOM = 4
const VIEW_EDITOR_MARKDOWN_DEFAULT_WIDTH = 540
const VIEW_EDITOR_MARKDOWN_MIN_WIDTH = 360
const VIEW_EDITOR_MARKDOWN_MIN_WIDTH_MOBILE = 280
const VIEW_EDITOR_CANVAS_MIN_WIDTH = 420
const VIEW_EDITOR_CANVAS_MIN_WIDTH_MOBILE = 260
const VIEW_EDITOR_MARKDOWN_RESIZE_HANDLE_WIDTH = 10
const VIEW_EDITOR_TOPBAR_NOTCH_LEFT_VAR = '--topbar-notch-left'
const SNAP_GRID: [number, number] = [30, 30]
const REMOTE_CURSOR_STALE_MS = 30000
const REMOTE_CURSOR_COLORS = ['#38bdf8', '#f97316', '#a78bfa', '#22c55e', '#f43f5e', '#eab308', '#14b8a6']

type CollaborationHeaderState = {
  viewers: RealtimeUserPresence[]
  collaborators: RealtimeUserPresence[]
  followUserId: string | null
  onAvatarClick?: (userId: string) => void
}

type RemoteCursorState = RealtimeCursor & {
  updatedAt: number
}

type CanvasViewport = {
  x: number
  y: number
  zoom: number
}

type CanvasViewportStore = {
  getSnapshot: () => CanvasViewport
  subscribe: (listener: () => void) => () => void
  setViewport: (viewport: CanvasViewport) => void
}

type ViewMetadataSnapshot = Pick<ViewTreeNode, 'id' | 'name' | 'level_label'>

function buildViewNameById(nodes: ViewTreeNode[]) {
  const names = new Map<number, string>()
  const visit = (items: ViewTreeNode[]) => {
    for (const item of items) {
      names.set(item.id, item.name)
      if (item.children.length > 0) visit(item.children)
    }
  }
  visit(nodes)
  return names
}

type PendingDuplicatePaste = {
  payload: ViewSelectionClipboardPayload
  targetViewId: number
  pasteCenter: { x: number; y: number }
  conflictElementIds: number[]
}

function cursorColorForUser(userId: string) {
  let hash = 0
  for (let i = 0; i < userId.length; i += 1) {
    hash = (hash * 31 + userId.charCodeAt(i)) >>> 0
  }
  return REMOTE_CURSOR_COLORS[hash % REMOTE_CURSOR_COLORS.length]
}

function isRenderableCursor(cursor: RealtimeCursor, selfUserId: string | null): cursor is RealtimeCursor {
  return !!cursor.user_id &&
    cursor.user_id !== selfUserId &&
    Number.isFinite(cursor.x) &&
    Number.isFinite(cursor.y)
}

function createCanvasViewportStore(initial: CanvasViewport = { x: 0, y: 0, zoom: 1 }): CanvasViewportStore {
  let snapshot = { ...initial }
  const listeners = new Set<() => void>()

  return {
    getSnapshot: () => snapshot,
    subscribe: (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    setViewport: (viewport) => {
      if (snapshot.x === viewport.x && snapshot.y === viewport.y && snapshot.zoom === viewport.zoom) return
      snapshot = { x: viewport.x, y: viewport.y, zoom: viewport.zoom }
      listeners.forEach((listener) => listener())
    },
  }
}

interface RemoteCursorOverlayProps {
  cursors: RemoteCursorState[]
  viewportStore: CanvasViewportStore
}

const RemoteCursorOverlay = memo(function RemoteCursorOverlay({ cursors, viewportStore }: RemoteCursorOverlayProps) {
  const canvasViewport = useSyncExternalStore(
    viewportStore.subscribe,
    viewportStore.getSnapshot,
    viewportStore.getSnapshot,
  )

  return (
    <Box position="absolute" inset={0} pointerEvents="none" zIndex={11} overflow="hidden">
      {cursors.map((cursor) => {
        const color = cursorColorForUser(cursor.user_id)
        const left = cursor.x * canvasViewport.zoom + canvasViewport.x
        const top = cursor.y * canvasViewport.zoom + canvasViewport.y
        if (!Number.isFinite(left) || !Number.isFinite(top)) return null
        return (
          <Box
            key={cursor.user_id}
            position="absolute"
            left={0}
            top={0}
            transform={`translate(${left}px, ${top}px)`}
            transition="transform 80ms linear"
            willChange="transform"
          >
            <svg width="20" height="24" viewBox="0 0 20 24" fill="none" aria-hidden="true">
              <path
                d="M2 2.5L17 12.5L10.7 14.1L7.2 22L2 2.5Z"
                fill={color}
                stroke="white"
                strokeWidth="1.5"
                strokeLinejoin="round"
              />
            </svg>
            <Text
              position="absolute"
              left="16px"
              top="16px"
              maxW="180px"
              px={2}
              py={0.5}
              borderRadius="6px"
              bg={color}
              color="white"
              fontSize="11px"
              fontWeight="600"
              lineHeight="1.2"
              whiteSpace="nowrap"
              overflow="hidden"
              textOverflow="ellipsis"
              boxShadow="0 6px 16px rgba(0,0,0,0.25)"
            >
              {cursor.username || 'User'}
            </Text>
          </Box>
        )
      })}
    </Box>
  )
})

function stringArraysEqual(a: string[], b: string[]) {
  if (a.length !== b.length) return false
  return a.every((item, index) => item === b[index])
}

async function copyTextToClipboard(text: string) {
  let clipboardError: unknown
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch (err) {
      clipboardError = err
    }
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.select()
  const copied = document.execCommand('copy')
  textarea.remove()
  if (!copied) throw clipboardError ?? new Error('Clipboard copy failed')
}

function elementUpdatePayload(element: WorkspaceElement) {
  return {
    name: element.name,
    description: element.description ?? '',
    kind: element.kind ?? '',
    technology: element.technology ?? '',
    url: element.url ?? '',
    logo_url: element.logo_url ?? '',
    technology_connectors: element.technology_connectors ?? [],
    tags: element.tags ?? [],
    repo: element.repo,
    branch: element.branch,
    file_path: element.file_path,
    language: element.language,
  }
}

function connectorUpdatePayload(connector: Connector) {
  return {
    source_element_id: connector.source_element_id,
    target_element_id: connector.target_element_id,
    label: connector.label ?? '',
    description: connector.description ?? '',
    relationship: connector.relationship ?? '',
    direction: connector.direction,
    style: connector.style === 'default' ? 'bezier' : connector.style,
    url: connector.url ?? '',
    source_handle: connector.source_handle,
    target_handle: connector.target_handle,
  }
}

function getContextBoundaryBounds(nodes: RFNode[]) {
  const boundaryNode = nodes.find((node) => node.type === 'ContextBoundaryElement')
  if (!boundaryNode) return null

  const boundaryData = boundaryNode.data as { width?: number; height?: number } | undefined
  const width = boundaryData?.width ?? boundaryNode.width
  const height = boundaryData?.height ?? boundaryNode.height
  if (width == null || height == null) return null

  return {
    left: boundaryNode.position.x,
    right: boundaryNode.position.x + width,
    top: boundaryNode.position.y,
    bottom: boundaryNode.position.y + height,
  }
}

function clampContextNodeChangePosition(
  node: RFNode,
  position: { x: number; y: number },
  bounds: NonNullable<ReturnType<typeof getContextBoundaryBounds>>,
  fixedPosition: { x: number; y: number } = node.position,
) {
  const side = (node.data as { side?: ContextSide } | undefined)?.side
  if (!side) return { position, override: null }

  if (side === 'left' || side === 'right') {
    const axisPosition = clampContextNodeAxisPosition(side, position.y, bounds)
    return {
      position: { x: fixedPosition.x, y: axisPosition },
      override: { side, axisPosition } as ContextNodePositionOverride,
    }
  }

  const axisPosition = clampContextNodeAxisPosition(side, position.x, bounds)
  return {
    position: { x: axisPosition, y: fixedPosition.y },
    override: { side, axisPosition } as ContextNodePositionOverride,
  }
}

function connectorSnapshotsEqual(left: Connector, right: Connector) {
  return JSON.stringify(connectorUpdatePayload(left)) === JSON.stringify(connectorUpdatePayload(right))
}

function elementSnapshotsEqual(left: WorkspaceElement, right: WorkspaceElement) {
  return JSON.stringify(elementUpdatePayload(left)) === JSON.stringify(elementUpdatePayload(right))
}

function placementSnapshotsEqual(left: PlacedElement, right: PlacedElement) {
  return left.view_id === right.view_id &&
    left.element_id === right.element_id &&
    Math.abs(left.position_x - right.position_x) < 0.5 &&
    Math.abs(left.position_y - right.position_y) < 0.5
}

function viewSnapshotsEqual(left: ViewMetadataSnapshot, right: ViewMetadataSnapshot) {
  return left.id === right.id && left.name === right.name && (left.level_label ?? '') === (right.level_label ?? '')
}

function initialViewMarkdown(name?: string | null) {
  const trimmed = name?.trim()
  return trimmed ? `# ${trimmed}\n\n` : ''
}

function isMarkdownConflictError(error: unknown) {
  return error instanceof Error && /markdown file changed/i.test(error.message)
}

function clampMarkdownPaneWidth(width: number, totalWidth: number, isMobileLayout: boolean) {
  const minMarkdownWidth = isMobileLayout
    ? VIEW_EDITOR_MARKDOWN_MIN_WIDTH_MOBILE
    : VIEW_EDITOR_MARKDOWN_MIN_WIDTH
  const minCanvasWidth = isMobileLayout
    ? VIEW_EDITOR_CANVAS_MIN_WIDTH_MOBILE
    : VIEW_EDITOR_CANVAS_MIN_WIDTH
  const maxMarkdownWidth = Math.max(minMarkdownWidth, totalWidth - minCanvasWidth)
  return Math.min(Math.max(width, minMarkdownWidth), maxMarkdownWidth)
}

function nodesMatchCurrentView(nodes: RFNode[], elements: PlacedElement[], viewId: number | null) {
  if (viewId === null || elements.length === 0) return false
  if (!elements.every((element) => element.view_id === viewId)) return false

  const nodesById = new Map(nodes.map((node) => [node.id, node]))
  return elements.every((element) => {
    const node = nodesById.get(String(element.element_id))
    return node !== undefined &&
      Math.abs(node.position.x - (element.position_x ?? 0)) < 0.5 &&
      Math.abs(node.position.y - (element.position_y ?? 0)) < 0.5
  })
}

function alphaColor(color: string, opacity: number): string {
  if (opacity >= 1) return color
  return `color-mix(in srgb, ${color} ${Math.round(opacity * 100)}%, transparent)`
}

function isEditableKeyboardTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  return !!target.closest('input, textarea, select, [contenteditable="true"], [contenteditable=""]')
}

function isCanvasKeyboardTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return true
  if (target === document.body || target === document.documentElement) return true
  return !!target.closest('[data-testid="vieweditor-canvas"]')
}

function countLabel(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`
}

function mermaidImportDescription(summary: MermaidImportSummary) {
  return [
    `Resolved ${countLabel(summary.resolvedElementCount, 'element')} and ${countLabel(summary.resolvedConnectorCount, 'connector')}.`,
    `Created ${countLabel(summary.createdElementCount, 'element')} and ${countLabel(summary.createdConnectorCount, 'connector')}.`,
  ].join(' ')
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

export interface ViewEditorPermissions {
  canEdit?: boolean
  isOwner?: boolean
  isFreePlan?: boolean
}

interface Props extends CoreUISlots, ViewEditorPermissions {
  demoOptions?: ViewEditorDemoOptions
}

function ViewEditorInner({
  demoOptions,
  canEdit = true,
  isOwner = true,
  isFreePlan = false,
  canvasOverlaySlot,
  toolbarSlot,
  shareSlot,
  elementPanelAfterContentSlot,
  connectorPanelAfterContentSlot,
  rightSlot: _rightSlot,
  mobileMenuSlot: _mobileMenuSlot,
  settingsSlot: _settingsSlot,
  mobileSettingsSlot: _mobileSettingsSlot,
  userControlsSlot: _userControlsSlot,
}: Props) {
  const { id: viewIdParam } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const viewId = parseNumericId(viewIdParam)
  const navigate = useNavigate()
  const platform = usePlatform()
  const navigateRef = useRef(navigate)
  navigateRef.current = navigate

  const toast = useToast()
  const { experimental } = useExperimental()
  const mermaidIntegrationEnabled = experimental.mermaidIntegrationEnabled
  const {
    canUndo: canUndoViewEdit,
    canRedo: canRedoViewEdit,
    isApplyingHistory,
    pushAction: pushEditAction,
    clearHistory: clearEditHistory,
    undo: undoViewEdit,
    redo: redoViewEdit,
  } = useViewEditHistory()
  const setHeader = useSetHeader()
  const realtimeRef = useRef<ViewRealtimeConnection | null>(null)
  const realtimeClockRef = useRef(0)
  const realtimeSelfUserIdRef = useRef<string | null>(null)
  const [remoteCursors, setRemoteCursors] = useState<RemoteCursorState[]>([])
  const canvasViewportStoreRef = useRef<CanvasViewportStore | null>(null)
  if (canvasViewportStoreRef.current === null) {
    canvasViewportStoreRef.current = createCanvasViewportStore()
  }
  const canvasViewportStore = canvasViewportStoreRef.current
  const [collaboration, setCollaboration] = useState<CollaborationHeaderState>({
    viewers: [],
    collaborators: [],
    followUserId: null,
  })
  const [liveThreadUpsert, setLiveThreadUpsert] = useState<ViewThread | null>(null)
  const [liveThreadResolve, setLiveThreadResolve] = useState<ThreadResolveEvent | null>(null)
  const [liveCommentCreate, setLiveCommentCreate] = useState<ViewComment | null>(null)
  const [liveReactions, setLiveReactions] = useState<ElementReactionSummary[] | undefined>(undefined)
  const handleCollaborationAvatarClick = useCallback((userId: string) => {
    setCollaboration((prev) => ({
      ...prev,
      followUserId: prev.followUserId === userId ? null : userId,
    }))
  }, [])
  const isMobileLayout = useBreakpointValue({ base: true, md: false }) ?? false
  const touchOnlyCanvasInput = useTouchOnlyCanvasInput()
  const enableCanvasWheelPan = shouldEnableCanvasWheelPan({ isMobileLayout, touchOnlyInput: touchOnlyCanvasInput })
  const [densityLevel, setDensityLevel] = useState(0)
  const [visibilityOverrides, setVisibilityOverrides] = useState<VisibilityOverride[]>([])
  const [noiseGateBusy, setNoiseGateBusy] = useState(false)
  const [pendingNoiseGateEnabled, setPendingNoiseGateEnabled] = useState<boolean | null>(null)
  const lastNonFullDensityLevelRef = useRef(0)

  const elementPanel = useDisclosure()
  const connectorPanel = useDisclosure()
  const proxyConnectorPanel = useDisclosure()
  const viewDetails = useDisclosure()
  const exportModal = useDisclosure()
  const importModal = useDisclosure()
  const codePreview = useDisclosure()
  const mergeDialog = useDisclosure()
  const duplicatePasteConfirm = useDisclosure()
  const [mergeSourceElement, setMergeSourceElement] = useState<WorkspaceElement | null>(null)
  const [adjustingLayout, setAdjustingLayout] = useState(false)
  const [pendingDuplicatePaste, setPendingDuplicatePaste] = useState<PendingDuplicatePaste | null>(null)
  const [isClipboardPasting, setIsClipboardPasting] = useState(false)

  useEffect(() => {
    if (viewId == null) {
      setDensityLevel(0)
      setVisibilityOverrides([])
      setNoiseGateBusy(false)
      setPendingNoiseGateEnabled(null)
      lastNonFullDensityLevelRef.current = 0
      return
    }
    let cancelled = false
    void Promise.all([
      api.workspace.views.density.get(viewId).catch(() => 0),
      api.workspace.views.visibilityOverrides.list(viewId).catch(() => []),
    ]).then(([level, overrides]) => {
      if (cancelled) return
      setDensityLevel(level)
      setVisibilityOverrides(overrides)
    })
    return () => { cancelled = true }
  }, [viewId])

  useEffect(() => {
    if (densityLevel !== 2) {
      lastNonFullDensityLevelRef.current = densityLevel
    }
  }, [densityLevel])

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

  const openViewDetailsRef = useRef(viewDetails.onToggle)
  openViewDetailsRef.current = viewDetails.onToggle

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
  const [selectedProxyConnectorDetails, setSelectedProxyConnectorDetails] = useState<ProxyConnectorDetails | null>(null)
  const [suppressedSelectedConnectorHandleHighlightId, setSuppressedSelectedConnectorHandleHighlightId] = useState<number | null>(null)
  const suppressSelectedConnectorHandleHighlight = selectedEdge !== null && suppressedSelectedConnectorHandleHighlightId === selectedEdge.id

  useEffect(() => {
    if (suppressedSelectedConnectorHandleHighlightId !== null && selectedEdge?.id !== suppressedSelectedConnectorHandleHighlightId) {
      setSuppressedSelectedConnectorHandleHighlightId(null)
    }
  }, [selectedEdge?.id, suppressedSelectedConnectorHandleHighlightId])

  const [prevViewId, setPrevViewId] = useState(viewId)
  if (viewId !== prevViewId) {
    setPrevViewId(viewId)
    setSelectedElement(null)
    setSelectedEdge(null)
    setSelectedProxyConnectorDetails(null)
  }
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
  const isPasteImportingRef = useRef(false)
  const pendingPasteSelectionRef = useRef<{ viewId: number; elementIds: Set<number> } | null>(null)
  const [isExporting, setIsExporting] = useState(false)
  const [bulkMergeLoadingId, setBulkMergeLoadingId] = useState<number | null>(null)
  const setViewEditorUi = useStore((state) => state.setViewEditorUi)
  const snapToGrid = useStore((state) => state.snapToGrid)
  const setStoreSnapToGrid = useStore((state) => state.setSnapToGrid)
  const upsertStoreConnector = useStore((state) => state.upsertConnector)
  const removeStoreConnector = useStore((state) => state.removeConnector)
  const updateStoreElementPosition = useStore((state) => state.updateElementPosition)
  const mergeElementsInto = useStore((state) => state.mergeElementsInto)
  const refreshElementsRef = useRef<() => Promise<void>>(async () => { })
  const setSnapToGrid = useCallback((snap: boolean) => {
    setStoreSnapToGrid(snap)
    if (typeof window !== 'undefined') localStorage.setItem('diag:snapToGrid', String(snap))
  }, [setStoreSnapToGrid])

  useEffect(() => {
    if (typeof window === 'undefined') return
    setStoreSnapToGrid(localStorage.getItem('diag:snapToGrid') === 'true')
  }, [setStoreSnapToGrid])

  useEffect(() => {
    setViewEditorUi({
      viewId,
      canEdit,
      isOwner,
      isFreePlan,
      snapToGrid,
      selectedElement,
      selectedConnector: selectedEdge,
    })
  }, [canEdit, isFreePlan, isOwner, selectedEdge, selectedElement, setViewEditorUi, snapToGrid, viewId])

  useEffect(() => { localStorage.setItem('diag:snapToGrid', String(snapToGrid)) }, [snapToGrid])
  const [, setHoveredZoom] = useState<{ elementId: number | null; type: 'in' | 'out' | null } | null>(null)
  const hoveredZoomRef = useRef<{ elementId: number | null; type: 'in' | 'out' | null } | null>(null)
  const hoverPanLockedUntilRef = useRef(0)

  const [activeTags, setActiveTags] = useState<string[]>([])
  const activeTagsRef = useRef<string[]>([])
  activeTagsRef.current = activeTags
  const applyingRemoteVisibilityRef = useRef(false)
  const { preview: versionPreview, followTarget: versionFollowTarget } = useWorkspaceVersionPreview()
  const [tagColors, setTagColors] = useState<Record<string, Tag>>({})

  useEffect(() => {
    api.workspace.orgs.tagColors.list().then((res) => {
      if (Array.isArray(res)) {
        const next: Record<string, Tag> = {}
        res.forEach(t => { next[t.name] = t })
        setTagColors(next)
      } else {
        setTagColors(res)
      }
    }).catch(() => { /* skip */ })
  }, [])

  const [layers, setLayers] = useState<import('../../types').ViewLayer[]>([])
  const [hiddenLayerTags, setHiddenLayerTags] = useState<string[]>(() => demoOptions?.defaultHiddenLayerTags ?? [])
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
      clearEditHistory()
      setLayers(prev => [...prev, layer])
    } catch (e) {
      toast({ status: 'error', title: 'Failed to create layer', description: String(e) })
    }
  }, [clearEditHistory, viewId, toast])

  const handleCreateTag = useCallback(async (tag: string, color?: string, description?: string) => {
    const name = tag.trim()
    if (!name) return

    const nextColor = color ?? tagColors[name]?.color ?? pickUnusedColor(Object.values(tagColors).map(t => t.color))
    const nextDescription = description ?? tagColors[name]?.description ?? null

    await api.workspace.orgs.tagColors.update(name, nextColor, nextDescription)
    clearEditHistory()
    setTagColors((prev) => ({ ...prev, [name]: { name, color: nextColor, description: nextDescription } }))
  }, [clearEditHistory, tagColors])

  const handleUpdateLayer = useCallback(async (layer: import('../../types').ViewLayer) => {
    if (viewId === null) return
    try {
      const updated = await api.workspace.views.layers.update(viewId, layer.id, layer)
      clearEditHistory()
      setLayers(prev => prev.map(l => l.id === updated.id ? updated : l))
    } catch (e) {
      toast({ status: 'error', title: 'Failed to update layer', description: String(e) })
    }
  }, [clearEditHistory, viewId, toast])

  const handleDeleteLayer = useCallback(async (layerId: number) => {
    if (viewId === null) return
    try {
      await api.workspace.views.layers.delete(viewId, layerId)
      clearEditHistory()
      setLayers(prev => prev.filter(l => l.id !== layerId))
    } catch (e) {
      toast({ status: 'error', title: 'Failed to delete layer', description: String(e) })
    }
  }, [clearEditHistory, viewId, toast])

  const containerRef = useRef<HTMLDivElement | null>(null)
  const viewEditorCanvasRef = useRef<HTMLDivElement | null>(null)
  const drawingCanvasRef = useRef<DrawingCanvasHandle | null>(null)

  const { safeFitView } = useSafeFitView(containerRef)
  const { screenToFlowPosition, fitView, setViewport, getViewport } = useReactFlow()
  const screenToFlowPositionRef = useRef(screenToFlowPosition)
  screenToFlowPositionRef.current = screenToFlowPosition
  const needsFitView = useRef(true)
  const rfReadyRef = useRef(false)
  const fittedContextForViewRef = useRef<number | null>(null)
  const [initialViewportReady, setInitialViewportReady] = useState(false)
  const [computedMinZoom, setComputedMinZoom] = useState(VIEW_EDITOR_MIN_ZOOM_FLOOR)
  const [interactionSourceId, setInteractionSourceId] = useState<number | null>(null)
  const [clickConnectMode, setClickConnectMode] = useState<ClickConnectModeState | null>(null)
  const [clickConnectCursorPos, setClickConnectCursorPos] = useState<ClickConnectCursorPosition | null>(null)
  const [isConnectorCreatePreviewActive, setConnectorCreatePreviewActive] = useState(false)
  const interactionSourceIdRef = useRef<number | null>(null)
  const multiConnectionSourceIdsRef = useRef<number[] | null>(null)
  const [deletedLibraryElementIds, setDeletedLibraryElementIds] = useState<number[]>([])

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
  const stableOnInteractionStartRef = useRef<(elementId: number, options?: { sourceHandle?: string; clientX?: number; clientY?: number }) => void>(() => { })
  const stableOnStartHandleReconnectRef = useRef<(args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number }) => void>(() => { })
  const stableOnReconnectPickRef = useRef<(targetElementId: number) => Promise<boolean>>(async () => false)

  // ── Drawing engine ────────────────────────────────────────────────────────
  const drawing = useDrawingEngine(viewId)
  const {
    drawingMode, setDrawingMode, drawingVisible, setDrawingVisible,
    drawingPaths, setDrawingPaths, drawingTool, setDrawingTool,
    drawingColor, setDrawingColor, drawingWidth, setDrawingWidth,
    textEditorState, setTextEditorState, commitDrawingText,
    drawingHistoryRef, drawingRedoStackRef,
    handleUndo, handleRedo, onPathComplete, onPathDelete, onPathUpdate,
  } = drawing

  // ── Data ──────────────────────────────────────────────────────────────────
  const stableOnZoomInRef = useRef<(id: number) => Promise<void>>(async () => { })
  const stableOnZoomOutRef = useRef<(id: number) => Promise<void>>(async () => { })
  const stableOnNavigateToViewRef = useRef<(id: number) => void>(() => { })
  const stableOnRemoveElementRef = useRef<(id: number) => Promise<void>>(async () => { })

  const data = useViewData({
    viewId,
    interactionSourceId,
    clickConnectMode,
    isConnectorCreatePreviewActive,
    selectedConnector: selectedEdge,
    suppressSelectedConnectorHandleHighlight,
    activeTags,
    hiddenLayerTags,
    hoveredLayerTags,
    hoveredLayerColor,
    tagColors,
    versionPreview,
    versionFollowTarget,
    stableOnZoomIn: useCallback(async (id: number) => { await stableOnZoomInRef.current(id) }, []),
    stableOnZoomOut: useCallback(async (id: number) => { await stableOnZoomOutRef.current(id) }, []),
    stableOnNavigateToView: useCallback((id: number) => { stableOnNavigateToViewRef.current(id) }, []),
    stableOnSelect: useCallback((obj: PlacedElement) => {
      void stableOnReconnectPickRef.current(obj.element_id).then((handled) => {
        if (handled) return
        setSelectedEdge(null)
        setSelectedProxyConnectorDetails(null)
        closeProxyConnectorPanelRef.current()
        closeConnectorPanelRef.current()
        setSelectedElement({
          id: obj.element_id, name: obj.name, description: obj.description, kind: obj.kind,
          technology: obj.technology, url: obj.url, logo_url: obj.logo_url,
          technology_connectors: obj.technology_connectors, tags: obj.tags, repo: obj.repo,
          branch: obj.branch, file_path: obj.file_path, language: obj.language,
          bypass_noise_gate: obj.bypass_noise_gate ?? false,
          created_at: '', updated_at: '', has_view: false, view_label: null,
        })
        openElementPanelRef.current()
      })
    }, []),
    stableOnOpenCodePreview: useCallback((elementId: number) => {
      const obj = previewViewElementsRef.current.find((o) => o.element_id === elementId)
      if (obj) {
        setPreviewElement(obj)
        openCodePreviewRef.current()
      }
    }, []),
    stableOnInteractionStart: useCallback((elementId: number, options?: { sourceHandle?: string; clientX?: number; clientY?: number }) => {
      stableOnInteractionStartRef.current(elementId, options)
    }, []),
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
    treeData, allElements,
    existingElementIds,
    viewElementsRef, linksMapRef, parentLinksMapRef, incomingLinksRef,
    treeDataRef, rfNodesRef, rfEdgesRef, viewIdRef,
    refreshGrid, refreshElements,
    handleElementDeleted, handleElementPermanentlyDeleted, handleElementSaved: applyElementSaved,
  } = data
  refreshElementsRef.current = refreshElements
  const remoteViewRefreshTimerRef = useRef<number | null>(null)

  const scheduleRemoteViewRefresh = useCallback(() => {
    if (remoteViewRefreshTimerRef.current !== null) return
    remoteViewRefreshTimerRef.current = window.setTimeout(() => {
      remoteViewRefreshTimerRef.current = null
      void Promise.all([refreshElements(), refreshGrid()])
    }, 100)
  }, [refreshElements, refreshGrid])

  useEffect(() => {
    return () => {
      if (remoteViewRefreshTimerRef.current !== null) {
        window.clearTimeout(remoteViewRefreshTimerRef.current)
        remoteViewRefreshTimerRef.current = null
      }
    }
  }, [viewId])

  const applyRemoteCanvasVisibility = useCallback((visibility: { active_tags: string[]; hidden_layer_tags: string[] }) => {
    const nextActiveTags = visibility.active_tags
    const nextHiddenLayerTags = visibility.hidden_layer_tags
    const activeChanged = !stringArraysEqual(activeTagsRef.current, nextActiveTags)
    const hiddenChanged = !stringArraysEqual(hiddenLayerTagsRef.current, nextHiddenLayerTags)
    if (!activeChanged && !hiddenChanged) return

    applyingRemoteVisibilityRef.current = true
    if (activeChanged) setActiveTags(nextActiveTags)
    if (hiddenChanged) setHiddenLayerTags(nextHiddenLayerTags)
    window.setTimeout(() => { applyingRemoteVisibilityRef.current = false }, 0)
  }, [])

  const realtimeHandlers = useMemo<ViewRealtimeHandlers>(() => ({
    onSnapshot: (snapshot) => {
      realtimeSelfUserIdRef.current = snapshot.self_user_id || null
      setCollaboration({
        viewers: snapshot.viewers,
        collaborators: snapshot.collaborators,
        followUserId: null,
      })
      const now = Date.now()
      setRemoteCursors(snapshot.cursors
        .filter((cursor) => isRenderableCursor(cursor, realtimeSelfUserIdRef.current))
        .map((cursor) => ({ ...cursor, updatedAt: now })))
      if (snapshot.has_canvas_visibility) {
        applyRemoteCanvasVisibility(snapshot.canvas_visibility)
      } else {
        realtimeRef.current?.sendCanvasVisibility(activeTagsRef.current, hiddenLayerTagsRef.current)
      }
      setDrawingPaths(snapshot.drawings.reduce<DrawingPath[]>((paths, drawing) => {
        if (!drawing.path_id) return paths
        paths.push({
          id: drawing.path_id,
          points: drawing.points ?? [],
          color: drawing.color ?? '#38bdf8',
          width: drawing.width ?? 3,
          text: drawing.text,
          fontSize: drawing.font_size,
        })
        return paths
      }, []))
      snapshot.crdt_elements.forEach((state) => {
        updateStoreElementPosition(state.element_id, state.x, state.y)
      })
      snapshot.crdt_connectors.forEach((state) => {
        if (state.deleted) {
          if (viewId !== null) removeConnectorGraphSnapshot(viewId, state.connector_id)
          removeStoreConnector(state.connector_id)
        } else if (state.connector) {
          upsertConnectorGraphSnapshot(state.connector)
          upsertStoreConnector(state.connector)
        }
      })
    },
    onPresenceJoin: (viewer) => {
      setCollaboration((prev) => {
        const viewers = prev.viewers.some((item) => item.user_id === viewer.user_id)
          ? prev.viewers.map((item) => item.user_id === viewer.user_id ? { ...viewer, online: true } : item)
          : [...prev.viewers, { ...viewer, online: true }]
        const collaborators = prev.collaborators.some((item) => item.user_id === viewer.user_id)
          ? prev.collaborators.map((item) => item.user_id === viewer.user_id ? { ...viewer, online: true } : item)
          : [...prev.collaborators, { ...viewer, online: true }]
        return { ...prev, viewers, collaborators }
      })
    },
    onPresenceLeave: (userId) => {
      setCollaboration((prev) => ({
        ...prev,
        viewers: prev.viewers.filter((viewer) => viewer.user_id !== userId),
        collaborators: prev.collaborators.map((viewer) => viewer.user_id === userId ? { ...viewer, online: false } : viewer),
        followUserId: prev.followUserId === userId ? null : prev.followUserId,
      }))
      setRemoteCursors((prev) => prev.filter((cursor) => cursor.user_id !== userId))
    },
    onCursor: (cursor) => {
      if (!isRenderableCursor(cursor, realtimeSelfUserIdRef.current)) return
      const nextCursor = { ...cursor, updatedAt: Date.now() }
      setRemoteCursors((prev) => {
        const existing = prev.find((item) => item.user_id === cursor.user_id)
        if (!existing) return [...prev, nextCursor]
        return prev.map((item) => item.user_id === cursor.user_id ? nextCursor : item)
      })
    },
    onSelection: () => { },
    onViewport: () => { },
    onCanvasVisibility: applyRemoteCanvasVisibility,
    onDrawing: (drawing) => {
      if (!drawing.path_id) return
      const next: DrawingPath = {
        id: drawing.path_id,
        points: drawing.points ?? [],
        color: drawing.color ?? '#38bdf8',
        width: drawing.width ?? 3,
        text: drawing.text,
        fontSize: drawing.font_size,
      }
      setDrawingPaths((prev) => {
        const exists = prev.some((path) => path.id === next.id)
        return exists ? prev.map((path) => path.id === next.id ? next : path) : [...prev, next]
      })
    },
    onDrawingDelete: (pathId) => {
      setDrawingPaths((prev) => prev.filter((path) => path.id !== pathId))
    },
    onCRDTElementPosition: (state) => {
      if (state.actor_user_id && state.actor_user_id === realtimeSelfUserIdRef.current) return
      updateStoreElementPosition(state.element_id, state.x, state.y)
    },
    onCRDTConnectorUpsert: (state) => {
      if (state.actor_user_id && state.actor_user_id === realtimeSelfUserIdRef.current) return
      if (state.connector) {
        upsertConnectorGraphSnapshot(state.connector)
        upsertStoreConnector(state.connector)
      }
    },
    onCRDTConnectorDelete: (state) => {
      if (state.actor_user_id && state.actor_user_id === realtimeSelfUserIdRef.current) return
      if (viewId !== null) removeConnectorGraphSnapshot(viewId, state.connector_id)
      removeStoreConnector(state.connector_id)
    },
    onViewElementAdd: (element) => {
      setViewElements((prev) => prev.some((item) => item.element_id === element.element_id)
        ? prev.map((item) => item.element_id === element.element_id ? element : item)
        : [...prev, element])
      void refreshGrid()
    },
    onViewElementRemove: (elementId) => {
      handleElementDeleted(elementId)
      void refreshGrid()
    },
    onElementUpdate: (element) => {
      applyElementSaved(element)
    },
    onThreadUpsert: (thread) => {
      setLiveThreadUpsert(thread as ViewThread)
    },
    onThreadResolve: (event) => {
      setLiveThreadResolve(event)
    },
    onCommentCreate: (comment) => {
      setLiveCommentCreate(comment as ViewComment)
    },
    onReactionsSnapshot: (items) => {
      setLiveReactions(items as ElementReactionSummary[])
    },
    onViewStateChange: scheduleRemoteViewRefresh,
    onClose: () => { },
    onRoomFull: () => {
      toast({ status: 'warning', title: 'Collaboration room is full' })
    },
  }), [
    applyElementSaved,
    applyRemoteCanvasVisibility,
    handleElementDeleted,
    refreshGrid,
    removeStoreConnector,
    scheduleRemoteViewRefresh,
    setDrawingPaths,
    setViewElements,
    toast,
    updateStoreElementPosition,
    upsertStoreConnector,
    viewId,
  ])

  useEffect(() => {
    realtimeRef.current?.disconnect()
    realtimeRef.current = null
    realtimeSelfUserIdRef.current = null
    setRemoteCursors([])
    setCollaboration({ viewers: [], collaborators: [], followUserId: null })
    setLiveThreadUpsert(null)
    setLiveThreadResolve(null)
    setLiveCommentCreate(null)
    setLiveReactions(undefined)

    if (viewId === null || !platform.connectRealtime) return
    realtimeRef.current = platform.connectRealtime(viewId, realtimeHandlers)

    return () => {
      realtimeRef.current?.disconnect()
      realtimeRef.current = null
    }
  }, [platform, realtimeHandlers, viewId])

  useEffect(() => {
    if (applyingRemoteVisibilityRef.current) return
    realtimeRef.current?.sendCanvasVisibility(activeTags, hiddenLayerTags)
  }, [activeTags, hiddenLayerTags])

  useEffect(() => {
    if (remoteCursors.length === 0) return
    const interval = window.setInterval(() => {
      const cutoff = Date.now() - REMOTE_CURSOR_STALE_MS
      setRemoteCursors((prev) => prev.filter((cursor) => cursor.updatedAt >= cutoff))
    }, 5000)
    return () => window.clearInterval(interval)
  }, [remoteCursors.length])

  useEffect(() => {
    realtimeRef.current?.sendSelection(selectedElement?.id ?? null, selectedEdge?.id ?? null)
  }, [selectedEdge?.id, selectedElement?.id])

  const handleRealtimePathComplete = useCallback((path: DrawingPath) => {
    onPathComplete(path)
    realtimeRef.current?.sendDrawing(path.id, path.points, path.color, path.width, path.text, path.fontSize)
  }, [onPathComplete])

  const handleRealtimePathDelete = useCallback((pathId: string) => {
    onPathDelete(pathId)
    realtimeRef.current?.sendDrawingDelete(pathId)
  }, [onPathDelete])

  const handleRealtimePathUpdate = useCallback((path: DrawingPath) => {
    onPathUpdate(path)
    realtimeRef.current?.sendDrawing(path.id, path.points, path.color, path.width, path.text, path.fontSize)
  }, [onPathUpdate])

  const handleRealtimeElementPositionPreview = useCallback((elementId: number, x: number, y: number) => {
    realtimeClockRef.current += 1
    realtimeRef.current?.sendCRDTElementPosition(elementId, x, y, realtimeClockRef.current)
  }, [])

  const publishRealtimeConnectorUpsert = useCallback((connector: Connector) => {
    realtimeClockRef.current += 1
    realtimeRef.current?.sendCRDTConnectorUpsert(connector, realtimeClockRef.current)
  }, [])

  const publishRealtimeConnectorDelete = useCallback((connectorId: number) => {
    realtimeClockRef.current += 1
    realtimeRef.current?.sendCRDTConnectorDelete(connectorId, realtimeClockRef.current)
  }, [])

  const [viewMarkdown, setViewMarkdown] = useState<ViewMarkdownDocument | null>(null)
  const [viewMarkdownContent, setViewMarkdownContent] = useState('')
  const [loadedViewMarkdownContent, setLoadedViewMarkdownContent] = useState('')
  const [viewMarkdownSyncToken, setViewMarkdownSyncToken] = useState(0)
  const [isMarkdownOpen, setIsMarkdownOpen] = useState(false)
  const [isMarkdownLoading, setIsMarkdownLoading] = useState(false)
  const [isMarkdownMutating, setIsMarkdownMutating] = useState(false)
  const [isMarkdownSaving, setIsMarkdownSaving] = useState(false)
  const [markdownSaveConflict, setMarkdownSaveConflict] = useState(false)
  const [isMarkdownResizing, setIsMarkdownResizing] = useState(false)
  const [markdownPaneWidth, setMarkdownPaneWidth] = useState(() => {
    if (typeof window === 'undefined') return VIEW_EDITOR_MARKDOWN_DEFAULT_WIDTH
    const stored = Number.parseFloat(window.localStorage.getItem('diag:markdownPaneWidth') ?? '')
    return Number.isFinite(stored) && stored > 0 ? stored : VIEW_EDITOR_MARKDOWN_DEFAULT_WIDTH
  })
  const markdownRequestSeqRef = useRef(0)
  const editorSplitRef = useRef<HTMLDivElement | null>(null)
  const markdownResizeStateRef = useRef<{ startX: number; startWidth: number } | null>(null)

  const getClampedMarkdownPaneWidth = useCallback((nextWidth: number) => {
    const totalWidth = editorSplitRef.current?.clientWidth ?? window.innerWidth
    return clampMarkdownPaneWidth(nextWidth, totalWidth, isMobileLayout)
  }, [isMobileLayout])

  useEffect(() => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem('diag:markdownPaneWidth', String(markdownPaneWidth))
  }, [markdownPaneWidth])

  useEffect(() => {
    if (!isMarkdownOpen) return

    const handleResize = () => {
      setMarkdownPaneWidth((current) => getClampedMarkdownPaneWidth(current))
    }

    handleResize()
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [getClampedMarkdownPaneWidth, isMarkdownOpen])

  useEffect(() => {
    if (!isMarkdownResizing) return

    const handlePointerMove = (event: PointerEvent) => {
      const resizeState = markdownResizeStateRef.current
      if (!resizeState) return
      event.preventDefault()
      const delta = resizeState.startX - event.clientX
      setMarkdownPaneWidth(getClampedMarkdownPaneWidth(resizeState.startWidth + delta))
    }

    const stopResizing = () => {
      markdownResizeStateRef.current = null
      setIsMarkdownResizing(false)
    }

    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', stopResizing)
    window.addEventListener('pointercancel', stopResizing)

    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', stopResizing)
      window.removeEventListener('pointercancel', stopResizing)
    }
  }, [getClampedMarkdownPaneWidth, isMarkdownResizing])

  const resetViewMarkdown = useCallback(() => {
    markdownRequestSeqRef.current += 1
    setViewMarkdown(null)
    setViewMarkdownContent('')
    setLoadedViewMarkdownContent('')
    setViewMarkdownSyncToken((prev) => prev + 1)
    setMarkdownSaveConflict(false)
    setIsMarkdownOpen(false)
    setIsMarkdownLoading(false)
  }, [])

  const loadViewMarkdown = useCallback(async (targetViewId: number, options: { silent?: boolean } = {}) => {
    const requestSeq = ++markdownRequestSeqRef.current
    setIsMarkdownLoading(true)
    try {
      const result = await api.workspace.views.markdown.get(targetViewId)
      if (markdownRequestSeqRef.current !== requestSeq) return null
      if (!result) {
        resetViewMarkdown()
        return null
      }
      setViewMarkdown(result.markdown)
      setViewMarkdownContent(result.content)
      setLoadedViewMarkdownContent(result.content)
      setViewMarkdownSyncToken((prev) => prev + 1)
      setMarkdownSaveConflict(false)
      return result
    } catch (error) {
      if (markdownRequestSeqRef.current !== requestSeq) return null
      resetViewMarkdown()
      if (!options.silent) {
        toast({
          status: 'error',
          title: 'Failed to load markdown',
          description: error instanceof Error ? error.message : String(error),
        })
      }
      return null
    } finally {
      if (markdownRequestSeqRef.current === requestSeq) setIsMarkdownLoading(false)
    }
  }, [resetViewMarkdown, toast])

  const loadedViewMarkdownPath = view && view.id === viewId ? view.markdown?.path ?? null : undefined

  useEffect(() => {
    if (viewId === null || !loadedViewMarkdownPath) {
      resetViewMarkdown()
      return
    }
    void loadViewMarkdown(viewId, { silent: true })
  }, [loadedViewMarkdownPath, loadViewMarkdown, resetViewMarkdown, viewId])

  const markdownDirty = viewMarkdownContent !== loadedViewMarkdownContent
  const markdownBusy = isMarkdownLoading || isMarkdownMutating || isMarkdownSaving

  const openMarkdownPathInEditor = useCallback((path?: string | null, content = viewMarkdownContent) => {
    if (!path || viewId === null) return
    vscodeBridge.postMessage({
      type: 'open-markdown',
      viewId,
      path,
      content,
      viewColumn: 'beside',
    })
  }, [viewId, viewMarkdownContent])

  const handleMarkdownResizeStart = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (!isMarkdownOpen) return
    markdownResizeStateRef.current = {
      startX: event.clientX,
      startWidth: markdownPaneWidth,
    }
    setIsMarkdownResizing(true)
    event.preventDefault()
  }, [isMarkdownOpen, markdownPaneWidth])

  const handleCreateManagedMarkdown = useCallback(async (options: { fileName?: string; initialContent?: string; openEditor?: boolean; targetKind?: string; path?: string } = {}) => {
    if (!canEdit || viewId === null) return null
    setIsMarkdownMutating(true)
    try {
      const updatedView = await api.workspace.views.markdown.create(viewId, {
        fileName: options.fileName,
        initialContent: options.initialContent ?? initialViewMarkdown(view?.name),
        targetKind: options.targetKind,
        path: options.path,
      })
      setView(updatedView)
      const loadedMarkdown = await loadViewMarkdown(viewId)
      if (options.openEditor !== false) setIsMarkdownOpen(true)
      return loadedMarkdown
    } catch (error) {
      toast({
        status: 'error',
        title: 'Failed to create markdown',
        description: error instanceof Error ? error.message : String(error),
      })
      return null
    } finally {
      setIsMarkdownMutating(false)
    }
  }, [canEdit, loadViewMarkdown, setView, toast, view?.name, viewId])

  const handleCreateMarkdownFromPanel = useCallback(async (targetKind: string, path?: string) => {
    await handleCreateManagedMarkdown({ targetKind, path, openEditor: true })
  }, [handleCreateManagedMarkdown])

  const handleToggleMarkdown = useCallback(() => {
    if (window.__TLD_VSCODE__) {
      if (!viewMarkdown) {
        void handleCreateManagedMarkdown({ openEditor: false }).then((result) => {
          openMarkdownPathInEditor(result?.markdown.path, result?.content ?? '')
        })
        return
      }
      openMarkdownPathInEditor(viewMarkdown.path)
      return
    }
    if (!viewMarkdown) {
      setIsMarkdownOpen((prev) => !prev)
      return
    }
    setIsMarkdownOpen((prev) => !prev)
  }, [handleCreateManagedMarkdown, openMarkdownPathInEditor, viewMarkdown])

  const handleOpenMarkdown = useCallback(() => {
    if (!viewMarkdown) return
    if (window.__TLD_VSCODE__) {
      openMarkdownPathInEditor(viewMarkdown.path)
      return
    }
    setIsMarkdownOpen(true)
  }, [openMarkdownPathInEditor, viewMarkdown])

  const handleReloadMarkdown = useCallback(async () => {
    if (viewId === null) return
    await loadViewMarkdown(viewId)
  }, [loadViewMarkdown, viewId])

  const handleLinkMarkdown = useCallback(async (path: string) => {
    if (!canEdit || viewId === null) return
    setIsMarkdownMutating(true)
    try {
      const updatedView = await api.workspace.views.markdown.link(viewId, path)
      setView(updatedView)
      await loadViewMarkdown(viewId)
      setMarkdownSaveConflict(false)
      setIsMarkdownOpen(true)
      toast({ status: 'success', title: 'Markdown linked' })
    } catch (error) {
      toast({
        status: 'error',
        title: 'Failed to link markdown',
        description: error instanceof Error ? error.message : String(error),
      })
    } finally {
      setIsMarkdownMutating(false)
    }
  }, [canEdit, loadViewMarkdown, setView, toast, viewId])

  const handleUnlinkMarkdown = useCallback(async ({ deleteManagedFile }: { deleteManagedFile: boolean } = { deleteManagedFile: false }) => {
    if (!canEdit || viewId === null) return
    setIsMarkdownMutating(true)
    try {
      const updatedView = await api.workspace.views.markdown.unlink(viewId, deleteManagedFile)
      setView(updatedView)
      resetViewMarkdown()
      toast({ status: 'success', title: 'Markdown unlinked' })
    } catch (error) {
      toast({
        status: 'error',
        title: 'Failed to unlink markdown',
        description: error instanceof Error ? error.message : String(error),
      })
    } finally {
      setIsMarkdownMutating(false)
    }
  }, [canEdit, resetViewMarkdown, setView, toast, viewId])

  const handleSaveMarkdown = useCallback(async (markdown: string, options: { force?: boolean } = {}) => {
    if (viewId === null || !viewMarkdown) return
    setIsMarkdownSaving(true)
    try {
      const updated = await api.workspace.views.markdown.save(viewId, markdown, {
        expectedFileVersion: options.force ? undefined : viewMarkdown.file_version,
        force: options.force ?? false,
      })
      setViewMarkdown(updated)
      setViewMarkdownContent(markdown)
      setLoadedViewMarkdownContent(markdown)
      setMarkdownSaveConflict(false)
      toast({ status: 'success', title: 'Notes saved' })
    } catch (error) {
      if (isMarkdownConflictError(error)) {
        setMarkdownSaveConflict(true)
        toast({
          status: 'warning',
          title: 'Markdown changed on disk',
          description: 'Reload the file or overwrite it from the notes panel.',
        })
        return
      }
      toast({
        status: 'error',
        title: 'Failed to save notes',
        description: error instanceof Error ? error.message : String(error),
      })
    } finally {
      setIsMarkdownSaving(false)
    }
  }, [toast, viewId, viewMarkdown])

  const handleForceSaveMarkdown = useCallback(async (markdown: string) => {
    await handleSaveMarkdown(markdown, { force: true })
  }, [handleSaveMarkdown])

  const handlePickMarkdownFile = useCallback(async () => {
    const result = await openTextFile([
      { displayName: 'Markdown Files (*.md;*.markdown;*.mdx)', pattern: '*.md;*.markdown;*.mdx' },
      { displayName: 'All Files (*.*)', pattern: '*.*' },
    ])
    if (result.canceled) return null
    return result.path
  }, [])

  const handleSaveMarkdownAs = useCallback(async (markdown: string) => {
    const baseName = sanitizeExportFilename(view?.name || 'view-notes')
    try {
      const result = await triggerBlobDownload(new Blob([markdown], { type: 'text/markdown;charset=utf-8' }), `${baseName}.md`, 'markdown')
      if (result.canceled) return
      toast({ status: 'success', title: 'Notes exported', description: result.path ? `Saved ${result.path}` : undefined })
    } catch (error) {
      toast({
        status: 'error',
        title: 'Failed to export notes',
        description: error instanceof Error ? error.message : String(error),
      })
    }
  }, [toast, view?.name])

  const handleOpenMarkdownInEditor = useCallback(() => {
    openMarkdownPathInEditor(viewMarkdown?.path)
  }, [openMarkdownPathInEditor, viewMarkdown?.path])

  const handleElementPermanentlyDeletedEverywhere = useCallback((elementId: number) => {
    handleElementPermanentlyDeleted(elementId)
    setDeletedLibraryElementIds((prev) => prev.includes(elementId) ? prev : [...prev, elementId])
  }, [handleElementPermanentlyDeleted])

  const { hasSignificantOverlaps, dismiss: dismissOverlapSuggestion } = useOverlapDetection(rfNodes, viewId)
  const selectedCanvasElementIds = useMemo(() => selectedElementIds(rfNodes), [rfNodes])
  const selectedCanvasElementCount = selectedCanvasElementIds.length
  const selectedCanvasElementIdKey = selectedCanvasElementIds.join(',')
  const selectedCanvasElements = useMemo(() => {
    const selectedIds = new Set(selectedCanvasElementIds)
    return viewElements.filter((element) => selectedIds.has(element.element_id))
  }, [selectedCanvasElementIds, viewElements])
  const selectedCanvasMergeOptions = useMemo(() => selectedCanvasElements.map((element) => ({
    id: element.element_id,
    name: element.name,
    kind: element.kind,
  })), [selectedCanvasElements])
  const selectedCanvasTagCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    selectedCanvasElements.forEach((element) => {
      const tags = element.tags ?? []
      tags.forEach((tag) => {
        counts[tag] = (counts[tag] ?? 0) + 1
      })
    })
    return counts
  }, [selectedCanvasElements])

  const focusCanvasElement = useCallback((elementId: number) => {
    window.requestAnimationFrame(() => {
      const target = containerRef.current?.querySelector<HTMLElement>(`[data-testid="vieweditor-node"][data-element-id="${elementId}"]`)
      target?.focus({ preventScroll: true })
    })
  }, [])

  const replaceCanvasElementSelection = useCallback((elementId: number) => {
    const placedElement = viewElementsRef.current.find((element) => element.element_id === elementId)

    setSelectedElement(placedElement ? placedElementToLibraryElement(placedElement) : null)
    setSelectedEdge(null)
    setSelectedProxyConnectorDetails(null)
    closeConnectorPanelRef.current()
    closeProxyConnectorPanelRef.current()

    setRfEdges((edges) => edges.map((edge) => edge.selected ? { ...edge, selected: false } : edge))
    setRfNodes((nodes) => nodes.map((node) => {
      const nodeElementId = node.type === 'elementNode' ? parseNumericId(node.id) : null
      const selected = nodeElementId === elementId
      return node.selected === selected ? node : { ...node, selected }
    }))
    focusCanvasElement(elementId)
  }, [focusCanvasElement, setRfEdges, setRfNodes, viewElementsRef])

  const selectAllVisibleCanvasElements = useCallback((elementIds: number[]) => {
    const selectedIds = new Set(elementIds)
    const singleSelectedId = selectedIds.size === 1 ? elementIds[0] : null
    const placedElement = singleSelectedId === null
      ? null
      : viewElementsRef.current.find((element) => element.element_id === singleSelectedId) ?? null

    setSelectedElement(placedElement ? placedElementToLibraryElement(placedElement) : null)
    setSelectedEdge(null)
    setSelectedProxyConnectorDetails(null)
    closeConnectorPanelRef.current()
    closeProxyConnectorPanelRef.current()
    if (selectedIds.size !== 1) closeElementPanelRef.current()

    setRfEdges((edges) => edges.map((edge) => edge.selected ? { ...edge, selected: false } : edge))
    setRfNodes((nodes) => nodes.map((node) => {
      const nodeElementId = node.type === 'elementNode' ? parseNumericId(node.id) : null
      const selected = nodeElementId !== null && selectedIds.has(nodeElementId)
      return node.selected === selected ? node : { ...node, selected }
    }))

    if (singleSelectedId !== null) focusCanvasElement(singleSelectedId)
  }, [focusCanvasElement, setRfEdges, setRfNodes, viewElementsRef])

  const getVisibleCanvasElementRects = useCallback(() => {
    const rect = containerRef.current?.getBoundingClientRect()
    const viewport = getViewport()
    return visibleElementSelectionRects(
      rfNodesRef.current,
      rect
        ? { ...viewport, width: rect.width, height: rect.height }
        : null,
    )
  }, [getViewport, rfNodesRef])

  useEffect(() => {
    if (drawingMode) return

    const handleKeyDown = (e: KeyboardEvent) => {
      if (isEditableKeyboardTarget(e.target) || !isCanvasKeyboardTarget(e.target)) return

      const key = e.key.toLowerCase()
      if (key === 'tab' && !e.ctrlKey && !e.metaKey && !e.altKey) {
        const visibleRects = getVisibleCanvasElementRects()
        if (visibleRects.length === 0) return

        e.preventDefault()
        const selectedIndex = visibleRects.findIndex((rect) => selectedCanvasElementIds.includes(rect.elementId))
        const nextIndex = selectedIndex === -1
          ? e.shiftKey ? visibleRects.length - 1 : 0
          : (selectedIndex + (e.shiftKey ? -1 : 1) + visibleRects.length) % visibleRects.length
        replaceCanvasElementSelection(visibleRects[nextIndex].elementId)
        return
      }

      if (key === 'a' && (e.ctrlKey || e.metaKey) && !e.altKey) {
        const visibleRects = getVisibleCanvasElementRects()
        if (visibleRects.length === 0) return

        e.preventDefault()
        selectAllVisibleCanvasElements(visibleRects.map((rect) => rect.elementId))
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [
    drawingMode,
    getVisibleCanvasElementRects,
    replaceCanvasElementSelection,
    selectAllVisibleCanvasElements,
    selectedCanvasElementIds,
  ])

  useEffect(() => {
    const pendingSelection = pendingPasteSelectionRef.current
    if (!pendingSelection || pendingSelection.viewId !== viewId || pendingSelection.elementIds.size === 0) return

    const visibleElementIds = new Set(
      rfNodes
        .filter((node) => node.type === 'elementNode')
        .map((node) => parseNumericId(node.id))
        .filter((id): id is number => id !== null),
    )
    for (const elementId of pendingSelection.elementIds) {
      if (!visibleElementIds.has(elementId)) return
    }

    pendingPasteSelectionRef.current = null
    setSelectedElement(null)
    setSelectedEdge(null)
    setSelectedProxyConnectorDetails(null)
    closeElementPanelRef.current()
    closeConnectorPanelRef.current()
    closeProxyConnectorPanelRef.current()
    setRfEdges((edges) => edges.map((edge) => edge.selected ? { ...edge, selected: false } : edge))
    setRfNodes((nodes) => nodes.map((node) => {
      const elementId = node.type === 'elementNode' ? parseNumericId(node.id) : null
      const selected = elementId !== null && pendingSelection.elementIds.has(elementId)
      return node.selected === selected ? node : { ...node, selected }
    }))
  }, [rfNodes, setRfEdges, setRfNodes, viewId])

  useEffect(() => {
    if (selectedCanvasElementIds.length <= 1) return
    setSelectedElement(null)
    setSelectedEdge(null)
    setSelectedProxyConnectorDetails(null)
    elementPanel.onClose()
    connectorPanel.onClose()
    proxyConnectorPanel.onClose()
  }, [connectorPanel, elementPanel, proxyConnectorPanel, selectedCanvasElementIdKey, selectedCanvasElementIds.length])

  const overrideDeltaFor = useCallback((resourceType: VisibilityOverride['resource_type'], resourceId?: number | null) => {
    if (resourceId == null) return 0
    return visibilityOverrides.find((override) => override.resource_type === resourceType && override.resource_id === resourceId)?.level_delta ?? 0
  }, [visibilityOverrides])

  const reloadVisibilityOverrides = useCallback(async () => {
    if (viewId == null) return
    const overrides = await api.workspace.views.visibilityOverrides.list(viewId).catch(() => [])
    setVisibilityOverrides(overrides)
  }, [viewId])

  const handleDensityLevelChange = useCallback(async (level: number) => {
    if (viewId == null) return
    setDensityLevel(level)
    try {
      await api.workspace.views.density.set(viewId, level)
      clearEditHistory()
      await refreshElements()
    } catch {
      toast({ status: 'error', title: 'Noise gate was not saved' })
    }
  }, [clearEditHistory, refreshElements, toast, viewId])

  const noiseGateEnabled = useMemo(
    () => deriveViewNoiseGateEnabled(densityLevel, visibilityOverrides, pendingNoiseGateEnabled),
    [densityLevel, pendingNoiseGateEnabled, visibilityOverrides],
  )

  const handleNoiseGateEnabledChange = useCallback(async (enabled: boolean) => {
    if (viewId == null || noiseGateBusy) return

    setNoiseGateBusy(true)
    setPendingNoiseGateEnabled(enabled)
    if (!enabled) {
      try {
        await handleDensityLevelChange(2)
      } finally {
        setNoiseGateBusy(false)
        setPendingNoiseGateEnabled(null)
      }
      return
    }

    const previousLevel = densityLevel
    const nextLevel = densityLevel === 2 ? lastNonFullDensityLevelRef.current : densityLevel
    setDensityLevel(nextLevel)
    try {
      const result = await api.workspace.views.noiseGate.initialize(viewId, nextLevel)
      setDensityLevel(result.density_level)
      clearEditHistory()
      await reloadVisibilityOverrides()
      await refreshElements()
    } catch {
      setDensityLevel(previousLevel)
      toast({ status: 'error', title: 'Noise gate was not initialized' })
    } finally {
      setNoiseGateBusy(false)
      setPendingNoiseGateEnabled(null)
    }
  }, [clearEditHistory, densityLevel, handleDensityLevelChange, noiseGateBusy, refreshElements, reloadVisibilityOverrides, toast, viewId])

  const handleVisibilityOverrideDeltaChange = useCallback(async (resourceType: VisibilityOverride['resource_type'], resourceId: number, levelDelta: number) => {
    if (viewId == null) return
    try {
      await api.workspace.views.visibilityOverrides.set(viewId, resourceType, resourceId, levelDelta)
      clearEditHistory()
      await reloadVisibilityOverrides()
      await refreshElements()
    } catch (error) {
      toast({ status: 'error', title: 'Noise gate override was not saved' })
      throw error
    }
  }, [clearEditHistory, refreshElements, reloadVisibilityOverrides, toast, viewId])

  const handleVisibilityOverride = useCallback(async (resourceType: VisibilityOverride['resource_type'], resourceId: number, action: 'promote' | 'demote' | 'reset') => {
    if (viewId == null) return
    try {
      if (action === 'promote') await api.workspace.views.visibilityOverrides.promote(viewId, resourceType, resourceId)
      else if (action === 'demote') await api.workspace.views.visibilityOverrides.demote(viewId, resourceType, resourceId)
      else await api.workspace.views.visibilityOverrides.reset(viewId, resourceType, resourceId)
      clearEditHistory()
      await reloadVisibilityOverrides()
      await refreshElements()
    } catch {
      toast({ status: 'error', title: 'Visibility override was not saved' })
    }
  }, [clearEditHistory, refreshElements, reloadVisibilityOverrides, toast, viewId])

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
        fitView({ nodes: [{ id: String(msg.elementId) }], duration: 800, padding: VIEW_EDITOR_FOCUS_FIT_PADDING })
      } else if (msg.type === 'element-placed') {
        if (viewId === null) return
        try {
          await api.workspace.views.placements.add(viewId, msg.elementId, msg.x, msg.y)
          void refreshElements()
        } catch (e) {
          console.error('Failed to place element from VS Code:', e)
        }
      } else if (msg.type === 'markdown-saved') {
        if (!window.__TLD_VSCODE__) return
        if (viewId === null || msg.viewId !== viewId) return
        setViewMarkdown(msg.markdown)
        setViewMarkdownContent(msg.content)
        setLoadedViewMarkdownContent(msg.content)
        setViewMarkdownSyncToken((prev) => prev + 1)
        setMarkdownSaveConflict(false)
      }
    })
    return unsub
  }, [fitView, viewId, refreshElements])

  const existingElements = useMemo(() => buildElementLibraryItems(allElements, viewElements), [allElements, viewElements])

  const availableTags = useMemo(() => {
    const tags = new Set<string>()
    viewElements.forEach((o) => o.tags?.forEach((t: string) => tags.add(t)))
    allElements.forEach((o) => o.tags?.forEach((t: string) => tags.add(t)))
    Object.keys(tagColors).forEach((t) => tags.add(t))
    return Array.from(tags).sort((a, b) => a.localeCompare(b))
  }, [allElements, tagColors, viewElements])

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

  const supportsZoomOut = useMemo(() => {
    if (viewId == null || !view) return false
    return view.parent_view_id != null && treeData.some(n => n.id === view.parent_view_id)
  }, [viewId, view, treeData])

  const supportsZoomIn = useMemo(() => {
    if (viewId == null) return false
    const onCanvasIds = new Set(viewElements.map((o) => o.element_id))
    return Object.entries(linksMap).some(([elementIdStr, links]) => {
      const elementId = Number(elementIdStr)
      if (!Number.isFinite(elementId) || !onCanvasIds.has(elementId)) return false
      return links && links.length > 0
    })
  }, [viewId, viewElements, linksMap])


  useEffect(() => {
    const requestedElementId = parseNumericId(searchParams.get('element'))
    if (requestedElementId == null) return
    const match = viewElements.find((element) => element.element_id === requestedElementId)
    if (!match) return
    setSelectedEdge(null)
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
      bypass_noise_gate: match.bypass_noise_gate ?? false,
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

  const handleUnsupportedMutation = useCallback(() => {
    clearEditHistory()
  }, [clearEditHistory])

  const pushViewEditAction = useCallback((before: ViewMetadataSnapshot, after: ViewMetadataSnapshot) => {
    if (viewSnapshotsEqual(before, after)) return
    pushEditAction({
      undo: async () => {
        const updated = await api.workspace.views.update(before.id, { name: before.name, label: before.level_label ?? '' })
        if (view && view.id === updated.id) setView({ ...view, name: updated.name, level_label: updated.label })
        await refreshGrid()
      },
      redo: async () => {
        const updated = await api.workspace.views.update(after.id, { name: after.name, label: after.level_label ?? '' })
        if (view && view.id === updated.id) setView({ ...view, name: updated.name, level_label: updated.label })
        await refreshGrid()
      },
    })
  }, [pushEditAction, refreshGrid, setView, view])

  const pushElementEditAction = useCallback((before: WorkspaceElement, after: WorkspaceElement) => {
    if (elementSnapshotsEqual(before, after)) return
    pushEditAction({
      undo: async () => {
        const saved = await api.elements.update(before.id, elementUpdatePayload(before))
        applyElementSaved(saved)
        setSelectedElement((current) => current?.id === saved.id ? saved : current)
        await refreshElements()
      },
      redo: async () => {
        const saved = await api.elements.update(after.id, elementUpdatePayload(after))
        applyElementSaved(saved)
        setSelectedElement((current) => current?.id === saved.id ? saved : current)
        await refreshElements()
      },
    })
  }, [applyElementSaved, pushEditAction, refreshElements])

  const pushElementEditBatchAction = useCallback((beforeBatch: WorkspaceElement[], afterBatch: WorkspaceElement[]) => {
    const pairs = beforeBatch
      .map((before, index) => ({ before, after: afterBatch[index] }))
      .filter((pair): pair is { before: WorkspaceElement; after: WorkspaceElement } =>
        !!pair.after && !elementSnapshotsEqual(pair.before, pair.after)
      )
    if (pairs.length === 0) return

    pushEditAction({
      undo: async () => {
        const saved = await Promise.all(pairs.map(({ before }) => api.elements.update(before.id, elementUpdatePayload(before))))
        saved.forEach(applyElementSaved)
        await refreshElements()
      },
      redo: async () => {
        const saved = await Promise.all(pairs.map(({ after }) => api.elements.update(after.id, elementUpdatePayload(after))))
        saved.forEach(applyElementSaved)
        await refreshElements()
      },
    })
  }, [applyElementSaved, pushEditAction, refreshElements])

  const handleUpdateTags = useCallback(async (elementId: number, tags: string[]) => {
    if (!canEdit) return
    const obj = resolveElementForUpdate(elementId, selectedElement, allElements, viewElements)
    if (!obj) return
    try {
      const saved = await api.elements.update(elementId, {
        ...elementUpdatePayload(obj),
        tags,
      })
      applyElementSaved(saved)
      pushElementEditAction(obj, saved)
      if (selectedElement?.id === elementId) {
        setSelectedElement(saved)
      }
    } catch (err) {
      console.error('Failed to update tags:', err)
    }
  }, [canEdit, selectedElement, allElements, viewElements, applyElementSaved, pushElementEditAction, setSelectedElement])

  const pushPlacementMoveAction = useCallback((before: PlacedElement, after: PlacedElement) => {
    if (placementSnapshotsEqual(before, after)) return
    pushEditAction({
      undo: async () => {
        await api.workspace.views.placements.updatePosition(before.view_id, before.element_id, before.position_x, before.position_y)
        await refreshElements()
      },
      redo: async () => {
        await api.workspace.views.placements.updatePosition(after.view_id, after.element_id, after.position_x, after.position_y)
        await refreshElements()
      },
    })
  }, [pushEditAction, refreshElements])

  const pushPlacementMoveBatchAction = useCallback((beforeBatch: PlacedElement[], afterBatch: PlacedElement[]) => {
    const pairs = beforeBatch
      .map((before, index) => ({ before, after: afterBatch[index] }))
      .filter((pair): pair is { before: PlacedElement; after: PlacedElement } =>
        !!pair.after && !placementSnapshotsEqual(pair.before, pair.after)
      )
    if (pairs.length === 0) return

    pushEditAction({
      undo: async () => {
        await Promise.all(pairs.map(({ before }) =>
          api.workspace.views.placements.updatePosition(before.view_id, before.element_id, before.position_x, before.position_y)
        ))
        await refreshElements()
      },
      redo: async () => {
        await Promise.all(pairs.map(({ after }) =>
          api.workspace.views.placements.updatePosition(after.view_id, after.element_id, after.position_x, after.position_y)
        ))
        await refreshElements()
      },
    })
  }, [pushEditAction, refreshElements])

  const pushPlacementRemoveAction = useCallback((placement: PlacedElement) => {
    pushEditAction({
      undo: async () => {
        await api.workspace.views.placements.add(placement.view_id, placement.element_id, placement.position_x, placement.position_y)
        await refreshElements()
      },
      redo: async () => {
        await api.workspace.views.placements.remove(placement.view_id, placement.element_id)
        await refreshElements()
      },
    })
  }, [pushEditAction, refreshElements])

  const pushPlacementRemoveBatchAction = useCallback((placements: PlacedElement[]) => {
    if (placements.length === 0) return
    pushEditAction({
      undo: async () => {
        await Promise.all(placements.map((placement) =>
          api.workspace.views.placements.add(placement.view_id, placement.element_id, placement.position_x, placement.position_y)
        ))
        await refreshElements()
      },
      redo: async () => {
        await Promise.all(placements.map((placement) =>
          api.workspace.views.placements.remove(placement.view_id, placement.element_id)
        ))
        await refreshElements()
      },
    })
  }, [pushEditAction, refreshElements])

  const commitSelectionPlacementUpdates = useCallback(async (updates: SelectionNodeUpdate[]) => {
    if (!canEdit || viewId === null || updates.length === 0) return
    const updatesByElementId = new Map(updates.map((update) => [update.elementId, update]))
    const updatesByNodeId = new Map(updates.map((update) => [update.id, update]))
    const beforePlacements: PlacedElement[] = []
    const afterPlacements: PlacedElement[] = []

    viewElementsRef.current.forEach((placement) => {
      const update = updatesByElementId.get(placement.element_id)
      if (!update) return
      const after = { ...placement, position_x: update.x, position_y: update.y }
      if (placementSnapshotsEqual(placement, after)) return
      beforePlacements.push({ ...placement })
      afterPlacements.push(after)
    })
    if (afterPlacements.length === 0) return

    setRfNodes((nodes) => nodes.map((node) => {
      const update = updatesByNodeId.get(node.id)
      return update ? { ...node, position: { x: update.x, y: update.y } } : node
    }))
    setViewElements((elements) => elements.map((element) => {
      const update = updatesByElementId.get(element.element_id)
      return update ? { ...element, position_x: update.x, position_y: update.y } : element
    }))

    try {
      await Promise.all(afterPlacements.map((placement) =>
        api.workspace.views.placements.updatePosition(placement.view_id, placement.element_id, placement.position_x, placement.position_y)
      ))
      afterPlacements.forEach((placement) => upsertPlacementGraphSnapshot(placement.view_id, placement))
      pushPlacementMoveBatchAction(beforePlacements, afterPlacements)
    } catch (err) {
      toast({ status: 'error', title: 'Failed to update selection', description: String(err) })
      await refreshElements()
    }
  }, [canEdit, pushPlacementMoveBatchAction, refreshElements, setRfNodes, setViewElements, toast, viewElementsRef, viewId])

  const handleSelectionAlign = useCallback((align: SelectionAlign) => {
    void commitSelectionPlacementUpdates(planSelectionAlignment(rfNodes, align))
  }, [commitSelectionPlacementUpdates, rfNodes])

  const handleSelectionDistribute = useCallback((direction: SelectionDistribute) => {
    void commitSelectionPlacementUpdates(planSelectionDistribution(rfNodes, direction))
  }, [commitSelectionPlacementUpdates, rfNodes])

  const handleFitSelection = useCallback(() => {
    const rects = elementSelectionRects(rfNodes)
    const bounds = selectionBounds(rects)
    if (!bounds) return
    safeFitView({
      nodes: rects.map((rect) => ({ id: rect.id })),
      duration: 500,
      padding: VIEW_EDITOR_FOCUS_FIT_PADDING,
    })
  }, [rfNodes, safeFitView])

  const handleBulkRemoveFromView = useCallback(async () => {
    if (!canEdit || selectedCanvasElements.length === 0) return
    const placements = selectedCanvasElements.map((placement) => ({ ...placement }))
    const selectedIds = new Set(placements.map((placement) => placement.element_id))

    setViewElements((elements) => elements.filter((element) => !selectedIds.has(element.element_id)))
    setRfNodes((nodes) => nodes.filter((node) => !selectedIds.has(Number(node.id))))
    setSelectedElement(null)
    setSelectedEdge(null)
    elementPanel.onClose()
    connectorPanel.onClose()

    try {
      await Promise.all(placements.map((placement) =>
        api.workspace.views.placements.remove(placement.view_id, placement.element_id)
      ))
      placements.forEach((placement) => removePlacementGraphSnapshot(placement.view_id, placement.element_id))
      pushPlacementRemoveBatchAction(placements)
    } catch (err) {
      toast({ status: 'error', title: 'Failed to remove selection', description: String(err) })
      await refreshElements()
    }
  }, [canEdit, connectorPanel, elementPanel, pushPlacementRemoveBatchAction, refreshElements, selectedCanvasElements, setRfNodes, setViewElements, toast])

  const handleBulkMergeInto = useCallback(async (survivorId: number) => {
    if (!canEdit || selectedCanvasElements.length < 2) return

    const sourceIds = selectedCanvasElements
      .map((element) => element.element_id)
      .filter((elementId) => elementId !== survivorId)
    if (sourceIds.length === 0) return

    const sourceIdSet = new Set(sourceIds)
    let survivor: WorkspaceElement | null = null

    setBulkMergeLoadingId(survivorId)
    try {
      for (const sourceId of sourceIds) {
        const result = await api.elements.merge(sourceId, survivorId, {})
        survivor = result.survivor
        mergeElementsInto(sourceId, result.survivor)
      }

      setSelectedElement(survivor)
      setSelectedEdge(null)
      setSelectedProxyConnectorDetails(null)
      elementPanel.onClose()
      connectorPanel.onClose()
      setRfEdges((edges) => edges.map((edge) => edge.selected ? { ...edge, selected: false } : edge))
      setRfNodes((nodes) => nodes
        .filter((node) => !sourceIdSet.has(parseNumericId(node.id) ?? -1))
        .map((node) => {
          const nodeElementId = node.type === 'elementNode' ? parseNumericId(node.id) : null
          const selected = nodeElementId === survivorId
          return node.selected === selected ? node : { ...node, selected }
        }))
      focusCanvasElement(survivorId)
      await refreshElements()
    } catch (err) {
      toast({ status: 'error', title: 'Failed to merge selection', description: String(err) })
      await refreshElements()
    } finally {
      setBulkMergeLoadingId(null)
    }
  }, [
    canEdit,
    connectorPanel,
    elementPanel,
    focusCanvasElement,
    mergeElementsInto,
    refreshElements,
    selectedCanvasElements,
    setRfEdges,
    setRfNodes,
    toast,
  ])

  const handleBulkTagChange = useCallback(async (tag: string, mode: 'add' | 'remove') => {
    if (!canEdit) return
    const name = tag.trim()
    if (!name) return
    if (mode === 'add' && !tagColors[name]) await handleCreateTag(name)

    const selectedIds = new Set(selectedCanvasElementIds)
    const beforeElements = selectedCanvasElementIds
      .map((elementId) => resolveElementForUpdate(elementId, selectedElement, allElements, viewElements))
      .filter((element): element is WorkspaceElement => element !== null)
    const afterElements = beforeElements
      .filter((element) => selectedIds.has(element.id))
      .map((element) => {
        const tags = element.tags ?? []
        const nextTags = mode === 'add'
          ? Array.from(new Set([...tags, name]))
          : tags.filter((existingTag) => existingTag !== name)
        return { ...element, tags: nextTags }
      })
      .filter((element, index) => !elementSnapshotsEqual(beforeElements[index], element))

    if (afterElements.length === 0) return
    const beforeChanged = afterElements
      .map((element) => beforeElements.find((before) => before.id === element.id))
      .filter((element): element is WorkspaceElement => element !== undefined)

    try {
      const saved = await Promise.all(afterElements.map((element) =>
        api.elements.update(element.id, elementUpdatePayload(element))
      ))
      saved.forEach(applyElementSaved)
      pushElementEditBatchAction(beforeChanged, saved)
    } catch (err) {
      toast({ status: 'error', title: 'Failed to update tags', description: String(err) })
      await refreshElements()
    }
  }, [
    allElements,
    applyElementSaved,
    canEdit,
    handleCreateTag,
    pushElementEditBatchAction,
    refreshElements,
    selectedCanvasElementIds,
    selectedElement,
    tagColors,
    toast,
    viewElements,
  ])

  const pushConnectorEditAction = useCallback((before: Connector, after: Connector) => {
    if (connectorSnapshotsEqual(before, after)) return
    pushEditAction({
      undo: async () => {
        const updated = await api.workspace.connectors.update(before.view_id, before.id, connectorUpdatePayload(before))
        const connector = connectorToConnector(updated)
        upsertConnectorGraphSnapshot(connector)
        upsertStoreConnector(connector)
        publishRealtimeConnectorUpsert(connector)
        setSelectedEdge((current) => current?.id === connector.id ? connector : current)
        await refreshElements()
      },
      redo: async () => {
        const updated = await api.workspace.connectors.update(after.view_id, after.id, connectorUpdatePayload(after))
        const connector = connectorToConnector(updated)
        upsertConnectorGraphSnapshot(connector)
        upsertStoreConnector(connector)
        publishRealtimeConnectorUpsert(connector)
        setSelectedEdge((current) => current?.id === connector.id ? connector : current)
        await refreshElements()
      },
    })
  }, [publishRealtimeConnectorUpsert, pushEditAction, refreshElements, upsertStoreConnector])

  const pushConnectorDeleteAction = useCallback((deleted: Connector) => {
    let activeConnector = deleted
    pushEditAction({
      undo: async () => {
        const created = await api.workspace.connectors.create(deleted.view_id, connectorUpdatePayload(deleted))
        activeConnector = connectorToConnector(created)
        upsertConnectorGraphSnapshot(activeConnector)
        upsertStoreConnector(activeConnector)
        publishRealtimeConnectorUpsert(activeConnector)
        await refreshElements()
      },
      redo: async () => {
        await api.workspace.connectors.delete('', activeConnector.id)
        removeConnectorGraphSnapshot(activeConnector.view_id, activeConnector.id)
        removeStoreConnector(activeConnector.id)
        publishRealtimeConnectorDelete(activeConnector.id)
        setSelectedEdge((current) => current?.id === activeConnector.id ? null : current)
        await refreshElements()
      },
    })
  }, [publishRealtimeConnectorDelete, publishRealtimeConnectorUpsert, pushEditAction, refreshElements, removeStoreConnector, upsertStoreConnector])

  const elementEditSessionRef = useRef<{ before: WorkspaceElement; after: WorkspaceElement | null } | null>(null)
  const finalizeElementEditSession = useCallback(() => {
    const session = elementEditSessionRef.current
    elementEditSessionRef.current = null
    if (session?.after) pushElementEditAction(session.before, session.after)
  }, [pushElementEditAction])

  useEffect(() => {
    if (!elementPanel.isOpen || !selectedElement) return
    const session = elementEditSessionRef.current
    if (!session || session.before.id !== selectedElement.id) {
      if (session?.after) pushElementEditAction(session.before, session.after)
      elementEditSessionRef.current = { before: selectedElement, after: null }
    }
  }, [elementPanel.isOpen, pushElementEditAction, selectedElement])

  const handleElementPanelSave = useCallback((saved: WorkspaceElement) => {
    const session = elementEditSessionRef.current
    if (!session || session.before.id !== saved.id) {
      elementEditSessionRef.current = { before: selectedElement?.id === saved.id ? selectedElement : saved, after: saved }
    } else {
      session.after = saved
    }
    applyElementSaved(saved)
    setSelectedElement(saved)
  }, [applyElementSaved, selectedElement])

  const handleElementPanelClose = useCallback(() => {
    finalizeElementEditSession()
    elementPanel.onClose()
  }, [elementPanel, finalizeElementEditSession])

  const connectorEditSessionRef = useRef<{ before: Connector; after: Connector | null } | null>(null)
  const finalizeConnectorEditSession = useCallback(() => {
    const session = connectorEditSessionRef.current
    connectorEditSessionRef.current = null
    if (session?.after) pushConnectorEditAction(session.before, session.after)
  }, [pushConnectorEditAction])

  useEffect(() => {
    if (!connectorPanel.isOpen || !selectedEdge) return
    const session = connectorEditSessionRef.current
    if (!session || session.before.id !== selectedEdge.id) {
      if (session?.after) pushConnectorEditAction(session.before, session.after)
      connectorEditSessionRef.current = { before: selectedEdge, after: null }
    }
  }, [connectorPanel.isOpen, pushConnectorEditAction, selectedEdge])

  const handleConnectorPanelSave = useCallback((updated: Connector) => {
    const connector = connectorToConnector(updated)
    const session = connectorEditSessionRef.current
    if (!session || session.before.id !== connector.id) {
      connectorEditSessionRef.current = { before: selectedEdge?.id === connector.id ? selectedEdge : connector, after: connector }
    } else {
      session.after = connector
    }
    upsertConnectorGraphSnapshot(connector)
    upsertStoreConnector(connector)
    publishRealtimeConnectorUpsert(connector)
    setSelectedEdge(connector)
  }, [publishRealtimeConnectorUpsert, selectedEdge, upsertStoreConnector])

  const handleConnectorPanelClose = useCallback(() => {
    finalizeConnectorEditSession()
    connectorPanel.onClose()
  }, [connectorPanel, finalizeConnectorEditSession])

  const handleUndoViewEdit = useCallback(async () => {
    try {
      await undoViewEdit()
    } catch (err) {
      toast({ status: 'error', title: 'Undo failed', description: err instanceof Error ? err.message : String(err) })
    }
  }, [undoViewEdit, toast])

  const handleRedoViewEdit = useCallback(async () => {
    try {
      await redoViewEdit()
    } catch (err) {
      toast({ status: 'error', title: 'Redo failed', description: err instanceof Error ? err.message : String(err) })
    }
  }, [redoViewEdit, toast])

  const handleToggleExplorer = useCallback(() => setIsExplorerOpen((v) => !v), [])

  const interactionNodesRef = useRef<RFNode[]>([])
  const { defaultConnectorStyle } = useConnectorStyle()
  const createConnectorStyle = connectorStyleForCreate(defaultConnectorStyle)
  const previewConnectorStyle = connectorStyleForPreview(defaultConnectorStyle)

  // ── Canvas interactions ────────────────────────────────────────────────────
  const canvas = useCanvasInteractions({
    viewId, canEdit,
    interactionSourceId,
    setInteractionSourceId,
    clickConnectMode,
    setClickConnectMode,
    clickConnectCursorPos,
    setClickConnectCursorPos,
    onConnectorCreatePreviewActiveChange: setConnectorCreatePreviewActive,
    drawingMode, isMobileLayout,
    rfNodesRef, interactionNodesRef, rfEdgesRef, viewElementsRef, viewIdRef,
    incomingLinksRef,
    treeDataRef,
    navigateRef,
    containerRef,
    gestureTargetRef: viewEditorCanvasRef,
    interactionSourceIdRef,
    multiConnectionSourceIdsRef,
    hoveredZoomRef, hoverPanLockedUntilRef,
    setViewElements, setConnectors,
    setRfNodes, setRfEdges,
    setLinksMap, setParentLinksMap,
    setHoveredZoom,
    refreshGrid, refreshElements,
    stableOnConnectTo: async (targetElementId: number) => {
      // Inline this is the real implementation, also stored in stableOnConnectToRef
      const sourceId = interactionSourceIdRef.current
      const sourceIds = Array.from(new Set(
        multiConnectionSourceIdsRef.current?.length ? multiConnectionSourceIdsRef.current : sourceId !== null ? [sourceId] : [],
      )).filter((id) => id !== targetElementId)
      const cid = viewIdRef.current
      if (sourceIds.length === 0 || cid === null) return
      interactionSourceIdRef.current = null
      multiConnectionSourceIdsRef.current = null
      const interactionNodes = interactionNodesRef.current.length > 0 ? interactionNodesRef.current : rfNodesRef.current
      const targetNode = interactionNodes.find((n) => n.id === String(targetElementId))
      try {
        for (const nextSourceId of sourceIds) {
          const sourceNode = interactionNodes.find((n) => n.id === String(nextSourceId))
          let finalSourceHandle = 'right'; let finalTargetHandle = 'left'
          if (sourceNode && targetNode) {
            const h = findClosestHandles(sourceNode, targetNode)
            finalSourceHandle = h.sourceHandle; finalTargetHandle = h.targetHandle
          }
          const newConnector = await api.workspace.connectors.create(cid, {
            source_element_id: nextSourceId, target_element_id: targetElementId,
            source_handle: finalSourceHandle, target_handle: finalTargetHandle, direction: 'forward',
            style: createConnectorStyle,
          })
          const connector = connectorToConnector(newConnector)
          upsertConnectorGraphSnapshot(connector)
          upsertStoreConnector(connector)
          publishRealtimeConnectorUpsert(connector)
        }
        handleUnsupportedMutation()
      } catch { /* intentionally empty */ }
    },
    defaultConnectorStyle,
    existingElementIds, linksMapRef, parentLinksMapRef,
    openElementPanel: useCallback(() => openElementPanelRef.current(), []),
    closeElementPanel: useCallback(() => closeElementPanelRef.current(), []),
    openConnectorPanel: useCallback(() => openConnectorPanelRef.current(), []),
    closeConnectorPanel: useCallback(() => closeConnectorPanelRef.current(), []),
    selectedElement, selectedConnector: selectedEdge, connectors,
    layers,
    setSelectedElement,
    setSelectedEdge,
    setSelectedProxyConnectorDetails,
    openProxyConnectorPanel: useCallback(() => openProxyConnectorPanelRef.current(), []),
    closeProxyConnectorPanel: useCallback(() => closeProxyConnectorPanelRef.current(), []),
    handleElementDeleted, handleElementPermanentlyDeleted: handleElementPermanentlyDeletedEverywhere,
    handleConnectorDeleted: useCallback((edgeId: number, ownerViewId?: number) => {
      const vid = ownerViewId ?? viewId
      if (vid != null) removeConnectorGraphSnapshot(vid, edgeId)
      removeStoreConnector(edgeId)
      publishRealtimeConnectorDelete(edgeId)
      void refreshElementsRef.current()
    }, [publishRealtimeConnectorDelete, removeStoreConnector, viewId]),
    onPlacementMoved: pushPlacementMoveAction,
    onPlacementsMoved: pushPlacementMoveBatchAction,
    onElementPositionPreview: handleRealtimeElementPositionPreview,
    onPlacementRemoved: pushPlacementRemoveAction,
    onConnectorSaved: publishRealtimeConnectorUpsert,
    onConnectorUpdated: pushConnectorEditAction,
    onConnectorReconnected: useCallback((connector: Connector) => {
      setSuppressedSelectedConnectorHandleHighlightId(connector.id)
    }, []),
    onConnectorSelected: useCallback(() => {
      setSuppressedSelectedConnectorHandleHighlightId(null)
    }, []),
    onConnectorDeleted: pushConnectorDeleteAction,
    onSelectionRemoveFromView: handleBulkRemoveFromView,
    onUnsupportedMutation: handleUnsupportedMutation,
    handleUpdateTags,
    drawingCanvasRef,
    snapToGrid,
    minZoom: computedMinZoom,
    maxZoom: VIEW_EDITOR_MAX_ZOOM,
    libraryOpen,
    openLibrary: useCallback(() => setLibraryOpen(true), []),
    toggleLibrary: useCallback(() => setLibraryOpen((v) => !v), []),
    toggleExplorer: handleToggleExplorer,
    toggleMarkdown: handleToggleMarkdown,
    onFitView: safeFitView,
    setSnapToGrid,
  })

  // Wire stable placeholders to the real implementations from canvas hook
  useEffect(() => {
    stableOnZoomInRef.current = canvas.stableOnZoomIn
    stableOnZoomOutRef.current = canvas.stableOnZoomOut
    stableOnNavigateToViewRef.current = canvas.stableOnNavigateToView
    stableOnRemoveElementRef.current = canvas.stableOnRemoveElement
    stableOnConnectToRef.current = canvas.stableOnConnectTo
    stableOnInteractionStartRef.current = canvas.stableOnInteractionStart
    stableOnStartHandleReconnectRef.current = canvas.stableOnStartHandleReconnect
    stableOnReconnectPickRef.current = canvas.stableOnReconnectPick
  }, [canvas.stableOnZoomIn, canvas.stableOnZoomOut, canvas.stableOnNavigateToView, canvas.stableOnRemoveElement, canvas.stableOnConnectTo, canvas.stableOnInteractionStart, canvas.stableOnStartHandleReconnect, canvas.stableOnReconnectPick])
  const viewName = view?.name ?? null
  const viewNameById = useMemo(() => buildViewNameById(treeData), [treeData])

  const [expandedAncestorGroups, setExpandedAncestorGroups] = useState<Set<string>>(new Set())
  const [contextNodePositionOverrides, setContextNodePositionOverrides] = useState<Record<string, ContextNodePositionOverride>>({})
  const stableOnToggleAncestorGroup = useCallback((anchorId: string) => {
    setExpandedAncestorGroups((prev) => {
      const next = new Set(prev)
      if (next.has(anchorId)) next.delete(anchorId)
      else next.add(anchorId)
      return next
    })
  }, [])

  const { contextNodes, contextConnectors, hiddenProxyCountsByPair, hiddenProxyDetailsByPair } = useViewContextNeighbours({
    snapshot: effectiveWorkspaceSnapshot,
    settings: crossBranchSettings,
    viewId,
    viewElements,
    rfNodes,
    stableOnNavigateToView: canvas.stableOnNavigateToView,
    contextNodePositionOverrides,
    onSelectContextElement: useCallback((element: WorkspaceElement) => {
      setSelectedEdge(null)
      setSelectedProxyConnectorDetails(null)
      closeConnectorPanelRef.current()
      closeProxyConnectorPanelRef.current()
      setSelectedElement(element)
      openElementPanelRef.current()
    }, []),
    onSelectProxyDetails: useCallback((details: ProxyConnectorDetails) => {
      setSelectedElement(null)
      setSelectedEdge(null)
      closeConnectorPanelRef.current()
      closeElementPanelRef.current()
      setSelectedProxyConnectorDetails(details)
      openProxyConnectorPanelRef.current()
    }, []),
    expandedAncestorGroups,
    onToggleAncestorGroup: stableOnToggleAncestorGroup,
  })

  useEffect(() => {
    setContextNodePositionOverrides({})
  }, [viewId])

  const rfEdgesWithProxyBadges = useMemo(() => {
    if (Object.keys(hiddenProxyCountsByPair).length === 0) return rfEdges

    let changed = false
    const badgePairs = new Set<string>()
    const next = rfEdges.map((edge) => {
      const pairKey = canonicalNodePairKey(edge.source, edge.target)
      const rawProxyBadgeCount = hiddenProxyCountsByPair[pairKey] ?? 0
      const showProxyBadge = rawProxyBadgeCount > 0 && !badgePairs.has(pairKey)
      if (rawProxyBadgeCount > 0) badgePairs.add(pairKey)
      const proxyBadgeCount = showProxyBadge ? rawProxyBadgeCount : 0
      const currentBadgeCount = (edge.data as { proxyBadgeCount?: number } | undefined)?.proxyBadgeCount ?? 0
      const proxyBadgeDetails = showProxyBadge ? (hiddenProxyDetailsByPair[pairKey] ?? null) : null
      const currentBadgeDetails = (edge.data as { proxyBadgeDetails?: ProxyConnectorDetails | null } | undefined)?.proxyBadgeDetails ?? null
      if (proxyBadgeCount === currentBadgeCount && proxyBadgeDetails === currentBadgeDetails) return edge
      changed = true
      return {
        ...edge,
        data: {
          ...(edge.data ?? {}),
          proxyBadgeCount: proxyBadgeCount > 0 ? proxyBadgeCount : undefined,
          proxyBadgeDetails,
          onOpenProxyBadge: (details: ProxyConnectorDetails) => {
            setSelectedElement(null)
            setSelectedEdge(null)
            closeConnectorPanelRef.current()
            closeElementPanelRef.current()
            setSelectedProxyConnectorDetails(details)
            openProxyConnectorPanelRef.current()
          },
        },
      }
    })

    return changed ? next : rfEdges
  }, [hiddenProxyCountsByPair, hiddenProxyDetailsByPair, rfEdges])

  // Keep context nodes in state so React Flow can store measured dimensions.
  // When computed positions change (e.g. main node drag), preserve the previously
  // measured width/height so nodes don't flash hidden while being re-measured.
  const [liveContextNodes, setLiveContextNodes] = useState<RFNode[]>([])
  const liveContextNodesRef = useRef<RFNode[]>([])
  const contextNodeIdsRef = useRef<Set<string>>(new Set())
  liveContextNodesRef.current = liveContextNodes
  interactionNodesRef.current = liveContextNodes.length === 0
    ? rfNodes
    : rfNodes.length === 0
      ? liveContextNodes
      : [...liveContextNodes, ...rfNodes]
  useEffect(() => {
    const nextIds = new Set(contextNodes.map((n) => n.id))
    contextNodeIdsRef.current = nextIds
    setContextNodePositionOverrides((prev) => {
      let changed = false
      const next: Record<string, ContextNodePositionOverride> = {}
      for (const [id, override] of Object.entries(prev)) {
        if (nextIds.has(id)) next[id] = override
        else changed = true
      }
      return changed ? next : prev
    })
    setLiveContextNodes((prev) => reconcileMeasuredContextNodes(prev, contextNodes))
  }, [contextNodes])

  const fadedNodeCacheRef = useRef<WeakMap<RFNode, RFNode>>(new WeakMap())
  const fadedEdgeCacheRef = useRef<WeakMap<RFEdge, RFEdge>>(new WeakMap())

  const pendingElementNode = useMemo((): RFNode | null => {
    const pending = canvas.pendingElement
    if (!pending || viewId === null) return null

    return {
      id: pending.id,
      type: 'elementNode',
      position: pending.position,
      selected: false,
      dragging: pending.dragging,
      draggable: !pending.preview,
      selectable: !pending.preview,
      zIndex: 2000,
      style: pending.preview ? { pointerEvents: 'none' } : undefined,
      data: {
        id: -1,
        view_id: viewId,
        element_id: -1,
        position_x: pending.position.x,
        position_y: pending.position.y,
        name: '',
        description: null,
        kind: null,
        technology: null,
        url: null,
        logo_url: null,
        technology_connectors: [],
        tags: [],
        bypass_noise_gate: false,
        has_view: false,
        view_label: null,
        links: EMPTY_LINKS,
        parentLinks: EMPTY_LINKS,
        parentViewId: null,
        onZoomIn: noopAsync,
        onZoomOut: noopAsync,
        onNavigateToDiagram: noop,
        onSelect: noop,
        onOpenCodePreview: noop,
        onInteractionStart: noop,
        onConnectTo: noopAsync,
        onStartHandleReconnect: undefined,
        onRemove: noopAsync,
        onHoverZoom: noop,
        isZoomHovered: null,
        interactionSourceId: null,
        isClickConnectMode: false,
        tagColors: EMPTY_TAG_COLORS,
        layerHighlightColor: undefined,
        forceShowTagPopup: false,
        isPendingElement: true,
        connectedHandleIds: [],
        selectedHandleIds: [],
        reconnectCandidates: [],
        isConnectorHighlighted: false,
        pendingCreate: pending.preview ? undefined : {
          allElements,
          existingElementIds,
          allowCreate: true,
          getSecondaryLabel: pending.mode === 'connect'
            ? (obj: WorkspaceElement) => placementSummaryByElementId[obj.id] ?? obj.technology ?? null
            : undefined,
          onConfirmNew: canvas.handleConfirmNewElement,
          onConfirmExisting: canvas.handleConfirmExistingElement,
          onCancel: canvas.cancelPendingElement,
        },
      },
    }
  }, [
    allElements,
    canvas.cancelPendingElement,
    canvas.handleConfirmExistingElement,
    canvas.handleConfirmNewElement,
    canvas.pendingElement,
    existingElementIds,
    placementSummaryByElementId,
    viewId,
  ])

  const flowNodes = useMemo(() => {
    const baseNodes = liveContextNodes.length === 0
      ? rfNodes
      : rfNodes.length === 0
        ? liveContextNodes
        : [...liveContextNodes, ...rfNodes]
    const allNodes = pendingElementNode ? [...baseNodes, pendingElementNode] : baseNodes

    let hasNodeSel = false
    const selectedNodeIds = new Set<string>()
    for (const n of allNodes) {
      if (n.selected) { selectedNodeIds.add(n.id); hasNodeSel = true }
    }

    const allEdges = contextConnectors.length === 0
      ? rfEdgesWithProxyBadges
      : rfEdgesWithProxyBadges.length === 0
        ? contextConnectors
        : [...contextConnectors, ...rfEdgesWithProxyBadges]

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
    const isMultiNodeSelection = selectedCanvasElementCount > 1
    const withSelectionMeta = (node: RFNode) => {
      if (!node.selected || node.type !== 'elementNode') return node
      const data = node.data as { isMultiSelected?: boolean } | undefined
      if (data?.isMultiSelected === isMultiNodeSelection) return node
      return { ...node, data: { ...(node.data ?? {}), isMultiSelected: isMultiNodeSelection } }
    }

    return allNodes.map((n) => {
      if (n.id === PENDING_ELEMENT_NODE_ID) return n
      const isHighlighted = selectedNodeIds.has(n.id) || selectedEdgeEndPoints.has(n.id) || neighborNodeIds.has(n.id)
      if (isHighlighted) return withSelectionMeta(n)
      const cached = cache.get(n)
      if (cached) return cached
      const faded: RFNode = {
        ...n,
        style: { ...n.style, opacity: (Number(n.style?.opacity ?? 1)) * 0.2 },
      }
      cache.set(n, faded)
      return faded
    })
  }, [liveContextNodes, rfNodes, pendingElementNode, contextConnectors, rfEdgesWithProxyBadges, selectedCanvasElementCount])

  const pendingPreviewEdges = useMemo((): RFEdge[] => {
    const pending = canvas.pendingElement
    if (!pending || !pendingElementNode || pending.sourceElementIds.length === 0) return []

    return pending.sourceElementIds
      .filter((sourceId) => sourceId !== -1)
      .map((sourceId) => {
        const sourceNode = rfNodes.find((node) => node.id === String(sourceId))
        const handles = sourceNode
          ? findClosestHandles(sourceNode, pendingElementNode)
          : { sourceHandle: DEFAULT_SOURCE_HANDLE_SIDE, targetHandle: DEFAULT_TARGET_HANDLE_SIDE }
        const pendingSourceSide = getLogicalHandleId(pending.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE)
        const previewTargetHandle = pending.sourceHandle && pendingSourceSide
          ? getOppositeHandleSide(pendingSourceSide)
          : handles.targetHandle
        return {
          id: `pending-element-edge-${sourceId}`,
          source: String(sourceId),
          target: pending.id,
          sourceHandle: getCenterVisualHandleId(pending.sourceHandle ?? handles.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE) ?? undefined,
          targetHandle: getCenterVisualHandleId(previewTargetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? undefined,
          type: previewConnectorStyle === 'bezier' ? 'default' : previewConnectorStyle,
          className: 'vieweditor-temporary-connector',
          label: '',
          data: {
            id: -sourceId,
            view_id: viewId ?? -1,
            source_element_id: sourceId,
            target_element_id: -1,
            label: null,
            description: null,
            relationship: null,
            direction: 'forward',
            style: previewConnectorStyle,
            url: null,
            source_handle: pending.sourceHandle ?? handles.sourceHandle,
            target_handle: previewTargetHandle,
            tags: [],
            created_at: '',
            updated_at: '',
          },
          style: TEMPORARY_CONNECTOR_EDGE_STYLE,
          labelStyle: { fontSize: 11, fill: 'var(--accent)', opacity: 1 },
          labelBgStyle: { fill: 'var(--chakra-colors-gray-900)', fillOpacity: 1 },
          markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14, color: 'var(--accent)' },
          zIndex: 1500,
        }
      })
  }, [canvas.pendingElement, pendingElementNode, previewConnectorStyle, rfNodes, viewId])

  const flowEdges = useMemo(() => {
    const baseEdges = contextConnectors.length === 0
      ? rfEdgesWithProxyBadges
      : rfEdgesWithProxyBadges.length === 0
        ? contextConnectors
        : [...contextConnectors, ...rfEdgesWithProxyBadges]
    const allEdges = pendingPreviewEdges.length === 0 ? baseEdges : [...baseEdges, ...pendingPreviewEdges]
    const baseNodes = liveContextNodes.length === 0
      ? rfNodes
      : rfNodes.length === 0
        ? liveContextNodes
        : [...liveContextNodes, ...rfNodes]
    const allNodes = pendingElementNode ? [...baseNodes, pendingElementNode] : baseNodes

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
      if (e.id.startsWith('pending-element-edge-')) return e
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
  }, [contextConnectors, rfEdgesWithProxyBadges, pendingPreviewEdges, liveContextNodes, rfNodes, pendingElementNode])

  // Route onNodesChange: context node changes (dimensions, selection) go to
  // liveContextNodes state; main node changes go to the canvas handler.
  const { onNodesChange: canvasOnNodesChange } = canvas
  const onNodesChange = useCallback((changes: NodeChange[]) => {
    const ctxChanges = changes.filter((c) => 'id' in c && contextNodeIdsRef.current.has((c as { id: string }).id))
    const mainChanges = changes.filter((c) => !('id' in c) || !contextNodeIdsRef.current.has((c as { id: string }).id))
    if (ctxChanges.length > 0) {
      const currentContextNodes = liveContextNodesRef.current
      const bounds = getContextBoundaryBounds(currentContextNodes)
      const adjustedChanges = !bounds
        ? ctxChanges
        : ctxChanges.map((change) => {
          if (change.type !== 'position' || !('position' in change)) return change
          const node = currentContextNodes.find((candidate) => candidate.id === change.id)
          if (!node) return change
          const nextPosition = change.position ?? node.position
          const { position } = clampContextNodeChangePosition(node, nextPosition, bounds, node.position)
          const fixedAbsolutePosition = node.positionAbsolute ?? node.position
          const nextPositionAbsolute = change.positionAbsolute ?? fixedAbsolutePosition
          const { position: positionAbsolute } = clampContextNodeChangePosition(node, nextPositionAbsolute, bounds, fixedAbsolutePosition)
          return { ...change, position, positionAbsolute }
        })

      if (bounds) {
        setContextNodePositionOverrides((prev) => {
          let changed = false
          const next = { ...prev }
          for (const change of adjustedChanges) {
            if (change.type !== 'position' || !('position' in change)) continue
            const node = currentContextNodes.find((candidate) => candidate.id === change.id)
            if (!node) continue
            const nextPosition = change.position ?? node.position
            const { override } = clampContextNodeChangePosition(node, nextPosition, bounds, node.position)
            if (!override) continue
            const previous = next[change.id]
            if (previous?.side === override.side && previous.axisPosition === override.axisPosition) continue
            next[change.id] = override
            changed = true
          }
          return changed ? next : prev
        })
      }

      setLiveContextNodes((nds) => applyNodeChangesWithStructuralSharing(adjustedChanges, nds))
    }
    if (mainChanges.length > 0) {
      canvasOnNodesChange(mainChanges)
    }
  }, [canvasOnNodesChange])

  const {
    canvasMenu, setCanvasMenu,
    pendingElement,
    reconnectPicking, setReconnectPicking, reconnectPickingRef,
    connectorLongPressMenu, setConnectorLongPressMenu,
    lastMousePosRef,
    showAddingElementAt,
    cancelPendingElement,
    onEdgesChange, onNodeDragStart, onNodeDrag, onNodeDragStop,
    onSelectionDragStart, onSelectionDrag, onSelectionDragStop,
    onConnect, onConnectStart, onConnectEnd,
    onReconnect, onReconnectStart, onReconnectEnd,
    onEdgeClick, onEdgeContextMenu, onPaneClick, onPaneContextMenu, onPaneMouseMove,
    onMoveStart, onMove, onMoveEnd,
    onTouchStart, onTouchMove, onTouchEnd,
    onContainerPointerDown, onContainerPointerMove, onContainerPointerUp,
    onDragOver, onDrop, onWheelCapture,
  } = canvas
  const connectionLineStyle = useMemo(() => {
    if (!pendingElement?.preview) return TEMPORARY_CONNECTOR_PATH_STYLE
    return { ...TEMPORARY_CONNECTOR_PATH_STYLE, opacity: 0 }
  }, [pendingElement?.preview])

  const handleRealtimeCanvasMouseMove = useCallback((event: React.MouseEvent) => {
    const flowPos = screenToFlowPositionRef.current({ x: event.clientX, y: event.clientY })
    realtimeRef.current?.sendCursor(flowPos.x, flowPos.y)
  }, [])

  const handleRealtimePaneMouseMove = useCallback((event: React.MouseEvent) => {
    onPaneMouseMove(event)
  }, [onPaneMouseMove])

  const handlePendingPanePointerDownCapture = useCallback((event: React.PointerEvent) => {
    if (!pendingElement || pendingElement.preview || event.button !== 0) return
    const target = event.target
    if (!(target instanceof Element)) return
    if (!target.closest('.react-flow__pane')) return
    if (target.closest('.react-flow__node, .react-flow__edge, .react-flow__handle')) return

    cancelPendingElement()
    event.preventDefault()
    event.stopPropagation()
  }, [cancelPendingElement, pendingElement])

  const handleRealtimeMove = useCallback((event: unknown, viewport: { x: number; y: number; zoom: number }) => {
    onMove(event, viewport)
    canvasViewportStore.setViewport(viewport)
    realtimeRef.current?.sendViewport(viewport.x, viewport.y, viewport.zoom)
  }, [canvasViewportStore, onMove])

  // ── FitView ────────────────────────────────────────────────────────────────
  const fitViewRef = useRef(safeFitView)
  fitViewRef.current = safeFitView
  const [computedTranslateExtent, setComputedTranslateExtent] = useState<[[number, number], [number, number]] | undefined>(undefined)
  const {
    clampedRevealProgress,
    applyDemoRevealViewport,
    disableImportExport,
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
    const mainNodes = rfNodesRef.current
    if (mainNodes.length === 0) {
      if (viewElementsRef.current.length === 0 && view?.id === viewIdRef.current) {
        setInitialViewportReady(true)
      }
      return
    }
    if (!nodesMatchCurrentView(mainNodes, viewElementsRef.current, viewIdRef.current)) return

    const contextFitNodes = crossBranchSettings.enabled
      ? liveContextNodes.filter((node) => node.type === 'contextNeighborNode')
      : []
    const nodes = contextFitNodes.length > 0
      ? [...mainNodes, ...contextFitNodes]
      : mainNodes

    if (!nodes.every((n) => typeof n.width === 'number' && n.width > 0 && typeof n.height === 'number' && n.height > 0)) return

    if (clampedRevealProgress !== null) {
      const ok = applyDemoRevealViewport()
      if (ok) {
        setInitialViewportReady(true)
        if (clampedRevealProgress >= 0.999) needsFitView.current = false
      }
      else if (!ok) setTimeout(() => { if (needsFitView.current) maybeFitView() }, 50)
      return
    }

    const ok = safeFitView({
      nodes: nodes.map((node) => ({ id: node.id })),
      duration: 0,
      padding: VIEW_EDITOR_INITIAL_FIT_PADDING,
      minZoom: computedMinZoom,
      maxZoom: 1,
    })
    if (ok) {
      if (contextFitNodes.length > 0) fittedContextForViewRef.current = viewIdRef.current
      needsFitView.current = false
      setInitialViewportReady(true)
    }
    else setTimeout(() => { if (needsFitView.current) maybeFitView() }, 50)
  }, [
    applyDemoRevealViewport,
    clampedRevealProgress,
    computedMinZoom,
    crossBranchSettings.enabled,
    liveContextNodes,
    safeFitView,
    view,
    rfNodesRef,
    viewElementsRef,
    viewIdRef,
  ])

  const onRFInit = useCallback(() => {
    rfReadyRef.current = true
    canvasViewportStore.setViewport(getViewport())
    maybeFitView()
  }, [canvasViewportStore, getViewport, maybeFitView])

  useEffect(() => {
    needsFitView.current = true
    fittedContextForViewRef.current = null
    setInitialViewportReady(false)
  }, [viewId])

  useEffect(() => { maybeFitView() }, [rfNodes, liveContextNodes, maybeFitView])

  useEffect(() => {
    if (!crossBranchSettings.enabled || viewId == null) return
    if (fittedContextForViewRef.current === viewId) return
    if (!liveContextNodes.some((node) => node.type === 'contextNeighborNode')) return
    needsFitView.current = true
    maybeFitView()
  }, [crossBranchSettings.enabled, liveContextNodes, maybeFitView, viewId])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver(() => { if (needsFitView.current) maybeFitView() })
    observer.observe(el)
    return () => observer.disconnect()
  }, [maybeFitView])

  useEffect(() => {
    setSelectedElement(null)
    setSelectedEdge(null)
    setSelectedProxyConnectorDetails(null)
    elementEditSessionRef.current = null
    connectorEditSessionRef.current = null
    clearEditHistory()
    closeElementPanelRef.current()
    closeConnectorPanelRef.current()
    closeProxyConnectorPanelRef.current()
  }, [clearEditHistory, viewId])

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

  // ── Keyboard shortcuts for view edits ─────────────────────────────────────
  useEffect(() => {
    if (drawingMode) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.defaultPrevented || isEditableKeyboardTarget(e.target) || !isCanvasKeyboardTarget(e.target)) return

      const key = e.key.toLowerCase()
      const isModifierShortcut = e.metaKey || e.ctrlKey
      const isRedoShortcut = isModifierShortcut && !e.altKey && (
        (key === 'z' && e.shiftKey) ||
        (key === 'y' && !e.metaKey)
      )
      const isUndoShortcut = isModifierShortcut && !e.altKey && key === 'z' && !e.shiftKey

      if (isRedoShortcut && canRedoViewEdit) {
        e.preventDefault()
        void handleRedoViewEdit()
        return
      }

      if (isUndoShortcut && canUndoViewEdit) {
        e.preventDefault()
        void handleUndoViewEdit()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [
    canRedoViewEdit,
    canUndoViewEdit,
    drawingMode,
    handleRedoViewEdit,
    handleUndoViewEdit,
  ])

  // ── V shortcut to open View Details ────────────────────────────────────────
  useEffect(() => {
    if (drawingMode) return
    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable) return
      if (e.ctrlKey || e.altKey || e.metaKey) return
      if (e.key.toLowerCase() === 'v') {
        e.preventDefault()
        openViewDetailsRef.current()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [drawingMode])

  // ── Overscroll prevention ──────────────────────────────────────────────────
  useEffect(() => {
    const html = document.documentElement
    const prev = html.style.overscrollBehaviorX
    html.style.overscrollBehaviorX = 'none'
    return () => { html.style.overscrollBehaviorX = prev }
  }, [])

  useEffect(() => {
    if (typeof window === 'undefined') return

    const root = document.documentElement
    const target = containerRef.current
    if (!target) {
      root.style.removeProperty(VIEW_EDITOR_TOPBAR_NOTCH_LEFT_VAR)
      return
    }

    const updateTopbarNotchPosition = () => {
      const rect = target.getBoundingClientRect()
      root.style.setProperty(VIEW_EDITOR_TOPBAR_NOTCH_LEFT_VAR, `${rect.left + (rect.width / 2)}px`)
    }

    updateTopbarNotchPosition()

    const resizeObserver = typeof ResizeObserver !== 'undefined'
      ? new ResizeObserver(() => updateTopbarNotchPosition())
      : null

    resizeObserver?.observe(target)
    window.addEventListener('resize', updateTopbarNotchPosition)

    return () => {
      resizeObserver?.disconnect()
      window.removeEventListener('resize', updateTopbarNotchPosition)
      root.style.removeProperty(VIEW_EDITOR_TOPBAR_NOTCH_LEFT_VAR)
    }
  }, [])

  useEffect(() => {
    setHeader({
      node: null,
      collaboration: {
        ...collaboration,
        onAvatarClick: handleCollaborationAvatarClick,
      },
    })
    return () => setHeader(null)
  }, [collaboration, handleCollaborationAvatarClick, setHeader])
  // ── Share ──────────────────────────────────────────────────────────────────
  const onShare = useCallback(() => { }, [])

  const handleExplorerHoverZoom = useCallback((elementId: number | null, type: 'in' | 'out' | null) => {
    setHoveredZoom(type && elementId ? { elementId, type } : null)
  }, [])
  const handleCloseLibrary = useCallback(() => setLibraryOpen(false), [])
  const handleCreateNewLibraryRef = useRef<() => void>(() => { })
  const handleCreateNewLibrary = useCallback(() => handleCreateNewLibraryRef.current(), [])
  const handleFocusModeChange = useCallback((v: boolean) => setCrossBranchEnabled(!v), [setCrossBranchEnabled])
  const handleOpenExport = useCallback(() => exportModal.onOpen(), [exportModal])
  const handleConnectorDeleted = useCallback((edgeId: number, ownerViewId?: number) => {
    const vid = ownerViewId ?? viewId
    if (vid != null) removeConnectorGraphSnapshot(vid, edgeId)
    removeStoreConnector(edgeId)
    publishRealtimeConnectorDelete(edgeId)
    void refreshElements()
  }, [publishRealtimeConnectorDelete, refreshElements, removeStoreConnector, viewId])

  const handleOpenMerge = useCallback((elementId: number) => {
    const el = allElements.find((e) => e.id === elementId)
      ?? (() => {
        const placed = viewElements.find((e) => e.element_id === elementId)
        return placed ? placedElementToLibraryElement(placed) : null
      })()
    if (el) {
      setMergeSourceElement(el)
      mergeDialog.onOpen()
    }
  }, [allElements, viewElements, mergeDialog])

  const handleMerge = useCallback(async (survivorId: number, resolved: {
    kind: string | null
    description: string | null
    repo: string | null
    branch: string | null
    file_path: string | null
    language: string | null
  }) => {
    if (!mergeSourceElement) return
    const result = await api.elements.merge(mergeSourceElement.id, survivorId, resolved)
    mergeElementsInto(mergeSourceElement.id, result.survivor)
    await refreshElements()
    mergeDialog.onClose()
    setMergeSourceElement(null)
    if (selectedElement?.id === mergeSourceElement.id) {
      setSelectedElement(result.survivor)
    }
  }, [mergeSourceElement, mergeElementsInto, mergeDialog, selectedElement, refreshElements])

  const handleConnectorDeleteInPanel = useCallback((edgeId: number, ownerViewId?: number) => {
    const deleted = selectedEdge?.id === edgeId ? selectedEdge : connectors.find((connector) => connector.id === edgeId) ?? null
    connectorEditSessionRef.current = null
    if (deleted) pushConnectorDeleteAction(deleted)
    handleConnectorDeleted(edgeId, ownerViewId)
    setSelectedEdge(null)
  }, [connectors, handleConnectorDeleted, pushConnectorDeleteAction, selectedEdge, setSelectedEdge])
  const handleViewSave = useCallback((updated: ViewTreeNode) => {
    if (view) {
      pushViewEditAction(
        { id: view.id, name: view.name, level_label: view.level_label },
        { id: updated.id, name: updated.name, level_label: updated.level_label },
      )
    }
    setView(updated)
    void refreshElements()
  }, [pushViewEditAction, setView, view, refreshElements])

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
  const handleCopyMermaidDirect = useCallback(async () => {
    setCanvasMenu(null)
    if (viewId === null) return
    try {
      const exported = await api.mermaid.exportView(viewId, { includeTldMetadata: true, markdownBlock: true })
      if (!exported.markdown) return
      await copyTextToClipboard(exported.markdown)
      toast({ status: 'success', title: 'Copied Mermaid block', description: 'Markdown Mermaid block copied to clipboard.' })
    } catch {
      toast({ status: 'error', title: 'Copy failed', description: 'Could not write Mermaid block to the clipboard.' })
    }
  }, [setCanvasMenu, toast, viewId])

  const handleBulkCopyMermaid = useCallback(async () => {
    if (viewId === null) return
    try {
      const exported = await api.mermaid.exportView(viewId, { includeTldMetadata: true, markdownBlock: true })
      if (!exported.markdown) return
      await copyTextToClipboard(exported.markdown)
      toast({ status: 'success', title: 'Copied Mermaid block', description: 'Markdown Mermaid block copied to clipboard.' })
    } catch {
      toast({ status: 'error', title: 'Copy failed', description: 'Could not write Mermaid block to the clipboard.' })
    }
  }, [toast, viewId])

  const handleExportMermaidDirect = useCallback(async () => {
    if (viewId === null) return
    const baseName = sanitizeExportFilename(viewName || 'view-export')
    const downloadName = `${baseName}.md`
    try {
      setIsExporting(true)
      const exported = await api.mermaid.exportView(viewId, { includeTldMetadata: true, markdownBlock: true })
      const result = await triggerBlobDownload(new Blob([exported.markdown], { type: 'text/markdown;charset=utf-8' }), downloadName, 'markdown')
      if (result.canceled) return
      toast({ status: 'success', title: 'Export complete', description: `Saved ${downloadName}` })
    } catch {
      toast({ status: 'error', title: 'Export failed', description: 'Please try again.' })
    } finally {
      setIsExporting(false)
    }
  }, [toast, viewId, viewName])

  const handleExportView = useCallback(async (options: ExportOptions) => {
    const baseName = sanitizeExportFilename(options.filename || viewName || 'view-export')
    const downloadName = options.format === 'mermaid' ? `${baseName}.md` : `${baseName}.${options.format}`
    const filterNode = (node: HTMLElement) => {
      const cn = node.className
      if (typeof cn !== 'string') return true
      return !cn.includes('react-flow__controls') && !cn.includes('react-flow__panel')
    }
    try {
      setIsExporting(true)
      if (options.format === 'mermaid') {
        if (viewId === null) return
        const exported = await api.mermaid.exportView(viewId, {
          includeTldMetadata: options.includeTldMetadata ?? true,
          markdownBlock: true,
        })
        const result = await triggerBlobDownload(new Blob([exported.markdown], { type: 'text/markdown;charset=utf-8' }), downloadName, 'markdown')
        if (result.canceled) return
      } else {
        const flowRoot = containerRef.current?.querySelector('.react-flow') as HTMLElement | null
        if (!flowRoot) { toast({ status: 'error', title: 'Export failed', description: 'Could not find the view canvas.' }); return }
        if (options.format === 'svg') {
          const result = await triggerDownload(await toSvg(flowRoot, { cacheBust: true, filter: filterNode }), downloadName, options.format)
          if (result.canceled) return
        } else {
          const result = await triggerDownload(await toPng(flowRoot, { cacheBust: true, pixelRatio: options.scale, filter: filterNode }), downloadName, options.format)
          if (result.canceled) return
        }
      }
      closeExportModalRef.current()
      toast({ status: 'success', title: 'Export complete', description: `Saved ${downloadName}` })
    } catch {
      toast({ status: 'error', title: 'Export failed', description: 'Please try again.' })
    } finally { setIsExporting(false) }
  }, [toast, viewId, viewName])

  const getClipboardPasteCenter = useCallback(() => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }

    const mouse = lastMousePosRef.current
    const screenPoint = mouse &&
      mouse.clientX >= rect.left &&
      mouse.clientX <= rect.right &&
      mouse.clientY >= rect.top &&
      mouse.clientY <= rect.bottom
      ? { x: mouse.clientX, y: mouse.clientY }
      : { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }

    return screenToFlowPositionRef.current(screenPoint)
  }, [lastMousePosRef])

  const getCanvasCenter = useCallback(() => {
    const rect = containerRef.current?.getBoundingClientRect()
    return rect
      ? screenToFlowPositionRef.current({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 })
      : { x: 0, y: 0 }
  }, [])

  const runMermaidImport = useCallback(async (
    parsed: ParsedImport,
    currentViewId: number,
    center: { x: number; y: number },
    options: {
      successTitle: string
      failureTitle: string
      onSuccess?: () => void
    },
  ) => {
    try {
      const result = await api.mermaid.importIntoView(currentViewId, parsed.source, center)
      const { summary } = result
      clearEditHistory()
      await refreshElements()
      pendingPasteSelectionRef.current = {
        viewId: currentViewId,
        elementIds: summary.importedElementIds,
      }
      options.onSuccess?.()
      toast({
        status: 'success',
        title: options.successTitle,
        description: [mermaidImportDescription(summary), ...result.warnings.slice(0, 1)].join(' '),
        duration: 5000,
        isClosable: true,
      })
      return true
    } catch (err) {
      await refreshElements()
      toast({ status: 'error', title: options.failureTitle, description: err instanceof Error ? err.message : String(err) })
      return false
    }
  }, [clearEditHistory, refreshElements, toast])

  const handleImportMarkdownMermaidBlock = useCallback(async (source: string) => {
    if (!canEdit) return
    const currentViewId = viewIdRef.current
    if (!currentViewId) return

    await runMermaidImport({ format: 'mermaid', source }, currentViewId, getCanvasCenter(), {
      successTitle: 'Imported Mermaid block',
      failureTitle: 'Mermaid import failed',
    })
  }, [canEdit, getCanvasCenter, runMermaidImport, viewIdRef])

  const pasteViewSelectionPayload = useCallback(async (
    payload: ViewSelectionClipboardPayload,
    targetViewId: number,
    pasteCenter: { x: number; y: number },
    duplicateElementIds: ReadonlySet<number>,
  ) => {
    if (!canEdit || isPasteImportingRef.current) return

    isPasteImportingRef.current = true
    setIsClipboardPasting(true)
    try {
      const duplicatedElementIdsBySourceId = new Map<number, number>()
      const duplicateElements = payload.elements.filter((element) => duplicateElementIds.has(element.elementId))
      const duplicated = await Promise.all(duplicateElements.map(async (element) => {
        const created = await api.elements.create({
          name: element.name,
          description: element.description ?? '',
          kind: element.kind ?? '',
          technology: element.technology ?? '',
          url: element.url ?? '',
          logo_url: element.logo_url ?? '',
          technology_connectors: element.technology_connectors,
          tags: element.tags,
          repo: element.repo,
          branch: element.branch,
          file_path: element.file_path,
          language: element.language,
          bypass_noise_gate: element.bypass_noise_gate,
        })
        return { sourceElementId: element.elementId, targetElementId: created.id }
      }))
      duplicated.forEach(({ sourceElementId, targetElementId }) => {
        duplicatedElementIdsBySourceId.set(sourceElementId, targetElementId)
      })

      const targetElementIdsBySourceId = mapViewSelectionElementIds(payload, duplicatedElementIdsBySourceId)
      const placementPlan = planViewSelectionPastePlacements(payload, targetElementIdsBySourceId, pasteCenter)
      const connectorPlan = planViewSelectionPasteConnectors(payload, targetElementIdsBySourceId)

      await Promise.all(placementPlan.map((placement) =>
        api.workspace.views.placements.add(targetViewId, placement.elementId, placement.x, placement.y)
      ))
      const createdConnectors = await Promise.all(connectorPlan.map((connector) =>
        api.workspace.connectors.create(targetViewId, {
          source_element_id: connector.sourceElementId,
          target_element_id: connector.targetElementId,
          label: connector.label ?? '',
          description: connector.description ?? '',
          relationship: connector.relationship ?? '',
          direction: connector.direction,
          style: connector.style,
          url: connector.url ?? '',
          source_handle: connector.source_handle,
          target_handle: connector.target_handle,
          tags: connector.tags,
        })
      ))
      createdConnectors.map(connectorToConnector).forEach(publishRealtimeConnectorUpsert)

      await refreshElements()
      pendingPasteSelectionRef.current = {
        viewId: targetViewId,
        elementIds: new Set(placementPlan.map((placement) => placement.elementId)),
      }
      toast({
        status: 'success',
        title: 'Pasted selection',
        description: `Placed ${placementPlan.length} element${placementPlan.length === 1 ? '' : 's'}.`,
      })
    } catch (err) {
      await refreshElements()
      toast({ status: 'error', title: 'Paste failed', description: err instanceof Error ? err.message : String(err) })
    } finally {
      isPasteImportingRef.current = false
      setIsClipboardPasting(false)
    }
  }, [canEdit, publishRealtimeConnectorUpsert, refreshElements, toast])

  const handleCopyCutViewSelection = useCallback((event: ClipboardEvent) => {
    if (isEditableKeyboardTarget(event.target) || !isCanvasKeyboardTarget(event.target) || drawingMode || textEditorState || pendingElement) return
    if (event.type === 'cut' && !canEdit) return

    const currentViewId = viewIdRef.current
    if (!currentViewId || !event.clipboardData) return

    const payload = buildViewSelectionClipboardPayload(
      currentViewId,
      viewElements,
      connectors,
      selectedCanvasElementIds,
    )
    if (!payload) return

    event.preventDefault()
    event.clipboardData.clearData()
    event.clipboardData.setData(VIEW_SELECTION_CLIPBOARD_MIME, serializeViewSelectionClipboardPayload(payload))

    if (event.type === 'cut') {
      void handleBulkRemoveFromView()
    }
  }, [
    canEdit,
    connectors,
    drawingMode,
    handleBulkRemoveFromView,
    pendingElement,
    selectedCanvasElementIds,
    textEditorState,
    viewElements,
    viewIdRef,
  ])

  useEffect(() => {
    window.addEventListener('copy', handleCopyCutViewSelection)
    window.addEventListener('cut', handleCopyCutViewSelection)
    return () => {
      window.removeEventListener('copy', handleCopyCutViewSelection)
      window.removeEventListener('cut', handleCopyCutViewSelection)
    }
  }, [handleCopyCutViewSelection])

  const handlePasteFromClipboard = useCallback(async (event: ClipboardEvent) => {
    if (!canEdit || isPasteImportingRef.current || textEditorState || pendingElement) return
    if (isEditableKeyboardTarget(event.target) || !isCanvasKeyboardTarget(event.target)) return

    const clipboardData = event.clipboardData
    const customRaw = clipboardData?.getData(VIEW_SELECTION_CLIPBOARD_MIME) ?? ''
    if (customRaw) {
      const payload = parseViewSelectionClipboardPayload(customRaw)
      event.preventDefault()
      if (!payload) {
        toast({ status: 'error', title: 'Paste failed', description: 'Clipboard data is not compatible with this version.' })
        return
      }

      const currentViewId = viewIdRef.current
      if (!currentViewId) return

      const pasteCenter = getClipboardPasteCenter()
      const existingIds = new Set(viewElementsRef.current.map((element) => element.element_id))
      const conflictElementIds = findViewSelectionPasteConflicts(payload, existingIds)
      if (conflictElementIds.length > 0) {
        setPendingDuplicatePaste({
          payload,
          targetViewId: currentViewId,
          pasteCenter,
          conflictElementIds,
        })
        duplicatePasteConfirm.onOpen()
        return
      }

      await pasteViewSelectionPayload(payload, currentViewId, pasteCenter, new Set())
      return
    }

    const mermaidText = event.clipboardData?.getData('text/plain') ?? ''
    if (!mermaidIntegrationEnabled || !mermaidText.trim()) return

    const currentViewId = viewIdRef.current
    if (!currentViewId) return

    isPasteImportingRef.current = true
    try {
      const parsed = await api.mermaid.parse(mermaidText)
      event.preventDefault()
      await runMermaidImport(parsed, currentViewId, getCanvasCenter(), {
        successTitle: 'Mermaid imported',
        failureTitle: 'Mermaid import failed',
      })
    } catch {
      return
    } finally {
      isPasteImportingRef.current = false
    }
  }, [
    canEdit,
    duplicatePasteConfirm,
    getCanvasCenter,
    getClipboardPasteCenter,
    mermaidIntegrationEnabled,
    pasteViewSelectionPayload,
    pendingElement,
    runMermaidImport,
    textEditorState,
    toast,
    viewElementsRef,
    viewIdRef,
  ])

  useEffect(() => {
    window.addEventListener('paste', handlePasteFromClipboard)
    return () => window.removeEventListener('paste', handlePasteFromClipboard)
  }, [handlePasteFromClipboard])

  const handleCancelDuplicatePaste = useCallback(() => {
    setPendingDuplicatePaste(null)
    duplicatePasteConfirm.onClose()
  }, [duplicatePasteConfirm])

  const handleConfirmDuplicatePaste = useCallback(() => {
    const pending = pendingDuplicatePaste
    if (!pending) return
    setPendingDuplicatePaste(null)
    duplicatePasteConfirm.onClose()
    void pasteViewSelectionPayload(
      pending.payload,
      pending.targetViewId,
      pending.pasteCenter,
      new Set(pending.conflictElementIds),
    )
  }, [duplicatePasteConfirm, pasteViewSelectionPayload, pendingDuplicatePaste])

  const handleImportView = useCallback(async (parsed: ParsedImport) => {
    const currentViewId = viewIdRef.current
    if (!currentViewId) return
    setIsImporting(true)
    try {
      if (parsed.format === 'mermaid') {
        await runMermaidImport(parsed, currentViewId, getCanvasCenter(), {
          successTitle: 'Import complete',
          failureTitle: 'Import failed',
          onSuccess: () => closeImportModalRef.current(),
        })
        return
      }

      const res = await api.import.resources('', { elements: parsed.elements, connectors: parsed.connectors })
      clearEditHistory()
      closeImportModalRef.current()
      toast({ status: 'success', title: 'Import complete', description: `Created ${parsed.elements.length} elements and ${parsed.connectors.length} connectors.`, duration: 5000, isClosable: true })
      if (res.view_id && res.view_id !== currentViewId) navigate(`/views/${res.view_id}`)
      else window.location.reload()
    } catch (e) {
      toast({ status: 'error', title: 'Import failed', description: e instanceof Error ? e.message : 'Unknown error' })
    } finally { setIsImporting(false) }
  }, [clearEditHistory, getCanvasCenter, navigate, runMermaidImport, toast, viewIdRef])

  // ─────────────────────────────────────────────────────────────────────────────
  // Render states
  // ─────────────────────────────────────────────────────────────────────────────
  if (view === undefined) {
    return <Flex h="100%" align="center" justify="center"><Spinner size="xl" /></Flex>
  }
  if (view === null) {
    return <Flex h="100%" align="center" justify="center"><Text>View not found.</Text></Flex>
  }

  const resolvedElementPanelAfterContentSlot = elementPanelAfterContentSlot ?? (
    <ElementPanelCollaboration
      element={selectedElement}
      liveThreadUpsert={liveThreadUpsert}
      liveThreadResolve={liveThreadResolve}
      liveCommentCreate={liveCommentCreate}
      liveReactions={liveReactions}
    />
  )
  const resolvedConnectorPanelAfterContentSlot = connectorPanelAfterContentSlot ?? (
    <ConnectorPanelCollaboration
      connector={selectedEdge}
      liveThreadUpsert={liveThreadUpsert}
      liveThreadResolve={liveThreadResolve}
      liveCommentCreate={liveCommentCreate}
    />
  )

  return (
    <ViewEditorContext.Provider value={{
      viewId, canEdit, isOwner, isFreePlan, snapToGrid, setSnapToGrid,
      selectedElement, selectedConnector: selectedEdge,
      isMarkdownOpen, markdownPaneWidth
    }}>
      <Box h="100%" display="flex" flexDir="column">
        <Flex ref={editorSplitRef} flex={1} overflow="hidden">
          <Box
            ref={containerRef}
            flex="1 1 auto"
            minW={0}
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
              <VStack
                position="absolute"
                top="50%"
                left={libraryOpen ? '328px' : 3}
                transform="translateY(-50%)"
                spacing={1}
                zIndex={1200}
                transition="left 0.2s cubic-bezier(0.25, 0.46, 0.45, 0.94)"
              >
                <Tooltip label={libraryOpen ? 'Close element library' : 'Open element library'} placement="right" openDelay={300}>
                  <IconButton
                    data-testid="vieweditor-toggle-library"
                    aria-label={libraryOpen ? 'Close element library' : 'Open element library'}
                    icon={libraryOpen ? <ChevronLeftIcon size={16} strokeWidth={3.5} /> : <LibraryIcon size={18} />}
                    size="md"
                    transition="transform 0.15s ease"
                    border='1px solid rgba(255, 255, 255, 0.08)'
                    variant="clay" colorScheme="gray" bg="var(--bg-panel)"
                    color={libraryOpen ? 'white' : 'gray.300'}
                    _hover={{ bg: 'var(--bg-card-solid)', transform: 'scale(1.1)', color: 'white' }}
                    onClick={() => setLibraryOpen((v) => !v)}
                  />
                </Tooltip>
                <KbdHint ml={0} opacity={0.6}>A</KbdHint>
              </VStack>
            )}

            {/* Explorer toggle */}
            {!isMobileLayout && !elementPanel.isOpen && !connectorPanel.isOpen && !viewDetails.isOpen && (
              <VStack
                position="absolute"
                top="50%"
                right={isExplorerOpen ? '328px' : 3}
                transform="translateY(-50%)"
                spacing={1}
                zIndex={5}
                transition="right 0.2s cubic-bezier(0.25, 0.46, 0.45, 0.94)"
              >
                <Tooltip label={isExplorerOpen ? 'Close view explorer' : 'Open view explorer'} placement="left" openDelay={300}>
                  <IconButton
                    data-testid="vieweditor-toggle-explorer"
                    aria-label={isExplorerOpen ? 'Close view explorer' : 'Open view explorer'}
                    icon={isExplorerOpen ? <ChevronRightIcon size={16} strokeWidth={3.5} /> : <NavigationIcon size={18} />}
                    size="md"
                    transition="transform 0.15s ease"
                    border="1px solid rgba(255, 255, 255, 0.08)"
                    variant="clay" colorScheme="gray" bg="var(--bg-panel)"
                    color={isExplorerOpen ? 'white' : 'gray.300'}
                    _hover={{ bg: 'var(--bg-card-solid)', transform: 'scale(1.1)', color: 'white' }}
                    onClick={() => setIsExplorerOpen((v) => !v)}
                  />
                </Tooltip>
                <KbdHint ml={0} opacity={0.6}>D</KbdHint>
              </VStack>
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
              ref={viewEditorCanvasRef}
              data-testid="vieweditor-canvas"
              data-connector-preview-active={isConnectorCreatePreviewActive || !!canvas.handleReconnectDrag || !!clickConnectMode || interactionSourceId !== null ? 'true' : undefined}
              position="relative"
              w="full"
              h="full"
              onWheelCapture={onWheelCapture}
              onPointerDownCapture={handlePendingPanePointerDownCapture}
              onMouseMove={handleRealtimeCanvasMouseMove}
              onTouchStart={onTouchStart}
              onTouchMove={onTouchMove}
              onTouchEnd={onTouchEnd}
              sx={{
                '.react-flow__nodes, .react-flow__edges, .react-flow__edgelabel-renderer': {
                  opacity: initialViewportReady ? 1 : 0,
                  pointerEvents: initialViewportReady ? undefined : 'none',
                  transition: initialViewportReady ? 'opacity 80ms ease-out' : 'none',
                },
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
                onSelectionDragStart={onSelectionDragStart} onSelectionDrag={onSelectionDrag} onSelectionDragStop={onSelectionDragStop}
                onEdgeClick={onEdgeClick} onEdgeContextMenu={onEdgeContextMenu}
                onPaneContextMenu={onPaneContextMenu} onPaneClick={onPaneClick}
                onPaneMouseMove={handleRealtimePaneMouseMove}
                onMoveStart={onMoveStart} onMove={handleRealtimeMove} onMoveEnd={onMoveEnd}
                translateExtent={computedTranslateExtent} nodeExtent={computedTranslateExtent} minZoom={computedMinZoom} maxZoom={VIEW_EDITOR_MAX_ZOOM}
                onReconnect={onReconnect} onReconnectStart={onReconnectStart} onReconnectEnd={onReconnectEnd}
                connectionLineStyle={connectionLineStyle}
                connectionLineType={previewConnectorStyle as ConnectionLineType}
                connectionLineComponent={ViewConnectorConnectionLine}
                nodeTypes={nodeTypesMemo} edgeTypes={edgeTypesMemo}
                nodesDraggable={canEdit} connectionMode={ConnectionMode.Loose} connectionRadius={25}
                edgesUpdatable={canEdit} reconnectRadius={0}
                snapToGrid={snapToGrid}
                snapGrid={SNAP_GRID}
                deleteKeyCode={null}
                connectOnClick={false}
                onlyRenderVisibleElements
                autoPanOnNodeDrag={false}
                selectionOnDrag={canEdit && !drawingMode}
                panOnDrag={drawingMode ? false : canEdit ? [1, 2] : true}
                panOnScroll={enableCanvasWheelPan} panOnScrollSpeed={1.2} panOnScrollMode={PanOnScrollMode.Free}
                zoomOnScroll={false} zoomOnPinch
              >
                <SafeBackground variant={BackgroundVariant.Dots} gap={16} color="#2D3748" size={1} />
              </ReactFlow>
              {remoteCursors.length > 0 && (
                <RemoteCursorOverlay cursors={remoteCursors} viewportStore={canvasViewportStore} />
              )}
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
              noFocusLock={!!pendingElement || !!textEditorState}
            />

            <EditorOverlays
              clickConnectMode={clickConnectMode}
              clickConnectCursorPos={clickConnectCursorPos}
              connectorRouteStyle={previewConnectorStyle}
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

            {textEditorState && (
              <Box
                position="absolute"
                left={textEditorState.canvasX}
                top={textEditorState.canvasY}
                transform="translate(-50%, -50%)"
                zIndex={2000}
              >
                <Input
                  data-testid="drawing-text-input"
                  autoFocus
                  variant="unstyled"
                  bg="var(--bg-panel)"
                  border="2px solid"
                  borderColor="var(--accent)"
                  rounded="md"
                  px={2}
                  py={1}
                  color={drawingColor}
                  fontSize={`${Math.max(14, drawingWidth * 5)}px`}
                  fontWeight="bold"
                  onBlur={(e: React.FocusEvent<HTMLInputElement>) => commitDrawingText(e.target.value, textEditorState)}
                  onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                    if (e.key === 'Enter') commitDrawingText(e.currentTarget.value, textEditorState)
                    if (e.key === 'Escape') setTextEditorState(null)
                  }}
                />
              </Box>
            )}

            <DrawingCanvas
              ref={drawingCanvasRef}
              paths={drawingPaths}
              isDrawing={drawingMode} isVisible={drawingVisible}
              strokeColor={drawingColor} strokeWidth={drawingWidth} mode={drawingTool}
              onPathComplete={handleRealtimePathComplete} onPathDelete={handleRealtimePathDelete} onPathUpdate={handleRealtimePathUpdate}
              onTextPositionSelected={(canvasX, canvasY, flowX, flowY) => setTextEditorState({ canvasX, canvasY, flowX, flowY })}
            />



            <ConnectorContextMenu
              menu={connectorLongPressMenu}
              onEdit={(edgeId) => { const connector = connectors.find((e) => e.id === edgeId); if (connector) { setSelectedEdge(connector); connectorPanel.onOpen() }; setConnectorLongPressMenu(null) }}
              onMoveSource={(edgeId) => { const picking = { edgeId, endpoint: 'source' as const }; reconnectPickingRef.current = picking; setReconnectPicking(picking); setConnectorLongPressMenu(null) }}
              onMoveTarget={(edgeId) => { const picking = { edgeId, endpoint: 'target' as const }; reconnectPickingRef.current = picking; setReconnectPicking(picking); setConnectorLongPressMenu(null) }}
              onDelete={async (edgeId) => {
                setConnectorLongPressMenu(null)
                if (!viewId) return
                try {
                  await api.workspace.connectors.delete('', edgeId)
                  handleConnectorDeleted(edgeId, viewId)
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

            {/* Overlap suggestion banner */}
            {hasSignificantOverlaps && !reconnectPicking && (
              <Box position="absolute" top="14px" left="50%" transform="translateX(-50%)" zIndex={2000}
                bg="blue.900" border="1px" borderColor="blue.700" px={4} py={2} rounded="xl" shadow="xl"
                display="flex" alignItems="center" gap={3} transition="all 0.2s"
                _hover={{ bg: 'blue.800', transform: 'translateX(-50%) translateY(-1px)' }}>
                <Text fontSize="sm" fontWeight="bold" color="blue.100">Elements are overlapping</Text>
                <Button
                  size="xs" colorScheme="blue" variant="solid"
                  isLoading={adjustingLayout}
                  onClick={async (e) => {
                    e.stopPropagation()
                    if (!viewId) return
                    setAdjustingLayout(true)
                    try {
                      await removeCollisions(viewId)
                      window.location.reload()
                    } catch {
                      setAdjustingLayout(false)
                    }
                  }}
                >
                  Layout
                </Button>
                <IconButton
                  aria-label="Dismiss" icon={<CloseButton size="sm" />}
                  size="xs" variant="ghost" colorScheme="blue"
                  onClick={(e) => { e.stopPropagation(); dismissOverlapSuggestion() }}
                />
              </Box>
            )}

            <CanvasContextMenu
              menu={canvasMenu}
              onAddElement={(x, y) => {
                const rect = containerRef.current?.getBoundingClientRect()
                if (rect) showAddingElementAt(x + rect.left, y + rect.top)
                setCanvasMenu(null)
              }}
              onCopyMermaid={handleCopyMermaidDirect}
            />

            <EmptyCanvasState isMobile={isMobileLayout} hasNodes={rfNodes.length > 0 || !!pendingElement} />

            <SelectionBulkBar
              count={drawingMode ? 0 : selectedCanvasElementIds.length}
              availableTags={availableTags}
              selectedTagCounts={selectedCanvasTagCounts}
              tagColors={tagColors}
              mergeOptions={selectedCanvasMergeOptions}
              mergeLoadingId={bulkMergeLoadingId}
              onAlign={handleSelectionAlign}
              onDistribute={handleSelectionDistribute}
              onFitSelection={handleFitSelection}
              onAddTag={(tag) => { void handleBulkTagChange(tag, 'add') }}
              onRemoveTag={(tag) => { void handleBulkTagChange(tag, 'remove') }}
              onMergeInto={(survivorId) => { void handleBulkMergeInto(survivorId) }}
              onRemoveFromView={() => { void handleBulkRemoveFromView() }}
              onCopyMermaid={handleBulkCopyMermaid}
            />

            <ViewHeaderButton
              name={viewName ?? undefined}
              isOpen={viewDetails.isOpen}
              onToggle={viewDetails.onToggle}
              supportsZoomOut={supportsZoomOut}
              supportsZoomIn={supportsZoomIn}
            />

            <ViewFloatingMenu
              drawingMode={drawingMode} setDrawingMode={setDrawingMode}
              hasDrawingPaths={drawingPaths.length > 0} drawingVisible={drawingVisible} setDrawingVisible={setDrawingVisible}
              extrasOpen={extrasOpen} setExtrasOpen={setExtrasOpen}
              hasMarkdown={!!viewMarkdown}
              markdownOpen={isMarkdownOpen}
              markdownBusy={markdownBusy}
              onMarkdownToggle={handleToggleMarkdown}
              focusMode={!crossBranchSettings.enabled}
              onFocusModeChange={handleFocusModeChange}
              densityLevel={densityLevel}
              onDensityLevelChange={handleDensityLevelChange}
              noiseGateEnabled={noiseGateEnabled}
              noiseGateBusy={noiseGateBusy}
              onNoiseGateEnabledChange={handleNoiseGateEnabledChange}
              canUndo={canUndoViewEdit}
              canRedo={canRedoViewEdit}
              undoRedoDisabled={isApplyingHistory}
              onUndo={handleUndoViewEdit}
              onRedo={handleRedoViewEdit}
              disableImportExport={disableImportExport}
              onImport={importModal.onOpen} onExport={handleOpenExport}
              onCopyMermaid={handleCopyMermaidDirect}
              onExportMermaid={handleExportMermaidDirect}
              onShare={onShare}
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
              hideFocusView={demoOptions?.hideFocusView}
              hideExpandExtras={demoOptions?.hideExpandExtras}
            />
          </Box>

          {isMarkdownOpen && (
            <>
              <Box
                role="separator"
                aria-orientation="vertical"
                aria-label="Resize markdown editor"
                flex="0 0 auto"
                w={`${VIEW_EDITOR_MARKDOWN_RESIZE_HANDLE_WIDTH}px`}
                cursor="col-resize"
                position="relative"
                onPointerDown={handleMarkdownResizeStart}
                bg={isMarkdownResizing ? 'whiteAlpha.100' : 'transparent'}
                _hover={{ bg: 'whiteAlpha.50' }}
                _active={{ bg: 'whiteAlpha.100' }}
              >
                <Box
                  position="absolute"
                  top={0}
                  bottom={0}
                  left="50%"
                  transform="translateX(-50%)"
                  w="1px"
                  bg="whiteAlpha.100"
                />
              </Box>

              <Box
                flex="0 0 auto"
                w={`${markdownPaneWidth}px`}
                minW={`${isMobileLayout ? VIEW_EDITOR_MARKDOWN_MIN_WIDTH_MOBILE : VIEW_EDITOR_MARKDOWN_MIN_WIDTH}px`}
                maxW={`calc(100% - ${isMobileLayout ? VIEW_EDITOR_CANVAS_MIN_WIDTH_MOBILE : VIEW_EDITOR_CANVAS_MIN_WIDTH}px)`}
                minH={0}
                borderLeft="1px solid"
                borderColor="whiteAlpha.100"
              >
                <ViewMarkdownPanel
                  isOpen={isMarkdownOpen}
                  onClose={() => setIsMarkdownOpen(false)}
                  viewName={view?.name}
                  markdown={viewMarkdown}
                  content={viewMarkdownContent}
                  syncToken={viewMarkdownSyncToken}
                  viewId={viewId}
                  viewNameById={viewNameById}
                  mermaidIntegrationEnabled={mermaidIntegrationEnabled}
                  canEdit={canEdit}
                  isLoading={isMarkdownLoading}
                  isSaving={isMarkdownSaving}
                  isDirty={markdownDirty}
                  hasSaveConflict={markdownSaveConflict}
                  onChange={setViewMarkdownContent}
                  onSave={handleSaveMarkdown}
                  onForceSave={handleForceSaveMarkdown}
                  onCreateMarkdown={handleCreateMarkdownFromPanel}
                  onAttachMarkdown={handleLinkMarkdown}
                  onUnlinkMarkdown={handleUnlinkMarkdown}
                  onPickMarkdownFile={handlePickMarkdownFile}
                  onSaveAs={handleSaveMarkdownAs}
                  onOpenInEditor={window.__TLD_VSCODE__ ? handleOpenMarkdownInEditor : undefined}
                  onReload={handleReloadMarkdown}
                  onNavigateToView={canvas.stableOnNavigateToView}
                  onImportMermaidBlock={handleImportMarkdownMermaidBlock}
                />
              </Box>
            </>
          )}
        </Flex>

        <ElementLibrary
          existingElementIds={existingElementIds}
          existingElements={existingElements}
          onCreateNew={handleCreateNewLibrary}
          isOpen={libraryOpen} onClose={handleCloseLibrary}
          onTapAdd={canEdit ? handleTapAdd : undefined}
          onFindElement={handleFindElement}
          onTouchDrop={canEdit ? handleTouchDrop : undefined}
          deletedElementIds={deletedLibraryElementIds}
          noFocusLock={!!pendingElement || !!textEditorState}
        />

        <ElementPanel
          isOpen={elementPanel.isOpen} onClose={handleElementPanelClose} element={selectedElement}
          onSave={handleElementPanelSave} autoSave
          onMerge={handleOpenMerge}
          onDelete={(elementId) => {
            elementEditSessionRef.current = null
            const placement = viewElements.find((item) => item.element_id === elementId)
            if (placement) pushPlacementRemoveAction(placement)
            handleElementDeleted(elementId)
          }} onPermanentDelete={handleElementPermanentlyDeletedEverywhere}
          visibilityOverrideDelta={overrideDeltaFor('element', selectedElement?.id)}
          onVisibilityOverrideDeltaChange={(id, delta) => handleVisibilityOverrideDeltaChange('element', id, delta)}
          onResetVisibility={(id) => handleVisibilityOverride('element', id, 'reset')}
          orgId={''}
          links={selectedElement ? (linksMap[selectedElement.id] || EMPTY_LINKS) : EMPTY_LINKS}
          parentLinks={selectedElement ? (parentLinksMap[selectedElement.id] || EMPTY_LINKS) : EMPTY_LINKS}
          hasBackdrop={isMobileLayout}
          availableTags={availableTags}
          noFocusLock={!!pendingElement || !!textEditorState}
          elementPanelAfterContentSlot={resolvedElementPanelAfterContentSlot}
        />

        <CodePreviewPanel isOpen={codePreview.isOpen} onClose={codePreview.onClose} element={previewElement} hasBackdrop={isMobileLayout} />

        <ConnectorPanel
          isOpen={connectorPanel.isOpen} onClose={handleConnectorPanelClose} connector={selectedEdge}
          orgId={''}
          onSave={handleConnectorPanelSave} autoSave
          onDelete={handleConnectorDeleteInPanel}
          visibilityOverrideDelta={overrideDeltaFor('connector', selectedEdge?.id)}
          onPromoteVisibility={(id) => handleVisibilityOverride('connector', id, 'promote')}
          onDemoteVisibility={(id) => handleVisibilityOverride('connector', id, 'demote')}
          onResetVisibility={(id) => handleVisibilityOverride('connector', id, 'reset')}
          hasBackdrop={isMobileLayout}
          noFocusLock={!!pendingElement || !!textEditorState}
          connectorPanelAfterContentSlot={resolvedConnectorPanelAfterContentSlot}
        />
        <ProxyConnectorPanel
          isOpen={proxyConnectorPanel.isOpen}
          onClose={proxyConnectorPanel.onClose}
          details={selectedProxyConnectorDetails}
          snapshot={effectiveWorkspaceSnapshot}
          hasBackdrop={isMobileLayout}
          onEdit={(connector) => {
            setSelectedEdge(connector)
            connectorPanel.onOpen()
          }}
          onDelete={(edgeId, ownerViewId) => {
            const deleted = selectedProxyConnectorDetails?.connectors.find((leaf) => leaf.connector.id === edgeId)?.connector
              ?? connectors.find((connector) => connector.id === edgeId)
              ?? null
            void api.workspace.connectors.delete('', edgeId).then(() => {
              if (deleted) pushConnectorDeleteAction(deleted)
              handleConnectorDeleted(edgeId, ownerViewId)
            }).catch(() => { /* intentionally empty */ })
          }}
        />

        <ViewPanel
          isOpen={viewDetails.isOpen} onClose={viewDetails.onClose}
          view={view as ViewTreeNode}
          onSave={handleViewSave}
          onUnsupportedMutation={handleUnsupportedMutation}
          onConnectorSaved={publishRealtimeConnectorUpsert}
          hasBackdrop={isMobileLayout}
          markdown={viewMarkdown}
          markdownLoading={isMarkdownLoading}
          onUnlinkMarkdown={handleUnlinkMarkdown}
          onOpenMarkdown={handleOpenMarkdown}
        />

        <ExportModal
          isOpen={exportModal.isOpen} onClose={exportModal.onClose}
          defaultFilename={sanitizeExportFilename((view as ViewTreeNode).name)}
          mermaidEnabled
          onExport={handleExportView} isExporting={isExporting}
        />
        <ImportModal
          isOpen={importModal.isOpen} onClose={importModal.onClose}
          mermaidEnabled={mermaidIntegrationEnabled}
          onImport={handleImportView} isImporting={isImporting}
        />
        <ConfirmDialog
          isOpen={duplicatePasteConfirm.isOpen}
          onClose={handleCancelDuplicatePaste}
          onConfirm={handleConfirmDuplicatePaste}
          title="Duplicate existing elements?"
          body={pendingDuplicatePaste
            ? `${pendingDuplicatePaste.conflictElementIds.length} pasted element${pendingDuplicatePaste.conflictElementIds.length === 1 ? '' : 's'} already exist in this view. Duplicate the conflicts to create separate elements, or cancel to leave this view unchanged.`
            : ''}
          confirmLabel="Duplicate"
          confirmColorScheme="blue"
          isLoading={isClipboardPasting}
        />
        <MergeDialog
          isOpen={mergeDialog.isOpen}
          onClose={() => { mergeDialog.onClose(); setMergeSourceElement(null) }}
          source={mergeSourceElement}
          onMerge={handleMerge}
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
