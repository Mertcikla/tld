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

  it('renders Mermaid SVG previews and sync status', async () => {
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
    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__status tld-mermaid-preview__status--synced' }).children.join('')).toContain('synced')
  })

  it('uses the current view name when TLD metadata matches the active view', async () => {
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

    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__title' }).children).toEqual(['Checkout'])
    expect(renderer!.root.findByProps({ className: 'tld-mermaid-preview__meta' }).children.join('')).toContain('current view')
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
