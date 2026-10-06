import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import { copyTextToClipboard } from '../utils/clipboard'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

vi.mock('../api/client', () => ({ api: { repositories: { impactMermaid: vi.fn() } } }))
vi.mock('../utils/clipboard', () => ({ copyTextToClipboard: vi.fn(async () => {}) }))
vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('./ViewMarkdownPanel/MarkdownPreview', () => ({
  MarkdownPreview: ({ markdown }: { markdown: string }) => React.createElement('pre', { 'data-testid': 'mock-markdown' }, markdown),
}))
vi.mock('./ViewMarkdownPanel/styles', () => ({ markdownPanelBodySx: {} }))
vi.mock('@chakra-ui/icons', () => ({ CheckIcon: () => null, CopyIcon: () => null }))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  const ButtonLike = ({ children, onClick, isDisabled, ...props }: { children?: React.ReactNode; onClick?: () => void; isDisabled?: boolean }) =>
    ReactModule.createElement('button', { ...props, disabled: isDisabled, onClick }, children)
  return {
    Box: BoxLike,
    Button: ButtonLike,
    Flex: BoxLike,
    Spinner: BoxLike,
    Text: BoxLike,
  }
})

const markdown = '```mermaid\nflowchart LR\n  a --> b\n```\n'

function renderPane(overrides: Partial<React.ComponentProps<typeof RepositoryChangeMermaid>> = {}) {
  return create(<RepositoryChangeMermaid repositoryId="repo-1" comparisonKey="key-1" radius={0} open {...overrides} />)
}

describe('RepositoryChangeMermaid', () => {
  beforeEach(() => {
    vi.mocked(api.repositories.impactMermaid).mockReset()
    vi.mocked(copyTextToClipboard).mockClear()
  })

  it('loads and previews the change diagram markdown', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: 'flowchart LR', markdown, warnings: [] })
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane()
    })

    expect(api.repositories.impactMermaid).toHaveBeenCalledWith('repo-1', 'key-1', expect.objectContaining({ markdown: true, radius: 0 }))
    expect(renderer.root.findByProps({ 'data-testid': 'mock-markdown' }).props.children).toContain('flowchart LR')
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

  it('does not fetch while closed', async () => {
    vi.mocked(api.repositories.impactMermaid).mockResolvedValue({ code: '', markdown: '', warnings: [] })
    await act(async () => {
      renderPane({ open: false })
    })

    expect(api.repositories.impactMermaid).not.toHaveBeenCalled()
  })
})
