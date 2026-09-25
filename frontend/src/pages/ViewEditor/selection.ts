import type { CSSProperties } from 'react'
import type { Node as RFNode } from 'reactflow'

const DEFAULT_NODE_WIDTH = 180
const DEFAULT_NODE_HEIGHT = 80

export const ELEMENT_NODE_WIDTH = 180
export const ELEMENT_NODE_HEIGHT = 85

const MARQUEE_SIZE_FACTOR_MIN = 0.5
const MARQUEE_SIZE_FACTOR_MAX = 4
const MARQUEE_ASPECT_MIN = 1
const MARQUEE_ASPECT_MAX = 5

export type RectLike = { x: number; y: number; width: number; height: number }

export type SelectionAlign =
  | 'left'
  | 'center'
  | 'right'
  | 'top'
  | 'middle'
  | 'bottom'

export type SelectionDistribute = 'horizontal' | 'vertical'

export type SelectionNodeUpdate = {
  id: string
  elementId: number
  x: number
  y: number
}

type SelectionNode = Pick<RFNode, 'id' | 'type' | 'position' | 'width' | 'height' | 'selected'>

type NodeRect = {
  id: string
  elementId: number
  x: number
  y: number
  width: number
  height: number
}

export type SelectionViewport = {
  x: number
  y: number
  zoom: number
  width: number
  height: number
}

function parseElementNodeId(node: SelectionNode): number | null {
  if (node.type !== 'elementNode') return null
  const id = Number(node.id)
  return Number.isInteger(id) && id > 0 ? id : null
}

function nodeRect(node: SelectionNode): NodeRect | null {
  const elementId = parseElementNodeId(node)
  if (elementId === null) return null
  return {
    id: node.id,
    elementId,
    x: node.position.x,
    y: node.position.y,
    width: node.width ?? DEFAULT_NODE_WIDTH,
    height: node.height ?? DEFAULT_NODE_HEIGHT,
  }
}

function samePosition(rect: NodeRect, x: number, y: number) {
  return Math.abs(rect.x - x) < 0.5 && Math.abs(rect.y - y) < 0.5
}

function updateFromRect(rect: NodeRect, x: number, y: number): SelectionNodeUpdate | null {
  if (samePosition(rect, x, y)) return null
  return { id: rect.id, elementId: rect.elementId, x, y }
}

export function selectedElementIds(nodes: SelectionNode[]): number[] {
  return nodes
    .filter((node) => node.selected)
    .map(parseElementNodeId)
    .filter((id): id is number => id !== null)
}

export function elementSelectionRects(nodes: SelectionNode[]): NodeRect[] {
  return nodes
    .filter((node) => node.selected)
    .map(nodeRect)
    .filter((rect): rect is NodeRect => rect !== null)
}

function isNodeHidden(node: SelectionNode & { style?: CSSProperties }): boolean {
  return node.style?.pointerEvents === 'none' || node.style?.display === 'none' || node.style?.visibility === 'hidden' || Number(node.style?.opacity ?? 1) <= 0
}

function intersectsViewport(rect: NodeRect, viewport: SelectionViewport): boolean {
  if (viewport.zoom <= 0 || viewport.width <= 0 || viewport.height <= 0) return true

  const left = -viewport.x / viewport.zoom
  const top = -viewport.y / viewport.zoom
  const right = (viewport.width - viewport.x) / viewport.zoom
  const bottom = (viewport.height - viewport.y) / viewport.zoom

  return rect.x < right && rect.x + rect.width > left && rect.y < bottom && rect.y + rect.height > top
}

export function visibleElementSelectionRects(
  nodes: Array<SelectionNode & { style?: CSSProperties }>,
  viewport: SelectionViewport | null,
): NodeRect[] {
  return nodes
    .filter((node) => !isNodeHidden(node))
    .map(nodeRect)
    .filter((rect): rect is NodeRect => rect !== null)
    .filter((rect) => viewport === null || intersectsViewport(rect, viewport))
    .sort((left, right) => {
      if (left.y !== right.y) return left.y - right.y
      return left.x - right.x
    })
}

export function selectionBounds(rects: NodeRect[]) {
  if (rects.length === 0) return null
  const left = Math.min(...rects.map((rect) => rect.x))
  const top = Math.min(...rects.map((rect) => rect.y))
  const right = Math.max(...rects.map((rect) => rect.x + rect.width))
  const bottom = Math.max(...rects.map((rect) => rect.y + rect.height))
  return {
    left,
    top,
    right,
    bottom,
    width: right - left,
    height: bottom - top,
    centerX: left + (right - left) / 2,
    centerY: top + (bottom - top) / 2,
  }
}

export function planSelectionAlignment(nodes: SelectionNode[], align: SelectionAlign): SelectionNodeUpdate[] {
  const rects = elementSelectionRects(nodes)
  if (rects.length < 2) return []
  const bounds = selectionBounds(rects)
  if (!bounds) return []

  return rects
    .map((rect) => {
      switch (align) {
        case 'left':
          return updateFromRect(rect, bounds.left, rect.y)
        case 'center':
          return updateFromRect(rect, bounds.centerX - rect.width / 2, rect.y)
        case 'right':
          return updateFromRect(rect, bounds.right - rect.width, rect.y)
        case 'top':
          return updateFromRect(rect, rect.x, bounds.top)
        case 'middle':
          return updateFromRect(rect, rect.x, bounds.centerY - rect.height / 2)
        case 'bottom':
          return updateFromRect(rect, rect.x, bounds.bottom - rect.height)
      }
    })
    .filter((update): update is SelectionNodeUpdate => update !== null)
}

export function isElementShapedMarqueeRect(rect: Pick<RectLike, 'width' | 'height'>): boolean {
  if (!(rect.width > 0) || !(rect.height > 0)) return false
  if (rect.width < ELEMENT_NODE_WIDTH * MARQUEE_SIZE_FACTOR_MIN) return false
  if (rect.width > ELEMENT_NODE_WIDTH * MARQUEE_SIZE_FACTOR_MAX) return false
  if (rect.height < ELEMENT_NODE_HEIGHT * MARQUEE_SIZE_FACTOR_MIN) return false
  if (rect.height > ELEMENT_NODE_HEIGHT * MARQUEE_SIZE_FACTOR_MAX) return false
  const aspect = rect.width / rect.height
  return aspect >= MARQUEE_ASPECT_MIN && aspect <= MARQUEE_ASPECT_MAX
}

export function marqueeHitsElementRect(
  rect: RectLike,
  nodes: Array<SelectionNode & { style?: CSSProperties; selectable?: boolean }>,
): boolean {
  const left = rect.x
  const top = rect.y
  const right = rect.x + rect.width
  const bottom = rect.y + rect.height

  return nodes.some((node) => {
    if (isNodeHidden(node)) return false
    if (node.selectable === false) return false
    const candidate = nodeRect(node)
    if (!candidate) return false
    const overlapX = Math.min(right, candidate.x + candidate.width) - Math.max(left, candidate.x)
    const overlapY = Math.min(bottom, candidate.y + candidate.height) - Math.max(top, candidate.y)
    return overlapX > 0 && overlapY > 0
  })
}

export function planSelectionDistribution(nodes: SelectionNode[], direction: SelectionDistribute): SelectionNodeUpdate[] {
  const rects = elementSelectionRects(nodes)
  if (rects.length < 3) return []

  const sorted = [...rects].sort((left, right) => {
    const leftCenter = direction === 'horizontal' ? left.x + left.width / 2 : left.y + left.height / 2
    const rightCenter = direction === 'horizontal' ? right.x + right.width / 2 : right.y + right.height / 2
    return leftCenter - rightCenter
  })

  const first = sorted[0]
  const last = sorted[sorted.length - 1]
  const firstCenter = direction === 'horizontal' ? first.x + first.width / 2 : first.y + first.height / 2
  const lastCenter = direction === 'horizontal' ? last.x + last.width / 2 : last.y + last.height / 2
  const step = (lastCenter - firstCenter) / (sorted.length - 1)

  return sorted
    .map((rect, index) => {
      if (index === 0 || index === sorted.length - 1) return null
      const targetCenter = firstCenter + step * index
      return direction === 'horizontal'
        ? updateFromRect(rect, targetCenter - rect.width / 2, rect.y)
        : updateFromRect(rect, rect.x, targetCenter - rect.height / 2)
    })
    .filter((update): update is SelectionNodeUpdate => update !== null)
}
