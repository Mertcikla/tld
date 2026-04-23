import { memo } from 'react'
import { EdgeProps, getStraightPath, useStore } from 'reactflow'

function getIntersectionPoint(
  nx: number, ny: number, nw: number, nh: number,
  tx: number, ty: number, tw: number, th: number
) {
  // Center of node
  const cx = nx + nw / 2
  const cy = ny + nh / 2
  // Center of target
  const cx2 = tx + tw / 2
  const cy2 = ty + th / 2

  const dx = cx2 - cx
  const dy = cy2 - cy

  // Intersection with the boundary of node (nx, ny, nw, nh)
  const hw = nw / 2
  const hh = nh / 2

  if (dx === 0 && dy === 0) return { x: cx, y: cy }

  const tanTheta = Math.abs(dy / dx)
  const boxRatio = hh / hw

  let ix, iy
  if (tanTheta < boxRatio) {
    // Left or right border
    ix = cx + Math.sign(dx) * hw
    iy = cy + Math.sign(dx) * hw * (dy / dx)
  } else {
    // Top or bottom border
    iy = cy + Math.sign(dy) * hh
    ix = cx + Math.sign(dy) * hh * (dx / dy)
  }

  return { x: ix, y: iy }
}
function ContextStraightConnector({
  id,
  source,
  target,
  style,
  markerEnd,
  markerStart,
  data,
}: EdgeProps) {
  const sourceNode = useStore((s) => s.nodeInternals.get(source))
  const targetNode = useStore((s) => s.nodeInternals.get(target))

  if (!sourceNode || !targetNode) {
    return null
  }

  let sx = sourceNode.positionAbsolute?.x ?? sourceNode.position.x
  let sy = sourceNode.positionAbsolute?.y ?? sourceNode.position.y
  let sw = sourceNode.width ?? 0
  let sh = sourceNode.height ?? 0

  let tx = targetNode.positionAbsolute?.x ?? targetNode.position.x
  let ty = targetNode.positionAbsolute?.y ?? targetNode.position.y
  let tw = targetNode.width ?? 0
  let th = targetNode.height ?? 0

  if (!sw || !sh || !tw || !th) return null

  if (sourceNode.type === 'contextNeighborNode') {
    const scale = 0.5
    sx = sx + sw * ((1 - scale) / 2)
    sy = sy + sh * ((1 - scale) / 2)
    sw = sw * scale
    sh = sh * scale
  }

  if (targetNode.type === 'contextNeighborNode') {
    const scale = 0.5
    tx = tx + tw * ((1 - scale) / 2)
    ty = ty + th * ((1 - scale) / 2)
    tw = tw * scale
    th = th * scale
  }

  // Shortest distance: from center of source to center of target, intersect with their borders.
  const startPoint = getIntersectionPoint(sx, sy, sw, sh, tx, ty, tw, th)
  const endPoint = getIntersectionPoint(tx, ty, tw, th, sx, sy, sw, sh)


  let finalStartPoint = startPoint
  let finalEndPoint = endPoint

  const { sourceGroupIndex, sourceGroupCount, targetGroupIndex, targetGroupCount } = (data as {
    sourceGroupIndex?: number; sourceGroupCount?: number;
    targetGroupIndex?: number; targetGroupCount?: number;
  }) || {}

  const dx = endPoint.x - startPoint.x
  const dy = endPoint.y - startPoint.y
  const len = Math.hypot(dx, dy)
  const PADDING = 12

  if (len > 0) {
    const nx = -dy / len
    const ny = dx / len

    if (typeof sourceGroupIndex === 'number' && typeof sourceGroupCount === 'number' && sourceGroupCount > 1) {
      const offset = (sourceGroupIndex - (sourceGroupCount - 1) / 2) * PADDING
      finalStartPoint = { x: startPoint.x + nx * offset, y: startPoint.y + ny * offset }
    }

    if (typeof targetGroupIndex === 'number' && typeof targetGroupCount === 'number' && targetGroupCount > 1) {
      const offset = (targetGroupIndex - (targetGroupCount - 1) / 2) * PADDING
      finalEndPoint = { x: endPoint.x + nx * offset, y: endPoint.y + ny * offset }
    }
  }


  const [connectorPath] = getStraightPath({
    sourceX: finalStartPoint.x,
    sourceY: finalStartPoint.y,
    targetX: finalEndPoint.x,
    targetY: finalEndPoint.y,
  })


  return (
    <>
      <path
        id={id}
        className="react-flow__connector-path"
        d={connectorPath}
        markerEnd={markerEnd}
        markerStart={markerStart}
        style={{
          ...style,
          pointerEvents: 'none',
          strokeDasharray: '5,5',
        }}
      />
    </>
  )
}

export default memo(ContextStraightConnector)
