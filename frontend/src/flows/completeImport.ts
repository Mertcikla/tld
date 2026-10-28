import { create } from '@bufbuild/protobuf'
import { BatchConnectorSchema, BatchElementSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'
import type { FlowCatalog, FlowDiagram } from './model'
import { importFlowYaml, parseFlowYaml } from './yaml'

/** Validate every reference before creating anything; use fresh batch refs rather than exported IDs. */
export function prepareCompleteFlowImport(source: string, viewId: number, catalog?: FlowCatalog) {
  const doc = parseFlowYaml(source)
  if (!doc.resources) throw new Error('This flow does not include elements and connectors.')
  const { elements, connectors } = doc.resources
  const preview: FlowDiagram = {
    elements: elements.map((element, index) => ({ ...element, element_id: index + 1 })),
    connectors: connectors.map((connector, index) => ({
      ...connector, id: index + 1, view_id: viewId, created_at: '', updated_at: '',
      label: connector.label ?? null, description: connector.description ?? null, relationship: connector.relationship ?? null,
      direction: connector.direction ?? 'forward', style: connector.style ?? 'solid', url: connector.url ?? null,
      source_handle: connector.source_handle ?? null, target_handle: connector.target_handle ?? null,
      source_element_id: elements.findIndex(element => element.name === connector.source) + 1,
      target_element_id: elements.findIndex(element => element.name === connector.target) + 1,
    })),
  }
  importFlowYaml(source, viewId, preview, {}, catalog)
  const ref = (name: string) => `element-${elements.findIndex(element => element.name === name)}`
  const batch = {
    elements: [create(BatchElementSchema, { ref: 'flow-owner', name: doc.name, description: doc.description, hasView: true, placements: [{ parentRef: 'root' }] }),
      ...elements.map(element => create(BatchElementSchema, {
        ref: ref(element.name), name: element.name, kind: element.kind, description: element.description,
        technology: element.technology, url: element.url, logoUrl: element.logo_url, tags: element.tags,
        technologyLinks: element.technology_connectors?.map(link => ({ type: link.type, slug: link.slug, label: link.label, isPrimaryIcon: link.is_primary_icon })),
        placements: [{ parentRef: 'flow-owner', positionX: element.position_x, positionY: element.position_y }],
      })),
    ],
    connectors: connectors.map((connector, index) => create(BatchConnectorSchema, {
      ref: `connector-${index}`, viewRef: 'flow-owner', sourceElementRef: ref(connector.source), targetElementRef: ref(connector.target),
      label: connector.label, description: connector.description, relationship: connector.relationship,
      direction: connector.direction, style: connector.style, url: connector.url,
      sourceHandle: connector.source_handle, targetHandle: connector.target_handle, tags: connector.tags,
    })),
  }
  return {
    batch,
    // Unqualified linked flows retain their existing interpretation in the importing view.
    resolve: (newViewId: number, diagram: FlowDiagram) => ({ ...importFlowYaml(source, viewId, diagram, {}, catalog), viewId: newViewId }),
  }
}
