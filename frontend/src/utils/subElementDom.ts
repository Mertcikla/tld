import { connectorRouteSize } from './connectorRoute'
import type { Node } from 'reactflow'
import { isSubHandle, subElementKind } from './subElements'
import { getHandleFlowPosition, type LogicalHandleSide } from './edgeDistribution'

function nodeElement(node: Node) {
  return document.querySelector<HTMLElement>(`.react-flow__node[data-id="${CSS.escape(node.id)}"]`)
}

export function nodeConnectorRouteSize(node: Node | undefined, width = 180, height = 85) {
  return connectorRouteSize(node?.width ?? width, node?.height ?? height, !!subElementKind(node?.data))
}

/** Use the mounted port geometry, including scrolled and aggregated endpoints. */
export function subHandleFlowPosition(node: Node | undefined, handle: string | null | undefined) {
  if (!node || !isSubHandle(handle) || typeof document === 'undefined') return null
  const element = nodeElement(node)
  const port = element?.querySelector<HTMLElement>(`.react-flow__handle[data-handleid="${CSS.escape(handle!)}"]`)
  if (!element || !port) return null
  const bounds = element.getBoundingClientRect(), point = port.getBoundingClientRect()
  if (!bounds.width || !bounds.height) return null
  const origin = node.positionAbsolute ?? node.position
  const side = port.classList.contains('react-flow__handle-left') ? 'left' : 'right'
  return { x: origin.x + (side === 'left' ? 0 : node.width ?? 180),
    y: origin.y + (point.y + point.height / 2 - bounds.y) * (node.height ?? 85) / bounds.height, side } as const
}

export function nodeHandleFlowPosition(node: Node, handle: string | null | undefined, fallback: LogicalHandleSide) {
  const origin = node.positionAbsolute ?? node.position
  return subHandleFlowPosition(node, handle) ?? getHandleFlowPosition(origin.x, origin.y, node.width ?? 180, node.height ?? 85, handle, fallback)
}

export function symbolHandleAtFlowPoint(node: Node, x: number, y: number) {
  if (node.data?.kind !== 'code' && node.data?.kind !== 'class') return null
  const element = nodeElement(node)
  if (!element) return null
  const bounds = element.getBoundingClientRect(), origin = node.positionAbsolute ?? node.position
  const target = document.elementFromPoint(bounds.x + (x - origin.x) * bounds.width / (node.width ?? 180), bounds.y + (y - origin.y) * bounds.height / (node.height ?? 85))
  return target?.closest('.react-flow__node') === element ? target.closest('[data-symbol-handle]')?.getAttribute('data-symbol-handle') : null
}
