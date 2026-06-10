import { C4, Flowchart, isC4Diagram, isFlowchartDiagram, parseAsync, parseC4, parseFlowchart } from 'mermaid-ast'
import type { PlanConnector, PlanElement } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'
import { applyTldMetadata, parseTldMetadata, stripMermaidCommentLines } from '../mermaid/metadata'

export { MermaidExporter, serializeViewToMermaid, type MermaidExportOptions, type MermaidWorkspaceView } from '../exporter/mermaid'

export type MermaidDirection = 'TB' | 'TD' | 'BT' | 'RL' | 'LR'

export interface ParsedImport {
  elements: PlanElement[]
  connectors: PlanConnector[]
  warnings: string[]
  direction: MermaidDirection
  source: string
}

export function parsedMermaidImportError(parsed: ParsedImport) {
  if (parsed.warnings.length > 0) return parsed.warnings[0] ?? 'Failed to parse Mermaid diagram'
  if (parsed.elements.length === 0 && parsed.connectors.length === 0) return 'No compatible diagram content found.'
  return null
}

function toPlanConnector(connector: Record<string, unknown>): PlanConnector {
  return connector as unknown as PlanConnector
}

function createParsedImport(source: string): ParsedImport {
  return {
    elements: [],
    connectors: [],
    warnings: [],
    direction: 'LR',
    source,
  }
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {}
}

function asString(value: unknown, fallback = '') {
  return typeof value === 'string' && value.trim() ? value : fallback
}

function mapEntries(value: unknown): Array<[string, Record<string, unknown>]> {
  if (!(value instanceof Map)) return []
  return Array.from(value.entries()).map(([key, item]) => [String(key), asRecord(item)])
}

function sanitizeLabel(value: string) {
  return value.replace(/<br\s*\/?>/gi, ' ').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim()
}

function addElement(result: ParsedImport, ref: string, name = ref, kind = 'system', description?: string, technology?: string) {
  if (!ref || result.elements.some((element) => element.ref === ref)) return
  result.elements.push({
    ref,
    name: sanitizeLabel(name) || ref,
    kind,
    description: description !== undefined ? sanitizeLabel(description) : undefined,
    technology,
    placements: [{ parentRef: 'root' }],
  } as PlanElement)
}

function addConnector(
  result: ParsedImport,
  source: string,
  target: string,
  label = '',
  index = result.connectors.length,
  options: Partial<PlanConnector> = {},
) {
  if (!source || !target) return
  result.connectors.push(toPlanConnector({
    ref: `${source}:${target}:${index}`,
    viewRef: 'root',
    sourceElementRef: source,
    targetElementRef: target,
    label: sanitizeLabel(label),
    ...options,
  }))
}

function toMermaidDirection(value: unknown): MermaidDirection {
  if (value === 'TB' || value === 'TD' || value === 'BT' || value === 'RL' || value === 'LR') return value
  return 'LR'
}

function isSupportedMermaidStart(code: string) {
  const lines = code.trim().split(/\r?\n/)
  const start = firstMermaidBodyLineIndex(lines)
  if (start < 0) return false
  return /^(?:flowchart|graph|sequenceDiagram|classDiagram|erDiagram|stateDiagram(?:-v2)?|requirementDiagram|sankey-beta|pie|gitGraph|quadrantChart|mindmap|journey|gantt|timeline|xychart-beta|architecture-beta|C4Context|C4Container|C4Component|C4Dynamic|C4Deployment)\b/i.test(lines[start].trim())
}

function firstMermaidBodyLineIndex(lines: string[]) {
  let index = 0
  while (index < lines.length) {
    const line = lines[index].trim()
    if (!line) {
      index += 1
      continue
    }
    if (line === '---') {
      index += 1
      while (index < lines.length && lines[index].trim() !== '---') index += 1
      if (index < lines.length) index += 1
      continue
    }
    if (line.startsWith('%%')) {
      index += 1
      continue
    }
    return index
  }
  return -1
}

function isArchitectureBetaDiagram(source: string) {
  const lines = source.trim().split(/\r?\n/)
  const start = firstMermaidBodyLineIndex(lines)
  return start >= 0 && /^architecture-beta\b/i.test(lines[start].trim())
}

export function extractMermaidCode(text: string): string | null {
  const trimmed = text.trim()
  if (!trimmed) return null

  const fenced = trimmed.match(/```(?:mermaid|mmd)[ \t]*\r?\n([\s\S]*?)```/i)
  if (fenced?.[1]?.trim()) return fenced[1].trim()

  if (isSupportedMermaidStart(trimmed)) return trimmed
  return null
}

export function tryParseMermaid(text: string): ParsedImport | null {
  const code = extractMermaidCode(text)
  if (!code) return null
  const parsed = parseMermaid(code)
  return parsedMermaidImportError(parsed) ? null : parsed
}

export async function tryParseMermaidAsync(text: string): Promise<ParsedImport | null> {
  const code = extractMermaidCode(text)
  if (!code) return null
  const parsed = await parseMermaidAsync(code)
  return parsedMermaidImportError(parsed) ? null : parsed
}

export function parseMermaid(code: string): ParsedImport {
  const source = extractMermaidCode(code) ?? code.trim()
  const result = createParsedImport(source)
  const metadata = parseTldMetadata(source)
  const parseSource = stripMermaidCommentLines(source)

  try {
    if (isArchitectureBetaDiagram(parseSource)) {
      convertArchitectureBetaSource(result, parseSource)
    } else if (isFlowchartDiagram(parseSource)) {
      const ast = parseFlowchart(parseSource)
      convertFlowchartAst(result, ast)
    } else if (isC4Diagram(parseSource)) {
      convertC4Ast(result, parseC4(parseSource))
    } else {
      result.warnings.push('Unsupported diagram type')
    }
    applyTldMetadata(result, metadata)
  } catch (err) {
    result.warnings.push(err instanceof Error ? err.message : 'Failed to parse Mermaid diagram')
  }

  return result
}

export async function parseMermaidAsync(code: string): Promise<ParsedImport> {
  const source = extractMermaidCode(code) ?? code.trim()
  const result = createParsedImport(source)
  const metadata = parseTldMetadata(source)
  const parseSource = stripMermaidCommentLines(source)

  try {
    if (isArchitectureBetaDiagram(parseSource)) {
      convertArchitectureBetaSource(result, parseSource)
    } else {
      const ast = await parseAsync(parseSource)
      convertMermaidAst(result, asRecord(ast))
    }
    applyTldMetadata(result, metadata)
  } catch (err) {
    result.warnings.push(err instanceof Error ? err.message : 'Failed to parse Mermaid diagram')
  }

  return result
}

function architectureKind(icon: string, nodeType: 'group' | 'service' | 'junction') {
  if (nodeType === 'group') return 'container'
  if (nodeType === 'junction') return 'component'
  const value = icon.toLowerCase()
  if (value.includes('database') || value.includes('disk') || value.includes('storage')) return 'database'
  if (value.includes('internet') || value.includes('cloud')) return 'external'
  if (value.includes('server')) return 'service'
  return 'system'
}

function architectureHandle(side: string) {
  switch (side.toUpperCase()) {
    case 'T': return 'top'
    case 'B': return 'bottom'
    case 'L': return 'left'
    case 'R': return 'right'
    default: return undefined
  }
}

function architectureDirection(leftArrow: string, rightArrow: string) {
  if (leftArrow && rightArrow) return 'both'
  if (leftArrow) return 'backward'
  if (rightArrow) return 'forward'
  return 'none'
}

function architectureDescription(type: 'group' | 'service' | 'junction', icon = '', parent = '') {
  const parts = [`Architecture ${type}`]
  if (icon) parts.push(`icon: ${icon}`)
  if (parent) parts.push(`in: ${parent}`)
  return parts.join('\n')
}

function stripArchitectureGroupModifier(value: string) {
  return value.replace(/\{group\}$/i, '')
}

function convertArchitectureBetaSource(result: ParsedImport, source: string) {
  result.direction = 'LR'
  const lines = source.trim().split(/\r?\n/)
  const start = firstMermaidBodyLineIndex(lines)
  if (start < 0) {
    result.warnings.push('Unable to find architecture-beta diagram body')
    return
  }

  const bodyLines = lines.slice(start + 1)
  bodyLines.forEach((rawLine, index) => {
    const line = rawLine.trim().replace(/;$/, '')
    if (!line || line.startsWith('%%')) return

    const nodeMatch = line.match(/^(group|service)\s+([A-Za-z_][\w.-]*)(?:\(([^)]*)\))?\s*\[([^\]]*)\](?:\s+in\s+([A-Za-z_][\w.-]*))?$/i)
    if (nodeMatch) {
      const [, nodeType, ref, icon = '', label, parent = ''] = nodeMatch
      const type = nodeType.toLowerCase() as 'group' | 'service'
      addElement(result, ref, label.trim() || ref, architectureKind(icon, type), architectureDescription(type, icon, parent), icon)
      return
    }

    const junctionMatch = line.match(/^junction\s+([A-Za-z_][\w.-]*)(?:\s+in\s+([A-Za-z_][\w.-]*))?$/i)
    if (junctionMatch) {
      const [, ref, parent = ''] = junctionMatch
      addElement(result, ref, ref, architectureKind('', 'junction'), architectureDescription('junction', '', parent))
      return
    }

    const edgeMatch = line.match(/^([A-Za-z_][\w.-]*(?:\{group\})?):([TBLR])\s*(<)?--(>)?\s*([TBLR]):([A-Za-z_][\w.-]*(?:\{group\})?)(?:\s*:(.*))?$/i)
    if (edgeMatch) {
      const [, rawSource, sourceSide, leftArrow = '', rightArrow = '', targetSide, rawTarget, label = ''] = edgeMatch
      const sourceRef = stripArchitectureGroupModifier(rawSource)
      const targetRef = stripArchitectureGroupModifier(rawTarget)
      addElement(result, sourceRef, sourceRef)
      addElement(result, targetRef, targetRef)
      addConnector(result, sourceRef, targetRef, label.trim(), result.connectors.length, {
        direction: architectureDirection(leftArrow, rightArrow),
        sourceHandle: architectureHandle(sourceSide),
        targetHandle: architectureHandle(targetSide),
      })
      return
    }

    result.warnings.push(`Unsupported architecture-beta line ${index + 2}: ${line}`)
  })
}

function convertMermaidAst(result: ParsedImport, ast: Record<string, unknown>) {
  switch (ast.type) {
    case 'flowchart':
      convertFlowchartAst(result, ast)
      return
    case 'c4':
      convertC4Ast(result, ast)
      return
    case 'sequence':
      convertSequenceAst(result, ast)
      return
    case 'classDiagram':
      convertClassAst(result, ast)
      return
    case 'erDiagram':
      convertErAst(result, ast)
      return
    case 'state':
      convertStateAst(result, ast)
      return
    case 'requirement':
      convertRequirementAst(result, ast)
      return
    case 'sankey':
      convertSankeyAst(result, ast)
      return
    case 'pie':
      convertPieAst(result, ast)
      return
    case 'gitGraph':
      convertGitGraphAst(result, ast)
      return
    case 'quadrant':
      convertQuadrantAst(result, ast)
      return
    case 'mindmap':
      convertMindmapAst(result, ast)
      return
    case 'journey':
      convertJourneyAst(result, ast)
      return
    case 'gantt':
      convertGanttAst(result, ast)
      return
    case 'timeline':
      convertTimelineAst(result, ast)
      return
    case 'xychart':
      convertXYChartAst(result, ast)
      return
    default:
      result.warnings.push(`Unsupported diagram type${ast.type ? `: ${String(ast.type)}` : ''}`)
  }
}

function convertFlowchartAst(result: ParsedImport, ast: unknown) {
  const diagram = Flowchart.from(ast as unknown as ReturnType<typeof parseFlowchart>)
  result.direction = toMermaidDirection(asRecord(ast).direction)

  for (const node of diagram.nodes) {
    addElement(result, node.id, node.text?.text || node.id)
  }

  diagram.links.forEach((link, index) => {
    addConnector(result, link.source, link.target, link.text?.text || '', index)
  })
}

function c4Kind(type: unknown) {
  const value = String(type ?? '')
  if (value.includes('person')) return 'person'
  if (value.includes('container')) return 'container'
  if (value.includes('component')) return 'component'
  if (value.includes('external')) return 'external'
  return 'system'
}

function convertC4Ast(result: ParsedImport, ast: unknown) {
  const diagram = C4.from(ast as unknown as ReturnType<typeof parseC4>)

  const collectElements = (elements: typeof diagram.elements) => {
    for (const el of elements) {
      addElement(result, el.alias, el.label || el.alias, c4Kind(el.type), 'description' in el ? el.description : undefined)
      if ('children' in el && el.children) collectElements(el.children as typeof diagram.elements)
    }
  }

  collectElements(diagram.elements)

  diagram.relationships.forEach((rel, index) => {
    addConnector(result, rel.from, rel.to, rel.label || rel.technology || '', index)
  })
}

function convertSequenceAst(result: ParsedImport, ast: Record<string, unknown>) {
  mapEntries(ast.actors).forEach(([id, actor]) => {
    addElement(result, id, asString(actor.name, id), actor.type === 'actor' ? 'person' : 'system')
  })

  const visitStatements = (statements: unknown, path = 'statement') => {
    if (!Array.isArray(statements)) return
    statements.forEach((statement, index) => {
      const item = asRecord(statement)
      if (item.type === 'message') addConnector(result, asString(item.from), asString(item.to), asString(item.text), result.connectors.length)
      visitStatements(item.statements, `${path}-${index}`)
      visitStatements(item.children, `${path}-${index}`)
    })
  }

  visitStatements(ast.statements)
}

function convertClassAst(result: ParsedImport, ast: Record<string, unknown>) {
  result.direction = toMermaidDirection(ast.direction)
  mapEntries(ast.classes).forEach(([id, item]) => {
    const members = Array.isArray(item.members)
      ? item.members.map((member) => {
        const record = asRecord(member)
        return `${asString(record.visibility)}${asString(record.text)}`
      }).filter(Boolean)
      : []
    addElement(result, id, id, 'component', members.join('\n') || undefined)
  })

  const relations = Array.isArray(ast.relations) ? ast.relations : []
  relations.forEach((relation, index) => {
    const item = asRecord(relation)
    const relationMeta = asRecord(item.relation)
    const label = asString(item.title) || asString(item.label) ||
      [relationMeta.type1, relationMeta.lineType, relationMeta.type2].filter(Boolean).join(' ')
    addConnector(result, asString(item.id1), asString(item.id2), label, index)
  })
}

function convertErAst(result: ParsedImport, ast: Record<string, unknown>) {
  result.direction = toMermaidDirection(ast.direction)
  mapEntries(ast.entities).forEach(([id, item]) => {
    const attributes = Array.isArray(item.attributes)
      ? item.attributes.map((attribute) => Object.values(asRecord(attribute)).filter(Boolean).join(' ')).filter(Boolean)
      : []
    addElement(result, id, asString(item.alias, asString(item.name, id)), 'database', attributes.join('\n') || undefined)
  })

  const relationships = Array.isArray(ast.relationships) ? ast.relationships : []
  relationships.forEach((relationship, index) => {
    const item = asRecord(relationship)
    addConnector(result, asString(item.entityA), asString(item.entityB), asString(item.role), index)
  })
}

function convertStateAst(result: ParsedImport, ast: Record<string, unknown>) {
  result.direction = toMermaidDirection(ast.direction)
  mapEntries(ast.states).forEach(([id, item]) => {
    addElement(result, id, asString(item.description, id), 'system')
  })

  const transitions = Array.isArray(ast.transitions) ? ast.transitions : []
  transitions.forEach((transition, index) => {
    const item = asRecord(transition)
    addConnector(result, asString(asRecord(item.state1).id), asString(asRecord(item.state2).id), asString(item.description), index)
  })
}

function convertRequirementAst(result: ParsedImport, ast: Record<string, unknown>) {
  result.direction = toMermaidDirection(ast.direction)
  mapEntries(ast.requirements).forEach(([id, item]) => {
    addElement(result, id, asString(item.name, id), 'system', asString(item.text))
  })
  mapEntries(ast.elements).forEach(([id, item]) => {
    addElement(result, id, asString(item.name, id), 'component', asString(item.docRef))
  })

  const relationships = Array.isArray(ast.relationships) ? ast.relationships : []
  relationships.forEach((relationship, index) => {
    const item = asRecord(relationship)
    addConnector(result, asString(item.source), asString(item.target), asString(item.relationshipType), index)
  })
}

function convertSankeyAst(result: ParsedImport, ast: Record<string, unknown>) {
  mapEntries(ast.nodes).forEach(([id, item]) => addElement(result, id, asString(item.label, id)))

  const links = Array.isArray(ast.links) ? ast.links : []
  links.forEach((link, index) => {
    const item = asRecord(link)
    const value = Number(item.value)
    if (!Number.isFinite(value)) return
    addConnector(result, asString(item.source), asString(item.target), String(value), index)
  })
}

function convertPieAst(result: ParsedImport, ast: Record<string, unknown>) {
  const title = asString(ast.title, 'Pie chart')
  addElement(result, 'pie_chart', title, 'system')
  const sections = Array.isArray(ast.sections) ? ast.sections : []
  sections.forEach((section, index) => {
    const item = asRecord(section)
    const ref = `pie_${index + 1}`
    addElement(result, ref, asString(item.label, ref), 'system', String(item.value ?? ''))
    addConnector(result, 'pie_chart', ref, String(item.value ?? ''), index)
  })
}

function convertGitGraphAst(result: ParsedImport, ast: Record<string, unknown>) {
  const statements = Array.isArray(ast.statements) ? ast.statements.map(asRecord) : []
  const branchHeads = new Map<string, string>()
  let currentBranch = 'main'
  let previousCommit: string | null = null
  addElement(result, currentBranch, currentBranch, 'system')

  statements.forEach((statement, index) => {
    if (statement.type === 'branch') {
      const branch = asString(statement.name, `branch_${index}`)
      addElement(result, branch, branch, 'system')
      if (previousCommit) addConnector(result, previousCommit, branch, 'branch', result.connectors.length)
      branchHeads.set(branch, previousCommit ?? branch)
    } else if (statement.type === 'checkout') {
      currentBranch = asString(statement.branch, currentBranch)
      addElement(result, currentBranch, currentBranch, 'system')
      previousCommit = branchHeads.get(currentBranch) ?? null
    } else if (statement.type === 'commit') {
      const ref = asString(statement.id, `commit_${index + 1}`)
      addElement(result, ref, ref, 'component')
      addConnector(result, previousCommit ?? currentBranch, ref, currentBranch, result.connectors.length)
      previousCommit = ref
      branchHeads.set(currentBranch, ref)
    } else if (statement.type === 'merge') {
      const branch = asString(statement.branch)
      const branchHead = branchHeads.get(branch) ?? branch
      const ref = asString(statement.id, `merge_${branch || index}_${index}`)
      addElement(result, ref, asString(statement.tag, ref), 'component')
      if (previousCommit) addConnector(result, previousCommit, ref, currentBranch, result.connectors.length)
      addConnector(result, branchHead, ref, 'merge', result.connectors.length)
      previousCommit = ref
      branchHeads.set(currentBranch, ref)
    }
  })
}

function convertQuadrantAst(result: ParsedImport, ast: Record<string, unknown>) {
  const title = asString(ast.title, 'Quadrant chart')
  addElement(result, 'quadrant_chart', title, 'system')
  const points = Array.isArray(ast.points) ? ast.points : []
  points.forEach((point, index) => {
    const item = asRecord(point)
    const ref = `quadrant_${index + 1}`
    const name = asString(item.name, ref)
    addElement(result, ref, name, 'system', `[${item.x ?? ''}, ${item.y ?? ''}]`)
    addConnector(result, 'quadrant_chart', ref, '', index)
  })
}

function convertMindmapAst(result: ParsedImport, ast: Record<string, unknown>) {
  const visit = (node: Record<string, unknown>, parentRef?: string) => {
    const ref = sanitizeMermaidId(asString(node.id, asString(node.description, `mindmap_${result.elements.length + 1}`)))
    addElement(result, ref, asString(node.description, ref))
    if (parentRef) addConnector(result, parentRef, ref, '', result.connectors.length)
    if (Array.isArray(node.children)) node.children.forEach((child) => visit(asRecord(child), ref))
  }
  visit(asRecord(ast.root))
}

function convertJourneyAst(result: ParsedImport, ast: Record<string, unknown>) {
  const title = asString(ast.title, 'Journey')
  addElement(result, 'journey', title, 'system')
  const sections = Array.isArray(ast.sections) ? ast.sections : []
  sections.forEach((section, sectionIndex) => {
    const sectionItem = asRecord(section)
    const sectionRef = `journey_section_${sectionIndex + 1}`
    addElement(result, sectionRef, asString(sectionItem.name, sectionRef), 'system')
    addConnector(result, 'journey', sectionRef, '', result.connectors.length)
    if (Array.isArray(sectionItem.tasks)) {
      sectionItem.tasks.forEach((task, taskIndex) => {
        const taskItem = asRecord(task)
        const taskRef = `${sectionRef}_task_${taskIndex + 1}`
        addElement(result, taskRef, asString(taskItem.name, taskRef), 'component', `Score: ${taskItem.score ?? ''}`)
        addConnector(result, sectionRef, taskRef, '', result.connectors.length)
      })
    }
  })
}

function convertGanttAst(result: ParsedImport, ast: Record<string, unknown>) {
  const title = asString(ast.title, 'Gantt')
  addElement(result, 'gantt', title, 'system')
  const sections = Array.isArray(ast.sections) ? ast.sections : []
  sections.forEach((section, sectionIndex) => {
    const sectionItem = asRecord(section)
    const sectionRef = `gantt_section_${sectionIndex + 1}`
    addElement(result, sectionRef, asString(sectionItem.name, sectionRef), 'system')
    addConnector(result, 'gantt', sectionRef, '', result.connectors.length)
    if (Array.isArray(sectionItem.tasks)) {
      sectionItem.tasks.forEach((task, taskIndex) => {
        const taskItem = asRecord(task)
        const taskRef = asString(taskItem.id, `${sectionRef}_task_${taskIndex + 1}`)
        addElement(result, taskRef, asString(taskItem.name, taskRef), 'component', `${taskItem.start ?? ''} ${taskItem.end ?? ''}`.trim())
        addConnector(result, sectionRef, taskRef, '', result.connectors.length)
      })
    }
  })
}

function convertTimelineAst(result: ParsedImport, ast: Record<string, unknown>) {
  const title = asString(ast.title, 'Timeline')
  addElement(result, 'timeline', title, 'system')
  const sections = Array.isArray(ast.sections) ? ast.sections : []
  sections.forEach((section, sectionIndex) => {
    const sectionItem = asRecord(section)
    const sectionRef = `timeline_section_${sectionIndex + 1}`
    addElement(result, sectionRef, asString(sectionItem.name, sectionRef), 'system')
    addConnector(result, 'timeline', sectionRef, '', result.connectors.length)
    if (Array.isArray(sectionItem.periods)) {
      sectionItem.periods.forEach((period, periodIndex) => {
        const periodItem = asRecord(period)
        const periodRef = `${sectionRef}_period_${periodIndex + 1}`
        addElement(result, periodRef, asString(periodItem.name, periodRef), 'component')
        addConnector(result, sectionRef, periodRef, '', result.connectors.length)
        if (Array.isArray(periodItem.events)) {
          periodItem.events.forEach((event, eventIndex) => {
            const eventItem = asRecord(event)
            const eventRef = `${periodRef}_event_${eventIndex + 1}`
            addElement(result, eventRef, asString(eventItem.text, eventRef), 'component')
            addConnector(result, periodRef, eventRef, '', result.connectors.length)
          })
        }
      })
    }
  })
}

function convertXYChartAst(result: ParsedImport, ast: Record<string, unknown>) {
  const title = asString(ast.title, 'XY chart')
  addElement(result, 'xychart', title, 'system')
  const xAxis = asRecord(ast.xAxis)
  const categories = Array.isArray(xAxis.categories) ? xAxis.categories.map(String) : []
  const series = Array.isArray(ast.series) ? ast.series : []
  series.forEach((serie, seriesIndex) => {
    const serieItem = asRecord(serie)
    const serieRef = `xy_series_${seriesIndex + 1}`
    addElement(result, serieRef, asString(serieItem.label, `${serieItem.type ?? 'series'} ${seriesIndex + 1}`), 'system')
    addConnector(result, 'xychart', serieRef, '', result.connectors.length)
    if (Array.isArray(serieItem.values)) {
      serieItem.values.forEach((value, valueIndex) => {
        const pointRef = `${serieRef}_point_${valueIndex + 1}`
        addElement(result, pointRef, categories[valueIndex] ?? pointRef, 'component', String(value))
        addConnector(result, serieRef, pointRef, String(value), result.connectors.length)
      })
    }
  })
}

function sanitizeMermaidId(value: string) {
  const sanitized = value.replace(/[^A-Za-z0-9_]/g, '_')
  return /^[A-Za-z_]/.test(sanitized) ? sanitized : `node_${sanitized}`
}
