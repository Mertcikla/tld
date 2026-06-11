import { Position } from 'reactflow'
import type { LogicalHandleSide } from './edgeDistribution'

const CURVATURE = 0.5

export type ConnectorRouteStyle = 'bezier' | 'straight' | 'step' | 'smoothstep'

type RoutePoint = { x: number; y: number }

export function routeStyleFromValue(value: unknown): ConnectorRouteStyle {
  return value === 'straight' || value === 'step' || value === 'smoothstep' ? value : 'bezier'
}

export function positionForHandleSide(side: LogicalHandleSide) {
  switch (side) {
    case 'top':
      return Position.Top
    case 'bottom':
      return Position.Bottom
    case 'left':
      return Position.Left
    case 'right':
      return Position.Right
  }
}

function controlPoint(
  x: number,
  y: number,
  tx: number,
  ty: number,
  position: Position,
  minStem: number,
): [number, number] {
  const dx = Math.abs(tx - x)
  const dy = Math.abs(ty - y)
  switch (position) {
    case Position.Left: return [x - Math.max(dx * CURVATURE, minStem), y]
    case Position.Right: return [x + Math.max(dx * CURVATURE, minStem), y]
    case Position.Top: return [x, y - Math.max(dy * CURVATURE, minStem)]
    case Position.Bottom: return [x, y + Math.max(dy * CURVATURE, minStem)]
  }
}

function edgePathFromPoints(points: RoutePoint[], borderRadius = 0) {
  if (points.length === 0) return ''
  const commands = [`M ${points[0].x},${points[0].y}`]

  for (let index = 1; index < points.length; index += 1) {
    const curr = points[index]
    const prev = points[index - 1]
    const next = points[index + 1]

    if (borderRadius > 0 && next) {
      const dPrevX = curr.x - prev.x
      const dPrevY = curr.y - prev.y
      const dPrevLen = Math.hypot(dPrevX, dPrevY)
      const dNextX = next.x - curr.x
      const dNextY = next.y - curr.y
      const dNextLen = Math.hypot(dNextX, dNextY)

      if (dPrevLen > 0 && dNextLen > 0) {
        const r = Math.min(borderRadius, dPrevLen / 2, dNextLen / 2)
        commands.push(`L ${curr.x - (dPrevX / dPrevLen) * r},${curr.y - (dPrevY / dPrevLen) * r}`)
        commands.push(`Q ${curr.x},${curr.y} ${curr.x + (dNextX / dNextLen) * r},${curr.y + (dNextY / dNextLen) * r}`)
        continue
      }
    }

    commands.push(`L ${curr.x},${curr.y}`)
  }

  return commands.join(' ')
}

function stepRoutePoints(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
  sourcePosition: Position,
  targetPosition: Position,
) {
  const midX = (sourceX + targetX) / 2
  const midY = (sourceY + targetY) / 2
  const sourceOrth = sourcePosition === Position.Left || sourcePosition === Position.Right ? 'h' : 'v'
  const targetOrth = targetPosition === Position.Left || targetPosition === Position.Right ? 'h' : 'v'
  const points: RoutePoint[] = [{ x: sourceX, y: sourceY }]

  if (sourceOrth === 'h' && targetOrth === 'h') {
    points.push({ x: midX, y: sourceY }, { x: midX, y: targetY })
  } else if (sourceOrth === 'v' && targetOrth === 'v') {
    points.push({ x: sourceX, y: midY }, { x: targetX, y: midY })
  } else if (sourceOrth === 'h' && targetOrth === 'v') {
    points.push({ x: targetX, y: sourceY })
  } else {
    points.push({ x: sourceX, y: targetY })
  }

  points.push({ x: targetX, y: targetY })
  return points
}

export function buildViewConnectorPath(args: {
  routeStyle: ConnectorRouteStyle
  sourceX: number
  sourceY: number
  targetX: number
  targetY: number
  sourcePosition: Position
  targetPosition: Position
  sourceWidth?: number
  sourceHeight?: number
  targetWidth?: number
  targetHeight?: number
}): { path: string; labelX: number; labelY: number } {
  const {
    routeStyle,
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    sourceWidth = 200,
    sourceHeight = 100,
    targetWidth = 200,
    targetHeight = 100,
  } = args

  if (routeStyle === 'straight') {
    return {
      path: `M ${sourceX},${sourceY} L ${targetX},${targetY}`,
      labelX: (sourceX + targetX) / 2,
      labelY: (sourceY + targetY) / 2,
    }
  }

  if (routeStyle === 'step' || routeStyle === 'smoothstep') {
    const points = stepRoutePoints(sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition)
    let labelX = (sourceX + targetX) / 2
    let labelY = (sourceY + targetY) / 2

    if (points.length === 4) {
      labelX = (points[1].x + points[2].x) / 2
      labelY = (points[1].y + points[2].y) / 2
    } else if (points.length === 3) {
      const d1 = Math.abs(points[1].x - points[0].x) + Math.abs(points[1].y - points[0].y)
      const d2 = Math.abs(points[2].x - points[1].x) + Math.abs(points[2].y - points[1].y)
      const first = d1 > d2 ? points[0] : points[1]
      const second = d1 > d2 ? points[1] : points[2]
      labelX = (first.x + second.x) / 2
      labelY = (first.y + second.y) / 2
    }

    return {
      path: edgePathFromPoints(points, routeStyle === 'smoothstep' ? 6 : 0),
      labelX,
      labelY,
    }
  }

  const sourceMinStem = (sourcePosition === Position.Left || sourcePosition === Position.Right)
    ? sourceWidth * 0.5 : sourceHeight * 0.5
  const targetMinStem = (targetPosition === Position.Left || targetPosition === Position.Right)
    ? targetWidth * 0.5 : targetHeight * 0.5

  const [cp1x, cp1y] = controlPoint(sourceX, sourceY, targetX, targetY, sourcePosition, sourceMinStem)
  const [cp2x, cp2y] = controlPoint(targetX, targetY, sourceX, sourceY, targetPosition, targetMinStem)

  return {
    path: `M ${sourceX},${sourceY} C ${cp1x},${cp1y} ${cp2x},${cp2y} ${targetX},${targetY}`,
    labelX: 0.125 * sourceX + 0.375 * cp1x + 0.375 * cp2x + 0.125 * targetX,
    labelY: 0.125 * sourceY + 0.375 * cp1y + 0.375 * cp2y + 0.125 * targetY,
  }
}
