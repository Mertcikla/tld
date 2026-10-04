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
  return { Badge: Box, Box, Button, Code: Box, Flex: Box, Grid: Box, HStack: Box, Input: Box, Spinner: Box, Text: Box, VStack: Box }
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
})
