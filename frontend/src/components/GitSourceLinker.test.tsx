import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LibraryElement } from '../types'
import GitSourceLinker from './GitSourceLinker'

const mocks = vi.hoisted(() => ({
  listRepositories: vi.fn(), history: vi.fn(), files: vi.fn(), source: vi.fn(),
  githubRequest: vi.fn(), getParser: vi.fn(), openExternal: vi.fn(),
}))
vi.mock('../api/client', () => ({ api: {
  repositories: { history: mocks.history, files: mocks.files }, editor: { source: mocks.source },
} }))
vi.mock('../utils/repositoryResolver', async (importOriginal) => ({
  ...await importOriginal<typeof import('../utils/repositoryResolver')>(),
  listIndexedRepositories: mocks.listRepositories,
}))
vi.mock('../lib/desktop', () => ({ openExternalUrl: mocks.openExternal }))
vi.mock('../utils/githubApi', () => ({ githubRequest: mocks.githubRequest }))
vi.mock('../utils/treesitter', async (importOriginal) => ({
  ...await importOriginal<typeof import('../utils/treesitter')>(),
  getParser: mocks.getParser,
  extractSymbols: () => [{ name: 'Start', qualifiedName: 'Start', type: 'function_declaration', startLine: 1, endLine: 2 }],
}))
vi.mock('react-dom', () => ({ createPortal: (children: React.ReactNode) => children }))
vi.mock('framer-motion', () => ({
  AnimatePresence: ({ children }: { children: React.ReactNode }) => children,
  motion: { div: ({ children }: { children: React.ReactNode }) => <div>{children}</div> },
}))
vi.mock('@chakra-ui/icons', () => ({ CheckIcon: () => null, ChevronRightIcon: () => null, ExternalLinkIcon: () => null, SearchIcon: () => null }))
vi.mock('@chakra-ui/react', () => {
  const Box = React.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(({ children, ...props }, ref) => <div {...props} ref={ref}>{children}</div>)
  Box.displayName = 'Box'
  const Input = React.forwardRef<HTMLInputElement, Record<string, unknown>>((props, ref) => <input {...props} ref={ref} />)
  Input.displayName = 'Input'
  return {
    Alert: Box, AlertDescription: Box, AlertIcon: Box, Badge: Box, Box, CloseButton: Box,
    FormControl: Box, FormLabel: Box, HStack: Box, Input, InputGroup: Box, InputRightElement: Box,
    Progress: Box, Spinner: Box, Text: Box, Tooltip: Box, VStack: Box,
    Accordion: Box, AccordionItem: Box, AccordionButton: Box, AccordionPanel: Box, AccordionIcon: Box,
    Portal: Box, Popover: Box, PopoverTrigger: Box, PopoverContent: Box, PopoverBody: Box,
    Button: ({ children, ...props }: { children?: React.ReactNode }) => <button {...props}>{children}</button>,
    useToast: () => vi.fn(),
  }
})
const baseElement: LibraryElement = {
  id: 10, name: 'API', kind: 'service', description: null, technology: null,
  url: null, logo_url: null, technology_connectors: [], tags: [], repo: null,
  branch: null, file_path: null, language: null, created_at: '', updated_at: '',
  has_view: false, view_label: null, bypass_noise_gate: false,
}
let renderer: ReactTestRenderer
async function renderLinker(overrides: Partial<LibraryElement> = {}, isReadOnly = false) {
  const onUpdate = vi.fn()
  await act(async () => {
    renderer = create(<GitSourceLinker element={{ ...baseElement, ...overrides }} isReadOnly={isReadOnly} onUpdate={onUpdate} />)
  })
  return onUpdate
}
function button(label: string) {
  return renderer.root.findAllByType('button').find(node => node.props.children === label)!
}
async function click(label: string) {
  await act(async () => { button(label).props.onClick({ stopPropagation: vi.fn() }) })
}
async function typeInput(value: string) {
  await act(async () => { renderer.root.findByType('input').props.onChange({ target: { value } }) })
}
async function select(label: string) {
  await act(async () => { renderer.root.findAllByType('div').find(node => node.props.children === label && node.props.onClick)!.props.onClick() })
}
async function openPaths(overrides: Partial<LibraryElement> = {}) {
  const onUpdate = await renderLinker({ repo: 'owner/repo', branch: 'main', ...overrides })
  await click('Edit')
  await click('Next')
  await click('Next')
  return onUpdate
}

describe('GitSourceLinker link granularity', () => {
  beforeEach(() => {
    vi.stubGlobal('document', { body: {} })
    mocks.listRepositories.mockResolvedValue([{ id: 'indexed', remoteUrl: 'https://github.com/owner/repo', root: '/workspace/repo', name: 'repo', latestSnapshotId: 'snapshot' }])
    mocks.history.mockResolvedValue({ branches: [{ name: 'main' }, { name: 'develop' }], currentBranch: 'main' })
    mocks.files.mockResolvedValue([{ anchor: { path: 'src/api/main.go' } }, { anchor: { path: 'README.md' } }])
    mocks.source.mockResolvedValue({ content: 'func Start() {}' })
    mocks.getParser.mockResolvedValue({ parse: vi.fn() })
  })
  afterEach(() => {
    act(() => { renderer?.unmount() })
    vi.resetAllMocks()
    vi.unstubAllGlobals()
  })
  it.each([
    ['src/api/main.go', 'blob/main/src/api/main.go'],
    ['src/api/', 'tree/main/src/api/'],
  ])('opens the summary GitHub link on the remote branch: %s', async (file_path, target) => {
    await renderLinker({ repo: 'owner/repo', branch: 'origin/main', file_path })
    await click('Open in GitHub')
    expect(mocks.openExternal).toHaveBeenCalledWith(`https://github.com/owner/repo/${target}`)
  })
  it('links a repository immediately without fetching branches or files', async () => {
    const onUpdate = await renderLinker()
    await typeInput('https://github.com/owner/repo.git')
    await click('Link repository')
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ repo: 'owner/repo', branch: null, file_path: null, language: null }))
    expect(mocks.history).not.toHaveBeenCalled()
    expect(mocks.files).not.toHaveBeenCalled()
  })
  it('saves the selected branch as a repository link and clears old anchors', async () => {
    const onUpdate = await renderLinker({ repo: 'owner/repo', branch: 'main', file_path: 'src/api/main.go#function_declaration:Start', language: 'go' })
    await click('Edit')
    await click('Next')
    await select('develop')
    await click('Link repository')
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ repo: 'owner/repo', branch: 'develop', file_path: null, language: null }))
    expect(mocks.files).not.toHaveBeenCalled()
  })
  it('offers nested folders and links one without requesting source or symbols', async () => {
    const onUpdate = await openPaths()
    await select('src/api/')
    expect(button('Next').props.isDisabled).toBe(true)
    await click('Link folder')
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ repository_id: 'indexed', file_path: 'src/api/', language: null }))
    expect(mocks.source).not.toHaveBeenCalled()
    expect(mocks.getParser).not.toHaveBeenCalled()
  })
  it('saves the typed file instead of a previously linked symbol', async () => {
    const onUpdate = await openPaths({ file_path: 'old.go#function_declaration:Old', language: 'go', tags: ['external', 'keep'] })
    await typeInput('  src/api/main.go  ')
    await click('Link file')
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ file_path: 'src/api/main.go', language: 'go', tags: ['keep'] }))
    expect(mocks.source).not.toHaveBeenCalled()
  })
  it('continues with the typed file and still saves a symbol anchor', async () => {
    const onUpdate = await openPaths({ file_path: 'old.go#function_declaration:Old' })
    await typeInput('src/api/main.go')
    await click('Next')
    expect(mocks.source).toHaveBeenCalledWith(expect.objectContaining({ file_path: 'src/api/main.go' }))
    await act(async () => {
      renderer.root.findAllByType('div').find(node => node.props.onClick && node.findAllByType('div').some(child => child.props.children === 'Start'))!.props.onClick()
    })
    await click('Apply')
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ file_path: 'src/api/main.go#function_declaration:Start', language: 'go' }))
  })
  it('still saves line anchors for unsupported files', async () => {
    const onUpdate = await openPaths()
    await typeInput('README.md')
    await click('Next')
    await act(async () => { renderer.root.findAllByType('div').find(node => node.props['data-line'] === 1)!.props.onClick() })
    await click('Apply')
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ file_path: 'README.md#L1', language: null }))
  })
  it('allows manually entered folders when GitHub cannot load a tree', async () => {
    mocks.listRepositories.mockResolvedValue([])
    mocks.githubRequest.mockResolvedValue({ status: 429 })
    const onUpdate = await openPaths()
    await typeInput('private/module/')
    await click('Link folder')
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ file_path: 'private/module/', language: null }))
    expect(mocks.source).not.toHaveBeenCalled()
  })
  it('disables saving when read only or the repository is invalid', async () => {
    await renderLinker()
    expect(button('Link repository').props.isDisabled).toBe(true)
    act(() => { renderer.unmount() })
    const onUpdate = await renderLinker({}, true)
    expect(button('Link repository').props.isDisabled).toBe(true)
    await click('Link repository')
    expect(onUpdate).not.toHaveBeenCalled()
  })
})
