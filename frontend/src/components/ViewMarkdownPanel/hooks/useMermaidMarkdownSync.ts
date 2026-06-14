import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, type MermaidMarkdownBlock, type MermaidMarkdownSyncStatus } from '../../../api/client'

interface UseMermaidMarkdownSyncParams {
  canEditDocument: boolean
  content: string
  enabled: boolean
  getMarkdown: () => string
  isOpen: boolean
  replaceMarkdown: (markdown: string) => void
  viewId: number | null
  viewName?: string | null
  onNavigateToView?: (viewId: number) => void
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
  onNavigateToView,
}: UseMermaidMarkdownSyncParams) {
  const [mermaidSyncStatus, setMermaidSyncStatus] = useState<MermaidMarkdownSyncStatus | null>(null)
  const [mermaidBlocks, setMermaidBlocks] = useState<MermaidMarkdownBlock[]>([])
  const [mermaidBlockStatusByCode, setMermaidBlockStatusByCode] = useState<Map<string, MermaidMarkdownSyncStatus>>(() => new Map())

  const applyInspectionResult = useCallback((result: { blocks: MermaidMarkdownBlock[]; syncStatus: MermaidMarkdownSyncStatus }) => {
    setMermaidSyncStatus(result.syncStatus)
    setMermaidBlocks(result.blocks)
    setMermaidBlockStatusByCode(new Map(result.blocks.map((block) => [block.code, block.syncStatusValue])))
  }, [])

  const resetInspectionResult = useCallback(() => {
    setMermaidSyncStatus(null)
    setMermaidBlocks([])
    setMermaidBlockStatusByCode(new Map())
  }, [])

  useEffect(() => {
    if (!isOpen || !enabled || !viewId) {
      resetInspectionResult()
      return undefined
    }

    let canceled = false
    const timer = globalThis.setTimeout(() => {
      void api.mermaid.inspectMarkdown(content, viewId).then((result) => {
        if (canceled) return
        applyInspectionResult(result)
      }).catch(() => {
        if (canceled) return
        resetInspectionResult()
      })
    }, 250)

    return () => {
      canceled = true
      globalThis.clearTimeout(timer)
    }
  }, [applyInspectionResult, content, enabled, isOpen, resetInspectionResult, viewId])

  const currentMermaidBlock = useMemo(() => {
    if (!viewId) return null
    return mermaidBlocks.find((block) => block.viewId === viewId) ?? null
  }, [mermaidBlocks, viewId])

  const otherViewMermaidBlocks = useMemo(() => {
    if (!viewId) return mermaidBlocks.filter((block) => block.viewId != null)
    return mermaidBlocks.filter((block) => block.viewId != null && block.viewId !== viewId)
  }, [mermaidBlocks, viewId])

  const mermaidContextValue = useMemo(() => ({
    blockStatusByCode: mermaidBlockStatusByCode,
    canEdit: canEditDocument,
    currentViewId: viewId,
    currentViewName: viewName,
    onNavigateToView,
  }), [canEditDocument, mermaidBlockStatusByCode, onNavigateToView, viewId, viewName])

  const handleSyncMermaidBlock = useCallback(async () => {
    if (!enabled || !viewId) return
    const currentMarkdown = getMarkdown()
    const result = await api.mermaid.upsertMarkdownBlock(viewId, currentMarkdown, true)
    const nextMarkdown = result.markdown
    replaceMarkdown(nextMarkdown)
    const inspection = await api.mermaid.inspectMarkdown(nextMarkdown, viewId)
    applyInspectionResult(inspection)
  }, [applyInspectionResult, enabled, getMarkdown, replaceMarkdown, viewId])

  return {
    currentMermaidBlock,
    mermaidBlocks,
    mermaidContextValue,
    mermaidSyncStatus,
    otherViewMermaidBlocks,
    handleSyncMermaidBlock,
  }
}
