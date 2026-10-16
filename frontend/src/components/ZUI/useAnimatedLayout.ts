import { useEffect, useRef, useState } from 'react'
import type { LayoutNode, ZUILayout } from './types'

const geometry = ['worldX', 'worldY', 'worldW', 'worldH', 'childScale', 'childOffsetX', 'childOffsetY'] as const
const mix = (a: number, b: number, t: number) => a + (b - a) * t

function blendNodes(before: LayoutNode[], after: LayoutNode[], t: number): LayoutNode[] {
  const old = new Map(before.map((node) => [node.id, node]))
  const current = new Set(after.map((node) => node.id))
  const nodes = after.map((node) => {
    const previous = old.get(node.id)
    const next = { ...node, transitionOpacity: mix(previous?.transitionOpacity ?? (previous ? 1 : 0), 1, t) }
    if (previous) for (const key of geometry) next[key] = mix(previous[key], node[key], t)
    next.children = blendNodes(previous?.children ?? [], node.children, t)
    return next
  })
  for (const node of before) {
    if (!current.has(node.id)) nodes.push({ ...node, transitionLeaving: true, transitionOpacity: (node.transitionOpacity ?? 1) * (1 - t) })
  }
  return nodes
}

/** Reconcile by placement identity; inputs remain immutable and hit testing uses
 * the same interpolated geometry as drawing. Removed nodes are noninteractive. */
export function interpolateLayout(before: ZUILayout, after: ZUILayout, t: number): ZUILayout {
  if (t >= 1) return after
  const previous = new Map(before.groups.map((group) => [group.diagramId, group]))
  const ids = new Set(after.groups.map((group) => group.diagramId))
  const groups = after.groups.map((group) => {
    const old = previous.get(group.diagramId)
    const next = { ...group, nodes: blendNodes(old?.nodes ?? [], group.nodes, t) }
    if (old) for (const key of ['worldX', 'worldY', 'worldW', 'worldH', 'diagramX', 'diagramY', 'diagramW', 'diagramH'] as const) next[key] = mix(old[key], group[key], t)
    return next
  })
  for (const group of before.groups) {
    if (!ids.has(group.diagramId)) groups.push({ ...group, edges: [], nodes: blendNodes(group.nodes, [], t) })
  }
  return { groups, bbox: {
    minX: mix(before.bbox.minX, after.bbox.minX, t), minY: mix(before.bbox.minY, after.bbox.minY, t),
    maxX: mix(before.bbox.maxX, after.bbox.maxX, t), maxY: mix(before.bbox.maxY, after.bbox.maxY, t),
  } }
}

export function useAnimatedLayout(target: ZUILayout, enabled: boolean): ZUILayout {
  const [displayed, setDisplayed] = useState(target)
  const current = useRef(target)
  useEffect(() => {
    if (!enabled) { current.current = target; return }
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      current.current = target
      setDisplayed(target)
      return
    }
    const from = current.current
    if (from === target) { setDisplayed(target); return }
    const start = performance.now()
    let frame = 0
    const step = (now: number) => {
      const progress = Math.min(1, Math.max(0, (now - start) / 360))
      current.current = interpolateLayout(from, target, 1 - (1 - progress) ** 3)
      setDisplayed(current.current)
      if (progress < 1) frame = requestAnimationFrame(step)
    }
    frame = requestAnimationFrame(step)
    return () => cancelAnimationFrame(frame)
  }, [target, enabled])
  return enabled ? displayed : target
}
