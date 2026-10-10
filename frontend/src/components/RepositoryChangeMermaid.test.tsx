import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { copyTextToClipboard } from '../utils/clipboard'
import type { SceneMermaidView } from '../utils/sceneMermaid'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

vi.mock('../utils/clipboard', () => ({ copyTextToClipboard: vi.fn(async () => {}) }))
vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('./ViewMarkdownPanel/MarkdownPreview', () => ({
  MarkdownPreview: ({ markdown }: { markdown: string }) => React.createElement('pre', { 'data-testid': 'mock-markdown' }, markdown),
}))
vi.mock('./ViewMarkdownPanel/styles', () => ({ markdownPanelBodySx: {} }))
vi.mock('@chakra-ui/icons', () => ({ CheckIcon: () => null, ChevronLeftIcon: () => null, CopyIcon: () => null }))

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

const view: SceneMermaidView = {
  data: {
    tree: [{ id: 51, name: 'Mine', description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children: [] }],
    views: {
      51: {
        placements: [{
          id: 7, element_id: 7, view_id: 51, position_x: 0, position_y: 0, name: 'svc/auth',
          description: null, kind: 'component', technology: null, url: null, logo_url: null,
          technology_connectors: [], tags: [], repo: null, repository_id: null, branch: null,
          file_path: 'svc/auth', language: null, bypass_noise_gate: false, has_view: false, view_label: null,
        }],
        connectors: [],
      },
    },
    navigations: [],
  },
  overlays: { 7: { change: 'modified', linesAdded: 3, linesRemoved: 1 } },
  provenance: { 7: 'authored' },
}

function renderPane(overrides: Partial<React.ComponentProps<typeof RepositoryChangeMermaid>> = {}) {
  return create(
    <RepositoryChangeMermaid
      repositoryId="repo-1"
      comparisonKey="key-1"
      radius={0}
      scope="grounded"
      view={view}
      open
      {...overrides}
    />,
  )
}

/** Counts only rendered DOM nodes, ignoring the mocked Chakra components themselves. */
function hostNodes(renderer: ReturnType<typeof create>, testId: string) {
  return renderer.root
    .findAllByProps({ 'data-testid': testId })
    .filter((node) => typeof node.type === 'string')
}

describe('RepositoryChangeMermaid', () => {
  beforeEach(() => {
    vi.mocked(copyTextToClipboard).mockClear()
  })

  it('renders the canvas view as mermaid without fetching', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane()
    })

    const markdown = renderer.root.findByProps({ 'data-testid': 'mock-markdown' }).props.children as string
    expect(markdown).toContain('```mermaid')
    expect(markdown).toContain('%% tld-scene repo=repo-1 key=key-1 scope=grounded radius=0')
    expect(markdown).toContain('subgraph view_51["Mine"]')
    expect(markdown).toContain('svc/auth<br/>modified +3 −1')
  })

  it('copies the change diagram as markdown', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane()
    })

    await act(async () => {
      renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-copy' }).props.onClick()
      await Promise.resolve()
    })

    expect(copyTextToClipboard).toHaveBeenCalledWith(expect.stringContaining('subgraph view_51'))
  })

  it('renders nothing while closed', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane({ open: false })
    })

    expect(renderer.root.findAllByProps({ 'data-testid': 'mock-markdown' })).toHaveLength(0)
  })

  it('renders the collapsed rail with only the Markdown expand button', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = renderPane({ collapsed: true })
    })

    expect(renderer.root.findByProps({ 'data-testid': 'repository-change-mermaid-expand' })).toBeTruthy()
    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-change-mermaid' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'repository-change-mermaid-copy' })).toHaveLength(0)
  })

  it('offers a way back to the docked form while covering the canvas', async () => {
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
})
