import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ViewMarkdownPanel from './index'

const mountedRenderers: ReactTestRenderer[] = []

function renderPanel(overrides: Partial<React.ComponentProps<typeof ViewMarkdownPanel>> = {}) {
  const props: React.ComponentProps<typeof ViewMarkdownPanel> = {
    isOpen: true,
    onClose: vi.fn(),
    viewName: 'Customer Portal',
    markdown: null,
    content: '',
    syncToken: 0,
    canEdit: true,
    onChange: vi.fn(),
    onSave: vi.fn(),
    ...overrides,
  }
  let renderer!: ReactTestRenderer
  act(() => {
    renderer = create(<ViewMarkdownPanel {...props} />)
  })
  mountedRenderers.push(renderer)
  return { props, renderer }
}

describe('ViewMarkdownPanel', () => {
  afterEach(() => {
    for (const renderer of mountedRenderers.splice(0)) {
      act(() => {
        renderer.unmount()
      })
    }
  })

  it('hides file-oriented setup actions in DB-only mode', () => {
    const onCreateMarkdown = vi.fn()
    const { renderer } = renderPanel({ dbOnlyNotes: true, onCreateMarkdown })

    expect(renderer.root.findAllByProps({ 'data-testid': 'view-markdown-repo-path' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'view-markdown-create-repo' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'view-markdown-attach-path' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'view-markdown-attach' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'view-markdown-choose-file' })).toHaveLength(0)

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'view-markdown-create-private' }).props.onClick()
    })

    expect(onCreateMarkdown).toHaveBeenCalledWith('PRIVATE_APP')
  })

  it('keeps repo and attach setup actions in the default mode', () => {
    const onCreateMarkdown = vi.fn()
    const { renderer } = renderPanel({ onCreateMarkdown, onAttachMarkdown: vi.fn() })

    expect(renderer.root.findByProps({ 'data-testid': 'view-markdown-repo-path' })).toBeTruthy()
    expect(renderer.root.findByProps({ 'data-testid': 'view-markdown-create-repo' })).toBeTruthy()
    expect(renderer.root.findByProps({ 'data-testid': 'view-markdown-attach-path' })).toBeTruthy()
    expect(renderer.root.findByProps({ 'data-testid': 'view-markdown-attach' })).toBeTruthy()

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'view-markdown-create-private' }).props.onClick()
    })

    expect(onCreateMarkdown).toHaveBeenCalledWith('PRIVATE_WORKSPACE')
  })
})
