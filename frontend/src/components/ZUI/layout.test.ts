import { afterEach, describe, expect, it } from 'vitest'
import type { ExploreData, PlacedElement } from '../../types'
import { isDetailedSubElement } from '../../utils/subElements'
import { writeSourcePreviewSize } from '../../utils/sourcePreviewSize'
import { computeLayout } from './layout'

function layoutElement(source: Partial<PlacedElement>) {
  const data: ExploreData = {
    tree: [{ id: 1, name: 'Source', owner_element_id: null, parent_view_id: null, description: null,
      level: 0, level_label: null, depth: 0, children: [], created_at: '', updated_at: '' }],
    views: { '1': { placements: [{ id: 17, view_id: 1, element_id: 17, name: 'api.py', kind: 'file',
      position_x: 0, position_y: 0, description: null, technology: null, url: null, logo_url: null,
      technology_connectors: [], tags: [], has_view: false, view_label: null, ...source }], connectors: [] } },
    navigations: [],
  }
  return computeLayout(data).groups[0].nodes[0]
}

afterEach(() => writeSourcePreviewSize(1, 17, null))

describe('Explore linked source layout', () => {
  it.each(['file', 'class', 'service', 'component', null])('reveals linked files of kind %s without a saved preview size', kind => {
    const node = layoutElement({ kind, repository_id: 'repo-1', file_path: 'ai/api.py' })
    expect(node.subKind).toBe('code')
    expect(node.type).toBe(kind ?? 'system')
    expect([node.worldW, node.worldH]).toEqual([520, 360])
    expect(isDetailedSubElement(node.subKind, node.subDetail, node.worldW, node.worldH, 1)).toBe(true)
    expect(isDetailedSubElement(node.subKind, node.subDetail, node.worldW, node.worldH, 0.1)).toBe(false)
  })

  it('supports repository paths and anchored files while preserving saved preview dimensions', () => {
    writeSourcePreviewSize(1, 17, { width: 640, height: 480 })
    const node = layoutElement({ repo: '/workspace/backend', file_path: 'ai/api.py#L10-L20' })
    expect(node.subKind).toBe('code')
    expect(node.sourceLink?.file_path).toBe('ai/api.py#L10-L20')
    expect([node.worldW, node.worldH]).toEqual([640, 480])
  })

  it.each([
    { repo: '/workspace/backend', file_path: 'ai/' },
    { repo: '/workspace/backend', file_path: '' },
    { file_path: 'ai/api.py' },
  ])('keeps non-file or incomplete links compact: %j', source => {
    const node = layoutElement(source)
    expect(node.subKind).toBeNull()
    expect([node.worldW, node.worldH]).toEqual([180, 85])
  })
})
