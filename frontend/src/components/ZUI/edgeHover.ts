import type { DiagramGroupLayout, HoveredItem, LayoutNode, ZUIViewState } from './types'
import { positionForHandleSide, stepRoutePoints } from '../../utils/connectorRoute'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getHandleFlowPosition,
} from '../../utils/edgeDistribution'

export type EdgeRoutePoint = { x: number; y: number }

type IndexedEdge =
  | {
    kind: 'edge'
    x1: number
    y1: number
    x2: number
    y2: number
    midX: number
    midY: number
    points: EdgeRoutePoint[]
    sourceLabel: string
    targetLabel: string
    label: string
    id: number
    diagramId: number
    sourceObjId: number
    targetObjId: number
  }
  | {
    kind: 'portal'
    x1: number
    y1: number
    x2: number
    y2: number
    midX: number
    midY: number
    points: EdgeRoutePoint[]
    sourceLabel: string
    targetLabel: string
    diagramId: number
    targetDiagId?: number
  }

export interface EdgeSpatialIndex {
  cellSize: number
  cells: Map<string, IndexedEdge[]>
}

const EDGE_INDEX_CELL_SIZE = 360

function cellKey(cx: number, cy: number): string {
  return `${cx},${cy}`
}

function addEdgeToSpatialIndex(index: EdgeSpatialIndex, edge: IndexedEdge): void {
  const points = edge.points.length > 0 ? edge.points : [{ x: edge.x1, y: edge.y1 }, { x: edge.x2, y: edge.y2 }]
  const minX = Math.min(...points.map((point) => point.x))
  const maxX = Math.max(...points.map((point) => point.x))
  const minY = Math.min(...points.map((point) => point.y))
  const maxY = Math.max(...points.map((point) => point.y))
  const startX = Math.floor(minX / index.cellSize)
  const endX = Math.floor(maxX / index.cellSize)
  const startY = Math.floor(minY / index.cellSize)
  const endY = Math.floor(maxY / index.cellSize)

  for (let cx = startX; cx <= endX; cx++) {
    for (let cy = startY; cy <= endY; cy++) {
      const key = cellKey(cx, cy)
      let bucket = index.cells.get(key)
      if (!bucket) {
        bucket = []
        index.cells.set(key, bucket)
      }
      bucket.push(edge)
    }
  }
}

function normalizeEdgeRouteType(type: string | null | undefined): 'bezier' | 'straight' | 'step' | 'smoothstep' {
  if (type === 'straight' || type === 'step' || type === 'smoothstep') return type
  return 'bezier'
}

function cubicBezierPoint(
  p0: EdgeRoutePoint,
  p1: EdgeRoutePoint,
  p2: EdgeRoutePoint,
  p3: EdgeRoutePoint,
  t: number,
): EdgeRoutePoint {
  const mt = 1 - t
  const mt2 = mt * mt
  const t2 = t * t
  return {
    x: mt2 * mt * p0.x + 3 * mt2 * t * p1.x + 3 * mt * t2 * p2.x + t2 * t * p3.x,
    y: mt2 * mt * p0.y + 3 * mt2 * t * p1.y + 3 * mt * t2 * p2.y + t2 * t * p3.y,
  }
}

export function buildEdgeRoutePoints(
  source: LayoutNode,
  target: LayoutNode,
  edge: DiagramGroupLayout['edges'][number],
): { points: EdgeRoutePoint[]; midX: number; midY: number } {
  const sourcePoint = getHandleFlowPosition(
    source.worldX,
    source.worldY,
    source.worldW,
    source.worldH,
    edge.sourceHandle,
    DEFAULT_SOURCE_HANDLE_SIDE,
  )
  const targetPoint = getHandleFlowPosition(
    target.worldX,
    target.worldY,
    target.worldW,
    target.worldH,
    edge.targetHandle,
    DEFAULT_TARGET_HANDLE_SIDE,
  )
  const type = normalizeEdgeRouteType(edge.type)

  if (type === 'bezier') {
    const dx = Math.abs(targetPoint.x - sourcePoint.x)
    const dy = Math.abs(targetPoint.y - sourcePoint.y)
    const sourceStem = Math.max(
      (sourcePoint.side === 'left' || sourcePoint.side === 'right' ? dx : dy) * 0.5,
      (sourcePoint.side === 'left' || sourcePoint.side === 'right' ? source.worldW : source.worldH) * 0.5,
    )
    const targetStem = Math.max(
      (targetPoint.side === 'left' || targetPoint.side === 'right' ? dx : dy) * 0.5,
      (targetPoint.side === 'left' || targetPoint.side === 'right' ? target.worldW : target.worldH) * 0.5,
    )
    const cp1 = {
      x: sourcePoint.x + (sourcePoint.side === 'left' ? -sourceStem : sourcePoint.side === 'right' ? sourceStem : 0),
      y: sourcePoint.y + (sourcePoint.side === 'top' ? -sourceStem : sourcePoint.side === 'bottom' ? sourceStem : 0),
    }
    const cp2 = {
      x: targetPoint.x + (targetPoint.side === 'left' ? -targetStem : targetPoint.side === 'right' ? targetStem : 0),
      y: targetPoint.y + (targetPoint.side === 'top' ? -targetStem : targetPoint.side === 'bottom' ? targetStem : 0),
    }
    const p0 = { x: sourcePoint.x, y: sourcePoint.y }
    const p3 = { x: targetPoint.x, y: targetPoint.y }
    const points = Array.from({ length: 17 }, (_, index) => cubicBezierPoint(p0, cp1, cp2, p3, index / 16))
    const mid = cubicBezierPoint(p0, cp1, cp2, p3, 0.5)
    return { points, midX: mid.x, midY: mid.y }
  }

  if (type === 'step' || type === 'smoothstep') {
    const points = stepRoutePoints(
      sourcePoint.x,
      sourcePoint.y,
      targetPoint.x,
      targetPoint.y,
      positionForHandleSide(sourcePoint.side),
      positionForHandleSide(targetPoint.side),
      source.worldW,
      source.worldH,
      target.worldW,
      target.worldH,
    )
    let midX = (sourcePoint.x + targetPoint.x) / 2
    let midY = (sourcePoint.y + targetPoint.y) / 2

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
      midX = (p1.x + p2.x) / 2
      midY = (p1.y + p2.y) / 2
    }

    return {
      points,
      midX,
      midY,
    }
  }

  const points = [{ x: sourcePoint.x, y: sourcePoint.y }, { x: targetPoint.x, y: targetPoint.y }]
  return {
    points,
    midX: (sourcePoint.x + targetPoint.x) / 2,
    midY: (sourcePoint.y + targetPoint.y) / 2,
  }
}

export function nearestDistanceSquaredToRoute(worldX: number, worldY: number, points: EdgeRoutePoint[]): number {
  let best = Number.POSITIVE_INFINITY
  for (let index = 1; index < points.length; index++) {
    const start = points[index - 1]
    const end = points[index]
    const dx = end.x - start.x
    const dy = end.y - start.y
    const lengthSquared = dx * dx + dy * dy
    if (lengthSquared === 0) continue

    let t = ((worldX - start.x) * dx + (worldY - start.y) * dy) / lengthSquared
    t = Math.max(0, Math.min(1, t))
    const nearestX = start.x + t * dx
    const nearestY = start.y + t * dy
    best = Math.min(best, (worldX - nearestX) ** 2 + (worldY - nearestY) ** 2)
  }
  return best
}

export function buildEdgeSpatialIndex(groups: DiagramGroupLayout[]): EdgeSpatialIndex {
  const index: EdgeSpatialIndex = { cellSize: EDGE_INDEX_CELL_SIZE, cells: new Map() }

  for (const group of groups) {
    const nodeMap = new Map<string, LayoutNode>()
    for (const node of group.nodes) {
      nodeMap.set(node.id, node)
    }

    for (const edge of group.edges) {
      const source = nodeMap.get(edge.sourceId)
      const target = nodeMap.get(edge.targetId)
      if (!source || !target) continue

      const route = buildEdgeRoutePoints(source, target, edge)
      const first = route.points[0]
      const last = route.points[route.points.length - 1]
      addEdgeToSpatialIndex(index, {
        kind: 'edge',
        x1: first.x,
        y1: first.y,
        x2: last.x,
        y2: last.y,
        midX: route.midX,
        midY: route.midY,
        points: route.points,
        sourceLabel: source.label,
        targetLabel: target.label,
        label: edge.label || 'Connector',
        id: edge.id,
        diagramId: group.diagramId,
        sourceObjId: source.elementId,
        targetObjId: target.elementId,
      })
    }

    for (const node of group.nodes) {
      if (!node.isPortal) continue
      const x1 = group.worldX + group.diagramX + group.diagramW / 2
      const y1 = group.worldY + group.diagramY + group.diagramH
      const x2 = node.worldX + node.worldW / 2
      const y2 = node.worldY
      addEdgeToSpatialIndex(index, {
        kind: 'portal',
        x1,
        y1,
        x2,
        y2,
        midX: (x1 + x2) / 2,
        midY: (y1 + y2) / 2,
        points: [{ x: x1, y: y1 }, { x: x2, y: y2 }],
        sourceLabel: group.label,
        targetLabel: node.label,
        diagramId: group.diagramId,
        targetDiagId: node.linkedDiagramId,
      })
    }
  }

  return index
}

export function findHoveredEdge(
  worldX: number,
  worldY: number,
  index: EdgeSpatialIndex,
  view: ZUIViewState,
): HoveredItem | null {
  const threshold = 18 / view.zoom
  const startX = Math.floor((worldX - threshold) / index.cellSize)
  const endX = Math.floor((worldX + threshold) / index.cellSize)
  const startY = Math.floor((worldY - threshold) / index.cellSize)
  const endY = Math.floor((worldY + threshold) / index.cellSize)
  const thresholdSquared = threshold * threshold
  let bestEdge: IndexedEdge | null = null
  let bestDistSquared = thresholdSquared

  for (let cx = startX; cx <= endX; cx++) {
    for (let cy = startY; cy <= endY; cy++) {
      const bucket = index.cells.get(cellKey(cx, cy))
      if (!bucket) continue

      for (const edge of bucket) {
        const distSquared = nearestDistanceSquaredToRoute(worldX, worldY, edge.points)
        if (distSquared < bestDistSquared) {
          bestDistSquared = distSquared
          bestEdge = edge
        }
      }
    }
  }

  if (!bestEdge) return null
  if (bestEdge.kind === 'portal') {
    return {
      type: 'edge',
      data: {
        sourceId: bestEdge.sourceLabel,
        targetId: bestEdge.targetLabel,
        label: '',
        diagramId: bestEdge.diagramId,
        targetDiagId: bestEdge.targetDiagId,
        isPortalConn: true,
      },
      absX: bestEdge.midX,
      absY: bestEdge.midY,
    }
  }

  return {
    type: 'edge',
    data: {
      sourceId: bestEdge.sourceLabel,
      targetId: bestEdge.targetLabel,
      label: bestEdge.label,
      id: bestEdge.id,
      diagramId: bestEdge.diagramId,
      sourceObjId: bestEdge.sourceObjId,
      targetObjId: bestEdge.targetObjId,
    },
    absX: bestEdge.midX,
    absY: bestEdge.midY,
  }
}
