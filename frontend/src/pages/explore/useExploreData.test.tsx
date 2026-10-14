import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import type { ExploreData } from '../../types'
import { useExploreData, type ExploreDataState } from './useExploreData'

vi.mock('../../api/client', () => ({
  api: { explore: { load: vi.fn(), loadShared: vi.fn() } },
}))
vi.mock('../../crossBranch/store', () => ({ primeWorkspaceGraphSnapshot: vi.fn() }))

const emptyWorkspace: ExploreData = { tree: [], views: {}, navigations: [] }
let state: ExploreDataState
let renderer: ReactTestRenderer | undefined

function Consumer({ token, enabled = true }: { token?: string; enabled?: boolean }) {
  state = useExploreData(token, enabled)
  return null
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.explore.load).mockResolvedValue(emptyWorkspace)
  vi.mocked(api.explore.loadShared).mockResolvedValue(emptyWorkspace)
})

afterEach(() => {
  act(() => renderer?.unmount())
  renderer = undefined
})

describe('explore data loading', () => {
  it('does not fetch when the canvas receives data owned by the page', async () => {
    await act(async () => { renderer = create(<Consumer enabled={false} />) })
    expect(api.explore.load).not.toHaveBeenCalled()
    expect(api.explore.loadShared).not.toHaveBeenCalled()
  })

  it('shares an in-flight load and permits a fresh load after it completes', async () => {
    let resolve!: (data: ExploreData) => void
    vi.mocked(api.explore.load).mockReturnValueOnce(new Promise((done) => { resolve = done }))
    await act(async () => { renderer = create(<Consumer />) })

    let first!: Promise<ExploreData | null>
    let second!: Promise<ExploreData | null>
    act(() => {
      first = state.reload()
      second = state.reload()
    })
    expect(first).toBe(second)
    expect(api.explore.load).toHaveBeenCalledTimes(1)

    await act(async () => { resolve(emptyWorkspace); await first })
    expect(state.data).toBe(emptyWorkspace)
    expect(state.loading).toBe(false)

    await act(async () => { await state.reload() })
    expect(api.explore.load).toHaveBeenCalledTimes(2)
  })

  it('keeps the error and retry behavior', async () => {
    vi.mocked(api.explore.load).mockRejectedValueOnce(new Error('Workspace unavailable'))
    await act(async () => { renderer = create(<Consumer />) })
    expect(state.error).toBe('Workspace unavailable')
    expect(state.loading).toBe(false)
    expect(state.data).toBeNull()

    await act(async () => { await state.reload() })
    expect(state.error).toBeNull()
    expect(state.data).toBe(emptyWorkspace)
    expect(api.explore.load).toHaveBeenCalledTimes(2)
  })

  it('continues loading standalone shared links through the shared endpoint', async () => {
    await act(async () => { renderer = create(<Consumer token="shared-link" />) })
    expect(api.explore.loadShared).toHaveBeenCalledWith('shared-link')
    expect(api.explore.load).not.toHaveBeenCalled()
    expect(state.data).toBe(emptyWorkspace)
  })

  it('does not expose password-protected shared data', async () => {
    vi.mocked(api.explore.loadShared).mockResolvedValueOnce({ ...emptyWorkspace, password_required: true })
    await act(async () => { renderer = create(<Consumer token="protected-link" />) })
    expect(state.data).toBeNull()
    expect(state.loading).toBe(false)
    expect(state.error).toBeNull()
  })
})
