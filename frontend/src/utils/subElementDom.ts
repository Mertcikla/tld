import { connectorRouteSize } from './connectorRoute'
import type { Edge, Node } from 'reactflow'
import { bundleFans, type BundleFan, type BundleInput } from './bundledConnector'
import { expandLineHandles, isSubHandle, subElementKind } from './subElements'
import { getHandleFlowPosition, type LogicalHandleSide } from './edgeDistribution'

function nodeElement(node: Node) {
  return document.querySelector<HTMLElement>(`.react-flow__node[data-id="${CSS.escape(node.id)}"]`)
}

export function nodeConnectorRouteSize(node: Node | undefined, width = 180, height = 85) {
  return connectorRouteSize(node?.width ?? width, node?.height ?? height, !!subElementKind(node?.data))
}

function readPortPosition(
  node: Node,
  element: HTMLElement,
  port: HTMLElement,
  side: LogicalHandleSide,
  cachedNodeBounds?: DOMRect | null,
): { x: number; y: number; side: LogicalHandleSide; bounds: DOMRect | null } {
  const origin = node.positionAbsolute ?? node.position
  const width = node.width ?? (element.offsetWidth || 180)
  // Fast path: SubElementSymbolLayer sets style.top = `${endpoint.y}px` in node-local coordinates.
  // Reading style.top directly avoids synchronous getBoundingClientRect() reflows during code scrolling.
  const styleTop = Number.parseFloat(port.style?.top ?? '')
  if (Number.isFinite(styleTop)) {
    return {
      x: origin.x + (side === 'left' ? 0 : width),
      y: origin.y + styleTop,
      side,
      bounds: cachedNodeBounds ?? null,
    }
  }
  const height = node.height ?? (element.offsetHeight || 85)
  const bounds = cachedNodeBounds === undefined ? element.getBoundingClientRect() : cachedNodeBounds
  if (bounds && bounds.width && bounds.height) {
    const point = port.getBoundingClientRect()
    return {
      x: origin.x + (side === 'left' ? 0 : width),
      y: origin.y + (point.y + point.height / 2 - bounds.y) * height / bounds.height,
      side,
      bounds,
    }
  }
  return { x: NaN, y: NaN, side, bounds: bounds ?? null }
}

/** Use the mounted port geometry, including scrolled and aggregated endpoints. */
export function subHandleFlowPosition(node: Node | undefined, handle: string | null | undefined) {
  if (!node || !isSubHandle(handle) || typeof document === 'undefined') return null
  const element = nodeElement(node)
  if (!element) return null
  const requestedSide: LogicalHandleSide = handle!.startsWith('left') ? 'left' : 'right'
  const oppositeHandle = handle!.replace(/^(left|right)-/, requestedSide === 'left' ? 'right-' : 'left-')
  const directPort = element.querySelector<HTMLElement>(`.react-flow__handle[data-handleid="${CSS.escape(handle!)}"]`)
  const port = directPort ?? element.querySelector<HTMLElement>(`.react-flow__handle[data-handleid="${CSS.escape(oppositeHandle)}"]`)
  if (!port) return null
  const side: LogicalHandleSide = directPort
    ? (port.classList.contains('react-flow__handle-left') ? 'left' : 'right')
    : requestedSide
  const resolved = readPortPosition(node, element, port, side)
  if (!Number.isFinite(resolved.x) || !Number.isFinite(resolved.y)) return null
  return { x: resolved.x, y: resolved.y, side: resolved.side } as const
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

/** Expanded aliases follow the same mounted ports as symbol highlights and overflow bands. */
export function bundleHandleFlowPositions(node: Node | undefined, handle: string | null | undefined) {
  if (!handle || expandLineHandles(handle).length < 2) return []
  return expandLineHandles(handle).flatMap(id => {
    const point = subHandleFlowPosition(node, id)
    return point ? [point] : []
  })
}

const emptyBundleFans = new Map<string, BundleFan>()
const nodeBundleFanCache = new WeakMap<Node, {
  edges: unknown
  x: number
  y: number
  w: number
  h: number
  portSignature?: string
  fans: Map<string, BundleFan>
}>()

/** Incoming and outgoing bundles share the same lane allocation on each card. */
export function nodeBundleFans(node: Node | undefined, edges: Pick<Edge, 'id' | 'source' | 'target' | 'sourceHandle' | 'targetHandle'>[]) {
  if (!node || typeof document === 'undefined') return emptyBundleFans
  const origin = node.positionAbsolute ?? node.position
  const w = node.width ?? 180
  const h = node.height ?? 85
  const cached = nodeBundleFanCache.get(node)
  if (cached && cached.edges === edges && cached.x === origin.x && cached.y === origin.y && cached.w === w && cached.h === h && !cached.portSignature) {
    return cached.fans
  }

  let hasBundleEdge = false
  for (const edge of edges) {
    if (edge.source === node.id && edge.sourceHandle && expandLineHandles(edge.sourceHandle).length >= 2) {
      hasBundleEdge = true
      break
    }
    if (edge.target === node.id && edge.targetHandle && expandLineHandles(edge.targetHandle).length >= 2) {
      hasBundleEdge = true
      break
    }
  }
  if (!hasBundleEdge) {
    nodeBundleFanCache.set(node, { edges, x: origin.x, y: origin.y, w, h, fans: emptyBundleFans })
    return emptyBundleFans
  }

  const element = nodeElement(node)
  if (!element) return emptyBundleFans

  const handlesById = new Map<string, HTMLElement>()
  for (const handleEl of element.querySelectorAll<HTMLElement>('.react-flow__handle[data-handleid]')) {
    const id = handleEl.getAttribute('data-handleid')
    if (id) handlesById.set(id, handleEl)
  }

  let cachedNodeBounds: DOMRect | null | undefined
  const resolveHandle = (handle: string) => {
    const requestedSide: LogicalHandleSide = handle.startsWith('left') ? 'left' : 'right'
    const oppositeHandle = handle.replace(/^(left|right)-/, requestedSide === 'left' ? 'right-' : 'left-')
    const directPort = handlesById.get(handle)
    const port = directPort ?? handlesById.get(oppositeHandle)
    if (!port) return null
    const side: LogicalHandleSide = directPort
      ? (port.classList.contains('react-flow__handle-left') ? 'left' : 'right')
      : requestedSide
    const resolved = readPortPosition(node, element, port, side, cachedNodeBounds)
    cachedNodeBounds = resolved.bounds
    if (!Number.isFinite(resolved.x) || !Number.isFinite(resolved.y)) return null
    return { x: resolved.x, y: resolved.y, side: resolved.side } as const
  }

  const inputs: BundleInput[] = []
  const bundleKeys = new Set<string>()
  const signatureParts: string[] = []
  for (const edge of edges) {
    for (const role of ['source', 'target'] as const) {
      if ((role === 'source' ? edge.source : edge.target) !== node.id) continue
      const handle = role === 'source' ? edge.sourceHandle : edge.targetHandle
      if (!handle) continue
      const key = `${edge.id}:${role}`
      const expanded = expandLineHandles(handle)
      if (expanded.length >= 2) {
        const ports = expanded.flatMap(id => {
          const point = resolveHandle(id)
          return point ? [point] : []
        })
        if (ports.length > 0) {
          bundleKeys.add(key)
          inputs.push({ key, ports, target: role === 'target' })
          signatureParts.push(`${key}:${ports.map(p => `${p.side}:${p.x}:${p.y}`).join(',')}`)
        }
      } else if (isSubHandle(handle)) {
        const single = resolveHandle(handle)
        if (single) {
          inputs.push({ key, ports: [single], target: role === 'target' })
          signatureParts.push(`${key}:${single.side}:${single.x}:${single.y}`)
        }
      }
    }
  }

  const portSignature = signatureParts.join('|')
  if (
    cached &&
    cached.edges === edges &&
    cached.x === origin.x &&
    cached.y === origin.y &&
    cached.w === w &&
    cached.h === h &&
    cached.portSignature === portSignature
  ) {
    return cached.fans
  }

  const fans = bundleFans(inputs)
  for (const key of [...fans.keys()]) {
    if (!bundleKeys.has(key)) fans.delete(key)
  }
  nodeBundleFanCache.set(node, { edges, x: origin.x, y: origin.y, w, h, portSignature, fans })
  return fans
}
