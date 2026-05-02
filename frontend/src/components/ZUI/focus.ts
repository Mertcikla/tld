import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'
import { getExpandThresholds } from './renderer'

interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface ZUIFocusTarget {
  id: string
  label: string
  type: 'group' | 'node'
  isCircular?: boolean
  absX: number
  absY: number
  absW: number
  absH: number
  absScale: number
  node?: LayoutNode
  contentRect?: Rect
}

export interface ZUIFocusViewportOptions {
  preferContent?: boolean
  minTargetScreenW?: number
  minChildScreenW?: number
  keepParentVisible?: boolean
}

function boundsForRects(rects: Rect[]): Rect | null {
  if (rects.length === 0) return null

  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity

  for (const rect of rects) {
    minX = Math.min(minX, rect.x)
    minY = Math.min(minY, rect.y)
    maxX = Math.max(maxX, rect.x + rect.w)
    maxY = Math.max(maxY, rect.y + rect.h)
  }

  if (!Number.isFinite(minX) || !Number.isFinite(minY) || !Number.isFinite(maxX) || !Number.isFinite(maxY)) {
    return null
  }

  return { x: minX, y: minY, w: positiveSize(maxX - minX), h: positiveSize(maxY - minY) }
}

function positiveSize(value: number): number {
  return Number.isFinite(value) && value > 0 ? value : 0.0001
}

function childContentRect(node: LayoutNode, absX: number, absY: number, absScale: number): Rect | null {
  if (node.children.length === 0) return null

  const childAbsScale = absScale * node.childScale
  return boundsForRects(node.children.map((child) => ({
    x: absX + (child.worldX - node.childOffsetX) * childAbsScale,
    y: absY + (child.worldY - node.childOffsetY) * childAbsScale,
    w: child.worldW * childAbsScale,
    h: child.worldH * childAbsScale,
  })))
}

function nodeTarget(
  node: LayoutNode,
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
): ZUIFocusTarget {
  const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
  const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
  const absW = node.worldW * parentAbsScale
  const absH = node.worldH * parentAbsScale

  return {
    id: node.id,
    label: node.linkedDiagramLabel || node.label,
    type: 'node',
    isCircular: node.isCircular,
    absX,
    absY,
    absW,
    absH,
    absScale: parentAbsScale,
    node,
    contentRect: childContentRect(node, absX, absY, parentAbsScale) ?? undefined,
  }
}

function findLinkedDiagramInNodes(
  viewId: number,
  nodes: DiagramGroupLayout['nodes'],
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
): ZUIFocusTarget | null {
  for (const node of nodes) {
    const target = nodeTarget(node, parentAbsX, parentAbsY, parentAbsScale, parentChildOffsetX, parentChildOffsetY)

    if (node.linkedDiagramId === viewId) {
      return target
    }

    if (node.children.length > 0) {
      const found = findLinkedDiagramInNodes(
        viewId,
        node.children,
        target.absX,
        target.absY,
        parentAbsScale * node.childScale,
        node.childOffsetX,
        node.childOffsetY,
      )
      if (found) return found
    }
  }

  return null
}

function findElementInNodes(
  viewId: number,
  elementId: number,
  nodes: DiagramGroupLayout['nodes'],
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
): ZUIFocusTarget | null {
  for (const node of nodes) {
    const target = nodeTarget(node, parentAbsX, parentAbsY, parentAbsScale, parentChildOffsetX, parentChildOffsetY)

    if (node.diagramId === viewId && node.elementId === elementId) {
      return target
    }

    if (node.children.length > 0) {
      const found = findElementInNodes(
        viewId,
        elementId,
        node.children,
        target.absX,
        target.absY,
        parentAbsScale * node.childScale,
        node.childOffsetX,
        node.childOffsetY,
      )
      if (found) return found
    }
  }

  return null
}

export function findDiagramFocusTarget(groups: DiagramGroupLayout[], viewId: number): ZUIFocusTarget | null {
  for (const group of groups) {
    if (group.diagramId === viewId) {
      return {
        id: `g-${group.diagramId}`,
        label: group.label,
        type: 'group',
        absX: group.worldX,
        absY: group.worldY,
        absW: group.worldW,
        absH: group.worldH,
        absScale: 1,
      }
    }

    const found = findLinkedDiagramInNodes(viewId, group.nodes, 0, 0, 1, 0, 0)
    if (found) return found
  }

  return null
}

export function findElementFocusTarget(groups: DiagramGroupLayout[], viewId: number, elementId: number): ZUIFocusTarget | null {
  for (const group of groups) {
    const found = findElementInNodes(viewId, elementId, group.nodes, 0, 0, 1, 0, 0)
    if (found) return found
  }

  return null
}

export function viewportForFocusTarget(
  target: ZUIFocusTarget,
  canvasW: number,
  canvasH: number,
  maxZoom: number,
  padding: number,
  options: ZUIFocusViewportOptions = {},
): ZUIViewState | null {
  const rect = options.preferContent && target.contentRect ? target.contentRect : {
    x: target.absX,
    y: target.absY,
    w: target.absW,
    h: target.absH,
  }

  const bboxW = positiveSize(rect.w)
  const bboxH = positiveSize(rect.h)
  const fitZoom = Math.min(
    (canvasW * (1 - padding * 2)) / bboxW,
    (canvasH * (1 - padding * 2)) / bboxH,
  )
  const minZooms: number[] = []

  if (options.minTargetScreenW && target.absW > 0) {
    minZooms.push(options.minTargetScreenW / target.absW)
  }

  if (target.node?.children.length && options.minChildScreenW && target.node.childScale > 0) {
    const childAbsW = target.node.worldW * target.absScale * target.node.childScale
    if (childAbsW > 0) {
      minZooms.push(options.minChildScreenW / childAbsW)
    }
  }

  const finiteMinZooms = minZooms.filter((value) => Number.isFinite(value) && value > 0)
  const zoomLimit = Math.max(maxZoom, ...finiteMinZooms)
  let zoom = Math.min(fitZoom, zoomLimit)

  for (const minZoom of finiteMinZooms) {
    zoom = Math.max(zoom, Math.min(minZoom, zoomLimit))
  }

  if (target.node?.children.length && options.keepParentVisible) {
    const thresholds = getExpandThresholds(canvasW)
    const maxParentScreenW = thresholds.start + (thresholds.end - thresholds.start) * 0.78
    zoom = Math.min(zoom, maxParentScreenW / positiveSize(target.absW))
  }

  if (!Number.isFinite(zoom) || zoom <= 0) return null

  return {
    x: (canvasW - bboxW * zoom) / 2 - rect.x * zoom,
    y: (canvasH - bboxH * zoom) / 2 - rect.y * zoom,
    zoom,
  }
}

export function viewportForDiagramFocusTarget(
  target: ZUIFocusTarget,
  canvasW: number,
  canvasH: number,
  maxZoom: number,
  isMobileLayout: boolean,
): ZUIViewState | null {
  return viewportForFocusTarget(
    target,
    canvasW,
    canvasH,
    maxZoom,
    isMobileLayout ? 0.18 : 0.16,
    {
      preferContent: true,
      minTargetScreenW: isMobileLayout ? 180 : 260,
      minChildScreenW: isMobileLayout ? 76 : 104,
    },
  )
}

export function viewportForElementFocusTarget(
  target: ZUIFocusTarget,
  canvasW: number,
  canvasH: number,
  maxZoom: number,
  isMobileLayout: boolean,
): ZUIViewState | null {
  return viewportForFocusTarget(
    target,
    canvasW,
    canvasH,
    maxZoom,
    isMobileLayout ? 0.2 : 0.18,
    {
      minTargetScreenW: isMobileLayout ? 220 : 320,
      keepParentVisible: true,
    },
  )
}
