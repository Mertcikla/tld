import type { Node as RFNode } from 'reactflow'

const DEFAULT_NODE_WIDTH = 180
const DEFAULT_NODE_HEIGHT = 80

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
