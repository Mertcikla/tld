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

function filterTreeForGrid(nodes: ViewTreeNode[], allowedIds: Set<number> | null): ViewTreeNode[] {
  if (!allowedIds) return nodes

  const visit = (node: ViewTreeNode): ViewTreeNode | null => {
    const children = node.children
      .map(visit)
      .filter((child): child is ViewTreeNode => child !== null)
    const include = allowedIds.has(node.id) || (node.parent_view_id === null && children.length > 0)
    if (!include) return null
    return { ...node, children }
  }

  return nodes
    .map(visit)
    .filter((node): node is ViewTreeNode => node !== null)
}

// ── Layout algorithm ──────────────────────────────────────────────────────────

const CELL_W = 260
const CELL_H = 150
const GAP_H = 80
const GAP_V = 120
const COMPACT_WORKSPACE_THRESHOLD = 32
const LAYOUT_TRANSITION = 'transform 560ms cubic-bezier(0.16, 1, 0.3, 1), opacity 260ms ease, filter 260ms ease'

interface GridDisplayNode {
  id: string
  kind: 'view' | 'cluster'
  view: ViewTreeNode
  parentId: string | null
  depth: number
  children: GridDisplayNode[]
  collapsedCount?: number
  dimmed?: boolean
}

function displaySubtreeWidth(node: GridDisplayNode): number {
  if (node.children.length === 0) return 1
  return node.children.reduce((sum, child) => sum + displaySubtreeWidth(child), 0)
}

/**
 * Compute layout positions for real cards plus collapsed cluster cards.
 * Y follows view depth so manual level overrides still read as horizontal bands.
 */
function computeDisplayLayout(roots: GridDisplayNode[]): Map<string, { x: number; y: number }> {
  const positions = new Map<string, { x: number; y: number }>()
  if (roots.length === 0) return positions
  const flat = flattenDisplayTree(roots)

  function layoutNode(node: GridDisplayNode, startCol: number) {
    const w = displaySubtreeWidth(node)
    const centerCol = startCol + (w - 1) / 2
    positions.set(node.id, {
      x: centerCol * (CELL_W + GAP_H),
      y: node.depth * (CELL_H + GAP_V),
    })
    let childStart = startCol
    for (const child of node.children) {
      layoutNode(child, childStart)
      childStart += displaySubtreeWidth(child)
    }
  }

  let col = 0
  for (const root of roots) {
    layoutNode(root, col)
    col += displaySubtreeWidth(root)
  }

  const descendants = new Map<string, Set<string>>()
  const collectDescendants = (node: GridDisplayNode): Set<string> => {
    const set = new Set([node.id])
    node.children.forEach((child) => {
      collectDescendants(child).forEach((id) => set.add(id))
    })
    descendants.set(node.id, set)
    return set
  }
  roots.forEach(collectDescendants)

  const byY = new Map<number, string[]>()
  flat.forEach((node) => {
    const y = node.depth * (CELL_H + GAP_V)
    if (!byY.has(y)) byY.set(y, [])
    byY.get(y)!.push(node.id)
  })

  const sortedYRows = Array.from(byY.entries()).sort(([ya], [yb]) => ya - yb)
  const step = CELL_W + GAP_H
  for (const [rowY, ids] of sortedYRows) {
    const origX = new Map<string, number>(ids.map((id) => [id, positions.get(id)?.x ?? 0]))
    ids.sort((a, b) => (origX.get(a) ?? 0) - (origX.get(b) ?? 0))
    let rightmostX = origX.get(ids[0]) ?? 0

    for (let i = 1; i < ids.length; i++) {
      const originalX = origX.get(ids[i]) ?? 0
      const placedX = Math.max(originalX, rightmostX + step)

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

function countDescendantViews(node: ViewTreeNode): number {
  return node.children.reduce((sum, child) => sum + 1 + countDescendantViews(child), 0)
}

function flattenDisplayTree(roots: GridDisplayNode[]): GridDisplayNode[] {
  const result: GridDisplayNode[] = []
  const visit = (node: GridDisplayNode) => {
    result.push(node)
    node.children.forEach(visit)
  }
  roots.forEach(visit)
  return result
}

function sumContentCounts(
  nodes: ViewTreeNode[],
  countsByView: Record<number, { nodes: number; edges: number }>
): { nodes: number; edges: number } {
  let nodesCount = 0
  let edgesCount = 0

  const visit = (node: ViewTreeNode) => {
    const counts = countsByView[node.id]
    nodesCount += counts?.nodes ?? 0
    edgesCount += counts?.edges ?? 0
    node.children.forEach(visit)
  }

  nodes.forEach(visit)
  return { nodes: nodesCount, edges: edgesCount }
}

function buildDisplayTree(roots: ViewTreeNode[], focusedId: number | null): GridDisplayNode[] {
  const flat = flattenTree(roots)
  if (flat.length <= COMPACT_WORKSPACE_THRESHOLD) {
    const convert = (node: ViewTreeNode, parentId: string | null): GridDisplayNode => ({
      id: String(node.id),
      kind: 'view',
      view: node,
      parentId,
      depth: node.depth,
      children: node.children.map((child) => convert(child, String(node.id))),
    })
    return roots.map((root) => convert(root, null))
  }

  const byId = new Map(flat.map((node) => [node.id, node]))
  const focusedNode = focusedId ? byId.get(focusedId) ?? null : null
  const visible = new Set<number>()
  const emphasis = new Set<number>()

  if (!focusedNode) {
    flat.forEach((node) => {
      if (node.parent_view_id === null || node.depth <= 1) visible.add(node.id)
    })
  } else {
    let cursor: ViewTreeNode | undefined = focusedNode
    while (cursor) {
      visible.add(cursor.id)
      emphasis.add(cursor.id)
      cursor = cursor.parent_view_id ? byId.get(cursor.parent_view_id) : undefined
    }

    roots.forEach((root) => visible.add(root.id))

    const parent = focusedNode.parent_view_id ? byId.get(focusedNode.parent_view_id) : null
    const siblings = flat.filter((node) => node.parent_view_id === focusedNode.parent_view_id)
    siblings.forEach((node) => {
      visible.add(node.id)
      emphasis.add(node.id)
    })

    focusedNode.children.forEach((child) => {
      visible.add(child.id)
      emphasis.add(child.id)
    })

    parent?.children.forEach((child) => visible.add(child.id))
  }

  const makeNode = (node: ViewTreeNode, parentId: string | null): GridDisplayNode | null => {
    if (!visible.has(node.id)) return null

    const displayId = String(node.id)
    const visibleChildren = node.children
      .map((child) => makeNode(child, displayId))
      .filter((child): child is GridDisplayNode => child !== null)

    const hiddenChildren = node.children.filter((child) => !visible.has(child.id))
    const hiddenCount = hiddenChildren.reduce((sum, child) => sum + 1 + countDescendantViews(child), 0)
    const cluster: GridDisplayNode[] = hiddenCount > 0 ? [{
      id: `${node.id}:cluster`,
      kind: 'cluster',
      view: node,
      parentId: displayId,
      depth: node.depth + 1,
      children: [],
      collapsedCount: hiddenCount,
      dimmed: focusedNode ? !emphasis.has(node.id) : false,
    }] : []

    return {
      id: displayId,
      kind: 'view',
      view: node,
      parentId,
      depth: node.depth,
      children: [...visibleChildren, ...cluster],
      dimmed: focusedNode ? !emphasis.has(node.id) : false,
    }
  }

  return roots
    .map((root) => makeNode(root, null))
    .filter((node): node is GridDisplayNode => node !== null)
}

function buildStableLayoutIds(flat: ViewTreeNode[], focusedId: number | null): Set<string> {
  const stable = new Set<string>()
  const byId = new Map(flat.map((node) => [node.id, node]))

  if (focusedId === null) {
    flat.forEach((node) => {
      if (node.parent_view_id === null || node.depth <= 1) stable.add(String(node.id))
    })
    return stable
  }

  const focused = byId.get(focusedId)
  if (!focused) return stable

  let cursor: ViewTreeNode | undefined = focused
  while (cursor) {
    stable.add(String(cursor.id))
    cursor = cursor.parent_view_id ? byId.get(cursor.parent_view_id) : undefined
  }

  flat.forEach((node) => {
    if (node.parent_view_id === focused.parent_view_id) stable.add(String(node.id))
  })

  return stable
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
  const [gridViewIds, setGridViewIds] = useState<Set<number> | null>(null)
  const roots = useMemo(() => filterTreeForGrid(treeData, gridViewIds), [treeData, gridViewIds])
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

  // Fetch visible grid cards and node/edge counts in one workspace roundtrip.
  useEffect(() => {
    let cancelled = false
    if (treeData.length === 0) {
      setGridViewIds(new Set())
      setCountsByDiagram({})
      return
    }
    ; (async () => {
      try {
        const workspace = await api.workspace.views.gridData()
        if (cancelled) return

        const visibleIds = new Set(flattenTree(workspace.views).map((view) => view.id))
        const next: Record<number, { nodes: number; edges: number }> = {}

        visibleIds.forEach((id) => {
          const content = workspace.content[id]
          next[id] = {
            nodes: content?.placements.length ?? 0,
            edges: content?.connectors.length ?? 0,
          }
        })

        setGridViewIds(visibleIds)
        setCountsByDiagram(next)
      } catch {
        if (!cancelled) {
          setGridViewIds(null)
          setCountsByDiagram({})
        }
      }
    })()
    return () => { cancelled = true }
  }, [treeData])

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
  const displayTree = useMemo(
    () => buildDisplayTree(roots, focusedId),
    [roots, focusedId]
  )
  const displayFlat = useMemo(() => flattenDisplayTree(displayTree), [displayTree])
  const rawLayoutPositions = useMemo(() => computeDisplayLayout(displayTree), [displayTree])
  const stableLayoutIds = useMemo(() => buildStableLayoutIds(flatTree, focusedId), [flatTree, focusedId])
  const previousLayoutRef = useRef<Map<string, { x: number; y: number }>>(new Map())

  const layoutPositions = useMemo(() => {
    const next = new Map(rawLayoutPositions)
    const previousLayout = previousLayoutRef.current

    if (focusedId !== null) {
      const focusedKey = String(focusedId)
      const previousFocusedPosition = previousLayout.get(focusedKey)
      const nextFocusedPosition = next.get(focusedKey)

      if (previousFocusedPosition && nextFocusedPosition) {
        const dx = previousFocusedPosition.x - nextFocusedPosition.x
        const dy = previousFocusedPosition.y - nextFocusedPosition.y

        if (dx !== 0 || dy !== 0) {
          next.forEach((position, id) => {
            next.set(id, { x: position.x + dx, y: position.y + dy })
          })
        }
      }
    }

    stableLayoutIds.forEach((id) => {
      const previousPosition = previousLayout.get(id)
      if (previousPosition && next.has(id)) {
        next.set(id, previousPosition)
      }
    })

    return next
  }, [rawLayoutPositions, focusedId, stableLayoutIds])

  useEffect(() => {
    previousLayoutRef.current = layoutPositions
  }, [layoutPositions])

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
    displayFlat.map((displayNode): RFNode => {
      const n = displayNode.view
      const isCluster = displayNode.kind === 'cluster'
      const hiddenChildren = isCluster
        ? n.children.filter((child) => !displayFlat.some((visibleNode) => visibleNode.kind === 'view' && visibleNode.view.id === child.id))
        : []

      return {
        id: displayNode.id,
        type: 'diagramGrid',
        position: layoutPositions.get(displayNode.id) ?? { x: 0, y: 0 },
        data: {
          id: n.id,
          name: isCluster ? `${n.name} descendants` : n.name,
          level_label: isCluster ? 'Collapsed stack' : n.level_label,
          counts: isCluster ? sumContentCounts(hiddenChildren, countsByView) : countsByView[n.id],
          kind: displayNode.kind,
          collapsedCount: displayNode.collapsedCount,
          dimmed: displayNode.dimmed,
          focused: !isCluster && focusedId === n.id,
          canEdit: !isCluster && canEdit,
          isEditing: !isCluster && editingId === n.id,
          editName,
          onFocus: () => onFocusChange(n.id),
          onOpen: () => isCluster ? onFocusChange(n.id) : navigate(`/views/${n.id}`),
          onStartRename: () => startEdit(n.id, n.name),
          onDetails: () => handleDetailsOpen(n.id),
          onDelete: () => { setDeleteTargetId(n.id); onDeleteOpen() },
          onShare: onShare ? () => onShare(n.id) : () => {},
          onEditNameChange: setEditName,
          onEditCommit: commitEdit,
          onEditCancel: cancelEdit,
          isMobile: isMobileLayout,
          wasdKey: isCluster ? undefined : wasdTargets[n.id],
        } satisfies ViewGridNodeData,
        draggable: false,
        style: {
          transition: LAYOUT_TRANSITION,
          zIndex: displayNode.kind === 'cluster' ? 1 : focusedId === n.id ? 3 : 2,
        },
      }
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [displayFlat, layoutPositions, focusedId, countsByView,
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
    displayFlat
      .filter((n) => n.parentId)
      .map((n) => ({
        id: `${n.parentId}-${n.id}`,
        source: n.parentId!,
        target: n.id,
        type: 'floating',
        animated: false,
        data: { color: n.kind === 'cluster' ? hexToRgba(accent, 0.28) : HIERARCHY_EDGE_COLOR, dashed: n.kind === 'cluster' },
      })),
    [displayFlat, accent]
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
    const pos = layoutPositions.get(String(focusedId))
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
              background: 'var(--bg-canvas)'
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
