import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { copyTextToClipboard } from '../utils/clipboard'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

vi.mock('../api/client', () => ({ api: { repositories: { impactMermaid: vi.fn() } } }))
vi.mock('../utils/clipboard', () => ({ copyTextToClipboard: vi.fn(async () => {}) }))
vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('./ViewMarkdownPanel/MarkdownPreview', async () => {
  const { MermaidHeaderActionsContext } = await import('./ViewMarkdownPanel/mermaidContext')
  return {
    MarkdownPreview: ({ markdown }: { markdown: string }) => React.createElement('div', null,
      React.useContext(MermaidHeaderActionsContext),
      React.createElement('pre', { 'data-testid': 'mock-markdown' }, markdown)),
  }
})
vi.mock('./ViewMarkdownPanel/styles', () => ({ markdownPanelBodySx: {} }))
vi.mock('@chakra-ui/icons', () => ({ CheckIcon: () => null, ChevronRightIcon: () => null, CopyIcon: () => null }))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const ButtonLike = ({ children, onClick, isDisabled, ...props }: { children?: React.ReactNode; onClick?: () => void; isDisabled?: boolean }) =>
    ReactModule.createElement('button', { ...props, disabled: isDisabled, onClick }, children)
  const IconButtonLike = ({ onClick, isDisabled, ...props }: { onClick?: () => void; isDisabled?: boolean }) =>
    ReactModule.createElement('button', { ...props, disabled: isDisabled, onClick })
  const TooltipLike = ({ children }: { children?: React.ReactNode }) => ReactModule.createElement('div', null, children)
  return {
    Box: BoxLike,
    Button: ButtonLike,
    Flex: BoxLike,
    IconButton: IconButtonLike,
    Spinner: BoxLike,
    Text: BoxLike,
    Tooltip: TooltipLike,
  }
})

const markdown = '```mermaid\nflowchart LR\n```\n'

function renderPane(overrides: Partial<React.ComponentProps<typeof RepositoryChangeMermaid>> = {}) {
  return create(<RepositoryChangeMermaid repositoryId="repo-1" comparisonKey="key-1" plain={false} open {...overrides} />)
}

/** Counts only rendered DOM nodes, ignoring the mocked Chakra components themselves. */
function hostNodes(renderer: ReturnType<typeof create>, testId: string) {
  return renderer.root
    .findAllByProps({ 'data-testid': testId })
    .filter((node) => typeof node.type === 'string')
}

describe('RepositoryChangeMermaid', () => {
  beforeEach(() => {
    vi.mocked(api.repositories.impactMermaid).mockReset()
    vi.mocked(copyTextToClipboard).mockClear()
  })

  it('fetches the backend-rendered scene diagram and previews it', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({
      code: 'flowchart LR',
      markdown,
      warnings: [],
    })
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane()
    })

    expect(api.repositories.impactMermaid).toHaveBeenCalledWith('repo-1', 'key-1', expect.objectContaining({ markdown: true, plain: false }))
    expect(renderer.root.findByProps({ 'data-testid': 'mock-markdown' }).props.children).toContain('flowchart LR')
  })

  it('forwards plain mode so the pane mirrors the canvas toggle', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: 'flowchart LR', markdown, warnings: [] })
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane({ plain: true })
    })

    expect(api.repositories.impactMermaid).toHaveBeenCalledWith('repo-1', 'key-1', expect.objectContaining({ plain: true }))
    expect(renderer.root.findByProps({ 'data-testid': 'mock-markdown' })).toBeTruthy()
  })

  it('copies the change diagram as markdown', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: 'flowchart LR', markdown, warnings: [] })
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane()
    })

    await act(async () => {
      renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-copy' }).props.onClick()
      await Promise.resolve()
    })

    expect(copyTextToClipboard).toHaveBeenCalledWith(markdown)
  })

  it('offers bounded zoom controls and resets when the comparison changes', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = renderPane({ embeddedMarkdown: markdown }) })
    const zoomIn = () => renderer.root.findByProps({ 'aria-label': 'Zoom in diagram' })
    const zoomOut = () => renderer.root.findByProps({ 'aria-label': 'Zoom out diagram' })
    const reset = () => renderer.root.findByProps({ 'aria-label': 'Reset diagram zoom' })

    act(() => { zoomIn().props.onClick() })
    expect(reset().props.children).toEqual([125, '%'])
    act(() => { reset().props.onClick() })
    expect(reset().props.children).toEqual([100, '%'])
    act(() => { for (let i = 0; i < 20; i++) zoomIn().props.onClick() })
    expect(zoomIn().props.isDisabled).toBe(true)
    expect(reset().props.children).toEqual([400, '%'])
    act(() => { for (let i = 0; i < 20; i++) zoomOut().props.onClick() })
    expect(zoomOut().props.isDisabled).toBe(true)
    expect(reset().props.children).toEqual([50, '%'])
    await act(async () => { renderer.update(<RepositoryChangeMermaid repositoryId="repo-1" comparisonKey="key-2" plain={false} open embeddedMarkdown={`${markdown}\nUpdated comparison`} />) })
    expect(reset().props.children).toEqual([100, '%'])
    renderer.unmount()
  })

  it('does not fetch while closed', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: '', markdown: '', warnings: [] })
    await act(async () => {
      renderPane({ open: false })
    })

    expect(api.repositories.impactMermaid).not.toHaveBeenCalled()
  })

  it('renders the collapsed rail with only the Markdown expand button', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: 'flowchart LR', markdown, warnings: [] })
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane({ collapsed: true })
    })

    expect(renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-expand' })).toBeTruthy()
    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-change-mermaid' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-change-mermaid-copy' })).toHaveLength(0)
  })

  it('keeps the markdown loaded while collapsed and expands from the rail', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: 'flowchart LR', markdown, warnings: [] })
    const onExpand = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane({ collapsed: true, onExpand })
    })

    expect(api.repositories.impactMermaid).toHaveBeenCalled()
    expect(copyTextToClipboard).not.toHaveBeenCalled()

    await act(async () => {
      renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-expand' }).props.onClick()
    })
    expect(onExpand).toHaveBeenCalled()
  })

  it('offers a collapse control while covering the canvas', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: 'flowchart LR', markdown, warnings: [] })
    const onDock = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane({ overlay: true, onDock })
    })

    expect(hostNodes(renderer, 'repository-change-mermaid-dock')).toHaveLength(1)
    expect(hostNodes(renderer, 'repository-change-mermaid')).toHaveLength(1)

    await act(async () => {
      renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-dock' }).props.onClick()
    })
    expect(onDock).toHaveBeenCalled()
  })

  it('keeps the collapse control when docked beside the canvas', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: 'flowchart LR', markdown, warnings: [] })
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane()
    })

    expect(hostNodes(renderer, 'repository-change-mermaid-dock')).toHaveLength(1)
    expect(hostNodes(renderer, 'repository-change-mermaid')).toHaveLength(1)
  })
})
