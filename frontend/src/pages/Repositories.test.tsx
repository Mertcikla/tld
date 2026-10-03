import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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
      compare: vi.fn(async () => ({
        repositoryId: 'repo-1', comparisonKey: 'pair', viewId: 9, version: 'v1', radius: 0, maxRadius: 2,
        nodes: [], edges: [], diff: { fromSnapshotId: 'snap-0', toSnapshotId: 'snap-1', fromGitRevision: 'old', toGitRevision: 'abc', sources: [], facts: { added: 0, removed: 0, modified: 0 }, edgeFacts: { added: 0, removed: 0, modified: 0 } },
      })),
      liveImpact: vi.fn(async () => ({ diagram: null, watching: false, error: '', gitBranch: 'main', gitRevision: 'abc' })),
      watchStatus: vi.fn(async () => ({
        repositoryId: 'repo-1', running: false, managed: false, state: 'stopped', stage: '',
        ownerKind: '', ownerPid: 0, repoRoot: '/repo/demo', gitBranch: 'main', gitRevision: 'abc',
        snapshotId: '', contentFingerprint: '', changedFiles: 0, pendingFiles: 0,
        startedUnix: 0, lastScanUnix: 0, lastScanMs: 0, heartbeatUnix: 0, stopRequested: false,
        pollIntervalMs: 2000, debounceMs: 500, error: '', cliAvailable: true, installHint: '',
      })),
      startWatch: vi.fn(async () => ({ running: true, state: 'starting' })),
      stopWatch: vi.fn(async () => ({ running: false, state: 'stopped' })),
      impactRadius: vi.fn(),
      delete: vi.fn(async () => {}),
      map: vi.fn(async (_repositoryId: string, handlers?: { onProgress?: (progress: { stage: string; current: number; total: number; detail: string }) => void }) => {
        handlers?.onProgress?.({ stage: 'clustering', current: 1, total: 2, detail: 'grow' })
        return { snapshotId: 'snap-1', runId: 'run-1', viewId: 5, facts: 4, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 }
      }),
    },
  },
}))

vi.mock('../components/RepositoryChangeCanvas', () => ({ default: (props: Record<string, unknown>) => React.createElement('div', { ...props, 'data-testid': 'mock-impact' }) }))

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
  afterEach(() => { vi.useRealTimers() })
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
  it('compares selected targets without running full map materialization', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(api.repositories.compare).toHaveBeenCalledWith('repo-1', expect.objectContaining({ base: { snapshotId: 'snap-0' }, head: { snapshotId: 'snap-1' }, signal: expect.any(AbortSignal) }))
    expect(api.repositories.map).not.toHaveBeenCalled()
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.diagram.viewId).toBe(9)
    renderer.unmount()
  })

  it('shows per-file insertion and deletion counts in the compact tree', async () => {
    const { api } = await import('../api/client')
    const result = await api.repositories.compare('repo-1', { base: { snapshotId: 'snap-0' }, head: { snapshotId: 'snap-1' } })
    vi.mocked(api.repositories.compare).mockResolvedValueOnce({ ...result, diff: { ...result.diff, sources: [
      { path: 'src/file.go', change: 'modified', fromHash: 'old', toHash: 'new', linesAdded: 12, linesRemoved: 3 },
    ] } })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    const counts = renderer.root.findAllByProps({ 'aria-label': '12 lines added, 3 lines removed' })[0]
    expect(counts.findAll((node) => node.children.filter((child) => typeof child === 'string').join('') === '+12').length).toBeGreaterThan(0)
    expect(counts.findAll((node) => node.children.filter((child) => typeof child === 'string').join('') === '−3').length).toBeGreaterThan(0)
    renderer.unmount()
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

  it('displays a failed comparison', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.compare).mockRejectedValueOnce(new Error('comparison unavailable'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(api.repositories.diff).not.toHaveBeenCalled()
    expect(renderer.root.findAll((node) => node.type === 'div' && node.children.includes('comparison unavailable')).length).toBeGreaterThan(0)
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

  it('ignores a comparison completed after cancellation', async () => {
    const { api } = await import('../api/client')
    const result = await api.repositories.compare('repo-1', { base: {}, head: {} })
    vi.mocked(api.repositories.compare).mockClear()
    let finish!: (value: typeof result) => void
    vi.mocked(api.repositories.compare).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    act(() => { renderer.root.findAllByType('button').find((button) => button.children.includes('Cancel'))!.props.onClick() })
    await act(async () => { finish(result) })
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.diagram).toBeNull()
    renderer.unmount()
  })

  it('ignores comparison completion after switching repositories', async () => {
    const { api } = await import('../api/client')
    const result = await api.repositories.compare('repo-1', { base: {}, head: {} })
    const [first] = await api.repositories.list()
    vi.mocked(api.repositories.list).mockResolvedValueOnce([first, { ...first, id: 'repo-2', root: '/repo/other' }])
    let finish!: (value: typeof result) => void
    vi.mocked(api.repositories.compare).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Select other' }).props.onClick() })
    await act(async () => { finish(result) })
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.diagram).toBeNull()
    expect(api.repositories.history).toHaveBeenLastCalledWith('repo-2', '', 50)
    renderer.unmount()
  })

  it('loads live state without initiating maps and changes radius without reindexing', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    const diagram = await api.repositories.compare('repo-1', { base: {}, head: {} })
    vi.mocked(api.repositories.compare).mockClear()
    vi.mocked(api.repositories.liveImpact).mockResolvedValue({ diagram: { ...diagram, comparisonKey: 'live' }, watching: true, error: '', gitBranch: 'main', gitRevision: 'abc' })
    vi.mocked(api.repositories.impactRadius).mockResolvedValue({ ...diagram, comparisonKey: 'live', radius: 1, version: 'v2' })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick() })
    expect(api.repositories.liveImpact).toHaveBeenCalledWith('repo-1', expect.any(AbortSignal))
    expect(api.repositories.compare).not.toHaveBeenCalled()
    expect(api.repositories.map).not.toHaveBeenCalled()
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.onRadius(1) })
    expect(api.repositories.impactRadius).toHaveBeenCalledWith('repo-1', 'live', 1, expect.any(AbortSignal))
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.diagram.radius).toBe(1)
    await act(async () => { renderer.unmount() })
    vi.useRealTimers()
  })

  it('starts and stops the watcher from the live panel', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'watch-start' }).props.onClick() })
    expect(api.repositories.startWatch).toHaveBeenCalledWith('repo-1', expect.objectContaining({ embed: true }))
    vi.mocked(api.repositories.watchStatus).mockResolvedValue({
      repositoryId: 'repo-1', running: true, managed: true, state: 'scanning', stage: 'tree-sitter',
      ownerKind: 'server', ownerPid: 42, repoRoot: '/repo/demo', gitBranch: 'main', gitRevision: 'abc',
      snapshotId: 'snap-1', contentFingerprint: 'fp', changedFiles: 2, pendingFiles: 1,
      startedUnix: 1, lastScanUnix: 2, lastScanMs: 12, heartbeatUnix: 3, stopRequested: false,
      pollIntervalMs: 2000, debounceMs: 500, error: '', cliAvailable: true, installHint: '',
    })
    await act(async () => { await Promise.resolve() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'watch-stop' }).props.onClick() })
    expect(api.repositories.stopWatch).toHaveBeenCalledWith('repo-1')
    await act(async () => { renderer.unmount() })
    vi.useRealTimers()
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
