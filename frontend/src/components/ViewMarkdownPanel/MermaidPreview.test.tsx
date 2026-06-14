import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MermaidPreview } from './MermaidPreview'
import { MermaidMarkdownContext } from './mermaidContext'

const mermaidMocks = vi.hoisted(() => ({
  initialize: vi.fn(),
  render: vi.fn(),
}))

vi.mock('mermaid', () => ({
  default: {
    initialize: mermaidMocks.initialize,
    render: mermaidMocks.render,
  },
}))

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('MermaidPreview', () => {
  beforeEach(() => {
    mermaidMocks.initialize.mockClear()
    mermaidMocks.render.mockReset()
    mermaidMocks.render.mockResolvedValue({ svg: '<svg role="img"><text>diagram</text></svg>' })
  })

  it('renders Mermaid SVG previews with sync and view metadata', async () => {
    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(
        <MermaidMarkdownContext.Provider value={{
          blockStatusByCode: new Map([['flowchart TD\n  A --> B', 'synced']]),
          canEdit: true,
          currentViewId: null,
        }}>
          <MermaidPreview code={'flowchart TD\n  A --> B'} />
        </MermaidMarkdownContext.Provider>,
      )
      await flushPromises()
    })

    expect(mermaidMocks.initialize).toHaveBeenCalledWith(expect.objectContaining({
      securityLevel: 'strict',
      startOnLoad: false,
    }))
    expect(mermaidMocks.render).toHaveBeenCalledWith(expect.stringMatching(/^tld-mermaid-/), 'flowchart TD\n  A --> B')
    expect(JSON.stringify(renderer!.toJSON())).toContain('diagram')
    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__title' }).children).toEqual(['Mermaid'])
    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__status tld-mermaid-preview__status--synced' }).children.join('')).toContain('synced')
    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__meta' }).children.join('')).toContain('unlinked')
  })

  it('shows current view metadata when TLD metadata matches the active view', async () => {
    const code = 'flowchart TD\n%% tld/v1 view=6\n  A --> B'
    let renderer: ReturnType<typeof create>

    await act(async () => {
      renderer = create(
        <MermaidMarkdownContext.Provider value={{
          blockStatusByCode: new Map([[code, 'stale']]),
          canEdit: true,
          currentViewId: 6,
          currentViewName: 'Checkout',
        }}>
          <MermaidPreview code={code} />
        </MermaidMarkdownContext.Provider>,
      )
      await flushPromises()
    })

    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__title' }).children).toEqual(['Mermaid'])
    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__status tld-mermaid-preview__status--stale' }).children.join('')).toContain('stale')
    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__meta' }).children.join('')).toContain('current view')
    expect(renderer!.root.findAllByProps({ 'data-testid': 'tld-mermaid-navigate-view' })).toHaveLength(0)
  })

  it('shows a navigate action for TLD Mermaid blocks linked to another view', async () => {
    const code = 'flowchart TD\n%% tld/v1 view=9\n  A --> B'
    const onNavigateToView = vi.fn()
    let renderer: ReturnType<typeof create>

    await act(async () => {
      renderer = create(
        <MermaidMarkdownContext.Provider value={{
          blockStatusByCode: new Map([[code, 'other']]),
          canEdit: true,
          currentViewId: 6,
          onNavigateToView,
        }}>
          <MermaidPreview code={code} />
        </MermaidMarkdownContext.Provider>,
      )
      await flushPromises()
    })

    expect(renderer!.root.findByProps({ 'data-testid': 'tld-mermaid-preview' }).props['data-tld-view-id']).toBe(9)
    const navigateButton = renderer!.root.findByProps({ 'data-testid': 'tld-mermaid-navigate-view' })
    expect(navigateButton.children.join('')).toContain('Navigate to view')

    act(() => {
      navigateButton.props.onClick()
    })

    expect(onNavigateToView).toHaveBeenCalledWith(9)
  })

  it('does not show a navigate action for unlinked Mermaid blocks', async () => {
    let renderer: ReturnType<typeof create>

    await act(async () => {
      renderer = create(
        <MermaidMarkdownContext.Provider value={{
          blockStatusByCode: new Map(),
          canEdit: true,
          currentViewId: 6,
          onNavigateToView: vi.fn(),
        }}>
          <MermaidPreview code="flowchart TD\n  A --> B" />
        </MermaidMarkdownContext.Provider>,
      )
      await flushPromises()
    })

    expect(renderer!.root.findAllByProps({ 'data-testid': 'tld-mermaid-navigate-view' })).toHaveLength(0)
  })

  it('shows Mermaid render errors', async () => {
    mermaidMocks.render.mockRejectedValueOnce(new Error('bad syntax'))
    let renderer: ReturnType<typeof create>

    await act(async () => {
      renderer = create(<MermaidPreview code="flowchart TD\n  A --" />)
      await flushPromises()
    })

    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__error' }).children).toEqual(['bad syntax'])
  })

  it('ignores stale render completions', async () => {
    let resolveFirst: ((value: { svg: string }) => void) | undefined
    mermaidMocks.render
      .mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve }))
      .mockResolvedValueOnce({ svg: '<svg><text>second</text></svg>' })

    let renderer: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<MermaidPreview code="flowchart TD\n  A --> B" />)
      await flushPromises()
    })

    await act(async () => {
      renderer.update(<MermaidPreview code="flowchart TD\n  B --> C" />)
      await flushPromises()
    })

    await act(async () => {
      resolveFirst?.({ svg: '<svg><text>first</text></svg>' })
      await flushPromises()
    })

    const rendered = JSON.stringify(renderer!.toJSON())
    expect(rendered).toContain('second')
    expect(rendered).not.toContain('first')
  })
})
