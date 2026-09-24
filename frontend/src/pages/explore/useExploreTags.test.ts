import { describe, expect, it } from 'vitest'
import type { ExploreData, PlacedElement, ViewLayer } from '../../types'
import { deriveExploreTagMetrics } from './useExploreTags'

function placed(elementId: number, tags: string[]): PlacedElement {
  return {
    id: elementId,
    view_id: 1,
    element_id: elementId,
    position_x: 0,
    position_y: 0,
    name: `Element ${elementId}`,
    description: null,
    kind: 'service',
    technology: null,
    url: null,
    logo_url: null,
    technology_connectors: [],
    tags,
    has_view: false,
    view_label: null,
  }
}

function layer(id: number, tags: string[]): ViewLayer {
  return {
    id,
    diagram_id: 1,
    name: `Layer ${id}`,
    color: '#fff',
    tags,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
  }
}

describe('deriveExploreTagMetrics', () => {
  it('counts tags and layer membership across views', () => {
    const groupTag = 'group:12345678-1234-4234-a234-123456789012'
    const data: ExploreData = {
      tree: [],
      navigations: [],
      views: {
        1: { placements: [placed(1, ['api', 'core', groupTag]), placed(2, ['core', groupTag])], connectors: [] },
        2: { placements: [placed(3, ['infra', groupTag])], connectors: [] },
      },
    }

    const groupLayer = { ...layer(12, [groupTag]), diagram_id: 1 }
    const metrics = deriveExploreTagMetrics(data, [layer(10, ['core', 'infra']), layer(11, ['missing']), groupLayer])

    expect(metrics.allTags).toEqual(['api', 'core', 'infra'])
    expect(metrics.tagCounts).toEqual({ api: 1, core: 2, infra: 1 })
    expect(metrics.layerElementCounts).toEqual({ 10: 3, 11: 0, 12: 2 })
  })

  it('returns empty metrics without explore data', () => {
    expect(deriveExploreTagMetrics(null, [])).toEqual({
      allTags: [],
      tagCounts: {},
      layerElementCounts: {},
    })
  })
})
