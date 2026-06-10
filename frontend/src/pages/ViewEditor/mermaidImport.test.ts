import { describe, expect, it, vi } from 'vitest'
import type { ParsedImport } from '../../pkg/importer/mermaid'
import type { Connector, LibraryElement as WorkspaceElement, PlacedElement, View } from '../../types'
import {
  importMermaidIntoView,
  layoutMermaidImport,
  mermaidImportReviewWarnings,
  mermaidLocalImportDescription,
  type MermaidImportClient,
} from './mermaidImport'

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

function view(id: number): View {
  return {
    id,
    owner_element_id: null,
    name: `View ${id}`,
    label: null,
    tags: [],
    is_root: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  }
}

describe('Mermaid view import', () => {
  it('warns when node ids resolve to existing elements with different names', () => {
    const parsed = {
      direction: 'LR',
      warnings: [],
      source: 'flowchart LR',
      elements: [
        { ref: 'node_57', name: 'Imported Auth', kind: 'service', placements: [{ parentRef: 'root' }] },
        { ref: 'api', name: 'API', kind: 'service', placements: [{ parentRef: 'root' }] },
      ],
      connectors: [],
    } as unknown as ParsedImport

    const warnings = mermaidImportReviewWarnings(parsed, [workspaceElement(57, 'Existing Auth')])

    expect(warnings).toHaveLength(1)
    expect(warnings[0]).toContain('node_57')
    expect(warnings[0]).toContain('Imported Auth')
    expect(warnings[0]).toContain('Existing Auth')
    expect(mermaidImportReviewWarnings(parsed, [workspaceElement(57, 'Imported Auth')])).toHaveLength(0)
  })

  it('lays out nodes without metadata away from positioned metadata nodes', () => {
    const parsed = {
      direction: 'LR',
      warnings: [],
      source: 'flowchart LR',
      elements: [
        { ref: 'node_1', name: 'Placed', kind: 'service', placements: [{ parentRef: 'root', positionX: 0, positionY: 0 }] },
        { ref: 'node_2', name: 'Missing A', kind: 'service', placements: [{ parentRef: 'root' }] },
        { ref: 'node_3', name: 'Missing B', kind: 'service', placements: [{ parentRef: 'root' }] },
      ],
      connectors: [
        { ref: 'a', viewRef: 'root', sourceElementRef: 'node_2', targetElementRef: 'node_3', label: '' },
      ],
    } as unknown as ParsedImport

    const positions = layoutMermaidImport(parsed, { x: 1000, y: 1000 })

    expect(positions.get('node_1')).toEqual({ x: 1000, y: 1000 })
    expect(positions.get('node_2')).not.toEqual({ x: 1000, y: 1000 })
    expect(positions.get('node_3')).not.toEqual({ x: 1000, y: 1000 })
    expect(positions.get('node_2')).not.toEqual(positions.get('node_3'))
  })

  it('resolves exported node refs and matching connectors before creating missing resources', async () => {
    const existing = workspaceElement(57, 'Auth')
    const created = workspaceElement(99, 'API')
    const createdConnector = connector(200, 99, 57, 'calls')

    const getElement = vi.fn(async (id: number) => {
      if (id === existing.id) return existing
      throw new Error('not found')
    })
    const createElement = vi.fn(async () => created)
    const deleteElement = vi.fn()
    const createView = vi.fn()
    const deleteView = vi.fn()
    const addPlacement = vi.fn()
    const removePlacement = vi.fn()
    const createConnector = vi.fn(async () => createdConnector)
    const deleteConnector = vi.fn()
    const onConnectorCreated = vi.fn()
    const client: MermaidImportClient = {
      getElement,
      createElement,
      deleteElement,
      createView,
      deleteView,
      addPlacement,
      removePlacement,
      createConnector,
      deleteConnector,
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
      allElements: [],
      viewElements: [placement(57)],
      connectors: [connector(100, 57, 99, 'uses')],
      onConnectorCreated,
      client,
    })

    expect(getElement).toHaveBeenCalledWith(57)
    expect(getElement).toHaveBeenCalledWith(8)
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
    expect(deleteElement).not.toHaveBeenCalled()
    expect(deleteView).not.toHaveBeenCalled()
    expect(removePlacement).not.toHaveBeenCalled()
    expect(deleteConnector).not.toHaveBeenCalled()
  })

  it('rolls back created resources when a later import step fails', async () => {
    const events: string[] = []
    const createdElements = [workspaceElement(90, 'API'), workspaceElement(91, 'DB')]

    const client: MermaidImportClient = {
      getElement: vi.fn(async () => {
        throw new Error('not found')
      }),
      createElement: vi.fn(async () => {
        const next = createdElements.shift()
        if (!next) throw new Error('unexpected create')
        events.push(`createElement:${next.id}`)
        return next
      }),
      deleteElement: vi.fn(async (id) => {
        events.push(`deleteElement:${id}`)
      }),
      createView: vi.fn(async () => {
        events.push('createView:501')
        return view(501)
      }),
      deleteView: vi.fn(async (viewId) => {
        events.push(`deleteView:${viewId}`)
      }),
      addPlacement: vi.fn(async (_viewId, elementId) => {
        events.push(`addPlacement:${elementId}`)
        return {
          id: elementId + 1000,
          view_id: 10,
          element_id: elementId,
          position_x: 0,
          position_y: 0,
        }
      }),
      removePlacement: vi.fn(async (_viewId, elementId) => {
        events.push(`removePlacement:${elementId}`)
      }),
      createConnector: vi.fn(async () => {
        events.push('createConnector:fail')
        throw new Error('connector failed')
      }),
      deleteConnector: vi.fn(async (connectorId) => {
        events.push(`deleteConnector:${connectorId}`)
      }),
    }

    const parsed = {
      direction: 'LR',
      warnings: [],
      source: 'flowchart LR',
      elements: [
        { ref: 'api', name: 'API', kind: 'service', hasView: true, placements: [{ parentRef: 'root' }] },
        { ref: 'db', name: 'DB', kind: 'database', placements: [{ parentRef: 'root' }] },
      ],
      connectors: [
        { ref: 'api:db:0', viewRef: 'root', sourceElementRef: 'api', targetElementRef: 'db', label: 'uses' },
      ],
    } as unknown as ParsedImport

    await expect(importMermaidIntoView({
      parsed,
      currentViewId: 10,
      center: { x: 0, y: 0 },
      allElements: [],
      viewElements: [],
      connectors: [],
      client,
    })).rejects.toThrow('connector failed')

    expect(events).toEqual([
      'createElement:90',
      'createView:501',
      'createElement:91',
      'addPlacement:90',
      'addPlacement:91',
      'createConnector:fail',
      'removePlacement:91',
      'removePlacement:90',
      'deleteElement:91',
      'deleteView:501',
      'deleteElement:90',
    ])
    expect(client.deleteConnector).not.toHaveBeenCalled()
  })

  it('does not create duplicate connectors from duplicate Mermaid edge lines in one import', async () => {
    const createdConnector = connector(200, 1, 2, 'uses')
    const client: MermaidImportClient = {
      getElement: vi.fn(),
      createElement: vi.fn(),
      deleteElement: vi.fn(),
      createView: vi.fn(),
      deleteView: vi.fn(),
      addPlacement: vi.fn(),
      removePlacement: vi.fn(),
      createConnector: vi.fn(async () => createdConnector),
      deleteConnector: vi.fn(),
    }
    const parsed = {
      direction: 'LR',
      warnings: [],
      source: 'flowchart LR',
      elements: [
        { ref: 'node_1', name: 'API', kind: 'service', placements: [{ parentRef: 'root' }] },
        { ref: 'node_2', name: 'DB', kind: 'database', placements: [{ parentRef: 'root' }] },
      ],
      connectors: [
        { ref: 'a', viewRef: 'root', sourceElementRef: 'node_1', targetElementRef: 'node_2', label: 'uses' },
        { ref: 'b', viewRef: 'root', sourceElementRef: 'node_1', targetElementRef: 'node_2', label: 'uses' },
      ],
    } as unknown as ParsedImport

    const summary = await importMermaidIntoView({
      parsed,
      currentViewId: 10,
      center: { x: 0, y: 0 },
      allElements: [workspaceElement(1, 'API'), workspaceElement(2, 'DB')],
      viewElements: [],
      connectors: [],
      client,
    })

    expect(client.createConnector).toHaveBeenCalledTimes(1)
    expect(summary.createdConnectorCount).toBe(1)
    expect(summary.resolvedConnectorCount).toBe(1)
  })
})
