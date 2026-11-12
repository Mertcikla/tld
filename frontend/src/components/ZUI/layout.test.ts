import { afterEach, describe, expect, it } from 'vitest'
import type { ExploreData, PlacedElement } from '../../types'
import { nodeDetail } from './subElements'
import { leafScreenWidthLimit } from './layoutEngine'
import { readSourcePreviewSize, writeSourcePreviewSize } from '../../utils/sourcePreviewSize'
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
    expect([node.worldW, node.worldH]).toEqual([180, 85])
    expect(nodeDetail(node, 1)).toBe('overview')
    expect(nodeDetail(node, 0.1)).toBe('overview')
    const maxZoom = leafScreenWidthLimit(node, 400) / node.worldW
    expect(nodeDetail(node, maxZoom)).toBe('code')
  })

  it('delays symbol names until detail level 40 while retaining later stage boundaries', () => {
    const node = layoutElement({ repository_id: 'repo-1', file_path: 'ai/api.py' })
    const start = 0.1 + 39 / 99 * (281 / 85 - 0.1)
    expect(nodeDetail(node, start - 0.000001)).toBe('overview')
    expect(nodeDetail(node, start)).toBe('summary')
    expect(nodeDetail(node, start + 0.000001)).toBe('summary')
    expect(nodeDetail(node, 36 / 17 - 0.000001)).toBe('summary')
    expect(nodeDetail(node, 36 / 17)).toBe('outline')
    expect(nodeDetail(node, 56 / 17 - 0.000001)).toBe('outline')
    expect(nodeDetail(node, 56 / 17)).toBe('code')
    expect(nodeDetail({ ...node, subDetail: 'outline' }, 1)).toBe('outline')
  })

  it('keeps anchored files compact despite a saved editor preview size', () => {
    writeSourcePreviewSize(1, 17, { width: 640, height: 480 })
    const node = layoutElement({ repo: '/workspace/backend', file_path: 'ai/api.py#L10-L20' })
    expect(node.subKind).toBe('code')
    expect(node.sourceLink?.file_path).toBe('ai/api.py#L10-L20')
    expect([node.worldW, node.worldH]).toEqual([180, 85])
    expect(nodeDetail(node, 1)).toBe('overview')
    expect(readSourcePreviewSize(1, 17)).toEqual({ width: 640, height: 480 })
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
