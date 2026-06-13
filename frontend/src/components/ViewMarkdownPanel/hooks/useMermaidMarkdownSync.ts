import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, type MermaidMarkdownSyncStatus } from '../../../api/client'

interface UseMermaidMarkdownSyncParams {
  canEditDocument: boolean
  content: string
  enabled: boolean
  getMarkdown: () => string
  isOpen: boolean
  replaceMarkdown: (markdown: string) => void
  viewId: number | null
  viewName?: string | null
}

export function useMermaidMarkdownSync({
  canEditDocument,
  content,
  enabled,
  getMarkdown,
  isOpen,
  replaceMarkdown,
  viewId,
  viewName,
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
    currentViewId: viewId,
    currentViewName: viewName,
  }), [canEditDocument, mermaidBlockStatusByCode, viewId, viewName])

  const handleSyncMermaidBlock = useCallback(async () => {
    if (!viewId) return
    const currentMarkdown = getMarkdown()
    const result = await api.mermaid.upsertMarkdownBlock(viewId, currentMarkdown, true)
    const nextMarkdown = result.markdown
    replaceMarkdown(nextMarkdown)
    setMermaidSyncStatus('synced')
  }, [getMarkdown, replaceMarkdown, viewId])

  return {
    mermaidContextValue,
    mermaidSyncStatus,
    handleSyncMermaidBlock,
  }
}
