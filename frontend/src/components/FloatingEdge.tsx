import { memo, useState } from 'react'
import { useStore, getStraightPath, type EdgeProps } from 'reactflow'

function getNodeBorderPoint(
  node: { positionAbsolute?: { x: number; y: number }; width?: number | null; height?: number | null },
  targetCenter: { x: number; y: number }
) {
  const w = (node.width ?? 0) / 2
  const h = (node.height ?? 0) / 2
  const cx = (node.positionAbsolute?.x ?? 0) + w
  const cy = (node.positionAbsolute?.y ?? 0) + h
  const dx = targetCenter.x - cx
  const dy = targetCenter.y - cy
  if (dx === 0 && dy === 0) return { x: cx, y: cy }
  const scaleX = dx !== 0 ? Math.abs(w / dx) : Infinity
  const scaleY = dy !== 0 ? Math.abs(h / dy) : Infinity
  const scale = Math.min(scaleX, scaleY)
  return { x: cx + dx * scale, y: cy + dy * scale }
}

export interface FloatingConnectorData {
  color: string
  /** true = portal/navigational link (dotted), false = hierarchy (solid) */
  dashed?: boolean
}

function FloatingConnector({
  source,
  target,
  data,
  selected,
}: EdgeProps<FloatingConnectorData>) {
  const [hovered, setHovered] = useState(false)
  const sourceNode = useStore((s) => s.nodeInternals.get(source))
  const targetNode = useStore((s) => s.nodeInternals.get(target))

  if (
    !sourceNode?.positionAbsolute || !targetNode?.positionAbsolute ||
    !isFinite(sourceNode.positionAbsolute.x) || !isFinite(sourceNode.positionAbsolute.y) ||
    !isFinite(targetNode.positionAbsolute.x) || !isFinite(targetNode.positionAbsolute.y)
  ) return null

  const sourceCx = (sourceNode.positionAbsolute.x ?? 0) + (sourceNode.width ?? 0) / 2
  const sourceCy = (sourceNode.positionAbsolute.y ?? 0) + (sourceNode.height ?? 0) / 2
  const targetCx = (targetNode.positionAbsolute.x ?? 0) + (targetNode.width ?? 0) / 2
  const targetCy = (targetNode.positionAbsolute.y ?? 0) + (targetNode.height ?? 0) / 2

  const sourcePoint = getNodeBorderPoint(sourceNode, { x: targetCx, y: targetCy })
  const targetPoint = getNodeBorderPoint(targetNode, { x: sourceCx, y: sourceCy })

  const [connectorPath] = getStraightPath({
    sourceX: sourcePoint.x,
    sourceY: sourcePoint.y,
    targetX: targetPoint.x,
    targetY: targetPoint.y,
  })

  const color = data?.color ?? '#718096'
  const isPortal = data?.dashed ?? false
  const active = hovered || !!selected

  return (
    <g>
      {/* Main stroke */}
      {isPortal ? (
        /* Portal: fine rounded dots - distinct from hierarchy */
        <path
          d={connectorPath}
          fill="none"
          stroke={color}
          strokeWidth={active ? 1.5 : 1}
          strokeDasharray="1.5 7"
          strokeLinecap="round"
          opacity={active ? 0.9 : 0.6}
          style={{ transition: 'opacity 0.15s ease, stroke-width 0.15s ease' }}
        />
      ) : (
        /* Hierarchy: solid line */
        <path
          d={connectorPath}
          fill="none"
          stroke={color}
          strokeWidth={active ? 1.5 : 1}
          opacity={active ? 0.85 : 0.6}
          style={{ transition: 'opacity 0.15s ease, stroke-width 0.15s ease' }}
        />
      )}

      {/* Source terminus dot - hierarchy only, signals the origin node */}
      {!isPortal && (
        <circle
          cx={sourcePoint.x}
          cy={sourcePoint.y}
          r={active ? 2.5 : 2}
          fill={color}
          opacity={active ? 0.85 : 0.55}
          style={{ transition: 'r 0.15s ease, opacity 0.15s ease', pointerEvents: 'none' }}
        />
      )}

      {/* Wide transparent hit area for hover detection */}
      <path
        d={connectorPath}
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
