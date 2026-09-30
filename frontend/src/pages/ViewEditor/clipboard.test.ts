import { describe, expect, it } from 'vitest'
import type { Connector, PlacedElement } from '../../types'
import {
  buildViewSelectionClipboardPayload,
  findViewSelectionMissingElementIds,
  findViewSelectionPasteConflicts,
  mapViewSelectionElementIds,
  parseViewSelectionClipboardPayload,
  planViewSelectionPasteConnectors,
  planViewSelectionPastePlacements,
  serializeViewSelectionClipboardPayload,
} from './clipboard'

function placement(elementId: number, x: number, y: number): PlacedElement {
  return {
    id: elementId + 1000,
    view_id: 10,
    element_id: elementId,
    position_x: x,
    position_y: y,
    name: `Element ${elementId}`,
    description: `Description ${elementId}`,
    kind: 'service',
    technology: 'Go',
    url: 'https://example.com',
    logo_url: null,
    technology_connectors: [{ type: 'custom', label: 'Go' }],
    tags: ['api'],
    repo: 'repo',
    branch: 'main',
    file_path: `src/${elementId}.go`,
    language: 'go',
    bypass_noise_gate: elementId === 1,
    has_view: false,
    view_label: null,
  }
}

function connector(id: number, sourceElementId: number, targetElementId: number, viewId = 10): Connector {
  return {
    id,
    view_id: viewId,
    source_element_id: sourceElementId,
    target_element_id: targetElementId,
    label: `Connector ${id}`,
    description: null,
    relationship: 'uses',
    direction: 'forward',
    style: 'bezier',
    url: null,
    source_handle: 'right',
    target_handle: 'left',
    tags: ['sync'],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  }
}

describe('ViewEditor clipboard helpers', () => {
  it('builds payloads from selected placements only', () => {
    const payload = buildViewSelectionClipboardPayload(
      10,
      [placement(1, 100, 100), placement(2, 200, 100), placement(3, 300, 100)],
      [connector(1, 1, 2), connector(2, 2, 3)],
      [1, 2],
    )

    expect(payload?.elements.map((element) => element.elementId)).toEqual([1, 2])
    expect(payload?.elements[0]).toMatchObject({
      elementId: 1,
      name: 'Element 1',
      repo: 'repo',
      bypass_noise_gate: true,
    })
  })

  it('includes only connectors whose endpoints are selected in the source view', () => {
    const payload = buildViewSelectionClipboardPayload(
      10,
      [placement(1, 100, 100), placement(2, 200, 100), placement(3, 300, 100)],
      [
        connector(1, 1, 2),
        connector(2, 2, 3),
        connector(3, 1, 2, 20),
      ],
      [1, 2],
    )

    expect(payload?.connectors).toHaveLength(1)
    expect(payload?.connectors[0]).toMatchObject({
      source_element_id: 1,
      target_element_id: 2,
      label: 'Connector 1',
    })
  })

  it('detects same-view placement conflicts', () => {
    const payload = buildViewSelectionClipboardPayload(
      10,
      [placement(1, 100, 100), placement(2, 200, 100)],
      [],
      [1, 2],
    )

    expect(payload).not.toBeNull()
    expect(findViewSelectionPasteConflicts(payload!, new Set([2, 3]))).toEqual([2])
  })

  it('detects elements that are missing from the target workspace', () => {
    const payload = buildViewSelectionClipboardPayload(
      10,
      [placement(1, 100, 100), placement(2, 200, 100), placement(3, 300, 100)],
      [],
      [1, 2, 3],
    )

    expect(payload).not.toBeNull()
    expect(findViewSelectionMissingElementIds(payload!, new Set([2, 3]))).toEqual([1])
    expect(findViewSelectionMissingElementIds(payload!, new Set([1, 2, 3]))).toEqual([])
  })

  it('plans reuse and duplicate element id mappings', () => {
    const payload = buildViewSelectionClipboardPayload(
      10,
      [placement(1, 100, 100), placement(2, 200, 100), placement(3, 300, 100)],
      [connector(1, 1, 2), connector(2, 2, 3)],
      [1, 2, 3],
    )

    expect(payload).not.toBeNull()
    const idMap = mapViewSelectionElementIds(payload!, new Map([[2, 22]]))

    expect(Array.from(idMap.entries())).toEqual([
      [1, 1],
      [2, 22],
      [3, 3],
    ])
    expect(planViewSelectionPasteConnectors(payload!, idMap).map((item) => [item.sourceElementId, item.targetElementId])).toEqual([
      [1, 22],
      [22, 3],
    ])
  })

  it('preserves relative placement positions around the paste center', () => {
    const payload = buildViewSelectionClipboardPayload(
      10,
      [placement(1, 100, 100), placement(2, 220, 160)],
      [],
      [1, 2],
    )

    expect(payload).not.toBeNull()
    const placements = planViewSelectionPastePlacements(
      payload!,
      mapViewSelectionElementIds(payload!, new Map([[2, 22]])),
      { x: 500, y: 400 },
    )

    expect(placements).toEqual([
      { sourceElementId: 1, elementId: 1, x: 440, y: 370 },
      { sourceElementId: 2, elementId: 22, x: 560, y: 430 },
    ])
  })

  it('round-trips valid clipboard JSON and rejects invalid payloads', () => {
    const payload = buildViewSelectionClipboardPayload(
      10,
      [placement(1, 100, 100), placement(2, 200, 100)],
      [connector(1, 1, 2)],
      [1, 2],
    )

    expect(payload).not.toBeNull()
    expect(parseViewSelectionClipboardPayload(serializeViewSelectionClipboardPayload(payload!))).toEqual(payload)
    expect(parseViewSelectionClipboardPayload('not json')).toBeNull()
    expect(parseViewSelectionClipboardPayload(JSON.stringify({ ...payload, version: 99 }))).toBeNull()
    expect(parseViewSelectionClipboardPayload(JSON.stringify({ ...payload, connectors: [{ source_element_id: 1, target_element_id: 99, direction: 'forward', style: 'bezier' }] }))).toBeNull()
  })
})
