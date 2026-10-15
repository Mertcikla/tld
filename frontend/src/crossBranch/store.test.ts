import { beforeEach, expect, it, vi } from 'vitest'
import type { ExploreData, ViewTreeNode } from '../types'

const { load } = vi.hoisted(() => ({ load: vi.fn() }))
vi.mock('../api/client', () => ({ api: { explore: { load } } }))

beforeEach(() => {
  vi.resetModules()
  load.mockReset()
})

function data(name: string): ExploreData {
  return {
    tree: [{ id: 1, name, parent_view_id: null, children: [] } as unknown as ViewTreeNode],
    views: { '1': { placements: [], connectors: [] } },
    navigations: [],
  }
}

it('keeps a refreshed hierarchy when an older workspace load finishes later', async () => {
  let finishOld!: (data: ExploreData) => void
  let finishFresh!: (data: ExploreData) => void
  load.mockImplementationOnce(() => new Promise<ExploreData>((resolve) => { finishOld = resolve }))
    .mockImplementationOnce(() => new Promise<ExploreData>((resolve) => { finishFresh = resolve }))
  const { loadWorkspaceGraphSnapshot } = await import('./store')
  const old = loadWorkspaceGraphSnapshot()
  const fresh = loadWorkspaceGraphSnapshot(true)
  finishFresh(data('After move'))
  await fresh
  finishOld(data('Before move'))
  await old
  expect((await loadWorkspaceGraphSnapshot()).viewById[1].name).toBe('After move')
})
