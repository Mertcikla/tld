import { C4, Flowchart, isC4Diagram, isFlowchartDiagram, parseC4, parseFlowchart } from 'mermaid-ast'
import type { PlanConnector, PlanElement } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'

export interface ParsedImport {
  elements: PlanElement[]
  connectors: PlanConnector[]
  warnings: string[]
}

function toPlanConnector(connector: Record<string, unknown>): PlanConnector {
	return connector as unknown as PlanConnector
}

export function parseMermaid(code: string): ParsedImport {
  const viewRef = 'root'
  const result: ParsedImport = {
    elements: [],
    connectors: [],
    warnings: [],
  }

  if (isFlowchartDiagram(code)) {
    const diagram = Flowchart.from(parseFlowchart(code))

    for (const node of diagram.nodes) {
      result.elements.push({
        ref: node.id,
        name: node.text?.text || node.id,
        kind: 'system',
        placements: [{ parentRef: viewRef }],
      } as PlanElement)
    }

    for (const link of diagram.links) {
      result.connectors.push(toPlanConnector({
        ref: `${link.source}:${link.target}`,
        viewRef,
        sourceElementRef: link.source,
        targetElementRef: link.target,
        label: link.text?.text || '',
      }))
    }
  } else if (isC4Diagram(code)) {
    const diagram = C4.from(parseC4(code))

    const collectElements = (elements: typeof diagram.elements) => {
      for (const el of elements) {
        result.elements.push({
          ref: el.alias,
          name: el.label || el.alias,
          kind: 'system',
          placements: [{ parentRef: viewRef }],
        } as PlanElement)
        if ('children' in el && el.children) {
          collectElements(el.children as typeof diagram.elements)
        }
      }
    }

    collectElements(diagram.elements)

    for (const rel of diagram.relationships) {
      result.connectors.push(toPlanConnector({
        ref: `${rel.from}:${rel.to}`,
        viewRef,
        sourceElementRef: rel.from,
        targetElementRef: rel.to,
        label: rel.label || '',
      }))
    }
  } else {
    result.warnings.push(`Unsupported diagram type`)
  }

  return result
}
