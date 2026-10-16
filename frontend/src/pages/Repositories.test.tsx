import React from 'react'
import type { LiveRepositoryImpact, RepositoryGitHistory } from '../api/client'
import { create as createMessage } from '@bufbuild/protobuf'
import { CodeFactSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/codeindex/v1/codeindex_pb.js'
import { act, create } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Repositories from './Repositories'
import RepositoryWatcherPanel from '../components/RepositoryWatcherPanel'
import type { RepositoryIndexerCheck } from '../api/client'

const missingIndexerCheck: RepositoryIndexerCheck = {
  ready: false,
  indexers: [
    { family: 'go', tool: 'scip-go', languages: ['go'], installed: true, version: '0.2.7', minVersion: '0.1.26', belowMinimum: false, installHint: '', downloadUrl: '', path: '/usr/local/bin/scip-go' },
    { family: 'dotnet', tool: 'scip-dotnet', languages: ['csharp'], installed: false, version: '', minVersion: '0.2.14', belowMinimum: false, installHint: 'dotnet tool install --global scip-dotnet', downloadUrl: 'https://github.com/sourcegraph/scip-dotnet/releases', path: '' },
  ],
}

const { navigateMock, searchParamsMock, setParamsMock } = vi.hoisted(() => ({ navigateMock: vi.fn(), searchParamsMock: vi.fn(), setParamsMock: vi.fn() }))

vi.mock('react-router-dom', () => ({
  useNavigate: () => navigateMock,
  useSearchParams: () => [searchParamsMock(), setParamsMock],
}))
vi.mock('../components/RepositoryTargetPicker', () => ({ default: (props: Record<string, unknown>) => React.createElement('div', props) }))
vi.mock('../components/RepositoryHistory', () => ({ default: (props: Record<string, unknown>) => {
  const { footerContent, ...rest } = props
  return React.createElement('div', { ...rest, 'data-testid': 'mock-history' }, footerContent as React.ReactNode)
} }))

vi.mock('../api/client', () => ({
  api: {
    editor: { open: vi.fn(async () => {}) },
    system: { capabilities: vi.fn(async () => ({ editor: true, watch: true, repositories: true })) },
    repositories: {
      list: vi.fn(async () => [{
        id: 'repo-1',
        root: '/repo/demo',
        latestSnapshotId: 'snap-1',
        latestCreatedUnix: 100,
        gitRevision: 'abc',
        gitBranch: 'main',
        facts: 4,
        edges: 0,
        sources: 4,
        remoteUrl: 'https://github.com/test/demo',
        name: 'demo',
        managed: false,
      }]),
      checkIndexers: vi.fn(async () => ({ ready: true, indexers: [] })),
      browseFolders: vi.fn(async (path: string) => ({ path: path || '/home/dev', parentPath: '/', folders: ['project'] })),
      snapshots: vi.fn(async () => [
        { id: 'snap-0', repositoryId: 'repo-1', createdUnix: 90, gitRevision: 'old', gitBranch: 'main', provenance: 'commit', contentFingerprint: 'fp-0', ingestionStatus: 'complete', compatible: true, projects: [], warnings: [] },
        { id: 'snap-1', repositoryId: 'repo-1', createdUnix: 100, gitRevision: 'abc', gitBranch: 'main', provenance: 'commit', contentFingerprint: 'fp-1', commitMessage: 'feat: snapshot message', ingestionStatus: 'complete', compatible: true, projects: [], warnings: [] },
      ]),
      captureSnapshot: vi.fn(async (_repositoryId: string, _handlers?: { workingTree?: boolean; onProgress?: (progress: { stage: string; current: number; total: number; detail: string }) => void }) => ({
        id: 'snap-2', repositoryId: 'repo-1', createdUnix: 110, gitRevision: 'abc', gitBranch: 'main', provenance: 'manual', contentFingerprint: 'fp-2', ingestionStatus: 'complete', compatible: true, projects: [], warnings: [],
      })),
      settings: vi.fn(async () => ({ mapDefaults: { resolution: 1, minGroupSize: 2, minRootGroups: 3, maxRootGroups: 20, maxChildren: 8, maxDepth: 4, maxLeafFiles: 40, maxConnectorsPerView: 40, maxLeafConnectorsPerView: 12, crossViewConnectors: true, crossViewMaxViews: 8, crossViewMaxElementsPerView: 2, crossViewMaxConnectorsPerView: 8, crossViewMaxConnectorsPerElement: 8 }, mapOverrides: {}, effectiveMap: {}, remotes: [], isGit: false, currentBranch: '', headSha: '' })),
      updateMapConfiguration: vi.fn(),
      updateRemote: vi.fn(),
      maps: vi.fn(async () => []),
      history: vi.fn(async (): Promise<RepositoryGitHistory> => ({ repositoryUrl: 'https://github.com/test/demo', commits: [], branches: [], headSha: '', currentBranch: '', isGit: false, hasMore: false })),
      commitDetails: vi.fn(async (_id: string, revision: string) => ({ commit: { sha: revision, subject: 'Latest change', body: 'Latest change\n\nFull commit details.', author: 'Developer', authorEmail: '', createdUnix: 1, parents: [], refs: [] }, files: [] })),
      rangeSummary: vi.fn(async () => ({ commits: 3, additions: 10, deletions: 4, changed: 6, files: 2 })),
      openPullRequests: vi.fn(async () => [{ number: 7, title: 'Feature PR', url: 'https://github.com/test/demo/pull/7', baseBranch: 'main', headBranch: 'feature' }]),
      pullRequest: vi.fn(async () => ({ title: 'Feature PR', url: 'https://github.com/test/demo/pull/7', baseSha: 'pr-base', headSha: 'pr-head', baseBranch: 'main', headBranch: 'feature' })),
      fileSymbols: vi.fn(async () => []),
      diff: vi.fn(async () => null),
      compare: vi.fn(async () => ({
        repositoryId: 'repo-1', comparisonKey: 'pair', viewId: 9, version: 'v1', maxRadius: 2,
        nodes: [], edges: [], diff: { fromSnapshotId: 'snap-0', toSnapshotId: 'snap-1', fromGitRevision: 'old', toGitRevision: 'abc', sources: [], facts: { added: 0, removed: 0, modified: 0 }, edgeFacts: { added: 0, removed: 0, modified: 0 } },
      })),
      watchLiveImpact: vi.fn(async (_id: string, options: { signal: AbortSignal; onUpdate: (update: LiveRepositoryImpact) => void }) => {
        const { api } = await import('../api/client')
        const watchStatus = await api.repositories.watchStatus(_id)
        if (!options.signal.aborted) options.onUpdate({ diagram: null, watching: watchStatus.running, error: '', gitBranch: 'main', gitRevision: 'abc', watchStatus })
        await new Promise<void>((resolve) => options.signal.addEventListener('abort', () => resolve(), { once: true }))
      }),
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
      delete: vi.fn(async () => {}),
      deleteSnapshot: vi.fn(async () => {}),
      add: vi.fn(async (_path: string, handlers?: { onProgress?: (progress: { stage: string; current: number; total: number; detail: string }) => void; materialize?: boolean }) => {
        handlers?.onProgress?.({ stage: 'tree-sitter', current: 1, total: 2, detail: 'parsing' })
        return { id: 'repo-2', root: '/repo/new', latestSnapshotId: 'snap-2' }
      }),
      map: vi.fn(async (_repositoryId: string, handlers?: { onProgress?: (progress: { stage: string; current: number; total: number; detail: string }) => void }) => {
        handlers?.onProgress?.({ stage: 'clustering', current: 1, total: 2, detail: 'grow' })
        return { snapshotId: 'snap-1', runId: 'run-1', viewId: 5, facts: 4, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 }
      }),
    },
  },
}))

vi.mock('../components/RepositoryChangeCanvas', () => ({
  default: (props: Record<string, unknown>) => React.createElement('div', { ...props, 'data-testid': 'mock-impact' }),
}))

vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('../utils/sourceEditor', () => ({ useSourceEditor: () => ({ editor: 'zed' }) }))

vi.mock('@chakra-ui/icons', () => ({
  ArrowBackIcon: () => null,
  AddIcon: () => null,
  CopyIcon: () => null,
  ExternalLinkIcon: () => null,
  DeleteIcon: () => null,
  RepeatIcon: () => null,
  TimeIcon: () => null,
  SettingsIcon: () => null,
  ChevronLeftIcon: () => null,
  ChevronRightIcon: () => null,
  ChevronDownIcon: () => null,
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  type NodeProps = { children?: React.ReactNode; isOpen?: boolean; leastDestructiveRef?: unknown; closeOnBlur?: boolean }
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
  const ModalLike = ({ children, isOpen, onClose: _onClose, isCentered: _isCentered, ...props }: NodeProps & { onClose?: () => void; isCentered?: boolean }) =>
    isOpen === false ? null : ReactModule.createElement('div', props, children)
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
    FormControl: BoxLike,
    Grid: BoxLike,
    HStack: BoxLike,
    Icon: BoxLike,
    IconButton: ButtonLike,
    Input: (props: Record<string, unknown>) => ReactModule.createElement('input', props),
    Modal: ModalLike,
    ModalBody: BoxLike,
    ModalCloseButton: BoxLike,
    ModalContent: BoxLike,
    ModalFooter: BoxLike,
    ModalHeader: BoxLike,
    ModalOverlay: BoxLike,
    Popover: ({ children, isOpen, closeOnBlur }: NodeProps) => {
      const kids = ReactModule.Children.toArray(children)
      return ReactModule.createElement(
        'div',
        {
          'data-testid': 'repositories-add-popover',
          'data-close-on-blur': String(closeOnBlur ?? true),
        },
        isOpen === false ? kids[0] : kids,
      )
    },
    PopoverArrow: () => null,
    PopoverBody: BoxLike,
    PopoverCloseButton: ({ children }: NodeProps) => ReactModule.createElement('div', null, children),
    PopoverContent: BoxLike,
    PopoverFooter: BoxLike,
    PopoverHeader: BoxLike,
    PopoverTrigger: ({ children }: NodeProps) => ReactModule.createElement('div', null, children),
    Portal: ({ children }: NodeProps) => ReactModule.createElement('div', null, children),
    Progress: BoxLike,
    Select: BoxLike,
    Spinner: (props: Record<string, unknown>) => ReactModule.createElement('span', { ...props, 'data-testid': 'mock-spinner' }),
    Switch: SwitchLike,
    Text: BoxLike,
    Tooltip: BoxLike,
    VStack: BoxLike,
  }
})

/** Counts only rendered DOM nodes, ignoring the mocked Chakra components themselves. */
function hostNodes(renderer: ReturnType<typeof create>, testId: string) {
  return renderer.root
    .findAll((node) => node.type === 'div' && node.props['data-testid'] === testId)
}

/**
 * The `data-testid` of every rendered div between `node` and the root. Resize
 * handles rely on `align-self: stretch`, so they must sit directly in the flex
 * row: any extra div in this chain means the drag target has no height.
 */
function divTestIdAncestry(node: { type: unknown; props: Record<string, unknown>; parent: unknown }) {
  const chain: (string | null)[] = []
  let current = node as { type: unknown; props: Record<string, unknown>; parent: unknown } | null
  while (current) {
    if (current.type === 'div') {
      const testId = current.props['data-testid']
      chain.push(typeof testId === 'string' ? testId : null)
    }
    current = current.parent as typeof current
  }
  return chain
}

describe('Repositories map action', () => {
  afterEach(() => { vi.useRealTimers() })
  beforeEach(() => {
    navigateMock.mockClear()
    vi.clearAllMocks()
    searchParamsMock.mockReturnValue(new URLSearchParams())
    globalThis.localStorage ??= { getItem: () => null, setItem: () => {} } as unknown as Storage
  })

  it('requests 100 commits, appends older pages once, and allows retry after a page error', async () => {
    const { api } = await import('../api/client')
    const commit = (sha: string) => ({ sha, subject: sha, author: 'Test', authorEmail: '', createdUnix: 1, parents: [], refs: [], body: '' })
    const firstPage: RepositoryGitHistory = { commits: [commit('new')], branches: [], headSha: 'new', currentBranch: 'main', isGit: true, hasMore: true, nextPageToken: 'older' }
    vi.mocked(api.repositories.history).mockResolvedValueOnce(firstPage)
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(api.repositories.history).toHaveBeenCalledWith('repo-1', '', 100)
    const history = () => renderer.root.findByProps({ 'data-testid': 'mock-history' })
    vi.mocked(api.repositories.history).mockRejectedValueOnce(new Error('Page unavailable'))
    await act(async () => { history().props.onLoadMore() })
    expect(history().props.history.commits).toEqual(firstPage.commits)
    expect(history().props.loadMoreError).toBe('Page unavailable')
    let finish!: (page: RepositoryGitHistory) => void
    vi.mocked(api.repositories.history).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    act(() => { history().props.onLoadMore(); history().props.onLoadMore() })
    expect(history().props.loadingMore).toBe(true)
    expect(api.repositories.history).toHaveBeenCalledTimes(3)
    expect(api.repositories.history).toHaveBeenLastCalledWith('repo-1', '', 100, { pageToken: 'older', signal: expect.any(AbortSignal) })
    await act(async () => { finish({ ...firstPage, commits: [commit('old')], hasMore: false, nextPageToken: '' }) })
    expect(history().props.history.commits.map((item: { sha: string }) => item.sha)).toEqual(['new', 'old'])
    expect(history().props.history.nextPageToken).toBe('')
    expect(history().props.loadMoreError).toBe('')
    expect(history().props.loadingMore).toBe(false)
    act(() => { renderer.unmount() })
  })

  it('aborts older history pages when switching repositories and ignores their late results', async () => {
    const { api } = await import('../api/client')
    const [repository] = await api.repositories.list()
    vi.mocked(api.repositories.list).mockResolvedValueOnce([repository, { ...repository, id: 'repo-2', name: 'other', root: '/repo/other' }])
    const firstPage: RepositoryGitHistory = { commits: [], branches: [], headSha: 'new', currentBranch: 'main', isGit: true, hasMore: true, nextPageToken: 'older' }
    vi.mocked(api.repositories.history).mockResolvedValueOnce(firstPage)
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    let finish!: (page: RepositoryGitHistory) => void
    vi.mocked(api.repositories.history).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    act(() => { renderer.root.findByProps({ 'data-testid': 'mock-history' }).props.onLoadMore() })
    const signal = vi.mocked(api.repositories.history).mock.calls.at(-1)?.[3]?.signal
    expect(signal?.aborted).toBe(false)
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Select other' }).props.onClick() })
    expect(signal?.aborted).toBe(true)
    await act(async () => { finish({ ...firstPage, nextPageToken: 'stale' }) })
    const history = renderer.root.findByProps({ 'data-testid': 'mock-history' }).props
    expect(history.repositoryId).toBe('repo-2')
    expect(history.history.nextPageToken).toBeUndefined()
    expect(history.loadingMore).toBe(false)
    act(() => { renderer.unmount() })
  })

  it('starts with the repository list collapsed when repositories already exist', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(renderer.root.findByProps({ 'aria-label': 'Expand repositories' })).toBeTruthy()
    await act(async () => { renderer.unmount() })
  })

  it('hides history in Watch and hosts repository information in settings', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-delete-repo-1' })).toHaveLength(0)
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Expand repositories' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-settings-repo-1' }).props.onClick({ stopPropagation: () => {} }) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-settings-page' }).length).toBeGreaterThan(0)
    await act(async () => { renderer.root.findByType((await import('./RepositorySettings')).default).props.onBack() })
    expect(renderer.root.findAllByProps({ 'data-testid': 'mock-history' })).toHaveLength(1)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick() })
    expect(renderer.root.findAllByProps({ 'data-testid': 'mock-history' })).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.children).toBe('Watch')
    await act(async () => { renderer.unmount() })
  })

  it('hides the Watch tab when the server disables watching', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.system.capabilities).mockResolvedValueOnce({ watch: false, editor: false, repositories: true })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-live-tab' })).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-compare-tab' })).toBeTruthy()
    await act(async () => { renderer.unmount() })
  })

  it('locks PR targets, compares PR commits, and restores the manual comparison on exit', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-pr-tab' }).props.onClick() })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-compare' })).toHaveLength(0)
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Pull request number or URL' }).props.onChange({ target: { value: '7' } }) })
    await act(async () => { await renderer.root.findAll((node) => node.props.as === 'form')[0].props.onSubmit({ preventDefault: () => {} }) })
    expect(api.repositories.pullRequest).toHaveBeenCalledWith('repo-1', '7', expect.any(AbortSignal))
    const target = renderer.root.findByProps({ 'data-testid': 'repositories-base-target' })
    expect(target.props.value).toBe('commit:pr-base')
    expect(target.props.isDisabled).toBe(true)
    expect(renderer.root.findByProps({ 'data-testid': 'mock-history' }).props.disabled).toBe(true)
    await act(async () => { target.props.onChange('working_tree') })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.value).toBe('commit:pr-base')
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(api.repositories.compare).toHaveBeenCalledWith('repo-1', expect.objectContaining({ base: expect.objectContaining({ gitRevision: 'pr-base' }), head: expect.objectContaining({ gitRevision: 'pr-head' }) }))
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare-tab' }).props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.value).toBe('snapshot:snap-0')
    await act(async () => { renderer.unmount() })
  })

  it('shows the origin URL and loads open PRs for selection', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-pr-tab' }).props.onClick() })
    expect(renderer.root.findAllByProps({ href: 'https://github.com/test/demo' }).length).toBeGreaterThan(0)
    const load = renderer.root.findAll((node) => node.type === 'button' && node.props['aria-label'] === 'Load open PRs')[0]
    await act(async () => { await load.props.onClick() })
    expect(api.repositories.openPullRequests).toHaveBeenCalledWith('repo-1', expect.any(AbortSignal))
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Open pull requests' }).props.onChange({ target: { value: '7' } }) })
    expect(api.repositories.pullRequest).toHaveBeenCalledWith('repo-1', '7', expect.any(AbortSignal))
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.value).toBe('commit:pr-head')
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.isDisabled).toBe(true)
    await act(async () => { renderer.unmount() })
  })

  it('shows an empty open PR list and reports GitHub errors', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.openPullRequests).mockResolvedValueOnce([])
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-pr-tab' }).props.onClick() })
    const load = () => renderer.root.findAll((node) => node.type === 'button' && node.props['aria-label'] === 'Load open PRs')[0]
    await act(async () => { await load().props.onClick() })
    expect(renderer.root.findAll((node) => node.type === 'div' && node.children.includes('No open pull requests.')).length).toBeGreaterThan(0)
    vi.mocked(api.repositories.openPullRequests).mockRejectedValueOnce(new Error('GitHub unavailable'))
    await act(async () => { await load().props.onClick() })
    expect(renderer.root.findAll((node) => node.type === 'div' && node.children.includes('GitHub unavailable')).length).toBeGreaterThan(0)
    await act(async () => { renderer.unmount() })
  })

  it('compares the selected targets without individual map buttons', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })
    const { api } = await import('../api/client')
    expect(renderer.root.findAll((node) => node.props['data-testid'] === 'repositories-map' || node.props['data-testid'] === 'repositories-map-base')).toHaveLength(0)
    await act(async () => {
      renderer.root.findByProps({ 'aria-label': 'Increase comparison context' }).props.onClick()
      renderer.root.findByProps({ 'aria-label': 'Increase comparison context' }).props.onClick()
    })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(api.repositories.compare).toHaveBeenCalledWith('repo-1', expect.objectContaining({ base: { snapshotId: 'snap-0' }, head: { snapshotId: 'snap-1' }, signal: expect.any(AbortSignal), blastRadius: 2 }))
    expect(navigateMock).not.toHaveBeenCalled()
    renderer.unmount()
  })

  it('deletes a repository after confirmation, including materialized resources when toggled', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.delete).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })

    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Expand repositories' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-settings-repo-1' }).props.onClick({ stopPropagation: () => {} }) })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-delete-repo-1' }).props.onClick({ stopPropagation: () => {} })
    })
    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-delete-materialized' }).props.onChange({ target: { checked: true } })
    })
    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'confirm-dialog-confirm' }).props.onClick()
    })

    expect(api.repositories.delete).toHaveBeenCalledWith('repo-1', { deleteMaterialized: true, deleteClone: false })
    expect(api.repositories.list).toHaveBeenCalled()
  })
  it.each(['base', 'head'] as const)('clears the selected %s target after deleting its snapshot', async (side) => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.deleteSnapshot).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })
    const snapshotId = side === 'base' ? 'snap-0' : 'snap-1'
    expect(renderer.root.findByProps({ 'data-testid': `repositories-${side}-target` }).props.value).toBe(`snapshot:${snapshotId}`)

    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })

    act(() => {
      renderer.root
        .findByProps({ 'data-testid': `repositories-snapshot-delete-${snapshotId}` })
        .props.onClick({ stopPropagation: () => {} })
    })
    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'confirm-dialog-confirm' }).props.onClick()
    })

    expect(api.repositories.deleteSnapshot).toHaveBeenCalledWith(snapshotId)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare-tab' }).props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': `repositories-${side}-target` }).props.value).toBe('')
    expect(renderer.root.findByProps({ 'data-testid': `repositories-${side === 'base' ? 'head' : 'base'}-target` }).props.value).toBe(`snapshot:${side === 'base' ? 'snap-1' : 'snap-0'}`)
    await act(async () => { renderer.unmount() })
  })
  it('clears PR targets when selecting a newly added repository', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-pr-tab' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Pull request number or URL' }).props.onChange({ target: { value: '7' } }) })
    await act(async () => { await renderer.root.findAll((node) => node.props.as === 'form')[0].props.onSubmit({ preventDefault: () => {} }) })
    const repositories = await api.repositories.list()
    vi.mocked(api.repositories.list).mockResolvedValueOnce([...repositories, { ...repositories[0], id: 'repo-2', root: '/repo/new', name: 'new' }])
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props['aria-pressed']).toBe(true)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-pr-tab' }).props.onClick() })
    expect(renderer.root.findByProps({ 'aria-label': 'Pull request number or URL' }).props.value).toBe('')
    expect(api.repositories.history).toHaveBeenCalledWith('repo-2', '', 100)
    expect(vi.mocked(api.repositories.history).mock.calls.filter(([id]) => id === 'repo-2').some(([, branch]) => branch === 'pr-head')).toBe(false)
    await act(async () => { renderer.unmount() })
  })
  it('adds a repository from the sidebar dialog', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.list).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })
    const repositories = await api.repositories.list()
    vi.mocked(api.repositories.list).mockResolvedValueOnce([...repositories, { ...repositories[0], id: 'repo-2', root: '/repo/new', name: 'new' }])

    const addCell = () =>
      renderer.root.findAll(
        (node) =>
          node.type === 'div' &&
          node.props['data-testid'] === 'repositories-add',
      )[0]

    act(() => {
      renderer.root.findByProps({ 'aria-label': 'Expand repositories' }).props.onClick()
    })
    act(() => {
      renderer.root.findByProps({ 'aria-label': 'Collapse repositories' }).props.onClick()
    })
    expect(addCell()).toBeTruthy()
    act(() => {
      renderer.root.findByProps({ 'aria-label': 'Expand repositories' }).props.onClick()
    })
    act(() => {
      addCell().props.onClick()
    })
    act(() => {
      renderer.root
        .findByProps({ 'data-testid': 'repositories-add-path' })
        .props.onChange({ target: { value: '/repo/new' } })
    })
    await act(async () => {
      await renderer.root
        .findByProps({ 'data-testid': 'repositories-add-submit' })
        .props.onClick()
    })

    expect(api.repositories.checkIndexers).toHaveBeenCalledWith(
      { path: '/repo/new' }, { signal: expect.any(AbortSignal) },
    )
    expect(api.repositories.add).toHaveBeenCalledWith(
      { path: '/repo/new' },
      expect.objectContaining({
        materialize: false,
        onProgress: expect.any(Function),
      }),
    )
    expect(api.repositories.list).toHaveBeenCalled()
    expect(api.repositories.startWatch).not.toHaveBeenCalled()
    expect(hostNodes(renderer, 'repositories-snapshots-panel')).toHaveLength(1)
    renderer.unmount()
  })
  it('adds a remote repository from the sidebar dialog', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.list).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick()
    })
    act(() => {
      renderer.root
        .findByProps({ 'data-testid': 'repositories-add-path' })
        .props.onChange({ target: { value: 'facebook/react' } })
    })
    await act(async () => {
      await renderer.root
        .findByProps({ 'data-testid': 'repositories-add-submit' })
        .props.onClick()
    })

    expect(api.repositories.checkIndexers).toHaveBeenCalledWith(
      { remoteUrl: 'facebook/react' }, { signal: expect.any(AbortSignal) },
    )
    expect(api.repositories.add).toHaveBeenCalledWith(
      { remoteUrl: 'facebook/react' },
      expect.objectContaining({ materialize: false, onProgress: expect.any(Function) }),
    )
    renderer.unmount()
  })
  it('browses folders and checks the selected absolute path before adding', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-browse' }).props.onClick() })
    expect(api.repositories.browseFolders).toHaveBeenCalledWith('', expect.any(AbortSignal))
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Open folder project' }).props.onClick() })
    expect(api.repositories.browseFolders).toHaveBeenLastCalledWith('/home/dev/project', expect.any(AbortSignal))
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-folder-up' }).props.onClick() })
    expect(api.repositories.browseFolders).toHaveBeenLastCalledWith('/', expect.any(AbortSignal))
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Open folder project' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-folder-select' }).props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.value).toBe('/project')
    expect(hostNodes(renderer, 'repositories-folder-picker')).toHaveLength(0)
    await act(async () => { await vi.advanceTimersByTimeAsync(500) })
    expect(api.repositories.checkIndexers).toHaveBeenCalledWith({ path: '/project' }, { signal: expect.any(AbortSignal) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })
    expect(api.repositories.add).toHaveBeenCalledWith({ path: '/project' }, expect.any(Object))
    renderer.unmount()
  })
  it('preserves the repository input when folder selection is cancelled or fails', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.browseFolders).mockRejectedValueOnce(new Error('Cannot browse folder'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/existing' } }) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-browse' }).props.onClick() })
    expect(api.repositories.browseFolders).toHaveBeenCalledWith('/repo/existing', expect.any(AbortSignal))
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-folder-select' }).props.isDisabled).toBe(true)
    expect(renderer.root.findAll((node) => node.props.children === 'Cannot browse folder').length).toBeGreaterThan(0)
    act(() => { renderer.root.findAll((node) => node.type === 'button' && node.props.children === 'Cancel selection')[0].props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.value).toBe('/repo/existing')
    renderer.unmount()
  })
  it('shows Git and indexer warnings without blocking Add or Enter', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    const warnings = ['No Git repository was found at this folder\'s root.', 'No required indexers were detected.']
    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce({ ready: true, indexers: [], warnings } as RepositoryIndexerCheck)
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { await vi.advanceTimersByTimeAsync(500) })
    for (const warning of warnings) {
      expect(renderer.root.findAll((node) => node.props.children === warning).length).toBeGreaterThan(0)
    }
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.isDisabled).toBe(false)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onKeyDown({ key: 'Enter' }) })
    expect(api.repositories.add).toHaveBeenCalledTimes(1)
    renderer.unmount()
  })
  it('automatically checks indexers in the add dialog and waits for missing tools to be installed', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce(missingIndexerCheck)
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { await vi.advanceTimersByTimeAsync(500) })

    expect(api.repositories.checkIndexers).toHaveBeenCalledTimes(1)
    expect(hostNodes(renderer, 'repositories-indexer-scip-go')).toHaveLength(1)
    expect(hostNodes(renderer, 'repositories-indexer-scip-dotnet')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.props.children === 'dotnet tool install --global scip-dotnet').length).toBeGreaterThan(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-indexer-download-scip-dotnet' }).props.href).toBe('https://github.com/sourcegraph/scip-dotnet/releases')
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.isDisabled).toBe(true)
    // Enter must respect the same requirement as the disabled submit button.
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onKeyDown({ key: 'Enter' }) })
    expect(api.repositories.add).not.toHaveBeenCalled()

    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce({ ready: true, indexers: [] })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-indexers-recheck' }).props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.isDisabled).toBe(false)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })
    expect(api.repositories.checkIndexers).toHaveBeenCalledTimes(2)
    expect(api.repositories.add).toHaveBeenCalledTimes(1)
    renderer.unmount()
  })
  it('ignores stale indexer checks after changing the repository input', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    let finish!: (check: RepositoryIndexerCheck) => void
    vi.mocked(api.repositories.checkIndexers).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/old' } }) })
    await act(async () => { await vi.advanceTimersByTimeAsync(500) })
    const signal = vi.mocked(api.repositories.checkIndexers).mock.calls[0][1]?.signal
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    expect(signal?.aborted).toBe(true)
    await act(async () => { await vi.advanceTimersByTimeAsync(500) })
    await act(async () => { finish(missingIndexerCheck) })
    expect(hostNodes(renderer, 'repositories-indexer-scip-dotnet')).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.isDisabled).toBe(false)
    expect(api.repositories.checkIndexers).toHaveBeenLastCalledWith({ path: '/repo/new' }, { signal: expect.any(AbortSignal) })
    renderer.unmount()
  })
  it('shows check failures in the dialog and allows a re-check', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.checkIndexers).mockRejectedValueOnce(new Error('Repository is unavailable'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })
    expect(api.repositories.add).not.toHaveBeenCalled()
    expect(renderer.root.findAll((node) => node.props.children === 'Repository is unavailable').length).toBeGreaterThan(0)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-indexers-check' }).props.onClick() })
    expect(renderer.root.findAll((node) => node.props.children === 'Repository is unavailable')).toHaveLength(0)
    expect(api.repositories.checkIndexers).toHaveBeenCalledTimes(2)
    renderer.unmount()
  })
  it('checks required indexers from the Snapshots tab and re-checks on demand', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce({
      ready: false,
      indexers: [
        {
          family: 'dotnet',
          tool: 'scip-dotnet',
          languages: ['csharp'],
          installed: false,
          installHint: 'dotnet tool install --global scip-dotnet',
          version: '',
          minVersion: '0.2.14',
          belowMinimum: false,
          downloadUrl: 'https://github.com/sourcegraph/scip-dotnet/releases',
          path: '',
        },
      ],
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    expect(renderer.root.findAll((node) => node.type === 'button' && node.props['data-testid'] === 'repositories-indexers-check')).toHaveLength(1)
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-indexers-check' }).props.onClick() })

    expect(api.repositories.checkIndexers).toHaveBeenCalledWith({ path: '/repo/demo' })
    expect(hostNodes(renderer, 'repositories-indexer-scip-dotnet')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.props.children === 'dotnet tool install --global scip-dotnet').length).toBeGreaterThan(0)
    expect(renderer.root.findAll((node) => node.props.children === '1 missing').length).toBeGreaterThan(0)

    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce({ ready: true, indexers: [] })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-indexers-recheck' }).props.onClick() })
    expect(api.repositories.checkIndexers).toHaveBeenLastCalledWith({ path: '/repo/demo' })
    renderer.unmount()
  })
  it('shows indexer versions, status, and download links', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce({
      ready: false,
      indexers: [
        {
          family: 'go',
          tool: 'scip-go',
          languages: ['go'],
          installed: true,
          installHint: 'go install github.com/scip-code/scip-go/cmd/scip-go@latest',
          version: 'scip-go 0.2.7',
          minVersion: '0.1.26',
          belowMinimum: false,
          downloadUrl: 'https://github.com/scip-code/scip-go/releases',
          path: '/usr/local/bin/scip-go',
        },
        {
          family: 'dotnet',
          tool: 'scip-dotnet',
          languages: ['csharp'],
          installed: false,
          installHint: 'dotnet tool install --global scip-dotnet',
          version: '',
          minVersion: '0.2.14',
          belowMinimum: false,
          downloadUrl: 'https://github.com/sourcegraph/scip-dotnet/releases',
          path: '',
        },
      ],
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-indexers-check' }).props.onClick() })

    expect(hostNodes(renderer, 'repositories-indexer-scip-go')).toHaveLength(1)
    expect(hostNodes(renderer, 'repositories-indexer-scip-dotnet')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.props.children === 'scip-go 0.2.7').length).toBeGreaterThan(0)
    expect(renderer.root.findAll((node) => node.props.children === 'min 0.1.26').length).toBeGreaterThan(0)
    expect(renderer.root.findAll((node) => node.props.children === 'min 0.2.14').length).toBeGreaterThan(0)
    expect(renderer.root.findAll((node) => node.props.children === '1 missing').length).toBeGreaterThan(0)
    const download = renderer.root.findAll((node) => node.type === 'button' && node.props['data-testid'] === 'repositories-indexer-download-scip-dotnet')
    expect(download).toHaveLength(1)
    expect(download[0].props.href).toBe('https://github.com/sourcegraph/scip-dotnet/releases')
    expect(renderer.root.findAll((node) => node.type === 'button' && node.props['data-testid'] === 'repositories-indexer-download-scip-go')).toHaveLength(0)
    renderer.unmount()
  })
  it('shows the commit subject with its full message on hover and worktree totals', async () => {
    const message = 'Improve watcher controls and materialization history\n\nPreserve status and focus while indexing.'
    const props = { status: null, repositoryRoot: '/repo/demo', branch: 'main', revision: 'abc123456', commitMessage: message, busy: false, onStart: vi.fn(), onStop: vi.fn(), onRestart: vi.fn() }
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryWatcherPanel {...props} worktreeStats={{ additions: 3400, deletions: 1234 }} />) })
    const commit = renderer.root.findByProps({ 'data-testid': 'watch-commit-message' })
    expect(commit.props.children).toBe(message.split('\n')[0])
    expect(commit.props.title).toBe(message)
    const lineText = (testId: string) => hostNodes(renderer, testId)[0].children.join('')
    expect(lineText('watch-lines-added')).toBe('+3,400')
    expect(lineText('watch-lines-removed')).toBe('−1,234')
    await act(async () => renderer.update(<RepositoryWatcherPanel {...props} worktreeStats={{ additions: 0, deletions: 0 }} />))
    expect(lineText('watch-lines-added')).toBe('+0')
    expect(lineText('watch-lines-removed')).toBe('−0')
    await act(async () => renderer.update(<RepositoryWatcherPanel {...props} />))
    expect(lineText('watch-lines-added')).toBe('+—')
    expect(lineText('watch-lines-removed')).toBe('−—')
    await act(async () => renderer.unmount())
  })

  it('loads the exact watched commit message when Git history does not contain it', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.commitDetails).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick())
    expect(api.repositories.commitDetails).toHaveBeenCalledWith('repo-1', 'abc')
    expect(api.repositories.rangeSummary).toHaveBeenCalledWith('repo-1', expect.objectContaining({ base: 'HEAD', headWorkingTree: true, signal: expect.any(AbortSignal) }))
    expect(hostNodes(renderer, 'watch-lines-added')[0].children.join('')).toBe('+10')
    expect(hostNodes(renderer, 'watch-lines-removed')[0].children.join('')).toBe('−4')
    expect(renderer.root.findByProps({ 'data-testid': 'watch-commit-message' }).props.title).toBe('Latest change\n\nFull commit details.')
    await act(async () => renderer.unmount())
  })

  it('keeps the latest Git identity through empty startup updates without leaking it to another checkout', async () => {
    const props = {
      status: null, repositoryRoot: '/repo/demo', branch: 'main', revision: 'abc123456', commitMessage: 'Initial commit', busy: false,
      onStart: vi.fn(), onStop: vi.fn(), onRestart: vi.fn(), onRefresh: vi.fn(),
    }
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryWatcherPanel {...props} />) })
    const badgeText = () => renderer.root.findByProps({ 'aria-label': 'Watched Git revision' }).findAll((node) => typeof node.type === 'string').flatMap((node) => node.children.filter((child): child is string => typeof child === 'string'))
    await act(async () => renderer.update(<RepositoryWatcherPanel {...props} branch="" revision="" />))
    expect(badgeText()).toEqual(expect.arrayContaining(['main', 'abc1234']))
    expect(renderer.root.findByProps({ 'data-testid': 'watch-commit-message' }).props.children).toBe('Initial commit')
    await act(async () => renderer.update(<RepositoryWatcherPanel {...props} branch="feature" revision="def123456" />))
    await act(async () => renderer.update(<RepositoryWatcherPanel {...props} branch="" revision="" />))
    expect(badgeText()).toEqual(expect.arrayContaining(['feature', 'def1234']))
    await act(async () => renderer.update(<RepositoryWatcherPanel {...props} branch="" revision="fed123456" />))
    expect(badgeText()).toEqual(expect.arrayContaining(['Detached HEAD', 'fed1234']))
    await act(async () => renderer.update(<RepositoryWatcherPanel {...props} repositoryRoot="/repo/other" branch="" revision="" />))
    expect(renderer.root.findAllByProps({ 'aria-label': 'Watched Git revision' })).toHaveLength(0)
    await act(async () => renderer.unmount())
  })

  it.each([
    { family: 'go', tool: 'gopls', languages: ['go'], installHint: 'go install golang.org/x/tools/gopls@latest', downloadUrl: 'https://go.dev/gopls/' },
    { family: 'web', tool: 'typescript-language-server', languages: ['typescript'], installHint: 'npm install -g typescript-language-server typescript', downloadUrl: 'https://github.com/typescript-language-server/typescript-language-server' },
    { family: 'rust', tool: 'rust-analyzer', languages: ['rust'], installHint: 'rustup component add rust-analyzer rust-src', downloadUrl: 'https://rust-analyzer.github.io/' },
    { family: 'python', tool: 'pyright-langserver', languages: ['python'], installHint: 'npm install -g pyright', downloadUrl: 'https://github.com/microsoft/pyright' },
  ])('shows $tool health failures and their install action', async ({ family, tool, languages, installHint, downloadUrl }) => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce({
      ready: false,
      indexers: [{
        family, tool, languages, installed: false,
        installHint,
        version: '', minVersion: '', belowMinimum: false,
        downloadUrl, path: `/usr/local/bin/${tool}`,
        checkError: 'LSP health check: language server disconnected',
      }],
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-indexers-check' }).props.onClick() })
    expect(hostNodes(renderer, `repositories-indexer-${tool}`)).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.props.children === 'LSP health check: language server disconnected').length).toBeGreaterThan(0)
    expect(renderer.root.findAll((node) => node.type === 'button' && node.props['data-testid'] === `repositories-indexer-download-${tool}`)).toHaveLength(1)
    renderer.unmount()
  })
  it('collapses the indexer checklist when every indexer is ready', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.checkIndexers).mockResolvedValueOnce({
      ready: true,
      indexers: [
        {
          family: 'go',
          tool: 'scip-go',
          languages: ['go'],
          installed: true,
          installHint: 'go install github.com/scip-code/scip-go/cmd/scip-go@latest',
          version: 'scip-go 0.2.7',
          minVersion: '0.1.26',
          belowMinimum: false,
          downloadUrl: 'https://github.com/scip-code/scip-go/releases',
          path: '/usr/local/bin/scip-go',
        },
      ],
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-indexers-check' }).props.onClick() })

    expect(renderer.root.findAll((node) => node.props.children === 'Ready').length).toBeGreaterThan(0)
    expect(hostNodes(renderer, 'repositories-indexer-scip-go')).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-indexers-toggle' }).props['aria-expanded']).toBe(false)
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-indexers-toggle' }).props.onClick() })
    expect(hostNodes(renderer, 'repositories-indexer-scip-go')).toHaveLength(1)
    renderer.unmount()
  })
  it('opens the Snapshots tab when a repository is added', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    const repositories = await api.repositories.list()
    vi.mocked(api.repositories.list).mockResolvedValueOnce([...repositories, { ...repositories[0], id: 'repo-2', root: '/repo/new', name: 'new' }])
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props['aria-pressed']).toBe(true)
    expect(hostNodes(renderer, 'repositories-snapshots-panel')).toHaveLength(1)
    renderer.unmount()
  })
  it('deletes a managed clone when toggled', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.list).mockResolvedValueOnce([{
      id: 'repo-1',
      root: '/data/repositories/github.com-test-demo-1234abcd',
      latestSnapshotId: 'snap-1',
      latestCreatedUnix: 100,
      gitRevision: 'abc',
      gitBranch: 'main',
      facts: 4,
      edges: 0,
      sources: 4,
      remoteUrl: 'https://github.com/test/demo',
      name: 'demo',
      managed: true,
    }])
    vi.mocked(api.repositories.delete).mockClear()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })

    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Expand repositories' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-settings-repo-1' }).props.onClick({ stopPropagation: () => {} }) })
    act(() => {
      renderer.root.findByProps({ 'data-testid': 'repositories-delete-repo-1' }).props.onClick({ stopPropagation: () => {} })
    })
    await act(async () => {
      await renderer.root.findByProps({ 'data-testid': 'confirm-dialog-confirm' }).props.onClick()
    })

    expect(api.repositories.delete).toHaveBeenCalledWith('repo-1', { deleteMaterialized: false, deleteClone: true })
    renderer.unmount()
  })
  it('keeps progress visible in the dialog while adding the first repository, then opens Snapshots', async () => {
    const { api } = await import('../api/client')
    const added = { ...(await api.repositories.list())[0], id: 'repo-2', root: '/repo/new', latestSnapshotId: 'snap-2' }
    vi.mocked(api.repositories.list).mockResolvedValueOnce([]).mockResolvedValueOnce([added])
    let finish!: (value: typeof added) => void
    vi.mocked(api.repositories.add).mockImplementationOnce((_path, handlers) => new Promise((resolve) => {
      handlers?.onProgress?.({ stage: 'clone', current: 0, total: 0, detail: 'Fetching repository' })
      finish = resolve
    }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })
    expect(hostNodes(renderer, 'repositories-add-progress')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.props.children === 'Cloning repository · Fetching repository').length).toBeGreaterThan(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.isDisabled).toBe(true)
    await act(async () => { finish(added) })
    expect(hostNodes(renderer, 'repositories-add-progress')).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-add-path' })).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props['aria-pressed']).toBe(true)
    expect(hostNodes(renderer, 'repositories-snapshots-panel')).toHaveLength(1)
    renderer.unmount()
  })
  it('keeps the dialog open with an error when adding the first repository fails', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.list).mockResolvedValueOnce([])
    vi.mocked(api.repositories.add).mockRejectedValueOnce(new Error('Indexing failed'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })
    expect(renderer.root.findAll((node) => node.props.children === 'Indexing failed').length).toBeGreaterThan(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.value).toBe('/repo/new')
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.isDisabled).toBe(false)
    expect(hostNodes(renderer, 'repositories-add-progress')).toHaveLength(0)
    renderer.unmount()
  })
  it('shows indexing progress on the Snapshots tab while adding a repository', async () => {
    const { api } = await import('../api/client')
    let finish!: (value: { id: string; root: string; latestSnapshotId: string }) => void
    vi.mocked(api.repositories.add).mockImplementationOnce(
      (_path, handlers) =>
        new Promise((resolve) => {
          handlers?.onProgress?.({
            stage: 'tree-sitter',
            current: 1,
            total: 4,
            detail: 'src/main.go',
          })
          finish = resolve
        }),
    )
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-add-path' }).props.onChange({ target: { value: '/repo/new' } }) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-add-submit' }).props.onClick() })

    expect(hostNodes(renderer, 'repositories-operation-progress')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.props.children === 'Parsing sources · src/main.go · 1/4').length).toBeGreaterThan(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props['aria-pressed']).toBe(true)

    await act(async () => {
      finish({ id: 'repo-2', root: '/repo/new', latestSnapshotId: 'snap-2' })
    })
    renderer.unmount()
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

  it('collapses the history list while keeping compare controls in its header', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    const history = () => renderer.root.findByProps({ 'data-testid': 'mock-history' })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-base-target' }).length).toBeGreaterThan(0)
    await act(async () => { history().props.onToggle() })
    expect(history().props.collapsed).toBe(true)
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-base-target' }).length).toBeGreaterThan(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-compare' })).toBeTruthy()
    await act(async () => { history().props.onToggle() })
    expect(history().props.collapsed).toBe(false)
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-base-target' }).length).toBeGreaterThan(0)
    renderer.unmount()
  })

  it('auto-collapses history after comparing while keeping controls visible', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(renderer.root.findByProps({ 'data-testid': 'mock-history' }).props.collapsed).toBe(false)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'mock-history' }).props.collapsed).toBe(true)
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-base-target' }).length).toBeGreaterThan(0)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-compare' })).toBeTruthy()
    renderer.unmount()
  })

  it('summarizes the selected range between base and head in the history footer', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.rangeSummary).mockResolvedValueOnce({ commits: 7, additions: 120, deletions: 30, changed: 45, files: 4 })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(api.repositories.rangeSummary).toHaveBeenCalledWith('repo-1', expect.objectContaining({ base: 'old', head: 'abc', baseWorkingTree: false, headWorkingTree: false }))
    const summary = renderer.root.findByProps({ 'data-testid': 'repositories-range-summary' })
    expect(summary.findAll((node) => node.props.children === '7 commits').length).toBeGreaterThan(0)
    expect(summary.findAll((node) => node.props.children === '+120').length).toBeGreaterThan(0)
    expect(summary.findAll((node) => node.props.children === '~45').length).toBeGreaterThan(0)
    expect(summary.findAll((node) => node.props.children === '−30').length).toBeGreaterThan(0)
    await act(async () => { renderer.unmount() })
  })

  it('re-runs the quick git diff when BASE or HEAD changes', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.onChange('working_tree') })
    expect(api.repositories.rangeSummary).toHaveBeenLastCalledWith('repo-1', expect.objectContaining({ base: 'old', head: '', headWorkingTree: true }))
    await act(async () => { renderer.unmount() })
  })

  it('shows truncated commit subjects in both range cards and their menu options', async () => {
    const { api } = await import('../api/client')
    const commit = (sha: string, subject: string) => ({ sha, subject, author: 'Developer', authorEmail: '', createdUnix: 1, parents: [], refs: [], body: '' })
    const older = commit('older', 'fix: preserve selection while filtering commit history')
    const newer = commit('newer', 'feat: browse repository folders and include branch metadata')
    vi.mocked(api.repositories.history).mockResolvedValueOnce({ commits: [newer, older], branches: [], headSha: 'newer', currentBranch: 'main', isGit: true, hasMore: false })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    vi.mocked(api.repositories.commitDetails).mockClear()
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'mock-history' }).props.onRange(older, newer) })
    for (const [side, subject] of [['base', older.subject], ['head', newer.subject]]) {
      const picker = renderer.root.findByProps({ 'data-testid': `repositories-${side}-target` })
      const summary = picker.props.triggerContent.props.children[1].props.children[0]
      expect(summary.props.children).toBe(subject)
      expect(summary.props.title).toBe(subject)
      expect(summary.props.isTruncated).toBe(true)
      expect(picker.props.groups[0].options.at(-1).label).toBe(subject)
    }
    expect(api.repositories.commitDetails).not.toHaveBeenCalled()
    act(() => { renderer.unmount() })
  })

  it('loads subjects for commit links outside the current history page and ignores obsolete responses', async () => {
    const { api } = await import('../api/client')
    searchParamsMock.mockReturnValue(new URLSearchParams('repo=repo-1&base=commit:old-commit&head=commit:head-commit'))
    let finish!: (value: Awaited<ReturnType<typeof api.repositories.commitDetails>>) => void
    vi.mocked(api.repositories.commitDetails).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    vi.mocked(api.repositories.commitDetails).mockResolvedValueOnce({ commit: { sha: 'head-commit', subject: 'feat: selected head subject', body: '', author: '', authorEmail: '', createdUnix: 1, parents: [], refs: [] }, files: [] })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    const summary = (side: string) => renderer.root.findByProps({ 'data-testid': `repositories-${side}-target` }).props.triggerContent.props.children[1].props.children[0].props.children
    expect(summary('head')).toBe('feat: selected head subject')
    vi.mocked(api.repositories.commitDetails).mockResolvedValueOnce({ commit: { sha: 'replacement', subject: 'fix: replacement base subject', body: '', author: '', authorEmail: '', createdUnix: 1, parents: [], refs: [] }, files: [] })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.onChange('commit:replacement') })
    expect(summary('base')).toBe('fix: replacement base subject')
    await act(async () => { finish({ commit: { sha: 'old-commit', subject: 'Obsolete subject', body: '', author: '', authorEmail: '', createdUnix: 1, parents: [], refs: [] }, files: [] }) })
    expect(summary('base')).toBe('fix: replacement base subject')
    act(() => { renderer.unmount() })
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

  it('compacts folder chains, preserves branches, and selects files after expanding', async () => {
    const { api } = await import('../api/client')
    const result = await api.repositories.compare('repo-1', { base: {}, head: {} })
    vi.mocked(api.repositories.compare).mockResolvedValueOnce({ ...result, diff: { ...result.diff, sources: [
      { path: 'frontend/src/pages/Repositories.tsx', change: 'modified', fromHash: 'old', toHash: 'new', linesAdded: 2, linesRemoved: 39 },
      { path: 'internal/mapper/math.go', change: 'modified', fromHash: 'old', toHash: 'new', linesAdded: 1, linesRemoved: 1 },
      { path: 'internal/store/apistore.go', change: 'modified', fromHash: 'old', toHash: 'new', linesAdded: 1, linesRemoved: 0 },
    ] } })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    const folder = (label: string) => renderer.root.findAllByProps({ 'aria-label': label }).find((node) => node.props.onClick)!
    expect(folder('Collapse frontend/src/pages').props['aria-expanded']).toBe(true)
    expect(folder('Collapse internal')).toBeDefined()
    expect(folder('Collapse internal/mapper')).toBeDefined()
    expect(folder('Collapse internal/store')).toBeDefined()

    act(() => { folder('Collapse frontend/src/pages').props.onClick() })
    expect(renderer.root.findAllByProps({ 'aria-label': 'frontend/src/pages/Repositories.tsx' })).toHaveLength(0)
    expect(folder('Collapse internal/store')).toBeDefined()
    act(() => { folder('Expand frontend/src/pages').props.onClick() })
    act(() => { folder('frontend/src/pages/Repositories.tsx').props.onClick() })
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.selectedPath).toBe('frontend/src/pages/Repositories.tsx')
    expect(api.repositories.fileSymbols).not.toHaveBeenCalled()
    expect(renderer.root.findAllByProps({ role: 'tab' }).find((node) => node.children.includes('Symbols '))?.props['aria-selected']).toBe(false)
    act(() => { renderer.root.findAllByProps({ role: 'tab' }).find((node) => node.props.onClick && node.children.includes('Files '))!.props.onClick() })
    expect(renderer.root.findAllByProps({ 'aria-label': '1 lines added, 0 lines removed' }).length).toBeGreaterThan(0)
    renderer.unmount()
  })

  it('shows only changed symbols for the selected file', async () => {
    const { api } = await import('../api/client')
    const result = await api.repositories.compare('repo-1', { base: {}, head: {} })
    const changed = createMessage(CodeFactSchema, { id: 'changed', logicalKey: 'changed', name: 'Changed', anchor: { path: 'src/file.go', startLine: 20 } })
    const other = createMessage(CodeFactSchema, { id: 'other', name: 'Other file', anchor: { path: 'src/other.go' } })
    vi.mocked(api.repositories.compare).mockResolvedValueOnce({ ...result, diff: { ...result.diff,
      sources: [{ path: 'src/file.go', change: 'modified', fromHash: 'old', toHash: 'new' }],
      facts: { added: 1, removed: 2, modified: 2 },
      factDetails: { added: [], removed: [], modified: [changed, other] },
    } })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    await act(async () => { renderer.root.findAllByProps({ 'aria-label': 'src/file.go' }).find((node) => node.props.onClick)!.props.onClick() })
    expect(renderer.root.findAllByProps({ 'aria-label': 'Changed symbols count' }).some((node) => node.children.includes('5'))).toBe(true)
    const symbols = renderer.root.findByProps({ 'data-testid': 'repository-symbols' })
    const rows = symbols.findAll((node) => node.type === 'div' && node.props['data-symbol-change'])
    expect(rows.map((node) => node.props['data-symbol-change'])).toEqual(['modified'])
    expect(renderer.root.findAllByProps({ role: 'tab' }).find((node) => node.children.includes('Files '))?.props['aria-selected']).toBe(true)
    const fileRow = renderer.root.findAllByProps({ 'aria-label': 'src/file.go' }).find((node) => node.props.onClick)!
    expect(fileRow.props['aria-expanded']).toBe(true)
    expect(rows[0].findAll((node) => node.children.includes('Changed')).length).toBeGreaterThan(0)
    expect(api.repositories.fileSymbols).not.toHaveBeenCalled()
    expect(symbols.findAll((node) => node.children.includes('Other file'))).toHaveLength(0)
    expect(symbols.findAll((node) => node.props.as === 'details')).toHaveLength(0)
    const editorButton = rows[0].findAllByType('button').find((node) => node.props['aria-label'] === 'Open in Zed')!
    await act(async () => { await editorButton.props.onClick() })
    expect(api.editor.open).toHaveBeenCalledWith({ editor: 'zed', repo: '/repo/demo', file_path: 'src/file.go', line: 21 })
    act(() => { fileRow.props.onClick() })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-symbols' })).toHaveLength(0)
    expect(fileRow.props['aria-expanded']).toBe(false)
    await act(async () => { renderer.unmount() })
  })

  it('groups all changed symbols by file when opening Symbols without selecting a file', async () => {
    const { api } = await import('../api/client')
    const result = await api.repositories.compare('repo-1', { base: {}, head: {} })
    const added = createMessage(CodeFactSchema, { id: 'added', name: 'Added', anchor: { path: 'src/b.go', startLine: 0 } })
    const removed = createMessage(CodeFactSchema, { id: 'removed', name: 'Removed', anchor: { path: 'src/a.go', startLine: 7 } })
    const modified = createMessage(CodeFactSchema, { id: 'modified', name: 'Modified', anchor: { path: 'src/b.go', startLine: 8 } })
    vi.mocked(api.repositories.compare).mockResolvedValueOnce({ ...result, diff: { ...result.diff,
      factDetails: { added: [added], removed: [removed], modified: [modified] },
    } })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    act(() => { renderer.root.findAllByProps({ role: 'tab' }).find((node) => node.props.onClick && node.children.includes('Symbols '))!.props.onClick() })
    const symbols = renderer.root.findByProps({ 'data-testid': 'repository-symbols' })
    const groups = symbols.findAll((node) => node.type === 'div' && node.props['data-symbol-file'])
    expect(groups.map((node) => node.props['data-symbol-file'])).toEqual(['src/a.go', 'src/b.go'])
    expect(groups[1].findAll((node) => node.type === 'div' && node.props['data-symbol-change']).map((node) => node.props['data-symbol-change'])).toEqual(['added', 'modified'])
    expect(api.repositories.fileSymbols).not.toHaveBeenCalled()
    const editorButton = groups[0].findAllByType('button').find((node) => node.props['aria-label'] === 'Open in Zed')!
    await act(async () => { await editorButton.props.onClick() })
    expect(api.editor.open).toHaveBeenCalledWith({ editor: 'zed', repo: '/repo/demo', file_path: 'src/a.go', line: 8 })
    const firstLineButton = groups[1].findAllByType('button').find((node) => node.props['aria-label'] === 'Open in Zed')!
    await act(async () => { await firstLineButton.props.onClick() })
    expect(api.editor.open).toHaveBeenCalledWith({ editor: 'zed', repo: '/repo/demo', file_path: 'src/b.go', line: 1 })
    await act(async () => { renderer.unmount() })
  })

  it('changes base and head independently while supporting working-tree targets', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.onChange('working_tree') })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.value).toBe('snapshot:snap-0')
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.value).toBe('working_tree')
    expect(api.repositories.map).not.toHaveBeenCalled()
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

  it('restores targets from the URL and preserves them across settings', async () => {
    searchParamsMock.mockReturnValue(new URLSearchParams('repo=repo-1&base=snapshot:snap-1&head=working_tree&branch=feature'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-base-target' }).props.value).toBe('snapshot:snap-1')
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.value).toBe('working_tree')
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Expand repositories' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-settings-repo-1' }).props.onClick({ stopPropagation: () => {} }) })
    await act(async () => { renderer.root.findByType((await import('./RepositorySettings')).default).props.onBack() })
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-head-target' }).props.value).toBe('working_tree')
    await act(async () => { renderer.unmount() })
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
    vi.mocked(api.repositories.list).mockResolvedValueOnce([first, { ...first, id: 'repo-2', root: '/repo/other', remoteUrl: 'https://github.com/test/other', name: 'other' }])
    let finish!: (value: typeof result) => void
    vi.mocked(api.repositories.compare).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'aria-label': 'Select other' }).props.onClick() })
    await act(async () => { finish(result) })
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.diagram).toBeNull()
    expect(api.repositories.history).toHaveBeenLastCalledWith('repo-2', '', 100)
    renderer.unmount()
  })

  it('loads live state without initiating maps', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    const diagram = await api.repositories.compare('repo-1', { base: {}, head: {} })
    vi.mocked(api.repositories.compare).mockClear()
    vi.mocked(api.repositories.watchLiveImpact).mockImplementationOnce(async (_id, options) => {
      options.onUpdate({ diagram: { ...diagram, comparisonKey: 'live' }, watching: true, error: '', gitBranch: 'main', gitRevision: 'abc' })
      await new Promise<void>((resolve) => options.signal.addEventListener('abort', () => resolve(), { once: true }))
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick() })
    expect(api.repositories.watchLiveImpact).toHaveBeenCalledWith('repo-1', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(api.repositories.liveImpact).not.toHaveBeenCalled()
    expect(api.repositories.compare).not.toHaveBeenCalled()
    expect(api.repositories.map).not.toHaveBeenCalled()
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' }).props.diagram.comparisonKey).toBe('live')
    await act(async () => { renderer.unmount() })
    vi.useRealTimers()
  })

  it('applies successive pushed scenes without remounting and aborts on leaving live mode', async () => {
    const { api } = await import('../api/client')
    const diagram = await api.repositories.compare('repo-1', { base: {}, head: {} })
    let update!: (event: LiveRepositoryImpact) => void
    let signal!: AbortSignal
    vi.mocked(api.repositories.watchLiveImpact).mockImplementationOnce(async (_id, options) => {
      update = options.onUpdate; signal = options.signal
      options.onUpdate({ diagram, watching: true, error: '', gitBranch: 'main', gitRevision: 'abc' })
      await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve(), { once: true }))
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick())
    const canvas = renderer.root.findByProps({ 'data-testid': 'mock-impact' })
    const next = { ...diagram, version: 'v2', comparisonKey: 'next' }
    await act(async () => update({ diagram: next, watching: true, error: '', gitBranch: 'main', gitRevision: 'abc' }))
    expect(renderer.root.findByProps({ 'data-testid': 'mock-impact' })).toBe(canvas)
    expect(canvas.props.diagram).toBe(next)
    await act(async () => update({ diagram: { ...next }, watching: true, error: '', gitBranch: 'main', gitRevision: 'abc' }))
    expect(canvas.props.diagram).toBe(next)
    await act(async () => renderer.unmount())
    expect(signal.aborted).toBe(true)
    expect(api.repositories.liveImpact).not.toHaveBeenCalled()
  })

  it('reconnects a failed live subscription', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.watchLiveImpact).mockRejectedValueOnce(new Error('Disconnected'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick())
    const attempts = vi.mocked(api.repositories.watchLiveImpact).mock.calls.length
    await act(async () => { await vi.advanceTimersByTimeAsync(500) })
    expect(api.repositories.watchLiveImpact).toHaveBeenCalledTimes(attempts + 1)
    await act(async () => renderer.unmount())
    vi.useRealTimers()
  })

  it('starts and stops the watcher from the live panel', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'watch-start' }).props.onClick() })
    expect(api.repositories.startWatch).toHaveBeenCalledWith('repo-1', expect.objectContaining({ materialize: false }))
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

  it.each([
    { state: 'stopping', running: true, label: 'Stopping…' },
    { state: 'stopped', running: false, label: 'Stopped' },
  ])('announces $state once and keeps watcher actions named for their action', async ({ state, running, label }) => {
    const { api } = await import('../api/client')
    const current = await api.repositories.watchStatus('repo-1')
    vi.mocked(api.repositories.watchStatus).mockResolvedValueOnce({ ...current, state, running, stopRequested: running, error: '' })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick())
    const panel = renderer.root.findByProps({ 'data-testid': 'repository-watcher' })
    const text = panel.findAll((node) => typeof node.type === 'string').flatMap((node) => node.children.filter((child): child is string => typeof child === 'string'))
    expect(text.filter((value) => value.startsWith(state === 'stopping' ? 'Stopping' : 'Stopped'))).toEqual([label])
    if (running) {
      expect(panel.findByProps({ 'data-testid': 'watch-stop' }).props.children).toBe('Stop watcher')
      expect(panel.findByProps({ 'data-testid': 'watch-stop' }).props.isDisabled).toBe(true)
      expect(panel.findAllByProps({ 'data-testid': 'mock-spinner' })).toHaveLength(1)
    } else {
      expect(panel.findByProps({ 'data-testid': 'watch-start' }).props.children).toBe('Start watcher')
    }
    await act(async () => renderer.unmount())
  })

  it('shares the follow toggle and pause state with the canvas', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick())
    const follow = () => renderer.root.findByProps({ 'aria-label': 'Follow' })
    expect(follow().props.isChecked).toBe(false)
    const panel = renderer.root.findByProps({ 'data-testid': 'repository-watcher' })
    expect(panel.findByProps({ 'aria-label': 'Follow' })).toBeTruthy()
    await act(async () => follow().props.onChange({ target: { checked: true } }))
    const canvas = () => renderer.root.findByProps({ 'data-testid': 'mock-impact' })
    expect(canvas().props.followMode).toBe('following')
    await act(async () => canvas().props.onFollowModeChange('paused'))
    expect(follow().props.isChecked).toBe(false)
    expect(renderer.root.findAllByType('button').some((button) => button.children.includes('Resume follow'))).toBe(false)
    await act(async () => follow().props.onChange({ target: { checked: true } }))
    expect(canvas().props.followMode).toBe('following')
    await act(async () => follow().props.onChange({ target: { checked: false } }))
    expect(canvas().props.followMode).toBe('off')
    await act(async () => renderer.unmount())
  })

  it('hosts restart and diagnostics in the options popover', async () => {
    const { api } = await import('../api/client')
    const current = await api.repositories.watchStatus('repo-1')
    vi.mocked(api.repositories.watchStatus).mockResolvedValueOnce({ ...current, running: true, state: 'idle' })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-live-tab' }).props.onClick() })
    const panel = renderer.root.findByProps({ 'data-testid': 'repository-watcher' })
    expect(panel.findAll((node) => node.props.as === 'details')).toHaveLength(0)
    expect(panel.findByProps({ 'aria-label': 'Watcher options' })).toBeTruthy()
    const options = panel.findByProps({ 'data-testid': 'watch-options-panel' })
    expect(options.findAll((node) => node.children.includes('Poll interval'))).toHaveLength(0)
    expect(options.findAll((node) => node.children.includes('Debounce')).length).toBeGreaterThan(0)
    vi.mocked(api.repositories.stopWatch).mockClear()
    vi.mocked(api.repositories.startWatch).mockClear()
    await act(async () => { options.findByProps({ 'data-testid': 'watch-restart' }).props.onClick() })
    expect(api.repositories.stopWatch).toHaveBeenCalledWith('repo-1')
    expect(api.repositories.startWatch).toHaveBeenCalledWith('repo-1', { materialize: false })
    expect(vi.mocked(api.repositories.stopWatch).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(api.repositories.startWatch).mock.invocationCallOrder[0])
    await act(async () => { renderer.unmount() })
  })

  it('ignores history and snapshot responses from a previously selected repository', async () => {
    const { api } = await import('../api/client')
    const [first] = await api.repositories.list()
    const saved = await api.repositories.snapshots('repo-1')
    vi.mocked(api.repositories.list).mockResolvedValueOnce([first, { ...first, id: 'repo-2', root: '/repo/other', remoteUrl: 'https://github.com/test/other', name: 'other' }])
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

  it('saves sparse repository overrides and restores inheritance', async () => {
    const { api } = await import('../api/client')
    const settings = await api.repositories.settings('repo-1')
    vi.mocked(api.repositories.updateMapConfiguration).mockResolvedValue(settings)
    searchParamsMock.mockReturnValue(new URLSearchParams('repo=repo-1&page=settings'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-settings-page' }).length).toBeGreaterThan(0)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repository-map-resolution' }).props.onChange({ target: { value: '2.4' } }) })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repository-map-save' }).props.onClick() })
    expect(api.repositories.updateMapConfiguration).toHaveBeenCalledWith('repo-1', { resolution: 2.4 })
    const reset = renderer.root.findAll(node => node.type === 'button' && node.props.children === 'Use global defaults')[0]
    await act(async () => { reset.props.onClick() })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repository-map-save' }).props.onClick() })
    expect(api.repositories.updateMapConfiguration).toHaveBeenLastCalledWith('repo-1', {})
    await act(async () => { renderer.unmount() })
  })

  it('validates overridden root bounds against inherited values', async () => {
    const { api } = await import('../api/client')
    searchParamsMock.mockReturnValue(new URLSearchParams('repo=repo-1&page=settings'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repository-map-minRootGroups' }).props.onChange({ target: { value: '21' } }) })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repository-map-save' }).props.onClick() })
    expect(api.repositories.updateMapConfiguration).not.toHaveBeenCalled()
    expect(renderer.root.findAllByProps({ role: 'alert' }).length).toBeGreaterThan(0)
    await act(async () => { renderer.unmount() })
  })

  it('overrides external import materialization only when changed', async () => {
    const { api } = await import('../api/client')
    const settings = await api.repositories.settings('repo-1')
    vi.mocked(api.repositories.updateMapConfiguration).mockResolvedValue(settings)
    searchParamsMock.mockReturnValue(new URLSearchParams('repo=repo-1&page=settings'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repository-map-includeExternalImports' }).props.onChange({ target: { checked: true } }) })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repository-map-save' }).props.onClick() })
    expect(api.repositories.updateMapConfiguration).toHaveBeenCalledWith('repo-1', { includeExternalImports: true })
    await act(async () => { renderer.unmount() })
  })

  it('shows fetch remotes as read-only repository information without push URLs', async () => {
    const { api } = await import('../api/client')
    const initial = await api.repositories.settings('repo-1')
    const remote = { name: 'origin', fetchUrls: ['https://example.com/repo.git', 'git@example.com:repo.git'], pushUrls: ['ssh://example.com/push.git'] }
    vi.mocked(api.repositories.settings).mockResolvedValueOnce({ ...initial, isGit: true, remotes: [remote] })
    searchParamsMock.mockReturnValue(new URLSearchParams('repo=repo-1&page=settings'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    const remotes = renderer.root.findByProps({ 'data-testid': 'repository-remotes' })
    expect(remotes.findAll(node => node.props.children === remote.fetchUrls[0]).length).toBeGreaterThan(0)
    expect(remotes.findAll(node => node.props.children === remote.fetchUrls[1]).length).toBeGreaterThan(0)
    let section = remotes.parent
    while (section && section.props.as !== 'section') section = section.parent
    expect(section?.findAll(node => node.props.children === 'Repository information').length).toBeGreaterThan(0)
    expect(renderer.root.findAll(node => node.props.children === remote.pushUrls[0])).toHaveLength(0)
    expect(remotes.findAll(node => ['input', 'textarea', 'button'].includes(String(node.type)))).toHaveLength(0)
    expect(renderer.root.findAll(node => node.type === 'button' && ['Add remote', 'Save remote', 'Remove remote'].includes(node.props.children))).toHaveLength(0)
    expect(api.repositories.updateRemote).not.toHaveBeenCalled()
    await act(async () => { renderer.unmount() })
  })

  it('shows the five newest snapshots and loads more on demand', async () => {
    const { api } = await import('../api/client')
    const seven = Array.from({ length: 7 }, (_, index) => ({
      id: `snap-${index}`,
      repositoryId: 'repo-1',
      createdUnix: 100 + index,
      gitRevision: `rev-${index}`,
      gitBranch: 'main',
      ingestionStatus: 'complete', compatible: true,
      projects: [],
      warnings: [],
      provenance: 'commit',
      contentFingerprint: `fp-${index}`,
    }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    vi.mocked(api.repositories.snapshots).mockResolvedValueOnce(seven)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })

    const visible = () => renderer.root
      .findAll((node) => node.type === 'button' && typeof node.props['data-testid'] === 'string' && node.props['data-testid'].startsWith('repositories-snapshot-delete-'))
      .map((node) => node.props['data-testid'] as string)
    expect(visible()).toHaveLength(5)
    expect(visible()).toContain('repositories-snapshot-delete-snap-6')
    expect(visible()).not.toContain('repositories-snapshot-delete-snap-0')

    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-load-more' }).props.onClick() })
    expect(visible()).toHaveLength(7)
    expect(visible()).toContain('repositories-snapshot-delete-snap-0')

    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-show-less' }).props.onClick() })
    expect(visible()).toHaveLength(5)
    renderer.unmount()
  })

  it('collapses the files and symbols panel into a rail that counts files and symbols', async () => {
    const { api } = await import('../api/client')
    const result = await api.repositories.compare('repo-1', { base: {}, head: {} })
    vi.mocked(api.repositories.compare).mockResolvedValueOnce({ ...result, diff: { ...result.diff,
      sources: [
        { path: 'src/file.go', change: 'modified', fromHash: 'old', toHash: 'new' },
        { path: 'src/other.go', change: 'modified', fromHash: 'old', toHash: 'new' },
      ],
      facts: { added: 3, removed: 2, modified: 1 },
    } })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })

    expect(hostNodes(renderer, 'repositories-files-panel')).toHaveLength(1)
    expect(hostNodes(renderer, 'repositories-files-resize')).toHaveLength(1)
    expect(hostNodes(renderer, 'repositories-files-collapsed')).toHaveLength(0)
    const resize = hostNodes(renderer, 'repositories-files-resize')[0]
    expect(resize.props.alignSelf).toBe('stretch')
    expect(divTestIdAncestry(resize).slice(0, 2)).toEqual([
      'repositories-files-resize',
      'repositories-split',
    ])

    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-files-collapse' }).props.onClick() })

    expect(hostNodes(renderer, 'repositories-files-panel')).toHaveLength(0)
    expect(hostNodes(renderer, 'repositories-files-resize')).toHaveLength(0)
    const rail = hostNodes(renderer, 'repositories-files-collapsed')[0]
    expect(rail.props['aria-expanded']).toBe(false)
    expect(rail.props['aria-label']).toBe('Expand files and symbols panel, 2 files, 6 symbols')
    expect(hostNodes(renderer, 'repositories-files-collapsed-count')).toHaveLength(0)
    expect(hostNodes(renderer, 'repositories-symbols-collapsed-count')).toHaveLength(0)

    await act(async () => { rail.props.onClick() })

    expect(hostNodes(renderer, 'repositories-files-panel')).toHaveLength(1)
    expect(hostNodes(renderer, 'repositories-files-collapsed')).toHaveLength(0)
    renderer.unmount()
  })

  it('renders the Snapshots tab as the first repository mode with its list', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    const tabIds = renderer.root
      .findAll((node) => node.type === 'button' && typeof node.props['data-testid'] === 'string' && node.props['data-testid'].endsWith('-tab'))
      .map((node) => node.props['data-testid'] as string)
    expect(tabIds[0]).toBe('repositories-snapshots-tab')
    expect(tabIds).toEqual(['repositories-snapshots-tab', 'repositories-compare-tab', 'repositories-live-tab', 'repositories-pr-tab'])
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    expect(hostNodes(renderer, 'repositories-snapshots-panel')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.type === 'button' && node.props['data-testid'] === 'repositories-snapshot-delete-snap-1')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.type === 'button' && node.props['data-testid'] === 'repositories-snapshot-capture')).toHaveLength(1)
    expect(renderer.root.findAll((node) => node.props.children === 'feat: snapshot message').length).toBeGreaterThan(0)
    await act(async () => { renderer.unmount() })
  })

  it('captures working tree and commit snapshots from the Snapshots tab', async () => {
    const { api } = await import('../api/client')
    const history = await api.repositories.history('repo-1')
    vi.mocked(api.repositories.history).mockResolvedValue({ ...history, isGit: true })
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-capture' }).props.onClick() })
    expect(api.repositories.captureSnapshot).toHaveBeenCalledWith('repo-1', expect.objectContaining({ workingTree: true, signal: expect.any(AbortSignal) }))
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-include-worktree' }).props.onChange({ target: { checked: false } }) })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-capture' }).props.onClick() })
    expect(api.repositories.captureSnapshot).toHaveBeenLastCalledWith('repo-1', expect.objectContaining({ workingTree: false }))
    await act(async () => { renderer.unmount() })
  })

  it('maps a saved snapshot into the workspace from the snapshot details', async () => {
    const { api } = await import('../api/client')
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    expect(renderer.root.findAll((node) => node.type === 'button' && node.props['data-testid'] === 'repositories-snapshot-map-snap-1')).toHaveLength(0)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-view-snap-1' }).props.onClick() })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-map-snap-1' }).props.onClick() })
    expect(api.repositories.map).toHaveBeenCalledWith('repo-1', expect.objectContaining({ snapshotId: 'snap-1', signal: expect.any(AbortSignal) }))
    await act(async () => { renderer.unmount() })
  })

  it('separates opening the shared map from applying a historical snapshot', async () => {
    const { api } = await import('../api/client')
    const result = { snapshotId: 'snap-0', runId: 'run-old', viewId: 5, facts: 4, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 }
    vi.mocked(api.repositories.maps).mockResolvedValue([
      { result: { ...result, snapshotId: 'snap-1', runId: 'run-new' }, completedUnix: 200, configHash: 'cfg' },
      { result, completedUnix: 100, configHash: 'cfg' },
    ])
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-view-snap-0' }).props.onClick() })
    const row = renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-snap-0' })
    expect(row.findAll((node) => node.props.children === 'Previously mapped').length).toBeGreaterThan(0)
    const open = row.findByProps({ 'data-testid': 'repositories-snapshot-open-map-snap-0' })
    expect(open.props.children).toBe('Open workspace map')
    await act(async () => { open.props.onClick() })
    expect(navigateMock).toHaveBeenCalledWith('/views/5')
    expect(api.repositories.map).not.toHaveBeenCalled()
    const apply = row.findByProps({ 'data-testid': 'repositories-snapshot-map-snap-0' })
    expect(apply.props.children).toBe('Apply snapshot again')
    await act(async () => { await apply.props.onClick() })
    expect(api.repositories.map).toHaveBeenCalledWith('repo-1', expect.objectContaining({ snapshotId: 'snap-0' }))
    await act(async () => { renderer.unmount() })
  })

  it.each([false, undefined])('makes incompatible snapshots deletion-only (compatible=%s)', async (compatible) => {
    const { api } = await import('../api/client')
    const snapshots = await api.repositories.snapshots('repo-1')
    const versionedSnapshots = [
      { ...snapshots[0], compatible, incompatibilityReason: 'Snapshot versions are incompatible. Capture a new snapshot.' },
      snapshots[1],
    ]
    vi.mocked(api.repositories.snapshots).mockResolvedValueOnce(versionedSnapshots).mockResolvedValueOnce(versionedSnapshots)
    searchParamsMock.mockReturnValue(new URLSearchParams('base=snapshot:snap-0&head=snapshot:snap-1'))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    const basePicker = renderer.root.findByProps({ 'data-testid': 'repositories-base-target' })
    const snapshotChoices = basePicker.props.groups.flatMap((group: { options: { value: string }[] }) => group.options)
    expect(snapshotChoices.some((option: { value: string }) => option.value === 'snapshot:snap-0')).toBe(false)
    expect(renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.isDisabled).toBe(true)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-compare' }).props.onClick() })
    expect(api.repositories.compare).not.toHaveBeenCalled()
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    const row = renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-snap-0' })
    expect(row.findAll((node) => node.props.children === 'Incompatible').length).toBeGreaterThan(0)
    for (const action of ['view', 'map', 'open-map']) {
      expect(row.findAllByProps({ 'data-testid': `repositories-snapshot-${action}-snap-0` })).toHaveLength(0)
    }
    await act(async () => { row.findByProps({ 'data-testid': 'repositories-snapshot-delete-snap-0' }).props.onClick({ stopPropagation: () => {} }) })
    await act(async () => { await renderer.root.findByProps({ 'data-testid': 'confirm-dialog-confirm' }).props.onClick() })
    expect(api.repositories.deleteSnapshot).toHaveBeenCalledWith('snap-0')
    await act(async () => { renderer.unmount() })
  })

  it('views snapshot metadata from the Snapshots tab', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<Repositories />) })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshots-tab' }).props.onClick() })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-view-snap-1' }).props.onClick() })
    const details = renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-details-snap-1' })
    expect(details.findAll((node) => node.props.children === 'feat: snapshot message').length).toBeGreaterThan(0)
    expect(details.findAll((node) => node.props.children === 'fp-1').length).toBeGreaterThan(0)
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-snapshot-view-snap-1' }).props.onClick() })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-snapshot-details-snap-1' })).toHaveLength(0)
    await act(async () => { renderer.unmount() })
  })

})
