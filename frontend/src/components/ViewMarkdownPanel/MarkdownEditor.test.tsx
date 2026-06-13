import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ViewMarkdownDocument } from '../../types'
import { MarkdownEditor, type MarkdownEditorHandle } from './MarkdownEditor'

const codeMirrorMock = vi.hoisted(() => {
  let doc = ''
  let selection = { from: 0, to: 0 }
  let onChange: ((value: string) => void) | undefined
  const view = {
    get state() {
      return {
        doc: {
          length: doc.length,
          toString: () => doc,
        },
        selection: {
          main: selection,
        },
      }
    },
    dispatch: vi.fn((transaction: { changes?: { from: number; to: number; insert: string }; selection?: { anchor: number; head?: number } }) => {
      if (transaction.changes) {
        const { from, to, insert } = transaction.changes
        doc = `${doc.slice(0, from)}${insert}${doc.slice(to)}`
        onChange?.(doc)
      }
      if (transaction.selection) {
        selection = {
          from: transaction.selection.anchor,
          to: transaction.selection.head ?? transaction.selection.anchor,
        }
      }
    }),
    focus: vi.fn(),
  }

  return {
    bind(value: string, nextOnChange?: (value: string) => void) {
      doc = value
      onChange = nextOnChange
      selection = { from: value.length, to: value.length }
    },
    change(value: string) {
      doc = value
      onChange?.(value)
    },
    reset() {
      doc = ''
      onChange = undefined
      selection = { from: 0, to: 0 }
      view.dispatch.mockClear()
      view.focus.mockClear()
    },
    setSelection(from: number, to = from) {
      selection = { from, to }
    },
    view,
  }
})

vi.mock('@uiw/react-codemirror', async () => {
  const ReactModule = await import('react')
  return {
    default: ReactModule.forwardRef(function MockCodeMirror(props: {
      value: string
      readOnly?: boolean
      onChange?: (value: string) => void
    }, ref: React.ForwardedRef<{ view: typeof codeMirrorMock.view }>) {
      codeMirrorMock.bind(props.value, props.onChange)
      ReactModule.useImperativeHandle(ref, () => ({ view: codeMirrorMock.view }))
      return ReactModule.createElement('textarea', {
        'data-testid': 'mock-codemirror',
        readOnly: props.readOnly,
        value: props.value,
        onChange: (event: React.ChangeEvent<HTMLTextAreaElement>) => codeMirrorMock.change(event.currentTarget.value),
      })
    }),
  }
})

vi.mock('@chakra-ui/react', async (importOriginal) => {
  const ReactModule = await import('react')
  const actual = await importOriginal<typeof import('@chakra-ui/react')>()
  return {
    ...actual,
    Tooltip: ({ children }: { children: React.ReactNode }) => ReactModule.createElement(ReactModule.Fragment, null, children),
  }
})

vi.mock('@codemirror/lang-markdown', () => ({
  markdown: vi.fn(() => 'markdown-extension'),
  markdownLanguage: {},
}))
vi.mock('@codemirror/language-data', () => ({ languages: [] }))
vi.mock('@codemirror/theme-one-dark', () => ({ oneDark: 'one-dark' }))
vi.mock('@codemirror/view', () => ({ EditorView: { lineWrapping: 'line-wrapping' } }))
vi.mock('../../config/runtime', () => ({ isWailsApp: false }))

function renderEditor(overrides: Partial<React.ComponentProps<typeof MarkdownEditor>> = {}) {
  const editorRef = { current: null } as React.MutableRefObject<MarkdownEditorHandle | null>
  const props: React.ComponentProps<typeof MarkdownEditor> = {
    canEditDocument: true,
    content: '# Title\n\n\nParagraph\n',
    editorRef,
    handleSyncMermaidBlock: vi.fn(),
    isDirty: true,
    isLoading: false,
    isSaving: false,
    markdown: { exists: true, can_edit: true, path: '/tmp/README.md' } as ViewMarkdownDocument,
    mermaidContextValue: { blockStatusByCode: new Map(), canEdit: true, currentViewId: 6, currentViewName: 'Test View' },
    mermaidSyncStatus: 'stale',
    onChange: vi.fn(),
    onClose: vi.fn(),
    onReload: vi.fn(),
    onSave: vi.fn(),
    viewId: 6,
    ...overrides,
  }
  const renderer = create(<MarkdownEditor {...props} />)
  return { editorRef, props, renderer }
}

describe('MarkdownEditor', () => {
  beforeEach(() => {
    codeMirrorMock.reset()
  })

  it('saves the exact CodeMirror string without markdown normalization', () => {
    const nextMarkdown = '# Title\n\n\nParagraph\n\n```mermaid\nflowchart TD\n  A --> B\n```\n'
    const onChange = vi.fn()
    const onSave = vi.fn()
    const { renderer } = renderEditor({ onChange, onSave })

    act(() => {
      renderer.root.findByProps({ 'data-testid': 'mock-codemirror' }).props.onChange({
        currentTarget: { value: nextMarkdown },
      })
    })
    act(() => {
      renderer.root.findByProps({ 'aria-label': 'Save' }).props.onClick()
    })

    expect(onChange).toHaveBeenCalledWith(nextMarkdown)
    expect(onSave).toHaveBeenCalledWith(nextMarkdown)
  })

  it('uses a single current-view Mermaid block injection action instead of the generic insert menu', () => {
    const handleSyncMermaidBlock = vi.fn()
    const { renderer } = renderEditor({ handleSyncMermaidBlock })
    const syncButton = renderer.root.findByProps({ 'data-testid': 'markdown-insert-view-mermaid-button' })

    act(() => {
      syncButton.props.onClick()
    })

    expect(handleSyncMermaidBlock).toHaveBeenCalled()
    expect(syncButton.props.children).toBe('Update Mermaid')
    expect(syncButton.props['aria-label']).toBe('Update current view Mermaid block')
    expect(renderer.root.findAllByProps({ 'data-testid': 'markdown-insert-button' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Search markdown' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Previous match' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'aria-label': 'Next match' })).toHaveLength(0)
  })

  it('displays a synced Mermaid status when the current view block is up to date', () => {
    const { renderer } = renderEditor({ mermaidSyncStatus: 'synced' })
    const syncButton = renderer.root.findByProps({ 'data-testid': 'markdown-insert-view-mermaid-button' })

    expect(syncButton.props.children).toBe('Mermaid synced')
    expect(syncButton.props['aria-label']).toBe('Current view Mermaid block is synced')
    expect(syncButton.props.isDisabled).toBe(true)
  })

  it('disables editing controls when the markdown document is read-only', () => {
    const { renderer } = renderEditor({ canEditDocument: false })

    expect(renderer.root.findByProps({ 'data-testid': 'mock-codemirror' }).props.readOnly).toBe(true)
    expect(renderer.root.findByProps({ 'aria-label': 'Save' }).props.isDisabled).toBe(true)
    expect(renderer.root.findByProps({ 'data-testid': 'markdown-insert-view-mermaid-button' }).props.isDisabled).toBe(true)
  })
})
