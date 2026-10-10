import type { ExploreData, ViewTreeNode } from '../types'
import type { ZUINodeProvenance } from '../components/ZUI/types'
import { connectorChangeFromTags } from '../components/ZUI/edgeChange'

/** The overlay fields the text diagram renders: kind plus line churn. */
export interface SceneMermaidOverlay {
  change: 'added' | 'removed' | 'modified' | 'unchanged'
  linesAdded?: number
  linesRemoved?: number
}

export interface SceneMermaidView {
  data: ExploreData
  overlays: Record<number, SceneMermaidOverlay>
  provenance?: Record<number, ZUINodeProvenance>
}

export interface SceneMermaidOptions {
  repositoryId: string
  comparisonKey: string
  scope: string
  radius: number
}

// Mirrors internal/mermaid EscapeMetadataValue so comment metadata
// round-trips identically on both renderers.
const escapeMetadata = (value: string): string => value
  .replace(/\\/g, '\\\\')
  .replace(/\n/g, '\\n')
  .replace(/\r/g, '\\r')
  .replace(/\t/g, '\\t')
  .replace(/=/g, '\\=')
  .replace(/,/g, '\\,')
  .replace(/\|/g, '\\|')
  .replace(/:/g, '\\:')

// Mirrors internal/mermaid/scene.go rule for rule: this renders the already
// scope/radius/plain-filtered canvas data, so the text diagram shows exactly
// what the canvas draws — views, nodes, badges, and connector arrows.
export function sceneMermaid(view: SceneMermaidView, options: SceneMermaidOptions): string {
  const lines = ['flowchart LR']
  lines.push(`%% tld-scene repo=${escapeMetadata(options.repositoryId)} key=${escapeMetadata(options.comparisonKey)} scope=${options.scope} radius=${options.radius}`)
  let linkIndex = 0
  const linkStyles: string[] = []
  const num = (id: number): string => (id < 0 ? `neg${-id}` : String(id))
  const nodeRef = (viewId: number, elementId: number): string => `el_${num(viewId)}_${num(elementId)}`
  const escape = (value: string): string => value
    .replace(/\r\n/g, ' ')
    .replace(/\n/g, ' ')
    .replace(/\r/g, ' ')
    .replace(/&/g, '&amp;')
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '&quot;')
  const glyphOf = (viewId: number, elementId: number): string => {
    const provenance = view.provenance?.[elementId]
    if (provenance === 'augmented') return '◇ '
    if (provenance === 'generated') return '▦ '
    return ''
  }
  const labelOf = (viewId: number, elementId: number, name: string): string => {
    let label = `${glyphOf(viewId, elementId)}${name.trim() === '' ? `element ${elementId}` : name}`
    const overlay = view.overlays[elementId]
    if (overlay) {
      if (overlay.change !== 'unchanged') {
        label += `<br/>${overlay.change} +${overlay.linesAdded ?? 0} \u2212${overlay.linesRemoved ?? 0}`
      } else {
        label += '<br/>(context)'
      }
    }
    return label
  }
  const connectorLine = (viewId: number, source: number, target: number, label: string | null, tags?: string[]): string => {
    const from = nodeRef(viewId, source)
    const to = nodeRef(viewId, target)
    const text = (label ?? '').trim()
    const styleLink = (style: string): void => {
      // No trailing semicolon: the mermaid grammar rejects it.
      linkStyles.push(`linkStyle ${linkIndex} ${style}`)
      linkIndex += 1
    }
    switch (connectorChangeFromTags(tags)) {
      case 'added':
        styleLink('stroke:#48bb78')
        return text !== '' ? `${from} ==>|"${escape(text)}"| ${to}` : `${from} ==> ${to}`
      case 'removed':
        styleLink('stroke:#fc8181,stroke-dasharray:5 5')
        return text !== '' ? `${from}--x|"${escape(text)}"|${to}` : `${from}--x${to}`
      case 'modified':
        styleLink('stroke:#ecc94b')
        return text !== '' ? `${from} -- "${escape(text)}" --> ${to}` : `${from} --> ${to}`
      default:
        return text !== '' ? `${from} -- "${escape(text)}" --> ${to}` : `${from} --> ${to}`
    }
  }
  // Tree order, flattened: parents append after their children, exactly like
  // the backend walk, so subgraph order matches across renderers.
  const walk = (tree: ViewTreeNode[]): number[] => tree.flatMap((node) => [...walk(node.children ?? []), node.id])
  for (const viewId of walk(view.data.tree)) {
    const content = view.data.views[String(viewId)]
    if (!content) continue
    lines.push('', `subgraph view_${num(viewId)}["${escape(viewName(view.data.tree, viewId))}"]`)
    const rendered = new Set<number>()
    for (const element of content.placements) {
      rendered.add(element.element_id)
      lines.push(`  ${nodeRef(viewId, element.element_id)}["${escape(labelOf(viewId, element.element_id, element.name))}"]`)
    }
    for (const connector of content.connectors ?? []) {
      if (!rendered.has(connector.source_element_id) || !rendered.has(connector.target_element_id)) continue
      lines.push(`  ${connectorLine(viewId, connector.source_element_id, connector.target_element_id, connector.label, connector.tags)}`)
    }
    lines.push('end')
  }
  if (linkStyles.length > 0) {
    lines.push('', ...linkStyles)
  }
  return `${lines.join('\n')}\n`
}

function viewName(tree: ViewTreeNode[], viewId: number): string {
  return findViewName(tree, viewId) ?? `view ${viewId}`
}

function findViewName(tree: ViewTreeNode[], viewId: number): string | undefined {
  for (const node of tree) {
    if (node.id === viewId) return node.name
    const nested = findViewName(node.children ?? [], viewId)
    if (nested !== undefined) return nested
  }
  return undefined
}
