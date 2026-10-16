import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RepositoryCommit, RepositoryGitHistory } from '../api/client'
import RepositoryHistory from './RepositoryHistory'

vi.mock('../api/client', () => ({ api: { repositories: {
  rangeSummary: vi.fn(async () => ({ commits: 1, additions: 0, deletions: 0, changed: 0, files: 0 })),
  commitDetails: vi.fn(async () => ({ commit: null, files: [] })),
  history: vi.fn(async (): Promise<RepositoryGitHistory> => ({ commits: [], branches: [], currentBranch: 'main', headSha: 'newer', isGit: true, hasMore: false })),
  historyFilters: vi.fn(async () => ({ authors: ['Test <test@example.com>'], tags: ['v1'] })),
} } }))
vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const Box = ReactModule.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(({ children, ...props }, ref) => ReactModule.createElement('div', { ...props, ref }, children))
  const Button = ({ children, isDisabled, ...props }: { children?: React.ReactNode; isDisabled?: boolean }) => ReactModule.createElement('button', { ...props, disabled: isDisabled }, children)
  return { Badge: Box, Box, Button, Code: Box, Flex: Box, Grid: Box, HStack: Box, Input: Box, InputGroup: Box, InputLeftElement: Box, InputRightElement: Box, Select: Box, Spinner: Box, Text: Box, VStack: Box }
})
const commits: RepositoryCommit[] = ['newer', 'middle', 'older', 'oldest'].map((sha, index) => ({
  sha, subject: sha, author: 'Test', authorEmail: '', createdUnix: 100 - index,
  parents: index < 3 ? [['middle', 'older', 'oldest'][index]] : [], refs: [], body: '',
}))
const history: RepositoryGitHistory = { commits, branches: [], currentBranch: 'main', headSha: 'newer', isGit: true, hasMore: false }

function renderHistory(base: string, head: string, onRange = vi.fn(), disabled = false) {
  let renderer!: ReturnType<typeof create>
  act(() => { renderer = create(<RepositoryHistory repositoryId="repo" history={history} base={base} head={head} collapsed={false} onToggle={() => {}} onRange={onRange} disabled={disabled} />) })
  return { renderer, onRange }
}

describe('RepositoryHistory selection', () => {
  afterEach(() => { vi.useRealTimers(); vi.clearAllMocks() })
  it('searches beyond loaded commits, displays only matches, and combines author and tag filters', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    const olderMatch = { ...commits[3], sha: 'outside-first-page', subject: 'Old fix' }
    vi.mocked(api.repositories.history).mockResolvedValueOnce({ ...history, commits: [olderMatch] })
    let renderer!: ReturnType<typeof create>
    const onBranchChange = vi.fn()
    await act(async () => { renderer = create(<RepositoryHistory repositoryId="repo" history={history} base="" head="" collapsed={false} onToggle={() => {}} onRange={vi.fn()} onBranchChange={onBranchChange} />) })
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.onChange({ target: { value: 'Old fix' } }) })
    expect(api.repositories.history).not.toHaveBeenCalled()
    await act(async () => { await vi.advanceTimersByTimeAsync(300) })
    expect(api.repositories.history).toHaveBeenLastCalledWith('repo', '', 100, { query: 'Old fix', author: '', tag: '', signal: expect.any(AbortSignal) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'commit-row-newer' })).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'commit-row-outside-first-page' })).toBeTruthy()
    expect(renderer.root.findAllByProps({ role: 'status' }).some((node) => JSON.stringify(node.props.children).includes('matching '))).toBe(true)
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter history by author' }).props.onChange({ target: { value: 'Test <test@example.com>' } }) })
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter history by tag' }).props.onChange({ target: { value: 'v1' } }) })
    await act(async () => { await vi.advanceTimersByTimeAsync(300) })
    expect(api.repositories.history).toHaveBeenLastCalledWith('repo', '', 100, { query: 'Old fix', author: 'Test <test@example.com>', tag: 'v1', signal: expect.any(AbortSignal) })
    expect(renderer.root.findAll((node) => node.props.children === 'No matching commits.').length).toBeGreaterThan(0)
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter history by branch' }).props.onChange({ target: { value: 'feature' } }) })
    expect(onBranchChange).toHaveBeenCalledWith('feature')
    act(() => { renderer.root.findAllByType('button').find((node) => node.props.children === 'Clear filters')!.props.onClick() })
    expect(renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.value).toBe('')
    expect(renderer.root.findByProps({ 'aria-label': 'Filter history by author' }).props.value).toBe('')
    expect(renderer.root.findByProps({ 'aria-label': 'Filter history by tag' }).props.value).toBe('')
    expect(renderer.root.findByProps({ 'data-testid': 'commit-row-newer' })).toBeTruthy()
    act(() => { renderer.unmount() })
  })
  it('cancels obsolete searches and auto-loads the next page of matching commits', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    let finish!: (next: RepositoryGitHistory) => void
    vi.mocked(api.repositories.history).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryHistory repositoryId="repo" history={history} base="" head="" collapsed={false} onToggle={() => {}} onRange={vi.fn()} />) })
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.onChange({ target: { value: 'old' } }) })
    await act(async () => { await vi.advanceTimersByTimeAsync(300) })
    const signal = vi.mocked(api.repositories.history).mock.calls[0][3]?.signal
    expect(signal?.aborted).toBe(false)
    vi.mocked(api.repositories.history).mockResolvedValueOnce({ ...history, commits: commits.slice(0, 1), hasMore: true, nextPageToken: 'more-matches' })
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.onChange({ target: { value: 'new' } }) })
    expect(signal?.aborted).toBe(true)
    await act(async () => { finish({ ...history, commits: [commits[3]] }); await vi.advanceTimersByTimeAsync(300) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'commit-row-oldest' })).toHaveLength(0)
    vi.mocked(api.repositories.history).mockResolvedValueOnce({ ...history, commits: [commits[1]], hasMore: false, nextPageToken: '' })
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'repositories-history-scroll' }).props.onScroll({ currentTarget: { scrollTop: 760, scrollHeight: 1000, clientHeight: 240 } }) })
    expect(api.repositories.history).toHaveBeenLastCalledWith('repo', '', 100, { query: 'new', author: '', tag: '', pageToken: 'more-matches', signal: expect.any(AbortSignal) })
    expect(renderer.root.findByProps({ 'data-testid': 'commit-row-newer' })).toBeTruthy()
    expect(renderer.root.findByProps({ 'data-testid': 'commit-row-middle' })).toBeTruthy()
    act(() => { renderer.unmount() })
  })
  it('keeps the first commit across filtering, empty results, and clearing filters', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    // Topological order must win even if commit timestamps are skewed.
    const skewedHistory = { ...history, commits: commits.map((commit, i) => ({ ...commit, createdUnix: i })) }
    const onRange = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryHistory repositoryId="repo" history={skewedHistory} base="" head="" collapsed={false} onToggle={() => {}} onRange={onRange} />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-newer' }).props.onClick() })
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.onChange({ target: { value: 'no match' } }) })
    await act(async () => { await vi.advanceTimersByTimeAsync(300) })
    expect(onRange).toHaveBeenCalledTimes(1)
    vi.mocked(api.repositories.history).mockResolvedValueOnce({ ...history, commits: [skewedHistory.commits[2]] })
    act(() => {
      renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.onChange({ target: { value: '' } })
      renderer.root.findByProps({ 'aria-label': 'Filter history by author' }).props.onChange({ target: { value: 'Test <test@example.com>' } })
      renderer.root.findByProps({ 'aria-label': 'Filter history by tag' }).props.onChange({ target: { value: 'v1' } })
    })
    await act(async () => { await vi.advanceTimersByTimeAsync(300) })
    act(() => { renderer.root.findAllByType('button').find((node) => node.props.children === 'Clear filters')!.props.onClick() })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-older' }).props.onClick() })
    expect(onRange).toHaveBeenLastCalledWith(skewedHistory.commits[2], skewedHistory.commits[0])
    expect(api.repositories.rangeSummary).not.toHaveBeenCalled()
    act(() => { renderer.unmount() })
  })

  it.each([0, 3])('pairs commits from separate search results using Git range ordering (%i commits)', async (count) => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    const first = { ...commits[0], sha: 'first-outside-page' }
    const second = { ...commits[3], sha: 'second-outside-page' }
    const onRange = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryHistory repositoryId="repo" history={history} base="" head="" collapsed={false} onToggle={() => {}} onRange={onRange} />) })
    const searchFor = async (commit: RepositoryCommit) => {
      vi.mocked(api.repositories.history).mockResolvedValueOnce({ ...history, commits: [commit] })
      act(() => { renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.onChange({ target: { value: commit.sha } }) })
      await act(async () => { await vi.advanceTimersByTimeAsync(300) })
    }
    await searchFor(first)
    act(() => { renderer.root.findByProps({ 'data-testid': `commit-row-${first.sha}` }).props.onClick() })
    await searchFor(second)
    vi.mocked(api.repositories.rangeSummary).mockResolvedValueOnce({ commits: count, additions: 0, deletions: 0, changed: 0, files: 0 })
    await act(async () => { renderer.root.findByProps({ 'data-testid': `commit-row-${second.sha}` }).props.onClick() })
    expect(api.repositories.rangeSummary).toHaveBeenLastCalledWith('repo', { base: first.sha, head: second.sha, signal: expect.any(AbortSignal) })
    expect(onRange).toHaveBeenLastCalledWith(count ? first : second, count ? second : first)
    expect(onRange).toHaveBeenCalledTimes(2)
    // After completion, a click begins a fresh pair.
    act(() => { renderer.root.findByProps({ 'data-testid': `commit-row-${second.sha}` }).props.onClick() })
    expect(onRange).toHaveBeenLastCalledWith(second, second)
    act(() => { renderer.unmount() })
  })

  it.each([['other', 'main'], ['third', 'feature']])('forgets the pending selection when changing to repository %s on branch %s', async (repositoryId, branch) => {
    const onRange = vi.fn()
    let renderer!: ReturnType<typeof create>
    const render = (repositoryId: string, branch: string) => <RepositoryHistory repositoryId={repositoryId} branch={branch} history={history} base="" head="" collapsed={false} onToggle={() => {}} onRange={onRange} />
    await act(async () => { renderer = create(render('repo', 'main')) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-newer' }).props.onClick() })
    await act(async () => { renderer.update(render(repositoryId, branch)) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-older' }).props.onClick() })
    expect(onRange).toHaveBeenLastCalledWith(commits[2], commits[2])
    act(() => { renderer.unmount() })
  })

  it('preserves both branch contexts when completing a range after changing branch', async () => {
    const onRange = vi.fn()
    let renderer!: ReturnType<typeof create>
    const render = (branch: string) => <RepositoryHistory repositoryId="repo" branch={branch} history={history} base="" head="" collapsed={false} onToggle={() => {}} onRange={onRange} />
    await act(async () => { renderer = create(render('main')) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-newer' }).props.onClick() })
    await act(async () => { renderer.update(render('feature')) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-older' }).props.onClick() })
    expect(onRange).toHaveBeenLastCalledWith(commits[2], commits[0], { base: 'feature', head: 'main' })
    act(() => { renderer.unmount() })
  })

  it('retains the first selection on range errors and cancels resolution when switching repository', async () => {
    vi.useFakeTimers()
    const { api } = await import('../api/client')
    const outside = { ...commits[3], sha: 'outside' }
    const onRange = vi.fn()
    let renderer!: ReturnType<typeof create>
    const render = (repositoryId: string) => <RepositoryHistory repositoryId={repositoryId} history={history} base="" head="" collapsed={false} onToggle={() => {}} onRange={onRange} />
    await act(async () => { renderer = create(render('repo')) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-newer' }).props.onClick() })
    vi.mocked(api.repositories.history).mockResolvedValueOnce({ ...history, commits: [outside] })
    act(() => { renderer.root.findByProps({ 'aria-label': 'Filter commits' }).props.onChange({ target: { value: 'outside' } }) })
    await act(async () => { await vi.advanceTimersByTimeAsync(300) })
    vi.mocked(api.repositories.rangeSummary).mockRejectedValueOnce(new Error('Range unavailable'))
    await act(async () => { renderer.root.findByProps({ 'data-testid': 'commit-row-outside' }).props.onClick() })
    expect(renderer.root.findAllByProps({ role: 'alert' }).some((node) => node.props.children === 'Range unavailable')).toBe(true)
    expect(onRange).toHaveBeenCalledTimes(1)
    let finish!: (value: Awaited<ReturnType<typeof api.repositories.rangeSummary>>) => void
    vi.mocked(api.repositories.rangeSummary).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-outside' }).props.onClick() })
    const signal = vi.mocked(api.repositories.rangeSummary).mock.lastCall![1].signal!
    expect(signal.aborted).toBe(false)
    await act(async () => { renderer.update(render('other')) })
    expect(signal.aborted).toBe(true)
    await act(async () => { finish({ commits: 0, additions: 0, deletions: 0, changed: 0, files: 0 }) })
    expect(onRange).toHaveBeenCalledTimes(1)
    act(() => { renderer.unmount() })
  })

  it('loads older commits at the bottom and keeps a pending range across appended pages', () => {
    const onLoadMore = vi.fn(), onRange = vi.fn()
    const firstPage = { ...history, commits: commits.slice(0, 2), hasMore: true, nextPageToken: 'next' }
    let renderer!: ReturnType<typeof create>
    const render = (nextHistory: RepositoryGitHistory, loadingMore = false, loadMoreError = '') => (
      <RepositoryHistory repositoryId="repo" history={nextHistory} base="" head="" collapsed={false} onToggle={() => {}} onRange={onRange} onLoadMore={onLoadMore} loadingMore={loadingMore} loadMoreError={loadMoreError} />
    )
    act(() => { renderer = create(render(firstPage)) })
    expect(renderer.root.findAllByProps({ 'data-testid': 'repositories-history-load-more' })).toHaveLength(0)
    const scroll = (scrollTop: number) => renderer.root.findByProps({ 'data-testid': 'repositories-history-scroll' }).props.onScroll({ currentTarget: { scrollTop, scrollHeight: 1000, clientHeight: 240 } })
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-newer' }).props.onClick() })
    act(() => { scroll(500) })
    expect(onLoadMore).not.toHaveBeenCalled()
    act(() => { scroll(760) })
    expect(onLoadMore).toHaveBeenCalledOnce()
    act(() => { renderer.update(render(firstPage, true)) })
    act(() => { scroll(760) })
    expect(onLoadMore).toHaveBeenCalledOnce()
    expect(renderer.root.findAll((node) => node.props.children === 'Loading older commits…').length).toBeGreaterThan(0)
    act(() => { renderer.update(render(firstPage, false, 'Try again')) })
    expect(renderer.root.findAllByProps({ role: 'alert' }).length).toBeGreaterThan(0)
    act(() => { scroll(500); scroll(760) })
    expect(onLoadMore).toHaveBeenCalledTimes(2)
    act(() => { renderer.update(render(history)) })
    act(() => { scroll(760) })
    expect(onLoadMore).toHaveBeenCalledTimes(2)
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-older' }).props.onClick() })
    expect(onRange).toHaveBeenLastCalledWith(commits[2], commits[0])
    act(() => { renderer.unmount() })
  })
  it('assigns the older selection to base regardless of click order', () => {
    for (const [first, second] of [['newer', 'older'], ['older', 'newer']]) {
      const { renderer, onRange } = renderHistory('', '')
      act(() => { renderer.root.findByProps({ 'data-testid': `commit-row-${first}` }).props.onClick() })
      expect(onRange).toHaveBeenLastCalledWith(expect.objectContaining({ sha: first }), expect.objectContaining({ sha: first }))
      act(() => { renderer.root.findByProps({ 'data-testid': `commit-row-${second}` }).props.onClick() })
      expect(onRange).toHaveBeenLastCalledWith(commits[2], commits[0])
      act(() => { renderer.unmount() })
    }
  })

  it('highlights the inclusive range and extends the graph divider through the full history', () => {
    const { renderer } = renderHistory('older', 'newer')
    for (const sha of ['newer', 'middle', 'older']) expect(renderer.root.findByProps({ 'data-testid': `commit-row-${sha}` }).props.bg).toBe('rgba(var(--accent-rgb), 0.12)')
    expect(renderer.root.findByProps({ 'data-testid': 'commit-row-oldest' }).props.bg).toBeUndefined()
    expect(renderer.root.findByProps({ 'data-testid': 'commit-row-middle' }).props.flexShrink).toBe(0)
    expect(renderer.root.findAllByType('rect')).toHaveLength(3)
    expect(renderer.root.findAllByProps({ borderRight: '1px solid' })[0].props.h).toBe(`${commits.length * 36}px`)
    act(() => { renderer.unmount() })
  })

  it('keeps inspect clicks from selecting the row', () => {
    const { renderer, onRange } = renderHistory('older', 'middle')
    const inspect = renderer.root.findAllByType('button').find((node) => node.props['aria-label'] === 'Inspect older')!
    const event = { stopPropagation: vi.fn() }
    act(() => { inspect.props.onClick(event) })
    expect(event.stopPropagation).toHaveBeenCalled()
    expect(onRange).not.toHaveBeenCalled()
    act(() => { renderer.unmount() })
  })

  it('supports keyboard row selection and ignores clicks while busy', () => {
    const { renderer, onRange } = renderHistory('', '')
    const preventDefault = vi.fn(), target = {}
    act(() => { renderer.root.findByProps({ 'data-testid': 'commit-row-middle' }).props.onKeyDown({ key: 'Enter', target, currentTarget: target, preventDefault }) })
    expect(onRange).toHaveBeenCalledWith(commits[1], commits[1])
    expect(preventDefault).toHaveBeenCalled()
    act(() => { renderer.unmount() })
    const blocked = renderHistory('', '', vi.fn(), true)
    act(() => { blocked.renderer.root.findByProps({ 'data-testid': 'commit-row-middle' }).props.onClick() })
    expect(blocked.onRange).not.toHaveBeenCalled()
    act(() => { blocked.renderer.unmount() })
  })

  it('toggles from the summary topbar', () => {
    const onToggle = vi.fn()
    let renderer!: ReturnType<typeof create>
    act(() => { renderer = create(<RepositoryHistory repositoryId="repo" history={history} base="" head="" collapsed={false} onToggle={onToggle} onRange={vi.fn()} />) })
    act(() => { renderer.root.findByProps({ 'data-testid': 'repositories-history-summary' }).props.onClick() })
    expect(onToggle).toHaveBeenCalledTimes(1)
    act(() => { renderer.unmount() })
  })
})
