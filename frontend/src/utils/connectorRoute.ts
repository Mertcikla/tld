import { Position } from 'reactflow'
import type { LogicalHandleSide } from './edgeDistribution'

const CURVATURE = 0.5

export type ConnectorRouteStyle = 'bezier' | 'straight' | 'step' | 'smoothstep'



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

export function handleSideForPosition(position: Position, fallback: LogicalHandleSide): LogicalHandleSide {
  switch (position) {
    case Position.Top:
      return 'top'
    case Position.Bottom:
      return 'bottom'
    case Position.Left:
      return 'left'
    case Position.Right:
      return 'right'
    default:
      return fallback
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

export type RoutePoint = { x: number; y: number }

export function edgePathFromPoints(points: RoutePoint[], borderRadius = 0) {
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

export function stepRoutePoints(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
  sourcePosition: Position,
  targetPosition: Position,
  _sourceWidth = 200,
  sourceHeight = 100,
  _targetWidth = 200,
  targetHeight = 100,
) {
  const sourceOrth = sourcePosition === Position.Left || sourcePosition === Position.Right ? 'h' : 'v'
  const targetOrth = targetPosition === Position.Left || targetPosition === Position.Right ? 'h' : 'v'

  const sourceOffset = (sourceHeight ?? 100) / 2
  const targetOffset = (targetHeight ?? 100) / 2

  let points: RoutePoint[] = []

  if (sourceOrth === 'h' && targetOrth === 'h') {
    if (sourcePosition === Position.Right && targetPosition === Position.Right) {
      const maxX = Math.max(sourceX, targetX)
      points = [
        { x: sourceX, y: sourceY },
        { x: maxX + sourceOffset, y: sourceY },
        { x: maxX + sourceOffset, y: targetY },
        { x: targetX, y: targetY }
      ]
    } else if (sourcePosition === Position.Left && targetPosition === Position.Left) {
      const minX = Math.min(sourceX, targetX)
      points = [
        { x: sourceX, y: sourceY },
        { x: minX - sourceOffset, y: sourceY },
        { x: minX - sourceOffset, y: targetY },
        { x: targetX, y: targetY }
      ]
    } else if (sourcePosition === Position.Right && targetPosition === Position.Left) {
      if (sourceX + 16 < targetX) {
        const midX = (sourceX + targetX) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: midX, y: sourceY },
          { x: midX, y: targetY },
          { x: targetX, y: targetY }
        ]
      } else {
        const midY = (sourceY + targetY) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: sourceX + sourceOffset, y: sourceY },
          { x: sourceX + sourceOffset, y: midY },
          { x: targetX - targetOffset, y: midY },
          { x: targetX - targetOffset, y: targetY },
          { x: targetX, y: targetY }
        ]
      }
    } else if (sourcePosition === Position.Left && targetPosition === Position.Right) {
      if (targetX + 16 < sourceX) {
        const midX = (sourceX + targetX) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: midX, y: sourceY },
          { x: midX, y: targetY },
          { x: targetX, y: targetY }
        ]
      } else {
        const midY = (sourceY + targetY) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: sourceX - sourceOffset, y: sourceY },
          { x: sourceX - sourceOffset, y: midY },
          { x: targetX + targetOffset, y: midY },
          { x: targetX + targetOffset, y: targetY },
          { x: targetX, y: targetY }
        ]
      }
    }
  } else if (sourceOrth === 'v' && targetOrth === 'v') {
    if (sourcePosition === Position.Bottom && targetPosition === Position.Bottom) {
      const maxY = Math.max(sourceY, targetY)
      points = [
        { x: sourceX, y: sourceY },
        { x: sourceX, y: maxY + sourceOffset },
        { x: targetX, y: maxY + sourceOffset },
        { x: targetX, y: targetY }
      ]
    } else if (sourcePosition === Position.Top && targetPosition === Position.Top) {
      const minY = Math.min(sourceY, targetY)
      points = [
        { x: sourceX, y: sourceY },
        { x: sourceX, y: minY - sourceOffset },
        { x: targetX, y: minY - sourceOffset },
        { x: targetX, y: targetY }
      ]
    } else if (sourcePosition === Position.Bottom && targetPosition === Position.Top) {
      if (sourceY + 16 < targetY) {
        const midY = (sourceY + targetY) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: sourceX, y: midY },
          { x: targetX, y: midY },
          { x: targetX, y: targetY }
        ]
      } else {
        const midX = (sourceX + targetX) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: sourceX, y: sourceY + sourceOffset },
          { x: midX, y: sourceY + sourceOffset },
          { x: midX, y: targetY - targetOffset },
          { x: targetX, y: targetY - targetOffset },
          { x: targetX, y: targetY }
        ]
      }
    } else if (sourcePosition === Position.Top && targetPosition === Position.Bottom) {
      if (targetY + 16 < sourceY) {
        const midY = (sourceY + targetY) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: sourceX, y: midY },
          { x: targetX, y: midY },
          { x: targetX, y: targetY }
        ]
      } else {
        const midX = (sourceX + targetX) / 2
        points = [
          { x: sourceX, y: sourceY },
          { x: sourceX, y: sourceY - sourceOffset },
          { x: midX, y: sourceY - sourceOffset },
          { x: midX, y: targetY + targetOffset },
          { x: targetX, y: targetY + targetOffset },
          { x: targetX, y: targetY }
        ]
      }
    }
  } else if (sourceOrth === 'h' && targetOrth === 'v') {
    const exitX = sourcePosition === Position.Left ? sourceX - sourceOffset : sourceX + sourceOffset
    const entryY = targetPosition === Position.Top ? targetY - targetOffset : targetY + targetOffset
    const isCorrectSide = (sourcePosition === Position.Right && targetX >= sourceX) ||
                          (sourcePosition === Position.Left && targetX <= sourceX)

    if (isCorrectSide && Math.abs(sourceX - targetX) >= 16) {
      points = [
        { x: sourceX, y: sourceY },
        { x: targetX, y: sourceY },
        { x: targetX, y: targetY }
      ]
    } else {
      points = [
        { x: sourceX, y: sourceY },
        { x: exitX, y: sourceY },
        { x: exitX, y: entryY },
        { x: targetX, y: entryY },
        { x: targetX, y: targetY }
      ]
    }
  } else {
    const exitY = sourcePosition === Position.Top ? sourceY - sourceOffset : sourceY + sourceOffset
    const entryX = targetPosition === Position.Left ? targetX - targetOffset : targetX + targetOffset
    const isCorrectSide = (targetPosition === Position.Right && sourceX >= targetX) ||
                          (targetPosition === Position.Left && sourceX <= targetX)

    if (isCorrectSide && Math.abs(sourceY - targetY) >= 16) {
      points = [
        { x: sourceX, y: sourceY },
        { x: sourceX, y: targetY },
        { x: targetX, y: targetY }
      ]
    } else {
      points = [
        { x: sourceX, y: sourceY },
        { x: sourceX, y: exitY },
        { x: entryX, y: exitY },
        { x: entryX, y: targetY },
        { x: targetX, y: targetY }
      ]
    }
  }

  const result: RoutePoint[] = []
  for (const p of points) {
    if (result.length === 0) {
      result.push(p)
    } else {
      const prev = result[result.length - 1]
      if (Math.abs(prev.x - p.x) > 0.01 || Math.abs(prev.y - p.y) > 0.01) {
        result.push(p)
      }
    }
  }

  return result
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
    const points = stepRoutePoints(
      sourceX,
      sourceY,
      targetX,
      targetY,
      sourcePosition,
      targetPosition,
      sourceWidth,
      sourceHeight,
      targetWidth,
      targetHeight,
    )
    let labelX = (sourceX + targetX) / 2
    let labelY = (sourceY + targetY) / 2

    if (points.length > 1) {
      let maxLen = -1
      let bestIdx = 0
      const startIdx = points.length >= 4 ? 1 : 0
      const endIdx = points.length >= 4 ? points.length - 2 : points.length - 1

      for (let i = startIdx; i < endIdx; i++) {
        const p1 = points[i]
        const p2 = points[i + 1]
        const len = Math.hypot(p2.x - p1.x, p2.y - p1.y)
        if (len > maxLen) {
          maxLen = len
          bestIdx = i
        }
      }
      const p1 = points[bestIdx]
      const p2 = points[bestIdx + 1]
      labelX = (p1.x + p2.x) / 2
      labelY = (p1.y + p2.y) / 2
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
