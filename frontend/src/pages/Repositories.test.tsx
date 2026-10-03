import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Repositories from './Repositories'

const { navigateMock, searchParamsMock, setParamsMock } = vi.hoisted(() => ({ navigateMock: vi.fn(), searchParamsMock: vi.fn(), setParamsMock: vi.fn() }))

vi.mock('react-router-dom', () => ({
  useNavigate: () => navigateMock,
  useSearchParams: () => [searchParamsMock(), setParamsMock],
}))
vi.mock('../components/RepositoryHistory', () => ({ default: () => null }))

vi.mock('../api/client', () => ({
  api: {
    repositories: {
      list: vi.fn(async () => [{
        id: 'repo-1',
        root: '/repo/demo',
        latestSnapshotId: 'snap-1',
        latestCreatedUnix: 100,
        gitRevision: 'abc',
        gitBranch: 'main',
        facts: 4,
        chunks: 4,
        edges: 0,
        sources: 4,
      }]),
      snapshots: vi.fn(async () => [
        { id: 'snap-0', repositoryId: 'repo-1', createdUnix: 90, gitRevision: 'old', gitBranch: 'main', provenance: 'commit', contentFingerprint: 'fp-0', ingestionStatus: 'complete', embeddingStatus: 'complete', projects: [], warnings: [] },
        { id: 'snap-1', repositoryId: 'repo-1', createdUnix: 100, gitRevision: 'abc', gitBranch: 'main', provenance: 'commit', contentFingerprint: 'fp-1', ingestionStatus: 'complete', embeddingStatus: 'complete', projects: [], warnings: [] },
      ]),
      maps: vi.fn(async () => []),
      history: vi.fn(async () => ({ commits: [], branches: [], headSha: '', currentBranch: '', isGit: false, hasMore: false })),
      diff: vi.fn(async () => null),
      delete: vi.fn(async () => {}),
      map: vi.fn(async (_repositoryId: string, handlers?: { onProgress?: (progress: { stage: string; current: number; total: number; detail: string }) => void }) => {
        handlers?.onProgress?.({ stage: 'clustering', current: 1, total: 2, detail: 'grow' })
        return { snapshotId: 'snap-1', runId: 'run-1', viewId: 5, facts: 4, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 }
      }),
    },
  },
}))

vi.mock('../utils/toast', () => ({ toast: vi.fn() }))

vi.mock('@chakra-ui/icons', () => ({
  CopyIcon: () => null,
  DeleteIcon: () => null,
  RepeatIcon: () => null,
  ChevronLeftIcon: () => null,
  ChevronRightIcon: () => null,
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  type NodeProps = { children?: React.ReactNode; isOpen?: boolean; leastDestructiveRef?: unknown }
  const BoxLike = ({ children, isOpen, leastDestructiveRef: _leastDestructiveRef, ...props }: NodeProps) => (isOpen === false ? null : ReactModule.createElement('div', props, children))
  const ButtonLike = ({ children, onClick, isLoading, loadingText, isDisabled, ...props }: {
    children?: React.ReactNode
    onClick?: () => void
    isLoading?: boolean
    loadingText?: string
    isDisabled?: boolean
  }) => ReactModule.createElement('button', { ...props, disabled: isDisabled, onClick }, isLoading && loadingText ? loadingText : children)
  const SwitchLike = ({ isChecked, onChange, isDisabled, size: _size, colorScheme: _colorScheme, ...props }: {
    isChecked?: boolean
    onChange?: React.ChangeEventHandler<HTMLInputElement>
    isDisabled?: boolean
    size?: string
    colorScheme?: string
  }) => ReactModule.createElement('input', { ...props, type: 'checkbox', checked: !!isChecked, disabled: isDisabled, onChange: onChange ?? (() => {}) })
  return {
    Alert: BoxLike,
    AlertDialog: BoxLike,
    AlertDialogBody: BoxLike,
    AlertDialogContent: BoxLike,
    AlertDialogFooter: BoxLike,
    AlertDialogHeader: BoxLike,
    AlertDialogOverlay: BoxLike,
    AlertIcon: BoxLike,
    Badge: BoxLike,
    Box: BoxLike,
    Button: ButtonLike,
    Center: BoxLike,
    Code: BoxLike,
    Divider: BoxLike,
    Flex: BoxLike,
    Grid: BoxLike,
    HStack: BoxLike,
    IconButton: ButtonLike,
    Progress: BoxLike,
    Select: BoxLike,
    Spinner: BoxLike,
    Switch: SwitchLike,
    Text: BoxLike,
    Tooltip: BoxLike,
    VStack: BoxLike,
  }
})

describe('Repositories map action', () => {
  beforeEach(() => {
    navigateMock.mockClear()
    vi.clearAllMocks()
    searchParamsMock.mockReturnValue(new URLSearchParams())
    globalThis.localStorage ??= { getItem: () => null, setItem: () => {} } as unknown as Storage
  })

  it('runs the mapper for the selected head and stays on this page', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })

    const mapButton = renderer.root.findByProps({ 'data-testid': 'repositories-map' })
    await act(async () => {
      await mapButton.props.onClick()
    })

    const { api } = await import('../api/client')
    expect(api.repositories.map).toHaveBeenCalledWith('repo-1', expect.objectContaining({ snapshotId: 'snap-1', onProgress: expect.any(Function) }))
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('passes the imports toggle through to the mapper', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.map).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-include-imports' }).props.onChange({ target: { checked: true } })
    })
    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'repositories-map' }).props.onClick()
    })

    expect(api.repositories.map).toHaveBeenCalledWith('repo-1', expect.objectContaining({ includeImports: true }))
  })

  it('deletes a repository after confirmation, including materialized resources when toggled', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.delete).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-delete-repo-1' }).props.onClick({ stopPropagation: () => {} })
    })
    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-delete-materialized' }).props.onChange({ target: { checked: true } })
    })
    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'confirm-dialog-confirm' }).props.onClick()
    })

    expect(api.repositories.delete).toHaveBeenCalledWith('repo-1', { deleteMaterialized: true })
    expect(api.repositories.list).toHaveBeenCalled()
  })
  it('prepares both selected maps in order before comparing their resolved snapshot IDs', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.map)
      .mockResolvedValueOnce({ snapshotId: 'resolved-base', runId: 'base-map', viewId: 5, facts: 1, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 })
      .mockResolvedValueOnce({ snapshotId: 'resolved-head', runId: 'head-map', viewId: 5, facts: 1, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(api.repositories.map).toHaveBeenNthCalledWith(1, 'repo-1', expect.objectContaining({ snapshotId: 'snap-0' }))
    expect(api.repositories.map).toHaveBeenNthCalledWith(2, 'repo-1', expect.objectContaining({ snapshotId: 'snap-1' }))
    expect(api.repositories.diff).toHaveBeenCalledWith({ fromSnapshotId: 'resolved-base', toSnapshotId: 'resolved-head' })
  })

  it('changes base and head independently and maps local working contents', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.onChange({ target: { value: 'working_tree' } }) })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.value).toBe('snapshot:snap-0')
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-map' }).props.onClick() })
    expect(api.repositories.map).toHaveBeenCalledWith('repo-1', expect.objectContaining({ workingTree: true }))
  })

  it('does not compare when preparing a map fails', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.map).mockRejectedValueOnce(new Error('embedding service unavailable'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(api.repositories.diff).not.toHaveBeenCalled()
    expect(renderer.root.findAll((node) => node.type === 'div' && node.children.includes('embedding service unavailable')).length).toBeGreaterThan(0)
  })

  it('restores targets from the URL and preserves them when browsing a branch', async () => {
    const { api } = await import('../api/client')
    searchParamsMock.mockReturnValue(new URLSearchParams('repo=repo-1&base=snapshot:snap-1&head=working_tree&branch=feature'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.value).toBe('snapshot:snap-1')
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.value).toBe('working_tree')
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'History branch' }).props.onChange({ target: { value: 'main' } }) })
    expect(api.repositories.history).toHaveBeenCalledWith('repo-1', 'main', 50)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.value).toBe('working_tree')
  })

  it('ignores a completed map when its operation was canceled', async () => {
    const { api } = await import('../api/client')
    let finish!: (value: Awaited<ReturnType<typeof api.repositories.map>>) => void
    vi.mocked(api.repositories.map).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    act(() => { renderer.root.findAllByType('button').find((button) => button.children.includes('Cancel'))!.props.onClick() })
    await act(async () => { finish({ snapshotId: 'cancelled', runId: 'map', viewId: 5, facts: 1, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 }) })
    expect(api.repositories.diff).not.toHaveBeenCalled()
    expect(api.repositories.map).toHaveBeenCalledTimes(1)
  })

  it('ignores map completion after switching repositories', async () => {
    const { api } = await import('../api/client')
    const [first] = await api.repositories.list()
    vi.mocked(api.repositories.list).mockResolvedValueOnce([first, { ...first, id: 'repo-2', root: '/repo/other' }])
    let finish!: (value: Awaited<ReturnType<typeof api.repositories.map>>) => void
    vi.mocked(api.repositories.map).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Select other' }).props.onClick() })
    await act(async () => { finish({ snapshotId: 'stale', runId: 'old-map', viewId: 5, facts: 1, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 }) })
    expect(api.repositories.diff).not.toHaveBeenCalled()
    expect(api.repositories.map).toHaveBeenCalledTimes(1)
    expect(api.repositories.history).toHaveBeenLastCalledWith('repo-2', '', 50)
  })

  it('ignores history and snapshot responses from a previously selected repository', async () => {
    const { api } = await import('../api/client')
    const [first] = await api.repositories.list()
    const saved = await api.repositories.snapshots('repo-1')
    vi.mocked(api.repositories.list).mockResolvedValueOnce([first, { ...first, id: 'repo-2', root: '/repo/other' }])
    let finish!: (value: typeof saved) => void
    vi.mocked(api.repositories.snapshots)
      .mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
      .mockResolvedValueOnce(saved.map((snapshot) => ({ ...snapshot, id: `other-${snapshot.id}`, repositoryId: 'repo-2' })))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Select other' }).props.onClick() })
    await act(async () => { finish(saved) })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.value).toBe('snapshot:other-snap-0')
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.value).toBe('snapshot:other-snap-1')
  })

})
