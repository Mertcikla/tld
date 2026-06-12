import { useCallback, useEffect, useMemo, useState, type MutableRefObject } from 'react'
import type { MDXEditorMethods } from '@mdxeditor/editor'
import { api, type MermaidMarkdownSyncStatus } from '../../../api/client'

interface UseMermaidMarkdownSyncParams {
  canEditDocument: boolean
  content: string
  editorRef: MutableRefObject<MDXEditorMethods | null>
  enabled: boolean
  isOpen: boolean
  latestContentRef: MutableRefObject<string>
  onChange: (markdown: string) => void
  viewId: number | null
}

export function useMermaidMarkdownSync({
  canEditDocument,
  content,
  editorRef,
  enabled,
  isOpen,
  latestContentRef,
  onChange,
  viewId,
}: UseMermaidMarkdownSyncParams) {
  const [mermaidSyncStatus, setMermaidSyncStatus] = useState<MermaidMarkdownSyncStatus | null>(null)
  const [mermaidBlockStatusByCode, setMermaidBlockStatusByCode] = useState<Map<string, MermaidMarkdownSyncStatus>>(() => new Map())

  useEffect(() => {
    if (!isOpen || !enabled || !viewId) {
      setMermaidSyncStatus(null)
      setMermaidBlockStatusByCode(new Map())
      return undefined
    }

    let canceled = false
    const timer = window.setTimeout(() => {
      void api.mermaid.inspectMarkdown(content, viewId).then((result) => {
        if (canceled) return
        setMermaidSyncStatus(result.syncStatus)
        setMermaidBlockStatusByCode(new Map(result.blocks.map((block) => [block.code, block.syncStatusValue])))
      }).catch(() => {
        if (canceled) return
        setMermaidSyncStatus(null)
        setMermaidBlockStatusByCode(new Map())
      })
    }, 250)

    return () => {
      canceled = true
      window.clearTimeout(timer)
    }
  }, [content, enabled, isOpen, viewId])

  const mermaidContextValue = useMemo(() => ({
    blockStatusByCode: mermaidBlockStatusByCode,
    canEdit: canEditDocument,
  }), [canEditDocument, mermaidBlockStatusByCode])

  const handleSyncMermaidBlock = useCallback(async () => {
    if (!viewId) return
    const currentMarkdown = editorRef.current?.getMarkdown() ?? latestContentRef.current
    const result = await api.mermaid.upsertMarkdownBlock(viewId, currentMarkdown, true)
    const nextMarkdown = result.markdown
    latestContentRef.current = nextMarkdown
    editorRef.current?.setMarkdown(nextMarkdown)
    setMermaidSyncStatus('synced')
    onChange(nextMarkdown)
  }, [editorRef, latestContentRef, onChange, viewId])

  return {
    mermaidContextValue,
    mermaidSyncStatus,
    handleSyncMermaidBlock,
  }
}
