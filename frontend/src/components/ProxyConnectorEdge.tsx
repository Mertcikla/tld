import { memo } from 'react'
import { BaseEdge, EdgeLabelRenderer, useStore, type EdgeProps } from 'reactflow'
import { useEdgeLabelLayout } from './ViewEditorEdgeLabelLayout'

function getIntersectionPoint(
  nx: number, ny: number, nw: number, nh: number,
  tx: number, ty: number, tw: number, th: number,
) {
  const cx = nx + nw / 2
  const cy = ny + nh / 2
  const cx2 = tx + tw / 2
  const cy2 = ty + th / 2
  const dx = cx2 - cx
  const dy = cy2 - cy
  const hw = nw / 2
  const hh = nh / 2

  if (dx === 0 && dy === 0) return { x: cx, y: cy }

  const tanTheta = Math.abs(dy / dx)
  const boxRatio = hh / hw

  if (tanTheta < boxRatio) {
    return {
      x: cx + Math.sign(dx) * hw,
      y: cy + Math.sign(dx) * hw * (dy / dx),
    }
  }

  return {
    y: cy + Math.sign(dy) * hh,
    x: cx + Math.sign(dy) * hh * (dx / dy),
  }
}

function scaleContextNodeBox(node: {
  positionAbsolute?: { x: number; y: number }
  position: { x: number; y: number }
  width?: number | null
  height?: number | null
  type?: string
} | undefined) {
  let x = node?.positionAbsolute?.x ?? node?.position.x ?? 0
  let y = node?.positionAbsolute?.y ?? node?.position.y ?? 0
  let w = node?.width ?? 0
  let h = node?.height ?? 0

  if (node?.type === 'contextNeighborNode') {
    const scale = 0.5
    x += w * ((1 - scale) / 2)
    y += h * ((1 - scale) / 2)
    w *= scale
    h *= scale
  }

  return { x, y, w, h }
}

function ProxyConnectorEdge({ id, source, target, selected, style }: EdgeProps) {
  const sourceNode = useStore((s) => s.nodeInternals.get(source))
  const targetNode = useStore((s) => s.nodeInternals.get(target))
  const edge = useStore((s) => s.edges.find((candidate) => candidate.id === id))
  const sourceBox = scaleContextNodeBox(sourceNode)
  const targetBox = scaleContextNodeBox(targetNode)
  const hasBoxes = !!(sourceBox.w && sourceBox.h && targetBox.w && targetBox.h)
  const start = hasBoxes
    ? getIntersectionPoint(sourceBox.x, sourceBox.y, sourceBox.w, sourceBox.h, targetBox.x, targetBox.y, targetBox.w, targetBox.h)
    : { x: 0, y: 0 }
  const end = hasBoxes
    ? getIntersectionPoint(targetBox.x, targetBox.y, targetBox.w, targetBox.h, sourceBox.x, sourceBox.y, sourceBox.w, sourceBox.h)
    : { x: 1, y: 0 }

  let finalStart = start
  let finalEnd = end

  const { sourceGroupIndex, sourceGroupCount, targetGroupIndex, targetGroupCount } = (edge?.data as {
    sourceGroupIndex?: number; sourceGroupCount?: number;
    targetGroupIndex?: number; targetGroupCount?: number;
  }) || {}

  const dx = end.x - start.x
  const dy = end.y - start.y
  const len = Math.hypot(dx, dy)
  const PADDING = 12

  if (len > 0) {
    const nx = -dy / len
    const ny = dx / len

    if (typeof sourceGroupIndex === 'number' && typeof sourceGroupCount === 'number' && sourceGroupCount > 1) {
      const offset = (sourceGroupIndex - (sourceGroupCount - 1) / 2) * PADDING
      finalStart = { x: start.x + nx * offset, y: start.y + ny * offset }
    }

    if (typeof targetGroupIndex === 'number' && typeof targetGroupCount === 'number' && targetGroupCount > 1) {
      const offset = (targetGroupIndex - (targetGroupCount - 1) / 2) * PADDING
      finalEnd = { x: end.x + nx * offset, y: end.y + ny * offset }
    }
  }


  const labelLayout = useEdgeLabelLayout({
    id,
    preferredX: (finalStart.x + finalEnd.x) / 2,
    preferredY: (finalStart.y + finalEnd.y) / 2,
    width: 24,
    height: 24,
    dx: finalEnd.x - finalStart.x,
    dy: finalEnd.y - finalStart.y,
    alongLineOnly: true,
  })

  if (!sourceNode || !targetNode) return null
  if (!sourceBox.w || !sourceBox.h || !targetBox.w || !targetBox.h) return null

  const path = `M ${finalStart.x},${finalStart.y} L ${finalEnd.x},${finalEnd.y}`

  const count = edge?.data && typeof (edge.data as { details?: { count?: number } }).details?.count === 'number'
    ? (edge.data as { details: { count: number } }).details.count
    : 1

  return (
    <>
      <BaseEdge
        id={id}
        path={path}
        style={{
          ...style,
          stroke: 'rgba(var(--accent-rgb), 0.8)',
          strokeWidth: selected ? 1 : 1,
          strokeDasharray: '1 4',
          animation: 'none',
        }}
      />
      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelLayout.x}px, ${labelLayout.y}px)`,
            pointerEvents: 'none',
            opacity: style?.opacity,
            zIndex: 2,
          }}
        >
          <div
            style={{
              width: 24,
              height: 24,
              borderRadius: 999,
              background: 'var(--bg-element)',
              border: '1px dashed rgba(var(--accent-rgb), 0.8)',
              color: 'white',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 11,
              fontWeight: 600,
              boxShadow: selected ? '0 0 0 1px rgba(255,255,255,0.2)' : 'none',
            }}
          >
            {count}
          </div>
        </div>
      </EdgeLabelRenderer>
    </>
  )
}

export default memo(ProxyConnectorEdge)
