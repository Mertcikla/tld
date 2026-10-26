import type { Connector, PlacedElement } from '../types'

export interface FlowConnectorHighlight { id: number; reverse: boolean }
export interface FlowStep {
  id: string
  title: string
  description: string
  elementIds: number[]
  connectors: FlowConnectorHighlight[]
  metadata: Record<string, string>
  overview?: boolean
  branches?: FlowBranches
  goToFlowId?: string
}
export interface FlowBranch { id: string; name: string; steps: FlowStep[] }
export interface FlowBranches { mode: string; paths: FlowBranch[] }
export interface FlowCatalog { flows: StoryFlow[]; views: { id: number; name: string }[] }
export interface StoryFlow { id: string; viewId: number; name: string; description: string; steps: FlowStep[]; revision: number }
export interface FlowDiagram { elements: Pick<PlacedElement, 'element_id' | 'name'>[]; connectors: Connector[] }
export interface FlowFrame { id: string; steps: FlowStep[]; path?: FlowBranch; choices?: Record<string, string> }
export interface FlowFocus {
  viewId: number
  elementIds: number[]
  connectors: FlowConnectorHighlight[]
}

export function newStep(diagram: FlowDiagram, selected?: { elementId?: number; connectorId?: number }): FlowStep {
  return {
    id: crypto.randomUUID(), title: '', description: '', metadata: {},
    elementIds: diagram.elements.filter(e => e.element_id === selected?.elementId).map(e => e.element_id),
    connectors: diagram.connectors.filter(c => c.id === selected?.connectorId).map(c => ({ id: c.id, reverse: false })),
  }
}

/** Derive display text without persisting a generated title over the author's choice. */
export function stepLabel(step: FlowStep, diagram: FlowDiagram): string {
  if (step.title.trim()) return step.title
  const elementName = (id: number) => diagram.elements.find(element => element.element_id === id)?.name.trim() || `Element #${id}`
  const labels = [
    ...step.elementIds.map(elementName),
    ...step.connectors.map(highlight => {
      const connector = diagram.connectors.find(connector => connector.id === highlight.id)
      if (!connector) return `Connection #${highlight.id}`
      if (connector.label?.trim()) return connector.label.trim()
      const source = elementName(connector.source_element_id)
      const target = elementName(connector.target_element_id)
      return highlight.reverse ? `${target} → ${source}` : `${source} → ${target}`
    }),
  ]
  return labels.join(', ') || (step.overview ? 'Flow overview' : step.branches ? step.branches.mode === 'parallel' ? 'Parallel paths' : 'Alternate paths' : step.goToFlowId ? 'Go to flow' : 'Text-only step')
}

export function allSteps(steps: FlowStep[]): FlowStep[] {
  return steps.flatMap(step => [step, ...(step.branches?.paths ?? []).flatMap(path => allSteps(path.steps))])
}

export function updateStep(steps: FlowStep[], id: string, update: (step: FlowStep) => FlowStep): FlowStep[] {
  return steps.map(step => {
    if (step.id === id) return update(step)
    if (!step.branches) return step
    const paths = step.branches.paths.map(path => {
      const children = updateStep(path.steps, id, update)
      return children.every((child, index) => child === path.steps[index]) ? path : { ...path, steps: children }
    })
    return paths.every((path, index) => path === step.branches!.paths[index]) ? step : { ...step, branches: { ...step.branches, paths } }
  })
}

export function duplicateSteps(steps: FlowStep[]): FlowStep[] {
  return steps.map(step => ({ ...step, id: crypto.randomUUID(), elementIds: [...step.elementIds], connectors: step.connectors.map(c => ({ ...c })), metadata: { ...step.metadata },
    ...(step.branches ? { branches: { ...step.branches, paths: step.branches.paths.map(path => ({ ...path, id: crypto.randomUUID(), steps: duplicateSteps(path.steps) })) } } : {}),
  }))
}

export function playbackFrames(steps: FlowStep[], choices: Record<string, string> = {}): FlowFrame[] {
  return steps.flatMap(step => {
    const path = step.branches?.paths.find(path => path.id === choices[step.id]) ?? step.branches?.paths[0]
    return [{ id: step.id, steps: [step], ...(path ? { path, choices } : {}) }, ...(path ? playbackFrames(path.steps, choices) : [])]
  })
}

/** Selecting a nested step also selects each of its ancestor paths. */
export function choicesForStep(steps: FlowStep[], id: string): Record<string, string> | undefined {
  for (const step of steps) {
    if (step.id === id) return {}
    for (const path of step.branches?.paths ?? []) {
      const found = choicesForStep(path.steps, id)
      if (found) return { [step.id]: path.id, ...found }
    }
  }
}

function initialHighlights(step: FlowStep, choices: Record<string, string> = {}): FlowStep[] {
  const paths = step.branches?.paths ?? []
  const preview = step.branches?.mode === 'alternate'
    ? [paths.find(path => path.id === choices[step.id]) ?? paths[0]].filter((path): path is FlowBranch => !!path)
    : paths
  return [step, ...preview.flatMap(path => path.steps[0] ? initialHighlights(path.steps[0], choices) : [])]
}

export function resolveFocus(flow: StoryFlow, frame: FlowFrame | undefined, diagram: FlowDiagram): FlowFocus {
  const elements = new Set<number>()
  const connectors = new Map<number, FlowConnectorHighlight>()
  const steps = !frame || frame.steps.some(step => step.overview)
    ? allSteps(flow.steps)
    : frame.steps.flatMap(step => step.branches?.mode === 'parallel' ? initialHighlights(step, frame.choices) : step.branches?.mode === 'alternate' ? [step, ...allSteps(frame.path?.steps ?? step.branches.paths[0]?.steps ?? [])] : [step])
  for (const step of steps) {
    for (const id of step.elementIds) if (diagram.elements.some(e => e.element_id === id)) elements.add(id)
    for (const highlight of step.connectors) {
      const connector = diagram.connectors.find(c => c.id === highlight.id)
      if (connector && [connector.source_element_id, connector.target_element_id].every(id => diagram.elements.some(e => e.element_id === id))) {
        elements.add(connector.source_element_id)
        elements.add(connector.target_element_id)
        connectors.set(connector.id, highlight)
      }
    }
  }
  return { viewId: flow.viewId, elementIds: [...elements], connectors: [...connectors.values()] }
}

export function missingReference(step: FlowStep, diagram: FlowDiagram): boolean {
  return step.elementIds.some(id => !diagram.elements.some(e => e.element_id === id)) || step.connectors.some(highlight => {
    const connector = diagram.connectors.find(c => c.id === highlight.id)
    return !connector || ![connector.source_element_id, connector.target_element_id].every(id => diagram.elements.some(e => e.element_id === id))
  })
}
