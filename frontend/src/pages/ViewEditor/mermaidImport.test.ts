import { describe, expect, it, vi } from 'vitest'
import type { ParsedImport } from '../../pkg/importer/mermaid'
import type { Connector, LibraryElement as WorkspaceElement, PlacedElement } from '../../types'
import { importMermaidIntoView, mermaidLocalImportDescription, type MermaidImportClient } from './mermaidImport'

function workspaceElement(id: number, name: string): WorkspaceElement {
  return {
    id,
    name,
    description: '',
    kind: 'service',
    technology: '',
    url: '',
    logo_url: null,
    technology_connectors: [],
    tags: [],
    repo: null,
    branch: null,
    file_path: null,
    language: null,
    bypass_noise_gate: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    has_view: false,
    view_label: null,
  }
}

function placement(elementId: number): PlacedElement {
  return {
    id: elementId + 1000,
    view_id: 10,
    element_id: elementId,
    position_x: 100,
    position_y: 100,
    name: `Element ${elementId}`,
    description: '',
    kind: 'service',
    technology: '',
    url: '',
    logo_url: null,
    technology_connectors: [],
    tags: [],
    repo: null,
    branch: null,
    file_path: null,
    language: null,
    bypass_noise_gate: false,
    has_view: false,
    view_label: null,
  }
}

function connector(id: number, sourceElementId: number, targetElementId: number, label: string): Connector {
  return {
    id,
    view_id: 10,
    source_element_id: sourceElementId,
    target_element_id: targetElementId,
    label,
    description: '',
    relationship: '',
    direction: 'forward',
    style: 'bezier',
    url: '',
    source_handle: 'right',
    target_handle: 'left',
    tags: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  }
}

describe('Mermaid view import', () => {
  it('resolves exported node refs and matching connectors before creating missing resources', async () => {
    const existing = workspaceElement(57, 'Auth')
    const created = workspaceElement(99, 'API')
    const createdConnector = connector(200, 99, 57, 'calls')

    const createElement = vi.fn(async () => created)
    const createView = vi.fn()
    const addPlacement = vi.fn()
    const createConnector = vi.fn(async () => createdConnector)
    const onConnectorCreated = vi.fn()
    const client: MermaidImportClient = {
      createElement,
      createView,
      addPlacement,
      createConnector,
    }

    const parsed = {
      direction: 'LR',
      warnings: [],
      source: 'flowchart LR',
      elements: [
        { ref: 'node_57', name: 'Auth', kind: 'service', placements: [{ parentRef: 'root', positionX: 100, positionY: 100 }] },
        { ref: 'node_8', name: 'API', kind: 'service', placements: [{ parentRef: 'root', positionX: 300, positionY: 100 }] },
      ],
      connectors: [
        { ref: 'existing', viewRef: 'root', sourceElementRef: 'node_57', targetElementRef: 'node_8', label: 'uses' },
        { ref: 'new', viewRef: 'root', sourceElementRef: 'node_8', targetElementRef: 'node_57', label: 'calls' },
      ],
    } as unknown as ParsedImport

    const summary = await importMermaidIntoView({
      parsed,
      currentViewId: 10,
      center: { x: 0, y: 0 },
      allElements: [existing],
      viewElements: [placement(57)],
      connectors: [connector(100, 57, 99, 'uses')],
      onConnectorCreated,
      client,
    })

    expect(createElement).toHaveBeenCalledTimes(1)
    expect(createElement).toHaveBeenCalledWith(expect.objectContaining({ name: 'API' }))
    expect(addPlacement).toHaveBeenCalledTimes(1)
    expect(addPlacement).toHaveBeenCalledWith(10, 99, expect.any(Number), expect.any(Number))
    expect(createConnector).toHaveBeenCalledTimes(1)
    expect(createConnector).toHaveBeenCalledWith(10, expect.objectContaining({
      source_element_id: 99,
      target_element_id: 57,
      label: 'calls',
    }))
    expect(onConnectorCreated).toHaveBeenCalledWith(createdConnector)
    expect(summary).toMatchObject({
      resolvedElementCount: 1,
      createdElementCount: 1,
      resolvedConnectorCount: 1,
      createdConnectorCount: 1,
    })
    expect(Array.from(summary.importedElementIds).sort((a, b) => a - b)).toEqual([57, 99])
    expect(mermaidLocalImportDescription(summary)).toBe('Resolved 1 element and 1 connector. Created 1 element and 1 connector.')
  })
})
