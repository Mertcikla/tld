import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import type { Connector, ViewLayer } from '../../types'
import { connectorAlreadyHasDraggedTags, connectorTagsForDrag, connectorTagsFromDrop, resolveTagDropConnector, saveConnectorTagDrop } from './connectorTagDrop'

vi.mock('../../api/client', () => ({ api: { workspace: { connectors: { update: vi.fn() } } } }))
const connector = { id: 12, view_id: 4, tags: ['existing'], label: 'reads', source_element_id: 1, target_element_id: 2 } as Connector
const layer = { id: 5, tags: ['critical', 'backend', 'critical'] } as ViewLayer
const transfer = (values: Record<string, string>) => ({ getData: (type: string) => values[type] ?? '' })

afterEach(() => vi.clearAllMocks())

describe('connector tag drops', () => {
  it('accepts ordinary tags and layer tags while excluding internal element groups', () => {
    expect(connectorTagsFromDrop(transfer({ 'application/diag-tag': 'critical' }), [])).toEqual(['critical'])
    expect(connectorTagsFromDrop(transfer({ 'application/diag-layer': '5' }), [layer])).toEqual(layer.tags)
    expect(connectorTagsFromDrop(transfer({ 'application/diag-layer': 'missing' }), [layer])).toEqual([])
    expect(connectorTagsFromDrop(transfer({ 'application/diag-tag': 'group:12345678-1234-4234-a234-123456789012' }), [])).toEqual([])
  })

  it('identifies existing tag membership and partial layer matches during a drag', () => {
    expect(connectorTagsForDrag({ tag: 'critical' }, [layer])).toEqual(['critical'])
    expect(connectorTagsForDrag({ layerId: 5 }, [layer])).toEqual(layer.tags)
    expect(connectorAlreadyHasDraggedTags(connector, ['existing'])).toBe(true)
    expect(connectorAlreadyHasDraggedTags(connector, ['critical'])).toBe(false)
    expect(connectorAlreadyHasDraggedTags(connector, ['existing', 'critical'])).toBe(false)
    expect(connectorAlreadyHasDraggedTags(connector, [])).toBe(false)
    expect(connectorAlreadyHasDraggedTags(connector, null)).toBe(false)
  })

  it('appends without duplicates using a tags-only patch and the owning view', async () => {
    const saved = { ...connector, tags: ['existing', 'critical', 'backend'] }
    vi.mocked(api.workspace.connectors.update).mockResolvedValueOnce(saved)
    expect(await saveConnectorTagDrop(connector, layer.tags)).toBe(saved)
    expect(api.workspace.connectors.update).toHaveBeenCalledExactlyOnceWith(4, 12, { tags: saved.tags })
    expect(connector.tags).toEqual(['existing'])
  })

  it('does not save duplicate or empty drops and propagates failed saves', async () => {
    expect(await saveConnectorTagDrop(connector, ['existing'])).toBeNull()
    expect(await saveConnectorTagDrop(connector, [])).toBeNull()
    expect(api.workspace.connectors.update).not.toHaveBeenCalled()
    vi.mocked(api.workspace.connectors.update).mockRejectedValueOnce(new Error('offline'))
    await expect(saveConnectorTagDrop(connector, ['critical'])).rejects.toThrow('offline')
    expect(connector.tags).toEqual(['existing'])
  })

  it('resolves line and label drop targets without treating proxy IDs or the canvas as connectors', () => {
    class FakeElement {
      constructor(private id?: string) {}
      closest() { return this.id === undefined ? null : this }
      getAttribute() { return this.id ?? null }
    }
    vi.stubGlobal('Element', FakeElement)
    try {
      expect(resolveTagDropConnector(new FakeElement('12') as unknown as EventTarget, [connector])).toBe(connector)
      expect(resolveTagDropConnector(new FakeElement('proxy:12') as unknown as EventTarget, [connector])).toBeNull()
      expect(resolveTagDropConnector(new FakeElement() as unknown as EventTarget, [connector])).toBeNull()
      expect(resolveTagDropConnector(null, [connector])).toBeNull()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
