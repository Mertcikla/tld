import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { PlacedElement } from '../types'
import CodePreviewPanel from './CodePreviewPanel'

const mocks = vi.hoisted(() => ({ list: vi.fn(), source: vi.fn(), fetch: vi.fn(), openExternal: vi.fn(), visibility: vi.fn() }))
vi.mock('../api/client', () => ({ api: { editor: { source: mocks.source } } }))
vi.mock('../utils/repositoryResolver', async (original) => ({
  ...await original<typeof import('../utils/repositoryResolver')>(),
  listIndexedRepositories: mocks.list,
}))
vi.mock('../utils/githubApi', () => ({ getGithubRepoVisibility: mocks.visibility }))
vi.mock('../utils/githubCache', () => ({ githubCache: {
  getRepoPublic: () => true, getContent: () => null, setContent: vi.fn(),
} }))
vi.mock('../lib/desktop', () => ({ openExternalUrl: mocks.openExternal }))
vi.mock('./OpenInEditorButton', () => ({ default: (props: Record<string, unknown>) => <button data-testid="open-editor" {...props} /> }))
vi.mock('./SlidingPanel', () => ({ default: ({ children, isOpen }: { children: React.ReactNode; isOpen: boolean }) => isOpen ? <div>{children}</div> : null }))
vi.mock('@uiw/react-codemirror', () => ({
  default: React.forwardRef<unknown, Record<string, unknown>>(function CodeMirror(props, _ref) {
    return <div data-testid="code-editor" {...props} />
  }),
}))
vi.mock('@chakra-ui/icons', () => ({ ExternalLinkIcon: () => null }))
vi.mock('@chakra-ui/react', () => {
  const Box = ({ children, ...props }: { children?: React.ReactNode }) => <div {...props}>{children}</div>
  return {
    Box, HStack: Box, VStack: Box, Text: Box, Tooltip: Box, Icon: Box, Spinner: Box,
    Button: ({ children, ...props }: { children?: React.ReactNode }) => <button {...props}>{children}</button>,
    CloseButton: (props: Record<string, unknown>) => <button {...props} />,
  }
})

const element: PlacedElement = {
  id: 1, view_id: 1, element_id: 10, position_x: 0, position_y: 0,
  name: 'API', description: null, kind: 'service', technology: null, url: null,
  logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null,
  repo: 'owner/repo', file_path: null,
}
const indexedRepo = {
  id: 'indexed', name: 'repo', root: '/workspace/repo', remoteUrl: 'https://github.com/owner/repo', gitBranch: 'main',
}
let renderer: ReactTestRenderer
const onClose = vi.fn()
async function renderPanel(overrides: Partial<PlacedElement> = {}) {
  await act(async () => { renderer = create(<CodePreviewPanel isOpen onClose={onClose} element={{ ...element, ...overrides }} />) })
}
async function openGithub() {
  await act(async () => { renderer.root.findByProps({ 'aria-label': 'Open in GitHub' }).props.onClick() })
}
function expectSummary(label: string) {
  expect(renderer.root.findAllByProps({ 'data-testid': 'code-editor' })).toHaveLength(0)
  expect(renderer.root.findAllByType('div').some(node => node.props.children === label)).toBe(true)
  expect(mocks.source).not.toHaveBeenCalled()
  expect(mocks.fetch).not.toHaveBeenCalled()
  expect(mocks.visibility).not.toHaveBeenCalled()
}

describe('CodePreviewPanel source targets', () => {
  beforeEach(() => {
    vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() })
    vi.stubGlobal('fetch', mocks.fetch)
    mocks.list.mockResolvedValue([])
    mocks.source.mockResolvedValue({ content: 'package api' })
    mocks.fetch.mockResolvedValue({ ok: true, text: async () => 'package api' })
  })
  afterEach(() => {
    act(() => { renderer?.unmount() })
    vi.resetAllMocks()
    vi.unstubAllGlobals()
  })

  it.each([
    [null, 'https://github.com/owner/repo'],
    ['origin/main', 'https://github.com/owner/repo/tree/main'],
    ['release/v2', 'https://github.com/owner/repo/tree/release%2Fv2'],
  ])('shows a repository summary and opens its selected branch: %s', async (branch, url) => {
    await renderPanel({ branch })
    expectSummary('Repository link')
    await openGithub()
    expect(mocks.openExternal).toHaveBeenCalledWith(url)
  })

  it.each(['develop', 'origin/develop'])('opens a folder on remote branch %s without fetching code', async (branch) => {
    await renderPanel({ file_path: 'src/my api/', branch })
    expectSummary('Folder link')
    expect(renderer.root.findAllByType('div').some(node => node.props.children === 'src/my api/')).toBe(true)
    await openGithub()
    expect(mocks.openExternal).toHaveBeenCalledWith('https://github.com/owner/repo/tree/develop/src/my%20api')
    expect(renderer.root.findByProps({ 'data-testid': 'open-editor' }).props.filePath).toBe('src/my api/')
  })

  it.each([null, 'src/api/'])('supports indexed repository and folder links: %s', async (file_path) => {
    mocks.list.mockResolvedValue([indexedRepo])
    await renderPanel({ repo: '/workspace/repo', repository_id: 'indexed', file_path })
    expectSummary(file_path ? 'Folder link' : 'Repository link')
    const editor = renderer.root.findByProps({ 'data-testid': 'open-editor' })
    expect(editor.props.filePath).toBe(file_path || '/workspace/repo')
    expect(editor.props.repositoryId).toBe('indexed')
    await openGithub()
    expect(mocks.openExternal).toHaveBeenCalledWith(file_path ? 'https://github.com/owner/repo/tree/main/src/api' : 'https://github.com/owner/repo')
  })

  it('keeps local-only repositories usable without a GitHub action', async () => {
    await renderPanel({ repo: '/workspace/local' })
    expectSummary('Repository link')
    expect(renderer.root.findAllByProps({ 'aria-label': 'Open in GitHub' })).toHaveLength(0)
    expect(renderer.root.findByProps({ 'data-testid': 'open-editor' }).props.filePath).toBe('/workspace/local')
  })

  it.each(['main', 'origin/main', 'refs/remotes/origin/main'])('loads files and opens line ranges on remote branch %s', async (branch) => {
    await renderPanel({ file_path: 'src/main.go#L30-L38', branch })
    expect(mocks.fetch).toHaveBeenCalledWith('https://raw.githubusercontent.com/owner/repo/refs/heads/main/src/main.go')
    expect(renderer.root.findByProps({ 'data-testid': 'code-editor' }).props.value).toBe('package api')
    expect(renderer.root.findByProps({ 'data-testid': 'open-editor' }).props.branch).toBe(branch)
    await openGithub()
    expect(mocks.openExternal).toHaveBeenCalledWith('https://github.com/owner/repo/blob/main/src/main.go#L30-L38')
  })

  it('removes the code editor and old errors when switching from a file to a folder', async () => {
    mocks.list.mockResolvedValue([indexedRepo])
    mocks.source.mockRejectedValue(new Error('File unavailable'))
    await renderPanel({ repo: '/workspace/repo', file_path: 'old.go' })
    expect(mocks.source).toHaveBeenCalledTimes(1)
    await act(async () => {
      renderer.update(<CodePreviewPanel isOpen onClose={onClose} element={{ ...element, repo: '/workspace/repo', file_path: 'src/api/' }} />)
    })
    expect(mocks.source).toHaveBeenCalledTimes(1)
    expect(renderer.root.findAllByProps({ 'data-testid': 'code-editor' })).toHaveLength(0)
    expect(renderer.root.findAllByType('div').some(node => node.props.children === 'File unavailable')).toBe(false)
  })
})
