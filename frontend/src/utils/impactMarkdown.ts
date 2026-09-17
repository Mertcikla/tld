import type { ImpactEdge, ImpactElement, ImpactFile, ImpactReport } from '../api/client'

// Mirrors internal/watch/impact_render.go and internal/watch/impact_diagram.go
// so the PR-comment markdown and Mermaid diagram produced in the UI match
// `tld impact --render markdown`.
//
// The diagram is a single reviewer projection: the bounded selection (changed
// elements plus their most connected neighbors) grouped into dependency
// lanes, with changed elements carrying a compact change badge instead of
// file paths. Containment edges derived by the backend from nested folder
// bindings render distinctly so folder hierarchies stay connected.

/** Deprecated: diagram styles were consolidated into a single reviewer view. Kept for compatibility; values are ignored. */
export type ImpactDiagramStyle = 'review' | 'full' | 'bounded' | 'lanes' | 'groups'

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

export function isContainmentEdge(edge: ImpactEdge): boolean {
  return !edge.observed && edge.label.trim().toLowerCase() === 'contains'
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

/** Compact change badge for a touched node, e.g. "3 files (+24 -5)". File
 * paths live in the files panel and markdown body; the diagram carries only
 * the aggregate so labels stay legible. */
function changeBadge(node: ImpactElement, files: Map<string, ImpactFile>): string {
  const paths = [...new Set((node.evidence ?? []).map((value) => (value ?? '').trim()).filter((value) => value.includes('/')))]
  if (!paths.length) return ''
  let added = 0
  let removed = 0
  let uniform: string | null = null
  for (const path of paths) {
    const file = files.get(path)
    if (!file) {
      uniform = uniform === null ? '' : uniform
      continue
    }
    added += file.added
    removed += file.removed
    const change = file.change === 'added' ? 'added' : file.change === 'deleted' ? 'deleted' : 'modified'
    if (uniform === null) uniform = change
    else if (uniform !== change) uniform = 'mixed'
  }
  const count = paths.length === 1 ? '1 file' : `${paths.length} files`
  if (added + removed > 0) return `${count} (+${added} -${removed})`
  if (uniform === 'added') return `${count} (added)`
  if (uniform === 'deleted') return `${count} (deleted)`
  return count
}

function changeBadges(report: ImpactReport, nodes: ImpactElement[]): Map<string, string> {
  const changed = new Set(report.changed.map((node) => node.ref))
  const files = new Map(report.changed_files.map((file) => [file.path, file]))
  const badges = new Map<string, string>()
  for (const node of nodes) {
    if (!changed.has(node.ref)) continue
    const badge = changeBadge(node, files)
    if (badge) badges.set(node.ref, badge)
  }
  return badges
}

function pluralize(count: number, unit: string): string {
  return count === 1 ? `1 ${unit}` : `${count} ${unit}s`
}

function omittedLine(omittedNodes: number, omittedEdges: number): string {
  const parts: string[] = []
  if (omittedNodes > 0) parts.push(pluralize(omittedNodes, 'node'))
  if (omittedEdges > 0) parts.push(pluralize(omittedEdges, 'edge'))
  if (!parts.length) return ''
  return `%% +${parts.join(', +')} omitted`
}

function renderGraph(
  nodes: ImpactElement[],
  edges: ImpactEdge[],
  changed: Set<string>,
  badges: Map<string, string>,
): string {
  const lines: string[] = ['flowchart LR']
  const ids = new Map<string, string>()
  nodes.forEach((node, index) => ids.set(node.ref, `n${index + 1}`))
  const nodeLine = (node: ImpactElement) => {
    let label = escapeMermaidLabel((node.name ?? '').trim() || node.ref)
    const badge = (badges.get(node.ref) ?? '').trim()
    if (badge) label += `<br/>${escapeMermaidLabel(badge)}`
    return `  ${ids.get(node.ref)}["${label}"]`
  }
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
  for (const edge of edges) {
    const sourceID = ids.get(edge.source_ref)
    const targetID = ids.get(edge.target_ref)
    if (!sourceID || !targetID) continue
    let arrow = '-->'
    if (edge.observed) arrow = '-.->'
    else if (isContainmentEdge(edge)) arrow = '--o'
    let label = (edge.label ?? '').trim()
    if (!label) {
      if (edge.observed) label = 'observed'
      else if (isContainmentEdge(edge)) label = 'contains'
    }
    lines.push(label
      ? `  ${sourceID} ${arrow}|${escapeMermaidLabel(label)}| ${targetID}`
      : `  ${sourceID} ${arrow} ${targetID}`)
  }
  const changedIDs = nodes.filter((node) => changed.has(node.ref)).map((node) => ids.get(node.ref)!).sort()
  if (changedIDs.length) {
    lines.push(`  class ${changedIDs.join(',')} changed`)
    lines.push('  classDef changed fill:#fde68a,stroke:#b45309,stroke-width:2px,color:#78350f;')
  }
  return lines.join('\n')
}

function withCoverage(report: ImpactReport, code: string): string {
  if (!report.coverage.applicable) return code
  return `%% coverage: ${report.coverage.percent}% (${report.coverage.confidence})\n${code}`
}

/** Builds the single reviewer diagram. The style argument is deprecated and ignored. */
export function buildImpactDiagram(report: ImpactReport, _style?: ImpactDiagramStyle): ImpactDiagramData {
  const graph = boundedDiagram(report)
  const changed = new Set(report.changed.map((node) => node.ref))
  const badges = changeBadges(report, graph.nodes)
  const omitted = omittedLine(graph.omittedNodes, graph.omittedEdges)
  const graphCode = renderGraph(graph.nodes, graph.edges, changed, badges)
  const code = withCoverage(report, omitted ? `${omitted}\n${graphCode}` : graphCode)
  return {
    style: 'review',
    nodes: graph.nodes,
    edges: graph.edges,
    omittedNodes: graph.omittedNodes,
    omittedEdges: graph.omittedEdges,
    groups: [],
    internalEdges: 0,
    code,
  }
}

/** Renders the single reviewer diagram as Mermaid. The style argument is deprecated and ignored. */
export function impactMermaid(report: ImpactReport, _style?: ImpactDiagramStyle): string {
  return buildImpactDiagram(report).code
}

function coverageSummaryLine(report: ImpactReport): string {
  const coverage = report.coverage
  if (!coverage.applicable) return 'No source code changed — nothing to reconcile.'
  let status = 'all changed source files are covered'
  if (!coverage.complete) status = 'analysis may be incomplete'
  else if (coverage.confidence === 'low') status = 'coverage is limited to broad bindings'
  return `${coverage.percent}% (${coverage.confidence}) — ${coverage.bound_source_files}/${coverage.source_files} source files covered; ${status}`
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

/** Renders the PR-comment markdown with the single reviewer diagram. The style argument is deprecated and ignored. */
export function impactMarkdown(report: ImpactReport, _style?: ImpactDiagramStyle): string {
  const lines: string[] = []
  lines.push('## Architecture Impact', '')

  lines.push('', '```mermaid', impactMermaid(report), '```')
  const summaryLine = coverageSummaryLine(report)
  if (summaryLine) lines.push(`**Coverage:** ${summaryLine}`, '')
  if (report.changed.length === 0 && report.candidates.length === 0 && report.unmapped.length === 0) {
    lines.push('No architecture-bound code changed.')
    return lines.join('\n')
  }

  const diagram = buildImpactDiagram(report)
  if (diagram.omittedNodes > 0 || diagram.omittedEdges > 0) {
    const parts: string[] = []
    if (diagram.omittedNodes > 0) parts.push(pluralize(diagram.omittedNodes, 'node'))
    if (diagram.omittedEdges > 0) parts.push(pluralize(diagram.omittedEdges, 'edge'))
    lines.push(`_Graph trimmed for readability: +${parts.join(', +')} omitted._`, '')
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
  if (report.edges.some(isContainmentEdge)) {
    lines.push('', '_–o edges show folder containment between bound elements._')
  }

  return lines.join('\n')
}
