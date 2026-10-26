import { MarkerType, type Edge, type Node } from 'reactflow'
import type { FlowFocus } from './model'
import type { LayoutNode, ZUILayout } from '../components/ZUI/types'

export function presentEditorFlow<N extends Node, E extends Edge>(nodes: N[], edges: E[], focus: FlowFocus | null, editing = false): { nodes: N[]; edges: E[] } {
  if (!focus?.elementIds.length) return { nodes, edges }
  const activeNodes = new Set(focus.elementIds.map(String))
  const activeEdges = new Map(focus.connectors.map(connector => [String(connector.id), connector]))
  const marker = { type: MarkerType.ArrowClosed, color: 'var(--accent)' }
  return {
    nodes: nodes.map(node => ({ ...node, style: { ...node.style, opacity: activeNodes.has(node.id) ? 1 : editing ? 0.6 : 0.18, ...(activeNodes.has(node.id) ? { filter: 'drop-shadow(0 0 8px rgba(var(--accent-rgb), 0.4))' } : {}) } })),
    edges: edges.map(edge => {
      const step = activeEdges.get(edge.id)
      return {
        ...edge,
        style: { ...edge.style, opacity: step ? 1 : editing ? 0.45 : 0.12, ...(step ? { stroke: 'var(--accent)', strokeWidth: 3 } : {}) },
        labelStyle: { ...edge.labelStyle, opacity: step ? 1 : editing ? 0.45 : 0.12 },
        ...(step ? { data: { ...edge.data, direction: step.reverse ? 'backward' : 'forward' }, markerStart: step.reverse ? marker : undefined, markerEnd: step.reverse ? undefined : marker } : {}),
      }
    }),
  }
}

export function presentZUIFlow(layout: ZUILayout, focus: FlowFocus | null): ZUILayout {
  if (!focus?.elementIds.length) return layout
  const activeNodes = new Set(focus.elementIds)
  const activeEdges = new Map(focus.connectors.map(connector => [connector.id, connector]))
  function decorateEdge<E extends { id: number; direction: string; label: string }>(edge: E, viewId: number): E {
    const step = viewId === focus!.viewId ? activeEdges.get(edge.id) : undefined
    return { ...edge, tagHighlighted: !!step, tagMuted: !step, ...(step ? { direction: step.reverse ? 'backward' : 'forward' } : {}) }
  }
  function decorate(node: LayoutNode): LayoutNode {
    const children = node.children.map(decorate)
    const active = node.diagramId === focus!.viewId && activeNodes.has(node.elementId)
    return {
      ...node, children, tagContext: false,
      tagHoverState: active || children.some(child => child.tagHoverState !== 'inactive') ? 'match' : 'inactive',
      edgesOut: node.edgesOut.map(edge => decorateEdge(edge, node.diagramId)),
    }
  }
  return { ...layout, groups: layout.groups.map(group => ({ ...group, nodes: group.nodes.map(decorate), edges: group.edges.map(edge => decorateEdge(edge, group.diagramId)) })) }
}
