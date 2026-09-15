import type { ImpactElement, ImpactReport } from '../api/client'

// Mirrors internal/watch/impact_render.go so the PR-comment markdown and Mermaid
// diagram produced in the UI match `tld impact --render markdown`.

function escapeMermaidLabel(value: string): string {
  return value.replace(/"/g, "'").replace(/\r?\n/g, ' ').trim()
}

export function impactMermaid(report: ImpactReport): string {
  const lines: string[] = []
  if (report.coverage.applicable) {
    lines.push(`%% coverage: ${report.coverage.percent}% (${report.coverage.confidence})`)
  }
  lines.push('flowchart TD')

  const ids = new Map<string, string>()
  let index = 0
  const idFor = (ref: string, name: string): string => {
    const existing = ids.get(ref)
    if (existing) return existing
    index += 1
    const id = `n${index}`
    ids.set(ref, id)
    const label = (name ?? '').trim() || ref
    lines.push(`  ${id}["${escapeMermaidLabel(label)}"]`)
    return id
  }

  const changedIDs = new Set<string>()
  for (const element of report.changed) changedIDs.add(idFor(element.ref, element.name))
  for (const element of report.candidates) idFor(element.ref, element.name)
  for (const element of report.related) idFor(element.ref, element.name)

  for (const edge of report.edges) {
    const sourceID = ids.get(edge.source_ref)
    const targetID = ids.get(edge.target_ref)
    if (!sourceID || !targetID) continue
    const arrow = edge.observed ? '-.->' : '-->'
    if ((edge.label ?? '').trim()) {
      lines.push(`  ${sourceID} ${arrow}|${escapeMermaidLabel(edge.label)}| ${targetID}`)
    } else {
      lines.push(`  ${sourceID} ${arrow} ${targetID}`)
    }
  }

  if (changedIDs.size > 0) {
    const sorted = Array.from(changedIDs).sort()
    lines.push(`  class ${sorted.join(',')} changed`)
    lines.push('  classDef changed fill:#fde68a,stroke:#b45309,stroke-width:2px;')
  }
  return lines.join('\n')
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

export function impactMarkdown(report: ImpactReport): string {
  const lines: string[] = []
  lines.push('## Architecture Impact', '')
  const summaryLine = coverageSummaryLine(report)
  if (summaryLine) lines.push(`**Coverage:** ${summaryLine}`, '')
  if (report.changed.length === 0 && report.candidates.length === 0 && report.unmapped.length === 0) {
    lines.push('No architecture-bound code changed.')
    return lines.join('\n')
  }

  lines.push('**Changed**', '')
  if (report.changed.length === 0) {
    lines.push('- _(none)_')
  } else {
    for (const element of report.changed) lines.push(elementLine(element))
  }

  if (report.candidates.length > 0) {
    lines.push('', '**Candidates** _(weak name matches)_', '')
    for (const element of report.candidates) lines.push(elementLine(element))
  }

  lines.push('', '**Related**', '')
  if (report.related.length === 0) {
    lines.push('- _(none)_')
  } else {
    for (const element of report.related) lines.push(`- ${element.name}`)
  }

  if (report.unmapped.length > 0) {
    lines.push('', '**Unmapped**', '')
    for (const file of report.unmapped) lines.push(`- \`${file}\``)
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

  lines.push('', '```mermaid', impactMermaid(report), '```')
  return lines.join('\n')
}
