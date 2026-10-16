import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import type { RepositoryCommit, RepositoryGitHistory } from '../api/client'
import RepositoryHistory from './RepositoryHistory'

vi.mock('../api/client', () => ({ api: { repositories: { commitDetails: vi.fn(async () => ({ commit: null, files: [] })) } } }))
vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const Box = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const Button = ({ children, isDisabled, ...props }: { children?: React.ReactNode; isDisabled?: boolean }) => ReactModule.createElement('button', { ...props, disabled: isDisabled }, children)
  return { Badge: Box, Box, Button, Code: Box, Flex: Box, Grid: Box, HStack: Box, Input: Box, InputGroup: Box, InputRightElement: Box, Spinner: Box, Text: Box, VStack: Box }
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
