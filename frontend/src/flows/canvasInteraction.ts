import { newStep, type FlowDiagram, type FlowStep } from './model'

export type FlowCanvasTarget = { elementId: number; connectorId?: never } | { connectorId: number; elementId?: never }
export interface FlowCanvasRecorder {
  mode?: 'record' | 'pick'
  capture: (target: FlowCanvasTarget) => void
  stop: () => void
}

/** A canvas click records one step, only for objects belonging to this view. */
export function captureCanvasStep(target: FlowCanvasTarget, diagram: FlowDiagram): FlowStep | null {
  const step = newStep(diagram, target)
  return step.elementIds.length || step.connectors.length ? step : null
}

/** Picking adds to the existing step without duplicating or replacing its highlights. */
export function addCanvasHighlight(step: FlowStep, target: FlowCanvasTarget, diagram: FlowDiagram): FlowStep {
  if (target.elementId !== undefined && diagram.elements.some(element => element.element_id === target.elementId) && !step.elementIds.includes(target.elementId)) {
    return { ...step, elementIds: [...step.elementIds, target.elementId] }
  }
  if (target.connectorId !== undefined && diagram.connectors.some(connector => connector.id === target.connectorId) && !step.connectors.some(connector => connector.id === target.connectorId)) {
    return { ...step, connectors: [...step.connectors, { id: target.connectorId, reverse: false }] }
  }
  return step
}

export function canvasHighlightTarget(target: EventTarget | null): FlowCanvasTarget | null {
  if (typeof Element === 'undefined' || !(target instanceof Element)) return null
  const nodeId = target.closest('.react-flow__node')?.getAttribute('data-id')
  if (nodeId && /^\d+$/.test(nodeId)) return { elementId: Number(nodeId) }
  const edgeId = target.closest('[data-tag-drop-connector-id]')?.getAttribute('data-tag-drop-connector-id')
    ?? target.closest('.react-flow__edge')?.getAttribute('data-id')
  return edgeId && /^\d+$/.test(edgeId) ? { connectorId: Number(edgeId) } : null
}

export function isFlowPanelTarget(target: EventTarget | null): boolean {
  return typeof Element !== 'undefined' && target instanceof Element && !!target.closest('[data-testid="flow-panel"]')
}
