import { useMemo } from 'react'
import { Box, Flex, Text } from '@chakra-ui/react'
import ReactFlow, { ConnectionMode, MarkerType, type Edge, type Node } from 'reactflow'
import ElementNode from '../ElementNode'
import ViewBezierConnector from '../ViewBezierConnector'
import type { Connector, LibraryElement } from '../../types'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getLogicalHandleId,
  getVisualHandleIdForGroup,
  type LogicalHandleSide,
} from '../../utils/edgeDistribution'

interface ConnectorRelationshipData {
  connector: Connector
  sourceEl: LibraryElement | undefined
  targetEl: LibraryElement | undefined
  sourcePlacement?: { x: number; y: number }
  targetPlacement?: { x: number; y: number }
}

interface ConnectorInspectorProps {
  data: ConnectorRelationshipData
  cardShadow: string
  onSelectRow: (key: string) => void
}

const NODE_WIDTH = 220
const NODE_HEIGHT = 120
const SCENE_WIDTH = 760
const SCENE_HEIGHT = 340
const MIN_CENTER_DISTANCE = 240

function sideVector(side: LogicalHandleSide) {
  if (side === 'left') return { x: -1, y: 0 }
  if (side === 'right') return { x: 1, y: 0 }
  if (side === 'top') return { x: 0, y: -1 }
  return { x: 0, y: 1 }
}

function getNodeOrigins(sourceSide: LogicalHandleSide, targetSide: LogicalHandleSide) {
  const sourceDir = sideVector(sourceSide)
  const targetDir = sideVector(targetSide)
  const preferred = { x: sourceDir.x - targetDir.x, y: sourceDir.y - targetDir.y }

  const signX = preferred.x === 0 && preferred.y === 0 ? 1 : Math.sign(preferred.x)
  const signY = preferred.x === 0 && preferred.y === 0 ? 0 : Math.sign(preferred.y)

  const spanX = signX !== 0 && signY !== 0 ? 220 : 260
  const spanY = signX !== 0 && signY !== 0 ? 120 : 0
  const verticalSpan = signY !== 0 && signX === 0 ? 190 : spanY

  const centerX = SCENE_WIDTH / 2
  const centerY = SCENE_HEIGHT / 2

  const sourceCenter = {
    x: centerX - signX * spanX / 2,
    y: centerY - signY * verticalSpan / 2,
  }
  const targetCenter = {
    x: centerX + signX * spanX / 2,
    y: centerY + signY * verticalSpan / 2,
  }

  return {
    source: { x: sourceCenter.x - NODE_WIDTH / 2, y: sourceCenter.y - NODE_HEIGHT / 2 },
    target: { x: targetCenter.x - NODE_WIDTH / 2, y: targetCenter.y - NODE_HEIGHT / 2 },
  }
}

function getNodeOriginsFromPlacements(
  sourcePlacement: { x: number; y: number },
  targetPlacement: { x: number; y: number },
) {
  const dx = targetPlacement.x - sourcePlacement.x
  const dy = targetPlacement.y - sourcePlacement.y
  const distance = Math.hypot(dx, dy)
  if (distance < 1) return null

  const spanX = Math.abs(dx)
  const spanY = Math.abs(dy)
  const availableX = SCENE_WIDTH - NODE_WIDTH - 48
  const availableY = SCENE_HEIGHT - NODE_HEIGHT - 32
  const scaleX = spanX > 0 ? availableX / spanX : 1
  const scaleY = spanY > 0 ? availableY / spanY : 1
  const scale = Math.max(0.5, Math.min(scaleX, scaleY, 1.35))

  const sourceX = sourcePlacement.x * scale
  const sourceY = sourcePlacement.y * scale
  const targetX = targetPlacement.x * scale
  const targetY = targetPlacement.y * scale

  const minX = Math.min(sourceX, targetX)
  const maxX = Math.max(sourceX, targetX)
  const minY = Math.min(sourceY, targetY)
  const maxY = Math.max(sourceY, targetY)

  const offsetX = (SCENE_WIDTH - (maxX - minX) - NODE_WIDTH) / 2 - minX
  const offsetY = (SCENE_HEIGHT - (maxY - minY) - NODE_HEIGHT) / 2 - minY

  let source = { x: sourceX + offsetX, y: sourceY + offsetY }
  let target = { x: targetX + offsetX, y: targetY + offsetY }

  const sourceCenter = { x: source.x + NODE_WIDTH / 2, y: source.y + NODE_HEIGHT / 2 }
  const targetCenter = { x: target.x + NODE_WIDTH / 2, y: target.y + NODE_HEIGHT / 2 }
  const centerDx = targetCenter.x - sourceCenter.x
  const centerDy = targetCenter.y - sourceCenter.y
  const centerDistance = Math.hypot(centerDx, centerDy)

  if (centerDistance > 0 && centerDistance < MIN_CENTER_DISTANCE) {
    const push = (MIN_CENTER_DISTANCE - centerDistance) / 2
    const ux = centerDx / centerDistance
    const uy = centerDy / centerDistance
    source = { x: source.x - ux * push, y: source.y - uy * push }
    target = { x: target.x + ux * push, y: target.y + uy * push }
  }

  return { source, target }
}

export function ConnectorInspector({ data, cardShadow: _cardShadow, onSelectRow }: ConnectorInspectorProps) {
  const sourceSide = getLogicalHandleId(data.connector.source_handle, DEFAULT_SOURCE_HANDLE_SIDE) ?? DEFAULT_SOURCE_HANDLE_SIDE
  const targetSide = getLogicalHandleId(data.connector.target_handle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE
  const origins =
    (data.sourcePlacement && data.targetPlacement
      ? getNodeOriginsFromPlacements(data.sourcePlacement, data.targetPlacement)
      : null)
    ?? getNodeOrigins(sourceSide, targetSide)

  const sourceHandle = getVisualHandleIdForGroup(sourceSide, 0, 1)
  const targetHandle = getVisualHandleIdForGroup(targetSide, 0, 1)

  const nodeTypes = useMemo(() => ({ elementNode: ElementNode }), [])
  const edgeTypes = useMemo(() => ({ default: ViewBezierConnector }), [])

  const sourceEl = data.sourceEl
  const targetEl = data.targetEl

  const sourceNode: Node | null = sourceEl
    ? {
      id: String(sourceEl.id),
      type: 'elementNode',
      position: origins.source,
      style: { width: NODE_WIDTH, height: NODE_HEIGHT },
      selected: false,
      data: {
        id: sourceEl.id,
        view_id: data.connector.view_id,
        element_id: sourceEl.id,
        position_x: origins.source.x,
        position_y: origins.source.y,
        name: sourceEl.name,
        description: sourceEl.description,
        kind: sourceEl.kind,
        technology: sourceEl.technology,
        url: sourceEl.url,
        logo_url: sourceEl.logo_url,
        technology_connectors: sourceEl.technology_connectors,
        tags: sourceEl.tags,
        repo: sourceEl.repo,
        branch: sourceEl.branch,
        file_path: sourceEl.file_path,
        language: sourceEl.language,
        bypass_noise_gate: sourceEl.bypass_noise_gate ?? false,
        has_view: false,
        view_label: sourceEl.view_label,
        links: [],
        parentLinks: [],
        onZoomIn: () => Promise.resolve(),
        onZoomOut: () => Promise.resolve(),
        onNavigateToDiagram: () => undefined,
        onSelect: () => onSelectRow(`element:${sourceEl.id}`),
        onInteractionStart: () => undefined,
        onConnectTo: () => Promise.resolve(),
        onRemove: () => Promise.resolve(),
        onHoverZoom: () => undefined,
        isZoomHovered: null,
        interactionSourceId: null,
        isCanvasMoving: false,
        connectedHandleIds: [sourceHandle],
        selectedHandleIds: [],
        reconnectCandidates: [],
        isConnectorHighlighted: false,
        tagColors: {},
      },
    }
    : null

  const targetNode: Node | null = targetEl
    ? {
      id: String(targetEl.id),
      type: 'elementNode',
      position: origins.target,
      style: { width: NODE_WIDTH, height: NODE_HEIGHT },
      selected: false,
      data: {
        id: targetEl.id,
        view_id: data.connector.view_id,
        element_id: targetEl.id,
        position_x: origins.target.x,
        position_y: origins.target.y,
        name: targetEl.name,
        description: targetEl.description,
        kind: targetEl.kind,
        technology: targetEl.technology,
        url: targetEl.url,
        logo_url: targetEl.logo_url,
        technology_connectors: targetEl.technology_connectors,
        tags: targetEl.tags,
        repo: targetEl.repo,
        branch: targetEl.branch,
        file_path: targetEl.file_path,
        language: targetEl.language,
        bypass_noise_gate: targetEl.bypass_noise_gate ?? false,
        has_view: false,
        view_label: targetEl.view_label,
        links: [],
        parentLinks: [],
        onZoomIn: () => Promise.resolve(),
        onZoomOut: () => Promise.resolve(),
        onNavigateToDiagram: () => undefined,
        onSelect: () => onSelectRow(`element:${targetEl.id}`),
        onInteractionStart: () => undefined,
        onConnectTo: () => Promise.resolve(),
        onRemove: () => Promise.resolve(),
        onHoverZoom: () => undefined,
        isZoomHovered: null,
        interactionSourceId: null,
        isCanvasMoving: false,
        connectedHandleIds: [targetHandle],
        selectedHandleIds: [],
        reconnectCandidates: [],
        isConnectorHighlighted: false,
        tagColors: {},
      },
    }
    : null

  const nodes: Node[] = [sourceNode, targetNode].filter((node): node is Node => Boolean(node))
  const edges: Edge[] = sourceNode && targetNode
    ? [
      {
        id: String(data.connector.id),
        source: sourceNode.id,
        target: targetNode.id,
        sourceHandle,
        targetHandle,
        type: 'default',
        label: '',
        data: {
          ...data.connector,
          sourceGroupIndex: 0,
          sourceGroupCount: 1,
          targetGroupIndex: 0,
          targetGroupCount: 1,
          sourceHandleSide: sourceSide,
          targetHandleSide: targetSide,
        },
        style: { stroke: 'var(--accent)', strokeWidth: 2, opacity: 0.9, pointerEvents: 'none' },
        markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14, color: 'var(--accent)' },
      },
    ]
    : []

  return (
    <Flex align="center" justify="center" w="full" h="full" position="relative">
      <Box
        position="relative"
        w="100%"
        h="100%"
        minH="220px"
        data-pan-block="true"
      >
        <Box
          position="absolute"
          inset={0}
          sx={{
            '[data-testid="vieweditor-node-zoom-in"], [data-testid="vieweditor-node-zoom-out"]': { visibility: 'hidden', pointerEvents: 'none' },
            '.react-flow__handle': { visibility: 'hidden', pointerEvents: 'none' },
            '.react-flow__pane': { cursor: 'default !important' },
          }}
        >
          <ReactFlow
            key={data.connector.id}
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            connectionMode={ConnectionMode.Loose}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable={true}
            selectNodesOnDrag={false}
            zoomOnScroll={false}
            zoomOnPinch={false}
            zoomOnDoubleClick={false}
            panOnDrag={false}
            panOnScroll={false}
            fitView
            fitViewOptions={{ padding: 0.22 }}
            proOptions={{ hideAttribution: true }}
            onNodeClick={(_e, node) => {
              onSelectRow(`element:${node.id}`)
            }}
            style={{ background: 'transparent' }}
          />
        </Box>

        {!data.sourceEl && (
          <Text color="gray.600" fontSize="xs" fontStyle="italic" position="absolute" left="12px" top="50%">
            Unknown Source
          </Text>
        )}
        {!data.targetEl && (
          <Text color="gray.600" fontSize="xs" fontStyle="italic" position="absolute" right="12px" top="50%">
            Unknown Target
          </Text>
        )}
      </Box>
    </Flex>
  )
}
