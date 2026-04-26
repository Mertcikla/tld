import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { useNavigate } from 'react-router-dom'
import { SafeBackground } from '../components/SafeBackground'
import { Text as HeaderText } from '@chakra-ui/react'
import ReactFlow, {
  BackgroundVariant,
  ReactFlowProvider,
  useReactFlow,
  useStore,
  type Edge as RFEdge,
  type Node as RFNode,
} from 'reactflow'
import FloatingEdge from '../components/FloatingEdge'
import 'reactflow/dist/style.css'
import { useSetHeader } from '../components/HeaderContext'
import {
  Box,
  Button,
  Flex,
  FormControl,
  FormLabel,
  Heading,
  HStack,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Spinner,
  Text,
  useDisclosure,
  useBreakpointValue,
} from '@chakra-ui/react'
import { api } from '../api/client'
import { toast } from '../utils/toast'
import type { ViewTreeNode } from '../types'
import ViewPanel from '../components/ViewPanel'
import ConfirmDialog from '../components/ConfirmDialog'
import ViewGridNode, { type ViewGridNodeData } from '../components/ViewGridNode'
import { useAccentColor } from '../context/ThemeContext'
import { hexToRgba } from '../constants/colors'

// ── Tree helpers ──────────────────────────────────────────────────────────────

function flattenTree(roots: ViewTreeNode[]): ViewTreeNode[] {
  const result: ViewTreeNode[] = []
  const traverse = (node: ViewTreeNode) => {
    result.push(node)
    node.children.forEach(traverse)
  }
  roots.forEach(traverse)
  return result
}

// ── Layout algorithm ──────────────────────────────────────────────────────────

const CELL_W = 260
const CELL_H = 150
const GAP_H = 80
const GAP_V = 120

function subtreeWidth(node: ViewTreeNode): number {
  if (node.children.length === 0) return 1
  return node.children.reduce((sum, c) => sum + subtreeWidth(c), 0)
}

function buildDescendantSets(roots: ViewTreeNode[]): Map<number, Set<number>> {
  const map = new Map<number, Set<number>>()

  function visit(node: ViewTreeNode): Set<number> {
    const set = new Set([node.id])
    node.children.forEach((child) => {
      const childSet = visit(child)
      childSet.forEach((id) => set.add(id))
    })
    map.set(node.id, set)
    return set
  }

  roots.forEach(visit)
  return map
}

/**
 * Compute layout positions.
 *
 * Y-axis: node.depth (= node.level) - honours manual level overrides so a
 *         diagram at L2 is always rendered in the L2 row even if its parent
 *         is at L0.
 *
 * X-axis: column derived from the tree-walk (pre-order rank within each
 *         level band), then a de-overlap pass shifts any colliding nodes and
 *         their subtrees rightward so nothing overlaps on the same row.
 */
function computeLayout(roots: ViewTreeNode[]): Map<number, { x: number; y: number }> {
  const positions = new Map<number, { x: number; y: number }>()
  const flat: ViewTreeNode[] = []
  const visit = (n: ViewTreeNode) => { flat.push(n); n.children.forEach(visit) }
  roots.forEach(visit)

  if (flat.length === 0) return positions

  // ── Step 1: initial column assignment via tree walk ─────────────────────────
  function layoutNode(node: ViewTreeNode, startCol: number) {
    const w = subtreeWidth(node)
    const centerCol = startCol + (w - 1) / 2
    positions.set(node.id, {
      x: centerCol * (CELL_W + GAP_H),
      y: node.depth * (CELL_H + GAP_V),
    })
    let childStart = startCol
    for (const child of node.children) {
      layoutNode(child, childStart)
      childStart += subtreeWidth(child)
    }
  }
  let col = 0
  for (const root of roots) {
    layoutNode(root, col)
    col += subtreeWidth(root)
  }
  // ── Step 2: build descendant sets so we can shift whole subtrees ────────────
  const descendants = buildDescendantSets(roots)

  // ── Step 3: de-overlap pass - per Y row (top-down), fix X collisions ────────
  const STEP = CELL_W + GAP_H
  const byY = new Map<number, number[]>()
  flat.forEach((n) => {
    const y = n.depth * (CELL_H + GAP_V)
    if (!byY.has(y)) byY.set(y, [])
    byY.get(y)!.push(n.id)
  })

  // Process rows top-down (ascending Y) so parent shifts propagate downward first
  const sortedYRows = Array.from(byY.entries()).sort(([ya], [yb]) => ya - yb)

  for (const [rowY, ids] of sortedYRows) {
    // Snapshot original X values before any mutations in this row -
    // this prevents a just-shifted node's new position from cascading
    // into the next comparison and wrongly pushing correct neighbors right.
    const origX = new Map<number, number>(ids.map((id) => [id, positions.get(id)?.x ?? 0]))
    ids.sort((a, b) => (origX.get(a) ?? 0) - (origX.get(b) ?? 0))

    let rightmostX = origX.get(ids[0]) ?? 0

    for (let i = 1; i < ids.length; i++) {
      const originalX = origX.get(ids[i]) ?? 0
      const placedX = Math.max(originalX, rightmostX + STEP)

      if (placedX > originalX) {
        const delta = placedX - originalX
        const toShift = descendants.get(ids[i]) ?? new Set([ids[i]])
        toShift.forEach((sid) => {
          const p = positions.get(sid)
          if (!p) return
          if (p.y === rowY && sid !== ids[i]) return
          positions.set(sid, { x: p.x + delta, y: p.y })
        })
      }

      rightmostX = placedX
    }
  }

  return positions
}





function DepthBoundaryNode({ data }: { data: { width: number; depth: number; isReparenting?: boolean; onLevelClick?: () => void; isActive?: boolean } }) {
  return (
    <Box
      w={`${data.width}px`}
      h="20px"
      position="relative"
      pointerEvents={data.isReparenting ? 'auto' : 'none'}
      userSelect="none"
      display="flex"
      alignItems="center"
      cursor={data.isReparenting ? 'crosshair' : 'default'}
      onClick={(e) => {
        if (data.isReparenting && data.onLevelClick) {
          e.stopPropagation()
          data.onLevelClick()
        }
      }}
      transition="background 0.2s"
      _hover={data.isReparenting ? { bg: 'whiteAlpha.50' } : undefined}
    >
      <Box
        w="100%"
        h="1px"
        borderTop="1px dashed"
        borderColor={data.isActive ? 'whiteAlpha.900' : (data.isReparenting ? 'var(--accent)' : 'whiteAlpha.400')}
        opacity={data.isActive ? 1 : (data.isReparenting ? 0.8 : 0.4)}
        transition="all 0.2s"
      />
    </Box>
  )
}

function ViewGridSidebar({ maxDepth, isReparenting, onLevelClick, activeLevel }: { maxDepth: number; isReparenting: boolean; onLevelClick: (level: number) => void; activeLevel?: number | null }) {
  const rowHeight = CELL_H + GAP_V
  const levelCount = Math.max(maxDepth + 2, 4)
  const transform = useStore((s) => s.transform)
  const [, translateY, zoom] = transform

  return (
    <Box
      position="absolute"
      left={0}
      top={0}
      bottom={0}
      w="120px"
      pointerEvents="none"
      zIndex={10}
      overflow="hidden"
    >

      {/* Layers Container - follows the zoom and pan of the grid */}
      <Box
        position="absolute"
        left={0}
        right={0}
        top={`${translateY}px`}
        transform={`scale(${zoom})`}
        transformOrigin="top left"
        h={`${levelCount * rowHeight}px`}
      >
        {Array.from({ length: levelCount }).map((_, i) => {
          const isActive = activeLevel === i
          return (
            <Flex
              key={`layer-${i}`}
              position="absolute"
              top={`${i * rowHeight + 75}px`}
              transform="translateY(-50%)"
              left="0"
              right="20px"
              h="140px"
              align="center"
              justify="flex-end"
              cursor={isReparenting ? 'pointer' : 'default'}
              pointerEvents="auto"
              onClick={(e) => {
                if (isReparenting) {
                  e.stopPropagation()
                  onLevelClick(i)
                }
              }}
              transition="all 0.4s cubic-bezier(0.4, 0, 0.2, 1)"
              role="group"
              _hover={isReparenting ? { transform: 'translateY(-50%) scale(1.05)', bg: 'whiteAlpha.50' } : {}}
            >
              {/* Technical Tick */}
              <Box
                position="absolute"
                top="50%"
                right="-20px"
                w={isReparenting || isActive ? "40px" : "20px"}
                h="1px"
                bg={isActive ? 'whiteAlpha.900' : (isReparenting ? 'var(--accent)' : "whiteAlpha.400")}
                transition="all 0.4s"
                _after={{
                  content: '""',
                  position: 'absolute',
                  top: '-2.5px',
                  right: '0',
                  w: '6px',
                  h: '6px',
                  borderRadius: 'full',
                  bg: isActive ? 'whiteAlpha.900' : (isReparenting ? 'var(--accent)' : "whiteAlpha.400"),
                }}
              />

              <Box textAlign="left" pr={2}>
                <Heading
                  fontSize="100px"
                  fontWeight="900"
                  color={isActive ? 'whiteAlpha.900' : (isReparenting ? 'var(--accent)' : "whiteAlpha.100")}
                  fontFamily="heading"
                  lineHeight="1"
                  letterSpacing="-0.06em"
                  transition="all 0.4s"
                  style={{
                    WebkitTextStroke: i === 0 || isReparenting || isActive ? 'none' : '1px rgba(255,255,255,0.1)',
                  }}
                  _groupHover={(isReparenting || isActive) ? { transform: 'scale(1.1)' } : {}}
                >
                  {i}
                </Heading>
              </Box>
            </Flex>
          )
        })}
      </Box>
    </Box>
  )
}

// ── Depth boundary separator nodes ──────────────────────────────────────────


// ── Node types (stable module-level constant) ─────────────────────────────────

const NODE_TYPES = { diagramGrid: ViewGridNode, depthBoundary: DepthBoundaryNode }
const EDGE_TYPES = { floating: FloatingEdge }

// Hierarchy edges: muted neutral - structure without color noise
const HIERARCHY_EDGE_COLOR = 'rgba(255,255,255,0.2)'

// ── Props ─────────────────────────────────────────────────────────────────────

interface Props {
  onShare?: (viewId: number) => void
  treeData: ViewTreeNode[]
  loading: boolean
  focusedId: number | null
  onFocusChange: (viewId: number | null) => void
  setTreeData: Dispatch<SetStateAction<ViewTreeNode[]>>
  refreshTree: () => Promise<void>
}

// ── Root component - provides ReactFlow context ───────────────────────────────

export default function ViewsGrid(props: Props) {
  return (
    <ReactFlowProvider>
      <ViewGridInner {...props} />
    </ReactFlowProvider>
  )
}

// ── Inner component - has access to useReactFlow() ────────────────────────────

function ViewGridInner({ onShare, treeData, loading, focusedId, onFocusChange, setTreeData, refreshTree }: Props) {
  const isMobileLayout = useBreakpointValue({ base: true, md: false }) ?? false
  const navigate = useNavigate()
  const { accent } = useAccentColor()
  const canEdit = true
  const setHeader = useSetHeader()

  useEffect(() => {
    setHeader({ node: <HeaderText fontWeight="medium" fontSize="sm" color="gray.300">View Hierarchy</HeaderText> })
    return () => setHeader(null)
  }, [setHeader])

  const { setCenter, getViewport, zoomIn, zoomOut } = useReactFlow()
  const rfContainerRef = useRef<HTMLDivElement>(null)

  // ── Trackpad gesture detection: suppress zoom during two-finger pan ────────
  const touchStateRef = useRef<{ lastMultiTouchWheelTime: number }>({
    lastMultiTouchWheelTime: 0,
  })

  // Native capture-phase wheel listener so we intercept before ReactFlow's
  // internal handlers. passive:false lets us call preventDefault().
  useEffect(() => {
    const el = rfContainerRef.current
    if (!el) return
    function onWheel(e: WheelEvent) {
      // Track multi-touch wheel events (deltaX !== 0 indicates two-finger contact)
      if (e.deltaX !== 0) {
        touchStateRef.current.lastMultiTouchWheelTime = Date.now()
      }

      // If we just finished a multi-touch gesture, suppress zoom for ~1000ms (trackpad momentum can last longer)
      const isRecentMultiTouch = Date.now() - touchStateRef.current.lastMultiTouchWheelTime < 1000

      // Only zoom on notched wheel (mouse), not trackpad
      const isNotchedWheel = !e.ctrlKey && e.deltaX === 0 && Number.isInteger(e.deltaY) && Math.abs(e.deltaY) >= 20
      const isMouseWheel = e.deltaMode !== 0 || isNotchedWheel

      if (isMouseWheel && !isRecentMultiTouch) {
        e.preventDefault()
        e.stopPropagation()
        if (e.deltaY > 0) zoomOut()
        else zoomIn()
      }
    }
    el.addEventListener('wheel', onWheel, { passive: false, capture: true })
    return () => el.removeEventListener('wheel', onWheel, { capture: true })
  }, [zoomIn, zoomOut])

  // ── Derived tree structures ─────────────────────────────────────────────────
  const roots = useMemo(() => treeData, [treeData])
  const flatTree = useMemo(() => flattenTree(roots), [roots])

  // Rename
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editName, setEditName] = useState('')

  // Counts cache
  const [countsByView, setCountsByDiagram] = useState<Record<number, { nodes: number; edges: number }>>({})

  // Onboarding wizard
  const [onboardingStep, setOnboardingStep] = useState<0 | 1 | 2>(0)
  const [onboardingName, setOnboardingName] = useState('My First Diagram')
  const [onboardingViewId, setOnboardingDiagramId] = useState<number | null>(null)
  const [onboardingCreating, setOnboardingCreating] = useState(false)

  // Details drawer
  const [detailsView, setDetailsDiagram] = useState<ViewTreeNode | null>(null)
  const [detailsLoading, setDetailsLoading] = useState(false)
  const { isOpen: isDetailsOpen, onOpen: onDetailsOpen, onClose: onDetailsClose } = useDisclosure()

  // Delete dialog
  const [deleteTargetId, setDeleteTargetId] = useState<number | null>(null)
  const { isOpen: isDeleteOpen, onOpen: onDeleteOpen, onClose: onDeleteClose } = useDisclosure()

  // Level change mode
  const [levelEditingNodeId, setLevelEditingNodeId] = useState<number | null>(null)

  useEffect(() => {
    if (treeData.length === 0 && !loading && !localStorage.getItem('onboarding_shown')) {
      localStorage.setItem('onboarding_shown', '1')
      setOnboardingStep(1)
    }
  }, [loading, treeData.length])

  // Fetch node/edge counts
  useEffect(() => {
    let cancelled = false
    const ids = flatTree.map((n) => n.id)
    if (ids.length === 0) { setCountsByDiagram({}); return }
    ; (async () => {
      const next: Record<number, { nodes: number; edges: number }> = {}
      await Promise.all(
        ids.map(async (id) => {
          try {
            const [objs, edges] = await Promise.all([
              api.workspace.views.placements.list(id),
              api.workspace.connectors.list(id),
            ])
            next[id] = { nodes: objs.length, edges: edges.length }
          } catch { /* ignore per-diagram failure */ }
        })
      )
      if (!cancelled) setCountsByDiagram((prev) => ({ ...prev, ...next }))
    })()
    return () => { cancelled = true }
  }, [flatTree])

  // ── Rename ──────────────────────────────────────────────────────────────────
  const startEdit = useCallback((id: number, name: string) => {
    setEditingId(id)
    setEditName(name)
  }, [])

  const commitEdit = useCallback(async () => {
    const id = editingId
    const name = editName.trim()
    setEditingId(null)
    if (id === null || !name) return
    const prev = treeData.find((n) => n.id === id)
    if (!prev || prev.name === name) return
    setTreeData((d) => d.map((n) => (n.id === id ? { ...n, name } : n)))
    await api.workspace.views.rename(id, name).catch(() =>
      setTreeData((d) => d.map((n) => (n.id === id ? { ...n, name: prev.name } : n)))
    )
  }, [editingId, editName, setTreeData, treeData])

  const cancelEdit = useCallback(() => setEditingId(null), [])

  // ── Details ─────────────────────────────────────────────────────────────────
  const handleDetailsOpen = useCallback(async (diagId: number) => {
    setDetailsLoading(true)
    onDetailsOpen()
    try {
      const d = await api.workspace.views.get(diagId)
      setDetailsDiagram(d)
    } catch { /* ignore */ } finally {
      setDetailsLoading(false)
    }
  }, [onDetailsOpen])

  const handleDetailsSave = useCallback((updated: ViewTreeNode) => {
    setTreeData((prev) =>
      prev.map((n) =>
        n.id === updated.id
          ? { ...n, name: updated.name, level_label: updated.level_label }
          : n
      )
    )
  }, [setTreeData])

  // ── Delete ──────────────────────────────────────────────────────────────────
  const handleDeleteConfirm = async () => {
    if (!deleteTargetId) return
    try {
      await api.workspace.views.delete('', deleteTargetId)
      setTreeData((prev) => prev.filter((n) => n.id !== deleteTargetId))
    } catch { /* ignore */ }
    onDeleteClose()
    setDeleteTargetId(null)
  }

  const handleSetLevel = useCallback(async (level: number) => {
    if (!levelEditingNodeId) return
    const id = levelEditingNodeId
    const node = treeData.find((n) => n.id === id)
    if (!node) return

    // Validate: must be strictly greater than parent's level
    if (node.parent_view_id !== null) {
      const parent = treeData.find((n) => n.id === node.parent_view_id)
      if (parent && level <= parent.level) {
        toast({ title: `Level must be > parent's level (L${parent.level})`, status: 'warning', duration: 3000, isClosable: true })
        return
      }
    }

    // Validate: must be strictly less than all direct children's levels
    const childLevels = treeData.filter((n) => n.parent_view_id === id).map((n) => n.level)
    if (childLevels.length > 0 && level >= Math.min(...childLevels)) {
      toast({ title: `Level must be < children's levels (min L${Math.min(...childLevels)})`, status: 'warning', duration: 3000, isClosable: true })
      return
    }

    setLevelEditingNodeId(null)
    // Optimistically update locally
    setTreeData((d) => d.map((n) => (n.id === id ? { ...n, level } : n)))
    try {
      await api.workspace.views.setLevel(id, level)
    } catch {
      // global error toast will show
    }
    await refreshTree()
  }, [levelEditingNodeId, treeData, refreshTree, setTreeData])

  const handleOnboardingCreate = async () => {
    setOnboardingCreating(true)
    try {
      const d = await api.workspace.views.create({ name: onboardingName.trim() || 'My First Diagram' })
      setOnboardingDiagramId(d.id)
      await refreshTree()
      setOnboardingStep(2)
    } catch { /* ignore */ } finally {
      setOnboardingCreating(false)
    }
  }

  // ── RF nodes - pure derivation, no useState/useEffect ───────────────────────
  const layoutPositions = useMemo(() => computeLayout(roots), [roots])

  // Stable during drag (layoutPositions only changes after treeData refresh, never on mouse moves)
  const computedMinZoom = useMemo(() => {
    if (layoutPositions.size === 0) return 0.2
    let minY = Infinity, maxY = -Infinity
    layoutPositions.forEach(({ y }) => {
      if (y < minY) minY = y
      if (y + CELL_H > maxY) maxY = y + CELL_H
    })
    const bboxH = maxY - minY
    let z = window.innerHeight / (Math.max(1, bboxH) * 1.2)
    if (!isFinite(z) || isNaN(z) || z <= 0) z = 0.1
    return Math.max(0.05, Math.min(z, 0.8))
  }, [layoutPositions])

  const computedTranslateExtent = useMemo((): [[number, number], [number, number]] | undefined => {
    if (layoutPositions.size === 0) return undefined
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    layoutPositions.forEach(({ x, y }) => {
      if (x < minX) minX = x
      if (y < minY) minY = y
      if (x + CELL_W > maxX) maxX = x + CELL_W
      if (y + CELL_H > maxY) maxY = y + CELL_H
    })
    const panMarginX = Math.max(window.innerWidth, 1000)
    const panMarginY = Math.max(window.innerHeight, 1000)
    return [
      [minX - panMarginX, minY - panMarginY],
      [maxX + panMarginX, maxY + panMarginY],
    ]
  }, [layoutPositions])
  const maxDepth = useMemo(
    () => flatTree.reduce((max, n) => Math.max(max, n.depth), 0),
    [flatTree]
  )

  // ── WASD navigation targets (IDs of the 4 navigable neighbors) ─────────────
  const wasdTargets = useMemo(() => {
    if (focusedId === null) return {} as Record<number, 'w' | 'a' | 's' | 'd'>
    const node = flatTree.find((n) => n.id === focusedId)
    if (!node) return {} as Record<number, 'w' | 'a' | 's' | 'd'>
    const siblings = flatTree.filter((n) => n.parent_view_id === node.parent_view_id)
    const idx = siblings.findIndex((n) => n.id === focusedId)
    const targets: Record<number, 'w' | 'a' | 's' | 'd'> = {}
    if (node.parent_view_id !== null) targets[node.parent_view_id] = 'w'
    const firstChild = flatTree.find((n) => n.parent_view_id === focusedId)
    if (firstChild) targets[firstChild.id] = 's'
    if (idx > 0) targets[siblings[idx - 1].id] = 'a'
    if (idx < siblings.length - 1) targets[siblings[idx + 1].id] = 'd'
    return targets
  }, [focusedId, flatTree])

  const rfNodes = useMemo((): RFNode[] =>
    flatTree.map((n): RFNode => ({
      id: String(n.id),
      type: 'diagramGrid',
      position: layoutPositions.get(n.id) ?? { x: 0, y: 0 },
      data: {
        id: n.id,
        name: n.name,
        level_label: n.level_label,
        counts: countsByView[n.id],
        focused: focusedId === n.id,
        canEdit,
        isEditing: editingId === n.id,
        editName,
        onFocus: () => onFocusChange(n.id),
        onOpen: () => navigate(`/views/${n.id}`),
        onStartRename: () => startEdit(n.id, n.name),
        onDetails: () => handleDetailsOpen(n.id),
        onDelete: () => { setDeleteTargetId(n.id); onDeleteOpen() },
        onShare: onShare ? () => onShare(n.id) : () => {},
        onEditNameChange: setEditName,
        onEditCommit: commitEdit,
        onEditCancel: cancelEdit,
        isMobile: isMobileLayout,
        wasdKey: wasdTargets[n.id],
      } satisfies ViewGridNodeData,
      draggable: false,
    })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [flatTree, layoutPositions, focusedId, countsByView,
      editingId, editName, canEdit, navigate, startEdit, handleDetailsOpen,
      commitEdit, cancelEdit, onDeleteOpen,
      wasdTargets, levelEditingNodeId]
  )

  // ── Depth boundary separator nodes ──────────────────────────────────────────
  const depthBoundaryNodes = useMemo((): RFNode[] => {
    if (levelEditingNodeId === null || maxDepth < 1 || layoutPositions.size === 0) return []
    let minX = Infinity, maxX = -Infinity
    layoutPositions.forEach(({ x }) => {
      if (x < minX) minX = x
      if (x + CELL_W > maxX) maxX = x + CELL_W
    })
    const startX = minX - 3 * GAP_H
    const totalW = maxX - minX + 5 * GAP_H
    const editingNode = flatTree.find((n) => n.id === levelEditingNodeId)
    const activeLevel = editingNode?.level ?? null

    return Array.from({ length: maxDepth + 2 }, (_, i) => {
      const depth = i
      return {
        id: `__depth_${depth}`,
        type: 'depthBoundary',
        position: { x: startX, y: depth * (CELL_H + GAP_V) - GAP_V / 2 - 8 },
        data: {
          width: totalW,
          depth,
          isReparenting: true,
          onLevelClick: () => handleSetLevel(depth),
          isActive: activeLevel === depth || activeLevel === depth - 1,
        },
        draggable: false,
        selectable: false,
        focusable: false,
        style: { zIndex: 0 },
      } as RFNode
    })
  }, [maxDepth, layoutPositions, levelEditingNodeId, flatTree, handleSetLevel])

  const allRfNodes = useMemo(
    () => levelEditingNodeId !== null ? [...depthBoundaryNodes, ...rfNodes] : rfNodes,
    [rfNodes, depthBoundaryNodes, levelEditingNodeId]
  )

  // ── RF edges ────────────────────────────────────────────────────────────────
  const rfEdges = useMemo((): RFEdge[] =>
    flatTree
      .filter((n) => n.parent_view_id)
      .map((n) => ({
        id: `${n.parent_view_id}-${n.id}`,
        source: String(n.parent_view_id!),
        target: String(n.id),
        type: 'floating',
        animated: false,
        data: { color: HIERARCHY_EDGE_COLOR, dashed: false },
      })),
    [flatTree]
  )

  const allRfEdges = rfEdges

  // ── WASD keyboard navigation ────────────────────────────────────────────────
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement).tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA') return

      if (e.key === 'Escape') {
        if (levelEditingNodeId !== null) {
          setLevelEditingNodeId(null)
        } else {
          onFocusChange(null)
        }
        return
      }

      if (e.key === 'Enter' && focusedId) { navigate(`/views/${focusedId}`); return }

      const isNav = ['w', 'W', 's', 'S', 'a', 'A', 'd', 'D'].includes(e.key)
      if (!isNav) return

      // Auto-select first card if nothing is focused yet
      if (!focusedId) {
        if (flatTree.length > 0) onFocusChange(flatTree[0].id)
        return
      }

      const node = flatTree.find((n) => n.id === focusedId)
      if (!node) return

      let nextId: number | null = null
      if (e.key === 'w' || e.key === 'W') {
        nextId = node.parent_view_id ?? null
      } else if (e.key === 's' || e.key === 'S') {
        nextId = flatTree.find((n) => n.parent_view_id === focusedId)?.id ?? null
      } else if (e.key === 'a' || e.key === 'A') {
        const siblings = flatTree.filter((n) => n.parent_view_id === node.parent_view_id)
        const idx = siblings.findIndex((n) => n.id === focusedId)
        nextId = idx > 0 ? siblings[idx - 1].id : null
      } else if (e.key === 'd' || e.key === 'D') {
        const siblings = flatTree.filter((n) => n.parent_view_id === node.parent_view_id)
        const idx = siblings.findIndex((n) => n.id === focusedId)
        nextId = idx < siblings.length - 1 ? siblings[idx + 1].id : null
      }

      if (nextId) onFocusChange(nextId)
    }

    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [focusedId, flatTree, navigate, levelEditingNodeId, onFocusChange])

  // ── Camera: pan to focused node only when it's out of view ──────────────────
  useEffect(() => {
    if (!focusedId) return
    const pos = layoutPositions.get(focusedId)
    if (!pos) return
    const t = setTimeout(() => {
      const { x: vpX, y: vpY, zoom } = getViewport()
      // Convert node screen-space bounds and check if comfortably inside the viewport
      const margin = 80
      const sl = pos.x * zoom + vpX
      const st = pos.y * zoom + vpY
      const sr = (pos.x + CELL_W) * zoom + vpX
      const sb = (pos.y + CELL_H) * zoom + vpY
      const cw = window.innerWidth
      const ch = window.innerHeight
      const inView = sl > margin && st > margin && sr < cw - margin && sb < ch - margin
      if (inView) return
      setCenter(
        pos.x + CELL_W / 2,
        pos.y + CELL_H / 2,
        { duration: 650, zoom: Math.max(zoom, 0.75) }
      )
    }, 30)
    return () => clearTimeout(t)
  }, [focusedId, layoutPositions, setCenter, getViewport])

  // ── Render ──────────────────────────────────────────────────────────────────
  if (loading) {
    return <Flex h="full" align="center" justify="center"><Spinner size="xl" /></Flex>
  }

  return (
    <Box h="full" display="flex" flexDir="column" position="relative">
      {/* Canvas */}
      <Box flex={1} position="relative">
        {/* Level change overlay banner */}
        {levelEditingNodeId && (
          <Flex
            position="absolute"
            top={6}
            left="50%"
            transform="translateX(-50%)"
            bg="rgba(15, 23, 42, 0.85)"
            border="1px solid var(--accent)"
            boxShadow="0 8px 32px rgba(0,0,0,0.6), 0 0 24px rgba(var(--accent-rgb), 0.3)"
            borderRadius="full"
            px={6}
            py={3}
            zIndex={100}
            align="center"
            gap={6}
            backdropFilter="blur(12px)"
          >
            <Flex align="center" gap={3}>
              <Box w={2} h={2} borderRadius="full" bg="var(--accent)" boxShadow="0 0 8px var(--accent)" />
              <Text color="gray.200" fontSize="sm" fontWeight="medium">
                Changing level for <Text as="span" color="white" fontWeight="bold">"{flatTree.find(n => n.id === levelEditingNodeId)?.name}"</Text>
              </Text>
            </Flex>
            <Box w="1px" h="16px" bg="whiteAlpha.300" />
            <Text color="gray.400" fontSize="sm">
              Click an L0-L9 level band to set diagram depth
            </Text>
            <Flex gap={2}>
              <Button size="xs" variant="ghost" color="gray.400" _hover={{ color: 'white', bg: 'whiteAlpha.200' }} onClick={() => setLevelEditingNodeId(null)}>
                Cancel
              </Button>
            </Flex>
          </Flex>
        )}

        {levelEditingNodeId !== null && (
          <ViewGridSidebar
            maxDepth={maxDepth}
            isReparenting={true}
            onLevelClick={handleSetLevel}
            activeLevel={flatTree.find((n) => n.id === levelEditingNodeId)?.level ?? null}
          />
        )}

        <Box
          ref={rfContainerRef}
          position="relative"
          w="full"
          h="full"
        >
          <ReactFlow
            nodes={allRfNodes}
            edges={allRfEdges}
            nodeTypes={NODE_TYPES}
            edgeTypes={EDGE_TYPES}
            onlyRenderVisibleElements
            fitView
            fitViewOptions={{ padding: 0.15, minZoom: 0.8, maxZoom: 1.2 }}
            panOnScroll={!isMobileLayout}
            zoomOnScroll={false}
            zoomOnPinch
            minZoom={computedMinZoom}
            maxZoom={2}
            translateExtent={computedTranslateExtent}
            nodesDraggable={false}
            nodesConnectable={false}
            onPaneClick={() => {
              onFocusChange(null)
            }}
            style={{
              background: 'var(--bg-canvas)',
              boxShadow: 'inset 0 0 100px rgba(0,0,0,0.6)'
            }}
          >
            {/* Micro dots for high precision technical feel */}
            <SafeBackground id="micro" variant={BackgroundVariant.Dots} gap={20} size={1} color={hexToRgba(accent, 0.2)} />
            {/* Minor cell grid for regular structural spacing */}
          </ReactFlow>
        </Box>

        {/* Empty state overlay */}
        {roots.length === 0 && (
          <Flex
            position="absolute"
            inset={0}
            align="center"
            justify="center"
            pointerEvents="none"
          >
            <Box textAlign="center">
              <Text color="gray.600" fontSize="sm" mb={1}>No views yet.</Text>
              {canEdit && (
                <>
                  <Text color="gray.700" fontSize="xs" mb={4}>Click "New Diagram" to get started.</Text>

                </>
              )}
            </Box>
          </Flex>
        )}

      </Box>

      {/* Legend + keyboard hint */}
      <Box
        position="fixed"
        bottom={0}
        left={0}
        right={0}
        zIndex={20}
        pointerEvents="none"
        pb={3}
      >
        {/* Edge type legend */}
        <Flex justify="center" align="center" gap={4} mb="3px">
          <HStack spacing={1}>
            <Box w="18px" style={{ borderTop: '1px solid rgba(255,255,255,0.2)' }} />
            <Text fontSize="9px" color="gray.700" letterSpacing="0.05em" lineHeight={1}>hierarchy link</Text>
          </HStack>
        </Flex>
        <Text fontSize="11px" color="gray.700" userSelect="none" letterSpacing="0.03em" textAlign="center">
          Click=Select · W↑ S↓ A← D→ · Enter=Open · Esc=Deselect
        </Text>
      </Box>

      {/* Confirm Delete Dialog */}
      <ConfirmDialog
        isOpen={isDeleteOpen}
        onClose={onDeleteClose}
        onConfirm={handleDeleteConfirm}
        title="Delete diagram"
        body="Are you sure you want to delete this diagram? This action cannot be undone."
        confirmLabel="Delete"
        confirmColorScheme="red"
      />

      {/* Details Drawer */}
      <ViewPanel
        isOpen={isDetailsOpen && !detailsLoading}
        onClose={onDetailsClose}
        view={detailsView}
        canEdit={canEdit}
        onSave={handleDetailsSave}
        hasBackdrop={isMobileLayout}
      />

      {/* Feature tutorial */}

      {/* Onboarding Wizard */}
      <Modal
        isOpen={onboardingStep === 1 || onboardingStep === 2}
        onClose={() => setOnboardingStep(0)}
        isCentered
        size="sm"
      >
        <ModalOverlay bg="blackAlpha.700" />
        <ModalContent bg="var(--bg-panel)" border="1px solid" borderColor="var(--border-main)">
          {onboardingStep === 1 && (
            <>
              <ModalHeader color="gray.100" pb={1}>Welcome to tldiagram!</ModalHeader>
              <ModalBody>
                <Text fontSize="sm" color="gray.400" mb={4}>
                  Start by creating your first diagram.
                </Text>
                <FormControl id="onboarding-view-name">
                  <FormLabel fontSize="xs" color="gray.500" textTransform="uppercase">
                    Diagram Name
                  </FormLabel>
                  <Input
                    name="name"
                    value={onboardingName}
                    onChange={(e) => setOnboardingName(e.target.value)}
                    size="sm"
                    autoFocus
                    onKeyDown={(e) => e.key === 'Enter' && handleOnboardingCreate()}
                  />
                </FormControl>
              </ModalBody>
              <ModalFooter gap={2}>
                <Button size="sm" variant="ghost" color="gray.500" onClick={() => setOnboardingStep(0)}>
                  Skip
                </Button>
                <Button
                  size="sm"
                  colorScheme="blue"
                  isLoading={onboardingCreating}
                  isDisabled={!onboardingName.trim()}
                  onClick={handleOnboardingCreate}
                >
                  Create Diagram
                </Button>
              </ModalFooter>
            </>
          )}
          {onboardingStep === 2 && (
            <>
              <ModalHeader color="gray.100" pb={1}>Your diagram is ready!</ModalHeader>
              <ModalBody>
                <Text fontSize="sm" color="gray.400">
                  Next, add elements to your diagram to start building your architecture.
                </Text>
              </ModalBody>
              <ModalFooter>
                <Button
                  size="sm"
                  colorScheme="blue"
                  onClick={() => {
                    setOnboardingStep(0)
                    if (onboardingViewId !== null) navigate(`/views/${onboardingViewId}`)
                  }}
                >
                  Start Building
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </Box>
  )
}
