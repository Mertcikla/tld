import { bundleFans, closestBundleHandle, type BundleFan, type BundleInput, type BundlePort } from '../../utils/bundledConnector'
import { expandLineHandles, isSubElement, isSubHandle, linesForHandle } from '../../utils/subElements'
import { sourceCardScale, subNodeHandleAnchor } from './subElements'
import type { DiagramGroupLayout, LayoutNode } from './types'

type Edge = DiagramGroupLayout['edges'][number]

export interface BundleNodeGeometry {
  node: LayoutNode
  x: number
  y: number
  scale: number
}

export interface ZUIBundleFan extends BundleFan {
  originX: number
  originY: number
  scale: number
  ports: BundlePort[]
}

const zuiNodeBundleCache = new WeakMap<LayoutNode, {
  portSignature: string
  fans: Map<string, BundleFan>
  portsByKey: Map<string, BundlePort[]>
}>()

/**
 * Align visual bundle handle sides toward their connected peer in layout space
 * without mutating saved connector definitions.
 */
export function alignCodeBundleHandles(nodes: LayoutNode[], edges: Edge[]): void {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  for (const edge of edges) {
    const source = byId.get(edge.sourceId)
    const target = byId.get(edge.targetId)
    if (!source || !target || source.id === target.id) continue
    const sx = source.worldX + source.worldW / 2
    const tx = target.worldX + target.worldW / 2
    const bundled = linesForHandle(edge.sourceHandle ?? '').length > 1 || linesForHandle(edge.targetHandle ?? '').length > 1
    if (edge.sourceHandle && isSubElement(source.subKind ?? source.type)) {
      edge.sourceHandle = closestBundleHandle(edge.sourceHandle, sx, tx, bundled)
    }
    if (edge.targetHandle && isSubElement(target.subKind ?? target.type)) {
      edge.targetHandle = closestBundleHandle(edge.targetHandle, tx, sx, bundled)
    }
  }

  for (const node of nodes) {
    node.edgesOut = edges.filter((edge) => edge.sourceId === node.id)
    node.subConnectedHandles = edges.flatMap((edge) => [
      ...(edge.sourceId === node.id && edge.sourceHandle ? [edge.sourceHandle] : []),
      ...(edge.targetId === node.id && edge.targetHandle ? [edge.targetHandle] : []),
    ])
    node.subConnections = edges.flatMap((edge) => [
      ...(edge.sourceId === node.id && edge.sourceHandle ? [{ id: edge.id, handleId: edge.sourceHandle }] : []),
      ...(edge.targetId === node.id && edge.targetHandle ? [{ id: edge.id, handleId: edge.targetHandle }] : []),
    ])
    if (node.children.length > 0) {
      const childrenEdges = node.children.flatMap((child) =>
        child.edgesOut.map((edge) => ({ ...edge, sourceId: child.id })),
      )
      alignCodeBundleHandles(node.children, childrenEdges)
    }
  }
}

/**
 * Build world-space bundle fan routes and port positions shared by Canvas drawing
 * and spatial hover/click hit-testing.
 */
export function buildZUIBundleFans(
  geometries: readonly BundleNodeGeometry[],
  edges: readonly Edge[],
  zoom: number,
): Map<string, ZUIBundleFan> {
  const result = new Map<string, ZUIBundleFan>()

  const edgesByNodeId = new Map<string, Array<{ edge: Edge; role: 'source' | 'target'; handle: string }>>()
  for (const edge of edges) {
    if (edge.sourceHandle && isSubHandle(edge.sourceHandle)) {
      let list = edgesByNodeId.get(edge.sourceId)
      if (!list) {
        list = []
        edgesByNodeId.set(edge.sourceId, list)
      }
      list.push({ edge, role: 'source', handle: edge.sourceHandle })
    }
    if (edge.targetHandle && isSubHandle(edge.targetHandle)) {
      let list = edgesByNodeId.get(edge.targetId)
      if (!list) {
        list = []
        edgesByNodeId.set(edge.targetId, list)
      }
      list.push({ edge, role: 'target', handle: edge.targetHandle })
    }
  }

  for (const geometry of geometries) {
    const { node } = geometry
    if (!isSubElement(node.subKind ?? node.type)) continue
    const nodeEdges = edgesByNodeId.get(node.id)
    if (!nodeEdges || !nodeEdges.some((item) => expandLineHandles(item.handle).length >= 2)) continue

    const cardScale = sourceCardScale(node)
    const inputs: BundleInput[] = []
    const bundlePortsByKey = new Map<string, BundlePort[]>()
    const sigParts: string[] = []

    for (const { edge, role, handle } of nodeEdges) {
      const key = `${edge.id}:${role}`
      const expanded = expandLineHandles(handle)
      if (expanded.length >= 2) {
        const rawPorts = expanded.flatMap((id) => {
          const anchor = subNodeHandleAnchor(node, id, zoom * geometry.scale)
          return anchor ? [{ x: anchor.x / cardScale, y: anchor.y / cardScale, side: anchor.side }] : []
        })
        const uniquePorts = [
          ...new Map(rawPorts.map((port) => [`${port.side}:${port.x}:${port.y}`, port])).values(),
        ].sort((a, b) => a.y - b.y || a.x - b.x)
        if (uniquePorts.length > 0) {
          bundlePortsByKey.set(key, uniquePorts)
          inputs.push({ key, ports: uniquePorts, target: role === 'target' })
          sigParts.push(`${key}:${role}:${uniquePorts.map((p) => `${p.side}:${p.x}:${p.y}`).join(',')}`)
        }
      } else {
        const anchor = subNodeHandleAnchor(node, handle, zoom * geometry.scale)
        if (anchor) {
          const single = { x: anchor.x / cardScale, y: anchor.y / cardScale, side: anchor.side }
          inputs.push({ key, ports: [single], target: role === 'target' })
          sigParts.push(`${key}:${role}:${single.side}:${single.x}:${single.y}`)
        }
      }
    }

    if (bundlePortsByKey.size === 0) continue

    const portSignature = sigParts.join('|')
    const cached = zuiNodeBundleCache.get(node)
    let fans: Map<string, BundleFan>
    if (cached && cached.portSignature === portSignature) {
      fans = cached.fans
    } else {
      fans = bundleFans(inputs)
      zuiNodeBundleCache.set(node, { portSignature, fans, portsByKey: bundlePortsByKey })
    }

    const scale = geometry.scale * cardScale
    for (const [key, fan] of fans) {
      const ports = bundlePortsByKey.get(key)
      if (!ports) continue
      result.set(key, {
        ...fan,
        x: geometry.x + fan.x * scale,
        y: geometry.y + fan.y * scale,
        originX: geometry.x,
        originY: geometry.y,
        scale,
        ports: ports.map((port) => ({
          ...port,
          x: geometry.x + port.x * scale,
          y: geometry.y + port.y * scale,
        })),
      })
    }
  }

  return result
}
