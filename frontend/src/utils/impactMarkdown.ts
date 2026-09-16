import type { ImpactEdge, ImpactElement, ImpactReport } from '../api/client'

// Mirrors internal/watch/impact_render.go and internal/watch/impact_diagram.go
// so the PR-comment markdown and Mermaid diagram produced in the UI match
// `tld impact --render markdown --diagram <style>`.

export type ImpactDiagramStyle = 'full' | 'bounded' | 'lanes' | 'groups'

const nodeBudget = 10
const edgeBudget = 16

export interface ImpactDiagramGroup {
  name: string
  touched: number
  members: string[]
}

export interface ImpactDiagramData {
  style: ImpactDiagramStyle
  code: string
  nodes: ImpactElement[]
  edges: ImpactEdge[]
  omittedNodes: number
  omittedEdges: number
  groups: ImpactDiagramGroup[]
  internalEdges: number
}

function escapeMermaidLabel(value: string): string {
  return value.replace(/"/g, "'").replace(/\r?\n/g, ' ').trim()
}

const nodeRefs = (nodes: ImpactElement[]): Set<string> => new Set(nodes.map((node) => node.ref))

function uniqueNodes(nodes: ImpactElement[]): ImpactElement[] {
  const seen = new Set<string>()
  const out: ImpactElement[] = []
  for (const node of nodes) {
    if (seen.has(node.ref)) continue
    seen.add(node.ref)
    out.push(node)
  }
  return out
}

function sortedNodes(nodes: ImpactElement[]): ImpactElement[] {
  return uniqueNodes(nodes).sort((a, b) => (a.ref < b.ref ? -1 : a.ref > b.ref ? 1 : 0))
}

const edgeKey = (edge: ImpactEdge): string =>
  [edge.source_ref, edge.target_ref, edge.observed, edge.label ?? ''].join('\u0000')

function uniqueEdges(edges: ImpactEdge[]): ImpactEdge[] {
  const seen = new Set<string>()
  const out: ImpactEdge[] = []
  for (const edge of edges) {
    const key = edgeKey(edge)
    if (seen.has(key)) continue
    seen.add(key)
    out.push(edge)
  }
  return out.sort((a, b) => (edgeKey(a) < edgeKey(b) ? -1 : edgeKey(a) > edgeKey(b) ? 1 : 0))
}

function validEdges(nodes: ImpactElement[], edges: ImpactEdge[]): ImpactEdge[] {
  const refs = nodeRefs(nodes)
  return edges.filter((edge) => refs.has(edge.source_ref) && refs.has(edge.target_ref))
}

/** Budgets are soft: changed nodes, changed-to-changed edges and one edge per
 * selected neighbor take precedence. Candidates never become confirmed context. */
export function boundedDiagram(report: ImpactReport, nodeLimit = nodeBudget, edgeLimit = edgeBudget) {
  const changed = sortedNodes(report.changed)
  const changedRefs = nodeRefs(changed)
  const related = sortedNodes(report.related).filter((node) => !changedRefs.has(node.ref))
  const edges = uniqueEdges(validEdges([...changed, ...related], report.edges))
  const incident = (ref: string) => edges.filter((edge) =>
    (edge.source_ref === ref && changedRefs.has(edge.target_ref)) ||
    (edge.target_ref === ref && changedRefs.has(edge.source_ref)))
  const ranked = related.map((node) => {
    const connections = incident(node.ref)
    const touched = new Set(connections.map((edge) => edge.source_ref === node.ref ? edge.target_ref : edge.source_ref)).size
    return { node, connections, touched, observed: connections.filter((edge) => edge.observed).length }
  }).filter((entry) => entry.connections.length > 0)
    .sort((a, b) => b.touched - a.touched || b.observed - a.observed || (a.node.ref < b.node.ref ? -1 : 1))
    .slice(0, Math.max(0, nodeLimit - changed.length))
  const nodes = [...changed, ...ranked.map((entry) => entry.node)]
  const eligible = validEdges(nodes, edges)
  const selected = new Map<string, ImpactEdge>()
  const add = (edge: ImpactEdge) => selected.set(edgeKey(edge), edge)
  eligible.filter((edge) => changedRefs.has(edge.source_ref) && changedRefs.has(edge.target_ref)).forEach(add)
  for (const entry of ranked) {
    const first = [...entry.connections].sort((a, b) => Number(b.observed) - Number(a.observed) || (edgeKey(a) < edgeKey(b) ? -1 : 1))[0]
    add(first)
  }
  // Spend remaining slots in neighbor-rank order, observed relationships first.
  const rank = new Map(ranked.map((entry, index) => [entry.node.ref, index]))
  const rankOf = (edge: ImpactEdge) => rank.get(edge.source_ref) ?? rank.get(edge.target_ref) ?? -1
  eligible.sort((a, b) =>
    rankOf(a) - rankOf(b) ||
    Number(b.observed) - Number(a.observed) || (edgeKey(a) < edgeKey(b) ? -1 : 1))
  for (const edge of eligible) {
    if (selected.size >= edgeLimit) break
    add(edge)
  }
  return {
    nodes, edges: uniqueEdges([...selected.values()]),
    omittedNodes: changed.length + related.length - nodes.length,
    omittedEdges: edges.length - selected.size,
  }
}

export function laneFor(ref: string, changed: Set<string>, edges: ImpactEdge[]): 'changed' | 'incoming' | 'outgoing' | 'both' {
  if (changed.has(ref)) return 'changed'
  const incoming = edges.some((edge) => edge.source_ref === ref && changed.has(edge.target_ref))
  const outgoing = edges.some((edge) => edge.target_ref === ref && changed.has(edge.source_ref))
  if (incoming && outgoing) return 'both'
  if (incoming) return 'incoming'
  return 'outgoing'
}

function renderGraph(nodes: ImpactElement[], edges: ImpactEdge[], changed: Set<string>, lanes: boolean): string {
  const lines: string[] = [lanes ? 'flowchart LR' : 'flowchart TD']
  const ids = new Map<string, string>()
  nodes.forEach((node, index) => ids.set(node.ref, `n${index + 1}`))
  const nodeLine = (node: ImpactElement) => `  ${ids.get(node.ref)}["${escapeMermaidLabel((node.name ?? '').trim() || node.ref)}"]`
  if (lanes) {
    const laneOrder: { key: ReturnType<typeof laneFor>; title: string }[] = [
      { key: 'incoming', title: 'Incoming context' },
      { key: 'changed', title: 'Code touched' },
      { key: 'outgoing', title: 'Outgoing context' },
      { key: 'both', title: 'Bidirectional context' },
    ]
    for (const lane of laneOrder) {
      const members = nodes.filter((node) => laneFor(node.ref, changed, edges) === lane.key)
      if (!members.length) continue
      lines.push(`  subgraph lane_${lane.key}["${lane.title}"]`, '    direction TB', ...members.map(nodeLine), '  end',
        `  style lane_${lane.key} fill:#172234,stroke:#42536a,color:#cbd5e0`)
    }
  } else {
    lines.push(...nodes.map(nodeLine))
  }
  for (const edge of edges) {
    const sourceID = ids.get(edge.source_ref)
    const targetID = ids.get(edge.target_ref)
    if (!sourceID || !targetID) continue
    const arrow = edge.observed ? '-.->' : '-->'
    lines.push((edge.label ?? '').trim()
      ? `  ${sourceID} ${arrow}|${escapeMermaidLabel(edge.label)}| ${targetID}`
      : `  ${sourceID} ${arrow} ${targetID}`)
  }
  const changedIDs = nodes.filter((node) => changed.has(node.ref)).map((node) => ids.get(node.ref)!).sort()
  if (changedIDs.length) {
    lines.push(`  class ${changedIDs.join(',')} changed`)
    lines.push('  classDef changed fill:#fde68a,stroke:#b45309,stroke-width:2px,color:#78350f;')
  }
  return lines.join('\n')
}

function groupedDiagram(report: ImpactReport) {
  const source = sortedNodes([...report.changed, ...report.related])
  const changed = new Set(report.changed.map((node) => node.ref))
  // Prefixes avoid collisions between an owner ref and an ungrouped element ref.
  const groupRef = (node: ImpactElement) => node.owner?.trim() ? `owner:${node.owner.trim()}` : `element:${node.ref}`
  const buckets = new Map<string, ImpactElement[]>()
  for (const node of source) {
    const ref = groupRef(node)
    buckets.set(ref, [...(buckets.get(ref) ?? []), node])
  }
  const groups: ImpactDiagramGroup[] = []
  const touchedGroups = new Set<string>()
  const nodes = [...buckets.keys()].sort().map((ref) => {
    const members = buckets.get(ref)!
    const touched = members.filter((node) => changed.has(node.ref)).length
    if (touched) touchedGroups.add(ref)
    const name = members[0].owner?.trim() || members[0].name
    groups.push({ name, touched, members: members.map((node) => node.name) })
    return { ...members[0], ref, name: `${name} (${touched}/${members.length} touched)` }
  })
  const aggregated = new Map<string, { source_ref: string; target_ref: string; observed: boolean; count: number }>()
  let internalEdges = 0
  for (const edge of uniqueEdges(validEdges(source, report.edges))) {
    const source_ref = groupRef(source.find((node) => node.ref === edge.source_ref)!)
    const target_ref = groupRef(source.find((node) => node.ref === edge.target_ref)!)
    if (source_ref === target_ref) { internalEdges++; continue }
    const key = [source_ref, target_ref, edge.observed].join('\u0000')
    const existing = aggregated.get(key)
    if (existing) { existing.count += 1; continue }
    aggregated.set(key, { source_ref, target_ref, observed: edge.observed, count: 1 })
  }
  const edges = uniqueEdges([...aggregated.values()].map(({ count, ...edge }) => ({
    ...edge, label: `${count} ${edge.observed ? 'observed' : 'declared'}`,
  })))
  return { nodes, edges, groups, touchedGroups, internalEdges }
}

function fullDiagramNodes(report: ImpactReport): ImpactElement[] {
  return uniqueNodes([...report.changed, ...report.candidates, ...report.related])
}

function withCoverage(report: ImpactReport, code: string): string {
  if (!report.coverage.applicable) return code
  return `%% coverage: ${report.coverage.percent}% (${report.coverage.confidence})\n${code}`
}

export function buildImpactDiagram(report: ImpactReport, style: ImpactDiagramStyle = 'full'): ImpactDiagramData {
  const common = { omittedNodes: 0, omittedEdges: 0, groups: [] as ImpactDiagramGroup[], internalEdges: 0 }
  if (style === 'groups') {
    const graph = groupedDiagram(report)
    return { ...common, style, nodes: graph.nodes, edges: graph.edges, groups: graph.groups, internalEdges: graph.internalEdges, code: withCoverage(report, renderGraph(graph.nodes, graph.edges, graph.touchedGroups, false)) }
  }
  if (style === 'bounded' || style === 'lanes') {
    const graph = boundedDiagram(report)
    return { ...common, style, ...graph, code: withCoverage(report, renderGraph(graph.nodes, graph.edges, new Set(report.changed.map((node) => node.ref)), style === 'lanes')) }
  }
  const nodes = fullDiagramNodes(report)
  const edges = validEdges(nodes, report.edges)
  return { ...common, style: 'full', nodes, edges, code: withCoverage(report, renderGraph(nodes, edges, new Set(report.changed.map((node) => node.ref)), false)) }
}

export function impactMermaid(report: ImpactReport, style: ImpactDiagramStyle = 'full'): string {
  return buildImpactDiagram(report, style).code
}

function coverageSummaryLine(report: ImpactReport): string {
  const coverage = report.coverage
  if (!coverage.applicable) return 'No source code changed — nothing to reconcile.'
  const status = coverage.complete ? 'analysis is complete' : 'analysis may be incomplete'
  return `${coverage.percent}% (${coverage.confidence}) — ${coverage.bound_source_files}/${coverage.source_files} source files owned; ${status}`
}

function evidenceSummary(evidence: string[] | undefined): string {
  const parts: string[] = []
  const seen = new Set<string>()
  for (const item of evidence ?? []) {
    const value = (item ?? '').trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    parts.push(value)
    if (parts.length >= 3) break
  }
  return parts.join(', ')
}

function elementLine(element: ImpactElement): string {
  const evidence = evidenceSummary(element.evidence)
  return evidence ? `- ${element.name} — ${evidence}` : `- ${element.name}`
}

export function impactMarkdown(report: ImpactReport, style: ImpactDiagramStyle = 'full'): string {
  const lines: string[] = []
  lines.push('## Architecture Impact', '')

  lines.push('', '```mermaid', impactMermaid(report, style), '```')
  const summaryLine = coverageSummaryLine(report)
  if (summaryLine) lines.push(`**Coverage:** ${summaryLine}`, '')
  if (report.changed.length === 0 && report.candidates.length === 0 && report.unmapped.length === 0) {
    lines.push('No architecture-bound code changed.')
    return lines.join('\n')
  }

  if (report.candidates.length > 0) {
    lines.push('**Candidates** _(weak name matches)_', '')
    for (const element of report.candidates) lines.push(elementLine(element))
  }

  if (report.coverage.gaps.length > 0) {
    lines.push('', '**Binding gaps**', '')
    lines.push('_Add or extend a binding so impact analysis covers these files, then re-run `tld impact`._', '')
    for (const gap of report.coverage.gaps) {
      let line = `- \`${gap.file}\``
      if (gap.change) line += ` (${gap.change})`
      line += ` — ${gap.reason}`
      lines.push(line)
      const score = gap.suggested_score.toFixed(2)
      if (gap.suggested_element_ref && !gap.suggested_element_pattern) {
        lines.push(`  - suggested owner: ${gap.suggested_element_name} (${score}) — \`tld bind ${gap.suggested_element_ref} --file ${JSON.stringify(gap.file)}\``)
      } else if (gap.suggested_element_ref) {
        lines.push(`  - suggested owner: ${gap.suggested_element_name} (${score}) already owns \`${gap.suggested_element_pattern}\`; extend that binding if this file belongs to it`)
      }
      if (gap.suggested_new_element) {
        lines.push(`  - or create an element: \`tld add ${JSON.stringify(gap.suggested_new_element)} --file ${JSON.stringify(gap.file)}\``)
      }
    }
  }

  if (report.edges.some((edge) => edge.observed)) {
    lines.push('', '_Dashed edges are observed in code but not declared in the architecture._')
  }

  return lines.join('\n')
}
