import { memo, useState } from 'react'
import { useStore, type Edge, type EdgeProps } from 'reactflow'

export interface FloatingConnectorData {
  color: string
  /** true = portal/navigational link (dotted), false = hierarchy (solid) */
  dashed?: boolean
}

type FlowNode = {
  positionAbsolute?: { x: number; y: number }
  width?: number | null
  height?: number | null
}

type Point = { x: number; y: number }
type RouteDirection = 'down' | 'up'

interface OrthogonalRoute {
  source: Point
  target: Point
  busY: number
  direction: RouteDirection
}

interface BundleMember {
  id: string
  target: Point
}

const GRID_BUNDLE_OFFSET = 52
const MIN_BUNDLE_OFFSET = 26

function nodeCenter(node: FlowNode) {
  return {
    x: (node.positionAbsolute?.x ?? 0) + (node.width ?? 0) / 2,
    y: (node.positionAbsolute?.y ?? 0) + (node.height ?? 0) / 2,
  }
}

function getOrthogonalRoute(sourceNode: FlowNode, targetNode: FlowNode): OrthogonalRoute {
  const sourceCenter = nodeCenter(sourceNode)
  const targetCenter = nodeCenter(targetNode)
  const sourceHeight = sourceNode.height ?? 0
  const targetHeight = targetNode.height ?? 0
  const direction: RouteDirection = targetCenter.y >= sourceCenter.y ? 'down' : 'up'
  const source = {
    x: sourceCenter.x,
    y: direction === 'down'
      ? (sourceNode.positionAbsolute?.y ?? 0) + sourceHeight
      : (sourceNode.positionAbsolute?.y ?? 0),
  }
  const target = {
    x: targetCenter.x,
    y: direction === 'down'
      ? (targetNode.positionAbsolute?.y ?? 0)
      : (targetNode.positionAbsolute?.y ?? 0) + targetHeight,
  }
  const availableGap = Math.abs(target.y - source.y)
  const offset = Math.max(MIN_BUNDLE_OFFSET, Math.min(GRID_BUNDLE_OFFSET, availableGap / 2))

  return {
    source,
    target,
    direction,
    busY: direction === 'down' ? source.y + offset : source.y - offset,
  }
}

function linePath(points: Point[]) {
  if (points.length === 0) return ''
  return points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x},${point.y}`).join(' ')
}

function edgeStyleKey(edge: Edge<FloatingConnectorData>) {
  return `${edge.data?.dashed ? 'dashed' : 'solid'}:${edge.data?.color ?? ''}`
}

function FloatingConnector({
  id,
  source,
  target,
  data,
  selected,
}: EdgeProps<FloatingConnectorData>) {
  const [hovered, setHovered] = useState(false)
  const edges = useStore((s) => s.edges as Edge<FloatingConnectorData>[])
  const nodeInternals = useStore((s) => s.nodeInternals)
  const sourceNode = nodeInternals.get(source)
  const targetNode = nodeInternals.get(target)

  if (
    !sourceNode?.positionAbsolute || !targetNode?.positionAbsolute ||
    !isFinite(sourceNode.positionAbsolute.x) || !isFinite(sourceNode.positionAbsolute.y) ||
    !isFinite(targetNode.positionAbsolute.x) || !isFinite(targetNode.positionAbsolute.y)
  ) return null

  const route = getOrthogonalRoute(sourceNode, targetNode)
  const styleKey = `${data?.dashed ? 'dashed' : 'solid'}:${data?.color ?? ''}`
  const members = edges
    .filter((edge): edge is Edge<FloatingConnectorData> => (
      edge.type === 'floating' &&
      edge.source === source &&
      edgeStyleKey(edge) === styleKey
    ))
    .map((edge) => {
      const edgeTargetNode = nodeInternals.get(edge.target)
      if (!edgeTargetNode?.positionAbsolute) return null
      const edgeRoute = getOrthogonalRoute(sourceNode, edgeTargetNode)
      if (edgeRoute.direction !== route.direction) return null
      return { id: edge.id, target: edgeRoute.target }
    })
    .filter((member): member is BundleMember => member !== null)
    .sort((a, b) => a.target.x - b.target.x || a.target.y - b.target.y || a.id.localeCompare(b.id))

  const isBundleRepresentative = members[0]?.id === id
  const isBundled = members.length > 1
  const busXs = isBundled
    ? [route.source.x, ...members.map((member) => member.target.x)]
    : [route.source.x, route.target.x]
  const busStartX = Math.min(...busXs)
  const busEndX = Math.max(...busXs)
  const connectorPath = isBundled
    ? linePath([
      { x: route.target.x, y: route.busY },
      route.target,
    ])
    : linePath([
      route.source,
      { x: route.source.x, y: route.busY },
      { x: route.target.x, y: route.busY },
      route.target,
    ])
  const sharedPath = isBundled && isBundleRepresentative
    ? linePath([
      route.source,
      { x: route.source.x, y: route.busY },
      { x: busStartX, y: route.busY },
      { x: busEndX, y: route.busY },
    ])
    : ''
  const hitPath = sharedPath ? `${sharedPath} ${connectorPath}` : connectorPath

  const color = data?.color ?? '#718096'
  const isPortal = data?.dashed ?? false
  const active = hovered || !!selected
  const strokeWidth = isBundled && isBundleRepresentative
    ? active ? 1.8 : 1.35
    : active ? 1.5 : 1

  return (
    <g>
      {sharedPath && (
        <path
          d={sharedPath}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray={isPortal ? '1.5 7' : undefined}
          strokeLinecap="round"
          strokeLinejoin="round"
          opacity={active ? 0.9 : 0.62}
          style={{ transition: 'opacity 0.15s ease, stroke-width 0.15s ease' }}
        />
      )}

      {/* Main stroke */}
      {isPortal ? (
        /* Portal: fine rounded dots - distinct from hierarchy */
        <path
          d={connectorPath}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeDasharray="1.5 7"
          strokeLinecap="round"
          strokeLinejoin="round"
          opacity={active ? 0.9 : 0.6}
          style={{ transition: 'opacity 0.15s ease, stroke-width 0.15s ease' }}
        />
      ) : (
        /* Hierarchy: solid line */
        <path
          d={connectorPath}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeLinejoin="round"
          opacity={active ? 0.85 : 0.6}
          style={{ transition: 'opacity 0.15s ease, stroke-width 0.15s ease' }}
        />
      )}

      {/* Source terminus dot - hierarchy only, signals the origin node */}
      {!isPortal && (!isBundled || isBundleRepresentative) && (
        <circle
          cx={route.source.x}
          cy={route.source.y}
          r={active ? 2.5 : 2}
          fill={color}
          opacity={active ? 0.85 : 0.55}
          style={{ transition: 'r 0.15s ease, opacity 0.15s ease', pointerEvents: 'none' }}
        />
      )}

      {/* Wide transparent hit area for hover detection */}
      <path
        d={hitPath}
        fill="none"
        stroke="transparent"
        strokeWidth={16}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        style={{ cursor: 'default' }}
      />
    </g>
  )
}

export default memo(FloatingConnector)
