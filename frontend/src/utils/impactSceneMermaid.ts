import defaultTheme from '../../../internal/mermaid/theme.json'
import { selectMermaidScene } from './impactSceneMermaidSelection'
import { ChangeKind, ImpactRole, type ImpactScene, type ScenePlacement, type ImpactSceneOverlay } from '@buf/tldiagramcom_diagram.bufbuild_es/codeindex/v1/codeindex_pb.js'
import type { View } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'

const numberRef = (id: number) => id < 0 ? `neg${-id}` : `${id}`
const nodeRef = (view: number, element: number) => `el_${numberRef(view)}_${numberRef(element)}`
const viewRef = (view: number) => `view_${numberRef(view)}`
const escapeLabel = (text: string) => text.replace(/\r\n|\n|\r|&|\\|"/g, (char) => ({ '&': '&amp;', '\\': '\\\\', '"': '&quot;' })[char] ?? ' ')
const escapeMetadata = (text: string) => text.replace(/[\\\n\r\t=,|:]/g, (char) => ({ '\n': '\\n', '\r': '\\r', '\t': '\\t' })[char] ?? `\\${char}`)
const changeWord = (change: ChangeKind) => change === ChangeKind.ADDED ? 'added' : change === ChangeKind.REMOVED ? 'removed' : change === ChangeKind.MODIFIED ? 'modified' : 'unchanged'

/** Export-only presentation; the canvas retains the complete supplied scene. */
export function impactSceneMermaid(input: ImpactScene, options: { plain?: boolean; noTheme?: boolean } = {}): string {
  const plain = options.plain ?? input.comparison?.plain ?? false
  const scene = selectMermaidScene(input, input.comparison?.effectiveRadius ?? 0, plain)
  const theme = options.noTheme ? undefined : defaultTheme
  type State = keyof typeof defaultTheme.nodes
  const classes = new Map<State, string[]>()
  const style = (properties: Record<string, string>) => Object.keys(properties).sort().filter(key => properties[key]).map(key => `${key}:${properties[key]}`).join(',')
  const nodeState = (id: number): State => { const change = changeWord(overlayOf.get(id)?.change ?? ChangeKind.UNSPECIFIED); return change === 'unchanged' ? 'context' : change }
  const styleNode = (id: string, state: State) => { if (theme) classes.set(state, [...(classes.get(state) ?? []), id]) }

  const overlayOf = new Map<number, ImpactSceneOverlay>()
  const connected = new Set<string>()
  const retained = new Set<number>()
  const names = new Map<number, string>()
  const walk = (views: View[]) => {
    for (const view of views) {
      if (retained.has(view.id)) continue
      retained.add(view.id)
      names.set(view.id, view.name)
      walk(view.children)
    }
  }
  walk(scene.tree)
  for (const [key, content] of Object.entries(scene.views)) {
    const viewId = Number(key)
    retained.add(viewId)
    const local = new Set(content.placements.map((p) => p.element!.elementId))
    for (const placement of content.placements) {
      if (placement.overlay) overlayOf.set(placement.element!.elementId, placement.overlay)
    }
    for (const connector of content.connectors) {
      connected.add(nodeRef(viewId, connector.sourceElementId))
      const targetView = local.has(connector.targetElementId) ? viewId : content.connectorTargets[String(connector.id)]?.viewId ?? viewId
      connected.add(nodeRef(targetView, connector.targetElementId))
    }
  }
  const lines = theme ? [
    '---', 'config:', `  theme: '${theme.baseTheme}'`, '  look: classic', '  htmlLabels: true',
    "  themeCSS: '.edgeLabel .labelBkg { background: transparent !important; }'", '  themeVariables:',
    ...Object.entries(theme.themeVariables).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => `    ${key}: ${JSON.stringify(value)}`),
    '---', 'flowchart LR',
  ] : ['flowchart LR']
  const metadata = ['%% tld-scene']
  for (const [key, value] of [['repo', scene.comparison?.repositoryId], ['key', scene.comparison?.key]]) {
    if (value?.trim()) metadata.push(`${key}=${escapeMetadata(value.trim())}`)
  }
  lines.push(metadata.join(' '))
  const summary = scene.summary
  if (summary) {
    lines.push(`%% impact files=${summary.files} added=${summary.knownLinesAdded} removed=${summary.knownLinesRemoved} complete=${summary.lineCountsComplete} basis=${summary.lineCountsBasis} binary=${summary.binaryFiles} affected=${summary.affectedPlacements} neighbours=${summary.dependencyNeighbours} authored=${summary.authoredContext} containers=${summary.containers} radius=${scene.comparison?.effectiveRadius ?? 0}`)
  }
  const seen = new Set<number>()
  const linkStyles: string[] = []
  let linkIndex = 0
  const nodeLabel = (placement: ScenePlacement, separator: string) => {
    const element = placement.element!
    let label = element.name.trim() || `element ${element.elementId}`
    const overlay = overlayOf.get(element.elementId)
    if (overlay) {
      const change = changeWord(overlay.change)
      if (change === 'unchanged') { if (!theme || theme.labels.context) label += separator + '(context)' }
      else {
        const parts: string[] = []
        if (!theme || theme.labels.change) parts.push(change)
        if ((!theme || theme.labels.churn) && overlay.linesAdded != null && overlay.linesRemoved != null) {
          const churnLabel = (sign: string, count: number, color?: string) => color ? `<span style='color:${color}!important'>${sign}${count}</span>` : `${sign}${count}`
          if (overlay.linesAdded > 0) parts.push(churnLabel('+', overlay.linesAdded, theme?.labels.addedColor))
          if (overlay.linesRemoved > 0) parts.push(churnLabel('−', overlay.linesRemoved, theme?.labels.removedColor))
        }
        if (parts.length) label += separator + parts.join(' ')
      }
    }
    return escapeLabel(label)
  }
  const connectorLine = (sourceView: number, source: number, targetView: number, target: number, label: string, tags: string[], direction: string) => {
    let from = nodeRef(sourceView, source), to = nodeRef(targetView, target)
    direction = direction.trim().toLowerCase()
    if (direction === 'backward') [from, to] = [to, from]
    const both = direction === 'both' || direction === 'bidirectional', none = direction === 'none'
    const change = tags.find((tag) => ['change:added', 'change:removed', 'change:modified'].includes(tag))
    if (theme) {
      const state = (change?.slice(7) ?? 'context') as State
      const properties = style(theme.edges[state])
      if (properties) linkStyles.push(`linkStyle ${linkIndex} ${properties}`)
    }
    let text = label.trim()
    if (!theme && change) text = `${text} (${change.slice(7)})`.trim()
    text = escapeLabel(text)
    let arrow = both ? '<-->' : none ? '---' : '-->'
    if (change === 'change:added') {
      arrow = both ? '<==>' : none ? '===' : '==>'
    } else if (change === 'change:removed') {
      arrow = both ? 'x--x' : none ? '---' : '--x'
      if (!both && !none) return text ? `${from}--x|"${text}"|${to}` : `${from}--x${to}`
    }
    if (text) return arrow === '-->' ? `${from} -- "${text}" --> ${to}` : `${from} ${arrow}|"${text}"| ${to}`
    return `${from} ${arrow} ${to}`
  }
  const renderNested = (viewId: number, depth: number, kids: View[]) => {
    const content = scene.views[String(viewId)]
    if (!content) {
      for (const kid of kids) renderTree(kid, depth)
      return
    }
    const head = '  '.repeat(depth), pad = '  '.repeat(depth + 1)
    if (!plain) lines.push('', `${head}subgraph ${viewRef(viewId)}["${escapeLabel(names.get(viewId) || `view ${viewId}`)}"]`)
    const rendered = new Set<number>()
    const isolated: ScenePlacement[] = []
    const placements = content.placements.filter((placement) => {
      if (!placement.element) return false
      const id = placement.element.elementId
      const redundantOwner = placement.role === ImpactRole.CONTAINER && !overlayOf.has(id) && !connected.has(nodeRef(viewId, id)) && kids.some((kid) => !seen.has(kid.id) && kid.ownerElementId === id && scene.views[String(kid.id)])
      return !redundantOwner
    })
    const isolatedCount = placements.filter((placement) => !connected.has(nodeRef(viewId, placement.element!.elementId))).length
    for (const placement of placements) {
      const id = placement.element!.elementId
      if (isolatedCount >= 3 && !connected.has(nodeRef(viewId, id))) {
        isolated.push(placement)
        continue
      }
      rendered.add(id)
      lines.push(`${pad}${nodeRef(viewId, id)}("${nodeLabel(placement, '<br/>')}")`)
      styleNode(nodeRef(viewId, id), nodeState(id))
    }
    if (isolated.length >= 3) {
      const states: (State | '')[] = theme ? ['context', 'modified', 'added', 'removed'] : ['']
      for (const state of states) {
        const group = isolated.filter(p => !state || nodeState(p.element!.elementId) === state)
        if (!group.length) continue
        const id = `${viewRef(viewId)}_isolated${state ? '_' + state : ''}`
        lines.push(`${pad}${id}("${group.map(p => nodeLabel(p, ' ')).join('<br/>')}")`)
        if (state) styleNode(id, state)
      }
    }
    for (const kid of kids) renderTree(kid, depth + 1)
    for (const connector of content.connectors) {
      if (!rendered.has(connector.sourceElementId)) continue
      let targetView = viewId
      if (!rendered.has(connector.targetElementId)) {
        const ref = content.connectorTargets[String(connector.id)]
        if (!ref) continue
        targetView = ref.viewId
      }
      lines.push(pad + connectorLine(viewId, connector.sourceElementId, targetView, connector.targetElementId, connector.label ?? '', connector.tags, connector.direction))
      linkIndex++
    }
    if (!plain) lines.push(head + 'end')
  }
  const renderTree = (view: View, depth: number) => {
    if (seen.has(view.id)) return
    seen.add(view.id)
    const kids = view.children.filter((kid) => !seen.has(kid.id) && retained.has(kid.id))
    renderNested(view.id, depth, kids)
  }
  for (const root of scene.tree) renderTree(root, 0)
  for (const id of [...retained].sort((a, b) => a - b)) {
    if (seen.has(id)) continue
    seen.add(id)
    renderNested(id, 0, [])
  }
  if (linkStyles.length) lines.push('', ...linkStyles)
  for (const state of ['context', 'modified', 'added', 'removed'] as State[]) {
    const ids = classes.get(state)
    if (theme && ids?.length) {
      const properties = style(theme.nodes[state])
      if (properties) lines.push(`classDef tld_${state} ${properties}`, `class ${ids.join(',')} tld_${state}`)
    }
  }
  return lines.join('\n') + '\n'
}
