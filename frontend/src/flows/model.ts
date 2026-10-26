import type { Connector, PlacedElement } from '../types'

export interface FlowConnectorHighlight { id: number; reverse: boolean }
export interface FlowStep {
  id: string
  title: string
  description: string
  elementIds: number[]
  connectors: FlowConnectorHighlight[]
  metadata: Record<string, string>
}
export interface StoryFlow { id: string; viewId: number; name: string; description: string; steps: FlowStep[]; revision: number }
export interface FlowDiagram { elements: Pick<PlacedElement, 'element_id' | 'name'>[]; connectors: Connector[] }
export interface FlowFrame { id: string; steps: FlowStep[] }
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
  return labels.join(', ') || 'Text-only step'
}

export function duplicateSteps(steps: FlowStep[]): FlowStep[] {
  return steps.map(step => ({ ...step, id: crypto.randomUUID(), elementIds: [...step.elementIds], connectors: step.connectors.map(c => ({ ...c })), metadata: { ...step.metadata } }))
}

export function playbackFrames(steps: FlowStep[]): FlowFrame[] {
  return steps.map(step => ({ id: step.id, steps: [step] }))
}

export function resolveFocus(flow: StoryFlow, frame: FlowFrame | undefined, diagram: FlowDiagram): FlowFocus {
  const elements = new Set<number>()
  const connectors = new Map<number, FlowConnectorHighlight>()
  for (const step of frame?.steps ?? flow.steps) {
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
