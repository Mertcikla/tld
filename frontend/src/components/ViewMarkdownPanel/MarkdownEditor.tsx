import { useEffect, useMemo, useRef, type MutableRefObject } from 'react'
import { MDXEditor, type MDXEditorMethods } from '@mdxeditor/editor'
import '@mdxeditor/editor/style.css'
import type { MermaidMarkdownSyncStatus } from '../../api/client'
import type { ViewMarkdownDocument } from '../../types'
import { createMarkdownEditorPlugins } from './plugins/createMarkdownEditorPlugins'
import { MermaidMarkdownContext, type MermaidMarkdownContextValue } from './plugins/mermaidContext'

interface MarkdownEditorProps {
  canEditDocument: boolean
  content: string
  currentEditorMarkdown: () => string
  editorRef: MutableRefObject<MDXEditorMethods | null>
  handleSyncMermaidBlock: () => Promise<void> | void
  isDirty: boolean
  isLoading: boolean
  isSaving: boolean
  markdown: ViewMarkdownDocument | null
  mermaidContextValue: MermaidMarkdownContextValue
  mermaidIntegrationEnabled: boolean
  mermaidSyncStatus: MermaidMarkdownSyncStatus | null
  onChange: (markdown: string) => void
  onClose: () => void
  onOpenInEditor?: () => void
  onReload?: () => Promise<void> | void
  onSave: (markdown: string) => Promise<void> | void
  onSaveAs?: (markdown: string) => Promise<void> | void
  syncToken: number
  viewId: number | null
}

export function MarkdownEditor({
  canEditDocument,
  content,
  currentEditorMarkdown,
  editorRef,
  handleSyncMermaidBlock,
  isDirty,
  isLoading,
  isSaving,
  markdown,
  mermaidContextValue,
  mermaidIntegrationEnabled,
  mermaidSyncStatus,
  onChange,
  onClose,
  onOpenInEditor,
  onReload,
  onSave,
  onSaveAs,
  syncToken,
  viewId,
}: MarkdownEditorProps) {
  const lastSyncTokenRef = useRef(syncToken)

  useEffect(() => {
    if (lastSyncTokenRef.current === syncToken) return
    lastSyncTokenRef.current = syncToken
    editorRef.current?.setMarkdown(content)
  }, [content, editorRef, syncToken])

  const plugins = useMemo(() => createMarkdownEditorPlugins({
    canEditDocument,
    currentEditorMarkdown,
    handleSyncMermaidBlock,
    isDirty,
    isLoading,
    isSaving,
    markdown,
    mermaidIntegrationEnabled,
    mermaidSyncStatus,
    onClose,
    onOpenInEditor,
    onReload,
    onSave,
    onSaveAs,
    viewId,
  }), [
    canEditDocument,
    currentEditorMarkdown,
    handleSyncMermaidBlock,
    isDirty,
    isLoading,
    isSaving,
    markdown,
    mermaidIntegrationEnabled,
    mermaidSyncStatus,
    onClose,
    onOpenInEditor,
    onReload,
    onSave,
    onSaveAs,
    viewId,
  ])

  return (
    <MermaidMarkdownContext.Provider value={mermaidContextValue}>
      <MDXEditor
        ref={editorRef}
        markdown={content}
        readOnly={!canEditDocument}
        spellCheck
        className="tld-markdown-editor"
        contentEditableClassName="tld-markdown-editor__content"
        placeholder="Start writing notes for this view…"
        plugins={plugins}
        onChange={(nextMarkdown) => onChange(nextMarkdown)}
      />
    </MermaidMarkdownContext.Provider>
  )
}
