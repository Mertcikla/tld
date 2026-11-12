import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import SourceLinkActions from './SourceLinkActions'
import { sourceActionTarget } from '../utils/sourceActionTarget'
import type { SubSource } from '../utils/subElementSource'

const mocks = vi.hoisted(() => ({ list: vi.fn(), openExternal: vi.fn(), bridge: vi.fn(), requestSource: vi.fn(), peekSource: vi.fn() }))
vi.mock('../utils/subElementSource', () => ({ peekSubSource: mocks.peekSource, requestSubSource: mocks.requestSource, subscribeSubSources: () => () => {} }))
vi.mock('../utils/repositoryResolver', async original => ({ ...await original<typeof import('../utils/repositoryResolver')>(), listIndexedRepositories: mocks.list }))
vi.mock('../lib/desktop', () => ({ openExternalUrl: mocks.openExternal }))
vi.mock('../lib/vscodeBridge', () => ({ vscodeBridge: { postMessage: mocks.bridge } }))
vi.mock('./OpenInEditorButton', () => ({ default: (props: Record<string, unknown>) => <button data-testid="open-editor" {...props} /> }))
vi.mock('@chakra-ui/icons', () => ({ ExternalLinkIcon: () => null }))
vi.mock('@chakra-ui/react', () => ({
  HStack: ({ children, ...props }: { children?: React.ReactNode }) => <div {...props}>{children}</div>,
  Tooltip: ({ children }: { children?: React.ReactNode }) => <>{children}</>,
  IconButton: (props: Record<string, unknown>) => <button {...props} />,
}))
const element = { repo: 'owner/repo', file_path: 'src/api.ts', branch: 'main', url: null }
const indexed = { id: 'indexed', name: 'repo', root: '/workspace/repo', remoteUrl: 'https://github.com/owner/repo', gitBranch: 'main' }
let renderer: ReactTestRenderer
beforeEach(() => { mocks.list.mockResolvedValue([]); vi.stubGlobal('window', { __TLD_VSCODE__: false }) })
afterEach(() => { act(() => renderer?.unmount()); vi.resetAllMocks(); vi.unstubAllGlobals() })

it.each([
  [null, null, 'https://github.com/owner/repo'],
  [null, 'origin/main', 'https://github.com/owner/repo/tree/main'],
  ['src/my api/', 'origin/develop', 'https://github.com/owner/repo/tree/develop/src/my%20api'],
  ['src/api.ts#L12-L20', 'release/v2', 'https://github.com/owner/repo/blob/release%2Fv2/src/api.ts#L12-L20'],
])('keeps repository, folder and anchored file navigation: %s', (file_path, branch, expected) => {
  expect(sourceActionTarget({ ...element, file_path, branch }).githubUrl).toBe(expected)
})

it('uses resolved symbol lines for both GitHub and editor navigation', () => {
  const source: SubSource = { status: 'ready', startLine: 25, endLine: 52, lines: [], tokens: [], symbols: [] }
  const target = sourceActionTarget({ ...element, file_path: 'src/api.ts#function_declaration:find' }, null, source)
  expect(target.githubUrl).toBe('https://github.com/owner/repo/blob/main/src/api.ts#L25-L52')
  expect(target.editorPath).toBe('src/api.ts')
  expect(target.start).toBe(25)
})

it('resolves indexed and local-only repository actions without fetching source', async () => {
  mocks.list.mockResolvedValue([indexed])
  await act(async () => { renderer = create(<SourceLinkActions element={{ ...element, repo: '/workspace/repo', repository_id: 'indexed' }} />) })
  const editor = renderer.root.findByProps({ 'data-testid': 'open-editor' })
  expect(editor.props).toMatchObject({ repo: '/workspace/repo', repositoryId: 'indexed', filePath: 'src/api.ts' })
  await act(async () => renderer.root.findByProps({ 'aria-label': 'Open in GitHub' }).props.onClick())
  expect(mocks.openExternal).toHaveBeenCalledWith('https://github.com/owner/repo/blob/main/src/api.ts')
  expect(sourceActionTarget({ ...element, repo: '/local', file_path: null }).editorPath).toBe('/local')
  expect(sourceActionTarget({ ...element, repo: '/local' }).githubUrl).toBeNull()
})

it('opens anchored files through the extension host in VS Code', async () => {
  vi.stubGlobal('window', { __TLD_VSCODE__: true })
  await act(async () => { renderer = create(<SourceLinkActions element={{ ...element, file_path: 'src/api.ts#function_declaration:find' }} />) })
  await act(async () => renderer.root.findByProps({ 'aria-label': 'Open in editor' }).props.onClick())
  expect(mocks.requestSource).toHaveBeenCalledWith(expect.objectContaining({ file_path: 'src/api.ts#function_declaration:find' }))
  expect(mocks.bridge).toHaveBeenCalledWith({ type: 'open-file', filePath: 'src/api.ts', startLine: undefined, symbolName: 'find', symbolKind: 'function_declaration' })
})

it('resolves symbol actions when the owning element has no loaded preview', async () => {
  mocks.peekSource.mockReturnValue({ status: 'ready', startLine: 25, endLine: 52 })
  await act(async () => { renderer = create(<SourceLinkActions element={{ ...element, file_path: 'src/api.ts#class_declaration:User' }} />) })
  expect(renderer.root.findByProps({ 'data-testid': 'open-editor' }).props.line).toBe(25)
  await act(async () => renderer.root.findByProps({ 'aria-label': 'Open in GitHub' }).props.onClick())
  expect(mocks.openExternal).toHaveBeenCalledWith('https://github.com/owner/repo/blob/main/src/api.ts#L25-L52')
})
